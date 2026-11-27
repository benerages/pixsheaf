# ADR 0004: libvips build and worker base image

- Status: Superseded by [ADR 0018](0018-wolfi-worker-base.md) on 2026-11-27
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Context

Upstream is libvips 8.18.7 and libheif 1.23.7. Debian trixie ships 8.16.1 with 17 open libheif issues. Alpine 3.24 ships 8.18.2, but its community repo is supported only until the next release. Wolfi ships `libvips-8.17` ([Debian vips](https://tracker.debian.org/pkg/vips), [Alpine](https://alpinelinux.org/releases/), [OSV CGA](https://osv.dev/vulnerability/CGA-349f-593f-g44r)).

## Options

(a) Build libvips and libheif from pinned source on `debian:trixie-slim`, linking Debian's codecs underneath. (b) Alpine 3.24 packages. (c) Wolfi packages. (d) Debian packages only.

## Decision

Option (a). Pin source tarballs by SHA-256 with `ARG LIBVIPS_VERSION` and `ARG LIBHEIF_VERSION`, tracked by Renovate custom regex managers. Configure meson to disable magick, poppler, pdfium, rsvg, openslide, matio, cfitsio, and OpenEXR. Include the `libde265` decoder plugin for HEIC input in the default worker image (see ADR 0016). Do not install `libheif-plugin-x265` (HEIC is input only). Build libvips against quantizr instead of libimagequant (see ADR 0017). Use jemalloc on glibc ([checklist](https://www.libvips.org/API/current/developer-checklist.html)). Pin the runtime to `debian:trixie-slim` by digest and rebuild weekly.

## Consequences

Pixsheaf gets libvips CVE fixes the day they ship, and Debian's security team keeps patching the low-level codecs. The cost is that the project tracks libvips and libheif CVEs itself, and builds take longer (cache them with BuildKit). Debian trixie's libjpeg-turbo is 2.1.5 and its lcms2 is 2.16; lcms2 2.16 is in the range affected by CVE-2026-42798 unless Debian backported the fix ([lcms2](https://security-tracker.debian.org/tracker/source-package/lcms2)). **Phase 2 must check the Debian security status of lcms2, libjxl, and libjpeg-turbo, and build them from source where they are unpatched.** Choose between libspng (stale since 2023) and libpng here; the recommendation is libpng unless benchmarks show a material gap. Re-evaluate Wolfi once it ships libvips 8.18 with verified codec flags.
