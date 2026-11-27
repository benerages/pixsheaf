# ADR 0006: Worker isolation

- Status: Accepted
- Date: 2026-11-27
- Approved by: project owner, 2026-11-27


## Context

The requirement is that a plain `docker compose up` works without privileged containers. nsjail's documented Docker usage needs `--privileged` or unprivileged user namespaces ([nsjail](https://raw.githubusercontent.com/google/nsjail/master/README.md)). Landlock works without privileges once no_new_privs is set, starting with Linux 5.13 ([Landlock](https://docs.kernel.org/userspace-api/landlock.html)).

## Options

(a) A per-job subprocess with rlimits, no_new_privs, Landlock, and seccomp. (b) nsjail or bubblewrap. (c) Per-call `vips` CLI processes. (d) gVisor by default. (e) Kata, Firecracker, or WASM.

## Decision

Option (a). The worker supervisor fork/execs `pixsheaf-worker job` once per file. The child sets RLIMIT_AS (or RLIMIT_DATA, because Go's address-space reservations make RLIMIT_AS awkward), RLIMIT_CPU, RLIMIT_FSIZE, RLIMIT_NOFILE, and RLIMIT_CORE=0, then sets no_new_privs. It then applies a best-effort Landlock ruleset: libraries read-only, its own job directory read-write, TCP denied, and abstract Unix sockets scoped (using go-landlock, with ABI 8 TSYNC or apply-before-threads handling for Go). Finally it installs a seccomp filter: no `socket`, `connect`, or `execve` after setup, and `ERRNO` with logging as the default action rather than kill. The supervisor kills the process group on timeout. Each container runs N = 1–2 jobs, and container-level `mem_limit`, `cpus`, and `pids_limit` act as the hard backstop. Ship gVisor as an opt-in `compose.gvisor.yaml` that sets `runtime: runsc`. gVisor supports amd64 and arm64, and CPU-bound work runs at near-native speed ([gVisor perf](https://gvisor.dev/docs/architecture_guide/performance/)).

## Consequences

Crashes and leaks die with the child process, which also contains GLib fragmentation ([govips README](https://github.com/davidbyttow/govips)). On older kernels Landlock degrades gracefully. RLIMIT_NPROC applies per UID across the container, not per job. Per-job cgroups would need cgroup delegation, which was not verified, so they are deferred. Kata and Firecracker require KVM and are out of scope. WASM (wasm-vips with wazero) is a future research option. The custom seccomp profile needs a CI corpus run on every libvips, glibc, or Go upgrade.
