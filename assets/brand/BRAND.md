# Pixsheaf brand assets

The mark is a sheaf: one image card in front, two more fanned behind it, tied together by a band. The front image dominates: Pixsheaf is about handling an image simply, and it scales to many when needed.

## Files

| File | Use |
|---|---|
| `pixsheaf-logo.svg` / `pixsheaf-logo-dark.svg` | Horizontal logo for light / dark backgrounds (README header, web UI) |
| `pixsheaf-logo-stacked.svg` / `-dark.svg` | Stacked logo with tagline "convert, resize, optimize · self-hosted" (social preview, docs landing page) |
| `pixsheaf-mark.svg` / `-dark.svg` | Mark only, 48 px and larger |
| `pixsheaf-mark-small.svg` / `-dark.svg` | Simplified mark (no sun) for 16–32 px |
| `favicon.svg` | Favicon; switches colors via `prefers-color-scheme` |
| `favicon.ico` | Legacy favicon (16, 32, 48 px) |
| `png/` | Raster exports: favicon-32, apple-touch-icon (180), icon-192, icon-512, logo PNGs |
| `build.py` | Regenerates all SVGs (requires fontTools and Inter SemiBold) |

## Colors

| Role | Light background | Dark background |
|---|---|---|
| Front card | `#0F6E56` | `#1D9E75` |
| Middle card | `#1D9E75` | `#5DCAA5` |
| Back card | `#5DCAA5` | `#9FE1CB` |
| Band / sun | `#EF9F27` | `#EF9F27` |
| Wordmark | `#12302A` | `#EAF5F1` |

## Typography

Wordmark: Inter SemiBold, lowercase `pixsheaf`, converted to outlines (no font dependency at runtime). Inter is licensed under the SIL Open Font License 1.1.

## Usage

- Keep clear space around the logo of at least the height of the band.
- Do not recolor, rotate, stretch, or add effects to the mark.
- Use the dark variants on backgrounds darker than `#808080`.
- In the web UI, embed the SVGs locally (no external CDNs), in line with the CSP.
