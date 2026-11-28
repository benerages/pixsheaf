# Deployment

Pixsheaf ships as two images started by `compose.yaml`:

| Service | Image | Network | Writable |
|---|---|---|---|
| `api` | `ghcr.io/benerages/pixsheaf-api` (distroless) | bridge, published on `127.0.0.1:8080` | `/tmp` (tmpfs), socket volume |
| `worker` | `ghcr.io/benerages/pixsheaf-worker` | none (`network_mode: none`) | `/tmp` (tmpfs, noexec) |

Both run as UID 10001 with a read-only root filesystem, no Linux capabilities, `no-new-privileges`, and memory, CPU, and process limits. Scale workers with `docker compose up -d --scale worker=3`.

## Publishing behind a reverse proxy

The API binds to the loopback interface of the host. Put a TLS-terminating reverse proxy in front of it and set `PIXSHEAF_TRUSTED_PROXIES` to the proxy's address so client IPs are used for rate limiting.

The proxy is responsible for (see [asvs-review.md](asvs-review.md), "Operator responsibilities"):

- TLS 1.2 and 1.3 only, with a publicly trusted certificate;
- `Strict-Transport-Security: max-age=63072000; includeSubDomains`;
- redirecting plain HTTP to HTTPS for browsers;
- a hostname of its own for Pixsheaf, not a path on a site shared with other applications;
- not caching `/api/` responses (the API sends `Cache-Control: no-store`) and not buffering the event stream `/api/v1/jobs/{id}/events`;
- request bodies at least as large as `PIXSHEAF_MAX_UPLOAD_MB` and `PIXSHEAF_MAX_ARCHIVE_MB`.

Caddy does most of this by default:

```caddyfile
pixsheaf.example.org {
	header Strict-Transport-Security "max-age=63072000; includeSubDomains"
	reverse_proxy 127.0.0.1:8080
}
```

With nginx:

```nginx
server {
    listen 443 ssl;
    server_name pixsheaf.example.org;
    ssl_protocols TLSv1.2 TLSv1.3;
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains" always;
    client_max_body_size 110m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_buffering off;
        proxy_read_timeout 1h;
    }
}
```

Serve Pixsheaf over HTTPS. The job cookie is always `Secure`; browsers send it over plain HTTP only to `localhost` and `127.0.0.1`. Over plain HTTP on another address, event streams and download links that rely on the cookie do not work.

Per-client limits (open jobs, requests per minute) and the fair queue tell clients apart by IP address (ADR 0022). The API sees the address of the connection's peer. With the default port mapping (`127.0.0.1:8080:8080`), a proxy on the host reaches the API through Docker's bridge, so the peer is the bridge gateway (for example `172.18.0.1`), not `127.0.0.1`. Add the network the proxy connects from to `PIXSHEAF_TRUSTED_PROXIES` (for example the `pixsheaf_frontend` subnet shown by `docker network inspect pixsheaf_frontend`). Without it, all clients behind the proxy share one identity and one set of limits. `X-Forwarded-For` is read only from trusted peers, and only its rightmost untrusted entry counts, so clients cannot choose their identity.

Uploads are anonymous by default (ADR 0015). To restrict access, use forward authentication or basic authentication in the proxy; Pixsheaf itself needs no change.

## Logs and metrics

Both services log JSON lines to standard output; collect them with Docker's log driver. What is logged, and what never is, is described in [security.md](security.md#logging).

Prometheus metrics are off by default. To enable them, set `PIXSHEAF_METRICS_ADDR=:9464` for the API and let your Prometheus reach the API container on an internal Docker network. Do not add a `ports:` entry for it.

## Optional: gVisor

To run the workers under gVisor, install `runsc`, register it as a Docker runtime, and start with:

```sh
docker compose -f compose.yaml -f deploy/compose.gvisor.yaml up -d
```

## Hosts with `userns-remap`

Pixsheaf works with Docker's `userns-remap`. The socket volume takes its ownership from the images, not from tmpfs mount options, which would refer to host UIDs (ADR 0007, amendment).

## Verifying the isolation

`tests/container/run.sh` builds both images and runs `pixsheaf-worker selfcheck` inside the worker container. It checks that the worker reaches the API socket, has no network interface besides loopback, cannot resolve names or connect out, runs as UID ≥ 10000 without capabilities, with `no_new_privs` and seccomp, has a read-only root filesystem and socket directory, and cannot execute files in `/tmp`.
