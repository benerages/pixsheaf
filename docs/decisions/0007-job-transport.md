# ADR 0007: Job transport

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Options

(a) A Unix socket on a shared tmpfs volume, with workers using `network_mode: none`. (b) An `internal: true` bridge network using HTTP or gRPC.

## Decision

Option (a), in a pull model. The API owns one socket (for example `/run/pixsheaf/api.sock`) on a named volume with `driver_opts: type=tmpfs` mounted `noexec,nosuid,nodev`. Workers connect to it and fetch jobs. Payloads are streamed over the socket using HTTP/1.1. Both images run as the same UID (≥ 10000) and the socket has mode 0660 ([UDS via volume, secondary source](https://oneuptime.com/blog/post/2026-02-08-how-to-set-up-docker-containers-with-unix-socket-communication/markdown)).

## Consequences

Workers have no IP address on any network, which is a stronger guarantee than option (b). Workers expose no listener, so `--scale worker=n` needs no service discovery. Pathname sockets cross network namespaces by design. The per-job child must be blocked from the socket through Landlock ABI 9 `FS_RESOLVE_UNIX` or by closing the descriptor before it decodes anything. The behavior under gVisor and rootless Docker is still untested (see Validation).

## Amendment 2026-11-27: volume type after validation

The spike in `spikes/uds-netnone/` tested two variants of the shared socket volume.

| Variant | Result | Observation |
|---|---|---|
| tmpfs-backed named volume (`driver_opts: type=tmpfs, o=uid=10001,gid=10001,mode=0770`) | **FAIL** | `bind: permission denied`. Inside the container the directory belonged to `65534:65534` (unmapped), although the mount options showed `uid=10001,gid=10001`. The host runs Docker with `userns-remap` (`/var/lib/docker/100000.100000/`); the daemon mounts the tmpfs with *host* UIDs, which do not map to UID 10001 in the container. |
| Plain named volume; the image ships `/run/pixsheaf` owned by `10001:10001`, mode `0770`, and Docker copies ownership into the empty volume on first mount | **PASS**, 13/13 checks | Socket call works; worker has only `lo`; DNS, IPv4, and IPv6 egress blocked; UID 10001; no capabilities; `no_new_privs`; seccomp mode 2; read-only root; `/tmp` noexec; socket directory read-only for the worker. |

**Decision (amended):** Use a plain named volume whose ownership comes from the image (second variant). The tmpfs-volume variant is rejected because its `uid`/`gid` options refer to host UIDs and break under `userns-remap`, which security-conscious hosts use. The API mounts the volume read-write; every worker mounts it **read-only** (connecting to an existing socket needs no write access to the directory).

**Consequences of the amendment:**

- The socket directory lives on the Docker volume storage (disk), not in RAM. It contains only the socket inode; job payloads are streamed over the socket and never written there. The worker cannot write to it at all, so it cannot be used to persist data.
- The image must create `/run/pixsheaf` with the service UID and mode `0770` (tightened to `0700`, see the second amendment); a container test checks ownership and mode.
- The same UID (10001) must be used in the API and worker images.

## Amendment 2026-11-27: owner-only permissions

API and worker images run as the same UID (10001), so the group bits on the socket and its directory grant nothing that is needed. Following the project rule that the more secure option wins, the project owner decided to drop them.

**Decision (amended):** The socket has mode `0600` and `/run/pixsheaf` has mode `0700`, both owned by `10001:10001`. Running workers under a different UID than the API is not supported; doing so would require a new ADR.

**Consequences:** One fewer `nolint` exception (gosec G302). A deployment that overrides the `user:` of only one service breaks the socket connection, which the container self-check reports as "api socket reachable: FAIL".

## Validation

| Setup | Result | Date |
|---|---|---|
| Docker 29.10.0, runc, `userns-remap` enabled, kernel 6.12.118 (Debian 13), amd64 | PASS (13/13) | 2026-11-27 |
| gVisor (`runsc`) | Not tested yet: runtime not installed on the test host | — |
| Rootless Docker | Not tested yet | — |
| Docker without `userns-remap` (GitHub Actions runner, Ubuntu 24.04, runc) | PASS (container workflow, self-check) | 2026-11-27 |
