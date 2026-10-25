#!/usr/bin/env bash
# Proves the signing scripts and the bundle check, using fake security, codesign, otool and plutil.
set -uo pipefail
cd "$(dirname "$0")/.."
root=$(pwd)
pass=0
total=12
tmps=()
cleanup() { for t in "${tmps[@]:-}"; do [ -n "$t" ] && rm -rf "$t"; done; }
trap cleanup EXIT

HASH=13696039679D3B35D7E756B113E7576F4DE421A7
NOTFOUND='sign-bundle: no "Notted Local Signing" certificate in your keychain; signing ad hoc. macOS asks for the microphone and screen recording permissions again after each build. Run bash scripts/create-signing-identity.sh once to stop that.'
UNTRUSTED='The certificate "Notted Local Signing" is in your keychain, but macOS does not trust it for code signing. Open Keychain Access, double-click "Notted Local Signing", open Trust, and set Code Signing to Always Trust. Then run make build.'

# fresh <state> <sign-kind>: new temp folder with fakes and a fake bundle.
fresh() {
  tmp=$(mktemp -d)
  tmps+=("$tmp")
  mkdir -p "$tmp/bin" "$tmp/tmpdir"
  echo "$1" >"$tmp/state"
  echo "${2:-adhoc}" >"$tmp/sign-kind"
  : >"$tmp/calls"
  cat >"$tmp/bin/security" <<FAKE
#!/bin/sh
echo "security \$*" >>"$tmp/calls"
line='  1) $HASH "Notted Local Signing"'
case "\$1" in
  find-identity)
    state=\$(cat "$tmp/state")
    if [ "\$2" = "-v" ]; then
      if [ "\$state" = valid ]; then echo "\$line"; echo "     1 valid identities found"; else echo "     0 valid identities found"; fi
    else
      case "\$state" in
        untrusted) echo "\$line (CSSMERR_TP_NOT_TRUSTED)" ;;
        valid) echo "\$line" ;;
      esac
    fi ;;
  import) echo untrusted >"$tmp/state" ;;
  add-trusted-cert)
    if [ -e "$tmp/trust-fails" ]; then exit 1; fi
    echo valid >"$tmp/state" ;;
esac
exit 0
FAKE
  cat >"$tmp/bin/codesign" <<FAKE
#!/bin/sh
echo "codesign \$*" >>"$tmp/calls"
case "\$1" in
  --verify) exit 0 ;;
  -dv)
    for last; do :; done
    case "\$last" in
      */audiocap) echo "Identifier=tech.bhlabs.notted.audiocap" ;;
      *) echo "Identifier=tech.bhlabs.notted" ;;
    esac
    case "\$(cat "$tmp/sign-kind")" in
      adhoc) echo "Signature=adhoc" ;;
      local) echo "Authority=Notted Local Signing" ;;
      mixed)
        case "\$last" in
          */audiocap) echo "Signature=adhoc" ;;
          *) echo "Authority=Notted Local Signing" ;;
        esac ;;
      foreign) echo "Authority=Apple Development: Ana" ;;
    esac ;;
esac
exit 0
FAKE
  cat >"$tmp/bin/otool" <<'FAKE'
#!/bin/sh
echo "Load command 20"
echo "          cmd LC_RPATH"
echo "      cmdsize 48"
echo "         path @executable_path/../Frameworks (offset 12)"
FAKE
  printf '#!/bin/sh\necho tech.bhlabs.notted\n' >"$tmp/bin/plutil"
  chmod +x "$tmp/bin/"*
  local c="$tmp/Notted.app/Contents"
  mkdir -p "$c/MacOS" "$c/Frameworks"
  printf '#!/bin/sh\n' >"$c/MacOS/Notted"
  printf '#!/bin/sh\n' >"$c/MacOS/audiocap"
  chmod +x "$c/MacOS/Notted" "$c/MacOS/audiocap"
  : >"$c/Frameworks/libonnxruntime.1.17.1.dylib"
  : >"$c/Frameworks/libsherpa-onnx-c-api.dylib"
  : >"$c/Info.plist"
}

# run <script> [args...]: runs a script with the fakes first on PATH; sets out and rc.
run() {
  out=$(PATH="$tmp/bin:$PATH" NOTTED_KEYCHAIN="$tmp/test.keychain-db" TMPDIR="$tmp/tmpdir" bash "$@" 2>&1)
  rc=$?
}

# verdict <name> <condition-result>
verdict() {
  if [ "$2" -eq 0 ]; then
    echo "test-signing: PASS $1"
    pass=$((pass + 1))
  else
    echo "test-signing: FAIL $1 (exit $rc)"
    printf '%s\n' "$out" | head -5
  fi
}

# keydir_gone: the key folder named by the logged import line exists no more.
keydir_gone() {
  local kd
  kd="$(sed -n 's|^security import \(.*\)/id\.p12 -k .*|\1|p' "$tmp/calls")"
  [ -n "$kd" ] && [ ! -e "$kd" ]
}
has() { grep -qF -- "$1" <<<"$out"; }
called() { grep -qF -- "$1" "$tmp/calls"; }

fresh valid
run scripts/signing-identity.sh
[ "$rc" -eq 0 ] && [ "$out" = "$HASH" ]
verdict identity-found $?

fresh none
run scripts/signing-identity.sh
[ "$rc" -eq 0 ] && [ "$out" = "-" ]
verdict identity-missing $?

fresh valid
app="$tmp/Notted.app"
run scripts/sign-bundle.sh "$app"
want="codesign -f -s $HASH $app/Contents/Frameworks/libonnxruntime.1.17.1.dylib
codesign -f -s $HASH $app/Contents/Frameworks/libsherpa-onnx-c-api.dylib
codesign -f -s $HASH --identifier tech.bhlabs.notted.audiocap $app/Contents/MacOS/audiocap
codesign -f -s $HASH $app"
[ "$rc" -eq 0 ] && has 'sign-bundle: signing with "Notted Local Signing"' && [ "$(grep '^codesign' "$tmp/calls")" = "$want" ]
verdict sign-local $?

fresh none
app="$tmp/Notted.app"
run scripts/sign-bundle.sh "$app"
[ "$rc" -eq 0 ] && has "$NOTFOUND" && [ "$(grep -c '^codesign' "$tmp/calls")" -eq 4 ] && [ "$(grep '^codesign' "$tmp/calls" | grep -c -- ' -s - ')" -eq 4 ]
verdict sign-adhoc $?

fresh none adhoc
run scripts/check-bundle.sh "$tmp/Notted.app"
[ "$rc" -eq 0 ] && has 'check-bundle: OK (signed ad hoc)'
verdict check-adhoc $?

fresh none local
run scripts/check-bundle.sh "$tmp/Notted.app"
[ "$rc" -eq 0 ] && has 'check-bundle: OK (signed with Notted Local Signing)'
verdict check-local $?

fresh none mixed
run scripts/check-bundle.sh "$tmp/Notted.app"
[ "$rc" -eq 1 ] && has 'check-bundle: FAIL MacOS/audiocap is signed differently from Notted.app'
verdict check-mixed $?

fresh none foreign
run scripts/check-bundle.sh "$tmp/Notted.app"
[ "$rc" -eq 1 ] && has 'check-bundle: FAIL Notted.app is signed neither ad hoc nor with Notted Local Signing'
verdict check-foreign $?

fresh valid
run scripts/create-signing-identity.sh
[ "$rc" -eq 0 ] && has 'The code-signing identity "Notted Local Signing" already exists. Nothing to do.' && ! called 'security import'
verdict create-exists $?

fresh untrusted
run scripts/create-signing-identity.sh
[ "$rc" -eq 1 ] && has "$UNTRUSTED" && ! called 'security import'
verdict create-untrusted $?

fresh none
run scripts/create-signing-identity.sh
kc="$tmp/test.keychain-db"
[ "$rc" -eq 0 ] &&
  has "Creating the code-signing identity \"Notted Local Signing\" in $kc." &&
  has 'macOS now asks for your login password to trust the certificate for code signing.' &&
  has 'Created the code-signing identity "Notted Local Signing". Run make build: macOS asks for the microphone and Screen & System Audio Recording permissions once more, then keeps them across builds and updates.' &&
  grep -Eq "^security import [^ ]+/id\.p12 -k $kc -P [A-Za-z0-9]{24} -T /usr/bin/codesign\$" "$tmp/calls" &&
  grep -Eq "^security add-trusted-cert -r trustRoot -p codeSign -k $kc [^ ]+/cert\.pem\$" "$tmp/calls" &&
  [ -z "$(grep '^security' "$tmp/calls" | grep -E ' -d |login\.keychain')" ] &&
  keydir_gone
verdict create-new $?

fresh none
touch "$tmp/trust-fails"
run scripts/create-signing-identity.sh
[ "$rc" -eq 1 ] && [ "$(printf '%s\n' "$out" | tail -n 1)" = "$UNTRUSTED" ] && keydir_gone
verdict create-trust-refused $?

echo "test-signing: $pass/$total passed"
[ "$pass" -eq "$total" ]
