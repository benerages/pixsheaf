package imaging

import (
	"image/color"
	"testing"
)

func TestRenderTextWidth(t *testing.T) {
	c := color.NRGBA{R: 255, G: 255, B: 255, A: 128}
	for _, width := range []int{40, 200, 1000} {
		img, err := RenderText("© Pixsheaf 2026", c, width, 10000)
		if err != nil {
			t.Fatalf("width %d: %v", width, err)
		}
		b := img.Bounds()
		// Rounding of the font size allows a few pixels of difference.
		if diff := b.Dx() - width; diff < -width/20-2 || diff > width/20+2 {
			t.Errorf("width %d: rendered %dx%d", width, b.Dx(), b.Dy())
		}
		if b.Dy() < 1 || b.Dy() > b.Dx() {
			t.Errorf("width %d: implausible height %d", width, b.Dy())
		}
		if !hasInk(img.Pix) {
			t.Errorf("width %d: nothing drawn", width)
		}
	}
}

func TestRenderTextFitsBox(t *testing.T) {
	c := color.NRGBA{A: 255}
	// One character in a wide, low box: the height limits the size.
	img, err := RenderText("W", c, 5000, 50)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dy() > 52 || b.Dx() > 5000 {
		t.Errorf("rendered %dx%d, want at most 5000x50", b.Dx(), b.Dy())
	}
	// A huge box: the pixel budget limits the size.
	img, err = RenderText("W", c, 100000, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); int64(b.Dx())*int64(b.Dy()) > maxRenderPixels*11/10 {
		t.Errorf("rendered %dx%d, more than the pixel budget", b.Dx(), b.Dy())
	}
}

func hasInk(pix []byte) bool {
	for i := 3; i < len(pix); i += 4 {
		if pix[i] > 0 {
			return true
		}
	}
	return false
}

func TestRenderTextRejectsEmpty(t *testing.T) {
	if _, err := RenderText("", color.NRGBA{}, 100, 100); err == nil {
		t.Error("empty text accepted")
	}
	if _, err := RenderText("x", color.NRGBA{}, 0, 100); err == nil {
		t.Error("zero width accepted")
	}
	if _, err := RenderText("x", color.NRGBA{}, 100, 0); err == nil {
		t.Error("zero height accepted")
	}
}

func TestParseColor(t *testing.T) {
	c, err := ParseColor("#FF8000", 0.5)
	if err != nil || c != (color.NRGBA{R: 255, G: 128, B: 0, A: 128}) {
		t.Errorf("ParseColor = %+v, %v", c, err)
	}
	for _, bad := range []string{"FF8000", "#FF80", "#GG8000"} {
		if _, err := ParseColor(bad, 1); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
