#!/usr/bin/env bash
# Runs every Go fuzz target (func FuzzXxx in *_test.go) for a short time.
#   bash scripts/fuzz.sh [duration]      default 15s per target
# Findings are written to testdata/fuzz/ of the package; commit them as
# regression seeds after fixing the bug.
set -Eeuo pipefail
cd "$(dirname "$0")/.."
duration=${1:-15s}
go_cmd=${GO:-go}

status=0
while IFS=: read -r file _ line; do
  name=${line#func }
  name=${name%%(*}
  pkg=./$(dirname "$file")
  echo "== $pkg $name ($duration)"
  if ! $go_cmd test -run='^$' -fuzz="^${name}\$" -fuzztime="$duration" "$pkg"; then
    status=1
  fi
done < <(grep -rn --include='*_test.go' '^func Fuzz' internal | sort)
exit $status
