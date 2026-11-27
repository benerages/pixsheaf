# Spike: Unix socket transport with `network_mode: none` (ADR 0007)

Throwaway test that verifies the riskiest assumption from Phase 1 before any production code depends on it:

> A worker container with `network_mode: none` can reach the API over a Unix socket on a shared tmpfs volume, and has no other network access.

The worker also checks the container hardening required by the project (UID ≥ 10000, no capabilities, `no_new_privs`, seccomp, read-only root filesystem, `noexec` tmpfs).

## Requirements

- Linux host with Docker Engine and the Compose plugin (Docker Desktop on Windows/macOS works too, but the target is Linux).
- Internet access for pulling `golang` and `distroless` images.
- Optional: gVisor (`runsc`) registered as a Docker runtime.

## Run

```sh
./run.sh            # default runtime (runc)
./run.sh gvisor     # with gVisor; skipped if runsc is not registered
```

Run it under each setup you intend to support: rootful Docker, rootless Docker, and gVisor.

## Expected output

For each volume variant (`sock_tmpfs`, `sock_image`): thirteen `PASS` lines from the worker and `RESULT (...): PASS`. The server and worker also print their UID/GID, the socket directory's owner and mode, and its mount options, so a permission failure can be explained from the output. Under gVisor, the `Seccomp` line may differ because gVisor implements system calls itself; record the actual result.

| Check | Meaning |
|---|---|
| unix socket call | Worker reached the server through the shared volume |
| socket dir not writable | Worker mounts the socket volume read-only and cannot create files there |
| only loopback interface | Worker has no network interface besides `lo` |
| DNS lookup blocked, TCP … blocked | No name resolution, no outbound IPv4/IPv6 |
| UID ≥ 10000, no effective capabilities, no_new_privs, seccomp | Process hardening |
| root filesystem read-only | `read_only: true` is effective |
| tmpfs writable and noexec | `/tmp` accepts files but refuses to execute them |

## Negative control

Running the same binary unhardened on a normal host reported 8 of 12 checks (before the read-only socket-dir check was added) as failed (network reachable, root user, capabilities, writable root, exec allowed in `/tmp`). This shows the checks can actually fail.

## Report back

Copy the line starting with `== Docker:` and the worker output for each runtime into the Phase 2 status report.
