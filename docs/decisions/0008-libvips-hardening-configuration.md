# ADR 0008: libvips hardening configuration

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Decision

| Control | Setting | Source |
|---|---|---|
| Untrusted operations | `VIPS_BLOCK_UNTRUSTED=1` in the worker environment, plus a shim call to `vips_block_untrusted_set(TRUE)` | [API docs](https://www.libvips.org/API/current/func.block_untrusted_set.html) |
| Loader allowlist | `vips_operation_block_set("VipsForeignLoad", TRUE)`, then unblock jpeg, png, webp, gif, tiff, bmp path, heif/avif including HEVC-coded HEIC (only with libheif ≥ 1.23.2; see ADR 0016), and jxl; same approach for savers (no jp2k) | [What's new in 8.13](https://www.libvips.org/2022/05/28/What's-new-in-8.13.html) |
| Minimum version | libvips ≥ 8.18.7 (8.17.2 is the floor because of CVE-2025-59933) | [ChangeLog](https://raw.githubusercontent.com/libvips/libvips/master/ChangeLog) |
| Decode strictness | `fail_on=error` by default (`warning` in strict mode); never `unlimited` | [checklist](https://www.libvips.org/API/current/developer-checklist.html), [sharp security](https://sharp.pixelplumbing.com/security) |
| Bomb checks | Load the header only, then check width, height, pages, and total canvas size; start from imagor's defaults (9999 × 9999 px, 81 MP) and make them configurable | [imagor](https://github.com/cshum/imagor/tree/master/processor/vipsprocessor) |
| libheif | Keep `heif_security_limits` (never set `LIBHEIF_SECURITY_LIMITS=off`) | [libheif README](https://raw.githubusercontent.com/strukturag/libheif/master/README.md) |
| Cache and concurrency | `vips_cache_set_max(0)`, concurrency 1 per job (vipsgen's defaults) | [vipsgen util.go](https://github.com/cshum/vipsgen/blob/main/vips/util.go) |
| Access mode | `access=sequential` where the pipeline allows it | [vipsgen](https://github.com/cshum/vipsgen) |


## Consequences

At build time, generate the list of operations marked untrusted (`vips -l | grep -i untrusted`) and commit it to the ADR. The names of the `fail_on` levels must be confirmed against the 8.18 API documentation.

The loader allowlist is the decoder-side half of the image-only upload policy in ADR 0014; the API-side magic-byte allowlist must list exactly the same formats.
