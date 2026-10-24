#!/usr/bin/env bash
# make install: checks the Mac, signs, builds and installs Notted, then opens it.
# Environment (all optional): NOTTED_INSTALL_DIR (default /Applications), NOTTED_HOME (default
# ~/.notted), NOTTED_SKIP_SIGNING_SETUP=1, NOTTED_NO_OPEN=1, NOTTED_QUIT_WAIT (seconds, default 15),
# NOTTED_MV (the rename command, default mv).
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd -P)
install_dir=${NOTTED_INSTALL_DIR:-/Applications}
notted_home=${NOTTED_HOME:-$HOME/.notted}
quit_wait=${NOTTED_QUIT_WAIT:-15}
mv_cmd=${NOTTED_MV:-mv}
app="$install_dir/Notted.app"
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
    need macOS "Notted needs macOS 14 or later on an Apple silicon Mac."
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
    echo "warning  claude: Notted needs Claude Code for summaries. Install it from claude.com/claude-code and log in."
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
if [ "${NOTTED_SKIP_SIGNING_SETUP:-}" != 1 ]; then
  identity=$(bash "$root/scripts/signing-identity.sh" 2>/dev/null) || identity=
  if [ -z "$identity" ] || [ "$identity" = "-" ]; then
    echo 'Creating the "Notted Local Signing" certificate. macOS asks for your login password once, so it can trust the certificate for signing Notted.'
    if ! bash "$root/scripts/create-signing-identity.sh"; then
      echo 'Continuing without the certificate: this build is signed ad hoc, and macOS asks for the microphone and screen recording permissions again after each update.'
    fi
  fi
fi

make -C "$root" build

built="$root/build/bin/Notted.app"
if [ ! -d "$built" ]; then
  echo "make build finished, but $built is missing. Nothing was installed."
  exit 1
fi

running() { pgrep -x Notted >/dev/null 2>&1; }

if running; then
  echo "Quitting Notted to replace it."
  osascript -e 'quit app "Notted"' >/dev/null 2>&1 || true
  n=$((quit_wait * 5))
  while [ "$n" -gt 0 ] && running; do
    sleep 0.2
    n=$((n - 1))
  done
  if running; then
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
  echo "Install failed; your previous Notted is unchanged."
  exit 1
}

mkdir -p "$install_dir" || install_failed
rm -rf "$staged"
ditto "$built" "$staged" || install_failed
if [ -e "$app" ] || [ -L "$app" ]; then
  rename "$app" "$old" || install_failed
  if ! rename "$staged" "$app"; then
    if ! rename "$old" "$app"; then
      rm -rf "$staged"
      echo "Install failed, and the previous Notted could not be put back. It is at $old. Move it back to $app in Finder."
      exit 1
    fi
    install_failed
  fi
  rm -rf "$old" || true
else
  rename "$staged" "$app" || install_failed
fi

# Save the clone, so Update Notted knows which folder to pull.
source_dir=$(git -C "$root" rev-parse --show-toplevel 2>/dev/null) || source_dir=$root
if mkdir -p "$notted_home" 2>/dev/null; then
  printf '%s\n' "$source_dir" >"$notted_home/install-source" 2>/dev/null || true
fi

echo "Installed $app."
if [ "${NOTTED_NO_OPEN:-}" != 1 ]; then
  open "$app"
fi
