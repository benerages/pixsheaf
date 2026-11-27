# ADR 0011: OpenAPI tooling

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Options

oapi-codegen; ogen; hand-written code only.

## Decision

oapi-codegen v2.8.2 generates models and the strict `net/http` server interface. OpenAPI 3.1 support has existed since v2.8.0 but is self-described as "initial" ([releases](https://github.com/oapi-codegen/oapi-codegen/releases)). Upload, SSE, and ZIP-download handlers are hand-written against the generated interface. On the client, openapi-typescript 7.x generates types only, and openapi-fetch 0.17 is pinned exactly. CI lints the spec with Redocly CLI.

## Consequences

No router dependency. ogen was rejected because it generates SSE clients only and buffers JSON by default ([ogen](https://github.com/ogen-go/ogen)). Keep the spec within the 3.1 subset oapi-codegen supports, and fall back to 3.0-compatible constructs if generation fails. Pin oapi-codegen at or above v2.8.2 because of the earlier code-injection fixes.
