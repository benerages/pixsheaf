# ADR 0017: Palette quantization with quantizr instead of libimagequant

- Status: Accepted
- Date: 2026-11-27

## Context

libvips uses a quantization library for palette PNG output and GIF saving. The usual choice, libimagequant, is licensed under GPL v3 or later (with historical permissive notices for the original pngquant code by Jef Poskanzer and Greg Roelofs) or a commercial license. Linking it into the worker would make the distributed worker image a GPL-3.0 combined work, with source-offer obligations, while Pixsheaf itself is MIT-licensed.

[quantizr](https://github.com/DarthSim/quantizr) (Rust, C API) is MIT-licensed and supported by libvips 8.13 and later as a drop-in replacement for libimagequant.

## Options

(a) quantizr. (b) libimagequant under GPL-3.0, making the worker image GPL-3.0. (c) libimagequant under a commercial license. (d) No quantization library (palette PNG and GIF quality degrade).

## Decision

Option (a). The worker builds libvips against quantizr, built from pinned source in the builder stage (requires a Rust toolchain in the build stage only). libimagequant is not installed and must not appear in the worker image; CI checks this by listing the shared libraries of the final image.

## Consequences

- The worker image stays free of GPL-3.0 components, which keeps licensing simple for an MIT project.
- quantizr's upstream activity is low (last push to the main repository in early 2024 as listed by ecosyste.ms; a fork exists under `imazen/quantizr`). Phase 2 checks its current release and maintenance status; if it is unmaintained, re-evaluate between a maintained fork and option (d).
- Phase 3 compares GIF and palette-PNG output quality and file size against expectations; if quality is unacceptable, revisit with the owner.
- All other native libraries still need a license check before release (Phase 2 gap list).
