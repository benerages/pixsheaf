package transport

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/benerages/pixsheaf/internal/jobproto"
	"github.com/benerages/pixsheaf/internal/jobs"
)

var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00")

func serveTasks(t *testing.T) (*jobs.Manager, *http.Client) {
	t.Helper()
	m, err := jobs.New(t.TempDir(), jobs.Limits{
		MaxUploadBytes: 1 << 20, MaxJobBytes: 1 << 20, MaxFiles: 1, MaxOutputBytes: 1 << 20,
		MaxConcurrency: 1, MaxQueued: 4, MaxJobs: 4, MaxJobsPerClient: 4, MaxStoredBytes: 1 << 30,
		TTL: time.Minute, LeaseTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(t.TempDir(), "api.sock")
	ln, err := Listen(t.Context(), sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(m)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = m.Close()
	})
	return m, NewClient(sock, 0)
}

func TestClaimAndReport(t *testing.T) {
	m, c := serveTasks(t)
	j, _, err := m.Create([]byte(`{"version":1,"steps":[]}`), 1, jobs.Naming{}, "client")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddFile(j.ID, "a.png", bytes.NewReader(pngHeader)); err != nil {
		t.Fatal(err)
	}

	task, err := Claim(t.Context(), c, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(task.Input, pngHeader) || task.Request.Op != jobproto.OpProcess {
		t.Fatalf("task %+v", task.Request)
	}
	resp := jobproto.Response{Output: &jobproto.ImageInfo{Format: "png", Width: 1, Height: 1, Pages: 1}}
	if err := Report(t.Context(), c, task.ID, resp, []byte("result")); err != nil {
		t.Fatal(err)
	}
	if err := Report(t.Context(), c, task.ID, resp, []byte("result")); err == nil {
		t.Error("second report accepted")
	}
	d, err := m.TakeDownload(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(d.File)
	_ = d.File.Close()
	if string(body) != "result" {
		t.Errorf("download %q", body)
	}
}

func TestClaimRefusesLargeInput(t *testing.T) {
	m, c := serveTasks(t)
	j, _, _ := m.Create([]byte(`{}`), 1, jobs.Naming{}, "client")
	if _, err := m.AddFile(j.ID, "a.png", bytes.NewReader(pngHeader)); err != nil {
		t.Fatal(err)
	}
	if _, err := Claim(t.Context(), c, 10); err == nil {
		t.Error("input above the worker limit accepted")
	}
}
