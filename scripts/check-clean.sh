#!/usr/bin/env bash
# Source check: no analytics, update checks, foreign hosts, quant imports, missing notices, wrong bundle id.
set -uo pipefail
cd "$(dirname "$0")/.."

fail=0
report() { # rule file line text
  echo "check-clean: FAIL $1: $2:$3: $4"
  fail=1
}

files=$(git ls-files -co --exclude-standard | grep -E '\.(go|ts|tsx|js|mjs|swift|json|sh|plist)$|^internal/application/service/archive_template/' |
  grep -vE '^(go\.sum|frontend/package-lock\.json|scripts/check-clean\.sh|scripts/test-checks\.sh)$' |
  grep -vE '^frontend/wailsjs/' | sort -u)

# scan <rule> <regex> <case-insensitive: 1|0> [file-filter-regex] [file-skip-regex]
scan() {
  local rule=$1 re=$2 ci=$3 filter=${4:-.} skip=${5:-} f
  local flags=-nIE
  [ "$ci" = 1 ] && flags=-nIiE
  for f in $files; do
    [ -f "$f" ] || continue
    printf '%s\n' "$f" | grep -qE "$filter" || continue
    [ -n "$skip" ] && printf '%s\n' "$f" | grep -qE "$skip" && continue
    while IFS= read -r hit; do
      report "$rule" "$f" "${hit%%:*}" "${hit#*:}"
    done < <(grep $flags -e "$re" "$f")
  done
}

scan analytics 'posthog|mixpanel|amplitude|segment\.(io|com)|sentry|google-analytics|googletagmanager|gtag\(|plausible|datadog|bugsnag|telemetry|analytics' 1
# Updates come only from this repo's GitHub Releases, through the release updater and its card,
# its bindings and its release scripts.
updater_files='^(internal/update/|internal/infra/wails/app_update(_test)?\.go$|internal/infra/run\.go$|frontend/src/components/(UpdatesCard|SettingsView)|frontend/src/(preview/mockApp|test/bindingsMock)\.ts$|scripts/(release|build-release|changelog)\.sh$)'
scan update-check 'auto-?update|auto_update|updater|checkforupdate|sparkle|selfupdate|go-update' 1 . "$updater_files"

for f in $files; do
  [ -f "$f" ] || continue
  case "$f" in internal/update/*_test.go) continue ;; esac # fake GitHub servers
  while IFS= read -r hit; do
    ln=${hit%%:*}
    rest=${hit#*:}
    while IFS= read -r url; do
      case "$url" in
        https://github.com/k2-fsa/sherpa-onnx/releases/download/*) ;;
        https://api.github.com) ;; # internal/update: this repo's releases only
        'https://github.com/$REPO/releases/tag/'*) ;;
        https://github.com/GabiHert/notted/releases/*) ;; # this repo's releases # scripts/release.sh prints the new release
        http://www.apple.com/DTDs/PropertyList-1.0.dtd) ;;
        *) report network-host "$f" "$ln" "$url" ;;
      esac
    done < <(printf '%s\n' "$rest" | grep -oE "https?://[^\"'\` )>]+")
  done < <(grep -nIE "https?://[^\"'\` )>]+" "$f")
done

scan reference-import '"quant/|"github\.com/brienze1/quant' 0 '(\.go|^go\.mod)$'
if [ -f go.mod ]; then
  while IFS= read -r hit; do
    report reference-import go.mod "${hit%%:*}" "${hit#*:}"
  done < <(grep -nE '"quant/|"github\.com/brienze1/quant' go.mod)
fi

# notice: every listed copied file exists and carries the copyright line in its first 30 lines.
if [ -f NOTICE.md ]; then
  while IFS= read -r row; do
    path=$(printf '%s\n' "$row" | awk -F'`' '{print $2}')
    [ -n "$path" ] || continue
    if [ ! -f "$path" ]; then
      report notice "$path" 0 "listed in NOTICE.md but missing"
    elif ! head -n 30 "$path" | grep -qF 'Copyright (c) 2026 Luis Brienze'; then
      report notice "$path" 1 "no 'Copyright (c) 2026 Luis Brienze' line in first 30 lines"
    fi
  done < <(awk '/^## Copied files/{on=1; next} /^## /{on=0} on && /^\| *`/' NOTICE.md)
else
  report notice NOTICE.md 0 "missing"
fi

# bundle-id
want=tech.bhlabs.phonon
plist=build/darwin/Info.plist
if [ ! -f "$plist" ]; then
  report bundle-id "$plist" 0 "missing"
else
  n=$(grep -c '<key>CFBundleIdentifier</key>' "$plist")
  if [ "$n" -ne 1 ]; then
    report bundle-id "$plist" 0 "expected exactly one CFBundleIdentifier, found $n"
  else
    got=$(grep -A1 '<key>CFBundleIdentifier</key>' "$plist" | sed -n 's:.*<string>\(.*\)</string>.*:\1:p')
    [ "$got" = "$want" ] || report bundle-id "$plist" 0 "CFBundleIdentifier is '$got', want $want"
  fi
fi
built=build/bin/Phonon.app/Contents/Info.plist
if [ -f "$built" ]; then
  got=$(plutil -extract CFBundleIdentifier raw "$built" 2>/dev/null)
  [ "$got" = "$want" ] || report bundle-id "$built" 0 "CFBundleIdentifier is '$got', want $want"
fi

if [ "$fail" -ne 0 ]; then exit 1; fi
echo "check-clean: OK"
