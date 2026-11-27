package transport

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPingOverSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "api.sock")
	ln, err := Listen(t.Context(), sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(nil)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	fi, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("socket mode = %o, want 600", perm)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := Ping(ctx, NewClient(sock, time.Second)); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestListenReplacesStaleSocketButNotFiles(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "api.sock")
	ln, err := Listen(t.Context(), sock)
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close() // leaves a stale socket file behind on some systems
	ln2, err := Listen(t.Context(), sock)
	if err != nil {
		t.Fatalf("second Listen: %v", err)
	}
	_ = ln2.Close()

	regular := filepath.Join(dir, "not-a-socket")
	if err := os.WriteFile(regular, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(t.Context(), regular); err == nil {
		t.Fatal("Listen replaced a regular file")
	}
}
