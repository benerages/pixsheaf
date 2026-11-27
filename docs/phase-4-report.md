# Pixsheaf Phase 4: Batch mode

Phase 4 is complete. A job can hold many files, uploaded one by one, from folders, or as ZIP archives; the archives are unpacked in the worker sandbox (ADR 0022). The result is a streamed ZIP archive with `report.json`. Files of different clients are processed in turn, and each client has job and request limits. All local checks, the two end-to-end tests, and the load test passed on commit 004437d plus the load test files. The web UI follows in Phase 5.

## 1. What is in place

| Area | State |
|---|---|
| Jobs with many files | `expectedFiles` up to `PIXSHEAF_MAX_FILES`; each upload is processed as soon as it arrives; a broken file fails alone |
| ZIP uploads | New job protocol operation `unzip` in the job process: two passes (count, then stream), limits for entries, entry size, total size, and a compression ratio of 100; symbolic links, encrypted entries, nested archives (ZIP, gzip, 7-Zip, RAR, bzip2, xz, zstd), corrupt entries, and non-images fail individually; the API validates the listing and sniffs every entry again |
| Folder structure | Relative paths from folder uploads and archives are sanitized per segment and kept in the ZIP download on request (`naming.keepFolders`) |
| Download | Streamed ZIP (`archive/zip`, images stored, `report.json` deflated) with per-file status, error, sizes, and saving; one-time, then everything is deleted |
| Naming | Patterns with `{name}`, `{index}`, `{width}`, `{height}`, `{date}`, `{ext}`; the output extension is always appended; duplicates get ` (2)`, compared case-insensitively |
| Fair queue | One FIFO per client, served round-robin; global limit `PIXSHEAF_MAX_CONCURRENCY` |
| Client limits | Client = peer address, or the rightmost untrusted `X-Forwarded-For` entry behind `PIXSHEAF_TRUSTED_PROXIES`; IPv6 per /64; stored only as an HMAC with an in-memory key; `PIXSHEAF_MAX_JOBS_PER_CLIENT` and `PIXSHEAF_REQUESTS_PER_MINUTE` (token bucket) with `429` and `Retry-After` |
| Presets | `GET /presets` with Web, Thumbnail, Social Media, Archive (validated at start); custom presets stay with the client as the same JSON document, so UI and API give identical results |
| Progress and cancellation | Server-Sent Events per job (from Phase 3), cancel one file or delete the whole job |
| API | OpenAPI extended first (presets, naming, limits, file kinds, report schema, archive error codes); `maxFrames` now in `GET /capabilities` |

## 2. Tests

| Kind | Coverage |
|---|---|
| Unit | Fair queue, client job limit, naming and collisions, archive listing validation, archive scan and write (links, encryption, unknown methods, CRC errors, bombs, nested archives, limits, path cleaning), client identity behind proxies, rate limiter, ZIP download with report, presets accepted by `POST /jobs`, unzip in a real job process |
| Fuzzing | New target `FuzzScan` for the archive reader (1.4 million runs without finding); 8 targets in total |
| End to end | `tests/e2e/batch.sh`, also in the container workflow: two images and one archive (image in a folder, text file, nested archive), naming with folders, ZIP download with report, second download 404 |
| Load | `tests/load/run.sh` with k6 2.4.0 (see §3) |

## 3. Load test

`bash tests/load/run.sh` with the defaults: 4 batches of 50 files, 2 batches at a time, one worker with 2 slots, default limits except the per-client limits (one k6 client sends all batches). Each file is a 3000 × 2000 PNG (18 MB of pixels); every tenth file is broken. Pipeline: auto-orient, resize to 1600 px, text watermark, WebP.

| Result | Value |
|---|---|
| Checks | 20 of 20 passed; no failed HTTP request |
| Files | 180 processed, 20 failed with `decode_failed` (exactly the broken ones) |
| Batch duration | 5.0 to 9.0 s, average 7.5 s |
| Peak memory, API | 38 MiB of 768 MiB (4 %) |
| Peak memory, worker | 114 MiB of 1024 MiB (11 %) |
| OOM kills | 0 |

The test PNG is a gradient, so it uploads quickly; decoding still allocates the full 18 MB of pixels per file. The run shows that the defaults leave ample headroom at this size; larger photos, more slots, and several workers (`WORKERS=2 VUS=4 BATCHES=8`) are a Phase 6 measurement. The load test is not part of CI yet because it takes minutes and depends on the host's cgroup layout.

## 4. Issues found and fixed

| Issue | Fix |
|---|---|
| The job protocol header was limited to 64 KB, but a pipeline with an image watermark carries up to 1.4 MB of base64: such jobs would have failed in the worker (since Phase 3) | Limit raised to 4 MiB, with a test at the hard watermark limit |
| `multipart.Part.FileName` drops folders | The file name is read from `Content-Disposition` and sanitized per segment |
| `report.json` types were not generated because no operation references them | `skip-prune` in `api/oapi-codegen.yaml` |

## 5. Known gaps

| Gap | Phase |
|---|---|
| Downloading single files of a batch (the requirement allows ZIP or individual files); the API has only the ZIP download so far | 5, together with the UI that needs it |
| Live preview on the first or a selected image of a batch (UI; the API can run a one-file job with the same pipeline) | 5 |
| Load test with real photos, several workers, a full queue, and in CI | 6 |
| Coverage gaps from Phase 3 (child processes, container code, libvips code) | 6 |
| Behind a reverse proxy that is not in `PIXSHEAF_TRUSTED_PROXIES`, all clients share one set of limits (documented in `docs/deployment.md`) | Operator |

## 6. Next: Phase 5

The web UI: the start page is the single-image workflow (upload, choose an action or preset, before-and-after preview, download); selecting several files, a folder, or a ZIP archive switches to batch mode with a file list, per-file status over Server-Sent Events, and the ZIP download; English and German, light and dark following the system, WCAG 2.2 AA, Playwright and axe-core tests.
