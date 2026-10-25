#!/usr/bin/env bash
if [ "$1" = "prompt" ] && [ "$2" = "--project" ]; then
  k="$3"
  if [ "$k" = "missing" ]; then
    echo "No project file instructions/projects/missing.md." >&2
    exit 2
  fi
  echo "profile: $MEETINGS_PROFILE" >&2
  printf 'PROMPT project=%s profile=%s' "$k" "$MEETINGS_PROFILE"
  env > "${STUB_OUT:-/dev/null}.env" 2>/dev/null
  exit 0
fi
exit 64
