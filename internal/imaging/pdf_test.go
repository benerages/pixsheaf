package imaging

import (
	"bytes"
	goimage "image"
	"image/jpeg"
	"regexp"
	"strconv"
	"testing"
)

func TestWrapJPEG(t *testing.T) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, goimage.NewRGBA(goimage.Rect(0, 0, 300, 200)), nil); err != nil {
		t.Fatal(err)
	}
	pdf := wrapJPEG(jpg.Bytes(), 300, 200, false)

	if !bytes.HasPrefix(pdf, []byte("%PDF-1.7\n")) || !bytes.HasSuffix(pdf, []byte("%%EOF\n")) {
		t.Fatal("missing PDF header or trailer")
	}
	// startxref points at the xref table, and every xref entry at its object.
	m := regexp.MustCompile(`startxref\n(\d+)\n`).FindSubmatch(pdf)
	if m == nil {
		t.Fatal("no startxref")
	}
	xref, _ := strconv.Atoi(string(m[1]))
	if !bytes.HasPrefix(pdf[xref:], []byte("xref\n0 6\n")) {
		t.Fatalf("startxref %d does not point at the xref table", xref)
	}
	entries := regexp.MustCompile(`(\d{10}) 00000 n `).FindAllSubmatch(pdf[xref:], -1)
	if len(entries) != 5 {
		t.Fatalf("%d xref entries, want 5", len(entries))
	}
	for i, e := range entries {
		off, _ := strconv.Atoi(string(e[1]))
		want := strconv.Itoa(i+1) + " 0 obj\n"
		if !bytes.HasPrefix(pdf[off:], []byte(want)) {
			t.Errorf("xref entry %d points at %q", i+1, pdf[off:off+10])
		}
	}
	// The image stream holds the JPEG unchanged.
	if !bytes.Contains(pdf, jpg.Bytes()) {
		t.Error("JPEG not embedded verbatim")
	}
	for _, want := range []string{"/Width 300 /Height 200", "/ColorSpace /DeviceRGB", "/Filter /DCTDecode",
		"/Length " + strconv.Itoa(jpg.Len()) + " ", "/MediaBox [0 0 841.89 595.28]"} {
		if !bytes.Contains(pdf, []byte(want)) {
			t.Errorf("missing %q", want)
		}
	}
	// 300 px at 96 DPI is 225 pt, centered on the landscape page.
	checkPlacement(t, pdf, 225, 150, 308.445, 222.64)
}

// checkPlacement compares the cm operator of the content stream.
func checkPlacement(t *testing.T, pdf []byte, w, h, x, y float64) {
	t.Helper()
	m := regexp.MustCompile(`q ([\d.]+) 0 0 ([\d.]+) ([\d.]+) ([\d.]+) cm /Im0 Do Q`).FindSubmatch(pdf)
	if m == nil {
		t.Fatal("no placement in the content stream")
	}
	for i, want := range []float64{w, h, x, y} {
		got, _ := strconv.ParseFloat(string(m[i+1]), 64)
		if d := got - want; d < -0.01 || d > 0.01 {
			t.Errorf("placement %s, want %.3f %.3f %.3f %.3f", m[0], w, h, x, y)
			return
		}
	}
}

func TestWrapJPEGFitsPage(t *testing.T) {
	pdf := wrapJPEG([]byte{0xFF, 0xD8}, 1000, 10000, true)
	if !bytes.Contains(pdf, []byte("/ColorSpace /DeviceGray")) || !bytes.Contains(pdf, []byte("/MediaBox [0 0 595.28 841.89]")) {
		t.Error("expected a gray image on a portrait page")
	}
	// The height limits the scale: (841.89 - 72) / 10000.
	checkPlacement(t, pdf, 76.989, 769.89, (595.28-76.989)/2, 36)
}

// FuzzWrapJPEG checks the cross-reference table for arbitrary sizes and
// payloads: every offset must point at its object.
func FuzzWrapJPEG(f *testing.F) {
	f.Add([]byte{0xFF, 0xD8, 0xFF, 0xD9}, 300, 200, false)
	f.Add([]byte("endstream endobj xref"), 1, 65535, true)
	f.Fuzz(func(t *testing.T, payload []byte, width, height int, gray bool) {
		if width < 1 || height < 1 || width > 1<<20 || height > 1<<20 {
			t.Skip()
		}
		pdf := wrapJPEG(payload, width, height, gray)
		m := regexp.MustCompile(`startxref\n(\d+)\n%%EOF\n$`).FindSubmatch(pdf)
		if m == nil {
			t.Fatal("no trailer at the end")
		}
		xref, _ := strconv.Atoi(string(m[1]))
		if xref >= len(pdf) || !bytes.HasPrefix(pdf[xref:], []byte("xref\n0 6\n")) {
			t.Fatalf("startxref %d does not point at the xref table", xref)
		}
		entries := regexp.MustCompile(`(\d{10}) 00000 n `).FindAllSubmatch(pdf[xref:], -1)
		if len(entries) != 5 {
			t.Fatalf("%d xref entries", len(entries))
		}
		for i, e := range entries {
			off, _ := strconv.Atoi(string(e[1]))
			if !bytes.HasPrefix(pdf[off:], []byte(strconv.Itoa(i+1)+" 0 obj\n")) {
				t.Fatalf("xref entry %d is wrong", i+1)
			}
		}
		if !bytes.Contains(pdf, []byte("/Length "+strconv.Itoa(len(payload))+" ")) {
			t.Fatal("image stream length is wrong")
		}
	})
}
