#!/usr/bin/env bash
# Retakes the README screenshots in docs/screenshots/ from the frontend preview build.
#
# Usage: scripts/screenshots.sh <base-url>
#
# <base-url> is the address that serves frontend/dist-preview, with a trailing slash or none.
# To refresh the screenshots:
#   1. Build the preview:  frontend/node_modules/.bin/vite build frontend --mode preview --outDir dist-preview --emptyOutDir
#   2. Serve frontend/dist-preview on a free local port, for example:
#        python3 -m http.server <port> --directory frontend/dist-preview --bind 127.0.0.1
#   3. Run this script with the address of that server.
# Environment: CHROME (the browser program, default Google Chrome in /Applications).
set -euo pipefail

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: scripts/screenshots.sh <base-url>" >&2
  exit 2
fi
base=${1%/}
root=$(cd "$(dirname "$0")/.." && pwd -P)
out="$root/docs/screenshots"
chrome=${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}
if [ ! -x "$chrome" ]; then
  echo "screenshots: Chrome not found at $chrome. Set CHROME to a Chrome or Chromium program." >&2
  exit 1
fi
mkdir -p "$out"

# shoot <file> <screen> <theme>
shoot() {
  echo "shooting $1 ($2, $3)"
  "$chrome" --headless=new --hide-scrollbars --force-device-scale-factor=2 --window-size=1200,800 \
    --virtual-time-budget=6000 --screenshot="$out/$1" "$base/?screen=$2&theme=$3" >/dev/null 2>&1
  [ -s "$out/$1" ] || { echo "screenshots: $1 was not written" >&2; exit 1; }
}

shoot recording.png recording light
shoot summary.png summarized light
shoot save-dialog.png save-dialog light
shoot first-run.png first-run light
shoot setup-prompt.png setup-prompt-shown light
shoot permissions.png permissions-mixed light
shoot mic-test.png mictest-ok light
shoot recording-dark.png recording dark
shoot summary-dark.png summarized dark
echo "screenshots: done. Check each image before you commit it."
