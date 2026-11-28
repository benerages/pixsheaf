#!/usr/bin/env bash
# Prints the changelog section for a release from Conventional Commit
# subjects since the previous release tag (or since the first commit).
#   scripts/changelog.sh v1.0.0 [ref]
set -Eeuo pipefail
tag=${1:?usage: $0 <tag> [ref]}
ref=${2:-HEAD}
prev=$(git describe --tags --abbrev=0 --match 'v[0-9]*' "$ref^" 2>/dev/null || true)
range=${prev:+$prev..}$ref

section() { # title, regex of types
  local lines
  lines=$(git log --no-merges --format='%s' "$range" |
    grep -E "^($2)(\([^)]*\))?!?: " |
    sed -E 's/^[a-z]+(\(([^)]*)\))?!?: /- **\2** /; s/- \*\*\*\* /- /' || true)
  if [[ -n $lines ]]; then
    printf '\n### %s\n\n%s\n' "$1" "$lines"
  fi
}

printf '## %s (%s)\n' "${tag#v}" "$(git log -1 --format=%cs "$ref")"
breaking=$(git log --no-merges --format='%s' "$range" | grep -E '^[a-z]+(\([^)]*\))?!: ' | sed 's/^/- /' || true)
[[ -z $breaking ]] || printf '\n### Breaking changes\n\n%s\n' "$breaking"
section "Features" "feat"
section "Fixes" "fix"
section "Security" "sec|security"
section "Documentation" "docs"
section "Tests" "test"
section "Build and CI" "build|ci|chore|refactor|perf|style"
