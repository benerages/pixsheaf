//go:build vips

package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"testing"

	"github.com/benerages/pixsheaf/internal/pipeline"
)

func parse(t *testing.T, doc string) pipeline.Pipeline {
	t.Helper()
	p, err := pipeline.Parse([]byte(doc), pipeline.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// source is a 64x48 test image (saved as JPEG unless a suffix is given).
func source(t *testing.T, suffix string) []byte {
	t.Helper()
	buf, err := saveRect(suffix, 64, 48)
	if err != nil {
		t.Fatal(err)
	}
	return buf
}

func TestProcessGeometry(t *testing.T) {
	in := source(t, ".png")
	cases := []struct {
		name          string
		steps         string
		width, height int
	}{
		{"none", `[]`, 64, 48},
		{"longest edge", `[{"op":"resize","params":{"longestEdge":32}}]`, 32, 24},
		{"percent", `[{"op":"resize","params":{"percent":50}}]`, 32, 24},
		{"fill", `[{"op":"resize","params":{"width":20,"height":20,"mode":"fill"}}]`, 20, 20},
		{"resize crop", `[{"op":"resize","params":{"width":20,"height":20,"mode":"crop"}}]`, 20, 20},
		{"smart crop", `[{"op":"resize","params":{"width":20,"height":20,"mode":"crop","gravity":"attention"}}]`, 20, 20},
		{"no enlarge", `[{"op":"resize","params":{"width":640}}]`, 64, 48},
		{"aspect crop", `[{"op":"crop","params":{"aspectRatio":{"width":1,"height":1}}}]`, 48, 48},
		{"rect crop", `[{"op":"crop","params":{"rect":{"left":60,"top":40,"width":100,"height":100}}}]`, 4, 8},
		{"rotate", `[{"op":"rotate","params":{"angle":90}}]`, 48, 64},
		{"flip", `[{"op":"flip","params":{"direction":"vertical"}}]`, 64, 48},
		{"orient", `[{"op":"autoOrient"}]`, 64, 48},
		{"condition skips", `[{"op":"resize","params":{"width":10},"when":{"widthGt":100}}]`, 64, 48},
		{"condition applies", `[{"op":"resize","params":{"width":10},"when":{"orientation":"landscape"}}]`, 10, 8},
	}
	for _, tc := range cases {
		p := parse(t, `{"version":1,"steps":`+tc.steps+`}`)
		out, err := Process(in, p, ProcessLimits{})
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if out.Width != tc.width || out.Height != tc.height || out.Format != "png" {
			t.Errorf("%s: got %s %dx%d, want png %dx%d", tc.name, out.Format, out.Width, out.Height, tc.width, tc.height)
		}
		info, err := Inspect(out.Data)
		if err != nil || info.Width != tc.width || info.Height != tc.height {
			t.Errorf("%s: encoded image: %+v, %v", tc.name, info, err)
		}
	}
}

func TestProcessFormats(t *testing.T) {
	in := source(t, ".png")
	for _, format := range []string{"jpeg", "png", "webp", "gif", "tiff", "avif", "jxl"} {
		variants := []string{
			`{"format":"` + format + `"}`,
			`{"format":"` + format + `","quality":40,"optimize":false,"progressive":true}`,
		}
		if format != "jpeg" && format != "avif" {
			variants = append(variants, `{"format":"`+format+`","lossless":true}`)
		}
		for _, params := range variants {
			p := parse(t, `{"version":1,"steps":[{"op":"convert","params":`+params+`}]}`)
			out, err := Process(in, p, ProcessLimits{})
			if err != nil {
				t.Errorf("%s: %v", params, err)
				continue
			}
			info, err := Inspect(out.Data)
			want := format
			if format == "avif" {
				want = "avif"
			}
			if err != nil || info.Format != want || out.Format != format {
				t.Errorf("%s: output %s, detected %+v, %v", params, out.Format, info, err)
			}
		}
	}
}

func TestProcessDefaultFormatAndCondition(t *testing.T) {
	// HEIF output is not possible, so a HEIF source would become JPEG; a
	// convert step that only applies to other sources keeps the source
	// format here.
	in := source(t, ".webp")
	p := parse(t, `{"version":1,"steps":[{"op":"convert","params":{"format":"png"},"when":{"sourceFormat":["heif"]}}]}`)
	out, err := Process(in, p, ProcessLimits{})
	if err != nil || out.Format != "webp" {
		t.Fatalf("got %s, %v; want webp", out.Format, err)
	}
}

func TestProcessLimits(t *testing.T) {
	in := source(t, ".png")
	p := parse(t, `{"version":1,"steps":[]}`)
	if _, err := Process(in, p, ProcessLimits{MaxInputPixels: 64*48 - 1}); !errors.Is(err, ErrTooManyPixels) {
		t.Errorf("input limit: err = %v, want ErrTooManyPixels", err)
	}
	p = parse(t, `{"version":1,"steps":[{"op":"resize","params":{"percent":1000,"enlarge":true}}]}`)
	if _, err := Process(in, p, ProcessLimits{MaxOutputPixels: 100_000}); !errors.Is(err, ErrTooManyPixels) {
		t.Errorf("output limit: err = %v, want ErrTooManyPixels", err)
	}
}

func TestProcessErrors(t *testing.T) {
	in := source(t, ".png")
	p := parse(t, `{"version":1,"steps":[{"op":"crop","params":{"rect":{"left":100,"top":0,"width":1,"height":1}}}]}`)
	if _, err := Process(in, p, ProcessLimits{}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("crop outside: err = %v, want ErrInvalidInput", err)
	}
	p = parse(t, `{"version":1,"steps":[]}`)
	if _, err := Process([]byte("not an image"), p, ProcessLimits{}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("garbage: err = %v, want ErrUnsupported", err)
	}
}

func TestProcessStripsMetadataByDefault(t *testing.T) {
	in := source(t, ".jpg")
	out, err := Process(in, parse(t, `{"version":1,"steps":[]}`), ProcessLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if hasExif(t, out.Data) {
		t.Error("output contains EXIF although metadata was not kept")
	}
	out, err = Process(in, parse(t, `{"version":1,"steps":[],"metadata":{"keep":true}}`), ProcessLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasExif(t, out.Data) {
		t.Error("output lacks EXIF although metadata was kept")
	}
}

// hasExif reports whether data contains an EXIF block header.
func hasExif(t *testing.T, data []byte) bool {
	t.Helper()
	return bytes.Contains(data, []byte("Exif\x00\x00"))
}

func TestProcessTruncatedInputIsDecodeError(t *testing.T) {
	in := source(t, ".png")
	// Keep the header, cut the image data: the error appears while saving.
	_, err := Process(in[:len(in)/2], parse(t, `{"version":1,"steps":[]}`), ProcessLimits{})
	if !errors.Is(err, ErrDecode) {
		t.Errorf("err = %v, want ErrDecode", err)
	}
}

func TestProcessCorruptDataIsDecodeError(t *testing.T) {
	// A valid PNG signature and header with image data that is not zlib:
	// the header reads fine, the pixels do not.
	chunk := func(typ string, data []byte) []byte {
		b := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
		b = append(b, typ...)
		b = append(b, data...)
		return binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(append([]byte(typ), data...)))
	}
	ihdr := binary.BigEndian.AppendUint32(nil, 64)
	ihdr = binary.BigEndian.AppendUint32(ihdr, 48)
	ihdr = append(ihdr, 8, 2, 0, 0, 0)
	in := []byte("\x89PNG\r\n\x1a\n")
	in = append(in, chunk("IHDR", ihdr)...)
	in = append(in, chunk("IDAT", bytes.Repeat([]byte("not zlib data at all"), 20))...)
	in = append(in, chunk("IEND", nil)...)
	for _, format := range []string{"png", "webp", "jpeg"} {
		p := parse(t, `{"version":1,"steps":[{"op":"convert","params":{"format":"`+format+`"}}]}`)
		if _, err := Process(in, p, ProcessLimits{}); !errors.Is(err, ErrDecode) {
			t.Errorf("%s: err = %v, want ErrDecode", format, err)
		}
	}
}
