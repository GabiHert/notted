#!/usr/bin/env bash
# Prints the SHA-1 of the valid "Phonon Local Signing" code-signing identity, or "-" (ad hoc).
set -uo pipefail
hash="$(security find-identity -v -p codesigning 2>/dev/null | awk -v n='"Phonon Local Signing"' 'index($0, n) {print $2; exit}')"
echo "${hash:--}"
