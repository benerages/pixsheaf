package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

// Server timeouts. The event stream extends its write deadline per event.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 60 * time.Second
	writeTimeout      = 120 * time.Second
	idleTimeout       = 60 * time.Second
	maxHeaderBytes    = 16 << 10
)

// New returns the public HTTP server serving the embedded UI, the health
// endpoint, and, if opts.Jobs is set, the job API under /api/v1.
func New(addr string, ui fs.FS, logger *slog.Logger, opts Options) *http.Server {
	mux := http.NewServeMux()
	if opts.Jobs != nil {
		(&jobAPI{m: opts.Jobs, opts: opts, logger: logger}).routes(mux)
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("GET /", http.FileServerFS(ui))

	return &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
}
