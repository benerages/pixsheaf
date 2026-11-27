package jobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/benerages/pixsheaf/internal/jobproto"
)

func batchManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	lim := testLimits()
	lim.MaxQueued, lim.MaxJobs, lim.MaxJobsPerClient = 100, 10, 2
	m, err := New(dir, lim)
	if err != nil {
		t.Fatal(err)
	}
	m.now = func() time.Time { return time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { _ = m.Close() })
	return m, dir
}

// finish completes the next task with an output of the given size.
func finish(t *testing.T, m *Manager, format string, w, h int) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	task, err := m.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = task.Input.Close()
	out := "out-" + task.ID
	resp := jobproto.Response{Output: &jobproto.ImageInfo{Format: format, Width: w, Height: h, Pages: 1}, OutputBytes: int64(len(out))}
	if err := m.Complete(task.ID, resp, strings.NewReader(out)); err != nil {
		t.Fatal(err)
	}
	return task.ID
}

func TestFairQueueAlternatesClients(t *testing.T) {
	m, _ := batchManager(t)
	big, _, _ := m.Create([]byte(`{}`), 5, Naming{}, "big")
	small, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "small")
	for range 5 {
		if _, err := m.AddFile(big.ID, "b.png", bytes.NewReader(pngHeader)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.AddFile(small.ID, "s.png", bytes.NewReader(pngHeader)); err != nil {
		t.Fatal(err)
	}
	m.lim.MaxConcurrency = 10
	var order []string
	for range 3 {
		task := claim(t, m)
		_ = task.Input.Close()
		order = append(order, m.leases[task.ID].file.job.client)
	}
	// The small client's only file comes second, not sixth.
	if strings.Join(order, ",") != "big,small,big" {
		t.Errorf("order %v, want big,small,big", order)
	}
}

func TestFairQueueRemove(t *testing.T) {
	q := newFairQueue()
	ja, jb := &job{client: "a"}, &job{client: "b"}
	files := []*file{{job: ja, index: 0}, {job: jb, index: 0}, {job: ja, index: 1}, {job: jb, index: 1}}
	for _, f := range files {
		q.push(f)
	}
	if q.pop() != files[0] {
		t.Fatal("first pop")
	}
	q.remove(func(f *file) bool { return f.job == jb })
	if q.len() != 1 || q.pop() != files[2] || q.pop() != nil || q.len() != 0 {
		t.Error("remove left the queue inconsistent")
	}
}

func TestClientJobLimit(t *testing.T) {
	m, _ := batchManager(t)
	a1, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "a")
	if _, _, err := m.Create([]byte(`{}`), 1, Naming{}, "a"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Create([]byte(`{}`), 1, Naming{}, "a"); !errors.Is(err, ErrClientJobs) {
		t.Errorf("third job: err = %v, want ErrClientJobs", err)
	}
	if _, _, err := m.Create([]byte(`{}`), 1, Naming{}, "b"); err != nil {
		t.Errorf("other client: %v", err)
	}
	_ = m.Delete(a1.ID)
	if _, _, err := m.Create([]byte(`{}`), 1, Naming{}, "a"); err != nil {
		t.Errorf("after delete: %v", err)
	}
}

func TestBatchDownload(t *testing.T) {
	m, dir := batchManager(t)
	j, _, err := m.Create([]byte(`{}`), 4, Naming{Pattern: "{index}_{name}_{width}x{height}", KeepFolders: true}, "c")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"trip/day 1/IMG.heic", "trip/IMG.heic", "broken.png", "x.png"} {
		if _, err := m.AddFile(j.ID, name, bytes.NewReader(pngHeader)); err != nil {
			t.Fatal(err)
		}
	}
	finish(t, m, "jpeg", 800, 600)
	finish(t, m, "jpeg", 600, 800)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	task, _ := m.Claim(ctx)
	cancel()
	_ = task.Input.Close()
	_ = m.Complete(task.ID, jobproto.Response{Error: &jobproto.Error{Code: "decode_failed", Message: "broken"}}, nil)
	finish(t, m, "webp", 10, 10)

	d, err := m.TakeDownload(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !d.IsArchive() || len(d.Entries) != 3 || len(d.Files) != 4 {
		t.Fatalf("download: archive %v, %d entries, %d files", d.IsArchive(), len(d.Entries), len(d.Files))
	}
	want := []string{"trip/day 1/1_IMG_800x600.jpg", "trip/2_IMG_600x800.jpg", "4_x_10x10.webp"}
	for i, e := range d.Entries {
		if e.Path != want[i] {
			t.Errorf("entry %d: %q, want %q", i, e.Path, want[i])
		}
		f, err := m.OpenEntry(e)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(f)
		_ = f.Close()
		if !strings.HasPrefix(string(b), "out-") || int64(len(b)) != e.Size {
			t.Errorf("entry %d content %q", i, b)
		}
	}
	if d.Files[2].Status != FileFailed || d.Files[2].ErrorCode != "decode_failed" || d.Files[0].Source != "trip/day 1/IMG.heic" {
		t.Errorf("report files %+v", d.Files)
	}
	if _, err := m.TakeDownload(j.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second download: %v", err)
	}
	d.Release()
	if n := countFiles(t, dir); n != 0 || m.stored != 0 {
		t.Errorf("%d blobs, %d bytes stored after release", n, m.stored)
	}
}

func TestBatchNamesAreUnique(t *testing.T) {
	m, _ := batchManager(t)
	j, _, _ := m.Create([]byte(`{}`), 3, Naming{}, "c")
	for _, name := range []string{"a/Photo.png", "b/photo.png", "PHOTO.jpg"} {
		if _, err := m.AddFile(j.ID, name, bytes.NewReader(pngHeader)); err != nil {
			t.Fatal(err)
		}
	}
	for range 3 {
		finish(t, m, "png", 1, 1)
	}
	d, err := m.TakeDownload(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Release()
	got := []string{d.Entries[0].Path, d.Entries[1].Path, d.Entries[2].Path}
	if strings.Join(got, "|") != "Photo.png|photo (2).png|PHOTO (3).png" {
		t.Errorf("names %v", got)
	}
}

func TestPatterns(t *testing.T) {
	for _, bad := range []string{"", "{nme}", "{name", "name}", "{{name}}", strings.Repeat("x", MaxPatternLen+1)} {
		if ValidatePattern(bad) == nil {
			t.Errorf("pattern %q accepted", bad)
		}
	}
	for _, good := range []string{"{name}", "photo", "{date}-{index}_{width}x{height}.{ext}"} {
		if err := ValidatePattern(good); err != nil {
			t.Errorf("pattern %q: %v", good, err)
		}
	}
	v := nameVars{name: "../IMG 1", index: 7, total: 120, width: 4, height: 3,
		date: time.Date(2026, 1, 2, 23, 0, 0, 0, time.UTC), format: "jpeg"}
	if got := expandPattern("{date}_{index}_{name}_{width}x{height}.{ext}", v); got != "2026-01-02_007_.._IMG 1_4x3.jpg.jpg" {
		t.Errorf("expandPattern = %q", got)
	}
	if got := expandPattern("a/b\\c", v); got != "a_b_c.jpg" {
		t.Errorf("separators in a pattern: %q", got)
	}
}
