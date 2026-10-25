#!/usr/bin/env bash
# Proves scripts/install.sh and the Update Notted installer in temp folders, with fake macOS and
# build tools first on PATH (or named by the NOTTED_* overrides).
# It never touches /Applications, ~/.notted, the keychain or a running Notted.
set -uo pipefail
cd "$(dirname "$0")/.."
root=$(pwd -P)
pass=0
fail=0
tmps=()
cleanup() { for t in "${tmps[@]:-}"; do [ -n "$t" ] && rm -rf "$t"; done; }
trap cleanup EXIT

MODULE=github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
STOPPED='make install stopped: fix the missing items above, then run make install again.'
UNCHANGED='Install failed; your previous Notted is unchanged.'

# fake <name> <body>: writes an executable fake into $T/bin.
fake() {
  printf '#!/bin/sh\n%s\n' "$2" >"$T/bin/$1"
  chmod +x "$T/bin/$1"
}

# fake_permission_tools: codesign reads a designated requirement from <app>/Contents/req (unsigned
# when the file is missing), PlistBuddy prints the Info.plist file as the bundle id, and tccutil only
# records its call (and fails with "boom" when $T/tccutil-fails exists).
fake_permission_tools() {
  fake codesign 'for a; do app=$a; done
if ! r=$(cat "$app/Contents/req" 2>/dev/null); then echo "$app: code object is not signed at all" >&2; exit 1; fi
echo "Executable=$app/Contents/MacOS/Notted"
echo "designated => $r"'
  fake plistbuddy 'echo "plistbuddy $*" >>"$T_FAKE/calls"; cat "$3"'
  fake tccutil 'echo "tccutil $*" >>"$T_FAKE/calls"
if [ -e "$T_FAKE/tccutil-fails" ]; then echo boom; exit 1; fi'
}

# fresh: a temp world with a fake clone, fake tools and the default "everything present" state.
fresh() {
  T=$(mktemp -d)
  T=$(cd "$T" && pwd -P)
  tmps+=("$T")
  CLONE="$T/my clone"
  APPS="$T/Apps dir"
  NHOME="$T/notted home"
  mkdir -p "$T/bin" "$CLONE/scripts" "$T/gopath/bin" "$T/hdir"
  cp "$root/scripts/install.sh" "$CLONE/scripts/install.sh" 2>/dev/null || true
  git init -q "$CLONE"
  export T_FAKE=$T
  : >"$T/calls"
  echo 14.5 >"$T/macos"
  echo arm64 >"$T/arch"
  echo go1.26.5 >"$T/goversion"
  echo v24.16.0 >"$T/nodeversion"
  echo v2.12.0 >"$T/wailsversion"
  : >"$T/has-claude"
  fake sw_vers 'echo "sw_vers $*" >>"$T_FAKE/calls"; cat "$T_FAKE/macos"'
  fake uname 'cat "$T_FAKE/arch"'
  fake xcrun 'echo "xcrun $*" >>"$T_FAKE/calls"; exit 0'
  fake go 'echo "go $*" >>"$T_FAKE/calls"
case "$1 $2" in
  "env GOVERSION") cat "$T_FAKE/goversion" ;;
  "env GOPATH") echo "$T_FAKE/gopath" ;;
  "env GOBIN") cat "$T_FAKE/gobin" 2>/dev/null ;;
  "install "*)
    if [ -e "$T_FAKE/go-install-fails" ]; then exit 1; fi
    d=$(cat "$T_FAKE/gobin" 2>/dev/null)
    [ -n "$d" ] || d=$T_FAKE/gopath/bin
    mkdir -p "$d"
    printf "#!/bin/sh\ncat \"%s/wailsversion\"\n" "$T_FAKE" >"$d/wails"
    chmod +x "$d/wails" ;;
esac'
  fake node 'cat "$T_FAKE/nodeversion"'
  fake wails 'cat "$T_FAKE/wailsversion"'
  fake osascript 'echo "osascript $*" >>"$T_FAKE/calls"; [ -e "$T_FAKE/stuck" ] || rm -f "$T_FAKE/running"; exit 0'
  fake pgrep '[ -e "$T_FAKE/running" ]'
  fake open 'echo "open $*" >>"$T_FAKE/calls"'
  fake make 'echo "make $*" >>"$T_FAKE/calls"
d=$2
mkdir -p "$d/build/bin/Notted.app/Contents/MacOS"
echo new-build >"$d/build/bin/Notted.app/Contents/MacOS/Notted"
echo com.example.test >"$d/build/bin/Notted.app/Contents/Info.plist"
if [ -e "$T_FAKE/new-req" ]; then cp "$T_FAKE/new-req" "$d/build/bin/Notted.app/Contents/req"; fi
exit 0'
  fake shell 'if [ -e "$T_FAKE/has-claude" ]; then echo /somewhere/claude; fi'
  fake mvfake 'n=$(cat "$T_FAKE/mvcount" 2>/dev/null || echo 0)
n=$((n + 1))
echo "$n" >"$T_FAKE/mvcount"
echo "mv $*" >>"$T_FAKE/calls"
if [ "$n" = "$(cat "$T_FAKE/mvfail-at" 2>/dev/null)" ]; then exit 1; fi
exec /bin/mv "$@"'
  fake rm 'if [ -e "$T_FAKE/rm-old-fails" ]; then
  for a; do
    case "$a" in *.old) echo "rm: $a: Permission denied" >&2; exit 1 ;; esac
  done
fi
exec /bin/rm "$@"'
  fake signing-identity 'echo "signing-identity" >>"$T_FAKE/calls"; cat "$T_FAKE/identity" 2>/dev/null'
  fake create-signing 'echo "create-signing" >>"$T_FAKE/calls"; [ ! -e "$T_FAKE/create-fails" ]'
  fake_permission_tools
  printf '#!/bin/sh\nexec "%s/bin/signing-identity" "$@"\n' "$T" >"$CLONE/scripts/signing-identity.sh"
  printf '#!/bin/sh\nexec "%s/bin/create-signing" "$@"\n' "$T" >"$CLONE/scripts/create-signing-identity.sh"
}

# existing_app <content> [requirement]: an installed Notted with a marker and, optionally, a requirement.
existing_app() {
  mkdir -p "$APPS/Notted.app/Contents/MacOS"
  echo "$1" >"$APPS/Notted.app/Contents/MacOS/Notted"
  echo com.example.test >"$APPS/Notted.app/Contents/Info.plist"
  if [ -n "${2:-}" ]; then echo "$2" >"$APPS/Notted.app/Contents/req"; fi
}

# run_install [VAR=value ...]: runs the script under /bin/bash with only the fakes and system tools on PATH.
run_install() {
  OUT=$(env -i HOME="$T/hdir" PATH="$T/bin:/usr/bin:/bin" SHELL="$T/bin/shell" \
    NOTTED_INSTALL_DIR="$APPS" NOTTED_HOME="$NHOME" NOTTED_SKIP_SIGNING_SETUP=1 NOTTED_NO_OPEN=1 \
    NOTTED_QUIT_WAIT=1 T_FAKE="$T" NOTTED_CODESIGN="$T/bin/codesign" NOTTED_TCCUTIL="$T/bin/tccutil" \
    NOTTED_PLISTBUDDY="$T/bin/plistbuddy" "$@" /bin/bash "$CLONE/scripts/install.sh" 2>&1)
  RC=$?
}

calls() { cat "$T/calls"; }
has() { # text <haystack>: succeeds when the haystack contains the text
  case "$2" in *"$1"*) return 0 ;; esac
  return 1
}
content() { cat "$1" 2>/dev/null; }

# check <name> <command...>: runs a condition and prints one line.
CASE_FAILED=0
check() {
  local msg=$1
  shift
  if ! "$@"; then
    echo "    not met: $msg"
    CASE_FAILED=1
  fi
}
begin() { CASE=$1; CASE_FAILED=0; }
finish() {
  if [ "$CASE_FAILED" -eq 0 ]; then
    echo "test-install: PASS $CASE"
    pass=$((pass + 1))
  else
    echo "test-install: FAIL $CASE"
    printf '%s\n' "$OUT" | sed 's/^/      | /'
    fail=$((fail + 1))
  fi
}

rc_is() { [ "$RC" -eq "$1" ]; }
out_has() { has "$1" "$OUT"; }
out_lacks() { ! has "$1" "$OUT"; }
calls_have() { has "$1" "$(calls)"; }
calls_lack() { ! has "$1" "$(calls)"; }
file_is() { [ "$(content "$1")" = "$2" ]; }
absent() { [ ! -e "$1" ] && [ ! -L "$1" ]; }
present() { [ -e "$1" ]; }

test_script_exists() {
  begin "script exists and is bash-clean"
  OUT=""
  RC=0
  check "scripts/install.sh exists" present "$root/scripts/install.sh"
  if [ -f "$root/scripts/install.sh" ]; then
    OUT=$(/bin/bash -n "$root/scripts/install.sh" 2>&1)
    RC=$?
    check "bash -n passes" rc_is 0
  fi
  finish
}

test_replace_existing() {
  begin "everything present: an existing app is replaced, .old is removed, install-source is written"
  fresh
  existing_app old-build
  run_install
  check "exit 0" rc_is 0
  check "preflight lines" out_has "ok  Go 1.26.5"
  check "macOS line" out_has "ok  macOS 14.5 on Apple silicon"
  check "Xcode line" out_has "ok  Xcode Command Line Tools"
  check "Node line" out_has "ok  Node 24.16.0"
  check "Wails line" out_has "ok  Wails v2.12.0"
  check "claude line" out_has "ok  claude"
  check "installed line" out_has "Installed $APPS/Notted.app."
  check "new build in place" file_is "$APPS/Notted.app/Contents/MacOS/Notted" new-build
  check "no .old" absent "$APPS/Notted.app.old"
  check "no .new" absent "$APPS/Notted.app.new"
  check "install-source" file_is "$NHOME/install-source" "$CLONE"
  check "make build ran once in the clone" calls_have "make -C $CLONE build"
  check "did not open" calls_lack "open "
  check "did not quit" calls_lack "osascript"
  check "no go install" calls_lack "go install"
  finish
}

test_first_install() {
  begin "first install: no existing app"
  fresh
  run_install
  check "exit 0" rc_is 0
  check "installed line" out_has "Installed $APPS/Notted.app."
  check "new build in place" file_is "$APPS/Notted.app/Contents/MacOS/Notted" new-build
  check "no .old" absent "$APPS/Notted.app.old"
  check "no .new" absent "$APPS/Notted.app.new"
  check "install-source" file_is "$NHOME/install-source" "$CLONE"
  finish
}

test_missing_item() {
  begin "one missing item: every result is printed, exit 1, nothing built or touched"
  fresh
  rm "$T/bin/node"
  run_install
  check "exit 1" rc_is 1
  check "Node missing line" out_has "missing  Node 20.19 or later: install it with brew install node (or from nodejs.org), then make install again."
  check "other items still printed" out_has "ok  Go 1.26.5"
  check "claude still checked" out_has "ok  claude"
  check "stop line" out_has "$STOPPED"
  check "make not run" calls_lack "make "
  check "install dir untouched" absent "$APPS"
  check "no install-source" absent "$NHOME/install-source"
  finish
}

test_old_macos() {
  begin "macOS 13 is missing"
  fresh
  echo 13.6 >"$T/macos"
  run_install
  check "exit 1" rc_is 1
  check "macOS fix" out_has "missing  macOS: Notted needs macOS 14 or later on an Apple silicon Mac."
  check "stop line" out_has "$STOPPED"
  check "make not run" calls_lack "make "
  finish
}

test_intel() {
  begin "an Intel Mac is missing"
  fresh
  echo x86_64 >"$T/arch"
  run_install
  check "exit 1" rc_is 1
  check "macOS fix" out_has "missing  macOS: Notted needs macOS 14 or later on an Apple silicon Mac."
  finish
}

test_old_go() {
  begin "Go 1.25 is missing, with the exact fix"
  fresh
  echo go1.25.9 >"$T/goversion"
  run_install
  check "exit 1" rc_is 1
  check "Go fix" out_has "missing  Go 1.26 or later: install it with brew install go (or from go.dev/dl), then make install again."
  check "make not run" calls_lack "make "
  finish
}

test_old_node() {
  begin "Node 20.18 is missing, Node 20.19 is enough"
  fresh
  echo v20.18.3 >"$T/nodeversion"
  run_install
  check "exit 1 on 20.18" rc_is 1
  check "Node missing" out_has "missing  Node 20.19 or later:"
  fresh
  echo v20.19.0 >"$T/nodeversion"
  run_install
  check "exit 0 on 20.19" rc_is 0
  finish
}

test_xcode_missing() {
  begin "missing Xcode Command Line Tools"
  fresh
  fake xcrun 'exit 1'
  run_install
  check "exit 1" rc_is 1
  check "Xcode fix" out_has "missing  Xcode Command Line Tools: run xcode-select --install, then make install again."
  finish
}

test_wails_missing() {
  begin "Wails missing: go install runs with the exact module"
  fresh
  rm "$T/bin/wails"
  run_install
  check "exit 0" rc_is 0
  check "go install call" calls_have "go install $MODULE"
  check "installing line" out_has "installing  wails v2.12.0"
  check "Wails ok after install" out_has "ok  Wails v2.12.0"
  check "build ran" calls_have "make -C $CLONE build"
  finish
  begin "Wails of another version is reinstalled"
  fresh
  echo v2.11.0 >"$T/wailsversion"
  # the fake installer writes a wails that reports the current version file, so the new one reports v2.12.0
  fake go 'echo "go $*" >>"$T_FAKE/calls"
case "$1 $2" in
  "env GOVERSION") cat "$T_FAKE/goversion" ;;
  "env GOPATH") echo "$T_FAKE/gopath" ;;
  "install "*)
    printf "#!/bin/sh\necho v2.12.0\n" >"$T_FAKE/gopath/bin/wails"
    chmod +x "$T_FAKE/gopath/bin/wails" ;;
esac'
  run_install
  check "exit 0" rc_is 0
  check "go install call" calls_have "go install $MODULE"
  finish
}

test_wails_install_fails() {
  begin "Wails missing and go install fails: stop before building"
  fresh
  rm "$T/bin/wails"
  : >"$T/go-install-fails"
  run_install
  check "exit 1" rc_is 1
  check "Wails missing line" out_has "missing  Wails v2.12.0:"
  check "stop line" out_has "$STOPPED"
  check "make not run" calls_lack "make "
  finish
}

test_claude_missing() {
  begin "Claude Code missing is a warning, not a stop"
  fresh
  rm "$T/has-claude"
  run_install
  check "exit 0" rc_is 0
  check "warning line" out_has "warning  claude: Notted needs Claude Code for summaries. Install it from claude.com/claude-code and log in."
  check "installed" out_has "Installed $APPS/Notted.app."
  finish
}

test_still_running() {
  begin "Notted still running after the wait: exit 1, app unchanged"
  fresh
  existing_app old-build
  : >"$T/running"
  : >"$T/stuck"
  run_install
  check "exit 1" rc_is 1
  check "quitting line" out_has "Quitting Notted to replace it."
  check "still-running line" out_has "Notted is still running. Quit it, then run make install again."
  check "asked it to quit" calls_have "osascript -e quit app \"Notted\""
  check "old app unchanged" file_is "$APPS/Notted.app/Contents/MacOS/Notted" old-build
  check "no .new" absent "$APPS/Notted.app.new"
  check "no .old" absent "$APPS/Notted.app.old"
  check "no install-source" absent "$NHOME/install-source"
  finish
}

test_quits_running() {
  begin "Notted running: quit, wait, then replace"
  fresh
  existing_app old-build
  : >"$T/running"
  run_install
  check "exit 0" rc_is 0
  check "quitting line" out_has "Quitting Notted to replace it."
  check "asked it to quit" calls_have "osascript -e quit app \"Notted\""
  check "new build in place" file_is "$APPS/Notted.app/Contents/MacOS/Notted" new-build
  finish
}

test_failed_second_rename() {
  begin "second rename fails: the old app is put back"
  fresh
  existing_app old-build
  echo 2 >"$T/mvfail-at"
  run_install NOTTED_MV="$T/bin/mvfake"
  check "exit 1" rc_is 1
  check "unchanged line" out_has "$UNCHANGED"
  check "old app back" file_is "$APPS/Notted.app/Contents/MacOS/Notted" old-build
  check "no .new" absent "$APPS/Notted.app.new"
  check "no .old" absent "$APPS/Notted.app.old"
  check "three renames tried" calls_have "mv $APPS/Notted.app.old $APPS/Notted.app"
  check "no install-source" absent "$NHOME/install-source"
  finish
}

test_failed_first_rename() {
  begin "first rename fails: nothing changes"
  fresh
  existing_app old-build
  echo 1 >"$T/mvfail-at"
  run_install NOTTED_MV="$T/bin/mvfake"
  check "exit 1" rc_is 1
  check "unchanged line" out_has "$UNCHANGED"
  check "old app in place" file_is "$APPS/Notted.app/Contents/MacOS/Notted" old-build
  check "no .new" absent "$APPS/Notted.app.new"
  finish
}

test_staged_leftover() {
  begin "a folder already at Notted.app.new is removed before copying"
  fresh
  existing_app old-build
  mkdir -p "$APPS/Notted.app.new/junk"
  echo junk >"$APPS/Notted.app.new/junk/file"
  run_install
  check "exit 0" rc_is 0
  check "new build in place" file_is "$APPS/Notted.app/Contents/MacOS/Notted" new-build
  check "junk gone" absent "$APPS/Notted.app/junk"
  check "no .new" absent "$APPS/Notted.app.new"
  finish
}

OLD_LEFT=' is left from an earlier install. Move it to the Trash, then run make install again.'

test_old_without_app() {
  begin "a leftover Notted.app.old and no app: stop with its path, change nothing"
  fresh
  mkdir -p "$APPS/Notted.app.old/Contents/MacOS"
  echo earlier >"$APPS/Notted.app.old/Contents/MacOS/Notted"
  run_install
  check "exit 1" rc_is 1
  check "leftover line" out_has "$APPS/Notted.app.old$OLD_LEFT"
  check "not the generic failure" out_lacks "$UNCHANGED"
  check "no app installed" absent "$APPS/Notted.app"
  check "no .new" absent "$APPS/Notted.app.new"
  check "leftover untouched" file_is "$APPS/Notted.app.old/Contents/MacOS/Notted" earlier
  check "make not run" calls_lack "make "
  check "no install-source" absent "$NHOME/install-source"
  finish
}

test_old_with_app() {
  begin "a leftover Notted.app.old and an app: stop with its path, change nothing"
  fresh
  existing_app current
  mkdir -p "$APPS/Notted.app.old/Contents/MacOS"
  echo earlier >"$APPS/Notted.app.old/Contents/MacOS/Notted"
  run_install
  check "exit 1" rc_is 1
  check "leftover line" out_has "$APPS/Notted.app.old$OLD_LEFT"
  check "not the generic failure" out_lacks "$UNCHANGED"
  check "current app in place" file_is "$APPS/Notted.app/Contents/MacOS/Notted" current
  check "leftover untouched" file_is "$APPS/Notted.app.old/Contents/MacOS/Notted" earlier
  check "no .new" absent "$APPS/Notted.app.new"
  check "make not run" calls_lack "make "
  finish
}

test_old_removal_fails() {
  begin "removing .old fails after a good swap: the install still finishes"
  fresh
  existing_app old-build
  : >"$T/rm-old-fails"
  run_install NOTTED_NO_OPEN=0
  check "exit 0" rc_is 0
  check "new build in place" file_is "$APPS/Notted.app/Contents/MacOS/Notted" new-build
  check "installed line" out_has "Installed $APPS/Notted.app."
  check "install-source" file_is "$NHOME/install-source" "$CLONE"
  check "opened" calls_have "open $APPS/Notted.app"
  finish
}

test_gobin() {
  begin "GOBIN set: it goes first on PATH, not GOPATH/bin"
  fresh
  rm "$T/bin/wails"
  mkdir -p "$T/my gobin"
  echo "$T/my gobin" >"$T/gobin"
  run_install
  check "exit 0" rc_is 0
  check "installed into GOBIN" present "$T/my gobin/wails"
  check "not into GOPATH/bin" absent "$T/gopath/bin/wails"
  check "Wails ok after install" out_has "ok  Wails v2.12.0"
  finish
}

test_open() {
  begin "the app is opened unless NOTTED_NO_OPEN=1"
  fresh
  run_install NOTTED_NO_OPEN=0
  check "exit 0" rc_is 0
  check "open called with the app path" calls_have "open $APPS/Notted.app"
  finish
}

test_signing_missing() {
  begin "no signing identity: the certificate is created"
  fresh
  run_install NOTTED_SKIP_SIGNING_SETUP=0
  check "exit 0" rc_is 0
  check "creating message" out_has 'Creating the "Notted Local Signing" certificate. macOS asks for your login password once, so it can trust the certificate for signing Notted.'
  check "create script ran" calls_have "create-signing"
  check "no ad hoc message" out_lacks "Continuing without the certificate"
  finish
  begin "an identity that is '-' counts as none"
  fresh
  echo - >"$T/identity"
  run_install NOTTED_SKIP_SIGNING_SETUP=0
  check "create script ran" calls_have "create-signing"
  finish
}

test_signing_present() {
  begin "signing identity present: no certificate is created"
  fresh
  echo 13696039679D3B35D7E756B113E7576F4DE421A7 >"$T/identity"
  run_install NOTTED_SKIP_SIGNING_SETUP=0
  check "exit 0" rc_is 0
  check "create script not run" calls_lack "create-signing"
  check "no creating message" out_lacks "Creating the"
  finish
}

test_signing_fails() {
  begin "certificate creation fails: continue ad hoc"
  fresh
  : >"$T/create-fails"
  run_install NOTTED_SKIP_SIGNING_SETUP=0
  check "exit 0" rc_is 0
  check "ad hoc message" out_has 'Continuing without the certificate: this build is signed ad hoc, and macOS asks for the microphone and screen recording permissions again after each update.'
  check "installed" out_has "Installed $APPS/Notted.app."
  finish
}

test_signing_skipped() {
  begin "NOTTED_SKIP_SIGNING_SETUP=1 skips signing"
  fresh
  run_install
  check "signing script not consulted" calls_lack "signing-identity"
  finish
}

SIG_CHANGED='the signature changed, so macOS will ask for the microphone and screen recording permissions again'
RESET_SCREEN="tccutil reset ScreenCapture com.example.test"
RESET_MIC="tccutil reset Microphone com.example.test"

# call_before <first> <second>: both calls are in the log, the first one earlier.
call_before() {
  local log rest
  log=$(calls)
  has "$1" "$log" || return 1
  rest=${log#*"$1"}
  has "$2" "$rest"
}

# install_with <installed requirement|-> <new requirement>: make install over an app signed so.
install_with() {
  fresh
  if [ "$1" = - ]; then existing_app old-build; else existing_app old-build "$1"; fi
  echo "$2" >"$T/new-req"
  run_install NOTTED_NO_OPEN=0
}

test_reset_equal() {
  begin "make install, same requirement: no reset and no extra line"
  install_with 'identifier "a" and anchor apple' 'identifier "a" and anchor apple'
  check "exit 0" rc_is 0
  check "no tccutil" calls_lack "tccutil"
  check "no signature line" out_lacks "$SIG_CHANGED"
  finish
}

test_reset_different() {
  begin "make install, another requirement: both permissions reset, one line, before open"
  install_with 'cdhash H"1"' 'identifier "a" and certificate leaf = H"2"'
  check "exit 0" rc_is 0
  check "screen reset" calls_have "$RESET_SCREEN"
  check "microphone reset" calls_have "$RESET_MIC"
  check "bundle id read from the new Info.plist" calls_have "plistbuddy -c Print :CFBundleIdentifier $APPS/Notted.app/Contents/Info.plist"
  check "the line, without a prefix" out_has "$SIG_CHANGED"
  check "line printed once" test "$(printf '%s\n' "$OUT" | grep -c "$SIG_CHANGED")" = 1
  check "screen reset before open" call_before "$RESET_SCREEN" "open $APPS/Notted.app"
  check "microphone reset before open" call_before "$RESET_MIC" "open $APPS/Notted.app"
  check "new build in place" file_is "$APPS/Notted.app/Contents/MacOS/Notted" new-build
  finish
}

test_reset_unreadable() {
  begin "make install, unreadable installed requirement: reset"
  install_with - 'identifier "a"'
  check "exit 0" rc_is 0
  check "screen reset" calls_have "$RESET_SCREEN"
  check "microphone reset" calls_have "$RESET_MIC"
  check "the line" out_has "$SIG_CHANGED"
  finish
}

test_reset_first_install() {
  begin "make install, first install: nothing reset"
  fresh
  echo 'identifier "a"' >"$T/new-req"
  run_install NOTTED_NO_OPEN=0
  check "exit 0" rc_is 0
  check "no tccutil" calls_lack "tccutil"
  check "no signature line" out_lacks "$SIG_CHANGED"
  finish
}

test_reset_tccutil_fails() {
  begin "make install, tccutil fails: logged, install succeeds"
  fresh
  existing_app old-build 'cdhash H"1"'
  echo 'cdhash H"2"' >"$T/new-req"
  : >"$T/tccutil-fails"
  run_install NOTTED_NO_OPEN=0
  check "exit 0" rc_is 0
  check "screen failure logged" out_has "could not reset the ScreenCapture permission: boom"
  check "microphone failure logged" out_has "could not reset the Microphone permission: boom"
  check "both tried" calls_have "$RESET_MIC"
  check "installed line" out_has "Installed $APPS/Notted.app."
  check "opened" calls_have "open $APPS/Notted.app"
  finish
}

test_reset_rollback() {
  begin "make install, failed swap: nothing reset"
  fresh
  existing_app old-build 'cdhash H"1"'
  echo 'cdhash H"2"' >"$T/new-req"
  echo 2 >"$T/mvfail-at"
  run_install NOTTED_MV="$T/bin/mvfake"
  check "exit 1" rc_is 1
  check "unchanged line" out_has "$UNCHANGED"
  check "no tccutil" calls_lack "tccutil"
  check "no signature line" out_lacks "$SIG_CHANGED"
  finish
}

# ---- the Update Notted installer (internal/integration/install/install.sh) ----

SWAPPER="$root/internal/integration/install/install.sh"

# upd_fresh: a temp world with fake open and permission tools; fake_exe is a signed copy of sleep.
upd_fresh() {
  T=$(mktemp -d)
  T=$(cd "$T" && pwd -P)
  tmps+=("$T")
  UAPPS="$T/apps dir"
  NEWAPP="$T/src/build/bin/Notted.app"
  mkdir -p "$T/bin" "$UAPPS"
  export T_FAKE=$T
  : >"$T/calls"
  cp /bin/sleep "$T/exe"
  /usr/bin/codesign -f -s - "$T/exe" >/dev/null 2>&1
  fake_permission_tools
  # open launches the app's executable (a sleep), so the installer sees it start
  fake open 'echo "open $*" >>"$T_FAKE/calls"
if [ ! -e "$T_FAKE/no-launch" ]; then
  "$1/Contents/MacOS/Notted" 30 >/dev/null 2>&1 &
  echo $! >>"$T_FAKE/pids"
fi'
  fake mvfake 'n=$(cat "$T_FAKE/mvcount" 2>/dev/null || echo 0)
n=$((n + 1))
echo "$n" >"$T_FAKE/mvcount"
if [ "$n" = "$(cat "$T_FAKE/mvfail-at" 2>/dev/null)" ]; then exit 1; fi
exec /bin/mv "$@"'
}

# upd_app <dir> <marker> [requirement]
upd_app() {
  mkdir -p "$1/Contents/MacOS"
  cp "$T/exe" "$1/Contents/MacOS/Notted"
  echo "$2" >"$1/Contents/marker"
  echo com.example.test >"$1/Contents/Info.plist"
  if [ -n "${3:-}" ]; then echo "$3" >"$1/Contents/req"; fi
}

# run_update [VAR=value ...]: runs the installer, with an already-gone pid to wait for.
run_update() {
  local dir="$T/notted-install-x" gone
  mkdir -p "$dir"
  cp "$SWAPPER" "$dir/install.sh"
  /bin/sleep 0 &
  gone=$!
  wait "$gone"
  OUT=$(env -i PATH=/usr/bin:/bin:/usr/sbin:/sbin T_FAKE="$T" NOTTED_OPEN="$T/bin/open" NOTTED_LAUNCH_SETTLE=0 \
    NOTTED_QUIT_WAIT=1 NOTTED_LAUNCH_WAIT=2 NOTTED_CODESIGN="$T/bin/codesign" NOTTED_TCCUTIL="$T/bin/tccutil" \
    NOTTED_PLISTBUDDY="$T/bin/plistbuddy" "$@" /bin/bash "$dir/install.sh" "$gone" "$NEWAPP" "$UAPPS/Notted.app" 2>&1)
  RC=$?
  if [ -s "$T/pids" ]; then kill -9 $(cat "$T/pids") 2>/dev/null; fi
}

# update_with <installed requirement|-> <new requirement>
update_with() {
  upd_fresh
  if [ "$1" = - ]; then upd_app "$UAPPS/Notted.app" old; else upd_app "$UAPPS/Notted.app" old "$1"; fi
  upd_app "$NEWAPP" new "$2"
  run_update
}

test_update_reset_equal() {
  begin "update installer, same requirement: no reset and no extra line"
  update_with 'identifier "a" and anchor apple' 'identifier "a" and anchor apple'
  check "exit 0" rc_is 0
  check "swapped" file_is "$UAPPS/Notted.app/Contents/marker" new
  check "no tccutil" calls_lack "tccutil"
  check "no signature line" out_lacks "$SIG_CHANGED"
  finish
}

test_update_reset_different() {
  begin "update installer, another requirement: both permissions reset, one line, before open"
  update_with 'cdhash H"1"' 'identifier "a" and certificate leaf = H"2"'
  check "exit 0" rc_is 0
  check "swapped" file_is "$UAPPS/Notted.app/Contents/marker" new
  check "screen reset" calls_have "$RESET_SCREEN"
  check "microphone reset" calls_have "$RESET_MIC"
  check "the line, prefixed" out_has "install: $SIG_CHANGED"
  check "line printed once" test "$(printf '%s\n' "$OUT" | grep -c "$SIG_CHANGED")" = 1
  check "screen reset before open" call_before "$RESET_SCREEN" "open $UAPPS/Notted.app"
  check "microphone reset before open" call_before "$RESET_MIC" "open $UAPPS/Notted.app"
  finish
}

test_update_reset_unreadable() {
  begin "update installer, unreadable installed requirement: reset"
  update_with - 'identifier "a"'
  check "exit 0" rc_is 0
  check "screen reset" calls_have "$RESET_SCREEN"
  check "microphone reset" calls_have "$RESET_MIC"
  check "the line" out_has "install: $SIG_CHANGED"
  finish
}

test_update_reset_no_installed_app() {
  begin "update installer, no installed app: nothing reset"
  upd_fresh
  upd_app "$NEWAPP" new 'identifier "a"'
  run_update
  check "exit 1 (nothing to swap)" rc_is 1
  check "no tccutil" calls_lack "tccutil"
  check "no signature line" out_lacks "$SIG_CHANGED"
  finish
}

test_update_reset_tccutil_fails() {
  begin "update installer, tccutil fails: logged, update succeeds"
  upd_fresh
  upd_app "$UAPPS/Notted.app" old 'cdhash H"1"'
  upd_app "$NEWAPP" new 'cdhash H"2"'
  : >"$T/tccutil-fails"
  run_update
  check "exit 0" rc_is 0
  check "swapped" file_is "$UAPPS/Notted.app/Contents/marker" new
  check "screen failure logged" out_has "install: could not reset the ScreenCapture permission: boom"
  check "microphone failure logged" out_has "install: could not reset the Microphone permission: boom"
  check "opened" calls_have "open $UAPPS/Notted.app"
  finish
}

test_update_reset_rollback() {
  begin "update installer, failed swap: restored and nothing reset"
  upd_fresh
  upd_app "$UAPPS/Notted.app" old 'cdhash H"1"'
  upd_app "$NEWAPP" new 'cdhash H"2"'
  echo 2 >"$T/mvfail-at"
  run_update NOTTED_MV="$T/bin/mvfake"
  check "exit 1" rc_is 1
  check "old app back" file_is "$UAPPS/Notted.app/Contents/marker" old
  check "no tccutil" calls_lack "tccutil"
  check "no signature line" out_lacks "$SIG_CHANGED"
  finish
}

test_script_exists
test_replace_existing
test_first_install
test_missing_item
test_old_macos
test_intel
test_old_go
test_old_node
test_xcode_missing
test_wails_missing
test_wails_install_fails
test_claude_missing
test_still_running
test_quits_running
test_failed_second_rename
test_failed_first_rename
test_old_without_app
test_old_with_app
test_old_removal_fails
test_gobin
test_staged_leftover
test_open
test_signing_missing
test_signing_present
test_signing_fails
test_signing_skipped
test_reset_equal
test_reset_different
test_reset_unreadable
test_reset_first_install
test_reset_tccutil_fails
test_reset_rollback
test_update_reset_equal
test_update_reset_different
test_update_reset_unreadable
test_update_reset_no_installed_app
test_update_reset_tccutil_fails
test_update_reset_rollback

echo "test-install: $pass passed, $fail failed"
if [ "$fail" -ne 0 ]; then exit 1; fi
