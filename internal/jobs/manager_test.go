package jobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/benerages/pixsheaf/internal/jobproto"
)

var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00")

func testLimits() Limits {
	return Limits{
		MaxUploadBytes: 1000, MaxJobBytes: 5000, MaxFiles: 10, MaxOutputBytes: 1000,
		MaxInputPixels: 100, MaxOutputPixels: 200, MaxFrames: 3,
		MaxConcurrency: 1, MaxQueued: 2, MaxJobs: 3, MaxJobsPerClient: 3, MaxStoredBytes: 1 << 20,
		TTL: time.Minute, LeaseTimeout: 10 * time.Second,
	}
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newManager(t *testing.T) (*Manager, *clock, string) {
	t.Helper()
	dir := t.TempDir()
	m, err := New(dir, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	c := &clock{t: time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)}
	m.now = c.now
	t.Cleanup(func() { _ = m.Close() })
	return m, c, dir
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func claim(t *testing.T, m *Manager) Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	task, err := m.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func okResponse(n int) jobproto.Response {
	return jobproto.Response{
		Output:      &jobproto.ImageInfo{Format: "webp", Width: 1, Height: 1, Pages: 1},
		OutputBytes: int64(n),
	}
}

func TestSingleFileLifecycle(t *testing.T) {
	m, _, dir := newManager(t)
	j, token, err := m.Create([]byte(`{"version":1,"steps":[]}`), 1, Naming{}, "client")
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 || len(j.ID) != 22 || j.Status != JobOpen {
		t.Fatalf("token %d chars, id %d chars, status %s", len(token), len(j.ID), j.Status)
	}
	if err := m.Authorize(j.ID, token); err != nil {
		t.Fatal(err)
	}
	// Change the last character to one that surely differs: a fixed "A"
	// matched the real token whenever that already ended in "A".
	last := "A"
	if token[42] == 'A' {
		last = "B"
	}
	for _, bad := range []string{"", token[:42] + last, "x"} {
		if err := m.Authorize(j.ID, bad); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("token %q: err = %v", bad, err)
		}
	}

	f, err := m.AddFile(j.ID, "../../etc/Holiday photo.HEIC", bytes.NewReader(pngHeader))
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != FileQueued || f.InputFormat != "png" || f.InputBytes != int64(len(pngHeader)) {
		t.Errorf("file %+v", f)
	}
	if _, err := m.AddFile(j.ID, "b.png", bytes.NewReader(pngHeader)); !errors.Is(err, ErrNotOpen) {
		t.Errorf("second upload: err = %v, want ErrNotOpen", err)
	}
	if _, err := m.TakeDownload(j.ID); !errors.Is(err, ErrNotReady) {
		t.Errorf("early download: err = %v, want ErrNotReady", err)
	}

	task := claim(t, m)
	in, _ := io.ReadAll(task.Input)
	_ = task.Input.Close()
	if !bytes.Equal(in, pngHeader) || task.Request.Op != jobproto.OpProcess || task.Request.MaxPixels != 100 ||
		task.Request.MaxFrames != 3 || string(task.Request.Pipeline) != `{"version":1,"steps":[]}` {
		t.Errorf("task %+v", task.Request)
	}
	if got, _ := m.Get(j.ID); got.Status != JobProcessing || got.Files[0].Status != FileProcessing {
		t.Errorf("while processing: %s / %s", got.Status, got.Files[0].Status)
	}

	if err := m.Complete(task.ID, okResponse(5), strings.NewReader("RIFFx")); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get(j.ID)
	if got.Status != JobDone || got.Files[0].OutputName != "Holiday photo.webp" || got.Files[0].OutputBytes != 5 {
		t.Errorf("done: %+v", got)
	}
	if n := countFiles(t, dir); n != 1 {
		t.Errorf("%d blobs after processing, want 1 (the output)", n)
	}

	d, err := m.TakeDownload(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(d.File)
	_ = d.File.Close()
	if string(body) != "RIFFx" || d.Name != "Holiday photo.webp" || d.ContentType != "image/webp" || d.Size != 5 {
		t.Errorf("download %q %+v", body, d)
	}
	if _, err := m.TakeDownload(j.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second download: err = %v, want ErrNotFound", err)
	}
	if n := countFiles(t, dir); n != 0 {
		t.Errorf("%d blobs after download, want 0", n)
	}
	if err := m.Complete(task.ID, okResponse(5), strings.NewReader("RIFFx")); !errors.Is(err, ErrUnknownTask) {
		t.Errorf("second result: err = %v, want ErrUnknownTask", err)
	}
}

func TestUploadRejected(t *testing.T) {
	m, _, dir := newManager(t)
	j, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "client")
	if _, err := m.AddFile(j.ID, "x", strings.NewReader("<svg>")); !errors.Is(err, ErrUnsupported) {
		t.Errorf("svg: err = %v", err)
	}
	big := append(bytes.Clone(pngHeader), make([]byte, 1000)...)
	if _, err := m.AddFile(j.ID, "x", bytes.NewReader(big)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("big: err = %v", err)
	}
	if _, err := m.AddFile("nope", "x", bytes.NewReader(pngHeader)); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown job: err = %v", err)
	}
	if n := countFiles(t, dir); n != 0 {
		t.Errorf("%d blobs left, want 0", n)
	}
	// A rejected upload does not use up the job's slot.
	if _, err := m.AddFile(j.ID, "x", bytes.NewReader(pngHeader)); err != nil {
		t.Errorf("valid upload after rejects: %v", err)
	}
}

func TestQueueAndJobLimits(t *testing.T) {
	m, _, _ := newManager(t)
	for i := range 2 {
		j, _, err := m.Create([]byte(`{}`), 1, Naming{}, "client")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.AddFile(j.ID, "x", bytes.NewReader(pngHeader)); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}
	j, _, err := m.Create([]byte(`{}`), 1, Naming{}, "client")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.AddFile(j.ID, "x", bytes.NewReader(pngHeader)); !errors.Is(err, ErrBusy) {
		t.Errorf("full queue: err = %v, want ErrBusy", err)
	}
	if _, _, err := m.Create([]byte(`{}`), 1, Naming{}, "client"); !errors.Is(err, ErrTooManyJobs) {
		t.Errorf("fourth job: err = %v, want ErrTooManyJobs", err)
	}
	if _, _, err := m.Create([]byte(`{}`), 11, Naming{}, "client"); err == nil {
		t.Error("expectedFiles above the limit accepted")
	}
}

func TestStorageLimit(t *testing.T) {
	dir := t.TempDir()
	lim := testLimits()
	lim.MaxStoredBytes = 1500 // room for one reserved upload (1000) only
	m, err := New(dir, lim)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	a, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "client")
	b, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "client")
	if _, err := m.AddFile(a.ID, "x", bytes.NewReader(pngHeader)); err != nil {
		t.Fatal(err)
	}
	// 29 bytes stored + 1000 reserved for the next upload still fit ...
	if _, err := m.AddFile(b.ID, "x", bytes.NewReader(pngHeader)); err != nil {
		t.Fatal(err)
	}
	c, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "client")
	lim.MaxStoredBytes = 1050
	m.lim = lim
	// ... but not with a lower limit.
	if _, err := m.AddFile(c.ID, "x", bytes.NewReader(pngHeader)); !errors.Is(err, ErrBusy) {
		t.Errorf("err = %v, want ErrBusy", err)
	}
	_ = m.Delete(a.ID)
	_ = m.Delete(b.ID)
	if m.stored != 0 || m.reserved != 0 {
		t.Errorf("stored %d, reserved %d after deleting everything", m.stored, m.reserved)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	m, _, _ := newManager(t)
	for range 2 {
		j, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "client")
		if _, err := m.AddFile(j.ID, "x", bytes.NewReader(pngHeader)); err != nil {
			t.Fatal(err)
		}
	}
	first := claim(t, m)
	_ = first.Input.Close()
	// MaxConcurrency is 1: the second claim waits until the first is done.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := m.Claim(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second claim: err = %v, want a timeout", err)
	}
	if err := m.Complete(first.ID, jobproto.Response{Error: &jobproto.Error{Code: "decode_failed", Message: "broken"}}, nil); err != nil {
		t.Fatal(err)
	}
	second := claim(t, m)
	_ = second.Input.Close()
}

func TestLeaseTimeoutAndExpiry(t *testing.T) {
	m, c, dir := newManager(t)
	j, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "client")
	if _, err := m.AddFile(j.ID, "x", bytes.NewReader(pngHeader)); err != nil {
		t.Fatal(err)
	}
	task := claim(t, m)
	_ = task.Input.Close()

	c.t = c.t.Add(11 * time.Second)
	m.Sweep()
	got, _ := m.Get(j.ID)
	if got.Status != JobDone || got.Files[0].Status != FileFailed || got.Files[0].ErrorCode != jobproto.CodeTimeout {
		t.Errorf("after lease timeout: %+v", got)
	}
	if _, err := m.TakeDownload(j.ID); !errors.Is(err, ErrNoResult) {
		t.Errorf("download of a failed file: err = %v, want ErrNoResult", err)
	}
	// A late result is discarded.
	if err := m.Complete(task.ID, okResponse(1), strings.NewReader("x")); !errors.Is(err, ErrUnknownTask) {
		t.Errorf("late result: err = %v", err)
	}

	j2, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "client")
	if _, err := m.AddFile(j2.ID, "x", bytes.NewReader(pngHeader)); err != nil {
		t.Fatal(err)
	}
	c.t = c.t.Add(2 * time.Minute)
	m.Sweep()
	if _, err := m.Get(j2.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired job still there: %v", err)
	}
	if n := countFiles(t, dir); n != 0 {
		t.Errorf("%d blobs after expiry, want 0", n)
	}
}

func TestDeleteDiscardsRunningResult(t *testing.T) {
	m, _, dir := newManager(t)
	j, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "client")
	if _, err := m.AddFile(j.ID, "x", bytes.NewReader(pngHeader)); err != nil {
		t.Fatal(err)
	}
	task := claim(t, m)
	_ = task.Input.Close()
	if err := m.Delete(j.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.Complete(task.ID, okResponse(1), strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if n := countFiles(t, dir); n != 0 {
		t.Errorf("%d blobs after delete, want 0", n)
	}
}

func TestInvalidWorkerResult(t *testing.T) {
	m, _, dir := newManager(t)
	for _, tc := range []struct {
		name string
		resp jobproto.Response
		body string
	}{
		{"too big", okResponse(2000), strings.Repeat("x", 2000)},
		{"short body", okResponse(10), "x"},
		{"bad format", jobproto.Response{Output: &jobproto.ImageInfo{Format: "exe", Width: 1, Height: 1, Pages: 1}, OutputBytes: 1}, "x"},
	} {
		j, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "client")
		if _, err := m.AddFile(j.ID, "x", bytes.NewReader(pngHeader)); err != nil {
			t.Fatal(err)
		}
		task := claim(t, m)
		_ = task.Input.Close()
		if err := m.Complete(task.ID, tc.resp, strings.NewReader(tc.body)); !errors.Is(err, ErrInvalidResult) {
			t.Errorf("%s: err = %v, want ErrInvalidResult", tc.name, err)
		}
		got, _ := m.Get(j.ID)
		if got.Files[0].Status != FileFailed || got.Files[0].ErrorCode != jobproto.CodeInternal {
			t.Errorf("%s: file %+v", tc.name, got.Files[0])
		}
		_ = m.Delete(j.ID)
	}
	if n := countFiles(t, dir); n != 0 {
		t.Errorf("%d blobs left, want 0", n)
	}
}

func TestDisabledFormats(t *testing.T) {
	dir := t.TempDir()
	lim := testLimits()
	lim.DisabledFormats = []string{"heif"}
	m, err := New(dir, lim)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	j, _, _ := m.Create([]byte(`{}`), 2, Naming{}, "client")
	if _, err := m.AddFile(j.ID, "a.heic", strings.NewReader("\x00\x00\x00\x18ftypheic")); !errors.Is(err, ErrFormatDisabled) {
		t.Errorf("heic: err = %v", err)
	}
	// AVIF shares the HEIF container but is not disabled.
	if _, err := m.AddFile(j.ID, "a.avif", strings.NewReader("\x00\x00\x00\x1cftypavif")); err != nil {
		t.Errorf("avif: %v", err)
	}
	if n := countFiles(t, dir); n != 1 {
		t.Errorf("%d blobs, want 1", n)
	}
	// The worker receives the list, so that it refuses the format too.
	if task := claimTask(t, m); len(task.Request.DisabledFormats) != 1 || task.Request.DisabledFormats[0] != "heif" {
		t.Errorf("request %+v", task.Request)
	}
	if s := m.Stats(); s.Jobs != 1 || s.InFlight != 1 {
		t.Errorf("stats %+v", s)
	}
}
