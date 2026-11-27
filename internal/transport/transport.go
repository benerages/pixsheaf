// Package transport implements the internal channel between the API and
// the workers: HTTP over a Unix socket on a shared volume (ADR 0007).
// The API owns and listens on the socket; workers connect to it.
package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

// PingPath is answered by the API on the internal socket.
const PingPath = "/internal/v1/ping"

// Listen creates the Unix socket at path, replacing a stale socket from a
// previous run, and restricts it to its owner (mode 0600). API and worker
// run as the same UID (ADR 0007), so no group or world access is needed.
func Listen(ctx context.Context, path string) (net.Listener, error) {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%s exists and is not a socket", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return ln, nil
}

// NewServer returns the internal HTTP server for workers.
func NewServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+PingPath, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "pong")
	})
	return &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// NewClient returns an HTTP client whose connections always go to the Unix
// socket at path, regardless of the URL host.
func NewClient(path string, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", path)
			},
			DisableCompression: true,
		},
	}
}

// Ping checks that the API answers on the socket.
func Ping(ctx context.Context, c *http.Client) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://pixsheaf"+PingPath, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	if resp.StatusCode != http.StatusOK || string(body) != "pong" {
		return errors.New("unexpected ping response")
	}
	return nil
}
