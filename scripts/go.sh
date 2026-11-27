#!/usr/bin/env bash
# Runs a go command in the pinned Go image used for the API build, so no
# local Go installation is needed. Examples:
#   scripts/go.sh get github.com/example/module@v1.2.3
#   scripts/go.sh mod tidy
#   scripts/go.sh test ./...
# The worker's libvips code (build tag "vips") is tested in the worker image:
#   docker build -f deploy/docker/worker.Dockerfile --target test .
set -Eeuo pipefail
cd "$(dirname "$0")/.."
go_image=$(sed -n 's/^ARG GO_IMAGE=//p' deploy/docker/api.Dockerfile)
[[ -n $go_image ]] || { echo "GO_IMAGE not found in api.Dockerfile" >&2; exit 1; }
exec docker run --rm --user "$(id -u):$(id -g)" \
  -e HOME=/tmp -e GOTOOLCHAIN=local -e GOFLAGS=-mod=mod \
  -v "$PWD:/src" -w /src "$go_image" go "$@"
