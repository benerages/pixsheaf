# Pixsheaf Phase 6: Hardening and release

Phase 6 reviews Pixsheaf against OWASP ASVS 5.0 Level 2, closes the gaps found, completes the documentation, and adds the signed release. All local checks pass: Go tests and vet, the libvips tests in the worker image, the container isolation self-check (27 of 27), the shell end-to-end tests, the browser tests in Chromium, Firefox, and a phone profile, and the load tests. Release 1.0.0 is built by the new release workflow from the tag `v1.0.0`.

## 1. ASVS 5.0 Level 2 review

[asvs-review.md](asvs-review.md) checks all 253 Level 1 and Level 2 requirements: 112 met, 14 fixed in this phase, 3 deliberate deviations, 9 operator responsibilities (TLS proxy, log shipping), and 115 not applicable (no accounts, no database, no OAuth, no WebRTC). It also contains the cryptographic inventory.

| Finding | Fix |
|---|---|
| Job cookie without the `__Host-` prefix (3.3.1, 3.3.3) | `__Host-pixsheaf-{jobId}`, `Path=/`; the job ID in the name binds the cookie to its job, so parallel jobs in one browser stay apart (ADR 0019 amendment) |
| JSON responses without `charset` (4.1.1) | `charset=utf-8` on JSON, problem details, and the event stream |
| File names ignored entirely (5.2.2) | Uploads and archive entries need an image or ZIP extension; the content still decides the format (ADR 0023) |
| The UI file server listed folders such as `/assets/` (13.4.3) | Folder paths answer 404 |
| Only internal errors were logged (16.x) | Every rejected API request is logged with route, status, problem code, job ID, and a pseudonymous client key; wrong tokens, cross-origin requests, and rate limits at warn; tokens, names, and addresses never (tested) |
| No metrics (project requirement) | Prometheus endpoint `PIXSHEAF_METRICS_ADDR`, off by default, on its own address |
| `PIXSHEAF_DISABLE_FORMATS`, promised by ADR 0016 as the switch for HEIC, never implemented | API refuses disabled formats (415 `format_disabled`, also for archive entries) and hides them in `/capabilities`; the job process keeps their libvips loaders blocked and checks the decoded format again |
| No remediation time frames, no SBOM, no cryptographic inventory (15.1.1, 15.1.2, 11.1.x) | `SECURITY.md`, release SBOMs, review document |

## 2. Release

`.github/workflows/release.yml` runs on tags `vX.Y.Z`:

| Step | What happens |
|---|---|
| Build | Both images natively on amd64 and arm64 runners, pushed by digest; BuildKit attaches an SPDX SBOM (Syft scanner) and SLSA provenance (`mode=max`) per platform |
| Publish | One multi-arch index per image with the tags `X.Y.Z`, `X.Y`, and `latest`; signed with cosign keyless (GitHub OIDC, recursively over all manifests) and attested with `actions/attest-build-provenance` |
| Release | GitHub release with the changelog from Conventional Commits (`scripts/changelog.sh`), the image references, verification commands, and the SBOMs |

Started by hand, the workflow is a dry run that builds everything for both architectures without publishing. All Actions are pinned by commit SHA and are at least seven days old (ADR 0013); for that reason `sigstore/cosign-installer` and `anchore/sbom-action` use the release before the latest one, and the SBOM comes from BuildKit's Syft scanner instead of a separate action.

## 3. Documentation

| Document | Change |
|---|---|
| README | Feature overview for 1.0, quick start in three commands with the release images, signature check, API, development |
| `docs/api.md` | Generated from `api/openapi.yaml` (`pnpm gen:docs`, part of `scripts/generate-api.sh`); CI fails when it is out of date |
| `docs/deployment.md` | Proxy responsibilities, Caddy and nginx examples, logs and metrics |
| `docs/security.md` | Logging inventory; threat model updated (archives in the sandbox, decided owner questions) |
| `docs/configuration.md`, `.env.example` | `PIXSHEAF_DISABLE_FORMATS`, `PIXSHEAF_METRICS_ADDR`, logging levels |
| `SECURITY.md` | Supported versions and remediation time frames |
| `CONTRIBUTING.md` | Go through `scripts/go.sh`, end-to-end tests, dependency policy, releases |

## 4. Acceptance criteria for 1.0

| Criterion | Evidence |
|---|---|
| All "Must" features via UI and API | Phase 3 to 5 tests; conditions and chained steps through the pipeline editor and the API |
| Single image without any batch step | Browser test `single.spec.ts` (no "batch" text, single-file download) |
| Several files switch to batch mode with a ZIP | `batch.spec.ts`, `tests/e2e/batch.sh` |
| Batch with the maximum file count within memory limits | Load test: 8 batches of 50 files (the default maximum), 3000 × 2000 pixels each; peak API 46 MiB of 768 MiB, worker 112 MiB of 1024 MiB, no OOM kill |
| Broken files rejected individually, listed in the report | Load test (every tenth file broken, exactly those failed), `batch.sh`, `batch.spec.ts` |
| UI preset and API give byte-identical results | New browser test `preset-parity.spec.ts` (Web preset, compared byte for byte) |
| `docker compose up -d` on a fresh host; `--scale worker=n` increases throughput | Compose pulls the release images before building; load test 10.9 files/s with one worker, 17.1 files/s with two (+57 %) |
| No critical or high CVEs with a fix | Grype gate in the container workflow; govulncheck; `pnpm audit` |
| Containers read-only, non-root, no capabilities; workers without internet | `tests/container/run.sh`: 27 of 27 checks |
| No files remain after download, cancel, or timeout | Job manager tests count the blobs after each path |
| Mozilla Observatory grade A or better | Open until a public deployment exists (see §6) |
| Signed images, SBOM and provenance on the release; docs complete; Renovate active | Release workflow; this phase's documentation; Renovate since Phase 2 |

## 5. Issues found and fixed

| Issue | Fix |
|---|---|
| The worker image failed to build after a new Wolfi glibc revision: the runtime root installed unpinned dependencies at the newest revision, the cached build stage had the previous one | `make-rootfs` resolves in two passes and pins every package to the build-stage version |
| Hadolint DL3066 on the browser test image | Test user with the numeric UID 10001 |
| Prettier did not format `.svelte` files | `prettier-plugin-svelte` 4.2.0 |

## 6. Known gaps after 1.0

| Gap | Note |
|---|---|
| Mozilla Observatory grade | Needs a public HTTPS deployment. The headers match the scoring rules for a top grade (strict CSP with `default-src 'none'`, `frame-ancestors 'none'`, `__Host-` cookie, `nosniff`, `no-referrer`, no CORS); HSTS and the HTTP redirect come from the proxy. Check with the deployment of 1.0 |
| Screen reader test by hand (NVDA, VoiceOver) | axe-core finds no violations; a manual test is still worth doing |
| Go coverage of packages with child processes and container code, and of the libvips code | Coverage of the job processes needs `GOCOVERDIR` across `exec`; planned for 1.1 |
| seccomp allowlist for the job process | The job process uses a denylist (no `socket`, `connect`, `execve` after setup) plus Docker's default profile and Landlock; a full allowlist needs syscall tracing of every codec |
| Load test with real photos and in CI | The k6 test uses synthetic PNGs and takes minutes; it stays a manual release check |
| Playwright's Firefox does not report blob downloads | Firefox checks the download link instead |
