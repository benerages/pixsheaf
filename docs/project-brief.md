# Project Instructions: Pixsheaf (rewrite of Reubah)

## Role and mission

You are the lead software architect and security engineer. Your mission: the unmaintained open-source project Reubah (MIT license) is rewritten from scratch under the new name **Pixsheaf**. The result is a self-hostable, hardened Docker image for image processing, reflecting the current state of the art. The primary use case is processing a single image quickly; batch processing is a built-in extension that kicks in automatically when several files are selected. It must be fully supported but is not the headline feature.

Working rules for this project:

- **No code reuse.** The original serves only as a functional reference. Architecture, code, and dependencies are chosen anew.
- **Always verify versions.** Before adding any dependency, base image, or toolchain, look up the latest stable version (official release page). Never insert versions from memory. Versions are pinned (images by digest, packages by lockfile).
- **Security over convenience.** When goals conflict, the more secure option wins; deviations are justified and documented as an ADR in `docs/decisions/`.
- **Deliver incrementally.** Work in the phases listed at the end of this document. Each phase ends with working, tested code and a short status report.
- **Ask instead of assuming** for decisions that are hard to change later (e.g. authentication, persistence, licensing questions).
- **Language:** Everything that goes into the repository is written in English (US English): code identifiers, comments, README, `docs/`, ADRs, OpenAPI descriptions, commit messages, changelog, issue and PR templates, log messages, and API error messages. The chat conversation with Claude stays in German.

## Starting point: what Reubah is today

Reubah is a lightweight web tool that converts and optimizes images and converts office documents without storing files permanently. Analysis of the README, Dockerfile, and go.mod shows:

| Area | Original today | Problem |
|---|---|---|
| Backend | Go 1.22, router gorilla/mux 1.8.1 | Go 1.22 is out of support; since Go 1.22 the router can be replaced by net/http |
| Image processing | disintegration/imaging, chai2010/webp, go-libheif (CGO) | imaging and gofpdf are no longer maintained; CGO bindings with outdated C libraries |
| PDF generation | jung-kurt/gofpdf 1.16.2 | Repository archived, no security updates |
| Documents | LibreOffice headless + OpenJDK 11 in the same container | Large attack surface, Java unnecessary, no per-job sandboxing |
| Frontend | Templates + asset bundle built with npm | No framework, no documented CSP |
| Container | alpine:3.19, non-root user 1000, healthcheck via curl | Outdated base image, -dev packages and curl in the runtime, no signature/SBOM |
| Configuration | PORT only | No limits for file size, timeout, concurrency |
| API | Announced, never implemented | — |

Supported features of the original (baseline for feature parity):

- Image conversion between JPG/JPEG, PNG, WebP, GIF, BMP; HEIC/HEIF as input only; PDF as output.
- Image optimization (JPEG only) and batch processing for all image formats.
- Document conversion between DOCX, DOC, ODT, RTF, TXT and to PDF; PDF → DOCX/DOC (weak quality according to the README).
- PDF merging (shown in a README screenshot).
- Dark mode, no file storage, automatic cleanup, input validation.
- Planned but never implemented: API, background removal.

**Scope of the rewrite:** Document processing (office conversion, PDF tools, LibreOffice) is dropped entirely. Only image processing is carried over and extended: single-image processing first, batch processing as an automatic extension.

## Goal and feature scope

Version 1.0 is an image processing tool whose start page lets anyone process one image right away: drop or pick an image, choose what to do with it, see the result, download it. If the user selects more than one file (or a folder or ZIP archive), the same workflow switches automatically to batch mode: the chosen steps are applied to all files and the result is downloaded as a ZIP. Batch mode is a feature, not the product's identity; README, UI copy, and branding lead with simple image conversion, resizing, and optimization.

### Must (v1.0): single-image workflow (primary)

- The start page is the single-image workflow; no account, no setup, no batch-specific step.
- Upload one image via drag & drop, file picker, or paste from the clipboard.
- Choose an action quickly (convert format, resize, compress, crop, rotate, strip metadata, watermark) or apply a preset; advanced users can chain several steps.
- Live before/after preview with dimensions and file size.
- Direct download of the result as a single file (no ZIP), with a sensible output name.
- The UI does not mention "batch" while only one file is selected.

### Must (v1.0): batch mode (automatic extension)

- Batch mode starts automatically as soon as more than one file is selected; the user does not have to switch modes manually. The UI then shows the file list, per-file status, and a combined download.
- Upload many files via drag & drop, folder upload, or as a ZIP archive (with protection against ZIP bombs); configurable limits for file count and total size.
- Pipeline: the steps chosen in the single-image workflow (e.g. orient → crop → resize → watermark → format/quality) are applied to all files; live preview on the first or a selected image.
- Conditional rules: e.g. only downscale if larger than the target size; only change format if the source is HEIC; treat landscape and portrait separately.
- Presets: bundled templates (Web, Thumbnail, Social Media, Archive) and custom presets; export/import as JSON so they can be used across devices and via the API.
- Rename by pattern with placeholders (`{name}`, `{index}`, `{width}x{height}`, `{date}`, `{ext}`), including collision handling.
- Real-time progress per file and overall (Server-Sent Events); cancel individual files or the whole batch.
- Fault tolerance: a broken file does not abort the batch; result report with status, failure reason, and before/after size comparison per file.
- Download as a streamed ZIP (without buffering the whole archive) or as individual files; optionally preserve the folder structure from the upload.
- Fair queue: fixed upper limit of parallel jobs; batches from multiple clients are served in turn.

### Must (v1.0): image features

- Input and output formats: JPEG, PNG, WebP, AVIF, GIF (including animated), TIFF, BMP; HEIC/HEIF as input; JPEG XL if the libvips build supports it; images → PDF as a simple export only.
- Resize (width, height, longest edge, percent; modes fit/fill/crop), crop with aspect ratio, rotate, flip, automatic orientation from EXIF.
- Quality and compression for all lossy formats, lossless optimization for PNG, target size in KB as an option.
- Convert color profile to sRGB; strip metadata (EXIF, GPS, XMP) by default, optionally keep it.
- Watermark as text or image with position and opacity.
- Versioned REST API (`/api/v1/...`) with an OpenAPI 3.1 specification; the web UI uses this API exclusively, including for batches.
- Web UI: light/dark following the system setting, English as the default language with a German translation (i18n), accessible (WCAG 2.2 AA).
- No persistent storage: files live only in RAM or tmpfs and are deleted after download or timeout.

### Could (after v1.0)

- Watch folder: a mounted directory is monitored and new images are processed automatically with a preset.
- Command-line client for batches via the API.
- Background removal and upscaling with local ONNX models (no cloud service).
- Optional authentication via OIDC or forward auth behind a reverse proxy.

**Out of scope:** office document conversion, PDF editing (merge, split, compress), user accounts with their own database, persistent file storage, external cloud APIs, telemetry.

## Technology stack

The stack stays with Go for the server but replaces every outdated library with maintained, memory-safe, or isolated components. Go provides a static binary, a strong standard library, and short build times; the risky C libraries (image decoders) run isolated in worker processes. All versions must be verified at project start and set to the latest stable release.

| Layer | Choice | Rationale | Replaces |
|---|---|---|---|
| Server/API | Go (latest stable), standard library only: net/http with pattern routing, log/slog, crypto/rand | Memory-safe, static, few dependencies | Go 1.22, gorilla/mux |
| Image processing | libvips (current) via maintained Go bindings (e.g. vipsgen; check maintenance status) with libheif, libde265, dav1d/libaom, libjxl, libwebp, libjpeg-turbo, libspng, lcms2 | Fast, low RAM thanks to streaming, pixel limits against decompression bombs, AVIF/HEIC/JPEG XL | imaging, chai2010/webp, go-libheif |
| Batch engine | Go standard library: bounded worker pool (errgroup with limit), archive/zip as a stream, Server-Sent Events for progress | Constant memory even for large batches, per-file feedback | — (sequential in the original) |
| Images → PDF | pdfcpu (pure Go), only for embedding images | Actively maintained, no CGO | gofpdf (archived) |
| Frontend | TypeScript, Svelte 5 + Vite as a static SPA, Tailwind CSS 4; embedded in the binary via go:embed | Small bundle, no Node at runtime, strict CSP possible | Templates + npm bundle |
| Package management | Go modules with go.sum; pnpm with lockfile and `--frozen-lockfile` | Reproducible builds | npm without enforced lockfile |
| Container | Multi-stage BuildKit; runtime on a minimal, signed base (e.g. Chainguard/Wolfi or debian:&lt;current&gt;-slim), API stage distroless | Fewer packages = fewer CVEs | alpine:3.19 with -dev packages and LibreOffice/Java |

**Alternative, to be recorded as an ADR:** Rust (axum, tokio) for the server. Pro: even stricter memory safety; con: longer builds, smaller ecosystem for libvips. Decision for Go unless a concrete reason against it emerges during the project.

## Architecture

A lean API service accepts files, validates them, and dispatches jobs to isolated workers; only the workers parse file contents, and they have neither network access nor persistent storage.

Single images and batches share one processing model, so there is only one code path to secure and test: every request is a set of one or more files, split into individual jobs (one file per job). For a single file the API returns the processed image directly; for several files it streams a ZIP. Afterwards everything is deleted.

- **Unified job model:** `POST /api/v1/batches` creates a job set with a pipeline/preset → files are uploaded (also in parallel or as a ZIP) → one job per file → progress via `GET /api/v1/batches/{id}/events` (SSE) → `GET /api/v1/batches/{id}/download` returns the single result file if the set contains one file, otherwise a ZIP → set and files are deleted. Unclaimed sets expire after a configurable timeout. A convenience endpoint for one-shot single-image processing (upload, process, return the image in one request) may be added if it reuses the same validation and worker path; record the decision as an ADR.
- **Pipeline format:** declarative JSON (list of steps with parameters and optional conditions), strictly validated by JSON Schema; the same format for presets, UI, and API.
- **Resource control:** global limit of parallel jobs, bounded share per client, backpressure on upload when the queue is full (HTTP 429 with Retry-After).
- **Worker interface:** lean internal protocol (e.g. HTTP over a Unix socket or gRPC on the internal Docker network); workers are stateless and horizontally scalable (`docker compose up --scale worker=n`).
- **Code structure:** `cmd/api`, `cmd/worker`, `internal/` (pipeline, imaging, batch, validate, httpapi, zipstream), `web/` (frontend), `deploy/` (compose, seccomp profiles), `docs/`.
- **API first:** the OpenAPI specification is written before implementation; server stubs and the TypeScript client are generated from it.

## Security requirements

Every uploaded file is considered hostile: it is processed only in an isolated worker without network access, with hard resource limits and without persistent storage. Guiding references are OWASP ASVS 5.0 (Level 2) and the OWASP recommendations on file uploads.

### Input

- Maximum file size, file count per batch, total size per batch, and number of open batches per client are configurable and restrictive by default.
- Detect file type by magic bytes, not by extension or Content-Type; allowlist of image formats.
- Pixel limit per image and frame limit for animated images against decompression bombs; libvips with strict fail-on settings for broken files.
- ZIP uploads: limits for entry count, total uncompressed size, and compression ratio; protection against Zip Slip (`../`, absolute paths, symlinks); no nested archives.
- Pipeline and preset JSON strictly validated by schema; limits for target dimensions, number of steps, and watermark graphics.
- Never use file names for paths; internally only random IDs. Output names from the rename pattern are sanitized; download name per RFC 6266.

### Processing isolation

- Each job runs as its own process with timeout, CPU, RAM, and process limits (cgroups v2, RLIMIT_*).
- Worker containers have no network (`network_mode: none` or an internal network without egress) and only tmpfs as writable storage.
- Seccomp profile, no-new-privileges, all Linux capabilities dropped, read-only root filesystem, non-root UID ≥ 10000.
- Optional: document gVisor (runsc) as the runtime for workers.

### HTTP and web UI

- Strict security headers: CSP without unsafe-inline/unsafe-eval, `frame-ancestors 'none'`, HSTS (when behind TLS), `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, Permissions-Policy.
- CSRF protection for state-changing requests (same-origin check plus token), CORS off by default.
- Rate limiting and a cap on concurrent jobs per client IP; X-Forwarded-For evaluated only from trusted proxies.
- Downloads only via short-lived, random job IDs (≥ 128 bits), retrievable once, deleted afterwards.
- Server timeouts set (ReadHeaderTimeout, ReadTimeout, WriteTimeout, IdleTimeout), MaxBytesReader on every upload.

### Data and logs

- No file contents, no original file names, and no plaintext IP addresses in logs (privacy under the Swiss FADP and the EU GDPR).
- Structured JSON logs, Prometheus metrics on a separate port, reachable internally only by default.
- No telemetry, no external fonts or CDNs.

### Supply chain

- All base images pinned by digest, GitHub Actions pinned by commit SHA.
- SBOM (SPDX or CycloneDX) with Syft, signing with cosign (keyless), SLSA provenance.
- Builds fail on known critical/high CVEs with an available fix (Trivy or Grype, govulncheck, pnpm audit).
- SECURITY.md with a vulnerability reporting process.

## Containers and operations

Delivered is a `compose.yaml` with two images from one repository, published as signed multi-arch images (linux/amd64, linux/arm64) in the GitHub Container Registry. A single all-in-one image is additionally allowed, as long as the workers inside it run as separate, restricted processes.

| Image | Contents | Network | Writable |
|---|---|---|---|
| api | Go binary with embedded frontend, batch queue, and ZIP streaming; distroless, target under 30 MB | Inbound from the reverse proxy only, internal network to workers | Nothing (tmpfs /tmp only) |
| worker | Go worker + libvips with codecs (HEIF, AVIF, JPEG XL, WebP), pdfcpu; scalable to any number | None except the internal job channel | tmpfs with size limit |

Requirements for `compose.yaml` and Dockerfiles:

- `read_only: true`, `cap_drop: [ALL]`, `security_opt: [no-new-privileges:true]`, non-root user, tmpfs with `noexec,nosuid` and a size limit.
- `mem_limit`, `cpus`, and `pids_limit` per service.
- Healthchecks without curl in the image: the binary ships its own `healthcheck` subcommand.
- No worker ports exposed; api listens on 127.0.0.1 by default or sits behind a reverse proxy (Traefik, Caddy, nginx) with TLS.
- Configuration exclusively via environment variables with secure defaults, documented in `docs/configuration.md` and `.env.example`. Examples: `PIXSHEAF_MAX_UPLOAD_MB`, `PIXSHEAF_MAX_FILES`, `PIXSHEAF_JOB_TIMEOUT`, `PIXSHEAF_MAX_CONCURRENCY`, `PIXSHEAF_TRUSTED_PROXIES`, `PIXSHEAF_LOG_LEVEL`.
- Graceful shutdown: finish running jobs, clear tmpfs.
- Project name: **Pixsheaf** (see ADR 0001); binaries `pixsheaf-api` and `pixsheaf-worker`, images `ghcr.io/<owner>/pixsheaf-api` and `ghcr.io/<owner>/pixsheaf-worker`. Keep the fork notice and the original's MIT license in LICENSE/NOTICE.

## Quality, tests, CI/CD, and maintenance

The project must not become outdated again: updates arrive automatically as pull requests, and every merge goes through the same security and test checks.

### Tests

- Unit tests for every conversion and validation, target coverage ≥ 80 % for Go code.
- Fuzzing (Go native fuzzing) for file type detection, parameter parsing, and PDF operations.
- Integration tests with a corpus of real and deliberately broken files (bombs, disguised types, ZIP bombs, Zip Slip archives, overlong file names).
- End-to-end tests of the UI with Playwright; accessibility with axe-core.
- Container tests: image starts read-only, without capabilities, workers cannot reach the internet.
- Load tests for batches (e.g. with k6): throughput, memory profile, and behavior with a full queue; batches with mixed valid and broken files.

### CI (GitHub Actions)

- Lint: golangci-lint, go vet, staticcheck, ESLint, svelte-check, hadolint for Dockerfiles.
- Security: govulncheck, Trivy/Grype on images, CodeQL, Gitleaks for secrets, OpenSSF Scorecard.
- Release: SemVer tags, changelog from Conventional Commits, multi-arch build, SBOM, cosign signature, provenance.

### Maintenance

- Renovate (or Dependabot) for Go modules, pnpm packages, base image digests, and Actions; security updates auto-merged on green CI.
- Weekly scheduled rebuild so patched system packages land in the image.
- Documentation: README.md (quick start in 3 commands), `docs/configuration.md`, `docs/api.md` (generated from OpenAPI), `docs/security.md` (threat model), `docs/decisions/` (ADRs), SECURITY.md, CONTRIBUTING.md.

## Process and acceptance

Work proceeds in six phases; each phase ends with a working state, green CI, and a short report (what is done, open questions, next steps).

1. **Analysis and decisions:** research versions of all components, create a threat model (STRIDE), ADRs for language, libvips bindings, worker isolation, and pipeline format (project name already decided in ADR 0001). Present the results for approval.
2. **Scaffold:** repository structure, Go modules, frontend setup, Dockerfiles, compose.yaml, CI pipeline with all checks (no features yet), Renovate.
3. **Image pipeline and single-image path:** pipeline format and JSON Schema, all individual steps (formats, size, crop, quality, metadata, watermark), worker isolation, single-image processing end to end via the API (upload → process → direct download), tests including fuzzing.
4. **Batch mode:** multi-file job sets, queue with fairness and backpressure, ZIP upload and streamed ZIP download, SSE progress, cancellation, rename patterns, presets, result report, load tests.
5. **Web UI:** start page with the single-image workflow (upload → choose action/preset → before/after preview → download); automatic switch to batch mode when several files are selected (file list, per-file status, ZIP download); EN/DE, dark mode, accessibility, E2E tests.
6. **Hardening and release:** review against ASVS L2, documentation, signed release 1.0.

### Acceptance criteria for 1.0

- [ ] All features from "Must (v1.0)" work via UI and API.
- [ ] On the start page, a single image can be uploaded, processed, previewed, and downloaded as a single file without any batch-specific step.
- [ ] Selecting more than one file switches to batch mode automatically, with the same steps applied to all files and a ZIP download.
- [ ] A batch with the configured maximum number of files completes without any container reaching its memory limit (proven by load test).
- [ ] Broken or malicious files in a batch are rejected individually; all other files are processed correctly and listed in the report.
- [ ] A preset created in the UI produces byte-identical results via the API for the same files.
- [ ] `docker compose up -d` starts the system on a fresh host without further steps; `--scale worker=n` increases throughput.
- [ ] Trivy/Grype report no critical or high CVEs with an available fix.
- [ ] All containers run read-only, non-root, without capabilities; workers without internet access (proven by test).
- [ ] After download, cancellation, or timeout, no files remain (proven by test).
- [ ] Security headers achieve grade A or better on Mozilla Observatory.
- [ ] Images are signed, SBOM and provenance are attached to the release; documentation is complete; Renovate is active.
