# ADR 0012: API resource naming

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Context

`POST /api/v1/batches` also serves single images, which contradicts the rule that the product leads with the single-image workflow.

## Options

`/jobs`; `/tasks`; `/sets`; keep `/batches`.

## Decision

Rename to `/api/v1/jobs`: `POST /jobs` creates a job, `PUT /jobs/{id}/files` uploads files, `GET /jobs/{id}/events` streams progress over SSE, `GET /jobs/{id}/download` returns a single file when there is one file and a ZIP otherwise, and `DELETE /jobs/{id}` cancels. Internally each file is a "task" (`/jobs/{id}/files/{fileId}` for per-file status and cancellation). Defer any one-shot convenience endpoint (`POST /api/v1/process`) to a later ADR, and allow it only if it reuses the same validation and worker path.

## Consequences

The names are neutral, and the UI never needs the word "batch". "Job" now means the whole set and "task" means one file, so the glossary in `docs/api.md` must state this explicitly.

## Amendment 2026-11-27

[ADR 0019](0019-job-access-and-upload-flow.md) replaces `PUT /jobs/{id}/files` with `POST /jobs/{jobId}/files` (one file per request), adds `expectedFiles` to job creation so that a job closes after its uploads, and defines the job token required for every job request.
