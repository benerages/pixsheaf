//go:build vips

package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	goimage "image"
	"image/color"
	"image/jpeg"
	"testing"

	"golang.org/x/image/bmp"
)

func encodeTestBMP(t *testing.T, img goimage.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := bmp.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestBMPInput(t *testing.T) {
	rgb := goimage.NewRGBA(goimage.Rect(0, 0, 40, 30))
	for i := range rgb.Pix {
		rgb.Pix[i] = 255
	}
	rgb.Set(0, 0, color.RGBA{R: 255, A: 255})
	pal := goimage.NewPaletted(goimage.Rect(0, 0, 40, 30), color.Palette{color.Black, color.White})
	for name, in := range map[string][]byte{"24-bit": encodeTestBMP(t, rgb), "8-bit": encodeTestBMP(t, pal)} {
		info, err := Inspect(in)
		if err != nil || info.Format != "bmp" || info.Width != 40 || info.Height != 30 {
			t.Fatalf("%s: Inspect = %+v, %v", name, info, err)
		}
		out, err := Process(in, parse(t, `{"version":1,"steps":[{"op":"resize","params":{"width":20}},{"op":"convert","params":{"format":"png"}}]}`), ProcessLimits{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		img := decodePNG(t, out.Data)
		if b := img.Bounds(); b.Dx() != 20 || b.Dy() != 15 {
			t.Errorf("%s: %dx%d, want 20x15", name, b.Dx(), b.Dy())
		}
		// An opaque BMP gives an image without alpha.
		if _, ok := img.(*goimage.RGBA); !ok {
			t.Errorf("%s: decoded as %T, want RGB", name, img)
		}
	}
}

func TestBMPOutput(t *testing.T) {
	// A transparent PNG: BMP output is flattened onto white, 24-bit.
	in := solidPNG(t, 30, 20, color.NRGBA{R: 255, A: 0})
	out, err := Process(in, parse(t, `{"version":1,"steps":[{"op":"convert","params":{"format":"bmp"}}]}`), ProcessLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Format != "bmp" || !isBMP(out.Data) {
		t.Fatalf("format %q, data %q", out.Format, out.Data[:2])
	}
	if bpp := binary.LittleEndian.Uint16(out.Data[28:]); bpp != 24 {
		t.Errorf("%d bits per pixel, want 24", bpp)
	}
	img, err := bmp.Decode(bytes.NewReader(out.Data))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 30 || b.Dy() != 20 {
		t.Errorf("%dx%d, want 30x20", b.Dx(), b.Dy())
	}
	if c := color.RGBAModel.Convert(img.At(5, 5)).(color.RGBA); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("pixel %+v, want white", c)
	}
}

func TestBMPRejected(t *testing.T) {
	valid := encodeTestBMP(t, goimage.NewRGBA(goimage.Rect(0, 0, 40, 30)))
	p := parse(t, `{"version":1,"steps":[]}`)

	rle := bytes.Clone(valid)
	binary.LittleEndian.PutUint32(rle[30:], 1) // BI_RLE8
	if _, err := Process(rle, p, ProcessLimits{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("RLE: err = %v, want ErrUnsupported", err)
	}
	if _, err := Process(valid[:len(valid)-100], p, ProcessLimits{}); !errors.Is(err, ErrDecode) {
		t.Errorf("truncated: err = %v, want ErrDecode", err)
	}
	// A header that claims a huge image is rejected before allocation.
	huge := bytes.Clone(valid)
	binary.LittleEndian.PutUint32(huge[18:], 30000)
	binary.LittleEndian.PutUint32(huge[22:], 30000)
	if _, err := Process(huge, p, ProcessLimits{}); !errors.Is(err, ErrTooManyPixels) {
		t.Errorf("huge: err = %v, want ErrTooManyPixels", err)
	}
	if _, err := Process(huge, p, ProcessLimits{MaxInputPixels: 1000}); !errors.Is(err, ErrTooManyPixels) {
		t.Errorf("limit: err = %v, want ErrTooManyPixels", err)
	}
}

func TestPDFOutput(t *testing.T) {
	in := solidPNG(t, 60, 40, color.NRGBA{G: 200, A: 128})
	out, err := Process(in, parse(t, `{"version":1,"steps":[{"op":"convert","params":{"format":"pdf","quality":90}}]}`), ProcessLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Format != "pdf" || !bytes.HasPrefix(out.Data, []byte("%PDF-")) {
		t.Fatalf("format %q, data %q", out.Format, out.Data[:8])
	}
	start := bytes.Index(out.Data, []byte{0xFF, 0xD8})
	end := bytes.LastIndex(out.Data, []byte{0xFF, 0xD9})
	if start < 0 || end < start {
		t.Fatal("no JPEG in the PDF")
	}
	img, err := jpeg.Decode(bytes.NewReader(out.Data[start : end+2]))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 60 || b.Dy() != 40 {
		t.Errorf("embedded %dx%d, want 60x40", b.Dx(), b.Dy())
	}
	if !bytes.Contains(out.Data, []byte("/ColorSpace /DeviceRGB")) {
		t.Error("want DeviceRGB")
	}
}
