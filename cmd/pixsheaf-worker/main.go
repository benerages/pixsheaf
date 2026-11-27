// Command pixsheaf-worker processes images in an isolated container.
// In Phase 2 it only connects to the API socket; image processing is
// added in Phase 3.
//
// Usage:
//
//	pixsheaf-worker [run]     connect to the API and wait for jobs (default)
//	pixsheaf-worker healthcheck
//	pixsheaf-worker selfcheck verify container isolation (container tests)
//	pixsheaf-worker version
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/benerages/pixsheaf/internal/config"
	"github.com/benerages/pixsheaf/internal/selfcheck"
	"github.com/benerages/pixsheaf/internal/transport"
	"github.com/benerages/pixsheaf/internal/version"
)

const pingInterval = 10 * time.Second

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
	case "noop":
		// Used by selfcheck to test that /tmp is mounted noexec.
	case "version":
		fmt.Println(version.Version)
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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := transport.NewClient(cfg.SocketPath, 3*time.Second)
	logger.Info("started", "version", version.Version, "socket", cfg.SocketPath)
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		if err := transport.Ping(ctx, client); err != nil {
			logger.Warn("api not reachable", "err", err)
		} else {
			logger.Debug("api reachable")
		}
		select {
		case <-ctx.Done():
			logger.Info("shutting down")
			return 0
		case <-ticker.C:
		}
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
	rs := selfcheck.Run(ctx, selfcheck.Options{SocketPath: cfg.SocketPath, NoopArgs: []string{"noop"}})
	if selfcheck.Print(os.Stdout, rs) > 0 {
		return 1
	}
	return 0
}
