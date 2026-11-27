//go:build vips

package imaging

import (
	"errors"
	"strings"
	"testing"
)

func TestLibraryVersion(t *testing.T) {
	if v := LibraryVersion(); !strings.HasPrefix(v, "8.") {
		t.Fatalf("LibraryVersion() = %q, want 8.x", v)
	}
}

func TestProbeAllowedFormats(t *testing.T) {
	for _, r := range Probe() {
		if r.Err != nil {
			t.Errorf("%s (%s): %v", r.Suffix, r.Loader, r.Err)
		}
	}
}

func TestSaversOutsideAllowlistFail(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	// Built-in savers (blocked) and savers not compiled in.
	for _, suffix := range []string{".v", ".csv", ".mat", ".ppm", ".hdr", ".dz", ".fits", ".exr", ".jp2", ".svg", ".pdf"} {
		if _, err := saveBlank(suffix, 16); err == nil {
			t.Errorf("saving %s succeeded, want an error", suffix)
		}
	}
}

func TestLoadersOutsideAllowlistFail(t *testing.T) {
	inputs := map[string][]byte{
		"ppm":    []byte("P6\n1 1\n255\n\x00\x00\x00"),
		"svg":    []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/>`),
		"pdf":    []byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n"),
		"csv":    []byte("1,2\n3,4\n"),
		"matrix": []byte("2 2\n1 2\n3 4\n"),
		"text":   []byte("hello"),
	}
	for name, buf := range inputs {
		if loader, err := FindLoader(buf); err == nil {
			t.Errorf("%s: loader %q accepted the input, want none", name, loader)
		}
	}
}

func TestFindLoaderEmptyInput(t *testing.T) {
	if _, err := FindLoader(nil); err == nil {
		t.Fatal("FindLoader(nil) succeeded, want an error")
	}
}

func TestInspectReadsHeader(t *testing.T) {
	for _, tc := range []struct{ suffix, format string }{
		{".jpg", "jpeg"}, {".png", "png"}, {".webp", "webp"}, {".gif", "gif"},
		{".tif", "tiff"}, {".avif", "avif"}, {".jxl", "jxl"},
	} {
		buf, err := saveBlank(tc.suffix, 64)
		if err != nil {
			t.Fatalf("%s: %v", tc.suffix, err)
		}
		info, err := Inspect(buf)
		if err != nil {
			t.Errorf("%s: Inspect: %v", tc.suffix, err)
			continue
		}
		if info.Format != tc.format || info.Width != 64 || info.Height != 64 || info.Pages != 1 {
			t.Errorf("%s: Inspect = %+v, want %s 64x64, 1 page", tc.suffix, info, tc.format)
		}
	}
}

func TestInspectErrors(t *testing.T) {
	if _, err := Inspect([]byte("%PDF-1.4\n")); !errors.Is(err, ErrUnsupported) {
		t.Errorf("PDF: err = %v, want ErrUnsupported", err)
	}
	// A PNG signature followed by garbage: the loader matches, the header
	// is broken.
	broken := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	if _, err := Inspect(broken); !errors.Is(err, ErrDecode) {
		t.Errorf("broken PNG: err = %v, want ErrDecode", err)
	}
}
