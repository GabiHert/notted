#!/usr/bin/env bash
out="$STUB_OUT"; mkdir -p "$out"
printf '%s\n' "$@" > "$out/argv"
pwd -P > "$out/cwd"
env > "$out/env"
cat > "$out/stdin"
prev=""
for a in "$@"; do
  if [ "$prev" = "--append-system-prompt-file" ]; then cp "$a" "$out/prompt"; echo "$a" > "$out/prompt-path"; fi
  prev="$a"
done
[ -n "${STUB_STDERR:-}" ] && printf '%s\n' "$STUB_STDERR" >&2
[ -n "${STUB_CHILD:-}" ] && { sleep 30 & echo $! > "$STUB_CHILD"; }
[ -n "${STUB_SLEEP:-}" ] && sleep "$STUB_SLEEP"
if [ -n "${STUB_REPLY_SUGGEST:-}" ] && grep -q '`suggest` format' "$out/stdin"; then
  printf '%s' "$STUB_REPLY_SUGGEST"
else
  printf '%s' "${STUB_REPLY:-# Summary: stub}"
fi
exit "${STUB_EXIT:-0}"
