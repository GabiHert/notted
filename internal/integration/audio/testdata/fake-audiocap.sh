#!/usr/bin/env bash
# Fake audiocap for tests: speaks the helper's JSON event contract and writes small WAVs.
# Behaviour comes from $FAKE_AUDIOCAP_MODE; one line is appended to $FAKE_AUDIOCAP_COUNT per start;
# the argv is written to $FAKE_AUDIOCAP_ARGS.
here="$(cd "$(dirname "$0")" && pwd)"
[ -n "${FAKE_AUDIOCAP_ARGS:-}" ] && printf '%s\n' "$*" > "$FAKE_AUDIOCAP_ARGS"
[ -n "${FAKE_AUDIOCAP_COUNT:-}" ] && echo start >> "$FAKE_AUDIOCAP_COUNT"
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
