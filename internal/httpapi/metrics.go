package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/benerages/pixsheaf/internal/jobs"
	"github.com/benerages/pixsheaf/internal/version"
)

// NewMetrics returns the server for Prometheus metrics on its own address
// (PIXSHEAF_METRICS_ADDR, off by default). It serves only GET /metrics in
// the Prometheus text format. The values are counts; they contain no IDs,
// names, or client keys. Keep the address unreachable from the internet.
func NewMetrics(addr string, m *jobs.Manager, reqs *RequestStats) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, renderMetrics(m.Stats(), reqs))
	})
	return &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    8 << 10,
	}
}

func renderMetrics(s jobs.Stats, reqs *RequestStats) string {
	var b strings.Builder
	metric := func(name, kind, help string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
	}
	metric("pixsheaf_build_info", "gauge", "Version of the running API.")
	fmt.Fprintf(&b, "pixsheaf_build_info{version=%q} 1\n", version.Version)
	metric("pixsheaf_jobs", "gauge", "Jobs kept in memory.")
	fmt.Fprintf(&b, "pixsheaf_jobs %d\n", s.Jobs)
	metric("pixsheaf_files_queued", "gauge", "Files waiting for a worker.")
	fmt.Fprintf(&b, "pixsheaf_files_queued %d\n", s.QueuedFiles)
	metric("pixsheaf_files_in_flight", "gauge", "Files being processed by workers.")
	fmt.Fprintf(&b, "pixsheaf_files_in_flight %d\n", s.InFlight)
	metric("pixsheaf_stored_bytes", "gauge", "Bytes of uploads and results on the tmpfs.")
	fmt.Fprintf(&b, "pixsheaf_stored_bytes %d\n", s.StoredBytes)
	metric("pixsheaf_files_done_total", "counter", "Images processed successfully.")
	fmt.Fprintf(&b, "pixsheaf_files_done_total %d\n", s.FilesDone)
	metric("pixsheaf_files_failed_total", "counter", "Files that failed, by error code.")
	codes := make([]string, 0, len(s.FilesFailed))
	for code := range s.FilesFailed {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	for _, code := range codes {
		fmt.Fprintf(&b, "pixsheaf_files_failed_total{code=%q} %d\n", code, s.FilesFailed[code])
	}
	if reqs != nil {
		metric("pixsheaf_http_responses_total", "counter", "API responses by status class.")
		for c := 1; c <= 5; c++ {
			fmt.Fprintf(&b, "pixsheaf_http_responses_total{class=\"%dxx\"} %d\n", c, reqs.byClass[c].Load())
		}
		metric("pixsheaf_security_events_total", "counter", "Rejected requests by reason.")
		fmt.Fprintf(&b, "pixsheaf_security_events_total{event=\"auth_failed\"} %d\n", reqs.authFailed.Load())
		fmt.Fprintf(&b, "pixsheaf_security_events_total{event=\"cross_origin_rejected\"} %d\n", reqs.crossOrigin.Load())
		fmt.Fprintf(&b, "pixsheaf_security_events_total{event=\"rate_limited\"} %d\n", reqs.rateLimited.Load())
	}
	return b.String()
}
