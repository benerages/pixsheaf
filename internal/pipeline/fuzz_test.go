package pipeline

import (
	"errors"
	"strings"
	"testing"
)

func FuzzParse(f *testing.F) {
	for _, s := range []string{
		`{"version":1,"steps":[]}`,
		`{"version":1,"steps":[{"op":"resize","params":{"width":100,"mode":"crop","gravity":"attention"}}]}`,
		`{"version":1,"steps":[{"op":"crop","params":{"rect":{"left":0,"top":0,"width":1,"height":1}}}]}`,
		`{"version":1,"steps":[{"op":"watermark","when":{"widthGt":10},"params":{"text":"x","opacity":0.3}}]}`,
		`{"version":1,"steps":[{"op":"convert","params":{"format":"webp","targetSizeKb":100}}],"metadata":{"keep":true}}`,
		`{"version":1,"steps":[{"op":"rotate","params":{"angle":90}},{"op":"flip","params":{"direction":"horizontal"}}]}`,
		`{"version":2}`, `[]`, `null`, `{"version":1,"steps":[{"op":"x"}]}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := Parse(data, Limits{MaxSteps: 8, MaxWatermarkBytes: 1024})
		if err != nil {
			var perr *Error
			if !errors.As(err, &perr) || len(perr.Problems) == 0 {
				t.Fatalf("error without problems: %v", err)
			}
			for _, pr := range perr.Problems {
				if pr.Pointer != "" && !strings.HasPrefix(pr.Pointer, "/") {
					t.Fatalf("pointer %q is not a JSON Pointer", pr.Pointer)
				}
			}
			return
		}
		if len(p.Steps) > 8 {
			t.Fatalf("%d steps accepted with MaxSteps 8", len(p.Steps))
		}
		for i, s := range p.Steps {
			switch {
			case s.Resize != nil:
				if s.Resize.Width > MaxDimension || s.Resize.Height > MaxDimension || s.Resize.Percent > MaxPercent {
					t.Fatalf("step %d: resize beyond the hard limits: %+v", i, *s.Resize)
				}
			case s.Watermark != nil:
				if len(s.Watermark.Image) > base64Len(1024) {
					t.Fatalf("step %d: watermark image above the limit", i)
				}
				if strings.ContainsAny(s.Watermark.Text, "\x00\n\r") {
					t.Fatalf("step %d: control characters in watermark text", i)
				}
			}
		}
		// Parsing is deterministic.
		if _, err2 := Parse(data, Limits{MaxSteps: 8, MaxWatermarkBytes: 1024}); err2 != nil {
			t.Fatalf("second parse failed: %v", err2)
		}
	})
}
