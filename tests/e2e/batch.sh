#!/usr/bin/env bash
# End-to-end test of batch mode against a running stack:
#   docker compose up -d --build --wait && bash tests/e2e/batch.sh
# Needs curl, jq, and python3 (to build and read ZIP archives).
set -Eeuo pipefail
trap 'echo "FAIL: line $LINENO: $BASH_COMMAND" >&2' ERR

base=${PIXSHEAF_URL:-http://127.0.0.1:8080}
api=$base/api/v1
fixtures=$(cd "$(dirname "$0")/fixtures" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok   $*"; }

# --- bundled presets
[[ $(curl -fsS "$api/presets" | jq -r '[.presets[].id] | join(",")') == "web,thumbnail,social,archive" ]] \
  || fail "presets"
pass "presets: web, thumbnail, social, archive"

# --- an archive with an image in a folder, a text file, and a nested archive
python3 - "$fixtures" "$tmp/photos.zip" <<'PY'
import io, sys, zipfile
fixtures, out = sys.argv[1], sys.argv[2]
inner = io.BytesIO()
with zipfile.ZipFile(inner, "w") as z:
    z.writestr("x.png", b"x")
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as z:
    z.write(f"{fixtures}/gradient.png", "inner/c.png")
    z.writestr("notes.txt", "not an image")
    z.writestr("nested.zip", inner.getvalue())
    z.writestr("empty-folder/", "")
PY

body=$(curl -fsS -H 'Content-Type: application/json' -d '{
  "pipeline": {"version": 1, "steps": [
    {"op": "resize", "params": {"width": 16}},
    {"op": "convert", "params": {"format": "png"}}]},
  "expectedFiles": 3,
  "naming": {"pattern": "{index}-{name}-{width}x{height}", "keepFolders": true}}' "$api/jobs")
id=$(jq -r .job.id <<<"$body")
token=$(jq -r .token <<<"$body")
auth=(-H "Authorization: Bearer $token")

curl -fsS "${auth[@]}" -F "file=@$fixtures/gradient.png;filename=trip/day 1/a.png" "$api/jobs/$id/files" >/dev/null
curl -fsS "${auth[@]}" -F "file=@$fixtures/broken.png;filename=b.png" "$api/jobs/$id/files" >/dev/null
curl -fsS "${auth[@]}" -F "file=@$tmp/photos.zip;filename=photos.zip" "$api/jobs/$id/files" >/dev/null
[[ $(curl -s -o /dev/null -w '%{http_code}' "${auth[@]}" -F "file=@$fixtures/gradient.png" "$api/jobs/$id/files") == 409 ]] \
  || fail "a fourth upload was accepted"
pass "uploaded 2 images and 1 archive; a fourth upload: 409"

for _ in $(seq 1 120); do
  job=$(curl -fsS "${auth[@]}" "$api/jobs/$id")
  [[ $(jq -r .status <<<"$job") == done ]] && break
  sleep 0.5
done
[[ $(jq -r .status <<<"$job") == done ]] || fail "job not done: $job"
summary=$(jq -c '[.files[] | {source, kind, status, code: .error.code}]' <<<"$job")
expected='[{"source":"trip/day 1/a.png","kind":"image","status":"done","code":null},{"source":"b.png","kind":"image","status":"failed","code":"decode_failed"},{"source":"photos.zip","kind":"archive","status":"done","code":null},{"source":"inner/c.png","kind":"image","status":"done","code":null},{"source":"notes.txt","kind":"image","status":"failed","code":"unsupported_format"},{"source":"nested.zip","kind":"image","status":"failed","code":"archive_entry_unsupported"}]'
[[ $summary == "$expected" ]] || fail "files: $summary"
pass "archive unpacked in the sandbox; broken, non-image, and nested entries failed alone"

curl -fsS "${auth[@]}" -D "$tmp/headers" -o "$tmp/result.zip" "$api/jobs/$id/download"
grep -qi '^content-type: application/zip' "$tmp/headers" || fail "download is not a ZIP"
python3 - "$tmp/result.zip" <<'PY'
import json, sys, zipfile
z = zipfile.ZipFile(sys.argv[1])
names = z.namelist()
want = ["trip/day 1/1-a-16x12.png", "inner/3-c-16x12.png", "report.json"]
assert names == want, f"entries {names}, want {want}"
for n in want[:2]:
    assert z.read(n).startswith(b"\x89PNG"), f"{n} is not a PNG"
report = json.loads(z.read("report.json"))
statuses = [(f["source"], f["status"]) for f in report["files"]]
assert len(statuses) == 6 and statuses[1] == ("b.png", "failed"), statuses
assert report["files"][0]["output"] == "trip/day 1/1-a-16x12.png", report["files"][0]
print("ok   ZIP download:", ", ".join(names))
PY

[[ $(curl -s -o /dev/null -w '%{http_code}' "${auth[@]}" "$api/jobs/$id/download") == 404 ]] \
  || fail "second download did not return 404"
pass "second download: 404 (deleted)"

echo "all batch checks passed"
