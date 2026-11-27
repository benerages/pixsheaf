<p align="center">
  <picture>
    <source srcset="assets/brand/pixsheaf-logo-dark.svg" media="(prefers-color-scheme: dark)">
    <img src="assets/brand/pixsheaf-logo.svg" alt="Pixsheaf" width="260">
  </picture>
</p>

<p align="center"><strong>Convert, resize, and optimize images. Self-hosted.</strong></p>

---

> **Status: early development (Phase 2 of 6).** The repository contains the hardened scaffold: services, containers, and CI. Image processing arrives in Phase 3. Do not use it in production yet.

Pixsheaf is a self-hosted web tool for everyday image work: drop an image, choose what to do with it, see the result, download it. Select several images and the same steps run on all of them automatically.

- **Formats:** JPEG, PNG, WebP, AVIF, GIF, TIFF, BMP, HEIC/HEIF input, JPEG XL.
- **Actions:** convert, resize, crop, rotate, compress, strip metadata, watermark; presets.
- **Private by design:** no accounts, no storage, no telemetry; files live in memory only and are deleted after download.
- **Hardened:** every upload is treated as hostile and decoded only in an isolated worker without network access.

## Quick start

```sh
git clone https://github.com/benerages/pixsheaf.git && cd pixsheaf
docker compose up -d --build
# open http://127.0.0.1:8080
```

The API listens on `127.0.0.1:8080` only. To make it reachable from other machines, put a TLS reverse proxy in front of it (see [docs/deployment.md](docs/deployment.md)). Configuration: [docs/configuration.md](docs/configuration.md).

## Architecture

```
browser ── reverse proxy (TLS) ── pixsheaf-api ── Unix socket ── pixsheaf-worker (×n)
                                   distroless,     shared volume   no network, read-only,
                                   embedded UI                      one process per file
```

Design decisions are recorded as ADRs in [docs/decisions/](docs/decisions/); the threat model is in [docs/security.md](docs/security.md).

## Security

Please report vulnerabilities privately; see [SECURITY.md](SECURITY.md).

## License

MIT, see [LICENSE](LICENSE). Pixsheaf is a from-scratch rewrite inspired by [Reubah](https://github.com/dendianugerah/reubah); see [NOTICE](NOTICE).
