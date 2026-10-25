#!/usr/bin/env bash
# Fake audiocap for tests: speaks the helper's JSON event contract and writes small WAVs.
# Behaviour comes from $FAKE_AUDIOCAP_MODE; one line is appended to $FAKE_AUDIOCAP_COUNT per start;
# the argv is written to $FAKE_AUDIOCAP_ARGS.
here="$(cd "$(dirname "$0")" && pwd)"
[ -n "${FAKE_AUDIOCAP_ARGS:-}" ] && printf '%s\n' "$*" > "$FAKE_AUDIOCAP_ARGS"
[ -n "${FAKE_AUDIOCAP_COUNT:-}" ] && echo start >> "$FAKE_AUDIOCAP_COUNT"
if [ "${1:-}" = "status" ] || [ "${1:-}" = "check" ]; then
  perm() { printf '{"event":"permission","name":"%s","granted":%s%s}\n' "$1" "$2" "${3:+,\"status\":\"$3\"}"; }
  case "${FAKE_AUDIOCAP_MODE:-normal}" in
    perm-allowed) perm microphone true allowed; perm screen true allowed; exit 0 ;;
    perm-not-asked) perm microphone false not_asked; perm screen false denied; exit 0 ;;
    perm-denied) perm microphone false denied; perm screen false denied; exit 2 ;;
    perm-legacy) perm microphone true; perm screen false; exit 3 ;;
    perm-empty) exit 0 ;;
    perm-partial) perm microphone true allowed; exit 0 ;;
    perm-garbage) echo 'not json'; echo '{"event":"permission","name":"microphone","granted":true,"status":"maybe"}'; exit 0 ;;
    perm-crash) echo 'boom' >&2; exit 1 ;;
    perm-hang) sleep 5; exit 0 ;;
    *) exit 64 ;;
  esac
fi
[ "${1:-}" = "record" ] || exit 64
out=""
while [ $# -gt 0 ]; do
  case "$1" in
    --out) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
mkdir -p "$out"
mode="${FAKE_AUDIOCAP_MODE:-normal}"

chunk() { # track index
  local f="$out/$1-$(printf '%06d' "$2").wav"
  cp "$here/tiny.wav" "$f"
  printf '{"event":"chunk","track":"%s","index":%d,"path":"%s","start_ms":%d,"end_ms":%d}\n' \
    "$1" "$2" "$f" $((($2 - 1) * 100)) $(($2 * 100))
}

case "$mode" in
  mic-denied)
    echo '{"event":"error","code":"mic_denied","message":"microphone denied"}'; exit 2 ;;
  screen-denied)
    echo '{"event":"error","code":"screen_denied","message":"screen denied"}'; exit 3 ;;
  no-device)
    echo '{"event":"error","code":"no_device","message":"no device"}'; exit 4 ;;
  crash)
    echo '{"event":"started","t0_ms":1000}'
    chunk mic 1
    echo '{"event":"error","code":"capture_failed","message":"capture_failed: stream stopped"}'
    exit 1 ;;
  killed)
    echo '{"event":"started","t0_ms":1000}'
    echo 'not json at all'
    kill -9 $$ ;;
  ignore-int)
    trap '' INT TERM
    echo '{"event":"started","t0_ms":1000}'
    while true; do sleep 0.05; done ;;
  silent-hang)
    trap 'exit 0' INT TERM
    while true; do sleep 0.05; done ;;
  normal)
    stop=0
    trap 'stop=1' INT TERM
    echo '{"event":"started","t0_ms":1000}'
    chunk mic 1
    chunk others 1
    while [ "$stop" -eq 0 ]; do sleep 0.05; done
    chunk mic 2
    chunk others 2
    echo '{"event":"stopped"}'
    exit 0 ;;
  *) exit 99 ;;
esac
