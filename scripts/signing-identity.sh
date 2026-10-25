#!/usr/bin/env bash
# Prints the SHA-1 of the valid "Notted Local Signing" code-signing identity, or "-" (ad hoc).
set -uo pipefail
hash="$(security find-identity -v -p codesigning 2>/dev/null | awk -v n='"Notted Local Signing"' 'index($0, n) {print $2; exit}')"
echo "${hash:--}"
