# Security design and threat model

This document describes what Pixsheaf protects, where the trust boundaries are, and which controls address each threat. The architecture decisions behind the controls are in [decisions/](decisions/); the full Phase 1 analysis, including sources, is in [phase-1-report.md](phase-1-report.md).

In the tables, "Project §…" refers to the sections of the [project brief](project-brief.md).

## Owner decisions that shape the threat model

- **No antivirus scanning; uploads restricted to images** (ADR 0014). Deviation from OWASP ASVS 5.0 requirement 5.4.3, compensated by magic-byte and decoder allowlists, header checks before decoding, mandatory re-encoding, and sandboxed decoding.
- **Anonymous uploads** (ADR 0015), compensated by loopback binding, rate limits, queue caps, and short-lived one-time downloads.
- **HEIC decoding in the default worker** (ADR 0016), contained by libheif security limits and the per-job sandbox; can be disabled at runtime.
- **Job transport validated** (ADR 0007, amendment): workers without network reach the API only through a read-only mounted Unix socket volume; verified on 2026-11-27 with Docker 29.10 and `userns-remap`.

## Decoders, not the API, carry the security risk
**The HEIF/HEVC chain has the most CVEs by far.** Debian lists about 40 libheif CVEs for 2025–2026. One of them, CVE-2026-120208, is a heap overflow fixed in **1.23.3** that Red Hat titles as arbitrary code execution ([Debian libheif](https://security-tracker.debian.org/tracker/source-package/libheif), [Red Hat 2565445](https://bugzilla.redhat.com/show_bug.cgi?id=2565445)). In libde265, CVE-2026-49346 lets a crafted SPS make the decoder allocate about 1 KB and then write about 4 GB (CVSS 7.1, fixed in 1.1.0) ([OSV](https://osv.dev/vulnerability/CVE-2026-49346)). Grid and overlay items amplify dimensions: CVE-2025-68431 is an `iovl` overlay over-read fixed in libheif 1.21.0 ([OSV](https://osv.dev/vulnerability/CVE-2025-68431)). **libvips itself** received hardening in every 8.18 point release. CVE-2026-33327 is a heap overflow in `vipsload` fixed in 8.18.1. Release 8.18.6 marks heifload as untrusted when libvips is built against libheif < 1.23.2, and 8.18.7 does the same for jp2ksave ([Debian CVE-2026-33327](https://security-tracker.debian.org/tracker/CVE-2026-33327), [ChangeLog](https://raw.githubusercontent.com/libvips/libvips/master/ChangeLog)). The operation-blocking API Pixsheaf depends on had its own bug: CVE-2025-59933 is an over-read in `vips_operation_block_set` in libvips ≤ 8.17.1 ([VulDB](https://vuldb.com/vuln/326340)). **libjxl** CVE-2026-1837 writes out of bounds only when lcms2 is the color management engine, and it is fixed in 0.13.0 ([Debian](https://security-tracker.debian.org/tracker/CVE-2026-1837)). **lcms2** CVE-2026-42798 is fixed in 2.19, and **libjpeg-turbo** CVE-2026-75466 affects 3.2.0 ([lcms2](https://security-tracker.debian.org/tracker/source-package/lcms2), [libjpeg-turbo](https://security-tracker.debian.org/tracker/source-package/libjpeg-turbo)). libwebp's CVE-2023-4863 is the precedent showing these bugs get exploited: it is in CISA's Known Exploited Vulnerabilities catalog ([OSV](https://osv.dev/vulnerability/CVE-2023-4863)).

**Distribution packages lag behind these fixes.** Debian trixie ships vips 8.16.1 and still has 17 open libheif issues ([vips](https://tracker.debian.org/pkg/vips), [libheif](https://tracker.debian.org/pkg/libheif)). That gap is why the worker builds libvips from source; ADR 0018 also moves the codec libraries to Wolfi, because several trixie codecs have open issues without a Debian security advisory.

**The bindings are attack surface too.** In October and November 2026, vipsgen fixed freeing a blob that libvips owns (#171) and narrowing blob lengths to 32 bits (#178) ([vipsgen commits](https://github.com/cshum/vipsgen/commits/main)). In September 2026, govips fixed three cgo bugs, including a double unref (#549) ([govips commits](https://github.com/davidbyttow/govips/commits/master)). A large or crafted input can reach both kinds of bug, so the binding runs inside the sandbox. OpenAPI codegen has a similar history: oapi-codegen v2.7.1 through v2.8.2 fixed Go code injection in generated code ([releases](https://github.com/oapi-codegen/oapi-codegen/releases)).

**Mutable tags and fresh releases are the supply-chain attack vector.** In March 2026 the Trivy ecosystem was compromised (CVE-2026-33634). Attackers published malicious Trivy v0.69.4 binaries and images, force-pushed `trivy-action` tags 0.0.1–0.34.2, and replaced `setup-trivy` tags. Users who referenced images by digest or built from source were not affected ([GHSA-69fq-xp46-6x23](https://github.com/aquasecurity/trivy/security/advisories/GHSA-69fq-xp46-6x23)). The malicious artifacts were available for only hours. On 2026-05-11, "Mini Shai-Hulud" poisoned the pnpm cache through a `pull_request_target` workflow and stole OIDC tokens. It published 84 malicious artifacts across 42 `@tanstack/*` packages **with valid provenance**, ran a `prepare` hook from an optional git dependency, and persisted through `.claude/settings.json` (CVE-2026-45321). That followed Shai-Hulud (September 2025, more than 500 packages) and Sha1-Hulud 2.0 (November 2025) ([Snyk](https://snyk.io/blog/tanstack-npm-packages-compromised)). Three conclusions follow. Provenance alone does not establish trust. A release-age delay of several days would have avoided every one of these incidents. And `pull_request_target` must never be used. GitHub now blocks it by default for public repositories from **2026-12-02** ([changelog](https://github.blog/changelog/2026-11-02-pull_request_target-blocked-by-default-december-2)).

## STRIDE threat model

### Assets

| ID | Asset | Why it matters |
|---|---|---|
| A1 | Uploaded images and their metadata (EXIF GPS, XMP) | Personal data under the Swiss FADP and the EU GDPR; confidentiality between clients |
| A2 | Processed outputs and download tokens (job IDs ≥ 128 bits) | Anyone holding a token can retrieve the output |
| A3 | Worker and host integrity | A decoder RCE is the most likely compromise |
| A4 | Service availability (CPU, RAM, queue) | Pixel, ZIP, and frame bombs; queue flooding |
| A5 | Logs and metrics | Must not contain file contents, original names, or plaintext IPs |
| A6 | Release artifacts (images, SBOM, signatures, CI secrets, OIDC identity) | Supply-chain attacks against every downstream self-hoster |

### Trust boundaries

| TB | Boundary | Crossing | Notes |
|---|---|---|---|
| TB1 | Browser → reverse proxy | HTTPS | Untrusted client; anonymous by default (ADR 0015) |
| TB2 | Reverse proxy → API | HTTP on an internal network or 127.0.0.1 | `X-Forwarded-For` is trusted only from `PIXSHEAF_TRUSTED_PROXIES` |
| TB3 | API → worker socket | Pathname Unix socket on a shared tmpfs volume | The API is the only party that parses ZIP and HTTP; workers have no network |
| TB4 | Worker supervisor → per-job child | fork/exec, pipes | The child applies rlimits, no_new_privs, Landlock, and seccomp before it decodes anything |
| TB5 | Child Go code → libvips and codecs (cgo) | In-process C | **The real hostile parse.** Treated as compromised |
| TB6 | CI → registry → self-hoster | Signed OCI images | cosign signature, SBOM, and provenance verified by digest |

### Threats and mitigations

ASVS references are to **OWASP ASVS 5.0 chapter V5 (File Handling)** ([ASVS 5.0 V5](https://github.com/OWASP/ASVS/blob/master/5.0/en/0x14-V5-File-Handling.md)). Requirements not in V5 map to sections of the project instructions. The full Level 2 review of all ASVS chapters is in [asvs-review.md](asvs-review.md).

**Spoofing**

| ID | Threat | Mitigation | Requirement / ASVS |
|---|---|---|---|
| S1 | A client spoofs `X-Forwarded-For` to evade per-IP limits | Honor XFF only from configured trusted proxies; otherwise use the socket peer address | Project §HTTP (rate limiting) |
| S2 | Cross-site request starts jobs (CSRF) | Same-origin check plus a CSRF token on state-changing requests; CORS off | Project §HTTP |
| S3 | A compromised worker impersonates another worker, or a fake process listens on the socket | The API owns the single socket in pull mode; socket mode 0600 and socket directory 0700, same UID ≥ 10000 in API and worker; volume mounted `noexec,nosuid,nodev`; per-worker subdirectories if workers listen | ADR 0007 |
| S4 | A tampered base image or action is substituted | Pin by digest and SHA; verify distroless with cosign (`keyless@distroless.iam.gserviceaccount.com`) ([distroless](https://github.com/GoogleContainerTools/distroless)) | Project §Supply chain; ADR 0013 |

**Tampering**

| ID | Threat | Mitigation | Requirement / ASVS |
|---|---|---|---|
| T1 | Zip Slip (`../`, absolute paths, symlinks) in an uploaded ZIP | ZIPs are unpacked only inside the worker sandbox with pure-Go `archive/zip` (ADR 0022); entry paths are cleaned per segment and used only for output names, never for file system paths; symlinks, encrypted entries, and nested archives fail individually | **5.3.2, 5.3.3 (L3), 5.2.5 (L3)** |
| T2 | Polyglot or disguised file (wrong extension or Content-Type) | Detect type from magic bytes against an allowlist; fully decode and re-encode; serve only the re-encoded bytes with a fixed `Content-Type`, `nosniff`, and `attachment` | **5.2.2**; [OWASP cheat sheet](https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html) |
| T3 | Pipeline or preset JSON injects oversized or unexpected parameters | JSON Schema 2020-12 with `additionalProperties: false`; limits on steps, dimensions, and watermark size | Project §Input; ADR 0009 |
| T4 | A compromised worker tampers with other jobs' data through the shared volume | Stream payloads over the socket, never through shared files; per-job tmpfs directory restricted by Landlock in the child | ADR 0006/0007 |
| T5 | Dependency or Action tag hijack (as with Trivy and TanStack) | SHA pins plus `actions.lock`; `--frozen-lockfile`; 7-day minimum release age; `blockExoticSubdeps`; `allowBuilds` allowlist | Project §Supply chain; ADR 0013 |

**Repudiation**

| ID | Threat | Mitigation | Requirement / ASVS |
|---|---|---|---|
| R1 | Abuse cannot be traced, but logging IPs would violate privacy | Structured JSON logs with a keyed hash or truncated IP, job ID, outcome, and reason code; never file names or contents | Project §Data and logs |
| R2 | A release cannot be tied to a source commit | `attest-build-provenance`, cosign keyless signing by digest, SBOM attestation | Project §Supply chain |

**Information disclosure**

| ID | Threat | Mitigation | Requirement / ASVS |
|---|---|---|---|
| I1 | GPS and EXIF leak in outputs | Strip metadata by default; keeping it is opt-in per pipeline | Project §Image features |
| I2 | Another client guesses or replays a download | Random IDs of at least 128 bits; each download works once and is then deleted; sets expire on timeout | Project §HTTP |
| I3 | Header injection or Unicode tricks through the download file name | Sanitize the rename output; set `Content-Disposition` per RFC 6266 with `filename*` | **5.4.1, 5.4.2** |
| I4 | A compromised decoder exfiltrates data | Workers use `network_mode: none`; Landlock forbids TCP (ABI ≥ 4) and scopes abstract Unix sockets (ABI ≥ 6); seccomp in the child denies `socket` and `connect` ([Landlock docs](https://docs.kernel.org/userspace-api/landlock.html)) | Project §Processing isolation |
| I5 | Cross-job leakage through the libvips cache or a long-lived process | One process per job; `vips_cache_set_max(0)` (vipsgen's default) ([vipsgen util.go](https://github.com/cshum/vipsgen/blob/main/vips/util.go)) | ADR 0006/0008 |
| I6 | XSS through UI or SVG | SVG is excluded entirely; strict CSP without `unsafe-inline`; Svelte 5 no longer injects `<style>` for transitions ([svelte#6662](https://github.com/sveltejs/svelte/issues/6662)) | Project §HTTP; ADR 0010 |
| I7 | CI secrets or OIDC tokens are stolen | No `pull_request_target`; `permissions` default read; harden-runner egress block; separate caches per workflow | ADR 0013 |

**Denial of service**

| ID | Threat | Mitigation | Requirement / ASVS |
|---|---|---|---|
| D1 | Pixel flood or decompression bomb | Load the header only and check width × height × pages (canvas size, not tile size) before decoding; keep libheif security limits on ([libheif README](https://raw.githubusercontent.com/strukturag/libheif/master/README.md)); never pass `unlimited` | **5.2.1, 5.2.6 (L3)**; [libvips checklist](https://www.libvips.org/API/current/developer-checklist.html) |
| D2 | Animation and frame bombs (GIF, WebP, AVIF) | Cap frames; reload with a page limit (the imagor pattern ([processor.go](https://github.com/cshum/imagor/blob/master/processor/vipsprocessor/processor.go))); adopt the multipage height limits in libvips 8.18.8 when released | **5.2.6** |
| D3 | ZIP bomb | Check entry count, per-entry and total uncompressed size counted from bytes actually read, and compression ratio, before and during extraction | **5.2.3** |
| D4 | A progressive JPEG uses far more RAM (about 4 GB versus 72 MB for baseline) | Route progressive or interlaced files to a low-priority queue, or apply a lower pixel cap ([checklist](https://www.libvips.org/API/current/developer-checklist.html)) | 5.2.1 |
| D5 | Queue flooding by one client | Global and per-client concurrency caps; a cap on open job sets per client; HTTP 429 with `Retry-After`; fair round-robin scheduling | **5.2.4 (L3)**; Project §Resource control |
| D6 | Slowloris attacks or oversized bodies | `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, and `IdleTimeout`; `MaxBytesReader` on every upload | 5.2.1 |
| D7 | Runaway decode (CPU or RAM) | Wall-clock SIGKILL of the child's process group; RLIMIT_CPU and RLIMIT_AS (or RLIMIT_DATA); container `mem_limit`, `cpus`, and `pids_limit` as the backstop | Project §Processing isolation |

**Elevation of privilege**

| ID | Threat | Mitigation | Requirement / ASVS |
|---|---|---|---|
| E1 | RCE in a decoder (libheif, libde265, libjxl, libwebp-class bugs) | Allowlist of loaders plus `VIPS_BLOCK_UNTRUSTED`; patched versions (libheif ≥ 1.23.7, libvips ≥ 8.18.7); a per-job sandbox; no ImageMagick, PDF, SVG, OpenSlide, FITS, or Matlab loaders compiled in | Project §Input; ADR 0008/0018 |
| E2 | Escape from the child into the container | no_new_privs, Landlock limited to read-only libraries and a read-write per-job directory, seccomp (no `execve` after setup), RLIMIT_CORE=0 | ADR 0006 |
| E3 | Escape from the container to the host | Non-root UID ≥ 10000, `cap_drop: [ALL]`, read-only root filesystem, a custom seccomp profile, optional gVisor ([gVisor](https://raw.githubusercontent.com/google/gvisor/master/README.md)) | Project §Containers |
| E4 | Uploaded files execute as code | No writable executable paths; tmpfs mounted `noexec`; files are never placed in a served directory | **5.3.1** |
| E5 | A malicious OpenAPI spec injects code during codegen | The spec is written by the project; oapi-codegen pinned to ≥ v2.8.2 | ADR 0011 |

**Coverage of ASVS V5 at L2:** 5.1.1 (documentation, in `docs/configuration.md` and `docs/security.md`), 5.2.1–5.2.3, 5.3.1–5.3.2, and 5.4.1–5.4.2 are covered. **5.4.3 (antivirus scanning) is a deliberate deviation** (ADR 0014). Pixsheaf voluntarily adopts the L3 items 5.2.4, 5.2.5, 5.2.6, and 5.3.3 ([ASVS 5.0 V5](https://github.com/OWASP/ASVS/blob/master/5.0/en/0x14-V5-File-Handling.md)). Mapping documents that use ASVS 4.0.3 numbering (V12) do not apply ([ASVS 4.0.3 V12](https://raw.githubusercontent.com/OWASP/ASVS/v4.0.3/4.0/en/0x20-V12-Files-Resources.md)).

## Logging

Both services write JSON lines to standard output and nowhere else (ASVS 5.0 V16).

| Event | Level | Fields |
|---|---|---|
| Rejected API request (4xx) | info, event `request_rejected` | `route` (pattern, such as `POST /api/v1/jobs/{jobId}/files`), `method`, `status`, `code`, `duration_ms`, `job` (job ID, if any), `client` |
| Wrong or missing job token (401) | warn, event `auth_failed` | as above |
| Cross-origin state change (403) | warn, event `cross_origin_rejected` | as above |
| Rate limit (429) | warn, event `rate_limited` | as above |
| Server error (5xx) | error, event `server_error` | as above; the cause is logged in a separate entry without user data |
| Successful API request | debug only | as above |
| Start, shutdown, worker claims and failures | info to error | version, addresses, slot number |

`client` is the first 12 hex digits of an HMAC-SHA-256 of the client address, keyed with a secret that exists only in memory and changes on every start. It allows correlating the requests of one client within one run, but not identifying the client. File contents, original file names, job tokens, and IP addresses are never logged; `TestSecurityEventsLoggedAndCounted` checks this. The same events are counted on the optional metrics endpoint (`pixsheaf_security_events_total`).

