package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Request logging and counters (ASVS 5.0 V16, docs/security.md "Logging").
//
// Every API response with status 400 or higher is logged; successful
// requests only at debug level, so that polling does not flood the log.
// A log entry carries the route pattern (never the path with its IDs
// beyond the job ID), the status, the problem code, the duration, and a
// short pseudonymous client key (an HMAC of the address with a key that
// exists only in memory). File names, contents, tokens, and IP addresses
// are never logged.

// RequestStats counts API responses for the metrics endpoint.
type RequestStats struct {
	byClass     [6]atomic.Uint64 // index status/100
	authFailed  atomic.Uint64
	crossOrigin atomic.Uint64
	rateLimited atomic.Uint64
}

func (s *RequestStats) count(status int, code string) {
	if s == nil {
		return
	}
	if c := status / 100; c >= 1 && c <= 5 {
		s.byClass[c].Add(1)
	}
	switch {
	case status == http.StatusUnauthorized:
		s.authFailed.Add(1)
	case code == codeCrossOrigin:
		s.crossOrigin.Add(1)
	case status == http.StatusTooManyRequests:
		s.rateLimited.Add(1)
	}
}

// statusWriter records the status and the problem code of a response.
type statusWriter struct {
	http.ResponseWriter
	status int
	code   string
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach Flush and deadlines.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// setProblemCode records the code of a problem response for the log.
func setProblemCode(w http.ResponseWriter, code string) {
	for {
		switch v := w.(type) {
		case *statusWriter:
			v.code = code
			return
		case interface{ Unwrap() http.ResponseWriter }:
			w = v.Unwrap()
		default:
			return
		}
	}
}

// observe logs and counts API requests. It runs inside limitClients, so
// the client key is in the request context, and directly around the mux,
// which sets the route pattern on the same request.
func observe(logger *slog.Logger, stats *RequestStats, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		stats.count(status, sw.code)
		logRequest(logger, r, status, sw.code, time.Since(start))
	})
}

// logRequest writes one request log entry; see the package comment above.
func logRequest(logger *slog.Logger, r *http.Request, status int, code string, d time.Duration) {
	level, event := slog.LevelDebug, "request"
	switch {
	case status >= 500:
		level, event = slog.LevelError, "server_error"
	case status == http.StatusUnauthorized:
		level, event = slog.LevelWarn, "auth_failed"
	case code == codeCrossOrigin:
		level, event = slog.LevelWarn, "cross_origin_rejected"
	case status == http.StatusTooManyRequests:
		level, event = slog.LevelWarn, "rate_limited"
	case status >= 400:
		level, event = slog.LevelInfo, "request_rejected"
	}
	if !logger.Enabled(r.Context(), level) {
		return
	}
	attrs := []slog.Attr{
		slog.String("event", event),
		slog.String("method", r.Method),
		slog.String("route", r.Pattern),
		slog.Int("status", status),
		slog.Int64("duration_ms", d.Milliseconds()),
	}
	if code != "" {
		attrs = append(attrs, slog.String("code", code))
	}
	if k := clientKeyFrom(r.Context()); k != "" {
		attrs = append(attrs, slog.String("client", k[:min(len(k), 12)]))
	}
	if id := r.PathValue("jobId"); validID(id) {
		attrs = append(attrs, slog.String("job", id))
	}
	logger.LogAttrs(r.Context(), level, "http", attrs...)
}
