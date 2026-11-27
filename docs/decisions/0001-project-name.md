# ADR 0001: Project name "Pixsheaf"

- Status: Accepted
- Date: 2026-11-27

## Context

The rewrite of Reubah needs its own name: the scope changes substantially (image processing only — single images first, automatic batch mode for multiple files — no document conversion), the codebase shares no code with the original, and the project must be clearly distinguishable from the unmaintained upstream. All project artifacts are in English, so the name must work for an international audience.

Requirements:

- Short, pronounceable, easy to type as a CLI command and image name.
- Fits the product idea: simple processing of an image, which scales naturally to many images with the same steps.
- No collision with existing software in the same space (self-hosted, Docker, GitHub/ghcr).

## Options considered

| Option | Assessment |
|---|---|
| Stapel, Pixelzug, Stanze, Kaskade | German; rejected after the decision that everything is in English. "Kaskade" also close to the existing image editor "Cascade". |
| Pixel Press / PixPress | Taken by several image optimizers and a Framer plugin. |
| Kiln | Taken by a static site generator with built-in image optimization. |
| Sheaf | Taken in the same niche: `sheaf-project/sheaf` (self-hostable app with signed Docker images, domain sheaf.sh); also `bryanl/sheaf` (Go CLI) and `sheaf-serve` on ghcr. |
| Ream, Sluice | No collision found; less distinctive and harder to search for. |
| **Pixsheaf** | "Sheaf" (a bound bundle of sheets) with a clear image prefix; reads as "images, neatly handled" without making batch processing the headline. No exact match found. |

## Decision

The project is named **Pixsheaf**.

- Repository and module: `pixsheaf`
- Binaries: `pixsheaf-api`, `pixsheaf-worker`
- Images: `ghcr.io/<owner>/pixsheaf-api`, `ghcr.io/<owner>/pixsheaf-worker`
- Environment variable prefix: `PIXSHEAF_`
- Tagline: "Pixsheaf — convert, resize, and optimize images. Self-hosted." (batch processing is mentioned as a feature, not in the tagline)
- Logo: an image card in front, two more fanned behind it, bound by a band (sheaf concept); the front image dominates, matching the single-image focus

LICENSE/NOTICE keeps the MIT license of Reubah and a note that Pixsheaf is a from-scratch rewrite inspired by it.

## Consequences

- "Picsheaf" (`mowtiie/Picsheaf`, a GTK desktop app that combines images into a PDF) sounds similar. Risk is accepted: different platform (desktop vs. self-hosted server) and different purpose; README states the distinction if needed.
- Before the first public release, verify availability of the GitHub organization/repository name, ghcr namespace, and a domain (e.g. pixsheaf.dev), and check trademark registers (Swissreg, EUIPO, classes 9/42). This is not legal advice; a professional check is recommended before any commercial use.
