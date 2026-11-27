#!/usr/bin/env bash
# One-time bootstrap: creates web/pnpm-lock.yaml inside a container, so no
# local Node.js installation is needed. Commit the resulting lockfile.
# Requires Docker and internet access.
#
# pnpm is installed the same way as in deploy/docker/api.Dockerfile and CI
# (npm install --global), so the lockfile matches what the builds expect.
set -euo pipefail
cd "$(dirname "$0")/.."

NODE_IMAGE="${NODE_IMAGE:-node:26.13.1-trixie-slim}"
# Keep the literal pnpm@X.Y.Z below: Renovate updates it (renovate.json).
PNPM_PKG="pnpm@12.23.0"

rm -f web/pnpm-lock.yaml

docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -e npm_config_ignore_scripts=true \
  -e npm_config_prefix=/tmp/npm-global \
  -v "$PWD/web:/work" -w /work \
  "$NODE_IMAGE" \
  sh -euc "
    npm install --global ${PNPM_PKG}
    /tmp/npm-global/bin/pnpm --version
    /tmp/npm-global/bin/pnpm install --lockfile-only --store-dir /tmp/pnpm-store
  "

# Sanity check: the lockfile must resolve the declared devDependencies.
for dep in svelte vite typescript; do
  if ! grep -q "^  ${dep}@" web/pnpm-lock.yaml; then
    echo "ERROR: web/pnpm-lock.yaml does not contain '${dep}'; resolution failed." >&2
    exit 1
  fi
done

echo "Created web/pnpm-lock.yaml. Review and commit it:"
echo "  git add web/pnpm-lock.yaml && git commit -m 'build: add pnpm lockfile'"
