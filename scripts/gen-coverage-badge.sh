#!/usr/bin/env bash
# Builds the README coverage badge from scripts/coverage-thresholds.sh, the
# single source of truth for the per-package coverage minimums. No external
# coverage service involved — the badge just states the enforced minimums,
# rendered via a shields.io static badge URL (no upload, no account).
set -euo pipefail

if [ "$#" -ne 1 ] || { [ "$1" != "--check" ] && [ "$1" != "--write" ]; }; then
  echo "usage: $0 --check|--write" >&2
  exit 2
fi

mode="$1"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
readme="$repo_root/README.md"
start_marker="<!-- coverage-badge:start -->"
end_marker="<!-- coverage-badge:end -->"

source "$script_dir/coverage-thresholds.sh"

message="core%20${CORE_COVERAGE_MIN}%25%20%C2%B7%20cli%20${CLI_COVERAGE_MIN}%25%20%C2%B7%20gui%20${GUI_COVERAGE_MIN}%25"
badge_url="https://img.shields.io/badge/coverage-${message}-brightgreen"
badge_line="[![coverage](${badge_url})](scripts/coverage-thresholds.sh)"

if ! grep -qF "$start_marker" "$readme" || ! grep -qF "$end_marker" "$readme"; then
  echo "README.md is missing the ${start_marker} / ${end_marker} markers" >&2
  exit 1
fi

block="$(printf '%s\n%s\n%s' "$start_marker" "$badge_line" "$end_marker")"

if [ "$mode" = "--check" ]; then
  current="$(awk "/^${start_marker//\//\\/}\$/,/^${end_marker//\//\\/}\$/" "$readme")"
  if [ "$current" != "$block" ]; then
    echo "README.md coverage badge is stale — run: just coverageBadge" >&2
    exit 1
  fi
  echo "README.md coverage badge is up to date"
else
  tmp="$(mktemp)"
  trap 'rm -f "$tmp"' EXIT
  awk -v block="$block" "
    /^${start_marker//\//\\/}\$/ { print block; skipping=1; next }
    /^${end_marker//\//\\/}\$/ { skipping=0; next }
    !skipping { print }
  " "$readme" > "$tmp"
  mv "$tmp" "$readme"
  echo "README.md coverage badge updated"
fi
