#!/usr/bin/env bash
# Tests scripts/changelog.sh, which make release uses to refuse a release without notes and to
# move [Unreleased] under the new version.
set -uo pipefail
cd "$(dirname "$0")/.."
cl=scripts/changelog.sh
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT
pass=0 failed=0
ok() { if eval "$2"; then pass=$((pass + 1)); else echo "test-changelog: FAIL $1"; failed=$((failed + 1)); fi; }

printf '# Changelog\n\n## [Unreleased]\n\n### Added\n- Updates from GitHub Releases.\n\n## [0.1.0] - 2026-11-01\n\n### Added\n- First.\n' >"$T/full.md"
printf '# Changelog\n\n## [Unreleased]\n\n## [0.1.0] - 2026-11-01\n\n- First.\n' >"$T/empty.md"

ok "check passes with Unreleased bullets" 'bash $cl check v0.2.0 "$T/full.md"'
ok "check refuses an empty Unreleased" '! bash $cl check v0.2.0 "$T/empty.md" 2>/dev/null'
ok "check passes for an existing section" 'bash $cl check v0.1.0 "$T/empty.md"'
cp "$T/full.md" "$T/p.md"
bash $cl promote v0.2.0 2026-11-11 "$T/p.md"
ok "promote adds the version heading" 'grep -qx "## \[0.2.0\] - 2026-11-11" "$T/p.md"'
ok "promote keeps an empty Unreleased" '[ -z "$(awk "/^## \\[Unreleased\\]/{on=1;next} /^## /{on=0} on && /^- /" "$T/p.md")" ]'
ok "notes print the promoted section" '[ "$(bash $cl notes v0.2.0 "$T/p.md")" = "$(printf "### Added\n- Updates from GitHub Releases.")" ]'
ok "check passes the CHANGELOG in the repo" 'bash $cl check v99.0.0'

echo "test-changelog: $pass passed, $failed failed"
[ "$failed" -eq 0 ]
