# ADR 0002: Server language

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Context

The project instructions default to Go and ask for an ADR on Rust (axum, tokio). The decoders are C either way, so memory safety at the API layer only partly reduces risk. Go 1.27.4 is current, and Go 1.26 is supported until 1.28 ships ([go.dev](https://go.dev/doc/devel/release)).

## Options

(a) Go with standard-library `net/http`. (b) Rust with axum and tokio.

## Decision

Go 1.27.x, using `net/http` pattern routing, `log/slog`, and `x/sync/errgroup`.

## Consequences

Static API binary with `CGO_ENABLED=0`. The worker needs cgo for libvips. The best-maintained libvips binding (vipsgen) and its production reference (imagor) are both Go ([imagor](https://github.com/cshum/imagor)). Rust's libvips bindings were **not evaluated** in this phase, so the decision rests on ecosystem fit and the project's default, not on a head-to-head comparison. Revisit if the worker's cgo layer causes recurring memory-safety incidents. Renovate tracks the Go toolchain version.
