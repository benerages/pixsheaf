# ADR 0019: Job access tokens and upload flow

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27 (job access model)
- Amends: [ADR 0012](0012-api-resource-naming.md)

## Context

Jobs are anonymous (ADR 0015). Job IDs appear in URLs, so they can end up in browser history, proxy logs, or screenshots. The project requires CSRF protection with a same-origin check plus a token. The web UI needs Server-Sent Events and plain download links, and neither `EventSource` nor a navigation can send an `Authorization` header. ADR 0012 named the resources but left open how uploads end and how the files endpoint behaves.

## Decision

### Job access (owner decision)

- `POST /api/v1/jobs` returns the job and a secret **job token** (256 bits from `crypto/rand`, base64url). Job and file IDs have 128 bits.
- Every request for a job needs the token. It is accepted:
  - as `Authorization: Bearer <token>` for all requests, and **only** this way for state-changing requests (`POST`, `DELETE`);
  - as the `pixsheaf_job` cookie only for `GET /jobs/{jobId}/events` and `GET /jobs/{jobId}/download`. The server sets the cookie on job creation with `HttpOnly; SameSite=Strict; Path=/api/v1/jobs/{jobId}; Max-Age=<ttl>`, plus `Secure` over TLS.
- The server stores only a hash of the token and compares in constant time.
- State-changing requests are rejected with 403 when `Sec-Fetch-Site` is `cross-site` or `same-site`, or when `Origin` is present and differs from the server origin. `POST /jobs` additionally requires `Content-Type: application/json`, which a cross-site form cannot send without a CORS preflight, and CORS stays off.

A leaked job URL therefore grants nothing, and the token doubles as the CSRF token, because a cross-site page can neither read it nor make the browser send it in a header.

### Upload flow (amends ADR 0012)

- `POST /jobs` takes the pipeline and `expectedFiles`. The job accepts exactly that many uploads and then closes on its own; no separate "start" request is needed. For the main use case, one image, the client makes three requests: create, upload, download (plus optional events).
- Files are uploaded with **`POST /jobs/{jobId}/files`**, one multipart request per file, instead of the `PUT` named in ADR 0012: every request creates a new file resource, which is `POST` semantics, and parallel uploads are independent. A ZIP upload (Phase 4) counts as one upload that expands into several files.
- Each file is queued as soon as it arrives. `GET /jobs/{jobId}/download` is available when the job is `done` and works once; then the job is deleted.
- `GET /capabilities` exposes formats and operator limits for client-side checks.
- Errors use RFC 9457 problem details with a stable `code`.

### Pipeline format details (refines ADR 0009)

- The output format is set by a `convert` step, so it can be conditional (for example only for HEIC sources). Without one, the source format is kept, and HEIC/HEIF becomes JPEG.
- Metadata and color handling are top-level pipeline options (`metadata`, `colorSpace`), because they apply once to the output, not as steps.
- JSON property names use camelCase throughout (`widthGt`, `sourceFormat`); the snake_case names in ADR 0009 were illustrative.
- Mutually exclusive parameters (for example `width` versus `percent`) are documented in the specification and enforced by the server's validator, not with `oneOf` constructs, so the generated code stays simple.

## Consequences

- API clients and the CLI (Phase "Could") must keep the token from the create response; losing it means the job cannot be accessed and simply expires.
- Adding authentication later (ADR 0015) stays a middleware in front of this model; the job token remains a capability for the job itself.
- Tests must cover: missing, wrong, and cookie-only tokens on state-changing requests; cross-site `Sec-Fetch-Site` and foreign `Origin`; uploads beyond `expectedFiles` (409); a second download (404).
