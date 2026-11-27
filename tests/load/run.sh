#!/usr/bin/env bash
# Load test for batch mode:
#   bash tests/load/run.sh               default: 4 batches of 50 files, 2 at a time
#   FILES=50 BATCHES=8 VUS=4 WORKERS=2 bash tests/load/run.sh
# Brings up the stack (compose.yaml plus compose.load.yaml), runs k6 in a
# container, and checks afterwards that no container came close to its
# memory limit or was OOM-killed. Needs docker, python3, and cgroup v2.
set -Eeuo pipefail
trap 'echo "FAIL: line $LINENO: $BASH_COMMAND" >&2' ERR
cd "$(dirname "$0")/../.."

# k6 v2.4.0 (verified 2026-11-27), pinned by digest.
k6_image="grafana/k6:2.4.0@sha256:d41bb79cd8de5a7ad619b53570e3712633e8bff0937010a5c6c57a4747e7a276"
[[ $k6_image != *K6_DIGEST* ]] || { echo "pin the k6 image digest in tests/load/run.sh first" >&2; exit 2; }

files=(-f compose.yaml -f tests/load/compose.load.yaml)
data=$(mktemp -d)
cleanup() {
  docker compose "${files[@]}" down --volumes >/dev/null 2>&1 || true
  rm -rf "$data"
}
trap cleanup EXIT

# A 3000 x 2000 gradient PNG: small to upload, 18 MB of pixels to decode.
python3 - "$data/large.png" <<'PY'
import struct, sys, zlib
w, h = 3000, 2000
row = bytes(x * 255 // w for x in range(w) for _ in range(3))
raw = b"".join(b"\x00" + row for _ in range(h))
def chunk(t, d):
    return struct.pack(">I", len(d)) + t + d + struct.pack(">I", zlib.crc32(t + d) & 0xffffffff)
png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) \
    + chunk(b"IDAT", zlib.compress(raw, 6)) + chunk(b"IEND", b"")
open(sys.argv[1], "wb").write(png)
PY
chmod -R a+rX "$data"

docker compose "${files[@]}" up -d --build --wait --scale "worker=${WORKERS:-1}"

k6_status=0
docker run --rm --network host --user "$(id -u):$(id -g)" \
  -e BASE=http://127.0.0.1:8080 -e FILES="${FILES:-50}" -e BATCHES="${BATCHES:-4}" -e VUS="${VUS:-2}" \
  -v "$PWD/tests:/tests:ro" -v "$data:/data:ro" \
  "$k6_image" run --quiet /tests/load/batch.js || k6_status=$?

# Peak memory of every container against its limit (cgroup v2).
status=$k6_status
for id in $(docker compose "${files[@]}" ps -q); do
  name=$(docker inspect -f '{{.Name}}' "$id")
  full=$(docker inspect -f '{{.Id}}' "$id")
  dir=""
  for d in "/sys/fs/cgroup/system.slice/docker-$full.scope" "/sys/fs/cgroup/docker/$full"; do
    [[ -r $d/memory.peak ]] && dir=$d && break
  done
  if [[ -z $dir ]]; then
    echo "WARN $name: no readable cgroup v2 memory.peak; memory not checked"
    continue
  fi
  peak=$(<"$dir/memory.peak")
  limit=$(<"$dir/memory.max")
  ooms=$(awk '$1 == "oom_kill" {print $2}' "$dir/memory.events")
  pct=$((100 * peak / limit))
  echo "     $name: peak $((peak >> 20)) MiB of $((limit >> 20)) MiB (${pct}%), OOM kills: ${ooms:-0}"
  if ((pct >= 90)) || ((${ooms:-0} > 0)); then
    echo "FAIL $name came within 10 % of its memory limit or was OOM-killed" >&2
    status=1
  fi
done
exit "$status"
