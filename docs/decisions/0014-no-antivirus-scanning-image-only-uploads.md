# ADR 0014: No antivirus scanning; uploads restricted to images

- Status: Accepted
- Date: 2026-11-27
- Deviates from: OWASP ASVS 5.0 requirement 5.4.3 (Level 2)

## Context

ASVS 5.4.3 asks for files from untrusted sources to be scanned by antivirus software before they are served. Pixsheaf has a "no external services" rule, never stores files, and never serves an uploaded file as-is. An antivirus engine such as ClamAV would add a large, complex parser to the attack surface and needs network access for signature updates. The project owner decided against antivirus scanning and instead requires that uploads be restricted as tightly as possible to images.

## Options

(a) No antivirus scanning; strict image-only upload policy (this ADR). (b) Optional ClamAV sidecar, off by default. (c) ClamAV on by default.

## Decision

Option (a). Pixsheaf ships no antivirus integration. Instead, the following controls together ensure that only images are accepted and only freshly encoded images leave the system:

1. **Client hint only:** the file input uses `accept` with the allowed image MIME types. This is a usability aid, never a security control.
2. **Format allowlist by magic bytes in the API:** JPEG, PNG, WebP, GIF, TIFF, BMP, AVIF, HEIC/HEIF, and JPEG XL. The declared `Content-Type` and the file extension are ignored for the decision; a mismatch between them and the detected type is logged (without file names) and the detected type wins. Everything else, including SVG, PDF, PSD, RAW, EXR, ICO, and any archive other than a top-level ZIP of images, is rejected with HTTP 415.
3. **ZIP uploads:** parsed in pure Go in the API; every entry must itself pass the magic-byte allowlist; nested archives, symlinks, absolute paths, and `..` segments are rejected; entry count, uncompressed size, and compression ratio are capped while counting bytes actually read.
4. **Decoder allowlist:** libvips blocks all loaders and re-enables only the formats from point 2 (ADR 0008). A file that passes the API check but is not decodable by an allowed loader fails.
5. **Header check before decode:** width, height, page/frame count, and total canvas size are checked from the header before any pixel data is decoded.
6. **Re-encode always:** the original bytes are never returned. Every output is a new encode produced by the sandboxed worker, with metadata stripped by default. This removes appended payloads and polyglot content.
7. **Sandbox:** decoding happens only in the per-job subprocess with no network (ADRs 0006 and 0007).
8. **Response headers:** downloads use `Content-Disposition: attachment` (RFC 6266), the detected output MIME type, and `X-Content-Type-Options: nosniff`.

## Consequences

- The deviation from ASVS 5.4.3 is documented in `docs/security.md` and the threat model as an accepted residual risk.
- Residual risk: an image that is valid and harmless as pixels but carries a payload in a format quirk is neutralized by re-encoding; a decoder exploit is contained by the sandbox, not prevented.
- Operators who need antivirus scanning can put a scanning reverse proxy in front of Pixsheaf; this is out of scope for v1.0 and not documented as a supported setup.
- Integration tests must cover: disguised types (wrong extension and Content-Type), polyglots (e.g. JPEG+ZIP, PNG+HTML), SVG and PDF uploads, and archives inside ZIPs.
