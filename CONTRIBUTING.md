# Contributing

Thanks for your interest in Pixsheaf. The project is in early development; please open an issue to discuss larger changes before starting work.

## Ground rules

- **Language:** code, comments, docs, commit messages, and log messages are in US English.
- **Commits:** [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `build:`, `ci:` …); the changelog is generated from them.
- **Decisions:** changes to architecture, security controls, or dependencies need an ADR in `docs/decisions/` (or an amendment to an existing one).
- **Dependencies:** pin exact versions; images by digest, Actions by commit SHA. New dependencies must be actively maintained and license-compatible with MIT (no GPL in the shipped images, see ADR 0017).
- **Security over convenience:** if in doubt, choose the more restrictive option and explain why.

## Development

Requirements: Go (version from `go.mod`), Node.js 26 LTS, pnpm (version from `web/package.json`), Docker with Compose.

```sh
# Go
go test ./...

# Web UI (writes the build to internal/ui/dist, which the Go binary embeds)
cd web
pnpm install --frozen-lockfile
pnpm dev        # dev server, proxies /api to 127.0.0.1:8080
pnpm lint && pnpm check && pnpm build

# Containers, including the isolation self-check
tests/container/run.sh
```

`pnpm build` replaces the placeholder `internal/ui/dist/index.html`; do not commit the build output.

## Pull requests

CI must be green: Go tests and linters, web lint/check/build, hadolint, zizmor, gitleaks, CodeQL, container isolation tests, and the Grype image scan.
