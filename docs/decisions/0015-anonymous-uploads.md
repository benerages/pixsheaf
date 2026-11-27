# ADR 0015: Anonymous uploads with compensating controls

- Status: Accepted
- Date: 2026-11-27

## Context

The OWASP File Upload Cheat Sheet recommends allowing uploads only to authenticated users. Pixsheaf v1.0 is meant to be usable without accounts: the start page lets anyone process an image immediately. Built-in authentication (OIDC or forward auth) is a "Could" item after v1.0. The project owner decided that anonymous uploads are desired, with compensating controls.

## Options

(a) Anonymous uploads with compensating controls. (b) Mandatory forward auth or OIDC in v1.0.

## Decision

Option (a), with these compensating controls, all enabled by default and configurable via environment variables:

- **Exposure:** the API binds to `127.0.0.1` by default; publishing it requires an explicit setting and is documented only behind a reverse proxy with TLS.
- **Rate limiting per client IP:** requests per minute, concurrent jobs, open jobs, and bytes uploaded per time window. Client IPs are taken from `X-Forwarded-For` only when the request comes from `PIXSHEAF_TRUSTED_PROXIES`.
- **Global backpressure:** a fixed queue size; when full, new jobs get HTTP 429 with `Retry-After`.
- **Short lifetimes:** unclaimed jobs expire after a short TTL; results can be downloaded once and are deleted afterwards; job IDs have at least 128 bits of entropy.
- **Size limits:** per-file size, files per job, total job size, pixel and frame limits (ADR 0008, ADR 0014).
- **CSRF and CORS:** same-origin check plus token for state-changing requests; CORS off by default.
- **Privacy:** IP addresses are only held in memory for rate limiting and never logged in plaintext.
- **Documentation:** `docs/deployment.md` includes forward-auth examples for Traefik, Caddy, and nginx for operators who want to restrict access.

## Consequences

- Residual risk (resource abuse by many IPs, use as an anonymous conversion service) is recorded in the threat model and mitigated by the operator's reverse proxy and network placement.
- Load tests must show that the rate limits and the queue cap keep memory and CPU within container limits under abuse-like traffic.
- Adding built-in authentication later must not change the job model; it will be a middleware in front of the existing API.
