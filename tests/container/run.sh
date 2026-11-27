#!/usr/bin/env bash
# Builds both images and runs the worker isolation self-check
# (container tests; see docs/security.md).
set -uo pipefail
cd "$(dirname "$0")/../.."

files=(-f compose.yaml -f tests/container/compose.test.yaml)
if [[ "${1:-}" == "gvisor" ]]; then
  files+=(-f deploy/compose.gvisor.yaml)
fi

cleanup() { docker compose "${files[@]}" down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker compose "${files[@]}" up --build --abort-on-container-exit --exit-code-from worker
rc=$?

# The API image must also be read-only, non-root, and without capabilities.
api_user=$(docker compose "${files[@]}" ps -a --format '{{.Name}}' api | head -n1)
if [[ -n "$api_user" ]]; then
  docker inspect --format 'api: User={{.Config.User}} ReadonlyRootfs={{.HostConfig.ReadonlyRootfs}} CapDrop={{.HostConfig.CapDrop}} SecurityOpt={{.HostConfig.SecurityOpt}}' "$api_user"
  docker inspect --format '{{.Config.User}} {{.HostConfig.ReadonlyRootfs}} {{.HostConfig.CapDrop}}' "$api_user" \
    | grep -q '^10001:10001 true \[ALL\]$' || { echo "api container hardening check failed"; rc=1; }
fi
exit $rc
