#!/usr/bin/env bash
# Builds build/bin/Notted.app, the bridge that moves a Notted install onto Phonon (cmd/nottedbridge).
# Notted's Update button runs make build in its source clone and installs build/bin/Notted.app; this
# bundle carries the freshly built Phonon.app and, once launched, puts it beside itself.
# Usage: make-notted-bridge.sh [Phonon.app] [Notted.app]
set -euo pipefail
cd "$(dirname "$0")/.."
PHONON="${1:-build/bin/Phonon.app}"
BRIDGE="${2:-build/bin/Notted.app}"
C="$BRIDGE/Contents"

[ -x "$PHONON/Contents/MacOS/Phonon" ] || { echo "make-notted-bridge: $PHONON is not built"; exit 1; }
rm -rf "$BRIDGE"
mkdir -p "$C/MacOS" "$C/Resources" "$C/Helpers"
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -o "$C/MacOS/Notted" ./cmd/nottedbridge
cp build/darwin/iconfile.icns "$C/Resources/iconfile.icns"
# cp -c clones on APFS, so the second copy of Phonon takes no extra disk.
cp -Rc "$PHONON" "$C/Helpers/Phonon.app" 2>/dev/null || /usr/bin/ditto "$PHONON" "$C/Helpers/Phonon.app"
cat >"$C/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleName</key><string>Notted</string>
  <key>CFBundleDisplayName</key><string>Notted → Phonon</string>
  <key>CFBundleExecutable</key><string>Notted</string>
  <key>CFBundleIdentifier</key><string>tech.bhlabs.phonon.nottedbridge</string>
  <key>CFBundleVersion</key><string>1</string>
  <key>CFBundleShortVersionString</key><string>1</string>
  <key>CFBundleIconFile</key><string>iconfile</string>
  <key>LSMinimumSystemVersion</key><string>14.0</string>
  <key>LSUIElement</key><true/>
</dict>
</plist>
PLIST
id="$(bash scripts/signing-identity.sh)"
codesign -f -s "$id" "$C/MacOS/Notted"
codesign -f -s "$id" "$BRIDGE"
codesign --verify --deep --strict "$BRIDGE"
codesign --verify --deep --strict "$C/Helpers/Phonon.app"
echo "make-notted-bridge: built $BRIDGE"
