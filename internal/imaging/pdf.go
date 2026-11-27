package imaging

import (
	"bytes"
	"fmt"
	"strconv"
)

// PDF output embeds one JPEG in a one-page PDF. The writer below produces
// a fixed, minimal document structure instead of using a PDF library
// (ADR 0021): there is nothing to parse, only bytes to write.

// A4 in points, the page margin, and the largest scale (96 DPI, so small
// images are not blown up to fill the page).
const (
	a4Short   = 595.28
	a4Long    = 841.89
	pdfMargin = 36.0
	pdfMaxDPI = 96.0
)

// wrapJPEG returns a PDF with jpeg (width x height pixels, gray if gray is
// true, else RGB) centered on an A4 page in the orientation of the image.
func wrapJPEG(jpeg []byte, width, height int, gray bool) []byte {
	pageW, pageH := a4Short, a4Long
	if width > height {
		pageW, pageH = a4Long, a4Short
	}
	w, h := float64(width), float64(height)
	scale := min((pageW-2*pdfMargin)/w, (pageH-2*pdfMargin)/h, 72/pdfMaxDPI)
	drawW, drawH := w*scale, h*scale
	x, y := (pageW-drawW)/2, (pageH-drawH)/2

	colorSpace := "/DeviceRGB"
	if gray {
		colorSpace = "/DeviceGray"
	}
	content := fmt.Sprintf("q %s 0 0 %s %s %s cm /Im0 Do Q\n", num(drawW), num(drawH), num(x), num(y))

	var b bytes.Buffer
	var offsets []int
	obj := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	b.WriteString("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n")
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>", num(pageW), num(pageH)))
	obj(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n%s\nendstream",
		width, height, colorSpace, len(jpeg), jpeg))
	obj(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))

	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return b.Bytes()
}

func num(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}
