# ADR 0018: Wolfi as the worker base; minimal runtime root filesystem

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27
- Supersedes: [ADR 0004](0004-libvips-build-and-worker-base-image.md)

## Context

ADR 0004 chose `debian:trixie-slim` with Debian's codec packages and required Phase 2 to check their security status. The check on 2026-11-27 ([Debian security tracker](https://security-tracker.debian.org/tracker/)) found open issues in trixie that Debian does not plan to fix through security advisories ("no DSA"), all in libraries that parse untrusted image data:

| Package (trixie version) | Open issues |
|---|---|
| libde265 (1.0.15) | Use-after-free and double free (GHSA-xp3h-6f5r-8cxp), heap use-after-free (GHSA-mm7m-v26f-wf8x), NULL dereference (CVE-2026-88373) |
| tiff (4.7.0) | Heap-buffer overflow (CVE-2026-18495), CVE-2026-52490, denial of service (CVE-2026-36849), and 11 issues rated unimportant |
| lcms2 (2.16) | Heap buffer overflow (CVE-2025-29070); trixie's version is in the range of CVE-2026-42798 |
| jpeg-xl (0.11.2) | Integer underflow fixed in libjxl 0.12 (CVE-2026-82522) |
| libpng1.6 (1.6.48) | CVE-2026-3713 |

HEIC decoding (libde265) is in the default image by ADR 0016, so these gaps cannot be closed by dropping a format. Wolfi ships the current upstream releases of the same libraries (libde265 1.1.3, tiff 4.7.2, lcms2 2.20, libjxl 0.13.0, libpng 1.6.59, libjpeg-turbo 3.2.1, libheif 1.23.7) on glibc and rebuilds packages continuously ([wolfi-dev/os](https://github.com/wolfi-dev/os)).

Wolfi's own `libheif` package enables many more codecs than Pixsheaf needs (OpenH264, OpenJPEG, OpenJPH, x265, rav1e, and others), and its `libvips` package auto-detects optional loaders such as librsvg.

## Options

(a) Wolfi codec packages; libvips, libheif, libde265, cgif, and quantizr built from pinned source. (b) Debian trixie with roughly twelve libraries built from source. (c) Debian trixie as is, relying on the sandbox alone.

## Decision

Option (a), chosen by the project owner.

- **Build stages** start from `cgr.dev/chainguard/wolfi-base` pinned by digest. Every package installed on top of it is pinned at an exact version in `deploy/docker/wolfi/build.lock`, which `scripts/update-worker-pins.sh` generates from `deploy/docker/wolfi/build.packages`.
- **Source builds** (libde265, libheif, libvips, cgif, quantizr) are pinned by tag and verified commit (`fetch-source` fails if the tag points elsewhere); Renovate tracks the tag and commit together:
  - libvips with an explicit feature list: jpeg, png (libpng; spng off), webp, cgif, nsgif, tiff, heif, jpeg-xl, lcms, exif, zlib, quantizr, and highway (SIMD, used if found; Wolfi's libjxl-dev ships its own libhwy and conflicts with highway-dev). Everything else is disabled at build time, including magick, poppler, pdfium, rsvg, openslide, openexr, openjpeg, matio, cfitsio, nifti, fftw, orc, pangocairo, fontconfig, archive, uhdr, imagequant, ppm, analyze, and radiance. Modules and introspection are off.
  - libde265 for HEIC decoding. Wolfi's `libde265-dev` 1.1.3 was found to install 1.1.3 headers next to an older `libde265.so.0.1.9` without the decoder security limits, so libheif failed to link. The riskiest decoder in the image is therefore built from source as well.
  - libheif with libde265 (HEIC decoding), dav1d (AVIF decoding), and SVT-AV1 (AVIF encoding) only; no HEVC encoder and no plugin loading (ADR 0016).
  - cgif for GIF output and quantizr for palette quantization (ADR 0017).
- **Runtime image** is built `FROM scratch`. `make-rootfs` asks the dynamic loader which shared libraries the worker binary needs, installs exactly the Wolfi packages that own them at the build stage's versions (with apk, so the package database is present for Grype and SBOM tools, but without install scripts), and copies the libraries built from source. It fails the build if the runtime image contains a shell, a package manager, or libimagequant, if a package version differs from the build stage, or if the binary does not start.
- The libvips choice of libpng over libspng resolves the open point of ADR 0004 (libspng has had no release since 2023).

## Consequences

- The open trixie issues above do not apply; codec fixes arrive as soon as Wolfi packages them and the lock is refreshed.
- The runtime image has no shell. Debugging uses `docker cp` or a debug build of the `rootfs` stage.
- Wolfi is a rolling distribution and removes old package versions over time, so a stale `build.lock` eventually stops building. The lock must be refreshed regularly. Until a scheduled refresh workflow exists, run `scripts/update-worker-pins.sh` at least weekly and on every Wolfi advisory for a worker library. The weekly scheduled rebuild alone no longer picks up codec fixes, because versions are locked.
- libvips, libheif, libde265, cgif, and quantizr are in `/usr/local` and are not in the apk database, so image scanners do not see them. The project tracks their advisories itself (as already required by ADR 0004); a supplementary SBOM entry is planned for the release workflow (Phase 6).
- The base digest is updated by Renovate; the lock file is not. A digest update without a lock refresh still builds, because the lock pins versions independently of the base.
- jemalloc (recommended for glibc in ADR 0004) is not yet used; evaluate it with the load tests in Phase 4.
