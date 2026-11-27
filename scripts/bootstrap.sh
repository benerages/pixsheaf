#!/usr/bin/env bash
# One-time bootstrap: creates web/pnpm-lock.yaml inside a container, so no
# local Node.js installation is needed. Commit the resulting lockfile.
# Requires Docker and internet access.
set -euo pipefail
cd "$(dirname "$0")/.."

NODE_IMAGE="${NODE_IMAGE:-node:26.13.1-trixie-slim}"

docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp -e npm_config_ignore_scripts=true \
  -v "$PWD/web:/work" -w /work \
  "$NODE_IMAGE" \
  sh -c 'npx --yes pnpm@12.23.0 install --lockfile-only'

echo "Created web/pnpm-lock.yaml. Review and commit it:"
echo "  git add web/pnpm-lock.yaml && git commit -m 'build: add pnpm lockfile'"
