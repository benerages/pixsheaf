<p align="center">
  <picture>
    <source srcset="assets/brand/pixsheaf-logo-dark.svg" media="(prefers-color-scheme: dark)">
    <img src="assets/brand/pixsheaf-logo.svg" alt="Pixsheaf" width="260">
  </picture>
</p>

<p align="center"><strong>Convert, resize, and optimize images. Self-hosted.</strong></p>

---

Pixsheaf is a self-hosted web tool for everyday image work: drop an image, choose what to do with it, compare before and after, download it. Select several images, a folder, or a ZIP archive and the same steps run on all of them automatically.

- **Formats:** JPEG, PNG, WebP, AVIF, GIF (also animated), TIFF, BMP, and JPEG XL in and out; HEIC/HEIF as input; images to PDF.
- **Actions:** convert, resize, crop to an aspect ratio, rotate, flip, compress to a quality or a target size, strip metadata, text or image watermark; presets you can export and share.
- **Batches:** many files, folders, or ZIP archives; live progress per file; rename patterns; download as one ZIP with a report, or file by file.
- **Private by design:** no accounts, no storage, no telemetry, no external fonts or CDNs. Files live in memory only and are deleted after the download, on cancel, or after 15 minutes.
- **Hardened:** every upload is treated as hostile and decoded only in a sandboxed process in a worker container without network access. Signed multi-arch images with SBOM and provenance.

English and German, light and dark, usable with the keyboard, and checked with axe-core against WCAG 2.2 AA.

## Quick start

On any host with Docker Compose (linux/amd64 or linux/arm64):

```sh
curl -fsSLO https://raw.githubusercontent.com/benerages/pixsheaf/v1.0.0/compose.yaml
IMAGE_TAG=1.0.0 docker compose up -d
# open http://127.0.0.1:8080
```

Pixsheaf listens on `127.0.0.1:8080` only. To use it from other machines, put a TLS reverse proxy in front of it ([docs/deployment.md](docs/deployment.md)). All settings are environment variables with secure defaults ([docs/configuration.md](docs/configuration.md)); more workers process more files at once: `docker compose up -d --scale worker=3`.

To verify the images before you run them:

```sh
cosign verify ghcr.io/benerages/pixsheaf-api:1.0.0 \
  --certificate-identity-regexp '^https://github.com/benerages/pixsheaf/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## API

Everything the web UI does is available over a versioned REST API (`/api/v1`), specified in [api/openapi.yaml](api/openapi.yaml) and described in [docs/api.md](docs/api.md). A preset saved in the UI is the same JSON document the API accepts, so both give identical results.

## Architecture

```
browser ── reverse proxy (TLS) ── pixsheaf-api ── Unix socket ── pixsheaf-worker (×n)
                                   distroless,     shared volume   no network, read-only,
                                   embedded UI                      one sandboxed process per file
```

Design decisions are recorded as ADRs in [docs/decisions/](docs/decisions/). The threat model is in [docs/security.md](docs/security.md), the OWASP ASVS 5.0 Level 2 review in [docs/asvs-review.md](docs/asvs-review.md).

## Development

```sh
git clone https://github.com/benerages/pixsheaf.git && cd pixsheaf
docker compose up -d --build
tests/e2e/single-image.sh && tests/e2e/batch.sh && tests/e2e/ui.sh
```

Go runs in the pinned build image (`scripts/go.sh test ./...`), so no local toolchain is needed. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md).

## License

MIT, see [LICENSE](LICENSE). Pixsheaf is a from-scratch rewrite inspired by [Reubah](https://github.com/dendianugerah/reubah); see [NOTICE](NOTICE).
