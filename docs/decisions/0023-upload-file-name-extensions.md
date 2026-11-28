# ADR 0023: Upload file names need an image extension; the content decides the format

- Status: Accepted
- Date: 2026-11-28
- Deciders: Claude (lead architect), from the Phase 6 ASVS review

## Context

OWASP ASVS 5.0 requirement 5.2.2 (Level 1) asks that an application accepting a file, on its own or inside an archive, checks that the file extension is an expected one and that the content matches the type the extension stands for.

Until Phase 6, Pixsheaf ignored file names entirely and decided the type from the magic bytes alone (ADR 0014). That already covers the risk 5.2.2 targets, a file that is served or executed according to a misleading name: Pixsheaf never stores uploads in a served folder, decodes only allowlisted formats in the worker sandbox, always re-encodes, and serves outputs with a fixed `Content-Type`, `nosniff`, and `Content-Disposition: attachment`. The extension check was still missing.

Requiring the extension to match the content exactly would reject common, harmless files: photos renamed by hand, PNGs saved as `.jpg` by websites, and HEIF files from some cameras with unusual suffixes. It would add nothing, because the content is decoded and re-encoded in any case.

## Decision

- An upload, and every entry of an uploaded ZIP archive, must have a file name that ends in an allowed extension, compared case-insensitively: `.jpg`, `.jpeg`, `.jpe`, `.jfif`, `.png`, `.webp`, `.avif`, `.gif`, `.tif`, `.tiff`, `.bmp`, `.jxl`, `.heic`, `.heif`, `.hif`, and `.zip` for uploads. Names without an extension, with another extension, or with a trailing segment such as `image.png.php` are refused with 415 `unsupported_format` (archive entries fail individually with the same code).
- The content still decides the format. An allowed extension that does not match the content (`photo.jpg` containing a PNG) is accepted and processed as what it is.
- The web UI names pasted clipboard images `pasted-image.<ext>` from their media type, so that they pass the check.

## Consequences

- Requirement 5.2.2 is met for the extension check; the content-matches-extension part is a documented, deliberate deviation with the compensating controls above (ADR 0014).
- API clients must send a file name. `curl -F file=@photo.heic` does so by default.
- Folder uploads with non-image files (`.DS_Store`, `Thumbs.db`, sidecar `.xmp` files) see those files refused before they are stored, instead of after sniffing.
- Addendum (1.1.2): the web UI applies the same extension list before uploading. Unsupported files in a selection or folder are left out and listed in a notice; hidden and system files (`.DS_Store`, `._*`, `Thumbs.db`, `desktop.ini`) are left out silently. Archive entries that fail with `unsupported_format` or `format_disabled` are hidden from the file list and listed in the same notice; `report.json` in the ZIP download still records them.
