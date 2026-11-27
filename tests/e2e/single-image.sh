#!/usr/bin/env bash
# End-to-end test of the single-image path against a running stack:
#   docker compose up -d --build && bash tests/e2e/single-image.sh
# Needs curl and jq. PIXSHEAF_URL overrides the address.
set -Eeuo pipefail
trap 'echo "FAIL: line $LINENO: $BASH_COMMAND" >&2' ERR

base=${PIXSHEAF_URL:-http://127.0.0.1:8080}
api=$base/api/v1
fixtures=$(cd "$(dirname "$0")/fixtures" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok   $*"; }

# create PIPELINE -> sets id and token
create() {
  local body
  body=$(curl -fsS -H 'Content-Type: application/json' \
    -d "{\"pipeline\":$1,\"expectedFiles\":1}" "$api/jobs")
  id=$(jq -r .job.id <<<"$body")
  token=$(jq -r .token <<<"$body")
}

# wait_done -> prints the job when it is done
wait_done() {
  local job status
  for _ in $(seq 1 120); do
    job=$(curl -fsS -H "Authorization: Bearer $token" "$api/jobs/$id")
    status=$(jq -r .status <<<"$job")
    if [[ $status == done ]]; then
      printf '%s' "$job"
      return 0
    fi
    sleep 0.5
  done
  fail "job $id not done after 60 s (status $status)"
}

http_code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }

# --- capabilities
jq -e '.pipelineVersion == 1 and (.outputFormats | index("avif"))' \
  <<<"$(curl -fsS "$api/capabilities")" >/dev/null || fail "capabilities"
pass "capabilities"

# --- convert and resize one image
create '{"version":1,"steps":[{"op":"resize","params":{"width":32}},{"op":"convert","params":{"format":"webp","quality":70}}]}'
curl -fsS -H "Authorization: Bearer $token" \
  -F "file=@$fixtures/gradient.png;filename=Holiday photo.png" "$api/jobs/$id/files" >/dev/null
job=$(wait_done)
jq -e '.files[0] | .status == "done" and .output.format == "webp" and .output.width == 32 and .output.height == 24 and .output.name == "Holiday photo.webp"' \
  <<<"$job" >/dev/null || fail "unexpected job: $job"
pass "processed: $(jq -c '.files[0].output' <<<"$job")"

curl -fsS -H "Authorization: Bearer $token" -D "$tmp/headers" -o "$tmp/out" "$api/jobs/$id/download"
[[ $(head -c 4 "$tmp/out") == RIFF && $(dd if="$tmp/out" bs=1 skip=8 count=4 status=none) == WEBP ]] \
  || fail "download is not WebP"
grep -qi '^content-disposition: attachment; filename="Holiday photo.webp"' "$tmp/headers" \
  || fail "content-disposition: $(grep -i content-disposition "$tmp/headers")"
pass "downloaded $(wc -c <"$tmp/out") bytes of WebP"

[[ $(http_code -H "Authorization: Bearer $token" "$api/jobs/$id/download") == 404 ]] \
  || fail "second download did not return 404"
pass "second download: 404 (deleted)"

# --- a broken file fails alone and cannot be downloaded
create '{"version":1,"steps":[]}'
curl -fsS -H "Authorization: Bearer $token" -F "file=@$fixtures/broken.png" "$api/jobs/$id/files" >/dev/null
job=$(wait_done)
jq -e '.files[0].status == "failed" and .files[0].error.code == "decode_failed"' <<<"$job" >/dev/null \
  || fail "broken file: $job"
[[ $(http_code -H "Authorization: Bearer $token" "$api/jobs/$id/download") == 409 ]] \
  || fail "download of a failed file did not return 409"
pass "broken file: failed with decode_failed, download 409"

# --- a file that is not an image is rejected at upload
create '{"version":1,"steps":[]}'
printf '<svg xmlns="http://www.w3.org/2000/svg"/>' >"$tmp/x.svg"
[[ $(http_code -H "Authorization: Bearer $token" -F "file=@$tmp/x.svg" "$api/jobs/$id/files") == 415 ]] \
  || fail "SVG upload was not rejected with 415"
pass "SVG upload: 415"

# --- access control
[[ $(http_code "$api/jobs/$id") == 401 ]] || fail "job without token"
[[ $(http_code -H "Authorization: Bearer wrong" "$api/jobs/$id") == 401 ]] || fail "job with wrong token"
[[ $(http_code -H 'Content-Type: application/json' -H 'Origin: https://evil.example' \
  -d '{"pipeline":{"version":1,"steps":[]},"expectedFiles":1}' "$api/jobs") == 403 ]] || fail "cross-origin create"
[[ $(http_code -X DELETE -H "Authorization: Bearer $token" "$api/jobs/$id") == 204 ]] || fail "delete"
[[ $(http_code -H "Authorization: Bearer $token" "$api/jobs/$id") == 404 ]] || fail "deleted job still there"
pass "access control: 401 without or with wrong token, 403 cross-origin, delete 204 then 404"

echo "all end-to-end checks passed"
