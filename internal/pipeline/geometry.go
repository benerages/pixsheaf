package pipeline

import "math"

// Size is an image size in pixels.
type Size struct {
	Width  int
	Height int
}

// Pixels returns Width x Height.
func (s Size) Pixels() int64 { return int64(s.Width) * int64(s.Height) }

// Box is a rectangle inside an image.
type Box struct {
	Left, Top, Width, Height int
}

// Facts are the properties conditions are evaluated against.
type Facts struct {
	Size
	// SourceFormat is the detected format of the uploaded file.
	SourceFormat string
}

// Matches reports whether all given conditions hold. A nil condition
// always matches.
func (c *Condition) Matches(f Facts) bool {
	if c == nil {
		return true
	}
	longest := max(f.Width, f.Height)
	switch {
	case c.WidthGt != nil && f.Width <= *c.WidthGt,
		c.WidthLt != nil && f.Width >= *c.WidthLt,
		c.HeightGt != nil && f.Height <= *c.HeightGt,
		c.HeightLt != nil && f.Height >= *c.HeightLt,
		c.LongestEdgeGt != nil && longest <= *c.LongestEdgeGt:
		return false
	}
	if c.SourceFormat != nil {
		found := false
		for _, s := range c.SourceFormat {
			if s == f.SourceFormat {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	switch c.Orientation {
	case "landscape":
		return f.Width > f.Height
	case "portrait":
		return f.Height > f.Width
	case "square":
		return f.Width == f.Height
	}
	return true
}

// ResizePlan says how to scale, and for mode "crop", what to cut from the
// scaled image.
type ResizePlan struct {
	HScale, VScale float64
	// Crop is the box to keep from the scaled image (mode crop), or nil.
	Crop *Box
	// Smart is "attention" or "entropy" when Crop should be chosen by
	// libvips' smart crop instead of the box position.
	Smart string
}

// Scaled returns the size of cur after scaling with p.
func (p ResizePlan) Scaled(cur Size) Size {
	return Size{
		Width:  max(1, int(math.Round(float64(cur.Width)*p.HScale))),
		Height: max(1, int(math.Round(float64(cur.Height)*p.VScale))),
	}
}

// Result returns the final size after scaling and cropping.
func (p ResizePlan) Result(cur Size) Size {
	if p.Crop != nil {
		return Size{p.Crop.Width, p.Crop.Height}
	}
	return p.Scaled(cur)
}

// Plan computes the scaling for an image of size cur.
func (r Resize) Plan(cur Size) ResizePlan {
	w, h := float64(cur.Width), float64(cur.Height)
	limit := func(s float64) float64 {
		if !r.Enlarge && s > 1 {
			return 1
		}
		return s
	}
	switch {
	case r.Percent > 0:
		s := limit(r.Percent / 100)
		return ResizePlan{HScale: s, VScale: s}
	case r.LongestEdge > 0:
		s := limit(float64(r.LongestEdge) / math.Max(w, h))
		return ResizePlan{HScale: s, VScale: s}
	case r.Width > 0 && r.Height == 0:
		s := limit(float64(r.Width) / w)
		return ResizePlan{HScale: s, VScale: s}
	case r.Height > 0 && r.Width == 0:
		s := limit(float64(r.Height) / h)
		return ResizePlan{HScale: s, VScale: s}
	}
	sw, sh := float64(r.Width)/w, float64(r.Height)/h
	switch r.Mode {
	case "fill":
		return ResizePlan{HScale: limit(sw), VScale: limit(sh)}
	case "crop":
		s := limit(math.Max(sw, sh))
		p := ResizePlan{HScale: s, VScale: s}
		scaled := p.Scaled(cur)
		cw, ch := min(r.Width, scaled.Width), min(r.Height, scaled.Height)
		if cw == scaled.Width && ch == scaled.Height {
			return p
		}
		box := place(scaled, cw, ch, r.Gravity)
		p.Crop = &box
		if r.Gravity == "attention" || r.Gravity == "entropy" {
			p.Smart = r.Gravity
		}
		return p
	default: // fit
		s := limit(math.Min(sw, sh))
		return ResizePlan{HScale: s, VScale: s}
	}
}

// Box computes the crop box for an image of size cur. ok is false when a
// rectangle lies entirely outside the image. smart is set for the gravities
// that libvips' smart crop chooses.
func (c Crop) Box(cur Size) (box Box, smart string, ok bool) {
	if r := c.Rect; r != nil {
		if r.Left >= cur.Width || r.Top >= cur.Height {
			return Box{}, "", false
		}
		return Box{
			Left:   r.Left,
			Top:    r.Top,
			Width:  min(r.Width, cur.Width-r.Left),
			Height: min(r.Height, cur.Height-r.Top),
		}, "", true
	}
	a := c.AspectRatio
	ratio := float64(a.Width) / float64(a.Height)
	w, h := cur.Width, cur.Height
	if float64(cur.Width)/float64(cur.Height) > ratio {
		w = max(1, int(math.Round(float64(cur.Height)*ratio)))
	} else {
		h = max(1, int(math.Round(float64(cur.Width)/ratio)))
	}
	if c.Gravity == "attention" || c.Gravity == "entropy" {
		smart = c.Gravity
	}
	return place(cur, w, h, c.Gravity), smart, true
}

// place positions a w x h box inside cur according to gravity.
func place(cur Size, w, h int, gravity string) Box {
	freeX, freeY := cur.Width-w, cur.Height-h
	x, y := freeX/2, freeY/2
	switch gravity {
	case "north", "northwest", "northeast":
		y = 0
	case "south", "southwest", "southeast":
		y = freeY
	}
	switch gravity {
	case "west", "northwest", "southwest":
		x = 0
	case "east", "northeast", "southeast":
		x = freeX
	}
	return Box{Left: x, Top: y, Width: w, Height: h}
}

// Place positions an overlay of size inner inside outer with a margin, for
// watermarks.
func Place(outer, inner Size, gravity string, margin int) Box {
	b := place(Size{outer.Width - 2*margin, outer.Height - 2*margin}, inner.Width, inner.Height, gravity)
	b.Left += margin
	b.Top += margin
	return b
}

// DefaultOutputFormat is the output format when no convert step applies:
// the source format, except that HEIC/HEIF (input only) becomes JPEG.
func DefaultOutputFormat(source string) string {
	if source == "heif" {
		return "jpeg"
	}
	return source
}
