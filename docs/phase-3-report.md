# Pixsheaf Phase 3: Image pipeline and single-image path

Phase 3 is complete. A single image can be uploaded, processed in the isolated worker, and downloaded once through the versioned API (`/api/v1`), and the same path is covered by unit, fuzz, container, and end-to-end tests. All four workflows (`ci`, `container`, `codeql`, `scorecard`) were green on commit 3773c23; the container workflow now also runs the end-to-end test against the running stack. There is no web UI for jobs yet (Phase 5), and jobs with more than one file are rejected until batch mode exists (Phase 4).

## 1. What is in place

| Area | State |
|---|---|
| Worker build | libvips 8.18.7 with libheif 1.23.7, libde265 1.1.3, cgif 0.5.4, and quantizr 1.4.3 built from pinned tags and verified commits on Wolfi (ADR 0018); runtime image `FROM scratch`, about 28 MB |
| libvips access | Direct cgo (ADR 0003 amendment); loaders and savers limited to an allowlist (ADR 0008) |
| Job sandbox | One process per file with rlimits, Landlock, and a seccomp denylist; input and output over pipes (ADR 0006 amendment); container self-check: 27 of 27 checks |
| Pipeline | Declarative JSON (ADR 0009), validated by a strict Go decoder with JSON Pointers in every error (ADR 0009 amendment) |
| Steps | auto-orient, resize (width, height, longest edge, percent; fit, fill, crop; content-aware gravity), crop (aspect ratio or rectangle), rotate, flip, text and image watermark, convert; conditions on size, orientation, and source format |
| Formats | Input: JPEG, PNG, WebP, GIF, TIFF, BMP, AVIF, HEIC/HEIF, JPEG XL. Output: JPEG, PNG, WebP, GIF, TIFF, BMP, AVIF, JPEG XL, PDF |
| Output options | Quality, lossless (WebP, JPEG XL), palette PNG, progressive, target size in KB (binary search over quality), metadata stripped by default with opt-in keep and separate GPS opt-in, conversion to sRGB |
| Animation | GIF and WebP animations keep their frames, delays, and loop count for GIF and WebP output; other formats get the first frame; frame limit |
| Text watermarks | Rendered in Go with the bundled Go Bold font, no pango or font files (ADR 0020) |
| BMP and PDF | BMP decoded and encoded in Go; PDF output by a minimal writer around a libvips JPEG, no PDF library (ADR 0021) |
| Job manager (API) | Jobs in memory, files on tmpfs under random names through `os.Root`, upload sniffing by magic bytes, queue and storage limits with 503 and `Retry-After`, lease timeouts, expiry, one-time download, cleanup of inputs after processing and of everything after download, cancellation, or expiry |
| Worker channel | Pull model over the Unix socket (ADR 0007): long-poll claim, result report with size and format checks; worker slots per container; graceful shutdown finishes running files |
| Public API | `capabilities`, create, get, delete, upload, get and cancel file, Server-Sent Events, download; job token as Bearer, cookie only for events and download, same-origin checks (ADR 0019); RFC 9457 problems with stable codes; RFC 6266 file names |
| Configuration | 20 `PIXSHEAF_*` variables with secure defaults (`docs/configuration.md`, `.env.example`) |

## 2. Tests

| Kind | Coverage |
|---|---|
| Unit (`go test -race`) | All packages; libvips code in the worker image (`--target test`) |
| Fuzzing | 7 targets (`scripts/fuzz.sh`, 15 s each in CI): format sniffing, pipeline parser, output names, job protocol header, `Content-Disposition`, same-origin check, PDF cross-reference table |
| Container | Isolation self-check of the worker; API hardening via `docker inspect` |
| End to end | `tests/e2e/single-image.sh` against `docker compose up`: convert and resize, download, second download 404, broken file fails alone with `decode_failed`, SVG rejected with 415, 401 without or with a wrong token, 403 cross-origin, delete |

Statement coverage without libvips (`go test -cover`, 2026-11-27):

| Package | Coverage | Note |
|---|---|---|
| config | 96.8 % | |
| jobproto | 92.6 % | |
| pipeline | 90.1 % | |
| imaging (without libvips) | 84.8 % | Go parts only (text, PDF); libvips code is tested in the worker image but not measured yet |
| httpapi | 83.4 % | |
| jobs | 79.4 % | Just below the target |
| ui | 75.0 % | |
| transport | 72.1 % | |
| sandbox | 50.9 % | `Apply` runs only in a job process |
| jobexec | 42.5 % | The job process is a child; its coverage is not collected |
| selfcheck | 17.0 % | Runs inside the container test |

The project target is 80 % for Go code. Five packages are below it, mostly because their code runs in a child process or a container, where `go test -cover` does not see it. Phase 6 collects coverage from those runs (`GOCOVERDIR`) and from the worker image before deciding on further tests.

## 3. Issues found and fixed

| Issue | Fix |
|---|---|
| Wolfi's libde265 package shipped an outdated shared library | libde265 built from source |
| AVIF lossless fails with SVT-AV1 (no 4:4:4) | Rejected by pipeline validation |
| Corrupt pixel data was reported as `encode_failed`, because libvips decodes lazily while saving | A failed save decodes the input once more; if that fails, the file fails with `decode_failed` (found by the end-to-end test) |
| `strings.EqualFold` treats any two invalid UTF-8 bytes as equal, so a malformed `Origin` could match | Origin must be http or https; hosts compared case-insensitively in ASCII only (found by fuzzing, kept as a regression seed) |
| gosec G304 and G124 | Job files through `os.Root`; the job cookie is always `Secure` |
| `golang.org/x/net` advisories | Bumped to v0.57.0 |

## 4. Known gaps carried into later phases

| Gap | Phase |
|---|---|
| Jobs with more than one file, ZIP upload and download, rename patterns, presets, report, fair queue per client | 4 |
| Rate limiting per client and `PIXSHEAF_TRUSTED_PROXIES` (configured but not used yet) | 4 and 6 |
| Web UI for jobs | 5 |
| `maxFrames` is enforced but not reported by `GET /capabilities` (the schema has no field yet) | 4 |
| Seccomp denylist instead of an allowlist; golangci-lint and staticcheck do not see code with the `vips` build tag | 6 |
| No fuzzing of the libvips decoders inside the sandbox; no HEIC or AVIF file in the end-to-end test | 6 |
| Watermarks on images with a non-sRGB ICC profile (other than CMYK) can shift slightly in color | Open; revisit with real images |
| Text watermarks support Latin, Greek, and Cyrillic only (ADR 0020) | Open |
| gVisor and rootless Docker untested (ADR 0007) | 6 |
| `minimatch` override in `web/pnpm-workspace.yaml` until ESLint ships the fix as a mature release | Renovate |

## 5. Next: Phase 4

Batch mode on the same job model: several files per job, ZIP upload with bomb and Zip Slip protection, streamed ZIP download with `report.json`, fair queue across clients, rename patterns, presets with JSON import and export, and load tests with k6.
