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
| `PIXSHEAF_MAX_CONCURRENCY` | `2` | 1–256 | Files processed in parallel across all clients. Each worker container also runs at most this many job processes, so its `mem_limit` should be at least this × `PIXSHEAF_JOB_MEMORY_MB` plus about 128 MB. |
| `PIXSHEAF_JOB_MEMORY_MB` | `384` | 64–65536 | Memory limit of one job process (RLIMIT_DATA, ADR 0006). Keep it below the worker container's `mem_limit`. |
| `PIXSHEAF_MAX_JOB_MB` | `250` | 1–65536 | Maximum size of all uploads of one job together. |
| `PIXSHEAF_MAX_OUTPUT_MB` | `100` | 1–256 | Maximum size of one processed file. |
| `PIXSHEAF_MAX_INPUT_PIXELS` | `50000000` | 1–1000000000 | Maximum width × height × frames of an upload, checked from the header before decoding. |
| `PIXSHEAF_MAX_OUTPUT_PIXELS` | `50000000` | 1–1000000000 | Maximum width × height of any intermediate or final image. |
| `PIXSHEAF_MAX_FRAMES` | `300` | 1–1000 | Maximum frames of an animated GIF or WebP. |
| `PIXSHEAF_MAX_WATERMARK_KB` | `512` | 0–1024 | Maximum size of an image watermark; `0` disables image watermarks. |
| `PIXSHEAF_MAX_QUEUE` | `100` | 1–100000 | Files waiting for a worker. Further uploads get `503` with `Retry-After`. |
| `PIXSHEAF_MAX_JOBS` | `200` | 1–100000 | Jobs kept in memory at the same time. |
| `PIXSHEAF_JOB_TTL` | `15m` | Go duration, 1m–24h | Lifetime of a job that is not downloaded; then it and its files are deleted. |
| `PIXSHEAF_TMP_DIR` | `/tmp/pixsheaf` | absolute path | Directory for uploads and results. Must be on a tmpfs; the API empties it on start and on shutdown. |
| `PIXSHEAF_MAX_STORE_MB` | `384` | 1–1048576 | All uploads and results together. Keep it below the size of the API's `/tmp` tmpfs (512 MB in `compose.yaml`); when it is reached, uploads get `503`. |
| `PIXSHEAF_TRUSTED_PROXIES` | empty | comma-separated CIDR prefixes | Reverse proxies whose `X-Forwarded-For` header is trusted. Empty: the header is ignored. |
| `PIXSHEAF_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` | Minimum log level. Logs never contain file contents, original file names, or plaintext IP addresses. |

`compose.yaml` additionally reads `IMAGE_TAG` (default `latest`) to select the image version. It is deliberately not a `PIXSHEAF_*` variable.

Settings for rate limits, job lifetime, pixel limits, and disabled formats (`PIXSHEAF_DISABLE_FORMATS`, ADR 0016) are added with the features that use them in Phases 3 and 4.
