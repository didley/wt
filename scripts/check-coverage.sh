#!/usr/bin/env bash
# Fails if the total statement coverage in a `go tool cover -func` profile is
# below a threshold. Used by `just test`/CI to gate coverage per package,
# the same way Jest's coverageThreshold does.
set -euo pipefail

if [ "$#" -lt 3 ] || [ "$#" -gt 4 ]; then
  echo "usage: $0 <coverprofile> <min-percent> <label> [module-dir]" >&2
  exit 2
fi

profile="$1"
threshold="$2"
label="$3"
moduledir="${4:-.}"

total="$(go -C "$moduledir" tool cover -func="$profile" | tail -1 | awk '{print $NF}' | tr -d '%')"

if awk -v t="$total" -v m="$threshold" 'BEGIN { exit !(t + 0 < m + 0) }'; then
  echo "coverage FAIL: $label is ${total}%, below the ${threshold}% minimum" >&2
  exit 1
fi

echo "coverage ok: $label is ${total}% (>= ${threshold}%)"
