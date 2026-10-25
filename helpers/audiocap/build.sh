#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p build
swiftc -O -swift-version 5 -target arm64-apple-macos14.0 \
  -framework AVFoundation -framework ScreenCaptureKit -framework CoreMedia -framework CoreGraphics \
  -Xlinker -sectcreate -Xlinker __TEXT -Xlinker __info_plist -Xlinker Info.plist \
  Sources/*.swift -o build/audiocap
codesign -f -s - --identifier tech.bhlabs.notted.audiocap build/audiocap
echo "built $(pwd)/build/audiocap"
