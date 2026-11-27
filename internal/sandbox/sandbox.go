// Package sandbox restricts the per-file job process (ADR 0006): resource
// limits, a Landlock ruleset, and a seccomp filter.
//
// Apply must run at the very start of the job process, before any C code
// (libvips, glib) starts threads. Landlock and seccomp then cover the Go
// runtime's threads (go-landlock and the seccomp TSYNC flag), and every
// thread created later inherits the restrictions.
package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	seccomp "github.com/elastic/go-seccomp-bpf"
	"github.com/landlock-lsm/go-landlock/landlock"
	"golang.org/x/sys/unix"
)

// Limits configures Apply.
type Limits struct {
	// MemoryBytes caps the data segment (RLIMIT_DATA), which covers heap
	// allocations of Go and C code. Zero leaves the limit unchanged.
	MemoryBytes uint64
	// CPUSeconds caps CPU time (RLIMIT_CPU): SIGXCPU at the limit, SIGKILL
	// one second later. Zero leaves the limit unchanged.
	CPUSeconds uint64
}

// ReadOnlyPaths are the only paths the job process can access, read-only:
// shared libraries, system configuration, its own /proc entry, and CPU
// topology. Nothing is writable; input and output travel over pipes.
var ReadOnlyPaths = []string{
	"/usr",
	"/lib",
	"/lib64",
	"/etc",
	"/proc/self",
	"/sys/devices/system/cpu",
	"/sys/fs/cgroup", // the Go runtime re-reads the CPU limit for GOMAXPROCS
}

// deniedSyscalls fail with EPERM inside the job process. The list targets
// what a compromised decoder would need to escape or persist: networking,
// executing programs, inspecting other processes, kernel attack surface,
// and namespace or mount changes. A full allowlist follows once a corpus
// run can record the syscalls libvips needs (ADR 0006, amendment).
var deniedSyscalls = []string{
	"socket", "socketpair", "connect", "bind", "listen", "accept", "accept4",
	"execve", "execveat",
	"ptrace", "process_vm_readv", "process_vm_writev",
	"mount", "umount2", "pivot_root", "chroot", "unshare", "setns",
	"bpf", "perf_event_open", "userfaultfd",
	"keyctl", "add_key", "request_key",
	"kexec_load", "init_module", "finit_module", "delete_module",
	"io_uring_setup", "io_uring_enter", "io_uring_register",
	"open_by_handle_at", "name_to_handle_at", "fanotify_init",
	"personality", "acct", "swapon", "swapoff", "reboot",
	"settimeofday", "clock_settime", "quotactl",
}

// Apply sets resource limits, then Landlock, then seccomp. It is
// irreversible for the calling process.
func Apply(l Limits) error {
	if err := setRlimits(l); err != nil {
		return err
	}
	// BestEffort degrades on kernels without Landlock or with an older ABI;
	// Probe reports what is enforced. Without network rules, all TCP bind
	// and connect calls are denied; scoping blocks abstract Unix sockets
	// and signals to processes outside the sandbox.
	err := landlock.V11.BestEffort().Restrict(
		landlock.RODirs(ReadOnlyPaths...).IgnoreIfMissing(),
	)
	if err != nil {
		return fmt.Errorf("sandbox: landlock: %w", err)
	}
	filter := seccomp.Filter{
		NoNewPrivs: true,
		Flag:       seccomp.FilterFlagTSync,
		Policy: seccomp.Policy{
			DefaultAction: seccomp.ActionAllow,
			Syscalls: []seccomp.SyscallGroup{
				{Action: seccomp.ActionErrno, Names: deniedSyscalls},
			},
		},
	}
	if err := seccomp.LoadFilter(filter); err != nil {
		return fmt.Errorf("sandbox: seccomp: %w", err)
	}
	return nil
}

func setRlimits(l Limits) error {
	set := func(name string, resource int, soft, hard uint64) error {
		if err := unix.Setrlimit(resource, &unix.Rlimit{Cur: soft, Max: hard}); err != nil {
			return fmt.Errorf("sandbox: setrlimit %s: %w", name, err)
		}
		return nil
	}
	if err := set("RLIMIT_CORE", unix.RLIMIT_CORE, 0, 0); err != nil {
		return err
	}
	// No regular file may be written at all (SIGXFSZ is ignored by the job
	// process, so writes fail with EFBIG instead of killing it).
	if err := set("RLIMIT_FSIZE", unix.RLIMIT_FSIZE, 0, 0); err != nil {
		return err
	}
	if err := set("RLIMIT_NOFILE", unix.RLIMIT_NOFILE, 64, 64); err != nil {
		return err
	}
	if l.MemoryBytes > 0 {
		if err := set("RLIMIT_DATA", unix.RLIMIT_DATA, l.MemoryBytes, l.MemoryBytes); err != nil {
			return err
		}
	}
	if l.CPUSeconds > 0 {
		if err := set("RLIMIT_CPU", unix.RLIMIT_CPU, l.CPUSeconds, l.CPUSeconds+1); err != nil {
			return err
		}
	}
	return nil
}

// Report describes the restrictions observed inside the calling process.
type Report struct {
	// LandlockABI is the Landlock ABI version the kernel supports; 0 means
	// Landlock is unavailable.
	LandlockABI int `json:"landlockAbi"`
	// SocketDenied: creating a socket fails with EPERM (seccomp).
	SocketDenied bool `json:"socketDenied"`
	// ExecDenied: execve fails with EPERM (seccomp).
	ExecDenied bool `json:"execDenied"`
	// FileCreateDenied: creating a file in the temporary directory fails
	// (Landlock).
	FileCreateDenied bool `json:"fileCreateDenied"`
	// OutsideReadDenied: listing "/" fails (Landlock).
	OutsideReadDenied bool `json:"outsideReadDenied"`

	CoreLimit      uint64 `json:"coreLimit"`
	FileSizeLimit  uint64 `json:"fileSizeLimit"`
	OpenFilesLimit uint64 `json:"openFilesLimit"`
	MemoryLimit    uint64 `json:"memoryLimit"`
	CPULimit       uint64 `json:"cpuLimit"`
}

// Probe tests the restrictions in effect for the calling process. It is
// safe to call without a sandbox; nothing it attempts has side effects that
// outlive the call.
func Probe() Report {
	var r Report
	if abi, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION); errno == 0 {
		r.LandlockABI = int(abi) //nolint:gosec // G115: the ABI version is a small integer
	}

	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err == nil {
		_ = unix.Close(fd)
	}
	r.SocketDenied = errors.Is(err, unix.EPERM)

	// execve(NULL, ...) fails with EFAULT when allowed and EPERM when the
	// seccomp filter denies it, so the probe never runs a program.
	_, _, errno := unix.Syscall(unix.SYS_EXECVE, 0, 0, 0)
	r.ExecDenied = errno == unix.EPERM

	name := make([]byte, 8)
	_, _ = rand.Read(name)
	path := filepath.Join(os.TempDir(), ".sandbox-probe-"+hex.EncodeToString(name))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // G304: random name in the temp dir
	if err == nil {
		_ = f.Close()
		_ = os.Remove(path)
	}
	r.FileCreateDenied = err != nil

	_, err = os.ReadDir("/")
	r.OutsideReadDenied = err != nil

	r.CoreLimit = limit(unix.RLIMIT_CORE)
	r.FileSizeLimit = limit(unix.RLIMIT_FSIZE)
	r.OpenFilesLimit = limit(unix.RLIMIT_NOFILE)
	r.MemoryLimit = limit(unix.RLIMIT_DATA)
	r.CPULimit = limit(unix.RLIMIT_CPU)
	return r
}

func limit(resource int) uint64 {
	var rl unix.Rlimit
	if err := unix.Getrlimit(resource, &rl); err != nil {
		return 0
	}
	return rl.Cur
}
