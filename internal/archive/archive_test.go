package archive

import (
	"archive/zip"
	"bytes"
	"errors"
	"hash/crc32"
	"io/fs"
	"strings"
	"testing"
)

var lim = Limits{MaxEntries: 100, MaxFiles: 50, MaxEntryBytes: 1 << 20, MaxTotalBytes: 3 << 20, MaxRatio: 100}

type zipEntry struct {
	name  string
	data  []byte
	mode  fs.FileMode
	store bool
	raw   *zip.FileHeader // written with CreateRaw, data as given
	isDir bool
}

func makeZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		switch {
		case e.raw != nil:
			w, err := zw.CreateRaw(e.raw)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write(e.data)
		case e.isDir:
			if _, err := zw.Create(e.name + "/"); err != nil {
				t.Fatal(err)
			}
		default:
			fh := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
			if e.store {
				fh.Method = zip.Store
			}
			if e.mode != 0 {
				fh.SetMode(e.mode)
			}
			w, err := zw.CreateHeader(fh)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write(e.data)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func noise(n int) []byte {
	b := make([]byte, n)
	x := uint32(1)
	for i := range b {
		x = x*1664525 + 1013904223
		b[i] = byte(x >> 24)
	}
	return b
}

func TestScanAndWrite(t *testing.T) {
	a, b := noise(1000), noise(2000)
	data := makeZip(t,
		zipEntry{name: "photos", isDir: true},
		zipEntry{name: "photos/a.jpg", data: a},
		zipEntry{name: "../../etc/b.png", data: b, store: true},
		zipEntry{name: "link.jpg", data: []byte("/etc/passwd"), mode: fs.ModeSymlink | 0o777},
		zipEntry{name: "inner.zip", data: makeZip(t, zipEntry{name: "x", data: []byte("x")})},
		zipEntry{name: "bomb.png", data: make([]byte, 1<<20)},
		zipEntry{name: "big.png", data: noise(2 << 20), store: true},
		zipEntry{name: "/abs/c.gif", data: []byte("GIF89a")},
	)
	entries, err := Scan(data, lim)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name  string
		bytes int64
		skip  string
	}{
		{"photos/a.jpg", 1000, ""},
		{"etc/b.png", 2000, ""},
		{"link.jpg", 0, SkipUnsupported},
		{"inner.zip", 0, SkipUnsupported},
		{"bomb.png", 0, SkipTooLarge},
		{"big.png", 0, SkipTooLarge},
		{"abs/c.gif", 6, ""},
	}
	if len(entries) != len(want) {
		t.Fatalf("%d entries: %+v", len(entries), entries)
	}
	for i, w := range want {
		if e := entries[i]; e.Name != w.name || e.Bytes != w.bytes || e.Skip != w.skip {
			t.Errorf("entry %d: %+v, want %+v", i, e, w)
		}
	}
	var out bytes.Buffer
	if err := Write(data, entries, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), append(append(append([]byte{}, a...), b...), "GIF89a"...)) {
		t.Errorf("output of %d bytes does not match the accepted entries", out.Len())
	}
}

func TestScanLimits(t *testing.T) {
	var many []zipEntry
	for i := range 51 {
		many = append(many, zipEntry{name: strings.Repeat("x", i+1), data: []byte("x")})
	}
	if _, err := Scan(makeZip(t, many...), lim); !errors.Is(err, ErrTooManyFiles) {
		t.Errorf("51 files: err = %v", err)
	}
	l := lim
	l.MaxEntries = 50
	if _, err := Scan(makeZip(t, many...), l); !errors.Is(err, ErrTooManyFiles) {
		t.Errorf("51 entries: err = %v", err)
	}
	// The total limit skips entries once it is reached.
	l = lim
	l.MaxTotalBytes = 1500
	entries, err := Scan(makeZip(t, zipEntry{name: "a", data: noise(1000)}, zipEntry{name: "b", data: noise(1000)}, zipEntry{name: "c", data: noise(400)}), l)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].Skip != "" || entries[1].Skip != SkipTooLarge || entries[2].Skip != "" {
		t.Errorf("total limit: %+v", entries)
	}
	for _, bad := range [][]byte{nil, []byte("PK\x03\x04 not really"), noise(5000)} {
		if _, err := Scan(bad, lim); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid archive: err = %v", err)
		}
	}
}

func TestScanRawHeaders(t *testing.T) {
	payload := []byte("hello")
	good := &zip.FileHeader{Name: "ok.png", Method: zip.Store, CRC32: crc32.ChecksumIEEE(payload),
		CompressedSize64: 5, UncompressedSize64: 5}
	badCRC := *good
	badCRC.Name, badCRC.CRC32 = "crc.png", 1
	encrypted := *good
	encrypted.Name, encrypted.Flags = "secret.png", 0x1
	method := *good
	method.Name, method.Method = "lzma.png", 14
	data := makeZip(t,
		zipEntry{raw: good, data: payload},
		zipEntry{raw: &badCRC, data: payload},
		zipEntry{raw: &encrypted, data: payload},
		zipEntry{raw: &method, data: payload},
	)
	entries, err := Scan(data, lim)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{entries[0].Skip, entries[1].Skip, entries[2].Skip, entries[3].Skip}
	if strings.Join(got, ",") != ","+SkipInvalid+","+SkipUnsupported+","+SkipUnsupported {
		t.Errorf("skips %v", got)
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		"a/b.png": "a/b.png", "../../x.png": "x.png", "/abs/./y.png": "abs/y.png",
		`win\dir\z.png`: "win/dir/z.png", "": "file", "../..": "file", "a//b": "a/b",
	} {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

func FuzzScan(f *testing.F) {
	f.Add(makeZipF(f))
	f.Add([]byte("PK\x05\x06" + strings.Repeat("\x00", 18)))
	f.Fuzz(func(t *testing.T, data []byte) {
		entries, err := Scan(data, lim)
		if err != nil {
			return
		}
		var total int64
		for _, e := range entries {
			if strings.HasPrefix(e.Name, "/") || strings.Contains("/"+e.Name+"/", "/../") {
				t.Fatalf("unsafe name %q", e.Name)
			}
			if e.Bytes > lim.MaxEntryBytes || (e.Skip != "" && e.Bytes != 0) {
				t.Fatalf("entry %+v", e)
			}
			total += e.Bytes
		}
		var out bytes.Buffer
		if err := Write(data, entries, &out); err != nil || int64(out.Len()) != total || total > lim.MaxTotalBytes {
			t.Fatalf("write: %v, %d of %d bytes", err, out.Len(), total)
		}
	})
}

func makeZipF(f *testing.F) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("dir/a.png")
	_, _ = w.Write([]byte("\x89PNG\r\n\x1a\nsome data"))
	_ = zw.Close()
	return buf.Bytes()
}
