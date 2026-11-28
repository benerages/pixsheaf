package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

func TestSecurityEventsLoggedAndCounted(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	stats := &RequestStats{}
	e := newAPI(t, Options{Requests: stats}, logger)
	jc, _ := e.create(t, emptyPipeline, 1)
	path := "/api/v1/jobs/" + jc.Job.Id

	expect(t, e.do(t, "GET", path, "wrong", "", nil), http.StatusUnauthorized, codeUnauthorized)
	expect(t, e.upload(t, jc.Job.Id, jc.Token, "a.png", pngHeader, "Sec-Fetch-Site", "cross-site"), http.StatusForbidden, codeCrossOrigin)
	expect(t, e.do(t, "GET", path, jc.Token, "", nil), http.StatusOK, "")

	var events []map[string]any
	for line := range strings.Lines(buf.String()) {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil && m["msg"] == "http" {
			events = append(events, m)
		}
	}
	if len(events) != 2 {
		t.Fatalf("want 2 log entries (successful requests only at debug), got %d: %s", len(events), buf.String())
	}
	auth, cross := events[0], events[1]
	if auth["event"] != "auth_failed" || auth["status"] != float64(401) || auth["job"] != jc.Job.Id ||
		auth["route"] != "GET /api/v1/jobs/{jobId}" || auth["level"] != "WARN" {
		t.Errorf("auth entry %v", auth)
	}
	if cross["event"] != "cross_origin_rejected" || cross["code"] != codeCrossOrigin {
		t.Errorf("cross-origin entry %v", cross)
	}
	if c, _ := auth["client"].(string); len(c) != 12 {
		t.Errorf("client key %q", c)
	}
	for _, secret := range []string{jc.Token, "wrong", "a.png", "127.0.0.1"} {
		if strings.Contains(buf.String(), secret) {
			t.Errorf("log contains %q", secret)
		}
	}
	if stats.authFailed.Load() != 1 || stats.crossOrigin.Load() != 1 || stats.byClass[2].Load() < 2 {
		t.Errorf("stats auth %d cross %d 2xx %d", stats.authFailed.Load(), stats.crossOrigin.Load(), stats.byClass[2].Load())
	}

	out := renderMetrics(e.m.Stats(), stats)
	for _, want := range []string{
		"pixsheaf_jobs 1\n",
		`pixsheaf_security_events_total{event="auth_failed"} 1`,
		`pixsheaf_http_responses_total{class="4xx"} 2`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics lack %q:\n%s", want, out)
		}
	}
}
