package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	c, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ListenAddr != "127.0.0.1:8080" {
		t.Errorf("ListenAddr = %q, want loopback default", c.ListenAddr)
	}
	if c.SocketPath != "/run/pixsheaf/api.sock" || c.MaxUploadMB != 25 || c.MaxFiles != 50 ||
		c.JobTimeout != 60*time.Second || c.MaxConcurrency != 2 || len(c.TrustedProxies) != 0 ||
		c.LogLevel != slog.LevelInfo || c.JobMemoryMB != 384 {
		t.Errorf("unexpected defaults: %+v", c)
	}
}

func TestOverrides(t *testing.T) {
	c, err := Load([]string{
		"PIXSHEAF_LISTEN_ADDR=:9000",
		"PIXSHEAF_MAX_UPLOAD_MB=100",
		"PIXSHEAF_JOB_TIMEOUT=2m",
		"PIXSHEAF_TRUSTED_PROXIES=10.0.0.0/8, 192.168.1.5/32",
		"PIXSHEAF_LOG_LEVEL=debug",
		"PATH=/usr/bin",
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.MaxUploadMB != 100 || c.JobTimeout != 2*time.Minute || c.LogLevel != slog.LevelDebug {
		t.Errorf("overrides not applied: %+v", c)
	}
	if len(c.TrustedProxies) != 2 || c.TrustedProxies[0].String() != "10.0.0.0/8" {
		t.Errorf("TrustedProxies = %v", c.TrustedProxies)
	}
	if got := c.LocalAddr(); got != "127.0.0.1:9000" {
		t.Errorf("LocalAddr = %q, want 127.0.0.1:9000", got)
	}
}

func TestRejects(t *testing.T) {
	cases := map[string]string{
		"unknown variable":  "PIXSHEAF_MAX_UPLOADS=5",
		"not an integer":    "PIXSHEAF_MAX_FILES=many",
		"above maximum":     "PIXSHEAF_MAX_CONCURRENCY=1000",
		"below minimum":     "PIXSHEAF_MAX_UPLOAD_MB=0",
		"timeout too short": "PIXSHEAF_JOB_TIMEOUT=10ms",
		"bad CIDR":          "PIXSHEAF_TRUSTED_PROXIES=10.0.0.1",
		"relative socket":   "PIXSHEAF_SOCKET=api.sock",
		"bad listen addr":   "PIXSHEAF_LISTEN_ADDR=localhost",
		"bad log level":     "PIXSHEAF_LOG_LEVEL=loud",
	}
	for name, kv := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load([]string{kv}); err == nil {
				t.Fatalf("Load(%q) succeeded, want error", kv)
			} else if !strings.Contains(err.Error(), "PIXSHEAF_") {
				t.Errorf("error does not name the variable: %v", err)
			}
		})
	}
}
