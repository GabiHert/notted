#!/usr/bin/env bash
# Source check: Notted holds no project knowledge, never writes the permission-skipping flag, and names Claude Code's bypass mode in one file only (the review switch, off by default).
set -uo pipefail
export LC_ALL=C
cd "$(dirname "$0")/.."

fail=0
files=$(git ls-files -co --exclude-standard |
  grep -vE '^(go\.sum|frontend/package-lock\.json|scripts/check-invariants\.sh|scripts/test-checks\.sh)$' | sort -u)

flag="--dangerously-skip-""permissions"
bypass_ok="internal/domain/permission_mode.go"

for f in $files; do
  [ -f "$f" ] || continue
  while IFS= read -r hit; do
    echo "check-invariants: FAIL knowledge: $f:${hit%%:*}: ${hit#*:}"
    fail=1
  done < <(sed -E 's/(repeating-)?linear-gradient//g' "$f" | grep -nIiwE 'planner|linear|maxsatt|cb5')
  while IFS= read -r hit; do
    echo "check-invariants: FAIL permission-flag: $f:${hit%%:*}: ${hit#*:}"
    fail=1
  done < <(if [ "$f" = "$bypass_ok" ]; then grep -nIF -e "$flag" "$f"; else grep -nIF -e "$flag" -e "bypassPermissions" "$f"; fi)
done

if [ "$fail" -ne 0 ]; then exit 1; fi
echo "check-invariants: OK"
