// Command pixsheaf-api is the public API and web UI of Pixsheaf.
//
// Usage:
//
//	pixsheaf-api [serve]   run the server (default)
//	pixsheaf-api healthcheck
//	pixsheaf-api version
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/benerages/pixsheaf/internal/config"
	"github.com/benerages/pixsheaf/internal/httpapi"
	"github.com/benerages/pixsheaf/internal/transport"
	"github.com/benerages/pixsheaf/internal/ui"
	"github.com/benerages/pixsheaf/internal/version"
)

const shutdownTimeout = 30 * time.Second

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "serve":
		os.Exit(serve())
	case "healthcheck":
		os.Exit(healthcheck())
	case "version":
		fmt.Println(version.Version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (want serve, healthcheck, version)\n", cmd)
		os.Exit(2)
	}
}

func serve() int {
	cfg, err := config.Load(os.Environ())
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 2
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ln, err := transport.Listen(cfg.SocketPath)
	if err != nil {
		logger.Error("internal socket", "path", cfg.SocketPath, "err", err)
		return 1
	}
	internal := transport.NewServer()
	public := httpapi.New(cfg.ListenAddr, ui.FS(), logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 2)
	go func() { errc <- internal.Serve(ln) }()
	go func() { errc <- public.ListenAndServe() }()
	logger.Info("started", "version", version.Version, "listen", cfg.ListenAddr, "socket", cfg.SocketPath)

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "err", err)
		}
	}

	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = public.Shutdown(sctx)
	_ = internal.Shutdown(sctx)
	_ = os.Remove(cfg.SocketPath)
	return 0
}

// healthcheck is used as the container health check; the image has no curl.
func healthcheck() int {
	cfg, err := config.Load(os.Environ())
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+cfg.LocalAddr()+"/healthz", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "unhealthy: status", resp.StatusCode)
		return 1
	}
	return 0
}
