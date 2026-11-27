# ADR 0013: Supply-chain hardening

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Decision

| Control | Setting | Source |
|---|---|---|
| Action pinning | Full commit SHAs, the repository policy "require SHA pinning", and `.github/workflows/actions.lock` (generally available since 2026-10-28, also pins transitive dependencies) | [GitHub changelog](https://github.blog/changelog/2025-08-15-github-actions-policy-now-supports-blocking-and-sha-pinning-actions/), [InfoQ](https://infoq.com/news/2026/11/github-actions-lockfiles-ga) |
| Image pinning | `tag@sha256:…` in Dockerfiles and compose files; Renovate `config:best-practices` and `default:automergeDigest` | [Renovate Docker](https://docs.renovatebot.com/docker/) |
| Release age | Renovate `minimumReleaseAge: "7 days"` and pnpm `minimumReleaseAge: 10080`, with documented exceptions for security fixes | [Snyk](https://snyk.io/blog/tanstack-npm-packages-compromised), [pnpm 11](https://pnpm.io/blog/releases/11.0) |
| pnpm | `--frozen-lockfile`, `blockExoticSubdeps: true`, `strictDepBuilds: true` with an explicit `allowBuilds`, `engineStrict`, `pnpm audit` | [pnpm 11](https://pnpm.io/blog/releases/11.0) |
| Workflows | No `pull_request_target`; top-level `permissions: contents: read` with per-job grants; `persist-credentials: false`; zizmor in CI; harden-runner with an egress allowlist; caches scoped per workflow | [changelog 2026-11-02](https://github.blog/changelog/2026-11-02-pull_request_target-blocked-by-default-december-2) |
| Scanners | Grype as the primary gate, Trivy as a secondary gate installed from a cosign-verified release; govulncheck, CodeQL, Gitleaks, Scorecard | [GHSA-69fq](https://github.com/aquasecurity/trivy/security/advisories/GHSA-69fq-xp46-6x23) |
| Provenance | `actions/attest-build-provenance` v4.2.2 (push to GHCR) instead of `slsa-github-generator`, whose last release was 2025-02-24; SBOM from Syft attached with `actions/attest`; cosign keyless signing by digest | [attest-build-provenance](https://github.com/actions/attest-build-provenance), [slsa-github-generator](https://github.com/slsa-framework/slsa-github-generator) |


## Consequences

Updates arrive about a week later. Whether Renovate can maintain `actions.lock` files is unknown; keep explicit SHA pins as the primary control. TanStack-style payloads target `.claude/settings.json` and `.vscode/tasks.json`, so add those paths to CODEOWNERS review.
