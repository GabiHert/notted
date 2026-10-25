#!/usr/bin/env bash
printf 'ARGV:%s\n' "$*"
printf 'CWD:%s\n' "$(pwd -P)"
printf 'KEY:%s TOKEN:%s\n' "${ANTHROPIC_API_KEY:-none}" "${ANTHROPIC_AUTH_TOKEN:-none}"
printf 'TTY:%s TERM:%s\n' "$([ -t 0 ] && echo yes || echo no)" "$TERM"
while IFS= read -r line; do
  case "$line" in
    size) stty size ;;
    utf8) printf 'ação ✓\n' ;;
    exit) exit 7 ;;
    *) printf 'ECHO:%s\n' "$line" ;;
  esac
done
