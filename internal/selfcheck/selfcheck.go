// Package selfcheck verifies, from inside a running worker container, that
// the isolation required by ADRs 0006 and 0007 is in effect and that every
// allowed codec works (ADRs 0008, 0018). It is used by
// `pixsheaf-worker selfcheck` in the container tests.
package selfcheck

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/benerages/pixsheaf/internal/imaging"
	"github.com/benerages/pixsheaf/internal/jobexec"
	"github.com/benerages/pixsheaf/internal/jobproto"
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
	// Job, if set, runs job processes to check the per-job sandbox
	// (ADR 0006) from the inside.
	Job *jobexec.Runner
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
	f, err := os.Create(probe) //nolint:gosec // G304: path derives from the configured socket dir, not from user input
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
		d := net.Dialer{Timeout: 3 * time.Second}
		conn, err := d.DialContext(ctx, "tcp", target)
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

	ok, info := tmpNoexec(ctx, o.NoopArgs)
	add("tmp writable and noexec", ok, info)

	// Every allowed output format round-trips through its loader.
	for _, p := range imaging.Probe() {
		pinfo := p.Loader
		if p.Err != nil {
			pinfo = p.Err.Error()
		}
		add("codec "+p.Suffix, p.Err == nil, pinfo)
	}

	if o.Job != nil {
		jobChecks(ctx, *o.Job, add)
	}
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
		_, _ = fmt.Fprintf(w, "%s  %-34s %s\n", mark, r.Name, r.Info)
	}
	_, _ = fmt.Fprintf(w, "selfcheck: %d of %d checks passed\n", len(rs)-failed, len(rs))
	return failed
}

func procStatus() map[string]string {
	m := map[string]string{}
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return m
	}
	defer func() { _ = f.Close() }()
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
func tmpNoexec(ctx context.Context, noopArgs []string) (bool, string) {
	self, err := os.ReadFile("/proc/self/exe")
	if err != nil {
		return false, "read self: " + err.Error()
	}
	path := filepath.Join(os.TempDir(), ".selfcheck-exec")
	// The copy must be executable for the probe to be meaningful.
	if err := os.WriteFile(path, self, 0o700); err != nil { //nolint:gosec // G306: executable on purpose
		return false, "write: " + err.Error()
	}
	defer func() { _ = os.Remove(path) }()
	err = exec.CommandContext(ctx, path, noopArgs...).Run() //nolint:gosec // G204: runs our own binary copy
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return true, "exec denied"
	}
	return false, fmt.Sprint("exec not denied: err=", err)
}

// jobChecks runs job processes and checks the sandbox from the inside:
// seccomp, Landlock, resource limits, and a real image header read.
func jobChecks(ctx context.Context, r jobexec.Runner, add func(string, bool, string)) {
	resp, _, err := r.Run(ctx, jobproto.Request{Op: jobproto.OpSandboxCheck}, nil)
	if err == nil && resp.Error != nil {
		err = fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}
	if err != nil || resp.Sandbox == nil {
		add("job sandbox report", false, fmt.Sprint(err))
		return
	}
	s := resp.Sandbox
	add("job: socket denied (seccomp)", s.SocketDenied, "")
	add("job: exec denied (seccomp)", s.ExecDenied, "")
	add("job: landlock available", s.LandlockABI > 0, fmt.Sprint("abi=", s.LandlockABI))
	add("job: file creation denied", s.FileCreateDenied, "")
	add("job: reads outside allowlist denied", s.OutsideReadDenied, "")
	add("job: no core dumps, no file writes", s.CoreLimit == 0 && s.FileSizeLimit == 0,
		fmt.Sprintf("core=%d fsize=%d", s.CoreLimit, s.FileSizeLimit))
	add("job: memory and CPU limits", s.MemoryLimit != math.MaxUint64 && s.CPULimit != math.MaxUint64,
		fmt.Sprintf("data=%d cpu=%d", s.MemoryLimit, s.CPULimit))

	resp, _, err = r.Run(ctx, jobproto.Request{Op: jobproto.OpInspect}, samplePNG())
	ok := err == nil && resp.Error == nil && resp.Image != nil &&
		resp.Image.Format == "png" && resp.Image.Width == 3 && resp.Image.Height == 2
	info := fmt.Sprint(err)
	if resp.Error != nil {
		info = resp.Error.Code + ": " + resp.Error.Message
	} else if resp.Image != nil {
		info = fmt.Sprintf("%s %dx%d", resp.Image.Format, resp.Image.Width, resp.Image.Height)
	}
	add("job: inspect image in sandbox", ok, info)
}

// samplePNG returns a 3x2 PNG encoded by the Go standard library.
func samplePNG() []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewGray(image.Rect(0, 0, 3, 2)))
	return buf.Bytes()
}
