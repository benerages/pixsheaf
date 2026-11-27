#!/usr/bin/env bash
# Runs the ADR 0007 spike for both volume variants and reports the results.
# Usage: ./run.sh            (default runtime, runc)
#        ./run.sh gvisor     (with compose.gvisor.yaml, needs runsc)
set -uo pipefail
cd "$(dirname "$0")"

files=(-f compose.yaml)
label="runc"
if [[ "${1:-}" == "gvisor" ]]; then
  if ! docker info --format '{{json .Runtimes}}' | grep -q '"runsc"'; then
    echo "== SKIPPED (gvisor): runsc is not registered as a Docker runtime on this host"
    exit 0
  fi
  files+=(-f compose.gvisor.yaml)
  label="gvisor"
fi

rootless=no; userns=no
sec=$(docker info --format '{{.SecurityOptions}}'); grep -q rootless <<<"$sec" && rootless=yes; grep -q userns <<<"$sec" && userns=yes
echo "== Docker: $(docker version --format '{{.Server.Version}}') | rootless: ${rootless} | userns-remap: ${userns} | runtime: ${label} | kernel: $(uname -r)"

overall=0
for variant in sock_tmpfs sock_image; do
  echo
  echo "== Variant: ${variant}"
  export SPIKE_VOLUME="${variant}"
  docker compose "${files[@]}" down -v --remove-orphans >/dev/null 2>&1
  docker compose "${files[@]}" up --build --abort-on-container-exit --exit-code-from worker
  rc=$?
  docker compose "${files[@]}" logs --no-color server 2>/dev/null | grep -E 'server: (uid|/run|mount)|listen:' || true
  docker compose "${files[@]}" down -v --remove-orphans >/dev/null 2>&1
  if [[ $rc -eq 0 ]]; then
    echo "== RESULT (${label}, ${variant}): PASS"
  else
    echo "== RESULT (${label}, ${variant}): FAIL (exit ${rc})"
    overall=1
  fi
done
exit $overall
