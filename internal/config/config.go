// Package config loads Pixsheaf settings from environment variables.
//
// Every setting has a secure default. Unknown PIXSHEAF_* variables are
// rejected so that typos do not silently fall back to defaults.
package config

import (
	"fmt"
	"log/slog"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Prefix is the common prefix of all configuration variables.
const Prefix = "PIXSHEAF_"

// Config holds the validated runtime configuration.
type Config struct {
	// ListenAddr is the public HTTP address of the API.
	ListenAddr string
	// SocketPath is the Unix socket the API serves workers on (ADR 0007).
	SocketPath string
	// MaxUploadMB is the maximum size of a single uploaded file.
	MaxUploadMB int
	// MaxFiles is the maximum number of files per job.
	MaxFiles int
	// JobTimeout bounds the processing time of one file.
	JobTimeout time.Duration
	// MaxConcurrency is the global number of files processed in parallel.
	MaxConcurrency int
	// JobMemoryMB caps the memory of one job process (RLIMIT_DATA).
	JobMemoryMB int
	// MaxJobMB bounds the uploads of one job together.
	MaxJobMB int
	// MaxOutputMB bounds one processed file.
	MaxOutputMB int
	// MaxInputPixels bounds width x height x frames of an upload.
	MaxInputPixels int64
	// MaxOutputPixels bounds every intermediate and final image.
	MaxOutputPixels int64
	// MaxFrames bounds the frames of an animated GIF or WebP.
	MaxFrames int
	// MaxWatermarkKB bounds an image watermark.
	MaxWatermarkKB int
	// MaxQueue is the number of files waiting for a worker; further
	// uploads get 503 with Retry-After.
	MaxQueue int
	// MaxJobs bounds the jobs kept in memory at the same time.
	MaxJobs int
	// JobTTL is how long a job and its files are kept if not downloaded.
	JobTTL time.Duration
	// TmpDir holds uploads and results (a tmpfs mount).
	TmpDir string
	// MaxStoreMB bounds all uploads and results stored in TmpDir; keep it
	// below the size of the tmpfs.
	MaxStoreMB int
	// TrustedProxies lists the networks whose X-Forwarded-For is honored.
	TrustedProxies []netip.Prefix
	// LogLevel is the minimum log level.
	LogLevel slog.Level
}

type setting struct {
	def   string
	apply func(c *Config, v string) error
}

var settings = map[string]setting{
	"LISTEN_ADDR": {"127.0.0.1:8080", func(c *Config, v string) error {
		if _, err := netip.ParseAddrPort(normalizeAddr(v)); err != nil {
			return fmt.Errorf("must be host:port, e.g. 127.0.0.1:8080: %w", err)
		}
		c.ListenAddr = v
		return nil
	}},
	"SOCKET": {"/run/pixsheaf/api.sock", func(c *Config, v string) error {
		if !strings.HasPrefix(v, "/") {
			return fmt.Errorf("must be an absolute path")
		}
		c.SocketPath = v
		return nil
	}},
	"MAX_UPLOAD_MB": {"25", intIn(1, 1024, func(c *Config, n int) { c.MaxUploadMB = n })},
	"MAX_FILES":     {"50", intIn(1, 10000, func(c *Config, n int) { c.MaxFiles = n })},
	"JOB_TIMEOUT": {"60s", func(c *Config, v string) error {
		d, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		if d < time.Second || d > 30*time.Minute {
			return fmt.Errorf("must be between 1s and 30m")
		}
		c.JobTimeout = d
		return nil
	}},
	"MAX_CONCURRENCY": {"2", intIn(1, 256, func(c *Config, n int) { c.MaxConcurrency = n })},
	"JOB_MEMORY_MB":   {"384", intIn(64, 65536, func(c *Config, n int) { c.JobMemoryMB = n })},
	"MAX_JOB_MB":      {"250", intIn(1, 65536, func(c *Config, n int) { c.MaxJobMB = n })},
	"MAX_OUTPUT_MB":   {"100", intIn(1, 256, func(c *Config, n int) { c.MaxOutputMB = n })},
	"MAX_INPUT_PIXELS": {"50000000", intIn(1, 1_000_000_000, func(c *Config, n int) {
		c.MaxInputPixels = int64(n)
	})},
	"MAX_OUTPUT_PIXELS": {"50000000", intIn(1, 1_000_000_000, func(c *Config, n int) {
		c.MaxOutputPixels = int64(n)
	})},
	"MAX_FRAMES":       {"300", intIn(1, 1000, func(c *Config, n int) { c.MaxFrames = n })},
	"MAX_WATERMARK_KB": {"512", intIn(0, 1024, func(c *Config, n int) { c.MaxWatermarkKB = n })},
	"MAX_QUEUE":        {"100", intIn(1, 100000, func(c *Config, n int) { c.MaxQueue = n })},
	"MAX_STORE_MB":     {"384", intIn(1, 1<<20, func(c *Config, n int) { c.MaxStoreMB = n })},
	"MAX_JOBS":         {"200", intIn(1, 100000, func(c *Config, n int) { c.MaxJobs = n })},
	"JOB_TTL": {"15m", func(c *Config, v string) error {
		d, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		if d < time.Minute || d > 24*time.Hour {
			return fmt.Errorf("must be between 1m and 24h")
		}
		c.JobTTL = d
		return nil
	}},
	"TMP_DIR": {"/tmp/pixsheaf", func(c *Config, v string) error {
		if !strings.HasPrefix(v, "/") || v == "/" {
			return fmt.Errorf("must be an absolute path below /")
		}
		c.TmpDir = v
		return nil
	}},
	"TRUSTED_PROXIES": {"", func(c *Config, v string) error {
		c.TrustedProxies = nil
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			p, err := netip.ParsePrefix(part)
			if err != nil {
				return fmt.Errorf("must be a comma-separated list of CIDR prefixes: %w", err)
			}
			c.TrustedProxies = append(c.TrustedProxies, p.Masked())
		}
		return nil
	}},
	"LOG_LEVEL": {"info", func(c *Config, v string) error {
		return c.LogLevel.UnmarshalText([]byte(v))
	}},
}

// Load builds a Config from an environment in os.Environ() format.
func Load(environ []string) (Config, error) {
	values := map[string]string{}
	var unknown []string
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(k, Prefix) {
			continue
		}
		name := strings.TrimPrefix(k, Prefix)
		if _, known := settings[name]; !known {
			unknown = append(unknown, k)
			continue
		}
		values[name] = v
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return Config{}, fmt.Errorf("unknown configuration variables: %s", strings.Join(unknown, ", "))
	}

	var c Config
	names := make([]string, 0, len(settings))
	for name := range settings {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		s := settings[name]
		v, set := values[name]
		if !set {
			v = s.def
		}
		if err := s.apply(&c, v); err != nil {
			return Config{}, fmt.Errorf("%s%s=%q: %w", Prefix, name, v, err)
		}
	}
	return c, nil
}

// LocalAddr returns an address for connecting to ListenAddr from the same
// host, mapping an unspecified host (":8080", "0.0.0.0:8080") to loopback.
func (c Config) LocalAddr() string {
	ap, err := netip.ParseAddrPort(normalizeAddr(c.ListenAddr))
	if err != nil {
		return c.ListenAddr
	}
	if ap.Addr().IsUnspecified() {
		ap = netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), ap.Port())
	}
	return ap.String()
}

func normalizeAddr(v string) string {
	if strings.HasPrefix(v, ":") {
		return "0.0.0.0" + v
	}
	return v
}

func intIn(lo, hi int, set func(*Config, int)) func(*Config, string) error {
	return func(c *Config, v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("must be an integer")
		}
		if n < lo || n > hi {
			return fmt.Errorf("must be between %d and %d", lo, hi)
		}
		set(c, n)
		return nil
	}
}
