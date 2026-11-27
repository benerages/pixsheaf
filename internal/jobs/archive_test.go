package jobs

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/benerages/pixsheaf/internal/jobproto"
)

var zipHeader = []byte("PK\x03\x04 the rest does not matter to the API")

func archiveManager(t *testing.T) (*Manager, string) {
	t.Helper()
	m, dir := batchManager(t)
	m.lim.MaxArchiveBytes, m.lim.MaxArchiveEntries, m.lim.MaxFiles = 2000, 50, 4
	return m, dir
}

func claimTask(t *testing.T, m *Manager) Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	task, err := m.Claim(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = task.Input.Close()
	return task
}

func TestArchiveUpload(t *testing.T) {
	m, dir := archiveManager(t)
	j, _, _ := m.Create([]byte(`{}`), 1, Naming{Pattern: "{index}-{name}", KeepFolders: true}, "c")
	f, err := m.AddFile(j.ID, "photos.zip", bytes.NewReader(zipHeader))
	if err != nil {
		t.Fatal(err)
	}
	if f.Kind != KindArchive || f.InputFormat != FormatZip {
		t.Fatalf("archive file %+v", f)
	}
	task := claimTask(t, m)
	r := task.Request
	if r.Op != jobproto.OpUnzip || r.MaxEntries != 50 || r.MaxFiles != 4 || r.MaxEntryBytes != 1000 || r.MaxTotalBytes != 5000 {
		t.Fatalf("unzip request %+v", r)
	}

	png := string(pngHeader)
	output := png + "not an image" + png
	resp := jobproto.Response{OutputBytes: int64(len(output)), Entries: []jobproto.ArchiveEntry{
		{Name: "a/one.png", Bytes: int64(len(png))},
		{Name: "notes.txt", Bytes: 12},
		{Name: "link.png", Code: "archive_entry_unsupported"},
		{Name: "b/two.png", Bytes: int64(len(png))},
	}}
	if err := m.Complete(task.ID, resp, strings.NewReader(output)); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get(j.ID)
	if got.Status != JobProcessing || len(got.Files) != 5 || got.Files[0].Status != FileDone {
		t.Fatalf("job after unpacking %+v", got)
	}
	wantStatus := []string{FileQueued, FileFailed, FileFailed, FileQueued}
	for i, w := range wantStatus {
		g := got.Files[i+1]
		if g.Status != w || g.ArchiveID != got.Files[0].ID || g.Kind != KindImage {
			t.Errorf("entry %d: %+v", i, g)
		}
	}
	if got.Files[2].ErrorCode != jobproto.CodeUnsupportedFormat || got.Files[3].ErrorCode != "archive_entry_unsupported" {
		t.Errorf("skip codes %q %q", got.Files[2].ErrorCode, got.Files[3].ErrorCode)
	}

	finish(t, m, "webp", 1, 1)
	finish(t, m, "webp", 1, 1)
	d, err := m.TakeDownload(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A job with one expected upload that was an archive downloads a ZIP;
	// {index} counts images only.
	if !d.IsArchive() || len(d.Entries) != 2 || d.Entries[0].Path != "a/1-one.webp" || d.Entries[1].Path != "b/4-two.webp" {
		t.Errorf("download entries %+v", d.Entries)
	}
	d.Release()
	if n := countFiles(t, dir); n != 0 || m.stored != 0 {
		t.Errorf("%d blobs, %d bytes stored after release", n, m.stored)
	}
}

func TestArchiveRejectedResults(t *testing.T) {
	m, _ := archiveManager(t)
	for name, resp := range map[string]jobproto.Response{
		"sum mismatch":    {OutputBytes: 5, Entries: []jobproto.ArchiveEntry{{Name: "a.png", Bytes: 4}}},
		"too many":        {Entries: []jobproto.ArchiveEntry{{Name: "1", Code: "too_large"}, {Name: "2", Code: "too_large"}, {Name: "3", Code: "too_large"}, {Name: "4", Code: "too_large"}, {Name: "5", Code: "too_large"}}},
		"unknown code":    {Entries: []jobproto.ArchiveEntry{{Name: "a", Code: "whatever"}}},
		"skip with bytes": {OutputBytes: 1, Entries: []jobproto.ArchiveEntry{{Name: "a", Bytes: 1, Code: "too_large"}}},
		"entry too big":   {OutputBytes: 1001, Entries: []jobproto.ArchiveEntry{{Name: "a", Bytes: 1001}}},
	} {
		j, _, err := m.Create([]byte(`{}`), 1, Naming{}, "c-"+name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.AddFile(j.ID, "x.zip", bytes.NewReader(zipHeader)); err != nil {
			t.Fatal(err)
		}
		task := claimTask(t, m)
		if err := m.Complete(task.ID, resp, strings.NewReader(strings.Repeat("x", int(resp.OutputBytes)))); err == nil {
			t.Errorf("%s: accepted", name)
		}
		got, _ := m.Get(j.ID)
		if len(got.Files) != 1 || got.Files[0].Status != FileFailed {
			t.Errorf("%s: job %+v", name, got)
		}
		_ = m.Delete(j.ID)
	}
}

func TestArchiveWorkerError(t *testing.T) {
	m, _ := archiveManager(t)
	j, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "c")
	if _, err := m.AddFile(j.ID, "x.zip", bytes.NewReader(zipHeader)); err != nil {
		t.Fatal(err)
	}
	task := claimTask(t, m)
	err := m.Complete(task.ID, jobproto.Response{Error: &jobproto.Error{Code: "too_many_files", Message: "too many"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get(j.ID)
	if got.Status != JobDone || got.Files[0].ErrorCode != "too_many_files" {
		t.Errorf("job %+v", got)
	}
}

func TestArchiveSizeLimit(t *testing.T) {
	m, _ := archiveManager(t)
	j, _, _ := m.Create([]byte(`{}`), 1, Naming{}, "c")
	big := append(bytes.Clone(zipHeader), make([]byte, 2000)...)
	if _, err := m.AddFile(j.ID, "x.zip", bytes.NewReader(big)); err == nil {
		t.Error("archive above MaxArchiveBytes accepted")
	}
	// An image above MaxUploadBytes but below MaxArchiveBytes is still too large.
	img := append(bytes.Clone(pngHeader), make([]byte, 1500)...)
	if _, err := m.AddFile(j.ID, "x.png", bytes.NewReader(img)); err == nil {
		t.Error("image above MaxUploadBytes accepted")
	}
}
