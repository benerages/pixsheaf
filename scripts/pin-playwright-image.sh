#!/usr/bin/env bash
# Pins tests/e2e/ui.Dockerfile to the Playwright image that matches
# @playwright/test in web/package.json, by tag and digest. Run it after
# changing the Playwright version by hand; Renovate does it otherwise.
# Requires Docker with buildx and internet access.
set -Eeuo pipefail
cd "$(dirname "$0")/.."

version=$(sed -n 's/.*"@playwright\/test": "\([^"]*\)".*/\1/p' web/package.json)
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  echo "@playwright/test is not pinned to an exact version in web/package.json" >&2
  exit 1
}
tag="mcr.microsoft.com/playwright:v${version}-noble"
# The first "Digest:" line is the multi-arch index (or the single manifest).
digest=$(docker buildx imagetools inspect "$tag" |
  sed -n 's/^Digest:[[:space:]]*\(sha256:[0-9a-f]\{64\}\).*/\1/p' | head -n 1)
[[ -n $digest ]] || { echo "no digest found for $tag" >&2; exit 1; }
sed -i "s|^FROM mcr.microsoft.com/playwright:.*|FROM ${tag}@${digest}|" tests/e2e/ui.Dockerfile
echo "pinned ${tag}@${digest}"
