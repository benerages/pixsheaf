# ADR 0005: API base image

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Options

`gcr.io/distroless/static-debian13:nonroot`; `cgr.dev/chainguard/static:latest@digest`; `scratch` plus a CA bundle.

## Decision

Use distroless `static-debian13:nonroot`, always with the explicit `-debian13` suffix because the unsuffixed default "may change", pinned by digest and verified with cosign in CI ([distroless](https://github.com/GoogleContainerTools/distroless)).

## Consequences

No shell and no package manager. The health check is a subcommand of the binary itself. The image is multi-arch. This avoids depending on Chainguard's tier policy, which limits free users to `latest` tags and digests ([Chainguard](https://www.chainguard.dev/unchained/a-guide-on-how-to-use-chainguard-images-for-public-catalog-tier-users)). The `nonroot` UID is commonly 65532 (not verified). Set `USER 10001:10001` explicitly to satisfy the UID ≥ 10000 rule deterministically.
