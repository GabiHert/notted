#!/usr/bin/env bash
# Signs the built Phonon.app inside-out with "Phonon Local Signing" when the keychain has it, else ad hoc.
set -euo pipefail
cd "$(dirname "$0")/.."
APP="${1:-build/bin/Phonon.app}"
C="$APP/Contents"
id="$(bash scripts/signing-identity.sh)"
if [ "$id" = "-" ]; then
  echo 'sign-bundle: no "Phonon Local Signing" certificate in your keychain; signing ad hoc. macOS asks for the microphone and screen recording permissions again after each build. Run bash scripts/create-signing-identity.sh once to stop that.'
else
  echo 'sign-bundle: signing with "Phonon Local Signing"'
fi
for lib in "$C"/Frameworks/*.dylib; do
  [ -e "$lib" ] || continue
  codesign -f -s "$id" "$lib"
done
codesign -f -s "$id" --identifier tech.bhlabs.phonon.audiocap "$C/MacOS/audiocap"
codesign -f -s "$id" "$APP"
