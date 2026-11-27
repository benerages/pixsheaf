// Command pixsheaf-worker processes images in an isolated container.
// It claims files from the API over the Unix socket (ADR 0007) and runs
// each one in its own sandboxed job process (ADR 0006).
//
// Usage:
//
//	pixsheaf-worker [run]     connect to the API and wait for jobs (default)
//	pixsheaf-worker healthcheck
//	pixsheaf-worker selfcheck verify container isolation, codecs, and the job sandbox (container tests)
//	pixsheaf-worker job       run one job in a sandbox (started by the worker itself)
//	pixsheaf-worker version   print the worker and libvips versions
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/benerages/pixsheaf/internal/config"
	"github.com/benerages/pixsheaf/internal/imaging"
	"github.com/benerages/pixsheaf/internal/jobexec"
	"github.com/benerages/pixsheaf/internal/jobproto"
	"github.com/benerages/pixsheaf/internal/selfcheck"
	"github.com/benerages/pixsheaf/internal/transport"
	"github.com/benerages/pixsheaf/internal/version"
)

// retryDelay is the pause after the API could not be reached.
const retryDelay = 2 * time.Second

// reportTimeout bounds sending one result to the API.
const reportTimeout = 2 * time.Minute

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "run":
		os.Exit(run())
	case "healthcheck":
		os.Exit(healthcheck())
	case "selfcheck":
		os.Exit(runSelfcheck())
	case "job":
		os.Exit(jobexec.Child(os.Args[2:], os.Stdin, os.Stdout))
	case "noop":
		// Used by selfcheck to test that /tmp is mounted noexec.
	case "version":
		fmt.Printf("%s (libvips %s)\n", version.Version, imaging.LibraryVersion())
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (want run, healthcheck, selfcheck, version)\n", cmd)
		os.Exit(2)
	}
}

func loadConfig() (config.Config, bool) {
	cfg, err := config.Load(os.Environ())
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return cfg, false
	}
	return cfg, true
}

func run() int {
	cfg, ok := loadConfig()
	if !ok {
		return 2
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	stopCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := imaging.Init(); err != nil {
		// Without libvips every job would fail; let the orchestrator see it.
		logger.Error("libvips", "err", err)
		return 1
	}

	runners := runners{
		images: newRunner(cfg),
		// An archive's output is all of its accepted entries (ADR 0022).
		archives: jobexec.NewRunner(cfg.JobTimeout, uint64(cfg.JobMemoryMB)<<20, int64(cfg.MaxJobMB)<<20), //nolint:gosec // G115: JobMemoryMB is validated to 64-65536
	}
	client := transport.NewClient(cfg.SocketPath, 0)
	maxInput := int64(max(cfg.MaxUploadMB, cfg.MaxArchiveMB)) << 20
	logger.Info("started", "version", version.Version, "socket", cfg.SocketPath, "slots", cfg.MaxConcurrency)

	// Each slot claims and processes one file at a time. On SIGTERM the
	// slots stop claiming, but a running file is finished and reported
	// (graceful shutdown); the job timeout bounds how long that takes.
	var wg sync.WaitGroup
	for slot := range cfg.MaxConcurrency {
		wg.Go(func() { work(stopCtx, logger.With("slot", slot), client, runners, maxInput) })
	}
	wg.Wait()
	logger.Info("stopped")
	return 0
}

func newRunner(cfg config.Config) jobexec.Runner {
	return jobexec.NewRunner(cfg.JobTimeout, uint64(cfg.JobMemoryMB)<<20, int64(cfg.MaxOutputMB)<<20) //nolint:gosec // G115: JobMemoryMB is validated to 64-65536
}

// runners start job processes for images and for archives, which differ
// in their output limit.
type runners struct {
	images, archives jobexec.Runner
}

func work(stopCtx context.Context, logger *slog.Logger, client *http.Client, runners runners, maxInput int64) {
	for stopCtx.Err() == nil {
		task, err := transport.Claim(stopCtx, client, maxInput)
		switch {
		case errors.Is(err, transport.ErrNoTask), stopCtx.Err() != nil:
			continue
		case err != nil:
			logger.Warn("claim failed", "err", err)
			select {
			case <-stopCtx.Done():
			case <-time.After(retryDelay):
			}
			continue
		}

		runCtx := context.WithoutCancel(stopCtx)
		start := time.Now()
		runner := runners.images
		if task.Request.Op == jobproto.OpUnzip {
			runner = runners.archives
		}
		resp, output, err := runner.Run(runCtx, task.Request, task.Input)
		if err != nil {
			code := jobproto.CodeInternal
			if errors.Is(err, jobexec.ErrTimeout) {
				code = jobproto.CodeTimeout
			} else {
				logger.Error("job process failed", "task", task.ID, "err", err)
			}
			resp, output = jobproto.Response{Error: &jobproto.Error{Code: code, Message: "processing failed"}}, nil
		}
		outcome := "done"
		if resp.Error != nil {
			outcome = resp.Error.Code
		}
		// No file names or contents are logged, only sizes and the outcome.
		logger.Info("task finished", "task", task.ID, "outcome", outcome,
			"input_bytes", len(task.Input), "output_bytes", len(output), "ms", time.Since(start).Milliseconds())

		reportCtx, cancel := context.WithTimeout(runCtx, reportTimeout)
		if err := transport.Report(reportCtx, client, task.ID, resp, output); err != nil {
			logger.Warn("report failed", "task", task.ID, "err", err)
		}
		cancel()
	}
}

func healthcheck() int {
	cfg, ok := loadConfig()
	if !ok {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := transport.Ping(ctx, transport.NewClient(cfg.SocketPath, 3*time.Second)); err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	return 0
}

func runSelfcheck() int {
	cfg, ok := loadConfig()
	if !ok {
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	job := newRunner(cfg)
	rs := selfcheck.Run(ctx, selfcheck.Options{SocketPath: cfg.SocketPath, NoopArgs: []string{"noop"}, Job: &job})
	if selfcheck.Print(os.Stdout, rs) > 0 {
		return 1
	}
	return 0
}
