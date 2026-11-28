# Security policy

## Reporting a vulnerability

Please do **not** open a public issue for security problems.

Report vulnerabilities privately through GitHub: **Security → Report a vulnerability** on <https://github.com/benerages/pixsheaf/security>. Include the affected version or commit, steps to reproduce, and, if possible, a sample file. Malicious sample images are welcome; please mark them clearly.

You can expect an acknowledgement within 7 days. Fixes are released as soon as they are ready and credited in the changelog unless you prefer otherwise.

## Supported versions

Only the latest minor release (currently 1.0.x) receives security fixes. Images are rebuilt every week, so patched system libraries reach the latest release without a new version number; pull the image again to get them.

## Remediation time frames

These time frames apply from the moment a fix is available, for vulnerabilities in Pixsheaf itself and in bundled components (libvips, codecs, Go, base images, npm packages):

| Severity (CVSS 3.1 or 4.0) | Fixed release within |
|---|---|
| Critical (9.0 or higher), or exploited in the wild | 3 days |
| High (7.0 to 8.9) | 14 days |
| Medium (4.0 to 6.9) | 30 days |
| Low | with the next regular release |

Renovate proposes dependency updates as soon as they are seven days old, and security updates immediately; CI fails on known critical or high vulnerabilities with an available fix (govulncheck, Grype, `pnpm audit`).

## Scope

In scope: the Pixsheaf services, container images, compose files, and CI configuration in this repository. Vulnerabilities in bundled libraries (for example libvips or image codecs) are welcome too; we will coordinate with upstream.

The design and threat model are documented in [docs/security.md](docs/security.md).
