//go:build vips

package imaging

import (
	"bytes"
	"errors"
	"fmt"
	goimage "image"
	"image/color"
	"image/gif"
	"testing"
)

// animatedGIF has three 40x30 frames in red, green and blue.
func animatedGIF(t *testing.T) []byte {
	t.Helper()
	pal := color.Palette{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 0, 255, 255}, color.White}
	g := &gif.GIF{LoopCount: 0}
	for i := range 3 {
		f := goimage.NewPaletted(goimage.Rect(0, 0, 40, 30), pal)
		for j := range f.Pix {
			f.Pix[j] = uint8(i)
		}
		g.Image = append(g.Image, f)
		g.Delay = append(g.Delay, 10*(i+1))
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAnimationKeptInGIF(t *testing.T) {
	in := animatedGIF(t)
	steps := `[{"op":"resize","params":{"width":20}},{"op":"watermark","params":{"text":"x","size":30}},{"op":"rotate","params":{"angle":90}}]`
	out, err := Process(in, parse(t, `{"version":1,"steps":`+steps+`}`), ProcessLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Format != "gif" || out.Pages != 3 || out.Width != 15 || out.Height != 20 {
		t.Fatalf("output %s, %d frames of %dx%d; want gif, 3 frames of 15x20", out.Format, out.Pages, out.Width, out.Height)
	}
	g, err := gif.DecodeAll(bytes.NewReader(out.Data))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 3 {
		t.Fatalf("%d frames, want 3", len(g.Image))
	}
	for i, want := range []color.RGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}} {
		if b := g.Image[i].Bounds(); b.Dx() != 15 || b.Dy() != 20 {
			t.Errorf("frame %d: %dx%d", i, b.Dx(), b.Dy())
		}
		if got := color.RGBAModel.Convert(g.Image[i].At(1, 1)).(color.RGBA); got != want {
			t.Errorf("frame %d: pixel %+v, want %+v", i, got, want)
		}
		if g.Delay[i] != 10*(i+1) {
			t.Errorf("frame %d: delay %d, want %d", i, g.Delay[i], 10*(i+1))
		}
	}
}

func TestAnimationGIFToWebP(t *testing.T) {
	out, err := Process(animatedGIF(t), parse(t, `{"version":1,"steps":[{"op":"convert","params":{"format":"webp"}}]}`), ProcessLimits{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := Inspect(out.Data)
	if err != nil || info.Format != "webp" || info.Pages != 3 || info.Width != 40 || info.Height != 30 {
		t.Errorf("Inspect = %+v, %v; want webp with 3 frames of 40x30", info, err)
	}
}

func TestAnimationFirstFrameOnly(t *testing.T) {
	out, err := Process(animatedGIF(t), parse(t, `{"version":1,"steps":[{"op":"convert","params":{"format":"png"}}]}`), ProcessLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Pages != 1 || out.Width != 40 || out.Height != 30 {
		t.Fatalf("%d pages of %dx%d, want 1 of 40x30", out.Pages, out.Width, out.Height)
	}
	if got := color.RGBAModel.Convert(decodePNG(t, out.Data).At(1, 1)).(color.RGBA); got != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("pixel %+v, want the red first frame", got)
	}
}

func TestAnimationSmartCropIsCentered(t *testing.T) {
	steps := []string{
		`{"op":"crop","params":{"aspectRatio":{"width":1,"height":1},"gravity":"attention"}}`,
		`{"op":"resize","params":{"width":10,"height":10,"mode":"crop","gravity":"entropy"}}`,
	}
	for _, step := range steps {
		out, err := Process(animatedGIF(t), parse(t, fmt.Sprintf(`{"version":1,"steps":[%s]}`, step)), ProcessLimits{})
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if out.Pages != 3 || out.Width != out.Height {
			t.Errorf("%s: %d frames of %dx%d", step, out.Pages, out.Width, out.Height)
		}
	}
}

func TestAnimationFrameLimit(t *testing.T) {
	p := parse(t, `{"version":1,"steps":[]}`)
	if _, err := Process(animatedGIF(t), p, ProcessLimits{MaxFrames: 2}); !errors.Is(err, ErrTooManyFrames) {
		t.Errorf("err = %v, want ErrTooManyFrames", err)
	}
	if _, err := Process(animatedGIF(t), p, ProcessLimits{MaxFrames: 3}); err != nil {
		t.Errorf("at the limit: %v", err)
	}
}
