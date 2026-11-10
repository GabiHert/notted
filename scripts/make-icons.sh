#!/usr/bin/env bash
# make-icons.sh: renders build/appicon.svg into the app icon files. Run it after changing the SVG.
#   build/appicon.png          1024 px master; wails build reads it.
#   build/darwin/iconfile.icns every macOS size (16 to 512 @1x and @2x) from iconutil; make build
#                              copies it over the one wails generates, for sharper small sizes.
# Needs Google Chrome (headless, to render the SVG), sips and iconutil.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd -P)
chrome=${PHONON_CHROME:-"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cp "$root/build/appicon.svg" "$tmp/appicon.svg"
printf '<!doctype html><html><head><style>html,body{margin:0;background:transparent}img{display:block;width:1024px;height:1024px}</style></head><body><img src="appicon.svg"></body></html>' >"$tmp/r.html"
"$chrome" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
  --default-background-color=00000000 --window-size=1024,1024 --screenshot="$tmp/appicon.png" "file://$tmp/r.html" >/dev/null 2>&1
mkdir "$tmp/Phonon.iconset"
for s in 16 32 128 256 512; do
  sips -z "$s" "$s" "$tmp/appicon.png" --out "$tmp/Phonon.iconset/icon_${s}x${s}.png" >/dev/null
  sips -z $((s * 2)) $((s * 2)) "$tmp/appicon.png" --out "$tmp/Phonon.iconset/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$tmp/Phonon.iconset" -o "$tmp/iconfile.icns"
cp "$tmp/appicon.png" "$root/build/appicon.png"
cp "$tmp/iconfile.icns" "$root/build/darwin/iconfile.icns"
echo "wrote build/appicon.png and build/darwin/iconfile.icns"
