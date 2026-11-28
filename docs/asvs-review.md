# OWASP ASVS 5.0 Level 2 review

This review checks Pixsheaf 1.0 against every Level 1 and Level 2 requirement of the [OWASP Application Security Verification Standard 5.0](https://github.com/OWASP/ASVS/tree/master/5.0) (253 requirements; requirement IDs as in the standard). It was done in Phase 6 against the code on `main` and complements the threat model in [security.md](security.md). The requirement texts are not repeated here; the topic column is a short paraphrase.

Status values:

- **Met**: implemented and, where possible, covered by tests.
- **Fixed in Phase 6**: a gap found by this review and closed before the release.
- **Deviation**: deliberately not met; the reason and the compensating controls are documented (ADR or section below).
- **Operator**: depends on the deployment, mostly the TLS reverse proxy; [deployment.md](deployment.md) says what to configure.
- **N/A**: the feature the requirement is about does not exist in Pixsheaf.

## Summary

| Status | Requirements |
|---|---|
| Met | 112 |
| Fixed in Phase 6 | 14 |
| Deviation | 3 |
| Operator | 9 |
| N/A | 115 |
| **Total** | **253** |

## Findings fixed in Phase 6

| ID | Finding | Fix |
|---|---|---|
| 3.3.1, 3.3.3 | The job cookie had no `__Host-` prefix. | Cookie `__Host-pixsheaf-{jobId}` with `Path=/`; the job ID in the name binds it to its job (ADR 0019 amendment). |
| 4.1.1 | JSON responses had no `charset` parameter. | `application/json; charset=utf-8`, likewise for problem details and the event stream. |
| 5.2.2 | File names were ignored completely. | Uploads and archive entries need an image or ZIP extension (ADR 0023). |
| 13.4.3 | The UI file server listed folders such as `/assets/`. | Folder paths answer 404. |
| 16.1.1, 16.2.1, 16.3.2, 16.3.3 | Only internal errors were logged. | Every rejected API request is logged with route, status, code, job ID, and a pseudonymous client key; security events at warn. |
| 11.1.1, 11.1.2 | No cryptographic inventory. | Section below. |
| 15.1.1 | No remediation time frames. | `SECURITY.md`. |
| 15.1.2 | No SBOM yet. | Release workflow attaches SPDX SBOMs. |
| 13.4.5 | Monitoring did not exist yet (project requirement). | Prometheus endpoint on a separate address, off by default. |

The review also found that `PIXSHEAF_DISABLE_FORMATS`, promised by ADR 0016 as the way to switch HEIC off, had never been implemented. It now works in the API (415 `format_disabled`, missing from `/capabilities`, archive entries fail individually) and in the job process, which keeps the libvips loader blocked and checks the decoded format again, so a file that claims AVIF but holds HEVC is refused too.

## Deviations

| ID | Deviation | Compensating controls |
|---|---|---|
| 5.4.3 | No antivirus scanning. | Owner decision, ADR 0014: only images are accepted, decoded in a sandbox without network, and always re-encoded; originals are never served. |
| 5.2.2 (second part) | An allowed extension that does not match the content is accepted. | ADR 0023: the content decides the format and is re-encoded; outputs are served as attachments with a fixed type and `nosniff`. |
| 12.3.1, 12.3.3 | API and workers talk plain HTTP over a Unix socket. | ADR 0007: the socket is on a private volume shared only by these containers, mode 0600 in a 0700 directory, same non-root UID, no network path; workers have no network at all. The metrics endpoint is plain HTTP, off by default, and must stay on an internal network. |

## Operator responsibilities

TLS (12.1.1, 12.1.2, 12.2.1, 12.2.2), HSTS with at least one year and `includeSubDomains` (3.4.1), HTTP-to-HTTPS redirects for browsers only (4.1.2), a hostname of its own (3.5.4), and protected, shipped logs (16.4.2, 16.4.3) belong to the deployment. [deployment.md](deployment.md) describes them.

## Cryptographic inventory

| Use | Algorithm | Key or secret | Lifetime |
|---|---|---|---|
| Job tokens | 256 random bits from `crypto/rand`, base64url | none | Until the job is downloaded, deleted, or expires |
| Stored token check | SHA-256 of the token, compared in constant time | none | As the job |
| Job and file IDs | 128 random bits from `crypto/rand` | none | As the job |
| Client key for limits and logs | HMAC-SHA-256 of the client address (IPv6 per /64), truncated to 128 bits | 256-bit secret from `crypto/rand`, in memory only | Until the API restarts; never written anywhere |
| Transport | TLS | Certificate of the operator's proxy | Operator |
| Release signatures | Sigstore keyless signing (cosign) with GitHub OIDC; SLSA provenance | No long-lived key | Per release |

No key is stored on disk or in configuration, so there is nothing to rotate; a restart replaces the only in-memory secret. Changing an algorithm needs no data migration, because nothing cryptographic is persisted.

## Requirements

Chapters without any applicable feature: V6 Authentication: no accounts or logins (ADR 0015).; V9 Self-contained tokens: no JWTs; job tokens are opaque reference tokens (see V7).; V10 OAuth and OIDC: not used.; V17 WebRTC: not used. Together these are 78 requirements, all N/A.


### V1 Encoding and Sanitization

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 1.1.1 | 2 | Canonical decoding once | Met | Bodies are decoded once by `encoding/json` and `mime/multipart` before validation. |
| 1.1.2 | 2 | Output encoding at the interpreter | Met | `encoding/json`, Svelte text interpolation, `net/http` headers. |
| 1.2.1 | 1 | Context-aware output encoding | Met | Svelte escapes all interpolations; `{@html}` is forbidden by ESLint; `Content-Disposition` per RFC 6266/8187. |
| 1.2.2 | 1 | Safe URL building | Met | URLs contain only validated base64url IDs; the UI links only to `/api/v1/...` and `blob:` URLs it created. |
| 1.2.3 | 1 | JSON/JS injection | Met | JSON is produced by `encoding/json`; no dynamic script. |
| 1.2.4 | 1 | Database injection | N/A | No database. |
| 1.2.5 | 1 | OS command injection | Met | No shell. The worker starts its own binary with fixed arguments; inputs travel over pipes. |
| 1.2.6 | 2 | LDAP injection | N/A | No LDAP. |
| 1.2.7 | 2 | XPath injection | N/A | No XPath. |
| 1.2.8 | 2 | LaTeX injection | N/A | No LaTeX. |
| 1.2.9 | 2 | Regex injection | Met | No regular expression is built from input. |
| 1.3.1 | 1 | HTML sanitization | N/A | No HTML input. |
| 1.3.2 | 1 | No eval | Met | No `eval`; the CSP has no `unsafe-eval`. |
| 1.3.3 | 2 | Sanitize for dangerous contexts | Met | Output names are sanitized per segment; watermark text and patterns have length limits. |
| 1.3.4 | 2 | SVG | Met | SVG is never accepted (magic-byte allowlist, ADR 0014). |
| 1.3.5 | 2 | Scriptable templates | Met | No Markdown, CSS, XSL, or similar input is accepted. |
| 1.3.6 | 2 | SSRF | Met | The API makes no outbound requests; workers have no network. |
| 1.3.7 | 2 | Template injection | Met | Naming patterns replace a fixed set of placeholders; there is no template engine. |
| 1.3.8 | 2 | JNDI | N/A | Not Java. |
| 1.3.9 | 2 | Memcache | N/A | No memcache. |
| 1.3.10 | 2 | Format strings | Met | Format strings are constants. |
| 1.3.11 | 2 | Mail injection | N/A | No mail. |
| 1.4.1 | 2 | Memory-safe code | Met | Go is memory-safe; the C codecs run in a per-file sandbox (ADR 0006) and are treated as compromised. |
| 1.4.2 | 2 | Integer overflow | Met | Sizes and pixel counts are range-checked in 64 bits before use; gosec G115 is enforced. |
| 1.4.3 | 2 | Use-after-free | Met | Go is garbage-collected; libvips objects are freed in one place per call, inside the sandbox. |
| 1.5.1 | 1 | XML parsers | N/A | No XML parser; XMP metadata is kept or stripped as an opaque blob. |
| 1.5.2 | 2 | Safe deserialization | Met | JSON is decoded into fixed types; no polymorphic deserialization. |

### V2 Validation and Business Logic

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 2.1.1 | 1 | Input rules documented | Met | OpenAPI 3.1 with JSON Schema (`api/openapi.yaml`), ADR 0009, ADR 0019. |
| 2.1.2 | 2 | Combined-data rules documented | Met | Mutually exclusive parameters are documented in the specification (ADR 0019). |
| 2.1.3 | 2 | Business limits documented | Met | `docs/configuration.md` lists per-client and global limits. |
| 2.2.1 | 1 | Positive validation | Met | Allowlists and ranges in the pipeline validator; `additionalProperties: false`. |
| 2.2.2 | 1 | Server-side validation | Met | The API validates everything; the UI's checks are conveniences. |
| 2.2.3 | 2 | Combined items checked | Met | The pipeline validator rejects conflicting parameters. |
| 2.3.1 | 1 | Steps in order | Met | Job states are enforced: no uploads beyond `expectedFiles` (409), no download before done (409), one download. |
| 2.3.2 | 2 | Business limits enforced | Met | Limits from `docs/configuration.md` are enforced by the job manager. |
| 2.3.3 | 2 | Atomic operations | Met | Job changes happen under one lock; a rejected upload does not use up a slot. |
| 2.3.4 | 2 | No double booking | Met | Upload slots and queue places are counted under the lock. |
| 2.4.1 | 2 | Anti-automation | Met | Per-client request rate, open jobs, queue size, and storage limits (ADR 0022). |

### V3 Web Frontend Security

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 3.2.1 | 1 | Correct rendering context | Met | Downloads are `attachment` with `nosniff` and `CSP: default-src 'none'; sandbox`; API responses are JSON. |
| 3.2.2 | 1 | Safe text rendering | Met | Svelte renders text nodes; `{@html}` is forbidden. |
| 3.3.1 | 1 | Secure cookie, prefix | Fixed in Phase 6 | The job cookie is now `__Host-pixsheaf-{jobId}` with `Secure` (ADR 0019 amendment). |
| 3.3.2 | 2 | SameSite | Met | `SameSite=Strict`. |
| 3.3.3 | 2 | __Host- prefix | Fixed in Phase 6 | See 3.3.1; the job ID in the name keeps parallel jobs apart. |
| 3.3.4 | 2 | HttpOnly | Met | `HttpOnly`; the token is never placed where scripts can read it after creation. |
| 3.4.1 | 1 | HSTS | Operator | Sent by the API when it terminates TLS itself; behind a proxy the proxy must send it with at least one year and `includeSubDomains` (`docs/deployment.md`). |
| 3.4.2 | 1 | CORS fixed or allowlisted | Met | CORS is off: no `Access-Control-*` headers are sent. |
| 3.4.3 | 2 | CSP | Met | Strict CSP without `unsafe-inline` or `unsafe-eval`; browser tests fail on violations. |
| 3.4.4 | 2 | nosniff | Met | On every response, including the metrics server. |
| 3.4.5 | 2 | Referrer policy | Met | `Referrer-Policy: no-referrer`. |
| 3.4.6 | 2 | frame-ancestors | Met | `frame-ancestors 'none'` plus `X-Frame-Options: DENY`. |
| 3.5.1 | 1 | Cross-origin request validation | Met | State-changing requests check `Sec-Fetch-Site` and `Origin`; `POST /jobs` requires JSON; the token travels in a header. |
| 3.5.2 | 1 | Preflight not bypassable | Met | No reliance on preflight; see 3.5.1. |
| 3.5.3 | 1 | Unsafe methods for changes | Met | Changes use POST and DELETE. The one-time download deletes the job as cleanup; its cookie is `SameSite=Strict`, so another site cannot trigger it. |
| 3.5.4 | 2 | Separate hostnames | Operator | Pixsheaf is one application; operators should give it its own hostname (`docs/deployment.md`). |
| 3.5.5 | 2 | postMessage | N/A | Not used. |
| 3.7.1 | 2 | Supported client technologies | Met | Plain HTML, CSS, and JavaScript. |
| 3.7.2 | 2 | Redirect allowlist | Met | No redirects to other hosts. |

### V4 API and Web Service

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 4.1.1 | 1 | Content-Type with charset | Fixed in Phase 6 | JSON, problem details, and the event stream now declare `charset=utf-8`; static files get it from `net/http`. |
| 4.1.2 | 2 | HTTP to HTTPS redirects | Operator | The API never redirects; only the operator's proxy may redirect browser traffic. |
| 4.1.3 | 2 | Proxy headers not user-controlled | Met | `X-Forwarded-For` is honored only from `PIXSHEAF_TRUSTED_PROXIES` (ADR 0022). |
| 4.2.1 | 2 | Request smuggling | Met | Go `net/http` parses message boundaries; the proxy is the operator's (`docs/deployment.md`). |
| 4.3.1 | 2 | GraphQL limits | N/A | No GraphQL. |
| 4.3.2 | 2 | GraphQL introspection | N/A | No GraphQL. |
| 4.4.1 | 1 | WSS | N/A | No WebSocket; progress uses Server-Sent Events. |
| 4.4.2 | 2 | WebSocket origin | N/A | No WebSocket. |
| 4.4.3 | 2 | WebSocket tokens | N/A | No WebSocket. |
| 4.4.4 | 2 | WebSocket session | N/A | No WebSocket. |

### V5 File Handling

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 5.1.1 | 2 | File types and sizes documented | Met | `docs/configuration.md`, `api/openapi.yaml` (formats, extensions, sizes, archive limits). |
| 5.2.1 | 1 | Size limits | Met | Upload, job, archive, output, pixel, and frame limits; checks before decoding (ADR 0008). |
| 5.2.2 | 1 | Extension and content | Fixed in Phase 6 | An image or ZIP extension is now required for uploads and archive entries; content decides the format (ADR 0023, deliberate deviation for mismatches). |
| 5.2.3 | 2 | Archive limits before unpacking | Met | Entry count, sizes, and compression ratio are checked in the sandbox (ADR 0022). |
| 5.3.1 | 1 | No execution of uploads | Met | Uploads live on a `noexec` tmpfs under random names and are never served. |
| 5.3.2 | 1 | No user file names in paths | Met | Random IDs inside `os.Root`. |
| 5.4.1 | 2 | File names ignored or validated | Met | Names are sanitized; `Content-Disposition` is always set. |
| 5.4.2 | 2 | File names encoded | Met | RFC 6266 with an ASCII fallback and RFC 8187 `filename*`; fuzz-tested. |
| 5.4.3 | 2 | Antivirus scanning | Deviation | Not done; images only, always re-encoded (ADR 0014, owner decision). |

### V7 Session Management (the job token)

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 7.1.1 | 2 | Session lifetimes documented | Met | A job and its token live at most `PIXSHEAF_JOB_TTL` (15 minutes by default) and end with the download (ADR 0019). |
| 7.1.2 | 2 | Concurrent sessions documented | Met | No accounts; open jobs per client are capped (`PIXSHEAF_MAX_JOBS_PER_CLIENT`). |
| 7.1.3 | 2 | Federated sessions | N/A | No federation. |
| 7.2.1 | 1 | Backend token verification | Met | The API compares a SHA-256 hash in constant time. |
| 7.2.2 | 1 | Dynamic tokens | Met | A new token per job. |
| 7.2.3 | 1 | Token entropy | Met | 256 bits from `crypto/rand`. |
| 7.2.4 | 1 | New token on authentication | N/A | No authentication (ADR 0015). |
| 7.3.1 | 2 | Inactivity timeout | Met | The job expires after the TTL regardless of activity. |
| 7.3.2 | 2 | Absolute lifetime | Met | The TTL is absolute. |
| 7.4.1 | 1 | Termination invalidates | Met | Deleting, downloading, or expiry removes the job; the token no longer works. |
| 7.4.2 | 1 | Terminate on account removal | N/A | No accounts. |
| 7.4.3 | 2 | Terminate after factor change | N/A | No accounts. |
| 7.4.4 | 2 | Visible logout | Met | "Start over" deletes the job on the server. |
| 7.4.5 | 2 | Admin session termination | N/A | No administrators; a restart drops all jobs. |
| 7.5.1 | 2 | Re-authentication for account changes | N/A | No accounts. |
| 7.5.2 | 2 | View and end sessions | N/A | No accounts. |
| 7.6.1 | 2 | RP/IdP sessions | N/A | No federation. |
| 7.6.2 | 2 | Session needs user action | Met | A job is created only when the user picks files. |

### V8 Authorization

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 8.1.1 | 1 | Authorization rules documented | Met | ADR 0019: the job token is a capability for one job; state changes need the header. |
| 8.1.2 | 2 | Field-level rules documented | Met | A token holder sees the whole job; nothing about other jobs or clients is exposed. |
| 8.2.1 | 1 | Function-level access | Met | Every job route checks the token. |
| 8.2.2 | 1 | Data-level access (IDOR) | Met | ID plus token per job; the cookie name binds it to its job (new test). |
| 8.2.3 | 2 | Field-level access | Met | Responses are built from fixed types; the token hash and client key are never returned. |
| 8.3.1 | 1 | Trusted enforcement layer | Met | Enforced in the API. |
| 8.4.1 | 2 | Cross-tenant isolation | Met | Per-job tokens; one sandboxed process per file. |

### V11 Cryptography

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 11.1.1 | 2 | Key management policy | Fixed in Phase 6 | Documented below ("Cryptographic inventory"): no long-lived keys; the client-key HMAC secret exists only in memory and changes on every start. |
| 11.1.2 | 2 | Cryptographic inventory | Fixed in Phase 6 | Documented below. |
| 11.2.1 | 2 | Validated implementations | Met | Go standard library (`crypto/rand`, `crypto/sha256`, `crypto/hmac`). |
| 11.2.2 | 2 | Crypto agility | Met | Algorithms are used in one place each and are not persisted; changing them needs no migration. |
| 11.2.3 | 2 | 128-bit security | Met | Tokens 256 bits, IDs 128 bits, SHA-256, HMAC-SHA-256. |
| 11.3.1 | 1 | No ECB or weak padding | N/A | No encryption in the application; TLS is the proxy's. |
| 11.3.2 | 1 | Approved ciphers | N/A | No encryption in the application. |
| 11.3.3 | 2 | Authenticated encryption | N/A | No encryption in the application. |
| 11.4.1 | 1 | Approved hashes | Met | SHA-256 only. |
| 11.4.2 | 2 | Password hashing | N/A | No passwords. |
| 11.4.3 | 2 | Collision-resistant hashes | Met | SHA-256. |
| 11.4.4 | 2 | Password-based KDF | N/A | No passwords. |
| 11.5.1 | 2 | CSPRNG for secrets | Met | `crypto/rand` for tokens, IDs, and the HMAC secret. |
| 11.6.1 | 2 | Key generation and signatures | Met | Release signing uses Sigstore keyless signing (cosign); the application generates no keys. |

### V12 Secure Communication

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 12.1.1 | 1 | TLS versions | Operator | TLS is terminated by the operator's proxy (`docs/deployment.md`). |
| 12.1.2 | 2 | Cipher suites | Operator | Proxy configuration. |
| 12.1.3 | 2 | mTLS client certificates | N/A | Not used. |
| 12.2.1 | 1 | TLS for external services | Operator | The proxy serves HTTPS; the API listens on loopback only by default. |
| 12.2.2 | 1 | Public certificates | Operator | Proxy configuration. |
| 12.3.1 | 2 | Encrypted internal connections | Deviation | API and workers talk HTTP over a Unix socket on a private volume, not over a network; the metrics endpoint is plain HTTP and off by default (see Deviations). |
| 12.3.2 | 2 | TLS clients validate certificates | N/A | No outbound TLS. |
| 12.3.3 | 2 | TLS between internal HTTP services | Deviation | See 12.3.1 (ADR 0007). |
| 12.3.4 | 2 | Trusted internal certificates | N/A | No internal TLS. |

### V13 Configuration

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 13.1.1 | 2 | Communication documented | Met | `docs/security.md`: no external services; trust boundaries TB1-TB6. |
| 13.2.1 | 2 | Backend components authenticated | Met | The socket is mode 0600 in a 0700 directory on a volume shared by API and workers only; same UID (ADR 0007). |
| 13.2.2 | 2 | Least privilege | Met | Non-root UID 10001, no capabilities, read-only root, no network for workers. |
| 13.2.3 | 2 | No default credentials | N/A | No credentials. |
| 13.2.4 | 2 | Outbound allowlist | Met | No outbound communication; workers have `network_mode: none`. |
| 13.2.5 | 2 | Server resource allowlist | Met | Files only inside `os.Root` on the tmpfs; Landlock in job processes. |
| 13.3.1 | 2 | Secrets management | N/A | No backend secrets; the HMAC secret is generated in memory. |
| 13.3.2 | 2 | Least privilege for secrets | N/A | See 13.3.1. |
| 13.4.1 | 1 | No source control metadata | Met | `.dockerignore` excludes `.git`; the API image contains only the binary. |
| 13.4.2 | 2 | Debug modes off | Met | No pprof, expvar, or debug endpoints. |
| 13.4.3 | 2 | No directory listings | Fixed in Phase 6 | The UI file server listed folders such as `/assets/`; folder paths now get 404 (new test). |
| 13.4.4 | 2 | No TRACE | Met | Routes are method-specific; TRACE gets 405. |
| 13.4.5 | 2 | Docs and monitoring not exposed | Fixed in Phase 6 | The new metrics endpoint is off by default and on a separate address. |

### V14 Data Protection

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 14.1.1 | 2 | Sensitive data classified | Met | `docs/security.md` assets A1-A6. |
| 14.1.2 | 2 | Protection requirements | Met | `docs/security.md` and this review. |
| 14.2.1 | 1 | No secrets in URLs | Met | Tokens travel in the `Authorization` header or the cookie, never in URLs. |
| 14.2.2 | 2 | No caching in server components | Met | `Cache-Control: no-store` on API responses; the operator's proxy must not cache `/api/` (`docs/deployment.md`). |
| 14.2.3 | 2 | No data to untrusted parties | Met | No telemetry, no external fonts or CDNs. |
| 14.2.4 | 2 | Data protection controls | Met | Metadata stripped by default; files in RAM only; deletion after download, cancel, or TTL (tested). |
| 14.3.1 | 1 | Client data cleared | Met | "Start over" deletes the job and revokes object URLs; the cookie is useless after deletion and expires with the job. |
| 14.3.2 | 2 | Anti-caching headers | Met | `Cache-Control: no-store`. |
| 14.3.3 | 2 | No sensitive data in browser storage | Met | `localStorage` holds only the language and custom presets. |

### V15 Secure Coding and Architecture

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 15.1.1 | 1 | Remediation time frames | Fixed in Phase 6 | Now in `SECURITY.md`. |
| 15.1.2 | 2 | SBOM | Fixed in Phase 6 | Every release carries SPDX SBOMs, attested with the image (release workflow). |
| 15.1.3 | 2 | Resource-heavy functions documented | Met | `docs/security.md` D1-D7; `docs/configuration.md`. |
| 15.2.1 | 1 | Components within time frames | Met | Renovate with a weekly rebuild; govulncheck, Grype, and `pnpm audit` gate CI. |
| 15.2.2 | 2 | Availability defenses | Met | Limits, fair queue, backpressure (503 with `Retry-After`), per-file timeouts. |
| 15.2.3 | 2 | No extraneous functionality | Met | No test code, samples, or debug features in the images. |
| 15.3.1 | 1 | Only required fields returned | Met | Responses are built from fixed API types. |
| 15.3.2 | 2 | No redirect following | N/A | No outbound requests. |
| 15.3.3 | 2 | Mass assignment | Met | `additionalProperties: false`; requests are decoded into fixed types. |
| 15.3.4 | 2 | Original IP from trusted fields | Met | Rightmost untrusted `X-Forwarded-For` entry behind trusted proxies (ADR 0022). |
| 15.3.5 | 2 | Strict types | Met | Go types; TypeScript `strict` with `noUncheckedIndexedAccess`. |
| 15.3.6 | 2 | Prototype pollution | Met | No merging of untrusted objects; imported presets are validated and sent to the server as JSON. |
| 15.3.7 | 2 | Parameter pollution | Met | Each parameter has one source (path or body); query strings are not read. |

### V16 Security Logging and Error Handling

| ID | L | Topic | Status | Evidence |
|---|---|---|---|---|
| 16.1.1 | 2 | Logging inventory | Fixed in Phase 6 | `docs/security.md` "Logging". |
| 16.2.1 | 2 | Log metadata | Fixed in Phase 6 | Time, event, route, status, problem code, job ID, and a pseudonymous client key. |
| 16.2.2 | 2 | UTC timestamps | Met | `log/slog` JSON timestamps carry their offset; the containers run in UTC. |
| 16.2.3 | 2 | Logs only to documented sinks | Met | Standard output only. |
| 16.2.4 | 2 | Common log format | Met | JSON lines. |
| 16.2.5 | 2 | Sensitive data in logs | Met | No file contents, original names, tokens, or IP addresses (tested). |
| 16.3.1 | 2 | Authentication logged | N/A | No authentication; token failures are covered by 16.3.2. |
| 16.3.2 | 2 | Failed authorization logged | Fixed in Phase 6 | `auth_failed` at warn. |
| 16.3.3 | 2 | Security events and bypass attempts | Fixed in Phase 6 | `cross_origin_rejected`, `rate_limited` at warn; rejected input at info. |
| 16.3.4 | 2 | Unexpected errors logged | Met | Server errors at error level. |
| 16.4.1 | 2 | Log injection | Met | Structured JSON encoding. |
| 16.4.2 | 2 | Logs protected | Operator | Container log driver of the host. |
| 16.4.3 | 2 | Logs shipped elsewhere | Operator | Operator's log collection. |
| 16.5.1 | 2 | Generic error messages | Met | Problem details with stable codes; internal errors return no detail. |
| 16.5.2 | 2 | Graceful degradation | Met | Without workers, files wait in the queue; a full queue answers 503 with `Retry-After`. |
| 16.5.3 | 2 | Fail closed | Met | Validation errors stop the request; a failing file never affects others. |
