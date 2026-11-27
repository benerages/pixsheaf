# Configuration

Pixsheaf is configured only through environment variables. Every setting has a secure default; you only need to set what you want to change. In `compose.yaml`, both services read an optional `.env` file in the repository root (see `.env.example`).

Unknown `PIXSHEAF_*` variables are rejected at startup, so a typo cannot silently fall back to a default. Invalid values are rejected with an error that names the variable.

| Variable | Default | Allowed values | Description |
|---|---|---|---|
| `PIXSHEAF_LISTEN_ADDR` | `127.0.0.1:8080` (binary), `:8080` (API image) | `host:port` | Public HTTP address of the API. In the container it listens on all interfaces; `compose.yaml` publishes it on `127.0.0.1` only. |
| `PIXSHEAF_SOCKET` | `/run/pixsheaf/api.sock` | absolute path | Unix socket between API and workers (ADR 0007). Change only together with the volume mount. |
| `PIXSHEAF_MAX_UPLOAD_MB` | `25` | 1–1024 | Maximum size of one uploaded file. |
| `PIXSHEAF_MAX_FILES` | `50` | 1–10000 | Maximum number of files per job. |
| `PIXSHEAF_JOB_TIMEOUT` | `60s` | Go duration, 1s–30m | Maximum processing time per file. |
| `PIXSHEAF_MAX_CONCURRENCY` | `2` | 1–256 | Files processed in parallel across all clients. |
| `PIXSHEAF_JOB_MEMORY_MB` | `384` | 64–65536 | Memory limit of one job process (RLIMIT_DATA, ADR 0006). Keep it below the worker container's `mem_limit`. |
| `PIXSHEAF_TRUSTED_PROXIES` | empty | comma-separated CIDR prefixes | Reverse proxies whose `X-Forwarded-For` header is trusted. Empty: the header is ignored. |
| `PIXSHEAF_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` | Minimum log level. Logs never contain file contents, original file names, or plaintext IP addresses. |

`compose.yaml` additionally reads `IMAGE_TAG` (default `latest`) to select the image version. It is deliberately not a `PIXSHEAF_*` variable.

Settings for rate limits, job lifetime, pixel limits, and disabled formats (`PIXSHEAF_DISABLE_FORMATS`, ADR 0016) are added with the features that use them in Phases 3 and 4.
