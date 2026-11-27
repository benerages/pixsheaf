# Deployment

Pixsheaf ships as two images started by `compose.yaml`:

| Service | Image | Network | Writable |
|---|---|---|---|
| `api` | `ghcr.io/benerages/pixsheaf-api` (distroless) | bridge, published on `127.0.0.1:8080` | `/tmp` (tmpfs), socket volume |
| `worker` | `ghcr.io/benerages/pixsheaf-worker` | none (`network_mode: none`) | `/tmp` (tmpfs, noexec) |

Both run as UID 10001 with a read-only root filesystem, no Linux capabilities, `no-new-privileges`, and memory, CPU, and process limits. Scale workers with `docker compose up -d --scale worker=3`.

## Publishing behind a reverse proxy

The API binds to the loopback interface of the host. Put a TLS-terminating reverse proxy in front of it and set `PIXSHEAF_TRUSTED_PROXIES` to the proxy's address so client IPs are used for rate limiting. The proxy should send `Strict-Transport-Security`.

Uploads are anonymous by default (ADR 0015). To restrict access, use forward authentication in the proxy. Examples for Traefik, Caddy, and nginx follow with the upload endpoints in Phase 3.

## Optional: gVisor

To run the workers under gVisor, install `runsc`, register it as a Docker runtime, and start with:

```sh
docker compose -f compose.yaml -f deploy/compose.gvisor.yaml up -d
```

## Hosts with `userns-remap`

Pixsheaf works with Docker's `userns-remap`. The socket volume takes its ownership from the images, not from tmpfs mount options, which would refer to host UIDs (ADR 0007, amendment).

## Verifying the isolation

`tests/container/run.sh` builds both images and runs `pixsheaf-worker selfcheck` inside the worker container. It checks that the worker reaches the API socket, has no network interface besides loopback, cannot resolve names or connect out, runs as UID ≥ 10000 without capabilities, with `no_new_privs` and seccomp, has a read-only root filesystem and socket directory, and cannot execute files in `/tmp`.
