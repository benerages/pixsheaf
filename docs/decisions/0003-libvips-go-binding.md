# ADR 0003: libvips Go binding

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Context

The worker needs AVIF, JPEG XL, HEIF, compositing, ICC transforms, and saving to a target size.

| | vipsgen v2 | govips v2 | `vips` CLI subprocess |
|---|---|---|---|
| Latest | v2.1.3 (2026-11-21) | v2.20.0 (2026-10-16) | Same as libvips |
| Commits in the last 12 months | 113 | 63 | — |
| Coverage | About 300 generated operations | 193+ generated operations plus hand-written I/O | One operation per process |
| `fail_on` / access mode | Every loader | Yes | Flags |
| Defaults | Cache 0, concurrency 1, vector off | Non-zero cache and concurrency | — |
| Block untrusted operations | Needs a shim | Needs a shim | Environment variable |
| Production use | imagor | Many users | — |

Sources: [vipsgen](https://github.com/cshum/vipsgen), [govips](https://github.com/davidbyttow/govips), [imagor](https://github.com/cshum/imagor). bimg was rejected because its last tag was in 2022 and it has had no commits in 12 months ([bimg](https://github.com/h2non/bimg)). vipsx was rejected because it was written in 4 days by one author and has been inactive since 2026-08 ([vipsx](https://github.com/kirklin/vipsx)).

## Decision

Use vipsgen v2 (`github.com/cshum/vipsgen/v2/vips`, pinned at ≥ v2.1.3) against libvips 8.18.x. Add about 10 lines of cgo shim in `internal/imaging` that calls `vips_operation_block_set` and `vips_block_untrusted_set`. Keep govips as the documented fallback and the CLI approach as an emergency fallback.

## Consequences

Both viable bindings depend on a single maintainer. The mitigation is that vipsgen is generated and MIT-licensed, so it can be forked and regenerated. The v2 API is new, so expect churn. Both bindings recently shipped memory fixes, so the binding stays inside the sandbox. Before approval, check the open-issue backlog by hand, because the GitHub API was unreachable during research.
