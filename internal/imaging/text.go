package imaging

import (
	"encoding/hex"
	"errors"
	"fmt"
	goimage "image"
	"image/color"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Text watermarks are rendered in Go with the bundled Go Bold font (BSD
// license, part of golang.org/x/image) instead of libvips' pango-based
// text operation, which is not compiled in (ADR 0020). The font data is
// compiled into the binary; nothing is read from disk.
var (
	fontOnce sync.Once
	textFont *opentype.Font
	errFont  error
)

// Font size bounds in points (at 72 DPI, points equal pixels), and the
// largest text image rendered in Go. Larger watermarks are scaled up by
// libvips, which keeps the Go heap small (4 bytes per pixel).
const (
	minFontSize     = 4
	maxFontSize     = 2000
	maxRenderPixels = 4 << 20
)

// RenderText draws text as a transparent image that fits in maxWidth x
// maxHeight pixels (and in maxRenderPixels), in color c (alpha included).
// The text is plain: no markup is interpreted.
func RenderText(text string, c color.NRGBA, maxWidth, maxHeight int) (*goimage.NRGBA, error) {
	if text == "" || maxWidth < 1 || maxHeight < 1 {
		return nil, errors.New("imaging: empty text or size")
	}
	fontOnce.Do(func() { textFont, errFont = opentype.Parse(gobold.TTF) })
	if errFont != nil {
		return nil, fmt.Errorf("imaging: font: %w", errFont)
	}

	// Measure at a reference size, then scale linearly to the wanted width.
	const reference = 100.0
	advance, height, err := measure(text, reference)
	if err != nil {
		return nil, err
	}
	size := min(
		reference*float64(maxWidth)/advance,
		reference*float64(maxHeight)/height,
		reference*math.Sqrt(maxRenderPixels/(advance*height)),
		maxFontSize,
	)
	size = max(size, minFontSize)

	face, err := opentype.NewFace(textFont, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, fmt.Errorf("imaging: font face: %w", err)
	}
	defer func() { _ = face.Close() }()
	m := face.Metrics()
	w := font.MeasureString(face, text).Ceil()
	h := (m.Ascent + m.Descent).Ceil()
	if w < 1 || h < 1 {
		return nil, errors.New("imaging: text has no visible size")
	}
	img := goimage.NewNRGBA(goimage.Rect(0, 0, w, h))
	d := font.Drawer{Dst: img, Src: goimage.NewUniform(c), Face: face, Dot: fixed.Point26_6{X: 0, Y: m.Ascent}}
	d.DrawString(text)
	return img, nil
}

// measure returns the advance width and line height of text at size.
func measure(text string, size float64) (advance, height float64, err error) {
	face, err := opentype.NewFace(textFont, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return 0, 0, fmt.Errorf("imaging: font face: %w", err)
	}
	defer func() { _ = face.Close() }()
	m := face.Metrics()
	advance = float64(font.MeasureString(face, text)) / 64
	height = float64(m.Ascent+m.Descent) / 64
	if advance <= 0 || height <= 0 {
		return 0, 0, errors.New("imaging: text has no visible size")
	}
	return advance, height, nil
}

// ParseColor converts "#RRGGBB" and an opacity in [0, 1] to a color.
func ParseColor(hexColor string, opacity float64) (color.NRGBA, error) {
	if len(hexColor) != 7 || hexColor[0] != '#' {
		return color.NRGBA{}, fmt.Errorf("imaging: invalid color %q", hexColor)
	}
	rgb, err := hex.DecodeString(hexColor[1:])
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("imaging: invalid color %q", hexColor)
	}
	opacity = min(max(opacity, 0), 1)
	return color.NRGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: uint8(opacity*255 + 0.5)}, nil
}
