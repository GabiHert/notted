#!/usr/bin/env bash
# Proves each rule of check-clean.sh and check-invariants.sh fails when violated.
set -uo pipefail
cd "$(dirname "$0")/.."
root=$(pwd)
pass=0
total=9
tmps=()
cleanup() { for t in "${tmps[@]:-}"; do [ -n "$t" ] && rm -rf "$t"; done; }
trap cleanup EXIT

fresh() {
  tmp=$(mktemp -d)
  tmps+=("$tmp")
  rsync -a --exclude node_modules --exclude build/bin ./ "$tmp/"
}

# expect <name> <script> <rule>; runs in $tmp
expect() {
  local name=$1 script=$2 rule=$3 out rc
  out=$(cd "$tmp" && bash "scripts/$script" 2>&1)
  rc=$?
  if [ "$rc" -eq 1 ] && printf '%s' "$out" | grep -q "FAIL $rule:"; then
    echo "test-checks: PASS $name"
    pass=$((pass + 1))
  else
    echo "test-checks: FAIL $name (exit $rc)"
    printf '%s\n' "$out" | head -5
  fi
}

fresh; mkdir -p "$tmp/internal"; echo '// posthog' > "$tmp/internal/x.go"
expect "analytics" check-clean.sh analytics

fresh; mkdir -p "$tmp/internal"; echo '// checkForUpdate' > "$tmp/internal/x.go"
expect "update-check" check-clean.sh update-check

fresh; mkdir -p "$tmp/internal"; printf 'package x\n\nconst u = "https://example.org/x"\n' > "$tmp/internal/x.go"
expect "network-host" check-clean.sh network-host

fresh; mkdir -p "$tmp/internal"; printf 'package x\n\nimport _ "quant/internal/x"\n' > "$tmp/internal/x.go"
expect "reference-import" check-clean.sh reference-import

fresh; mkdir -p "$tmp/internal"; echo 'package x' > "$tmp/internal/nolicense.go"
row='| `internal/nolicense.go` | `internal/nolicense.go` (lines 1-1) | planted |'
awk -v row="$row" '{print} /^\|---\|---\|---\|$/{print row}' "$tmp/NOTICE.md" > "$tmp/NOTICE.new" && mv "$tmp/NOTICE.new" "$tmp/NOTICE.md"
expect "notice" check-clean.sh notice

fresh; sed -i.bak 's/tech\.bhlabs\.notted/com.wails.notted/' "$tmp/build/darwin/Info.plist"; rm -f "$tmp/build/darwin/Info.plist.bak"
expect "bundle-id" check-clean.sh bundle-id

fresh; mkdir -p "$tmp/internal"; echo '// acme planner' > "$tmp/internal/k.go"
expect "knowledge" check-invariants.sh knowledge

fresh; mkdir -p "$tmp/internal"; flag="--dangerously-skip-""permissions"; echo "// $flag" > "$tmp/internal/f.go"
expect "permission-flag" check-invariants.sh permission-flag

fresh; mkdir -p "$tmp/internal"; printf '// \xff\xfe acme planner\n' > "$tmp/internal/bin.go"
expect "knowledge-non-utf8" check-invariants.sh knowledge

if bash scripts/check-clean.sh >/dev/null 2>&1 && bash scripts/check-invariants.sh >/dev/null 2>&1; then
  echo "test-checks: PASS clean tree"
else
  echo "test-checks: FAIL clean tree"
  pass=0
fi
echo "test-checks: $pass/$total PASS"
[ "$pass" -eq "$total" ]
