#!/usr/bin/env bash
# Bundle check: the built Phonon.app carries its helper, links only system libraries and is signed ad hoc or with Phonon Local Signing.
set -uo pipefail
cd "$(dirname "$0")/.."

APP="${1:-build/bin/Phonon.app}"
C="$APP/Contents"

fail() {
  echo "check-bundle: FAIL $*"
  exit 1
}

[ -x "$C/MacOS/Phonon" ] || fail "missing Contents/MacOS/Phonon"
[ -x "$C/MacOS/audiocap" ] || fail "missing Contents/MacOS/audiocap"
# whisper.cpp is linked in statically: the binary needs only system libraries, Metal among them.
libs="$(otool -L "$C/MacOS/Phonon" | tail -n +2 | awk '{print $1}')"
grep -q '^/System/Library/Frameworks/Metal.framework/' <<<"$libs" || fail "Phonon does not link Metal"
while IFS= read -r lib; do
  case "$lib" in
    /usr/lib/* | /System/Library/*) ;;
    *) fail "Phonon links $lib, outside the system" ;;
  esac
done <<<"$libs"
! compgen -G "$C/Frameworks/*.dylib" >/dev/null || fail "unexpected libraries in Contents/Frameworks"

codesign --verify --deep --strict "$APP" 2>/dev/null || fail "codesign --verify --deep --strict"

id="$(plutil -extract CFBundleIdentifier raw "$C/Info.plist" 2>/dev/null)"
[ "$id" = "tech.bhlabs.phonon" ] || fail "CFBundleIdentifier is '$id'"

sig="$(codesign -dv --verbose=2 "$C/MacOS/audiocap" 2>&1)"
grep -qx 'Identifier=tech.bhlabs.phonon.audiocap' <<<"$sig" ||
  fail "audiocap identifier is not tech.bhlabs.phonon.audiocap"

# kind_of <path>: adhoc, local (Phonon Local Signing) or other.
kind_of() {
  local info
  info="$(codesign -dv --verbose=2 "$1" 2>&1)"
  if grep -qx 'Signature=adhoc' <<<"$info"; then
    echo adhoc
  elif grep -qx 'Authority=Phonon Local Signing' <<<"$info"; then
    echo local
  else
    echo other
  fi
}
want="$(kind_of "$APP")"
[ "$want" != other ] || fail "Phonon.app is signed neither ad hoc nor with Phonon Local Signing"
for f in "$C/MacOS/audiocap"; do
  [ -e "$f" ] || continue
  [ "$(kind_of "$f")" = "$want" ] || fail "${f#"$C/"} is signed differently from Phonon.app"
done
if [ "$want" = adhoc ]; then
  echo "check-bundle: OK (signed ad hoc)"
else
  echo "check-bundle: OK (signed with Phonon Local Signing)"
fi
