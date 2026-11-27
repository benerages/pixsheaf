//go:build vips

package imaging

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	goimage "image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"testing"
)

// solidPNG encodes a w x h image in one color with the Go encoder.
func solidPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := goimage.NewNRGBA(goimage.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodePNG(t *testing.T, data []byte) goimage.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestWatermarkText(t *testing.T) {
	in := solidPNG(t, 200, 100, color.NRGBA{A: 255})
	p := parse(t, `{"version":1,"steps":[
		{"op":"watermark","params":{"text":"PIXSHEAF","size":50,"margin":0,"opacity":1,"position":"southeast"}},
		{"op":"convert","params":{"format":"png"}}]}`)
	out, err := Process(in, p, ProcessLimits{})
	if err != nil {
		t.Fatal(err)
	}
	img := decodePNG(t, out.Data)
	if out.Width != 200 || out.Height != 100 {
		t.Errorf("size %dx%d, want 200x100", out.Width, out.Height)
	}
	var left, right int
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r > 0x8000 {
				if x < 100 {
					left++
				} else {
					right++
				}
			}
		}
	}
	if right == 0 || left != 0 {
		t.Errorf("bright pixels: %d left, %d right; want text on the right half only", left, right)
	}
}

func TestWatermarkImage(t *testing.T) {
	in := solidPNG(t, 200, 100, color.NRGBA{A: 255})
	mark := base64.StdEncoding.EncodeToString(solidPNG(t, 10, 10, color.NRGBA{R: 255, A: 255}))
	for _, tc := range []struct {
		opacity float64
		wantR   uint8
	}{{1, 255}, {0.5, 128}} {
		p := parse(t, fmt.Sprintf(`{"version":1,"steps":[
			{"op":"watermark","params":{"image":%q,"size":10,"margin":0,"opacity":%g,"position":"northwest"}},
			{"op":"convert","params":{"format":"png"}}]}`, mark, tc.opacity))
		out, err := Process(in, p, ProcessLimits{})
		if err != nil {
			t.Fatal(err)
		}
		img := decodePNG(t, out.Data)
		// The base has no alpha band, so the result has none either.
		if _, ok := img.(*goimage.RGBA); !ok {
			t.Errorf("opacity %g: decoded as %T, want an RGB PNG without alpha", tc.opacity, img)
		}
		got := color.RGBAModel.Convert(img.At(10, 10)).(color.RGBA)
		if d := int(got.R) - int(tc.wantR); d < -3 || d > 3 || got.G != 0 {
			t.Errorf("opacity %g: pixel inside the watermark = %+v, want R≈%d", tc.opacity, got, tc.wantR)
		}
		if got := color.RGBAModel.Convert(img.At(30, 30)).(color.RGBA); got.R != 0 {
			t.Errorf("opacity %g: pixel outside the 20x20 watermark = %+v", tc.opacity, got)
		}
	}
}

func TestWatermarkImageRejected(t *testing.T) {
	in := solidPNG(t, 200, 100, color.NRGBA{A: 255})
	img := goimage.NewRGBA(goimage.Rect(0, 0, 8, 8))
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, img, nil); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"jpeg":    jpg.Bytes(),
		"garbage": []byte("not an image at all"),
	} {
		mark := base64.StdEncoding.EncodeToString(data)
		p := parse(t, fmt.Sprintf(`{"version":1,"steps":[{"op":"watermark","params":{"image":%q}}]}`, mark))
		if _, err := Process(in, p, ProcessLimits{}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}

// noisePNG is hard to compress, so the encoded size depends on quality.
func noisePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	r := rand.New(rand.NewPCG(1, 2))
	img := goimage.NewRGBA(goimage.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = uint8(r.IntN(256))
		if i%4 == 3 {
			img.Pix[i] = 255
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTargetSize(t *testing.T) {
	in := noisePNG(t, 256, 256)
	// Palette PNG of 256x256 noise needs about 66 KB (one byte per pixel).
	for format, kb := range map[string]int{"jpeg": 64, "webp": 64, "avif": 64, "png": 100} {
		t.Run(format, func(t *testing.T) {
			p := parse(t, fmt.Sprintf(`{"version":1,"steps":[{"op":"convert","params":{"format":%q,"targetSizeKb":%d}}]}`, format, kb))
			out, err := Process(in, p, ProcessLimits{})
			if err != nil {
				t.Fatal(err)
			}
			if len(out.Data) > kb*1024 {
				t.Errorf("%d bytes, want at most %d", len(out.Data), kb*1024)
			}
			// The search should use most of the budget, not the lowest quality.
			if format != "png" && len(out.Data) < kb*1024/3 {
				t.Errorf("%d bytes, far below the %d KB target", len(out.Data), kb)
			}
		})
	}
}

func TestTargetSizeUnreachable(t *testing.T) {
	in := noisePNG(t, 256, 256)
	for _, format := range []string{"jpeg", "tiff"} {
		p := parse(t, fmt.Sprintf(`{"version":1,"steps":[{"op":"convert","params":{"format":%q,"targetSizeKb":1}}]}`, format))
		if _, err := Process(in, p, ProcessLimits{}); !errors.Is(err, ErrTargetUnreachable) {
			t.Errorf("%s: err = %v, want ErrTargetUnreachable", format, err)
		}
	}
}

func TestTargetSizeLosslessFits(t *testing.T) {
	in := solidPNG(t, 64, 64, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	p := parse(t, `{"version":1,"steps":[{"op":"convert","params":{"format":"png","targetSizeKb":100}}]}`)
	out, err := Process(in, p, ProcessLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decodePNG(t, out.Data).(*goimage.Paletted); ok {
		t.Error("a PNG that fits losslessly was quantized")
	}
}
