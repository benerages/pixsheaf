# Security policy

## Reporting a vulnerability

Please do **not** open a public issue for security problems.

Report vulnerabilities privately through GitHub: **Security → Report a vulnerability** on <https://github.com/benerages/pixsheaf/security>. Include the affected version or commit, steps to reproduce, and, if possible, a sample file. Malicious sample images are welcome; please mark them clearly.

You can expect an acknowledgement within 7 days. Fixes are released as soon as they are ready and credited in the changelog unless you prefer otherwise.

## Supported versions

Pixsheaf has not had a release yet. Once 1.0 is released, only the latest minor version receives security fixes.

## Scope

In scope: the Pixsheaf services, container images, compose files, and CI configuration in this repository. Vulnerabilities in bundled libraries (for example libvips or image codecs) are welcome too; we will coordinate with upstream.

The design and threat model are documented in [docs/security.md](docs/security.md).
