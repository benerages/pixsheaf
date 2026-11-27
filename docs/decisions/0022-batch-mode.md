# ADR 0022: Batch mode: archives in the sandbox, fair queue, naming, presets

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27 (archives are unpacked in the worker sandbox)
- Amends: [ADR 0019](0019-job-access-and-upload-flow.md)

## Context

Phase 4 adds jobs with many files on the job model of ADR 0019: folder and ZIP uploads, a streamed ZIP download with a report, rename patterns, presets, and a queue that serves several clients fairly. The project rule is that only workers parse file contents, and a ZIP archive is file content. Clients are anonymous (ADR 0015), so fairness and per-client limits need a client identity without accounts.

## Decisions

### ZIP archives are unpacked in the worker sandbox (owner decision)

The alternative was `archive/zip` in the API process. It is memory-safe Go, and the main risk, archive bombs, is handled by limits either way. The owner chose the sandbox so that the rule holds without exception.

- An uploaded archive is a file of kind `archive`. The API sniffs it like an image (the ZIP signature) and queues it as a task with the new job protocol operation `unzip`.
- The job process reads the archive from its input and checks every entry twice: a first pass decompresses each entry into a counter to learn and check its real size; a second pass writes the entries to the output. The response header lists the entries (sanitized name, size, or a reason why an entry was skipped) before any entry data, and memory stays bounded by one entry at a time.
- Limits: at most `maxArchiveEntries` entries and `maxArchiveBytes` of archive; each entry at most `maxUploadBytes`; all entries together at most the job's remaining `maxJobBytes`; a compression ratio of at most 100 per entry (real decompressed size against the stored compressed size). Header sizes are never trusted; the real sizes count.
- Directories are skipped. Symbolic links (by mode), encrypted entries, and nested ZIP archives are reported as `archive_entry_unsupported`; entries above a size or ratio limit as `too_large`. Each becomes a failed file, so it appears in the report. Too many entries for the job fail the archive with `too_many_files`; an unreadable archive with `archive_invalid`.
- Entry names are never used as paths. The job process cleans them (`path.Clean`, no absolute paths, no `..`); the API sanitizes every segment again and uses the result only for display and for the folder structure inside the ZIP download. Zip Slip therefore cannot reach a file system.
- The API stores each entry as a new blob, sniffs it, and queues the images. Archive entries go through exactly the same image path as single uploads.

### Client identity and fairness

- A client is identified by the peer IP address of the connection. Only if that address is in `PIXSHEAF_TRUSTED_PROXIES` is `X-Forwarded-For` read, taking the rightmost address that is not a trusted proxy. IPv6 addresses are reduced to their /64 prefix, because one host usually controls a whole /64.
- The key is an HMAC of the address with a random key created at start, so the address itself is kept nowhere and never logged.
- The queue keeps one FIFO per client and hands out files round-robin across clients: a client with 1000 files and a client with one file alternate. The global limit of parallel files stays `PIXSHEAF_MAX_CONCURRENCY`.
- Per client: at most `PIXSHEAF_MAX_JOBS_PER_CLIENT` open jobs and `PIXSHEAF_REQUESTS_PER_MINUTE` API requests (token bucket with a burst of the same size); beyond that `429` with `Retry-After`. Queue and storage limits stay global (`503`).

### Naming

- `naming.pattern` with the placeholders `{name}`, `{index}`, `{width}`, `{height}`, `{date}`, `{ext}` builds the name without extension; the extension of the output format is always appended, so a pattern cannot produce a misleading type. Unknown placeholders and stray braces are rejected at job creation.
- The result is sanitized like any output name (ADR 0019); duplicates within a job get ` (2)`, ` (3)`, and so on, case-insensitively, because the download may be unpacked on a case-insensitive file system.
- `naming.keepFolders` keeps the sanitized folder path of folder uploads and archive entries inside the ZIP download.

### Presets

- The server keeps no presets, in line with "no persistent storage". `GET /presets` returns the bundled presets (Web, Thumbnail, Social Media, Archive) as `Preset` documents. Custom presets live in the client and are exported and imported as the same JSON document.
- A preset is applied by sending its pipeline and naming unchanged in `POST /jobs`. There is no server-side preset reference, so a preset gives byte-identical results in the UI and through the API by construction.

## Consequences

- One more job protocol operation and one more task kind in the job manager; the worker needs no changes beyond the new operation.
- An archive is decompressed twice in the job process; this costs CPU, not memory, and is bounded by the limits above.
- Behind a reverse proxy that is not listed in `PIXSHEAF_TRUSTED_PROXIES`, all clients share one identity and therefore one set of limits; `docs/deployment.md` must say so.
- Clients behind one NAT share limits. The defaults are set for a small self-hosted installation.
