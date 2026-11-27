//go:build vips

package imaging

/*
#cgo pkg-config: vips
#include <vips/vips.h>

// px_from_memory wraps 8-bit pixels with 1 to 4 bands (a copy is made).
static int px_from_memory(const void *pix, int width, int height, int bands, VipsImage **out) {
	VipsImage *t = vips_image_new_from_memory_copy(pix, (size_t) width * height * bands,
		width, height, bands, VIPS_FORMAT_UCHAR);
	if (!t)
		return -1;
	int r = vips_copy(t, out, "interpretation",
		bands < 3 ? VIPS_INTERPRETATION_B_W : VIPS_INTERPRETATION_sRGB, NULL);
	g_object_unref(t);
	return r;
}

static int px_first_bands(VipsImage *in, VipsImage **out, int n) {
	return vips_extract_band(in, out, 0, "n", n, NULL);
}

// px_to_rgba8 renders the image as 8-bit sRGB with alpha (opaque if the
// image has none) into a buffer the caller frees with g_free.
static int px_to_rgba8(VipsImage *in, void **buf, size_t *len) {
	VipsImage *t[3] = { NULL, NULL, NULL };
	int r = -1;
	if (vips_colourspace(in, &t[0], VIPS_INTERPRETATION_sRGB, NULL))
		goto done;
	if (vips_image_hasalpha(t[0])) {
		g_object_ref(t[0]);
		t[1] = t[0];
	} else if (vips_bandjoin_const1(t[0], &t[1], 255.0, NULL))
		goto done;
	if (vips_cast(t[1], &t[2], VIPS_FORMAT_UCHAR, NULL))
		goto done;
	if (vips_image_get_bands(t[2]) != 4) {
		vips_error("pixsheaf", "unexpected band count");
		goto done;
	}
	*buf = vips_image_write_to_memory(t[2], len);
	r = *buf ? 0 : -1;
done:
	for (int i = 0; i < 3; i++)
		if (t[i])
			g_object_unref(t[i]);
	return r;
}
*/
import "C"

import (
	"fmt"
	goimage "image"
	"image/draw"
	"runtime"
	"unsafe"
)

// fromGo copies a Go image into libvips: 8-bit sRGB, with an alpha band
// only if the image is not opaque.
func fromGo(src goimage.Image) (*image, error) {
	b := src.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 {
		return nil, fmt.Errorf("%w: empty image", ErrDecode)
	}
	var pix []byte
	switch m := src.(type) {
	case *goimage.NRGBA:
		if m.Stride == 4*b.Dx() {
			pix = m.Pix[:4*b.Dx()*b.Dy()]
		}
	case *goimage.RGBA:
		// Premultiplied and non-premultiplied are the same when opaque.
		if m.Stride == 4*b.Dx() && m.Opaque() {
			pix = m.Pix[:4*b.Dx()*b.Dy()]
		}
	}
	if pix == nil {
		n := goimage.NewNRGBA(goimage.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(n, n.Bounds(), src, b.Min, draw.Src)
		pix = n.Pix
	}
	im := &image{}
	if C.px_from_memory(unsafe.Pointer(&pix[0]), C.int(b.Dx()), C.int(b.Dy()), 4, &im.p) != 0 {
		return nil, fmt.Errorf("%w: %s", ErrDecode, lastError())
	}
	runtime.KeepAlive(pix)
	if o, ok := src.(interface{ Opaque() bool }); ok && o.Opaque() {
		if err := im.apply("drop alpha", func(in *C.VipsImage, out **C.VipsImage) C.int {
			return C.px_first_bands(in, out, 3)
		}); err != nil {
			im.close()
			return nil, err
		}
	}
	return im, nil
}

// toNRGBA renders the image as 8-bit sRGB with alpha. The pixels live in C
// memory until free is called; the image must not be used afterwards.
func toNRGBA(im *image) (img *goimage.NRGBA, free func(), err error) {
	var p unsafe.Pointer
	var n C.size_t
	if C.px_to_rgba8(im.p, &p, &n) != 0 {
		return nil, nil, encodeError(lastError())
	}
	s := im.size()
	if int(n) != 4*s.Width*s.Height {
		C.g_free(C.gpointer(p))
		return nil, nil, fmt.Errorf("%w: unexpected buffer size", ErrEncode)
	}
	img = &goimage.NRGBA{
		Pix:    unsafe.Slice((*byte)(p), int(n)),
		Stride: 4 * s.Width,
		Rect:   goimage.Rect(0, 0, s.Width, s.Height),
	}
	return img, func() { C.g_free(C.gpointer(p)) }, nil
}
