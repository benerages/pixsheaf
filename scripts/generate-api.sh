#!/usr/bin/env bash
# Regenerates code from api/openapi.yaml (ADR 0011) and lints the
# specification, inside containers so that no local Go or Node.js is needed:
#   internal/api/api.gen.go       Go models (oapi-codegen)
#   web/src/lib/api/schema.d.ts   TypeScript types (openapi-typescript)
#   docs/api.md                   API reference (web/scripts/api-docs.mjs)
# CI runs the same generators and fails if the committed output differs.
# Requires Docker and internet access.
set -Eeuo pipefail
trap 'echo "generate-api: failed at line $LINENO" >&2' ERR
cd "$(dirname "$0")/.."

# Same toolchain images as the API build.
go_image=$(sed -n 's/^ARG GO_IMAGE=//p' deploy/docker/api.Dockerfile)
node_image=$(sed -n 's/^ARG NODE_IMAGE=//p' deploy/docker/api.Dockerfile)
[[ -n $go_image && -n $node_image ]] || { echo "toolchain images not found in api.Dockerfile" >&2; exit 1; }

# Keep the literal tool@vX.Y.Z and pnpm@X.Y.Z below: Renovate updates them.
docker run --rm --user "$(id -u):$(id -g)" \
  -e HOME=/tmp -e GOTOOLCHAIN=local -e GOFLAGS=-mod=mod \
  -v "$PWD:/src" -w /src "$go_image" sh -euc '
    go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2 \
      -config api/oapi-codegen.yaml api/openapi.yaml
    go mod tidy
  '
echo "generated internal/api/api.gen.go"

docker run --rm --user "$(id -u):$(id -g)" \
  -e HOME=/tmp -e npm_config_ignore_scripts=true -e npm_config_prefix=/tmp/npm-global \
  -v "$PWD:/src" -w /src/web "$node_image" sh -euc '
    npm install --global pnpm@12.23.0 > /dev/null
    pnpm=/tmp/npm-global/bin/pnpm
    $pnpm install --frozen-lockfile --store-dir /tmp/pnpm-store > /dev/null
    $pnpm run --silent gen:api
    $pnpm run --silent gen:docs > /dev/null
    $pnpm run --silent lint:api
  '
echo "generated web/src/lib/api/schema.d.ts and docs/api.md; specification lint passed"

git --no-pager diff --stat -- internal/api web/src/lib/api docs/api.md go.mod go.sum
