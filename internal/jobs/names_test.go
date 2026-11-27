package jobs

import "testing"

func TestOutputName(t *testing.T) {
	for _, tc := range []struct{ in, format, want string }{
		{"photo.HEIC", "jpeg", "photo.jpg"},
		{"../../etc/passwd", "png", "passwd.png"},
		{`C:\Users\x\Bild 1.png`, "webp", "Bild 1.webp"},
		{"Ärger über €.jpg", "avif", "Ärger über.avif"},
		{"a\x00b\nc\u202e.png", "png", "a_b_c.png"},
		{".hidden", "gif", "hidden.gif"},
		{"", "tiff", "image.tif"},
		{"...", "pdf", "image.pdf"},
		{"x.tar.gz", "bmp", "x.tar.bmp"},
	} {
		if got := OutputName(tc.in, tc.format); got != tc.want {
			t.Errorf("OutputName(%q, %s) = %q, want %q", tc.in, tc.format, got, tc.want)
		}
	}
	long := ""
	for range 300 {
		long += "a"
	}
	if got := OutputName(long+".png", "png"); len(got) != 104 {
		t.Errorf("long name: %d bytes", len(got))
	}
}

func TestSniff(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"\xFF\xD8\xFF\xE0", "jpeg"},
		{"\x89PNG\r\n\x1a\n", "png"},
		{"GIF89a", "gif"},
		{"RIFF\x00\x00\x00\x00WEBPVP8 ", "webp"},
		{"RIFF\x00\x00\x00\x00WAVEfmt ", ""},
		{"II*\x00", "tiff"},
		{"BM\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00", "bmp"},
		{"BM", ""},
		{"\x00\x00\x00\x1cftypavif", "avif"},
		{"\x00\x00\x00\x18ftypheic", "heif"},
		{"\x00\x00\x00\x18ftypmp42", ""},
		{"\xFF\x0A", "jxl"},
		{"<svg xmlns", ""},
		{"%PDF-1.7", ""},
		{"", ""},
	} {
		if got := Sniff([]byte(tc.in)); got != tc.want {
			t.Errorf("Sniff(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
