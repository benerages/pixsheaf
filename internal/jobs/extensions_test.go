package jobs

import "testing"

func TestAllowedExtension(t *testing.T) {
	for name, want := range map[string]bool{
		"photo.jpg": true, "IMG_0001.HEIC": true, "scan.TiFf": true, "trip/day 1/a.png": true,
		`C:\Users\a\b.webp`: true, "archive.zip": true, "a.b.jxl": true,
		"": false, "photo": false, ".png": false, "photo.": false, "photo.svg": false,
		"script.php": false, "image.png.exe": false, "readme.txt": false, "dir.png/file": false,
	} {
		if got := AllowedExtension(name); got != want {
			t.Errorf("AllowedExtension(%q) = %v, want %v", name, got, want)
		}
	}
}

func FuzzAllowedExtension(f *testing.F) {
	for _, s := range []string{"a.png", "a.PNG", "../a.jpg", "a.png\x00.exe", "a/.png"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if AllowedExtension(name) && !allowedExtensions[lowerExt(name)] {
			t.Fatalf("accepted %q", name)
		}
	})
}

func lowerExt(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		switch name[i] {
		case '.':
			s := name[i+1:]
			b := []byte(s)
			for j, c := range b {
				if c >= 'A' && c <= 'Z' {
					b[j] = c + 32
				}
			}
			return string(b)
		case '/', '\\':
			return ""
		}
	}
	return ""
}
