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

## Amendment 2026-11-27: implementation (Phase 3)

Implemented in `internal/sandbox` (restrictions), `internal/jobexec` (supervisor and job process), and `internal/jobproto` (messages).

- **Process model:** the worker starts `/proc/self/exe job` per file in its own process group with `Pdeathsig=SIGKILL`; on timeout the group is killed with SIGKILL. The job process gets a minimal environment.
- **Order inside the job process:** resource limits, then Landlock, then seccomp, all before reading input and before libvips starts. Landlock (go-landlock, which uses libpsx for cgo programs) and seccomp (`SECCOMP_FILTER_FLAG_TSYNC`) cover the Go runtime's threads; libvips and glib threads are created later and inherit the restrictions.
- **No files:** request, input, response, and output travel over stdin and stdout (length-prefixed JSON header plus raw bytes), so the job process needs no writable path at all. `RLIMIT_FSIZE` is 0 and SIGXFSZ is ignored, so file writes fail with EFBIG.
- **Resource limits:** `RLIMIT_CORE=0`, `RLIMIT_FSIZE=0`, `RLIMIT_NOFILE=64`, `RLIMIT_DATA` from `PIXSHEAF_JOB_MEMORY_MB` (default 384), `RLIMIT_CPU` = timeout + 1 s (hard limit one second higher).
- **Landlock:** `landlock.V11.BestEffort()` with read-only access to `/usr`, `/lib`, `/lib64`, `/etc`, `/proc/self`, `/sys/devices/system/cpu`, and `/sys/fs/cgroup`; nothing writable; no TCP bind or connect; abstract Unix sockets and signals scoped to the sandbox.
- **seccomp, deviation:** the filter is a **denylist** for now (default allow, EPERM for networking, `execve`/`execveat`, `ptrace` and `process_vm_*`, mount and namespace calls, `bpf`, `perf_event_open`, `userfaultfd`, keyring, module, `io_uring`, and handle-based open calls). The allowlist with ERRNO as the default action (decided above) needs a recorded syscall set from a corpus run of every format and operation; it follows in Phase 6 together with the corpus tests. Docker's default seccomp profile remains the outer layer.
- **Verification:** the container self-check starts real job processes and requires that sockets and `exec` are denied, Landlock is available and denies file creation and reads outside the allowlist, limits are set, and an image header can be read inside the sandbox.
