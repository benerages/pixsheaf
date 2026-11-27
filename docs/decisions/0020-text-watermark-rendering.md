# ADR 0020: Text watermarks rendered in Go with the bundled Go fonts

- Status: Accepted
- Date: 2026-11-27

## Context

Text watermarks need a font renderer. libvips has a `text` operation, but it depends on pango, cairo, fontconfig and FreeType (and through them HarfBuzz, fribidi and glib extras). The worker build (ADR 0018) compiles libvips without them. Adding them would roughly double the native code in the worker, add font discovery from the file system (fontconfig reads configuration and cache files, which the Landlock rules of ADR 0006 would have to allow), and pango interprets markup unless every caller escapes it.

## Options

(a) libvips `text` with pango and a bundled font. (b) Render text in Go with `golang.org/x/image/font/opentype` (pure Go, memory-safe) and the Go fonts, which are compiled into the binary, then hand the pixels to libvips for scaling and compositing. (c) No text watermarks; image watermarks only.

## Decision

Option (b). The worker renders one line of plain text with Go Bold (BSD-3-Clause, same license as `golang.org/x/image`) into an RGBA image of at most 4 megapixels and passes it to libvips, which scales it to the requested width, applies the opacity and composites it. Nothing is read from disk, and no markup is interpreted. The pipeline rejects control characters in watermark text, so a text is always one line.

## Consequences

- No new native libraries and no font files in the worker image; the font parser runs in Go inside the job sandbox.
- One font only. Characters outside the Go font's repertoire (most non-Latin scripts, emoji) render as a placeholder box, and there is no complex shaping (Arabic, Indic scripts) or right-to-left layout. This is documented in the API description; a font choice can be added later as a compiled-in set without changing the pipeline format.
- Text larger than 4 megapixels is upscaled by libvips and gets slightly softer edges; this bounds the Go heap in the job process.
- `golang.org/x/image` becomes a dependency of the worker. Only its font packages are linked; govulncheck checks reachability.
