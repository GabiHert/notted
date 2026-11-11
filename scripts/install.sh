#!/usr/bin/env bash
# make install: checks the Mac, signs, builds and installs Phonon, then opens it.
# Environment (all optional): PHONON_INSTALL_DIR (default /Applications), PHONON_HOME (default
# ~/.phonon), PHONON_SKIP_SIGNING_SETUP=1, PHONON_NO_OPEN=1, PHONON_QUIT_WAIT (seconds, default 15),
# PHONON_MV (the rename command, default mv), PHONON_CODESIGN, PHONON_TCCUTIL, PHONON_PLISTBUDDY
# (the macOS tools that read the signature, reset permissions and read the bundle id).
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd -P)
install_dir=${PHONON_INSTALL_DIR:-/Applications}
quit_wait=${PHONON_QUIT_WAIT:-15}
mv_cmd=${PHONON_MV:-mv}
codesign_cmd=${PHONON_CODESIGN:-/usr/bin/codesign}
tccutil_cmd=${PHONON_TCCUTIL:-/usr/bin/tccutil}
plistbuddy_cmd=${PHONON_PLISTBUDDY:-/usr/libexec/PlistBuddy}
app="$install_dir/Phonon.app"
staged="$app.new"
old="$app.old"

wails_module=github.com/wailsapp/wails/v2/cmd/wails@v2.12.0

missing=0
ok() { echo "ok  $1"; }
need() { # item fix
  echo "missing  $1: $2"
  missing=1
}

# num <text>: the leading digits of the text, or 0.
num() {
  local n=${1%%[!0-9]*}
  echo "${n:-0}"
}
# at_least <version> <major> <minor>: succeeds when "M.m..." is at least major.minor.
at_least() {
  local v=$1 rest maj min
  maj=$(num "$v")
  rest=${v#*.}
  [ "$rest" = "$v" ] && rest=0
  min=$(num "$rest")
  [ "$maj" -gt "$2" ] || { [ "$maj" -eq "$2" ] && [ "$min" -ge "$3" ]; }
}

check_macos() {
  local v arch
  v=$(sw_vers -productVersion 2>/dev/null) || v=0
  arch=$(uname -m 2>/dev/null) || arch=unknown
  if at_least "$v" 14 0 && [ "$arch" = arm64 ]; then
    ok "macOS $v on Apple silicon"
  else
    need macOS "Phonon needs macOS 14 or later on an Apple silicon Mac."
  fi
}

check_xcode() {
  if xcrun --find swiftc >/dev/null 2>&1; then
    ok "Xcode Command Line Tools"
  else
    need "Xcode Command Line Tools" "run xcode-select --install, then make install again."
  fi
}

have_go=0
check_go() {
  local v
  v=$(go env GOVERSION 2>/dev/null) || v=
  v=${v#go}
  if [ -n "$v" ] && at_least "$v" 1 26; then
    have_go=1
    ok "Go $v"
  else
    need "Go 1.26 or later" "install it with brew install go (or from go.dev/dl), then make install again."
  fi
}

check_node() {
  local v
  v=$(node --version 2>/dev/null) || v=
  v=${v#v}
  if [ -n "$v" ] && at_least "$v" 20 19; then
    ok "Node $v"
  else
    need "Node 20.19 or later" "install it with brew install node (or from nodejs.org), then make install again."
  fi
}

wails_ok() {
  local out
  out=$(wails version 2>/dev/null) || return 1
  out=${out//$'\n'/ }
  case " $out " in *" v2.12.0 "*) return 0 ;; esac
  return 1
}

check_wails() {
  if wails_ok; then
    ok "Wails v2.12.0"
    return
  fi
  if [ "$have_go" -ne 1 ]; then
    need "Wails v2.12.0" "run go install $wails_module after installing Go, then make install again."
    return
  fi
  echo "installing  wails v2.12.0"
  local gobin
  gobin=$(go env GOBIN 2>/dev/null) || gobin=
  [ -n "$gobin" ] || gobin=$(go env GOPATH)/bin
  if go install "$wails_module" >/dev/null 2>&1; then
    PATH="$gobin:$PATH"
    export PATH
  fi
  if wails_ok; then
    ok "Wails v2.12.0"
  else
    need "Wails v2.12.0" "run go install $wails_module, then make install again."
  fi
}

check_claude() {
  local found
  found=$("${SHELL:-/bin/zsh}" -l -i -c 'command -v claude' 2>/dev/null) || found=
  if [ -n "$found" ]; then
    ok "claude"
  else
    echo "warning  claude: Phonon needs Claude Code for summaries. Install it from claude.com/claude-code and log in."
  fi
}

check_macos
check_xcode
check_go
check_node
check_wails
check_claude
if [ "$missing" -ne 0 ]; then
  echo "make install stopped: fix the missing items above, then run make install again."
  exit 1
fi

# A copy left from an earlier install is never overwritten, so stop before anything changes.
if [ -e "$old" ] || [ -L "$old" ]; then
  echo "$old is left from an earlier install. Move it to the Trash, then run make install again."
  exit 1
fi

# Signing: create the local certificate once, and carry on ad hoc if that fails.
if [ "${PHONON_SKIP_SIGNING_SETUP:-}" != 1 ]; then
  identity=$(bash "$root/scripts/signing-identity.sh" 2>/dev/null) || identity=
  if [ -z "$identity" ] || [ "$identity" = "-" ]; then
    echo 'Creating the "Phonon Local Signing" certificate. macOS asks for your login password once, so it can trust the certificate for signing Phonon.'
    if ! bash "$root/scripts/create-signing-identity.sh"; then
      echo 'Continuing without the certificate: this build is signed ad hoc, and macOS asks for the microphone and screen recording permissions again after each update.'
    fi
  fi
fi

make -C "$root" build

built="$root/build/bin/Phonon.app"
if [ ! -d "$built" ]; then
  echo "make build finished, but $built is missing. Nothing was installed."
  exit 1
fi

running() { pgrep -x Phonon >/dev/null 2>&1; }

if running; then
  echo "Quitting Phonon to replace it."
  osascript -e 'quit app "Phonon"' >/dev/null 2>&1 || true
  n=$((quit_wait * 5))
  while [ "$n" -gt 0 ] && running; do
    sleep 0.2
    n=$((n - 1))
  done
  if running; then
    echo "Phonon is still running. Quit it, then run make install again."
    exit 1
  fi
fi

# Phonon was called Notted. Phonon copies Notted's data when it first opens, which it refuses to do
# while Notted runs (the copy would not be consistent), so quit Notted first.
notted_running() { pgrep -x Notted >/dev/null 2>&1; }
if notted_running; then
  echo "Quitting Notted, Phonon's old name, so Phonon can copy its data."
  osascript -e 'quit app "Notted"' >/dev/null 2>&1 || true
  n=$((quit_wait * 5))
  while [ "$n" -gt 0 ] && notted_running; do
    sleep 0.2
    n=$((n - 1))
  done
  if notted_running; then
    echo "Notted is still running. Quit it, then run make install again."
    exit 1
  fi
fi

# rename <from> <to>: never lands on an existing path, because mv would move into a folder.
rename() {
  [ ! -e "$2" ] && [ ! -L "$2" ] || return 1
  "$mv_cmd" "$1" "$2"
}
install_failed() {
  rm -rf "$staged"
  echo "Install failed; your previous Phonon is unchanged."
  exit 1
}

# requirement <app>: the designated requirement macOS stores a permission against, or nothing.
requirement() {
  "$codesign_cmd" -d -r- "$1" 2>/dev/null | sed -n 's/^designated => //p'
}
# reset_permissions <app>: macOS keeps a permission with the signature that got it, so after a
# signature change the stored one never matches and switching it on does nothing. A failure only
# gets logged.
reset_permissions() {
  local id service out
  echo "the signature changed, so macOS will ask for the microphone and screen recording permissions again"
  if ! id=$("$plistbuddy_cmd" -c 'Print :CFBundleIdentifier' "$1/Contents/Info.plist" 2>&1) || [ -z "$id" ]; then
    echo "could not read the bundle id: $id"
    return 0
  fi
  for service in ScreenCapture Microphone; do
    out=$("$tccutil_cmd" reset "$service" "$id" 2>&1) || echo "could not reset the $service permission: $out"
  done
  return 0
}

mkdir -p "$install_dir" || install_failed
rm -rf "$staged"
ditto "$built" "$staged" || install_failed
reset_needed=0
if [ -e "$app" ] || [ -L "$app" ]; then
  # Read the old signature before the swap, while the old app is still at $app.
  old_req=$(requirement "$app") || old_req=
  new_req=$(requirement "$built") || new_req=
  if [ -z "$old_req" ] || [ "$old_req" != "$new_req" ]; then reset_needed=1; fi
  rename "$app" "$old" || install_failed
  if ! rename "$staged" "$app"; then
    if ! rename "$old" "$app"; then
      rm -rf "$staged"
      echo "Install failed, and the previous Phonon could not be put back. It is at $old. Move it back to $app in Finder."
      exit 1
    fi
    install_failed
  fi
  rm -rf "$old" || true
else
  rename "$staged" "$app" || install_failed
fi
if [ "$reset_needed" -eq 1 ]; then reset_permissions "$app"; fi

echo "Installed $app."

# Notted.app, Phonon's old name, stays where it is: Phonon copies Notted's data when it first
# opens and leaves ~/.notted as it was, so Notted still works.
if [ "${PHONON_NO_OPEN:-}" != 1 ]; then
  open "$app"
fi
