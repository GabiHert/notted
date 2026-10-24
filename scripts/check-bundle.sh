#!/usr/bin/env bash
# Bundle check: the built Notted.app carries its helper and speech libraries and is signed.
set -uo pipefail
cd "$(dirname "$0")/.."

APP="${1:-build/bin/Notted.app}"
C="$APP/Contents"

fail() {
  echo "check-bundle: FAIL $*"
  exit 1
}

[ -x "$C/MacOS/Notted" ] || fail "missing Contents/MacOS/Notted"
[ -x "$C/MacOS/audiocap" ] || fail "missing Contents/MacOS/audiocap"
[ -f "$C/Frameworks/libsherpa-onnx-c-api.dylib" ] || fail "missing Contents/Frameworks/libsherpa-onnx-c-api.dylib"
compgen -G "$C/Frameworks/libonnxruntime*.dylib" >/dev/null || fail "missing Contents/Frameworks/libonnxruntime*.dylib"

rpaths="$(otool -l "$C/MacOS/Notted" | awk '/LC_RPATH/{grab=2; next} grab && /path /{print $2; grab=0}')"
case "$rpaths" in
  *sherpa-onnx-go-macos*) fail "LC_RPATH into the Go module cache" ;;
esac
case "$rpaths" in
  *"@executable_path/../Frameworks"*) ;;
  *) fail "LC_RPATH @executable_path/../Frameworks not present" ;;
esac

codesign --verify --deep --strict "$APP" 2>/dev/null || fail "codesign --verify --deep --strict"

id="$(plutil -extract CFBundleIdentifier raw "$C/Info.plist" 2>/dev/null)"
[ "$id" = "tech.bhlabs.notted" ] || fail "CFBundleIdentifier is '$id'"

sig="$(codesign -dv --verbose=2 "$C/MacOS/audiocap" 2>&1)"
grep -qx 'Identifier=tech.bhlabs.notted.audiocap' <<<"$sig" ||
  fail "audiocap identifier is not tech.bhlabs.notted.audiocap"

echo "check-bundle: OK"
