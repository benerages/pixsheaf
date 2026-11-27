# ADR 0021: BMP and PDF handled in Go; no PDF library

- Status: Accepted
- Date: 2026-11-27
- Amends: the technology stack in the project instructions (pdfcpu for images → PDF)

## Context

The libvips build (ADR 0004, ADR 0018) has no BMP loader or saver: in libvips, BMP is only available through ImageMagick, which ADR 0008 excludes. PDF output is required only as a simple export of one image. The project instructions name pdfcpu for this.

## Options

BMP: (a) `golang.org/x/image/bmp` (pure Go, already a dependency since ADR 0020); (b) ImageMagick loader in libvips; (c) drop BMP.

PDF: (a) pdfcpu; (b) a minimal PDF writer in Pixsheaf that embeds one JPEG with `/DCTDecode`; (c) libvips `pdfsave` (not available; needs cairo or a PDF library).

## Decision

BMP option (a). The worker detects BMP by its `BM` signature, reads the header with `bmp.DecodeConfig`, checks the pixel limits, and only then decodes. Decoded BMPs are limited to 32 megapixels, because the Go decoder and the copy into libvips each need four bytes per pixel within the job memory limit. RLE-compressed and other unusual variants are rejected as unsupported. Output is uncompressed 24-bit BMP; transparency is flattened onto white, because the Go encoder writes 32-bit BMPs with the 40-byte header, whose alpha channel most readers ignore.

PDF option (b). Output never parses a PDF; it writes a fixed structure of five objects (catalog, pages, page, image, content stream) around a JPEG produced by libvips. That is about 60 lines of code with a unit test that checks the cross-reference table, against a library with a PDF parser, many features, and several transitive dependencies that the worker would not use. The image is converted to sRGB (or stays grayscale) so that `/DeviceRGB` or `/DeviceGray` describes it correctly. The page is A4 in the orientation of the image, with 0.5 inch margins, and the image is shown at most at 96 DPI so that small images are not enlarged.

## Consequences

- No new dependency for PDF, and no PDF parser in the worker.
- If PDF features beyond one image per page are needed later (several images per PDF, page size options, metadata), revisit pdfcpu then; the output format of `wrapJPEG` can be checked against it in tests.
- BMP decoding happens in Go in the sandboxed job process, like text rendering (ADR 0020); a decoder bug is a memory-safe failure, not a memory corruption.
- Uncommon BMP variants (RLE, 16-bit, OS/2 headers) are not supported.
