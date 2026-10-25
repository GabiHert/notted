#!/usr/bin/env bash
# Fake `open` for tests: records its argv to $FAKE_OPEN_ARGS; fails when $FAKE_OPEN_FAIL is set.
printf '%s\n' "$*" > "$FAKE_OPEN_ARGS"
[ -z "${FAKE_OPEN_FAIL:-}" ] || { echo "no such pane" >&2; exit 1; }
