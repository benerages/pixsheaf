# Pixsheaf Phase 1: Pin, isolate, and decide

Phase 1 is ready for owner approval with one caveat: every version below was verified on 2026-11-27, but none is pinned by digest or checksum yet. The recommended baseline is **Go 1.27.4, libvips 8.18.7 built from source on `debian:trixie-slim`, vipsgen v2.1.3, libheif 1.23.7, pdfcpu v0.16.4, Svelte 5.59 + Vite 8 + Tailwind 4.3.5, pnpm 12.23.0, and TypeScript held at 6.0.x**. The threat model centers on the decoder chain. In 2025–2026, libheif, libde265, libjxl, and libvips itself had dozens of memory-safety CVEs, so the architecture assumes the decoders will be compromised and contains them. Each job runs in a short-lived subprocess restricted by rlimits, no_new_privs, Landlock, and seccomp. That subprocess runs inside a worker container with `network_mode: none` that talks to the API only over a Unix socket, and libvips itself blocks every loader outside an allowlist. Twelve ADR drafts (0002–0013) are proposed. They include renaming `/api/v1/batches` to `/api/v1/jobs` and switching SLSA provenance to `actions/attest-build-provenance`. Four decisions need the owner's input: antivirus scanning (ASVS 5.4.3) versus the "no external services" rule, anonymous uploads, whether HEIC/HEVC decoding ships in the default image, and the libimagequant license. Because the March 2026 Trivy compromise and the 2025–2026 npm worms spread through mutable tags and fresh releases, SHA pinning and release-age delays are treated as required, not optional.

## 1. Executive summary: decisions awaiting approval

| # | Decision | Recommendation | ADR | Approval needed |
|---|---|---|---|---|
| A | Server language | Go 1.27.x (fallback 1.26.x) | 0002 | Confirm |
| B | libvips binding | vipsgen v2 (≥ v2.1.3) plus a Pixsheaf-owned cgo shim for untrusted-operation blocking; govips as fallback | 0003 | Confirm |
| C | Worker base and libvips build | libvips 8.18.x (and libheif) built from pinned source on `debian:trixie-slim`, with Debian-patched codecs underneath | 0004 | Confirm |
| D | API base image | `gcr.io/distroless/static-debian13:nonroot`, pinned by digest and verified with cosign | 0005 | Confirm |
| E | Worker isolation | One subprocess per job with rlimits, no_new_privs, Landlock, and seccomp; container limits as the backstop; gVisor as an opt-in compose override | 0006 | Confirm |
| F | Job transport | Unix socket on a shared tmpfs volume, owned by the API; workers dial in (pull model) and run with `network_mode: none` | 0007 | Confirm |
| G | libvips hardening | `VIPS_BLOCK_UNTRUSTED=1`, a loader/saver allowlist, header-first pixel and page checks, strict `fail_on`, cache off | 0008 | Confirm |
| H | Pipeline format | Declarative JSON validated by JSON Schema 2020-12 with hard limits | 0009 | Confirm |
| I | Frontend | Plain Svelte 5 + Vite SPA (not SvelteKit), Paraglide JS for i18n, TypeScript pinned to 6.0.x | 0010 | Confirm |
| J | OpenAPI tooling | oapi-codegen v2.8.2 for models and interfaces, hand-written streaming handlers, openapi-typescript and openapi-fetch on the client | 0011 | Confirm |
| K | Resource naming | `/api/v1/jobs` instead of `/batches` | 0012 | Confirm |
| L | Supply chain | SHA pinning, Actions lockfile, 7-day minimum release age, zizmor, harden-runner, `attest-build-provenance` | 0013 | Confirm |
| O1–O4 | Open questions | Antivirus scanning, anonymous uploads, HEIC in the default image, libimagequant license | §6 | **Owner input** |

## 2. Version matrix: what is current as of 2026-11-27

**Verification status legend.** **TAG** means read from the upstream git tag. The date shown is the tag's commit date, which usually matches the release date within a day but not always; GitHub release pages and the API could not be reached. **REG** means read from the npm registry's `latest` or `dist-tags` endpoint. **IDX** means read from the npm search index only, which was stale for other packages (it reported pnpm 12.8.1 while [npm dist-tags](https://registry.npmjs.org/-/package/pnpm/dist-tags) showed 12.23.0); re-verify these. **YR?** means the release page showed no year, so the year was inferred. **NV** means not verified.

### 2.1 Toolchain

| Component | Version | Date | Source | Status | Notes |
|---|---|---|---|---|---|
| Go | **1.27.4** | 2026-11-10 | [go.dev release history](https://go.dev/doc/devel/release) | TAG | Security release (net/http, cgo, go command, compiler, linker) |
| Go (fallback) | 1.26.11 | 2026-11-10 | [go.dev](https://go.dev/doc/devel/release) | TAG | Supported until Go 1.28 ships (expected about Feb 2027) |
| Node.js (build only) | **26.13.1** (Active LTS) | 2026-11-10 | [nodejs.org](https://nodejs.org/about/previous-releases), [schedule](https://github.com/nodejs/release#release-schedule) | REG | EOL 2029-04-30; the README label still says "Current", which is stale |
| pnpm | **12.23.0** | 2026-11-22 | [npm dist-tags](https://registry.npmjs.org/-/package/pnpm/dist-tags), [pnpm blog](https://pnpm.io/blog) | REG | 12.24.0 (2026-11-26) is on `next-12` and is younger than the 7-day release-age gate |
| TypeScript | **6.0.x (pin)**; 7.0.2 is `latest` | 6.0: 2026-03-23; 7.0: 2026-07-08 | [npm dist-tags](https://registry.npmjs.org/-/package/typescript/dist-tags), [TS blog](https://devblogs.microsoft.com/typescript/) | REG; **6.0 patch level NV** | Tooling cannot use TypeScript 7 yet (see §2.4) |
| Docker buildx | v0.39.1 | 2026-11-19 | [docker/buildx](https://github.com/docker/buildx/releases) | TAG | — |
| BuildKit | v0.35.1 | 2026-11-19 | [moby/buildkit](https://github.com/moby/buildkit/releases) | TAG | — |

### 2.2 Native image libraries (worker)

| Component | Version | Date | Source | Status | Notes |
|---|---|---|---|---|---|
| libvips | **8.18.7** | tag 2026-09-26 (the ChangeLog says 20 Sep) | [releases](https://github.com/libvips/libvips/releases), [ChangeLog](https://raw.githubusercontent.com/libvips/libvips/master/ChangeLog) | TAG | 8.19.0 is at rc3; the unreleased 8.18.8 adds total-height limits for multipage images |
| libheif | **1.23.7** | 2026-10-26 | [releases](https://github.com/strukturag/libheif/releases) | TAG | Seven patch releases since 1.23.0, many of them security fixes |
| libde265 | 1.1.3 | 2026-09-14 | [releases](https://github.com/strukturag/libde265/releases) | TAG | HEVC decoder; highest-risk component (see §3) |
| dav1d | 1.5.5 | 2026-11-16 | [code.videolan.org](https://code.videolan.org/videolan/dav1d) | TAG (via GitHub mirror) | AV1 decoder for AVIF |
| **libaom** | **NOT VERIFIED** | — | [aomedia.googlesource.com](https://aomedia.googlesource.com/aom) | **NV** | Host unreachable during research; also needed only if AVIF *encoding* uses aom |
| libjxl | 0.13.0 | 2026-11-11 | [releases](https://github.com/libjxl/libjxl/releases) | TAG | Release notes not read |
| libwebp | 1.6.0 | 2025-07-07 | [releases](https://github.com/webmproject/libwebp/releases) | TAG | No newer stable release |
| libjpeg-turbo | 3.2.1 | 2026-10-26 | [releases](https://github.com/libjpeg-turbo/libjpeg-turbo/releases) | TAG | Follows a CVE in 3.2.0 |
| libspng | 0.7.4 | 2023-05-08 | [releases](https://github.com/randy408/libspng/releases) | TAG | **Stale (more than 3 years without a release)**; libpng is the alternative |
| lcms2 | 2.20 | 2026-10-31 | [releases](https://github.com/mm2/Little-CMS/releases) | TAG | — |
| libexif | 0.6.26 | 2026-04-14 | [releases](https://github.com/libexif/libexif/releases) | TAG | CVEs not checked |
| cgif | 0.5.4 | 2026-09-02 | [releases](https://github.com/dloebl/cgif/releases) | TAG | GIF encoder; CVEs not checked |
| libimagequant | 4.4.1 | 2025-07-13 | [repo](https://github.com/ImageOptim/libimagequant) | TAG | More than 12 months old; **license needs a decision** (§6) |

### 2.3 Go modules and Go tooling

| Module | Version | Date | Source | Status | Notes |
|---|---|---|---|---|---|
| github.com/cshum/vipsgen/v2 | **v2.1.3** | 2026-11-21 | [cshum/vipsgen](https://github.com/cshum/vipsgen) | TAG | v2 API is new (2026-10-17, breaking) |
| github.com/davidbyttow/govips/v2 (fallback) | v2.20.0 | 2026-10-16 | [davidbyttow/govips](https://github.com/davidbyttow/govips) | TAG | — |
| github.com/pdfcpu/pdfcpu | v0.16.4 | 2026-10-27 | [releases](https://github.com/pdfcpu/pdfcpu/releases) | TAG | Pure Go |
| golang.org/x/sync (errgroup) | v0.25.0 | 2026-10-15 | [pkg.go.dev](https://pkg.go.dev/golang.org/x/sync) | TAG | — |
| github.com/oapi-codegen/oapi-codegen/v2 | v2.8.2 | 31 Oct | [releases](https://github.com/oapi-codegen/oapi-codegen/releases) | **YR?** (2026 inferred) | Initial OpenAPI 3.1 support since v2.8.0 |
| github.com/landlock-lsm/go-landlock | **NV** | — | [pkg.go.dev](https://pkg.go.dev/github.com/landlock-lsm/go-landlock/landlock) | **NV** | Official landlock-lsm binding; version not retrieved |
| Go seccomp library (elastic/go-seccomp-bpf or libseccomp-golang) | **NV** | — | — | **NV** | Maintenance status unverified |
| govulncheck (golang/vuln) | v1.10.0 | 2026-11-17 | [golang/vuln](https://github.com/golang/vuln) | TAG | — |
| golangci-lint | v2.15.0 | 2026-10-16 | [releases](https://github.com/golangci/golangci-lint/releases) | TAG | — |
| staticcheck | v0.8.2 | 2026-10-12 | [releases](https://github.com/dominikh/go-tools/releases) | TAG | Optional; reportedly part of golangci-lint v2 (unverified) |

### 2.4 Frontend

| Package | Version | Date | Source | Status | Notes |
|---|---|---|---|---|---|
| svelte | **5.59.2** | 2026-11-25 | [npm latest](https://registry.npmjs.org/svelte/latest) | REG | — |
| vite | **8.4.0** | 2026-11-26 | [npm search](https://registry.npmjs.org/-/v1/search?text=vite&size=5) | **IDX** | Bundles with Rolldown ([Vite 8](https://vite.dev/blog/announcing-vite8)) |
| @sveltejs/vite-plugin-svelte | 7.3.2 | 2026-11-23 | [npm search](https://registry.npmjs.org/-/v1/search?text=svelte&size=60) | **IDX** | — |
| tailwindcss / @tailwindcss/vite | **4.3.5** | **NV** | [npm latest](https://registry.npmjs.org/@tailwindcss/vite/latest) | REG (date NV) | Peer dependency accepts Vite ^8 |
| @inlang/paraglide-js | 3.2.2 | 2026-11-10 | [npm search](https://registry.npmjs.org/-/v1/search?text=paraglide&size=6) | **IDX** | Do **not** install the bare package `paraglide` (0.0.1 scaffold, typosquat risk) |
| typescript-eslint | 8.74.0 | 2026-11-23 | [npm latest](https://registry.npmjs.org/typescript-eslint/latest) | REG | Peer dependency `typescript <6.1.0` |
| svelte-check | 4.7.8 | 2026-11-01 | [npm latest](https://registry.npmjs.org/svelte-check/latest) | REG | Peer dependency `typescript ^5 \|\| ^6` |
| eslint | 10.14.0 | 2026-10-30 | [npm search](https://registry.npmjs.org/-/v1/search?text=eslint&size=5) | **IDX** | Flat config only |
| eslint-plugin-svelte | 3.24.1 | 2026-11-07 | [npm search](https://registry.npmjs.org/-/v1/search?text=svelte&size=60) | **IDX** | Use the `svelte/no-inline-styles` rule to protect the CSP |
| prettier (+ plugin-svelte 4.2.0) | 3.10.1 | 2026-11-12 | [npm search](https://registry.npmjs.org/-/v1/search?text=prettier&size=3) | **IDX** | — |
| openapi-typescript | 7.13.0 | 2026-02-11 | [npm search](https://registry.npmjs.org/-/v1/search?text=openapi-typescript&size=6) | **IDX** | — |
| openapi-fetch | 0.17.0 | 2026-02-11 | [npm search](https://registry.npmjs.org/-/v1/search?text=openapi-fetch&size=3) | **IDX** | Still 0.x; pin the exact version |
| @redocly/cli | 2.84.0 | 2026-11-26 | [npm search](https://registry.npmjs.org/-/v1/search?text=axe-core%20prettier%20redocly%20spectral-cli&size=8) | **IDX** | Spec lint (Spectral 6.17.1 is the alternative) |
| *Rejected:* @sveltejs/kit 3.4.1, svelte-i18n 4.0.1 (last release 2024-10-21, unmaintained) | — | — | [npm search](https://registry.npmjs.org/-/v1/search?text=svelte&size=60) | IDX | See ADR 0010 |

### 2.5 Testing

| Tool | Version | Date | Source | Status |
|---|---|---|---|---|
| Playwright | 1.64.0 | 2026-10-07 | [npm search](https://registry.npmjs.org/-/v1/search?text=playwright&size=5) | **IDX** |
| @axe-core/playwright / axe-core | 4.14.1 | 2026-11-24 / 2026-11-16 | [npm search](https://registry.npmjs.org/-/v1/search?text=axe-core%20prettier%20redocly%20spectral-cli&size=8) | **IDX** |
| Grafana k6 | v2.4.0 | 12 Nov | [k6 releases](https://github.com/grafana/k6/releases) | **YR?**; v2 removed several CLI commands and changed the module path to `go.k6.io/k6/v2` |
| Go fuzzing, testing | (Go toolchain) | — | [go.dev](https://go.dev/doc/devel/release) | TAG |

### 2.6 Containers and base images

| Image / runtime | Version | Date | Source | Status | Notes |
|---|---|---|---|---|---|
| `gcr.io/distroless/static-debian13:nonroot` | Debian 13 | — | [distroless README](https://github.com/GoogleContainerTools/distroless) | **Digest NV** | Signed with cosign keyless; supports amd64 and arm64 |
| `debian:trixie-slim` | Debian 13 | — | [tracker.debian.org](https://tracker.debian.org/pkg/vips) | **Point release and digest NV** | Stable ships vips **8.16.1** (security build deb13u2, 2026-10-11) |
| Debian trixie libheif | 1.23.6-2~deb13u1 | — | [tracker.debian.org/pkg/libheif](https://tracker.debian.org/pkg/libheif) | REG | **17 open security issues** in stable |
| Alpine (alternative) | 3.24; vips 8.18.2-r0 | branched 2026-06-09 | [alpinelinux.org](https://alpinelinux.org/releases/), [pkgs](https://pkgs.alpinelinux.org/packages?name=vips*&branch=v3.24&arch=x86_64) | REG | The community repo is supported only until the next stable release |
| Wolfi (alternative) | `libvips-8.17` (8.17.3) | — | [OSV CGA-349f-593f-g44r](https://osv.dev/vulnerability/CGA-349f-593f-g44r) | Partial | Affected by CVE-2026-35590; codec build flags NV |
| `cgr.dev/chainguard/static` (alternative) | `:latest` plus digest | — | [Chainguard tier guide](https://www.chainguard.dev/unchained/a-guide-on-how-to-use-chainguard-images-for-public-catalog-tier-users) | Policy page from 2023 | Free tier can pull only `latest` tags or digests |
| gVisor (runsc) | release-20261116.0 | 17 Nov | [gVisor releases](https://github.com/google/gvisor/releases) | **YR?** | Supports x86_64 and ARM64; requires Linux 5.6+ |

### 2.7 Supply-chain and CI tools (pin by the listed SHA, then re-verify)

| Tool / Action | Tag | Date | Commit SHA | Source | Status |
|---|---|---|---|---|---|
| actions/checkout | v6.0.2 | 2026-01-09 | de0fac2e4500dabe0009e67214ff5f5447ce83dd | [repo](https://github.com/actions/checkout) | TAG |
| docker/setup-buildx-action | v4.4.1 | 2026-09-16 | f87e5991a6d7451dcb8d9637bfbc97413f497069 | [repo](https://github.com/docker/setup-buildx-action) | TAG |
| docker/build-push-action | v7.4.0 | 2026-09-15 | c3c9e263c25d99ce0380d002d59b67737d91b0dc | [repo](https://github.com/docker/build-push-action) | TAG |
| Syft | v1.57.2 | 2026-11-16 | 1a72047fd46c72e0a12ee77e24b23d2316753d08 | [releases](https://github.com/anchore/syft/releases) | TAG |
| anchore/sbom-action | v0.24.4 | 2026-11-23 | dfcb83b97731532d72be422cbc51ee64adb6fd56 | [repo](https://github.com/anchore/sbom-action) | TAG |
| Grype | v0.122.1 | 2026-11-18 | 8f3ddeddc4a4ed6314248aaf0a7c510f98db8882 | [releases](https://github.com/anchore/grype/releases) | TAG |
| anchore/scan-action | v7.4.3 | 2026-11-23 | 11983a512817154c80a9db660c1a184be7513ed6 | [repo](https://github.com/anchore/scan-action) | TAG |
| Trivy | v0.76.2 | 2026-11-05 | 3f6334d223b52188324af3b9923b4249cd206652 | [releases](https://github.com/aquasecurity/trivy/releases) | TAG |
| aquasecurity/trivy-action | v0.36.0 | 2026-04-22 | ed142fd0673e97e23eac54620cfb913e5ce36c25 | [repo](https://github.com/aquasecurity/trivy-action) | TAG |
| aquasecurity/setup-trivy | v0.3.1 | 2026-06-03 | 81e514348e19b6112ce2a7e3ecbafe19c1e1f567 | [repo](https://github.com/aquasecurity/setup-trivy) | TAG |
| cosign | v3.1.6 | 2026-11-25 | 252f10a80ba871152a12e5ee324ad1e02c85d079 | [releases](https://github.com/sigstore/cosign/releases) | TAG |
| sigstore/cosign-installer | v4.1.4 | 2026-11-24 | 881c63bd87e38389154debd8331404ce7ca8a3ab | [repo](https://github.com/sigstore/cosign-installer) | TAG |
| actions/attest-build-provenance | v4.2.2 | 2026-08-06 | 4d101475d8b20a2381f78447822ac1eab6504dd8 | [repo](https://github.com/actions/attest-build-provenance) | TAG |
| actions/attest | v4.2.2 | 2026-08-04 | 1e69f48acb82d1966a394da916b4c1698aa569d6 | [repo](https://github.com/actions/attest) | TAG |
| *slsa-github-generator (rejected)* | v2.1.0 | **2025-02-24** | f7dd8c54c2067bafc12ca7a55595d5ee9b75204a | [repo](https://github.com/slsa-framework/slsa-github-generator) | TAG; stale |
| golangci-lint-action | v9.3.0 | 2026-06-29 | ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a | [repo](https://github.com/golangci/golangci-lint-action) | TAG |
| hadolint / hadolint-action | v2.15.1 / v3.5.0 | 2026-07-31 / 2026-08-24 | 2eece55955ced00200be9729e9728cb7dacca505 / 06be81baf89a55ffd0e24b8f04a4185738dd3387 | [hadolint](https://github.com/hadolint/hadolint/releases) | TAG |
| github/codeql-action | v4.39.5 | 2026-11-24 | 915412ff23e8b7659517e607487dbeae018f2934 | [repo](https://github.com/github/codeql-action) | TAG |
| Gitleaks / gitleaks-action | v8.30.1 / v3.0.0 | 2026-03-12 / 2026-05-30 | 83d9cd684c87d95d656c1458ef04895a7f1cbd8e / e0c47f4f8be36e29cdc102c57e68cb5cbf0e8d1e | [gitleaks](https://github.com/gitleaks/gitleaks/releases) | TAG; whether organizations need a license key for the action is NV |
| ossf/scorecard-action | v2.6.0 | 2026-11-19 | 23f22cd43608bae7a1f1fa2603d7314dee9538a5 | [repo](https://github.com/ossf/scorecard-action) | TAG |
| step-security/harden-runner | v2.23.0 | 2026-11-17 | aebdcf6156ab6691adf63218240e476564a1ed4c | [repo](https://github.com/step-security/harden-runner) | TAG |
| zizmor / zizmor-action | v1.33.1 / v0.9.2 | 2026-11-22 | 0af0be985a4cae71ecfaa64ef574a94b35dc722a / 1b4550e929643b8b26a62303c6bd64814271a4f1 | [zizmor](https://github.com/zizmorcore/zizmor/releases) | TAG |
| Renovate | 45.1.1 | 2026-11-27 | 94efe07acdf49af738d411e4aa25ea3d9ad80820 | [releases](https://github.com/renovatebot/renovate/releases) | TAG; same-day release, so it is blocked by the release-age rule anyway |

**Not verified at all:** libaom (version, date, and CVEs), the current digests of every base image, the Debian 13 point release, the Wolfi libvips 8.18 stream and its codec flags, go-landlock and the Go seccomp library versions, the TypeScript 6.0.x patch level, the Tailwind 4.3.5 release date, and all licenses except libheif (LGPL) and the Go bindings (MIT).

## 3. Decoders, not the API, carry the security risk

**The HEIF/HEVC chain has the most CVEs by far.** Debian lists about 40 libheif CVEs for 2025–2026. One of them, CVE-2026-120208, is a heap overflow fixed in **1.23.3** that Red Hat titles as arbitrary code execution ([Debian libheif](https://security-tracker.debian.org/tracker/source-package/libheif), [Red Hat 2565445](https://bugzilla.redhat.com/show_bug.cgi?id=2565445)). In libde265, CVE-2026-49346 lets a crafted SPS make the decoder allocate about 1 KB and then write about 4 GB (CVSS 7.1, fixed in 1.1.0) ([OSV](https://osv.dev/vulnerability/CVE-2026-49346)). Grid and overlay items amplify dimensions: CVE-2025-68431 is an `iovl` overlay over-read fixed in libheif 1.21.0 ([OSV](https://osv.dev/vulnerability/CVE-2025-68431)). **libvips itself** received hardening in every 8.18 point release. CVE-2026-33327 is a heap overflow in `vipsload` fixed in 8.18.1. Release 8.18.6 marks heifload as untrusted when libvips is built against libheif < 1.23.2, and 8.18.7 does the same for jp2ksave ([Debian CVE-2026-33327](https://security-tracker.debian.org/tracker/CVE-2026-33327), [ChangeLog](https://raw.githubusercontent.com/libvips/libvips/master/ChangeLog)). The operation-blocking API Pixsheaf depends on had its own bug: CVE-2025-59933 is an over-read in `vips_operation_block_set` in libvips ≤ 8.17.1 ([VulDB](https://vuldb.com/vuln/326340)). **libjxl** CVE-2026-1837 writes out of bounds only when lcms2 is the color management engine, and it is fixed in 0.13.0 ([Debian](https://security-tracker.debian.org/tracker/CVE-2026-1837)). **lcms2** CVE-2026-42798 is fixed in 2.19, and **libjpeg-turbo** CVE-2026-75466 affects 3.2.0 ([lcms2](https://security-tracker.debian.org/tracker/source-package/lcms2), [libjpeg-turbo](https://security-tracker.debian.org/tracker/source-package/libjpeg-turbo)). libwebp's CVE-2023-4863 is the precedent showing these bugs get exploited: it is in CISA's Known Exploited Vulnerabilities catalog ([OSV](https://osv.dev/vulnerability/CVE-2023-4863)).

**Distribution packages lag behind these fixes.** Debian trixie ships vips 8.16.1 and still has 17 open libheif issues ([vips](https://tracker.debian.org/pkg/vips), [libheif](https://tracker.debian.org/pkg/libheif)). That gap is why ADR 0004 builds libvips from source.

**The bindings are attack surface too.** In October and November 2026, vipsgen fixed freeing a blob that libvips owns (#171) and narrowing blob lengths to 32 bits (#178) ([vipsgen commits](https://github.com/cshum/vipsgen/commits/main)). In September 2026, govips fixed three cgo bugs, including a double unref (#549) ([govips commits](https://github.com/davidbyttow/govips/commits/master)). A large or crafted input can reach both kinds of bug, so the binding runs inside the sandbox. OpenAPI codegen has a similar history: oapi-codegen v2.7.1 through v2.8.2 fixed Go code injection in generated code ([releases](https://github.com/oapi-codegen/oapi-codegen/releases)).

**Mutable tags and fresh releases are the supply-chain attack vector.** In March 2026 the Trivy ecosystem was compromised (CVE-2026-33634). Attackers published malicious Trivy v0.69.4 binaries and images, force-pushed `trivy-action` tags 0.0.1–0.34.2, and replaced `setup-trivy` tags. Users who referenced images by digest or built from source were not affected ([GHSA-69fq-xp46-6x23](https://github.com/aquasecurity/trivy/security/advisories/GHSA-69fq-xp46-6x23)). The malicious artifacts were available for only hours. On 2026-05-11, "Mini Shai-Hulud" poisoned the pnpm cache through a `pull_request_target` workflow and stole OIDC tokens. It published 84 malicious artifacts across 42 `@tanstack/*` packages **with valid provenance**, ran a `prepare` hook from an optional git dependency, and persisted through `.claude/settings.json` (CVE-2026-45321). That followed Shai-Hulud (September 2025, more than 500 packages) and Sha1-Hulud 2.0 (November 2025) ([Snyk](https://snyk.io/blog/tanstack-npm-packages-compromised)). Three conclusions follow. Provenance alone does not establish trust. A release-age delay of several days would have avoided every one of these incidents. And `pull_request_target` must never be used. GitHub now blocks it by default for public repositories from **2026-12-02** ([changelog](https://github.blog/changelog/2026-11-02-pull_request_target-blocked-by-default-december-2)).

## 4. STRIDE threat model

### 4.1 Assets

| ID | Asset | Why it matters |
|---|---|---|
| A1 | Uploaded images and their metadata (EXIF GPS, XMP) | Personal data under the Swiss FADP and the EU GDPR; confidentiality between clients |
| A2 | Processed outputs and download tokens (job IDs ≥ 128 bits) | Anyone holding a token can retrieve the output |
| A3 | Worker and host integrity | A decoder RCE is the most likely compromise |
| A4 | Service availability (CPU, RAM, queue) | Pixel, ZIP, and frame bombs; queue flooding |
| A5 | Logs and metrics | Must not contain file contents, original names, or plaintext IPs |
| A6 | Release artifacts (images, SBOM, signatures, CI secrets, OIDC identity) | Supply-chain attacks against every downstream self-hoster |

### 4.2 Trust boundaries

| TB | Boundary | Crossing | Notes |
|---|---|---|---|
| TB1 | Browser → reverse proxy | HTTPS | Untrusted client; anonymous by default (open decision O2) |
| TB2 | Reverse proxy → API | HTTP on an internal network or 127.0.0.1 | `X-Forwarded-For` is trusted only from `PIXSHEAF_TRUSTED_PROXIES` |
| TB3 | API → worker socket | Pathname Unix socket on a shared tmpfs volume | The API is the only party that parses ZIP and HTTP; workers have no network |
| TB4 | Worker supervisor → per-job child | fork/exec, pipes | The child applies rlimits, no_new_privs, Landlock, and seccomp before it decodes anything |
| TB5 | Child Go code → libvips and codecs (cgo) | In-process C | **The real hostile parse.** Treated as compromised |
| TB6 | CI → registry → self-hoster | Signed OCI images | cosign signature, SBOM, and provenance verified by digest |

### 4.3 Threats and mitigations

ASVS references are to **OWASP ASVS 5.0 chapter V5 (File Handling)** ([ASVS 5.0 V5](https://github.com/OWASP/ASVS/blob/master/5.0/en/0x14-V5-File-Handling.md)). Requirements not in V5 map to sections of the project instructions. Requirement IDs were paraphrased from the source; re-check the exact wording before quoting them in `docs/security.md`.

**Spoofing**

| ID | Threat | Mitigation | Requirement / ASVS |
|---|---|---|---|
| S1 | A client spoofs `X-Forwarded-For` to evade per-IP limits | Honor XFF only from configured trusted proxies; otherwise use the socket peer address | Project §HTTP (rate limiting) |
| S2 | Cross-site request starts jobs (CSRF) | Same-origin check plus a CSRF token on state-changing requests; CORS off | Project §HTTP |
| S3 | A compromised worker impersonates another worker, or a fake process listens on the socket | The API owns the single socket in pull mode; socket mode 0660, matching UID ≥ 10000; volume mounted `noexec,nosuid,nodev`; per-worker subdirectories if workers listen | ADR 0007 |
| S4 | A tampered base image or action is substituted | Pin by digest and SHA; verify distroless with cosign (`keyless@distroless.iam.gserviceaccount.com`) ([distroless](https://github.com/GoogleContainerTools/distroless)) | Project §Supply chain; ADR 0013 |

**Tampering**

| ID | Threat | Mitigation | Requirement / ASVS |
|---|---|---|---|
| T1 | Zip Slip (`../`, absolute paths, symlinks) in an uploaded ZIP | Parse ZIPs only in the API with pure-Go `archive/zip`; ignore entry paths and use random internal IDs; reject symlink entries and nested archives | **5.3.2, 5.3.3 (L3), 5.2.5 (L3)** |
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
| E1 | RCE in a decoder (libheif, libde265, libjxl, libwebp-class bugs) | Allowlist of loaders plus `VIPS_BLOCK_UNTRUSTED`; patched versions (libheif ≥ 1.23.7, libvips ≥ 8.18.7); a per-job sandbox; no ImageMagick, PDF, SVG, OpenSlide, FITS, or Matlab loaders compiled in | Project §Input; ADR 0004/0008 |
| E2 | Escape from the child into the container | no_new_privs, Landlock limited to read-only libraries and a read-write per-job directory, seccomp (no `execve` after setup), RLIMIT_CORE=0 | ADR 0006 |
| E3 | Escape from the container to the host | Non-root UID ≥ 10000, `cap_drop: [ALL]`, read-only root filesystem, a custom seccomp profile, optional gVisor ([gVisor](https://raw.githubusercontent.com/google/gvisor/master/README.md)) | Project §Containers |
| E4 | Uploaded files execute as code | No writable executable paths; tmpfs mounted `noexec`; files are never placed in a served directory | **5.3.1** |
| E5 | A malicious OpenAPI spec injects code during codegen | The spec is written by the project; oapi-codegen pinned to ≥ v2.8.2 | ADR 0011 |

**Coverage of ASVS V5 at L2:** 5.1.1 (documentation, in `docs/configuration.md` and `docs/security.md`), 5.2.1–5.2.3, 5.3.1–5.3.2, and 5.4.1–5.4.2 are covered. **5.4.3 (antivirus scanning) is open** (O1). Pixsheaf voluntarily adopts the L3 items 5.2.4, 5.2.5, 5.2.6, and 5.3.3 ([ASVS 5.0 V5](https://github.com/OWASP/ASVS/blob/master/5.0/en/0x14-V5-File-Handling.md)). Mapping documents that use ASVS 4.0.3 numbering (V12) do not apply ([ASVS 4.0.3 V12](https://raw.githubusercontent.com/OWASP/ASVS/v4.0.3/4.0/en/0x20-V12-Files-Resources.md)).

## 5. ADR drafts 0002–0013

### ADR 0002: Server language

**Status:** Proposed
**Context:** The project instructions default to Go and ask for an ADR on Rust (axum, tokio). The decoders are C either way, so memory safety at the API layer only partly reduces risk. Go 1.27.4 is current, and Go 1.26 is supported until 1.28 ships ([go.dev](https://go.dev/doc/devel/release)).
**Options:** (a) Go with standard-library `net/http`. (b) Rust with axum and tokio.
**Decision:** Go 1.27.x, using `net/http` pattern routing, `log/slog`, and `x/sync/errgroup`.
**Consequences:** Static API binary with `CGO_ENABLED=0`. The worker needs cgo for libvips. The best-maintained libvips binding (vipsgen) and its production reference (imagor) are both Go ([imagor](https://github.com/cshum/imagor)). Rust's libvips bindings were **not evaluated** in this phase, so the decision rests on ecosystem fit and the project's default, not on a head-to-head comparison. Revisit if the worker's cgo layer causes recurring memory-safety incidents. Renovate tracks the Go toolchain version.

### ADR 0003: libvips Go binding

**Status:** Proposed
**Context:** The worker needs AVIF, JPEG XL, HEIF, compositing, ICC transforms, and saving to a target size.

| | vipsgen v2 | govips v2 | `vips` CLI subprocess |
|---|---|---|---|
| Latest | v2.1.3 (2026-11-21) | v2.20.0 (2026-10-16) | Same as libvips |
| Commits in the last 12 months | 113 | 63 | — |
| Coverage | About 300 generated operations | 193+ generated operations plus hand-written I/O | One operation per process |
| `fail_on` / access mode | Every loader | Yes | Flags |
| Defaults | Cache 0, concurrency 1, vector off | Non-zero cache and concurrency | — |
| Block untrusted operations | Needs a shim | Needs a shim | Environment variable |
| Production use | imagor | Many users | — |

Sources: [vipsgen](https://github.com/cshum/vipsgen), [govips](https://github.com/davidbyttow/govips), [imagor](https://github.com/cshum/imagor). bimg was rejected because its last tag was in 2022 and it has had no commits in 12 months ([bimg](https://github.com/h2non/bimg)). vipsx was rejected because it was written in 4 days by one author and has been inactive since 2026-08 ([vipsx](https://github.com/kirklin/vipsx)).
**Decision:** Use vipsgen v2 (`github.com/cshum/vipsgen/v2/vips`, pinned at ≥ v2.1.3) against libvips 8.18.x. Add about 10 lines of cgo shim in `internal/imaging` that calls `vips_operation_block_set` and `vips_block_untrusted_set`. Keep govips as the documented fallback and the CLI approach as an emergency fallback.
**Consequences:** Both viable bindings depend on a single maintainer. The mitigation is that vipsgen is generated and MIT-licensed, so it can be forked and regenerated. The v2 API is new, so expect churn. Both bindings recently shipped memory fixes, so the binding stays inside the sandbox. Before approval, check the open-issue backlog by hand, because the GitHub API was unreachable during research.

### ADR 0004: libvips build and worker base image

**Status:** Proposed
**Context:** Upstream is libvips 8.18.7 and libheif 1.23.7. Debian trixie ships 8.16.1 with 17 open libheif issues. Alpine 3.24 ships 8.18.2, but its community repo is supported only until the next release. Wolfi ships `libvips-8.17` ([Debian vips](https://tracker.debian.org/pkg/vips), [Alpine](https://alpinelinux.org/releases/), [OSV CGA](https://osv.dev/vulnerability/CGA-349f-593f-g44r)).
**Options:** (a) Build libvips and libheif from pinned source on `debian:trixie-slim`, linking Debian's codecs underneath. (b) Alpine 3.24 packages. (c) Wolfi packages. (d) Debian packages only.
**Decision:** Option (a). Pin source tarballs by SHA-256 with `ARG LIBVIPS_VERSION` and `ARG LIBHEIF_VERSION`, tracked by Renovate custom regex managers. Configure meson to disable magick, poppler, pdfium, rsvg, openslide, matio, cfitsio, and OpenEXR. Do not install `libheif-plugin-x265` (HEIC is input only). Use jemalloc on glibc ([checklist](https://www.libvips.org/API/current/developer-checklist.html)). Pin the runtime to `debian:trixie-slim` by digest and rebuild weekly.
**Consequences:** Pixsheaf gets libvips CVE fixes the day they ship, and Debian's security team keeps patching the low-level codecs. The cost is that the project tracks libvips and libheif CVEs itself, and builds take longer (cache them with BuildKit). Debian trixie's libjpeg-turbo is 2.1.5 and its lcms2 is 2.16; lcms2 2.16 is in the range affected by CVE-2026-42798 unless Debian backported the fix ([lcms2](https://security-tracker.debian.org/tracker/source-package/lcms2)). **Phase 2 must check the Debian security status of lcms2, libjxl, and libjpeg-turbo, and build them from source where they are unpatched.** Choose between libspng (stale since 2023) and libpng here; the recommendation is libpng unless benchmarks show a material gap. Re-evaluate Wolfi once it ships libvips 8.18 with verified codec flags.

### ADR 0005: API base image

**Status:** Proposed
**Options:** `gcr.io/distroless/static-debian13:nonroot`; `cgr.dev/chainguard/static:latest@digest`; `scratch` plus a CA bundle.
**Decision:** Use distroless `static-debian13:nonroot`, always with the explicit `-debian13` suffix because the unsuffixed default "may change", pinned by digest and verified with cosign in CI ([distroless](https://github.com/GoogleContainerTools/distroless)).
**Consequences:** No shell and no package manager. The health check is a subcommand of the binary itself. The image is multi-arch. This avoids depending on Chainguard's tier policy, which limits free users to `latest` tags and digests ([Chainguard](https://www.chainguard.dev/unchained/a-guide-on-how-to-use-chainguard-images-for-public-catalog-tier-users)). The `nonroot` UID is commonly 65532 (not verified). Set `USER 10001:10001` explicitly to satisfy the UID ≥ 10000 rule deterministically.

### ADR 0006: Worker isolation

**Status:** Proposed
**Context:** The requirement is that a plain `docker compose up` works without privileged containers. nsjail's documented Docker usage needs `--privileged` or unprivileged user namespaces ([nsjail](https://raw.githubusercontent.com/google/nsjail/master/README.md)). Landlock works without privileges once no_new_privs is set, starting with Linux 5.13 ([Landlock](https://docs.kernel.org/userspace-api/landlock.html)).
**Options:** (a) A per-job subprocess with rlimits, no_new_privs, Landlock, and seccomp. (b) nsjail or bubblewrap. (c) Per-call `vips` CLI processes. (d) gVisor by default. (e) Kata, Firecracker, or WASM.
**Decision:** Option (a). The worker supervisor fork/execs `pixsheaf-worker job` once per file. The child sets RLIMIT_AS (or RLIMIT_DATA, because Go's address-space reservations make RLIMIT_AS awkward), RLIMIT_CPU, RLIMIT_FSIZE, RLIMIT_NOFILE, and RLIMIT_CORE=0, then sets no_new_privs. It then applies a best-effort Landlock ruleset: libraries read-only, its own job directory read-write, TCP denied, and abstract Unix sockets scoped (using go-landlock, with ABI 8 TSYNC or apply-before-threads handling for Go). Finally it installs a seccomp filter: no `socket`, `connect`, or `execve` after setup, and `ERRNO` with logging as the default action rather than kill. The supervisor kills the process group on timeout. Each container runs N = 1–2 jobs, and container-level `mem_limit`, `cpus`, and `pids_limit` act as the hard backstop. Ship gVisor as an opt-in `compose.gvisor.yaml` that sets `runtime: runsc`. gVisor supports amd64 and arm64, and CPU-bound work runs at near-native speed ([gVisor perf](https://gvisor.dev/docs/architecture_guide/performance/)).
**Consequences:** Crashes and leaks die with the child process, which also contains GLib fragmentation ([govips README](https://github.com/davidbyttow/govips)). On older kernels Landlock degrades gracefully. RLIMIT_NPROC applies per UID across the container, not per job. Per-job cgroups would need cgroup delegation, which was not verified, so they are deferred. Kata and Firecracker require KVM and are out of scope. WASM (wasm-vips with wazero) is a future research option. The custom seccomp profile needs a CI corpus run on every libvips, glibc, or Go upgrade.

### ADR 0007: Job transport

**Status:** Proposed
**Options:** (a) A Unix socket on a shared tmpfs volume, with workers using `network_mode: none`. (b) An `internal: true` bridge network using HTTP or gRPC.
**Decision:** Option (a), in a pull model. The API owns one socket (for example `/run/pixsheaf/api.sock`) on a named volume with `driver_opts: type=tmpfs` mounted `noexec,nosuid,nodev`. Workers connect to it and fetch jobs. Payloads are streamed over the socket using HTTP/1.1. Both images run as the same UID (≥ 10000) and the socket has mode 0660 ([UDS via volume, secondary source](https://oneuptime.com/blog/post/2026-02-08-how-to-set-up-docker-containers-with-unix-socket-communication/markdown)).
**Consequences:** Workers have no IP address on any network, which is a stronger guarantee than option (b). Workers expose no listener, so `--scale worker=n` needs no service discovery. Pathname sockets cross network namespaces by design. The per-job child must be blocked from the socket through Landlock ABI 9 `FS_RESOLVE_UNIX` or by closing the descriptor before it decodes anything. **No primary source confirms that `network_mode: none` works with a Unix-socket volume, or how this behaves under gVisor or rootless Docker.** This needs a Phase 2 container test.

### ADR 0008: libvips hardening configuration

**Status:** Proposed
**Decision:**

| Control | Setting | Source |
|---|---|---|
| Untrusted operations | `VIPS_BLOCK_UNTRUSTED=1` in the worker environment, plus a shim call to `vips_block_untrusted_set(TRUE)` | [API docs](https://www.libvips.org/API/current/func.block_untrusted_set.html) |
| Loader allowlist | `vips_operation_block_set("VipsForeignLoad", TRUE)`, then unblock jpeg, png, webp, gif, tiff, bmp path, heif/avif (only with libheif ≥ 1.23.2), and jxl; same approach for savers (no jp2k) | [What's new in 8.13](https://www.libvips.org/2022/05/28/What's-new-in-8.13.html) |
| Minimum version | libvips ≥ 8.18.7 (8.17.2 is the floor because of CVE-2025-59933) | [ChangeLog](https://raw.githubusercontent.com/libvips/libvips/master/ChangeLog) |
| Decode strictness | `fail_on=error` by default (`warning` in strict mode); never `unlimited` | [checklist](https://www.libvips.org/API/current/developer-checklist.html), [sharp security](https://sharp.pixelplumbing.com/security) |
| Bomb checks | Load the header only, then check width, height, pages, and total canvas size; start from imagor's defaults (9999 × 9999 px, 81 MP) and make them configurable | [imagor](https://github.com/cshum/imagor/tree/master/processor/vipsprocessor) |
| libheif | Keep `heif_security_limits` (never set `LIBHEIF_SECURITY_LIMITS=off`) | [libheif README](https://raw.githubusercontent.com/strukturag/libheif/master/README.md) |
| Cache and concurrency | `vips_cache_set_max(0)`, concurrency 1 per job (vipsgen's defaults) | [vipsgen util.go](https://github.com/cshum/vipsgen/blob/main/vips/util.go) |
| Access mode | `access=sequential` where the pipeline allows it | [vipsgen](https://github.com/cshum/vipsgen) |

**Consequences:** At build time, generate the list of operations marked untrusted (`vips -l | grep -i untrusted`) and commit it to the ADR. The names of the `fail_on` levels must be confirmed against the 8.18 API documentation.

### ADR 0009: Pipeline format

**Status:** Proposed
**Context:** Presets, the UI, and the API all need the same format, and the result must be byte-identical whether a preset runs from the UI or the API.
**Options:** (a) Declarative JSON validated by JSON Schema 2020-12. (b) A custom DSL. (c) Query parameters in the style of imagor's URLs.
**Decision:** Option (a). A pipeline has the shape `{"version":1,"steps":[{"op":"resize","params":{...},"when":{...}}]}` and is validated against a closed schema (`additionalProperties:false`) that is embedded in the OpenAPI 3.1 spec. Conditions come from a fixed vocabulary (`width_gt`, `source_format`, `orientation`) with no expressions. Limits are configurable: maximum number of steps, maximum output dimensions and pixels, maximum watermark size and bytes, and maximum rename-pattern length. The server re-validates every request regardless of what the UI did.
**Consequences:** One validator serves presets, the UI, and the API. Presets can be exported and imported as plain JSON. Adding new operations requires bumping the schema version. Fuzz the parser.

### ADR 0010: Frontend stack

**Status:** Proposed
**Context:** The UI needs a strict CSP without `unsafe-inline`, has one or two screens, and is embedded with `go:embed`. SvelteKit with adapter-static emits an inline bootstrap script that needs hashes, and delivers its CSP through a `<meta>` tag where `frame-ancestors` is ignored ([SvelteKit config](https://svelte.dev/docs/kit/configuration)).
**Options:** (a) A plain Svelte 5 + Vite 8 SPA. (b) SvelteKit 3 with adapter-static.
**Decision:** Option (a), with Tailwind 4.3.5 through `@tailwindcss/vite`, component CSS emitted to files (never `css: 'injected'`), and the `svelte/no-inline-styles` lint rule. Use Paraglide JS 3.x for English and German instead of svelte-i18n, which has been unmaintained since 2024. Pin TypeScript `~6.0.x`, because typescript-eslint 8.74.0 requires `<6.1.0` and svelte-check 4.7.8 requires `^5 || ^6` ([typescript-eslint](https://registry.npmjs.org/typescript-eslint/latest), [svelte-check](https://registry.npmjs.org/svelte-check/latest)). Use pnpm 12.23.0 through `packageManager`, and Node 26 LTS at build time only.
**Consequences:** The Go server can send `script-src 'self'; style-src 'self'` as headers. Svelte 5 transitions use the Web Animations API and need no inline styles ([svelte#6662](https://github.com/sveltejs/svelte/issues/6662)). Revisit TypeScript 7 when 7.1 is stable and the tools update their peer ranges. Not yet verified: that Paraglide uses no `eval`, and Vite 8's modulepreload polyfill behavior.

### ADR 0011: OpenAPI tooling

**Status:** Proposed
**Options:** oapi-codegen; ogen; hand-written code only.
**Decision:** oapi-codegen v2.8.2 generates models and the strict `net/http` server interface. OpenAPI 3.1 support has existed since v2.8.0 but is self-described as "initial" ([releases](https://github.com/oapi-codegen/oapi-codegen/releases)). Upload, SSE, and ZIP-download handlers are hand-written against the generated interface. On the client, openapi-typescript 7.x generates types only, and openapi-fetch 0.17 is pinned exactly. CI lints the spec with Redocly CLI.
**Consequences:** No router dependency. ogen was rejected because it generates SSE clients only and buffers JSON by default ([ogen](https://github.com/ogen-go/ogen)). Keep the spec within the 3.1 subset oapi-codegen supports, and fall back to 3.0-compatible constructs if generation fails. Pin oapi-codegen at or above v2.8.2 because of the earlier code-injection fixes.

### ADR 0012: API resource naming

**Status:** Proposed
**Context:** `POST /api/v1/batches` also serves single images, which contradicts the rule that the product leads with the single-image workflow.
**Options:** `/jobs`; `/tasks`; `/sets`; keep `/batches`.
**Decision:** Rename to `/api/v1/jobs`: `POST /jobs` creates a job, `PUT /jobs/{id}/files` uploads files, `GET /jobs/{id}/events` streams progress over SSE, `GET /jobs/{id}/download` returns a single file when there is one file and a ZIP otherwise, and `DELETE /jobs/{id}` cancels. Internally each file is a "task" (`/jobs/{id}/files/{fileId}` for per-file status and cancellation). Defer any one-shot convenience endpoint (`POST /api/v1/process`) to a later ADR, and allow it only if it reuses the same validation and worker path.
**Consequences:** The names are neutral, and the UI never needs the word "batch". "Job" now means the whole set and "task" means one file, so the glossary in `docs/api.md` must state this explicitly.

### ADR 0013: Supply-chain hardening

**Status:** Proposed
**Decision:**

| Control | Setting | Source |
|---|---|---|
| Action pinning | Full commit SHAs, the repository policy "require SHA pinning", and `.github/workflows/actions.lock` (generally available since 2026-10-28, also pins transitive dependencies) | [GitHub changelog](https://github.blog/changelog/2025-08-15-github-actions-policy-now-supports-blocking-and-sha-pinning-actions/), [InfoQ](https://infoq.com/news/2026/11/github-actions-lockfiles-ga) |
| Image pinning | `tag@sha256:…` in Dockerfiles and compose files; Renovate `config:best-practices` and `default:automergeDigest` | [Renovate Docker](https://docs.renovatebot.com/docker/) |
| Release age | Renovate `minimumReleaseAge: "7 days"` and pnpm `minimumReleaseAge: 10080`, with documented exceptions for security fixes | [Snyk](https://snyk.io/blog/tanstack-npm-packages-compromised), [pnpm 11](https://pnpm.io/blog/releases/11.0) |
| pnpm | `--frozen-lockfile`, `blockExoticSubdeps: true`, `strictDepBuilds: true` with an explicit `allowBuilds`, `engineStrict`, `pnpm audit` | [pnpm 11](https://pnpm.io/blog/releases/11.0) |
| Workflows | No `pull_request_target`; top-level `permissions: contents: read` with per-job grants; `persist-credentials: false`; zizmor in CI; harden-runner with an egress allowlist; caches scoped per workflow | [changelog 2026-11-02](https://github.blog/changelog/2026-11-02-pull_request_target-blocked-by-default-december-2) |
| Scanners | Grype as the primary gate, Trivy as a secondary gate installed from a cosign-verified release; govulncheck, CodeQL, Gitleaks, Scorecard | [GHSA-69fq](https://github.com/aquasecurity/trivy/security/advisories/GHSA-69fq-xp46-6x23) |
| Provenance | `actions/attest-build-provenance` v4.2.2 (push to GHCR) instead of `slsa-github-generator`, whose last release was 2025-02-24; SBOM from Syft attached with `actions/attest`; cosign keyless signing by digest | [attest-build-provenance](https://github.com/actions/attest-build-provenance), [slsa-github-generator](https://github.com/slsa-framework/slsa-github-generator) |

**Consequences:** Updates arrive about a week later. Whether Renovate can maintain `actions.lock` files is unknown; keep explicit SHA pins as the primary control. TanStack-style payloads target `.claude/settings.json` and `.vscode/tasks.json`, so add those paths to CODEOWNERS review.

## 6. Open decisions requiring the owner's input

| # | Question | Options | Recommendation |
|---|---|---|---|
| **O1** | ASVS 5.4.3 (L2) requires antivirus scanning before serving files; this conflicts with "no external services" | (a) A deviation ADR: the original bytes are never served, every output is re-encoded by a sandboxed decoder, and no files are stored. (b) An optional ClamAV sidecar (local, no cloud) behind `PIXSHEAF_AV_SCAN=clamd://…`, off by default. (c) ClamAV on by default | **(a) plus (b) as an opt-in.** Antivirus adds little against re-encoded raster output, and OWASP itself says rewriting is necessary but not sufficient ([cheat sheet](https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html)). ClamAV on by default would add a large parser and signature updates that need network egress |
| **O2** | The OWASP cheat sheet says "authenticate before upload", but v1.0 accepts anonymous uploads | (a) Anonymous, with compensating controls. (b) Mandatory forward auth or OIDC in v1.0 | **(a)**, with compensating controls: bind to 127.0.0.1 by default, per-IP rate limits and job caps, a global queue cap with 429 responses, one-time downloads, short TTLs, and documented forward-auth examples for Traefik, Caddy, and nginx. Record the residual risk in the threat model |
| **O3** | HEIC/HEVC decoding through libde265 has the highest CVE count and unclear patent status (libheif's README is silent on patents) ([libheif README](https://raw.githubusercontent.com/strukturag/libheif/master/README.md)) | (a) Include it in the default worker. (b) Ship a separate `pixsheaf-worker-heic` variant with the libde265 plugin loaded through `LIBHEIF_PLUGIN_PATH`. (c) No HEIC support | **(b).** The default image keeps AVIF (dav1d) but leaves out HEVC; the HEIC variant is opt-in, documented, and runs under the same sandbox. This isolates both the legal question and the riskiest decoder. Feature parity with Reubah (HEIC input) is then available but not the default. Get legal advice if the owner wants HEIC in the default image |
| **O4** | libimagequant is reportedly GPL-3.0 or commercial (from prior knowledge, **unverified**); libvips uses it for quantizing GIF and PNG palettes | (a) Do not link it; use cgif without libimagequant quantization, or quantizr. (b) Link it and accept GPL-3.0 for the worker image. (c) Buy a commercial license | **(a)** for v1.0, so that Pixsheaf's MIT licensing story stays simple. Benchmark GIF and palette-PNG quality in Phase 3. Verify every codec license against its LICENSE file before release |

## 7. Verification gaps to close at the start of Phase 2

| Gap | Action |
|---|---|
| Base image digests (`distroless/static-debian13:nonroot`, `debian:trixie-slim`, the Go builder, the Node builder) | `crane digest` or `docker buildx imagetools inspect`; verify distroless with cosign; let Renovate pin them |
| Versions read from tag commit dates or the stale npm index | Re-check on the release pages and with `pnpm view <pkg> version time --json` (Vite, Playwright, ESLint, Paraglide, openapi-typescript, openapi-fetch, Redocly, Prettier, eslint-plugin-svelte); confirm the k6, oapi-codegen, and gVisor years |
| TypeScript 6.0.x patch level and Tailwind 4.3.5 release date | Read the npm `time` field |
| libaom | Read `https://aomedia.googlesource.com/aom/+refs` and its 2025–2026 CVEs; decide whether to use aom for AVIF encoding or another encoder |
| CVE fix versions | Map the libvips, libheif, and libde265 2026 CVEs to fixed versions using the GHSA advisories and OSV; check libexif, cgif, libspng, and dav1d; check Debian trixie's patch status for lcms2, libjxl, and libjpeg-turbo |
| Licenses | Read the LICENSE file of every native library and Go module; produce NOTICE and LGPL compliance (dynamic linking, source offer) |
| Untrusted-operation list | Run `vips -l \| grep -i untrusted` against the built 8.18.7 and record it in ADR 0008; confirm the `fail_on` level names |
| Unix socket with `network_mode: none` | Container test: the worker reaches the API socket, cannot resolve or connect to any IP, under runc, rootless Docker, and runsc |
| Landlock on target kernels | Probe the ABI at startup and log it; test go-landlock's thread handling (TSYNC) |
| CSP of the built SPA | Grep `dist/index.html` for inline `<script>` and `<style>`; run Playwright with CSP-violation listeners; score on Mozilla Observatory |
| Binding health | Check open issues and PRs and response times for vipsgen and govips manually |
| Gitleaks action licensing; Renovate support for `actions.lock` | Check the documentation, or fall back to the pinned binary and SHA pins |

## Conclusion

The research changes where Pixsheaf's security effort should go. The Go API is the smaller problem. The C decoder chain is under heavy CVE pressure, and a downstream distribution cannot keep pace with it. So the decisive controls are structural: build libvips from source for patch speed, block every loader outside the allowlist, run each job in a separate process, give workers no network, and make HEIC opt-in. Each of these assumes compromise instead of trying to prevent it. In the same way, the 2026 supply-chain incidents show that signatures and provenance prove *where* an artifact came from, not that it is *safe*. Time delays and immutable pins are what would have stopped Trivy and TanStack.

Two risks will outlast Phase 1. Every viable libvips binding depends on a single maintainer, and several sandbox layers (Landlock ABI levels, Unix sockets under gVisor, a custom seccomp profile) are reasoned through but not yet tested. Phase 2 therefore should open with the container and isolation tests, not with features: if the `network_mode: none` design with a Unix socket fails in practice, ADRs 0006 and 0007 have to change before any image code depends on them.
