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
