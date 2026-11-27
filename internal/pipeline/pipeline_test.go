package pipeline

import (
	"errors"
	"strings"
	"testing"
)

func mustParse(t *testing.T, doc string) Pipeline {
	t.Helper()
	p, err := Parse([]byte(doc), Limits{})
	if err != nil {
		t.Fatalf("Parse(%s): %v", doc, err)
	}
	return p
}

func TestParseFullPipeline(t *testing.T) {
	p := mustParse(t, `{"version":1,"steps":[
		{"op":"autoOrient"},
		{"op":"resize","params":{"longestEdge":1600},"when":{"longestEdgeGt":1600}},
		{"op":"crop","params":{"aspectRatio":{"width":16,"height":9},"gravity":"attention"}},
		{"op":"rotate","params":{"angle":90}},
		{"op":"flip","params":{"direction":"horizontal"}},
		{"op":"watermark","params":{"text":"© Pixsheaf","opacity":0.3}},
		{"op":"convert","params":{"format":"webp","quality":80},"when":{"sourceFormat":["heif"]}}
	],"metadata":{"keep":true},"colorSpace":"keep"}`)
	if len(p.Steps) != 7 || p.ColorSpace != "keep" || !p.Metadata.Keep {
		t.Fatalf("unexpected pipeline: %+v", p)
	}
	if r := p.Steps[1].Resize; r == nil || r.LongestEdge != 1600 || r.Mode != "fit" || r.Gravity != "center" {
		t.Errorf("resize defaults: %+v", r)
	}
	if w := p.Steps[5].Watermark; w == nil || w.Color != "#FFFFFF" || w.Size != 20 || w.Position != "southeast" || w.Opacity != 0.3 {
		t.Errorf("watermark defaults: %+v", w)
	}
	if c := p.Steps[6].Convert; c == nil || !c.OptimizeOrDefault() || p.Steps[6].When.SourceFormat[0] != "heif" {
		t.Errorf("convert: %+v", c)
	}
}

func TestParseDefaults(t *testing.T) {
	p := mustParse(t, `{"version":1,"steps":[]}`)
	if p.ColorSpace != "srgb" || p.Metadata.Keep {
		t.Errorf("defaults: %+v", p)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"unknown top-level field": `{"version":1,"steps":[],"extra":1}`,
		"wrong version":           `{"version":2,"steps":[]}`,
		"missing steps":           `{"version":1}`,
		"unknown op":              `{"version":1,"steps":[{"op":"sharpen","params":{}}]}`,
		"unknown param":           `{"version":1,"steps":[{"op":"resize","params":{"width":10,"x":1}}]}`,
		"width and percent":       `{"version":1,"steps":[{"op":"resize","params":{"width":10,"percent":50}}]}`,
		"no size":                 `{"version":1,"steps":[{"op":"resize","params":{"mode":"fit"}}]}`,
		"too large":               `{"version":1,"steps":[{"op":"resize","params":{"width":70000}}]}`,
		"bad gravity":             `{"version":1,"steps":[{"op":"resize","params":{"width":10,"height":10,"mode":"crop","gravity":"up"}}]}`,
		"crop both":               `{"version":1,"steps":[{"op":"crop","params":{"aspectRatio":{"width":1,"height":1},"rect":{"left":0,"top":0,"width":1,"height":1}}}]}`,
		"crop rect with gravity":  `{"version":1,"steps":[{"op":"crop","params":{"rect":{"left":0,"top":0,"width":1,"height":1},"gravity":"north"}}]}`,
		"rotate 45":               `{"version":1,"steps":[{"op":"rotate","params":{"angle":45}}]}`,
		"flip diagonal":           `{"version":1,"steps":[{"op":"flip","params":{"direction":"diagonal"}}]}`,
		"watermark both":          `{"version":1,"steps":[{"op":"watermark","params":{"text":"a","image":"AAAA"}}]}`,
		"watermark color on img":  `{"version":1,"steps":[{"op":"watermark","params":{"image":"AAAA","color":"#000000"}}]}`,
		"watermark bad color":     `{"version":1,"steps":[{"op":"watermark","params":{"text":"a","color":"red"}}]}`,
		"convert bmp-ish":         `{"version":1,"steps":[{"op":"convert","params":{"format":"psd"}}]}`,
		"target with quality":     `{"version":1,"steps":[{"op":"convert","params":{"format":"jpeg","quality":80,"targetSizeKb":100}}]}`,
		"lossless avif":           `{"version":1,"steps":[{"op":"convert","params":{"format":"avif","lossless":true}}]}`,
		"lossless jpeg":           `{"version":1,"steps":[{"op":"convert","params":{"format":"jpeg","lossless":true}}]}`,
		"empty condition":         `{"version":1,"steps":[{"op":"autoOrient","when":{}}]}`,
		"bad source format":       `{"version":1,"steps":[{"op":"autoOrient","when":{"sourceFormat":["svg"]}}]}`,
		"params on autoOrient":    `{"version":1,"steps":[{"op":"autoOrient","params":{}}]}`,
		"location without keep":   `{"version":1,"steps":[],"metadata":{"keepLocation":true}}`,
		"trailing data":           `{"version":1,"steps":[]} {}`,
		"not JSON":                `version=1`,
	}
	for name, doc := range cases {
		_, err := Parse([]byte(doc), Limits{})
		var pe *Error
		if !errors.As(err, &pe) || len(pe.Problems) == 0 {
			t.Errorf("%s: err = %v, want *Error with problems", name, err)
		}
	}
}

func TestParseOperatorLimits(t *testing.T) {
	doc := `{"version":1,"steps":[{"op":"autoOrient"},{"op":"autoOrient"},{"op":"autoOrient"}]}`
	if _, err := Parse([]byte(doc), Limits{MaxSteps: 2}); err == nil {
		t.Error("3 steps with MaxSteps 2 accepted")
	}
	img := `{"version":1,"steps":[{"op":"watermark","params":{"image":"` + strings.Repeat("A", 8) + `"}}]}`
	if _, err := Parse([]byte(img), Limits{MaxWatermarkBytes: 3}); err == nil {
		t.Error("6-byte watermark with MaxWatermarkBytes 3 accepted")
	}
	if _, err := Parse([]byte(img), Limits{MaxWatermarkBytes: 6}); err != nil {
		t.Errorf("6-byte watermark with MaxWatermarkBytes 6 rejected: %v", err)
	}
}

func TestErrorPointers(t *testing.T) {
	_, err := Parse([]byte(`{"version":1,"steps":[{"op":"autoOrient"},{"op":"rotate","params":{"angle":45}}]}`), Limits{})
	var pe *Error
	if !errors.As(err, &pe) || pe.Problems[0].Pointer != "/steps/1/params/angle" {
		t.Fatalf("err = %v, want a problem at /steps/1/params/angle", err)
	}
}
