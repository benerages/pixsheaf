# Configuration

Pixsheaf is configured only through environment variables. Every setting has a secure default; you only need to set what you want to change. In `compose.yaml`, both services read an optional `.env` file in the same folder as `compose.yaml` (see [`.env.example`](../.env.example)).

Unknown `PIXSHEAF_*` variables are rejected at startup, so a typo cannot silently fall back to a default. Invalid values are rejected with an error that names the variable.

| Variable | Default | Allowed values | Description |
|---|---|---|---|
| `PIXSHEAF_LISTEN_ADDR` | `127.0.0.1:8080` (binary), `:8080` (API image) | `host:port` | Public HTTP address of the API. In the container it listens on all interfaces; `compose.yaml` publishes it on `127.0.0.1` only. |
| `PIXSHEAF_SOCKET` | `/run/pixsheaf/api.sock` | absolute path | Unix socket between API and workers (ADR 0007). Change only together with the volume mount. |
| `PIXSHEAF_MAX_UPLOAD_MB` | `25` | 1–1024 | Maximum size of one uploaded file. |
| `PIXSHEAF_MAX_FILES` | `50` | 1–10000 | Maximum number of files per job. |
| `PIXSHEAF_JOB_TIMEOUT` | `60s` | Go duration, 1s–30m | Maximum processing time per file. |
| `PIXSHEAF_MAX_CONCURRENCY` | `2` | 1–256 | Files processed in parallel across all clients. Each worker container also runs at most this many job processes, so its `mem_limit` should be at least this × `PIXSHEAF_JOB_MEMORY_MB` plus about 256 MB. |
| `PIXSHEAF_JOB_MEMORY_MB` | `1024` | 64–65536 | Memory limit of one job process (RLIMIT_DATA, ADR 0006). Keep it below the worker container's `mem_limit`. A job needs about 110 MB plus, per pixel of a large photo, roughly 2 bytes for JPEG, 4 for PNG, 7 for lossy WebP, and 25 for lossless WebP; the default covers JPEG, PNG, TIFF, and lossy WebP up to `PIXSHEAF_MAX_INPUT_PIXELS`, while AVIF, JPEG XL, and lossless WebP of large photos can need more. A file over the limit fails with `memory_limit_exceeded`. |
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
| `PIXSHEAF_MAX_JOBS_PER_CLIENT` | `10` | 1–100000 | Jobs one client may have open at the same time; beyond that `POST /jobs` answers `429`. Clients are told apart by IP address (ADR 0022). |
| `PIXSHEAF_REQUESTS_PER_MINUTE` | `300` | 10–100000 | API requests per client and minute, with a burst of the same size; beyond that `429` with `Retry-After`. Health check and UI files are not counted. |
| `PIXSHEAF_MAX_ARCHIVE_MB` | `100` | 1–32768, at most half of `PIXSHEAF_JOB_MEMORY_MB` | Size of one uploaded ZIP archive. The archive is unpacked in a job process that holds it in memory. |
| `PIXSHEAF_MAX_ARCHIVE_ENTRIES` | `1000` | 1–100000 | Entries (files and folders) in one ZIP archive. |
| `PIXSHEAF_TRUSTED_PROXIES` | empty | comma-separated CIDR prefixes | Reverse proxies whose `X-Forwarded-For` header is trusted. Empty: the header is ignored. |
| `PIXSHEAF_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` | Minimum log level. Rejected API requests are logged at `info`, security events (wrong token, cross-origin request, rate limit) at `warn`, successful requests only at `debug`. Logs never contain file contents, original file names, tokens, or plaintext IP addresses; clients appear as a short pseudonymous key that changes on every restart. |
| `PIXSHEAF_METRICS_ADDR` | empty (off) | empty or `host:port` | Address of the Prometheus endpoint `GET /metrics` (job, queue, and storage gauges; processed and failed files; responses by status class; security events). It carries no personal data, but keep it off the internet: publish it only on an internal network, for example `:9464` inside the container without a `ports:` entry. |

The development `compose.yaml` in the repository additionally reads `IMAGE_TAG` (default `latest`) to select the image version; it is deliberately not a `PIXSHEAF_*` variable. The `compose.yaml` attached to a release pins the images by digest instead.

