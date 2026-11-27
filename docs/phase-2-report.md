# Pixsheaf Phase 2: Scaffold

Phase 2 is complete. The repository `github.com/benerages/pixsheaf` exists. The first push on 2026-11-27 ran all four workflows (`ci`, `container`, `codeql`, `scorecard`), and all were green as of commit "ci: run container tests via bash and restore executable bit". No image features exist yet. The API serves a placeholder UI and the worker only connects to the API socket. Every check from the project brief that applies to a scaffold now runs on each push. The container workflow builds both images, runs the worker isolation self-check inside the hardened compose setup, and scans both images with Grype. Several gates are still advisory or missing, and they are listed in §4.

## 1. What is in place

| Area | State |
|---|---|
| Repository layout | `cmd/pixsheaf-api`, `cmd/pixsheaf-worker`, `internal/` (config, httpapi, selfcheck, transport, ui, version), `web/`, `deploy/`, `docs/`, `tests/container/`, `spikes/`, `scripts/` |
| Go | Module `github.com/benerages/pixsheaf`, Go 1.27 with toolchain go1.27.4, standard library only (no third-party modules yet) |
| API | `serve`, `healthcheck`, `version` subcommands; security headers with strict CSP; GET/HEAD only; internal Unix socket server (ADR 0007) |
| Worker | `run`, `healthcheck`, `selfcheck`, `version` subcommands; pings the API over the socket; no image processing yet |
| Frontend | Svelte 5 + Vite 8 + Tailwind 4 SPA, embedded via `go:embed`; pnpm 12.23.0 with lockfile |
| Containers | `api` on distroless static (digest-pinned), `worker` on `debian:trixie-slim`; both UID 10001, read-only root, `cap_drop: [ALL]`, `no-new-privileges`, tmpfs `/tmp` with `noexec,nosuid,nodev`, memory/CPU/PID limits; worker with `network_mode: none` and the socket volume mounted read-only |
| CI (`ci`) | gofmt, go vet, `go test -race` with coverage, govulncheck, staticcheck, golangci-lint (errcheck, gosec, noctx, bodyclose, errorlint, nolintlint), pnpm audit/lint/svelte-check/build, inline-script check for CSP, hadolint, zizmor, gitleaks |
| CI (`container`) | Builds both images, runs the isolation self-check (`tests/container/run.sh`), checks API hardening via `docker inspect`, Grype scan of both images (fail on fixable high/critical) |
| CI (other) | CodeQL, OpenSSF Scorecard; weekly scheduled container rebuild |
| Supply chain | All Actions pinned by commit SHA; Renovate config with 7-day minimum release age; pnpm `minimumReleaseAge` (7 days), `blockExoticSubdeps`, `strictDepBuilds` |
| Docs | README, SECURITY.md, CONTRIBUTING.md, `docs/configuration.md`, `docs/deployment.md`, `docs/security.md`, ADRs 0001–0017 |

## 2. Container isolation self-check

The worker's `selfcheck` subcommand verifies from inside the running container: API socket reachable; socket directory read-only; only the loopback interface; DNS, IPv4, and IPv6 egress blocked; UID ≥ 10000; no effective capabilities; `no_new_privs`; seccomp active; root filesystem read-only; `/tmp` writable but `noexec`. It passed in CI on GitHub's Ubuntu 24.04 runner (runc, no userns-remap). Together with the Phase 1 spike on Docker 29.10 with userns-remap (13/13), both host variants are now covered.

## 3. Issues found and fixed during bring-up

| Issue | Fix |
|---|---|
| `bootstrap.sh` produced a lockfile with only pnpm's own `packageManagerDependencies` and no resolved packages | Install pnpm via `npm install --global` (same as Dockerfile and CI), store in `/tmp`, and fail unless `svelte`, `vite`, and `typescript` are resolved |
| Five pinned devDependencies were younger than the 7-day release-age gate | Re-pinned to the newest mature versions chosen by pnpm (`svelte` 5.59.0, `vite` 8.3.8, `@sveltejs/vite-plugin-svelte` 7.3.1, `typescript-eslint` 8.73.0); new helper `scripts/pin-mature-deps.sh`. Renovate raises them once they mature. |
| `pnpm add --save-exact` kept caret ranges when a range was passed | Helper re-adds each package with its resolved version in a second pass |
| golangci-lint: 12 findings (errcheck 7, noctx 3, gosec 2) | Explicitly discarded close/remove errors; `transport.Listen` now takes a context and uses `net.ListenConfig`; `DialContext`, `CommandContext`; justified `nolint` for G304 (probe path from config) and G302 (socket mode 0660 per ADR 0007) |
| `run.sh` lost its executable bit (repo copied from a sync folder with mode 0600) | Workflow calls `bash tests/container/run.sh`; executable bits restored in the index |
| Renovate's regex for `pnpm@X.Y.Z` no longer matched the rewritten bootstrap script | Literal `pnpm@12.23.0` restored in both scripts |

## 4. Known gaps carried into later phases

| Gap | Planned in |
|---|---|
| Go coverage is about 24 % (cmd packages untested); the 80 % gate is not enforced yet | Phase 3 (enforce once the pipeline packages exist) |
| `node:26.13.1-trixie-slim` and `debian:trixie-slim` are not pinned by digest | First Renovate run (pin digests) |
| harden-runner runs with `egress-policy: audit`, not `block` | Phase 6, after collecting the allowed endpoints from audit logs |
| Trivy scan, release workflow (multi-arch, SBOM, cosign, provenance) | Phase 6 |
| Worker image has no libvips yet | Phase 3 |
| Socket mode 0660 although both images use the same UID; 0600 would suffice | Owner decision (optional ADR 0007 amendment) |
| Container test under gVisor (`run.sh gvisor`) not run yet | Phase 3 or 6 |

## 5. Repository settings still to do (owner)

- Enable private vulnerability reporting.
- Install the Renovate GitHub App for the repository.
- Actions settings: require actions to be pinned to a full-length commit SHA.

## 6. Next: Phase 3

Build libvips 8.18.x with codecs from pinned source in the worker image (ADR 0004), apply the hardening configuration (ADR 0008), write the pipeline JSON Schema (ADR 0009) and the OpenAPI specification (ADR 0011/0012), implement the single-image path end to end (upload → isolated job subprocess → direct download), and add tests including fuzzing for type detection and parameter parsing.
