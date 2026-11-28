package imaging

import "testing"

func TestIsMemoryMessage(t *testing.T) {
	for msg, want := range map[string]bool{
		"vips_tracked_malloc: out of memory --- size == 146MB":         true,
		"GLib-ERROR **: gmem.c:177: failed to allocate 67108864 bytes": true,
		"fatal error: runtime: out of memory":                          true,
		"Insufficient memory (case 4)":                                 true,
		"webpsave: picture memory error":                               true,
		"tiff2vips: Memory allocation failed":                          true,
		"open: Cannot allocate memory":                                 true,
		"webpsave: unable to encode | webpsave: mux error":             false,
		"VipsJpeg: Premature end of input file":                        false,
		"heifsave: Unsupported compression":                            false,
		"":                                                             false,
	} {
		if got := IsMemoryMessage(msg); got != want {
			t.Errorf("IsMemoryMessage(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestEstimateBytes(t *testing.T) {
	const photo = 7401 * 4934 // the 36.5 MP photo the WebP figures come from
	webp := EstimateBytes(photo, photo, "webp", false)
	if webp < 300<<20 || webp > 400<<20 {
		t.Errorf("WebP estimate for 36.5 MP = %d MB, want 300-400 MB (measured: about 350 MB)", webp>>20)
	}
	if lossless := EstimateBytes(photo, photo, "webp", true); lossless < 2*webp {
		t.Errorf("lossless WebP estimate %d MB is not well above lossy %d MB", lossless>>20, webp>>20)
	}
	if small := EstimateBytes(photo, photo/16, "webp", false); small >= webp {
		t.Errorf("a smaller output should need less memory: %d >= %d", small, webp)
	}
	if unknown := EstimateBytes(1000, 1000, "unknown", false); unknown <= baseJobBytes {
		t.Errorf("unknown format estimate %d is not above the base", unknown)
	}
}
