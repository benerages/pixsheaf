package jobexec

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/benerages/pixsheaf/internal/jobproto"
)

func TestUnzipInJobProcess(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range map[string]string{"a/one.png": "first", "two.png": "second!"} {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(data))
	}
	w, _ := zw.Create("nested.zip")
	_, _ = w.Write([]byte("PK\x03\x04nested"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	req := jobproto.Request{Op: jobproto.OpUnzip, MaxEntries: 10, MaxFiles: 10, MaxEntryBytes: 1 << 20, MaxTotalBytes: 1 << 20}
	resp, out, err := testRunner(t).Run(t.Context(), req, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil || len(resp.Entries) != 3 || resp.OutputBytes != int64(len(out)) {
		t.Fatalf("response %+v, %d output bytes", resp, len(out))
	}
	var accepted string
	for _, e := range resp.Entries {
		if e.Code == "" {
			accepted += e.Name + "=" + string(out[:e.Bytes]) + ";"
			out = out[e.Bytes:]
		} else if e.Name != "nested.zip" || e.Code != "archive_entry_unsupported" {
			t.Errorf("skipped entry %+v", e)
		}
	}
	if accepted != "a/one.png=first;two.png=second!;" && accepted != "two.png=second!;a/one.png=first;" {
		t.Errorf("entries %q", accepted)
	}

	req.MaxFiles = 2
	resp, _, err = testRunner(t).Run(t.Context(), req, buf.Bytes())
	if err != nil || resp.Error == nil || resp.Error.Code != jobproto.CodeTooManyFiles {
		t.Errorf("too many files: %+v, %v", resp, err)
	}
	resp, _, err = testRunner(t).Run(t.Context(), req, []byte("not a zip"))
	if err != nil || resp.Error == nil || resp.Error.Code != jobproto.CodeArchiveInvalid {
		t.Errorf("invalid archive: %+v, %v", resp, err)
	}
}
