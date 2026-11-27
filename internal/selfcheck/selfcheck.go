// Package selfcheck verifies, from inside a running worker container, that
// the isolation required by ADRs 0006 and 0007 is in effect. It is used by
// `pixsheaf-worker selfcheck` in the container tests.
package selfcheck

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/benerages/pixsheaf/internal/transport"
)

// Result is the outcome of one check.
type Result struct {
	Name string
	OK   bool
	Info string
}

// Options configures the checks.
type Options struct {
	// SocketPath is the API socket the worker must reach.
	SocketPath string
	// NoopArgs runs the current binary as a no-op; used to test that
	// /tmp is mounted noexec.
	NoopArgs []string
}

// Run executes all checks and returns their results in a stable order.
func Run(ctx context.Context, o Options) []Result {
	var rs []Result
	add := func(name string, ok bool, info string) { rs = append(rs, Result{name, ok, info}) }

	// Socket reachable (retry while the API starts).
	client := transport.NewClient(o.SocketPath, 2*time.Second)
	var pingErr error
	for i := 0; i < 30; i++ {
		if pingErr = transport.Ping(ctx, client); pingErr == nil {
			break
		}
		select {
		case <-ctx.Done():
			i = 30
		case <-time.After(500 * time.Millisecond):
		}
	}
	add("api socket reachable", pingErr == nil, fmt.Sprint(pingErr))

	probe := filepath.Join(filepath.Dir(o.SocketPath), ".selfcheck-write")
	f, err := os.Create(probe)
	if f != nil {
		_ = f.Close()
		_ = os.Remove(probe)
	}
	add("socket dir read-only", err != nil, fmt.Sprint(err))

	ifaces, err := net.Interfaces()
	onlyLo := err == nil
	var names []string
	for _, ifc := range ifaces {
		names = append(names, ifc.Name)
		if ifc.Flags&net.FlagLoopback == 0 {
			onlyLo = false
		}
	}
	add("only loopback interface", onlyLo, strings.Join(names, ","))

	dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	_, dnsErr := net.DefaultResolver.LookupHost(dctx, "example.com")
	cancel()
	add("dns blocked", dnsErr != nil, fmt.Sprint(dnsErr))
	for _, target := range []string{"1.1.1.1:443", "[2606:4700:4700::1111]:443"} {
		conn, err := net.DialTimeout("tcp", target, 3*time.Second)
		if conn != nil {
			_ = conn.Close()
		}
		add("egress blocked "+target, err != nil, fmt.Sprint(err))
	}

	st := procStatus()
	add("uid >= 10000", os.Getuid() >= 10000, fmt.Sprint("uid=", os.Getuid()))
	add("no capabilities", st["CapEff"] == "0000000000000000", "CapEff="+st["CapEff"])
	add("no_new_privs", st["NoNewPrivs"] == "1", "NoNewPrivs="+st["NoNewPrivs"])
	add("seccomp active", st["Seccomp"] == "2", "Seccomp="+st["Seccomp"])

	f, err = os.Create("/.selfcheck-write")
	if f != nil {
		_ = f.Close()
		_ = os.Remove("/.selfcheck-write")
	}
	add("root filesystem read-only", err != nil, fmt.Sprint(err))

	ok, info := tmpNoexec(o.NoopArgs)
	add("tmp writable and noexec", ok, info)
	return rs
}

// Print writes results as PASS/FAIL lines and returns the number of failures.
func Print(w io.Writer, rs []Result) int {
	failed := 0
	for _, r := range rs {
		mark := "PASS"
		if !r.OK {
			mark = "FAIL"
			failed++
		}
		fmt.Fprintf(w, "%s  %-34s %s\n", mark, r.Name, r.Info)
	}
	fmt.Fprintf(w, "selfcheck: %d of %d checks passed\n", len(rs)-failed, len(rs))
	return failed
}

func procStatus() map[string]string {
	m := map[string]string{}
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return m
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if k, v, ok := strings.Cut(s.Text(), ":"); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

// tmpNoexec copies the running binary to /tmp and tries to execute it. The
// images contain no shell, so a script would fail for the wrong reason.
func tmpNoexec(noopArgs []string) (bool, string) {
	self, err := os.ReadFile("/proc/self/exe")
	if err != nil {
		return false, "read self: " + err.Error()
	}
	path := filepath.Join(os.TempDir(), ".selfcheck-exec")
	// The copy must be executable for the probe to be meaningful.
	if err := os.WriteFile(path, self, 0o700); err != nil { //nolint:gosec // G306: executable on purpose

		return false, "write: " + err.Error()
	}
	defer os.Remove(path)
	err = exec.Command(path, noopArgs...).Run() //nolint:gosec // G204: runs our own binary copy
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return true, "exec denied"
	}
	return false, fmt.Sprint("exec not denied: err=", err)
}
