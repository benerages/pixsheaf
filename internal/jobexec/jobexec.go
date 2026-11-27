// Package jobexec runs one file in its own sandboxed process (ADR 0006).
//
// The supervisor (Runner) starts "pixsheaf-worker job" in a new process
// group, sends a jobproto.Request plus the input over stdin, and reads a
// jobproto.Response plus the output from stdout. On timeout the whole
// process group is killed. The job process (Child) applies the sandbox
// before it reads any input or starts libvips.
package jobexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/benerages/pixsheaf/internal/jobproto"
)

// ErrTimeout is returned when a job exceeds Runner.Timeout.
var ErrTimeout = errors.New("jobexec: job timed out")

// stderrLimit bounds the job process's stderr kept for error messages.
const stderrLimit = 8 << 10

// Runner starts job processes.
type Runner struct {
	// Path is the executable; typically "/proc/self/exe".
	Path string
	// Args come before the limit flags; typically ["job"].
	Args []string
	// Env is the complete environment of the job process.
	Env []string
	// Timeout bounds the wall-clock time of one job. It also sets the
	// CPU-time limit inside the job process.
	Timeout time.Duration
	// MemoryBytes is the job process's data-segment limit; zero for none.
	MemoryBytes uint64
	// MaxOutputBytes bounds the output a job may return.
	MaxOutputBytes int64
}

// childEnv lists the variables passed from the worker to job processes.
var childEnv = []string{"LD_LIBRARY_PATH", "VIPS_BLOCK_UNTRUSTED", "SVT_LOG", "TMPDIR"}

// NewRunner returns a Runner that re-executes the current binary with a
// minimal environment.
func NewRunner(timeout time.Duration, memoryBytes uint64, maxOutputBytes int64) Runner {
	var env []string
	for _, name := range childEnv {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return Runner{
		Path:           "/proc/self/exe",
		Args:           []string{"job"},
		Env:            env,
		Timeout:        timeout,
		MemoryBytes:    memoryBytes,
		MaxOutputBytes: maxOutputBytes,
	}
}

// Run processes one input in a new job process and returns its response
// and output. A job-level failure (for example an undecodable image) is
// reported in Response.Error with a nil error; the error return is for
// failures of the process itself.
func (r Runner) Run(ctx context.Context, req jobproto.Request, input []byte) (jobproto.Response, []byte, error) {
	if r.Timeout <= 0 {
		return jobproto.Response{}, nil, errors.New("jobexec: timeout must be positive")
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	cpuSeconds := uint64(r.Timeout/time.Second) + 1 //nolint:gosec // G115: Timeout is positive
	args := append(append([]string{}, r.Args...),
		"--memory="+strconv.FormatUint(r.MemoryBytes, 10),
		"--cpu="+strconv.FormatUint(cpuSeconds, 10),
	)
	cmd := exec.CommandContext(ctx, r.Path, args...) //nolint:gosec // G204: runs our own binary with fixed arguments
	cmd.Env = r.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,            // own process group, killed as a whole
		Pdeathsig: syscall.SIGKILL, // dies with the supervisor
	}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
	stderr := &limitedBuffer{max: stderrLimit}
	cmd.Stderr = stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return jobproto.Response{}, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return jobproto.Response{}, nil, err
	}
	if err := cmd.Start(); err != nil {
		return jobproto.Response{}, nil, fmt.Errorf("jobexec: start: %w", err)
	}

	req.InputBytes = int64(len(input))
	writeDone := make(chan error, 1)
	go func() {
		err := jobproto.WriteHeader(stdin, req)
		if err == nil {
			_, err = stdin.Write(input)
		}
		if cerr := stdin.Close(); err == nil {
			err = cerr
		}
		writeDone <- err
	}()

	var resp jobproto.Response
	var output []byte
	readErr := jobproto.ReadHeader(stdout, &resp)
	if readErr == nil {
		switch {
		case resp.OutputBytes < 0 || resp.OutputBytes > r.MaxOutputBytes:
			readErr = fmt.Errorf("jobexec: output of %d bytes exceeds the limit", resp.OutputBytes)
		case resp.OutputBytes > 0:
			output = make([]byte, resp.OutputBytes)
			_, readErr = io.ReadFull(stdout, output)
		}
	}
	waitErr := cmd.Wait()
	writeErr := <-writeDone

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return jobproto.Response{}, nil, ErrTimeout
	}
	if readErr != nil {
		return jobproto.Response{}, nil, fmt.Errorf("jobexec: reading response: %w%s", readErr, stderr.suffix())
	}
	if waitErr != nil {
		return jobproto.Response{}, nil, fmt.Errorf("jobexec: job process: %w%s", waitErr, stderr.suffix())
	}
	if writeErr != nil {
		return jobproto.Response{}, nil, fmt.Errorf("jobexec: sending input: %w", writeErr)
	}
	return resp, output, nil
}

// limitedBuffer keeps the first max bytes written to it.
type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.max - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *limitedBuffer) suffix() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := strings.TrimSpace(b.buf.String())
	if s == "" {
		return ""
	}
	return " (stderr: " + s + ")"
}
