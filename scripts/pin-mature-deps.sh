#!/usr/bin/env bash
# Re-pins the given web devDependencies to the newest version that already
# satisfies pnpm's minimumReleaseAge (web/pnpm-workspace.yaml, ADR 0013).
# pnpm itself picks the version, so no version number is typed by hand.
#
# Usage: scripts/pin-mature-deps.sh <package@range> [...]
#   e.g. scripts/pin-mature-deps.sh 'svelte@^5' 'vite@^8'
# Requires Docker and internet access.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ $# -gt 0 ]] || { echo "usage: $0 <package@range> [...]" >&2; exit 2; }

NODE_IMAGE="${NODE_IMAGE:-node:26.13.1-trixie-slim}"
# Keep the literal pnpm@X.Y.Z below: Renovate updates it (renovate.json).
PNPM_PKG="pnpm@12.23.0"

docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -e npm_config_ignore_scripts=true \
  -e npm_config_prefix=/tmp/npm-global \
  -v "$PWD/web:/work" -w /work \
  "$NODE_IMAGE" \
  sh -euc 'npm install --global "$0" >/dev/null
           /tmp/npm-global/bin/pnpm add --workspace-root --save-dev --save-exact \
             --store-dir /tmp/pnpm-store "$@"' "$PNPM_PKG" "$@"

echo "Updated web/package.json and web/pnpm-lock.yaml:"
git --no-pager diff --stat -- web/package.json web/pnpm-lock.yaml || true
