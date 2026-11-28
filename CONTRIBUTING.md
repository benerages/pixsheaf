# Contributing

Thanks for your interest in Pixsheaf. Please open an issue to discuss larger changes before starting work. Security problems go through [SECURITY.md](SECURITY.md), not public issues.

## Ground rules

- **Language:** code, comments, docs, commit messages, and log messages are in US English.
- **Commits:** [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `build:`, `ci:` …); the changelog is generated from them.
- **Decisions:** changes to architecture, security controls, or dependencies need an ADR in `docs/decisions/` (or an amendment to an existing one).
- **Dependencies:** pin exact versions; images by digest, Actions by commit SHA. New dependencies must be actively maintained and license-compatible with MIT (no GPL in the shipped images, see ADR 0017).
- **Security over convenience:** if in doubt, choose the more restrictive option and explain why.

## Development

Requirements: Docker with Compose. A local Go toolchain is optional: `scripts/go.sh` runs Go in the pinned build image. For the web UI you need Node.js 26 and pnpm (version from `web/package.json`).

```sh
# Go (or plain `go test ./...` with Go from go.mod)
scripts/go.sh vet ./... && scripts/go.sh test ./...
# libvips code (build tag "vips") in the worker image
docker build -f deploy/docker/worker.Dockerfile --target test .
# After changing api/openapi.yaml
scripts/generate-api.sh

# Web UI (writes the build to internal/ui/dist, which the Go binary embeds)
cd web
pnpm install --frozen-lockfile
pnpm dev        # dev server, proxies /api to 127.0.0.1:8080
pnpm lint && pnpm check && pnpm build

# Containers, including the isolation self-check
tests/container/run.sh

# End-to-end against a running stack
docker compose up -d --build --wait
tests/e2e/single-image.sh && tests/e2e/batch.sh && tests/e2e/ui.sh
```

New web dependencies are added with `scripts/pin-mature-deps.sh` (only versions at least seven days old, ADR 0013).

`pnpm build` replaces the placeholder `internal/ui/dist/index.html`; do not commit the build output.

## Pull requests

CI must be green: Go tests and linters, web lint/check/build, hadolint, zizmor, gitleaks, CodeQL, container isolation tests, the end-to-end and browser tests (Playwright, axe-core), and the Grype image scan.

## Releases

Push a tag `vX.Y.Z` on `main`. The release workflow builds both architectures natively, publishes a signed multi-arch index with SBOM and provenance attestations to GHCR, and creates the GitHub release with the changelog from `scripts/changelog.sh` and a digest-pinned `compose.yaml` from `scripts/release-compose.sh`. Start the workflow by hand first for a dry run that builds everything without publishing.
