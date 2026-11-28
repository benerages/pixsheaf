package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
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
	var handler http.Handler = mux
	if opts.Jobs != nil {
		presets, err := loadPresets()
		if err != nil {
			// The presets are compiled in; a broken one is a build error.
			panic(err)
		}
		(&jobAPI{m: opts.Jobs, opts: opts, logger: logger, presets: presets}).routes(mux)
		var limiter *rateLimiter
		if opts.RequestsPerMinute > 0 {
			limiter = newRateLimiter(opts.RequestsPerMinute)
		}
		handler = limitClients(newClientIdentifier(opts.TrustedProxies), limiter, logger, opts.Requests,
			observe(logger, opts.Requests, mux))
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("GET /", noDirectoryListing(http.FileServerFS(ui)))

	return &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(handler),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
}

// noDirectoryListing answers 404 for folder paths other than the root, so
// that the file server never lists a folder of the UI (ASVS 5.0 13.4.3).
func noDirectoryListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; p != "/" && strings.HasSuffix(p, "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
