package jobs

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

var sniffFormats = map[string]bool{
	"": true, "jpeg": true, "png": true, "gif": true, "webp": true, "tiff": true,
	"bmp": true, "jxl": true, "avif": true, "heif": true,
}

func FuzzSniff(f *testing.F) {
	for _, s := range []string{
		"\xFF\xD8\xFF\xE0", "\x89PNG\r\n\x1a\n", "GIF89a", "RIFF\x00\x00\x00\x00WEBPVP8 ",
		"II*\x00", "BM\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00",
		"\x00\x00\x00\x1cftypavif", "\x00\x00\x00\x18ftypheic", "\xFF\x0A", "<svg", "",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		got := Sniff(b)
		if !sniffFormats[got] {
			t.Fatalf("Sniff returned unknown format %q", got)
		}
		// Only the first SniffLen bytes may matter.
		if len(b) > SniffLen && Sniff(b[:SniffLen]) != got {
			t.Fatalf("result depends on bytes after SniffLen")
		}
	})
}

func FuzzOutputName(f *testing.F) {
	for _, s := range []string{"photo.HEIC", "../../etc/passwd", `C:\x\y.png`, "a\x00b", "\u202e.png", "...", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, uploaded string) {
		name := OutputName(uploaded, "png")
		base, ok := strings.CutSuffix(name, ".png")
		if !ok || base == "" {
			t.Fatalf("OutputName(%q) = %q: missing base or extension", uploaded, name)
		}
		if !utf8.ValidString(name) || utf8.RuneCountInString(base) > maxBaseName {
			t.Fatalf("OutputName(%q) = %q: invalid UTF-8 or too long", uploaded, name)
		}
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, " ") {
			t.Fatalf("OutputName(%q) = %q: leading dot or space", uploaded, name)
		}
		for _, r := range name {
			if r == '/' || r == '\\' || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				t.Fatalf("OutputName(%q) = %q: contains %U", uploaded, name, r)
			}
		}
	})
}
