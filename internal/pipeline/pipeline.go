// Package pipeline parses and validates processing pipelines (ADR 0009,
// ADR 0019) and computes their geometry. It has no libvips dependency, so
// the API can validate requests and the worker can plan operations with
// the same code.
//
// Parsing is strict: every object is decoded with unknown fields rejected,
// which makes the accepted documents the closed schema of
// api/openapi.yaml, and the rules that the schema states only in prose
// (mutually exclusive parameters, operator limits) are enforced here.
package pipeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Version is the pipeline format version this package implements.
const Version = 1

// Hard limits from api/openapi.yaml. Operator limits may be lower.
const (
	MaxSteps        = 32
	MaxDimension    = 65535
	MaxPercent      = 1000
	MaxTextLength   = 200
	MaxWatermarkB64 = 1398104
)

// Pipeline is a parsed and validated pipeline.
type Pipeline struct {
	Steps    []Step
	Metadata Metadata
	// ColorSpace is "srgb" (default) or "keep".
	ColorSpace string
}

// document is the JSON form of a pipeline.
type document struct {
	Version    int       `json:"version"`
	Steps      []rawStep `json:"steps"`
	Metadata   Metadata  `json:"metadata"`
	ColorSpace string    `json:"colorSpace"`
}

// Metadata is the metadata policy.
type Metadata struct {
	Keep         bool `json:"keep"`
	KeepLocation bool `json:"keepLocation"`
}

// Op names, as in api/openapi.yaml.
const (
	OpAutoOrient = "autoOrient"
	OpResize     = "resize"
	OpCrop       = "crop"
	OpRotate     = "rotate"
	OpFlip       = "flip"
	OpWatermark  = "watermark"
	OpConvert    = "convert"
)

// Step is one validated step. Exactly the field matching Op is set.
type Step struct {
	Op        string
	When      *Condition
	Resize    *Resize
	Crop      *Crop
	Rotate    int    // 90, 180, or 270
	Flip      string // horizontal or vertical
	Watermark *Watermark
	Convert   *Convert
}

// Condition restricts a step to matching images. Nil fields are not
// checked.
type Condition struct {
	WidthGt       *int     `json:"widthGt"`
	WidthLt       *int     `json:"widthLt"`
	HeightGt      *int     `json:"heightGt"`
	HeightLt      *int     `json:"heightLt"`
	LongestEdgeGt *int     `json:"longestEdgeGt"`
	SourceFormat  []string `json:"sourceFormat"`
	Orientation   string   `json:"orientation"`
}

// Resize parameters; exactly one of Width/Height (one or both), LongestEdge,
// or Percent is set.
type Resize struct {
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	LongestEdge int     `json:"longestEdge"`
	Percent     float64 `json:"percent"`
	Mode        string  `json:"mode"`
	Gravity     string  `json:"gravity"`
	Enlarge     bool    `json:"enlarge"`
}

// Crop parameters; either AspectRatio (with Gravity) or Rect.
type Crop struct {
	AspectRatio *AspectRatio `json:"aspectRatio"`
	Gravity     string       `json:"gravity"`
	Rect        *Rect        `json:"rect"`
}

// AspectRatio such as 16:9.
type AspectRatio struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Rect in pixels.
type Rect struct {
	Left   int `json:"left"`
	Top    int `json:"top"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Watermark parameters; exactly one of Text and Image is set.
type Watermark struct {
	Text     string  `json:"text"`
	Color    string  `json:"color"`
	Image    string  `json:"image"`
	Size     float64 `json:"size"`
	Position string  `json:"position"`
	Margin   float64 `json:"margin"`
	Opacity  float64 `json:"opacity"`
	set      map[string]bool
}

// Convert sets the output format and encoder options.
type Convert struct {
	Format       string `json:"format"`
	Quality      int    `json:"quality"`
	Lossless     bool   `json:"lossless"`
	Optimize     *bool  `json:"optimize"`
	TargetSizeKb int    `json:"targetSizeKb"`
	Progressive  bool   `json:"progressive"`
}

// OptimizeOrDefault reports whether to spend more effort (default true).
func (c Convert) OptimizeOrDefault() bool { return c.Optimize == nil || *c.Optimize }

// rawStep holds a step until its op is known.
type rawStep struct {
	Op     string          `json:"op"`
	When   json.RawMessage `json:"when"`
	Params json.RawMessage `json:"params"`
}

// Limits are the operator limits (GET /capabilities). Zero means the hard
// limit applies.
type Limits struct {
	MaxSteps          int
	MaxWatermarkBytes int
}

// Error lists every problem found, each with a JSON Pointer.
type Error struct {
	Problems []Problem
}

// Problem is one invalid value.
type Problem struct {
	Pointer string
	Message string
}

func (e *Error) Error() string {
	parts := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		parts = append(parts, p.Pointer+": "+p.Message)
	}
	return "pipeline: " + strings.Join(parts, "; ")
}

func (e *Error) add(pointer, format string, args ...any) {
	e.Problems = append(e.Problems, Problem{Pointer: pointer, Message: fmt.Sprintf(format, args...)})
}

var (
	outputFormats = set("jpeg", "png", "webp", "gif", "tiff", "bmp", "avif", "jxl", "pdf")
	inputFormats  = set("jpeg", "png", "webp", "gif", "tiff", "bmp", "avif", "heif", "jxl")
	gravities     = set("center", "north", "northeast", "east", "southeast", "south", "southwest", "west", "northwest", "attention", "entropy")
)

func set(values ...string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

// strictUnmarshal decodes exactly one JSON value into v, rejecting unknown
// fields and trailing data.
func strictUnmarshal(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}

// Parse decodes and validates a pipeline document. On invalid input it
// returns an *Error listing all problems found.
func Parse(data []byte, lim Limits) (Pipeline, error) {
	var doc document
	e := &Error{}
	if err := strictUnmarshal(data, &doc); err != nil {
		e.add("", "invalid pipeline document: %v", err)
		return Pipeline{}, e
	}
	if doc.Version != Version {
		e.add("/version", "must be %d", Version)
	}
	maxSteps := MaxSteps
	if lim.MaxSteps > 0 && lim.MaxSteps < maxSteps {
		maxSteps = lim.MaxSteps
	}
	if doc.Steps == nil {
		e.add("/steps", "is required")
	}
	if len(doc.Steps) > maxSteps {
		e.add("/steps", "has %d steps; the limit is %d", len(doc.Steps), maxSteps)
	}
	p := Pipeline{Metadata: doc.Metadata, ColorSpace: doc.ColorSpace}
	switch p.ColorSpace {
	case "":
		p.ColorSpace = "srgb"
	case "srgb", "keep":
	default:
		e.add("/colorSpace", "must be srgb or keep")
	}
	if p.Metadata.KeepLocation && !p.Metadata.Keep {
		e.add("/metadata/keepLocation", "requires keep: true")
	}
	for i, rs := range doc.Steps {
		if s, ok := parseStep(rs, fmt.Sprintf("/steps/%d", i), lim, e); ok {
			p.Steps = append(p.Steps, s)
		}
	}
	if len(e.Problems) > 0 {
		return Pipeline{}, e
	}
	return p, nil
}

func parseStep(rs rawStep, ptr string, lim Limits, e *Error) (Step, bool) {
	n := len(e.Problems)
	s := Step{Op: rs.Op}
	if len(rs.When) > 0 {
		var c Condition
		if err := strictUnmarshal(rs.When, &c); err != nil {
			e.add(ptr+"/when", "invalid: %v", err)
		} else {
			validateCondition(c, ptr+"/when", e)
			s.When = &c
		}
	}
	needParams := rs.Op != OpAutoOrient
	if needParams && len(rs.Params) == 0 {
		e.add(ptr+"/params", "is required for %s", rs.Op)
		return s, false
	}
	if !needParams && len(rs.Params) > 0 {
		e.add(ptr+"/params", "is not allowed for %s", rs.Op)
	}
	pp := ptr + "/params"
	switch rs.Op {
	case OpAutoOrient:
	case OpResize:
		var r Resize
		if decode(rs.Params, &r, pp, e) {
			validateResize(&r, pp, e)
			s.Resize = &r
		}
	case OpCrop:
		var c Crop
		if decode(rs.Params, &c, pp, e) {
			validateCrop(&c, pp, e)
			s.Crop = &c
		}
	case OpRotate:
		var r struct {
			Angle int `json:"angle"`
		}
		if decode(rs.Params, &r, pp, e) {
			if r.Angle != 90 && r.Angle != 180 && r.Angle != 270 {
				e.add(pp+"/angle", "must be 90, 180, or 270")
			}
			s.Rotate = r.Angle
		}
	case OpFlip:
		var f struct {
			Direction string `json:"direction"`
		}
		if decode(rs.Params, &f, pp, e) {
			if f.Direction != "horizontal" && f.Direction != "vertical" {
				e.add(pp+"/direction", "must be horizontal or vertical")
			}
			s.Flip = f.Direction
		}
	case OpWatermark:
		w := Watermark{}
		if decode(rs.Params, &w, pp, e) {
			validateWatermark(rs.Params, &w, pp, lim, e)
			s.Watermark = &w
		}
	case OpConvert:
		var c Convert
		if decode(rs.Params, &c, pp, e) {
			validateConvert(c, pp, e)
			s.Convert = &c
		}
	default:
		e.add(ptr+"/op", "unknown operation %q", rs.Op)
	}
	return s, len(e.Problems) == n
}

func decode(data json.RawMessage, v any, ptr string, e *Error) bool {
	if err := strictUnmarshal(data, v); err != nil {
		e.add(ptr, "invalid: %v", err)
		return false
	}
	return true
}

func inRange(e *Error, ptr string, v, lo, hi int) {
	if v < lo || v > hi {
		e.add(ptr, "must be between %d and %d", lo, hi)
	}
}

func validateCondition(c Condition, ptr string, e *Error) {
	empty := true
	check := func(name string, v *int, lo, hi int) {
		if v != nil {
			empty = false
			inRange(e, ptr+"/"+name, *v, lo, hi)
		}
	}
	check("widthGt", c.WidthGt, 0, MaxDimension)
	check("widthLt", c.WidthLt, 1, MaxDimension+1)
	check("heightGt", c.HeightGt, 0, MaxDimension)
	check("heightLt", c.HeightLt, 1, MaxDimension+1)
	check("longestEdgeGt", c.LongestEdgeGt, 0, MaxDimension)
	if c.SourceFormat != nil {
		empty = false
		if len(c.SourceFormat) == 0 {
			e.add(ptr+"/sourceFormat", "must not be empty")
		}
		seen := map[string]bool{}
		for i, f := range c.SourceFormat {
			if !inputFormats[f] {
				e.add(fmt.Sprintf("%s/sourceFormat/%d", ptr, i), "unknown input format %q", f)
			}
			if seen[f] {
				e.add(fmt.Sprintf("%s/sourceFormat/%d", ptr, i), "duplicate format %q", f)
			}
			seen[f] = true
		}
	}
	if c.Orientation != "" {
		empty = false
		if c.Orientation != "landscape" && c.Orientation != "portrait" && c.Orientation != "square" {
			e.add(ptr+"/orientation", "must be landscape, portrait, or square")
		}
	}
	if empty {
		e.add(ptr, "needs at least one condition")
	}
}

func validateResize(r *Resize, ptr string, e *Error) {
	byBox := r.Width > 0 || r.Height > 0
	ways := 0
	for _, used := range []bool{byBox, r.LongestEdge > 0, r.Percent > 0} {
		if used {
			ways++
		}
	}
	if ways != 1 {
		e.add(ptr, "give exactly one of width/height, longestEdge, or percent")
	}
	for name, v := range map[string]int{"width": r.Width, "height": r.Height, "longestEdge": r.LongestEdge} {
		if v < 0 || v > MaxDimension {
			e.add(ptr+"/"+name, "must be between 1 and %d", MaxDimension)
		}
	}
	if r.Percent < 0 || r.Percent > MaxPercent {
		e.add(ptr+"/percent", "must be greater than 0 and at most %d", MaxPercent)
	}
	switch r.Mode {
	case "":
		r.Mode = "fit"
	case "fit", "fill", "crop":
	default:
		e.add(ptr+"/mode", "must be fit, fill, or crop")
	}
	if r.Gravity == "" {
		r.Gravity = "center"
	} else if !gravities[r.Gravity] {
		e.add(ptr+"/gravity", "unknown gravity %q", r.Gravity)
	}
}

func validateCrop(c *Crop, ptr string, e *Error) {
	switch {
	case c.AspectRatio != nil && c.Rect != nil:
		e.add(ptr, "give either aspectRatio or rect, not both")
	case c.AspectRatio == nil && c.Rect == nil:
		e.add(ptr, "give aspectRatio or rect")
	case c.Rect != nil && c.Gravity != "":
		e.add(ptr+"/gravity", "applies only to aspectRatio")
	}
	if a := c.AspectRatio; a != nil {
		inRange(e, ptr+"/aspectRatio/width", a.Width, 1, 1000)
		inRange(e, ptr+"/aspectRatio/height", a.Height, 1, 1000)
	}
	if r := c.Rect; r != nil {
		inRange(e, ptr+"/rect/left", r.Left, 0, MaxDimension)
		inRange(e, ptr+"/rect/top", r.Top, 0, MaxDimension)
		inRange(e, ptr+"/rect/width", r.Width, 1, MaxDimension)
		inRange(e, ptr+"/rect/height", r.Height, 1, MaxDimension)
	}
	if c.Gravity == "" {
		c.Gravity = "center"
	} else if !gravities[c.Gravity] {
		e.add(ptr+"/gravity", "unknown gravity %q", c.Gravity)
	}
}

func validateWatermark(raw json.RawMessage, w *Watermark, ptr string, lim Limits, e *Error) {
	var present map[string]json.RawMessage
	_ = json.Unmarshal(raw, &present)
	w.set = map[string]bool{}
	for k := range present {
		w.set[k] = true
	}
	hasText, hasImage := w.set["text"], w.set["image"]
	switch {
	case hasText == hasImage:
		e.add(ptr, "give exactly one of text or image")
	case hasImage && w.set["color"]:
		e.add(ptr+"/color", "applies only to text")
	}
	if hasText && (len([]rune(w.Text)) < 1 || len([]rune(w.Text)) > MaxTextLength) {
		e.add(ptr+"/text", "must be 1 to %d characters", MaxTextLength)
	}
	if hasImage {
		maxB64 := MaxWatermarkB64
		if lim.MaxWatermarkBytes > 0 && base64Len(lim.MaxWatermarkBytes) < maxB64 {
			maxB64 = base64Len(lim.MaxWatermarkBytes)
		}
		if len(w.Image) == 0 || len(w.Image) > maxB64 {
			e.add(ptr+"/image", "must be base64 of at most %d bytes", maxB64/4*3)
		}
	}
	if !w.set["color"] {
		w.Color = "#FFFFFF"
	} else if !isHexColor(w.Color) {
		e.add(ptr+"/color", "must be #RRGGBB")
	}
	if !w.set["size"] {
		w.Size = 20
	} else if w.Size <= 0 || w.Size > 100 {
		e.add(ptr+"/size", "must be greater than 0 and at most 100")
	}
	if !w.set["margin"] {
		w.Margin = 2
	} else if w.Margin < 0 || w.Margin > 25 {
		e.add(ptr+"/margin", "must be between 0 and 25")
	}
	if !w.set["opacity"] {
		w.Opacity = 0.5
	} else if w.Opacity < 0 || w.Opacity > 1 {
		e.add(ptr+"/opacity", "must be between 0 and 1")
	}
	if w.Position == "" {
		w.Position = "southeast"
	} else if !gravities[w.Position] || w.Position == "attention" || w.Position == "entropy" {
		e.add(ptr+"/position", "must be center or a compass direction")
	}
}

func base64Len(n int) int { return (n + 2) / 3 * 4 }

func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

func validateConvert(c Convert, ptr string, e *Error) {
	if !outputFormats[c.Format] {
		e.add(ptr+"/format", "unknown output format %q", c.Format)
	}
	if c.Quality != 0 {
		inRange(e, ptr+"/quality", c.Quality, 1, 100)
	}
	// SVT-AV1, the only AV1 encoder in the worker, has no 4:4:4 mode, which
	// lossless AV1 requires.
	if c.Lossless && (c.Format == "jpeg" || c.Format == "avif" || c.Format == "pdf") {
		e.add(ptr+"/lossless", "is not available for %s", c.Format)
	}
	if c.TargetSizeKb != 0 {
		inRange(e, ptr+"/targetSizeKb", c.TargetSizeKb, 1, 1048576)
		if c.Quality != 0 || c.Lossless {
			e.add(ptr+"/targetSizeKb", "cannot be combined with quality or lossless")
		}
	}
}
