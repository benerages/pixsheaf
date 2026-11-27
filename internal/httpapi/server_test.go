package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ui := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Pixsheaf</title>")}}
	srv := New("127.0.0.1:0", ui, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)
	return ts
}

func TestHealthz(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("got %d %q", resp.StatusCode, body)
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	ts := newTestServer(t)
	for _, path := range []string{"/", "/healthz", "/does-not-exist"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		want := map[string]string{
			"Content-Security-Policy": ContentSecurityPolicy,
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "no-referrer",
			"X-Frame-Options":         "DENY",
		}
		for k, v := range want {
			if got := resp.Header.Get(k); got != v {
				t.Errorf("%s: %s = %q, want %q", path, k, got, v)
			}
		}
		if resp.Header.Get("Strict-Transport-Security") != "" {
			t.Errorf("%s: HSTS must not be sent over plain HTTP", path)
		}
	}
}

func TestOnlyGetAndHeadAllowed(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Post(ts.URL+"/healthz", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /healthz = %d, want 405", resp.StatusCode)
	}
}

func TestCSPHasNoUnsafeSources(t *testing.T) {
	for _, bad := range []string{"unsafe-inline", "unsafe-eval", "*"} {
		if contains(ContentSecurityPolicy, bad) {
			t.Errorf("CSP contains %q", bad)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
