# API

The HTTP API is specified in [`api/openapi.yaml`](../api/openapi.yaml) (OpenAPI 3.1). The specification is the source of truth: Go types (`internal/api`) and TypeScript types (`web/src/lib/api`) are generated from it with `scripts/generate-api.sh`, and CI fails when they are out of date. A rendered reference will be generated from the specification for the 1.0 release.

## Glossary

| Term | Meaning |
|---|---|
| Job | One request to process one or more files with one pipeline. One file: the download is the image. Several files: the download is a ZIP archive. |
| File (task) | One uploaded image inside a job, processed on its own in an isolated worker process. |
| Pipeline | The ordered steps applied to every file, plus metadata and color options (ADR 0009, ADR 0019). |
| Job token | Secret returned when a job is created; required for every request about that job (ADR 0019). |

## Processing one image

```sh
# 1. Create a job for one file: resize to 1600 px on the longest edge, save as WebP.
curl -s http://127.0.0.1:8080/api/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{"expectedFiles":1,"pipeline":{"version":1,"steps":[
        {"op":"autoOrient"},
        {"op":"resize","params":{"longestEdge":1600}},
        {"op":"convert","params":{"format":"webp","quality":80}}]}}'
# -> {"job":{"id":"<jobId>",...},"token":"<token>"}

# 2. Upload the file.
curl -s http://127.0.0.1:8080/api/v1/jobs/<jobId>/files \
  -H 'Authorization: Bearer <token>' -F file=@photo.heic

# 3. Download the result once the job is done (poll GET /jobs/<jobId> or follow /events).
curl -s -OJ http://127.0.0.1:8080/api/v1/jobs/<jobId>/download \
  -H 'Authorization: Bearer <token>'
```

The endpoints are implemented in Phase 3; this page describes the agreed contract.
