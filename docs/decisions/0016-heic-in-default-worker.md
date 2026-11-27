# ADR 0016: HEIC/HEIF decoding in the default worker image

- Status: Accepted
- Date: 2026-11-27

## Context

HEIC input was supported by Reubah and is a common format from phones. Decoding HEVC-coded HEIC requires libde265 as a libheif plugin. In 2025–2026, libheif and libde265 had the highest number of memory-safety CVEs among the decoders Pixsheaf uses (see the Phase 1 report). The HEVC patent situation is unclear; libheif's README does not address it. The Phase 1 report recommended a separate opt-in worker variant. The project owner prefers a single package that includes HEIC.

## Options

(a) Include libde265 in the default worker image. (b) Separate `pixsheaf-worker-heic` variant. (c) No HEIC support.

## Decision

Option (a). The default worker image includes libheif with the libde265 decoder plugin (decode only; no x265 encoder). Compensating measures:

- libheif ≥ 1.23.7 and libde265 at the latest release, built or patched promptly; both tracked by Renovate and the weekly rebuild.
- libheif's built-in security limits stay enabled (`LIBHEIF_SECURITY_LIMITS` is never set to `off`).
- HEIC/HEIF goes through the same header-first canvas, page, and grid checks as all other formats (ADR 0008).
- Decoding runs only in the per-job sandbox (ADR 0006).
- Operators can disable HEIC at runtime with `PIXSHEAF_DISABLE_FORMATS=heic`; the API then rejects HEIC uploads with HTTP 415 and the worker keeps the loader blocked.

## Consequences

- The default image carries the riskiest decoder; the sandbox is the main containment and must be tested with a HEIC fuzz corpus.
- Patent questions around HEVC decoding are not resolved by this ADR. The README and NOTICE state that HEVC decoding is included via libde265 and that operators are responsible for compliance in their jurisdiction. This is not legal advice; obtain professional advice before commercial distribution.
- AVIF (AV1 via dav1d) is unaffected by this decision.
