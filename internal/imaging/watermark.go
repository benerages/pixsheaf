//go:build vips

package imaging

/*
#cgo pkg-config: vips
#include <vips/vips.h>

static int px_load_overlay(const void *buf, size_t len, VipsImage **out) {
	*out = vips_image_new_from_buffer(buf, len, "", "fail_on", VIPS_FAIL_ON_ERROR, NULL);
	return *out ? 0 : -1;
}

// px_from_nrgba wraps non-premultiplied 8-bit RGBA pixels (a copy is made).
static int px_from_nrgba(const void *pix, int width, int height, VipsImage **out) {
	VipsImage *t = vips_image_new_from_memory_copy(pix, (size_t) width * height * 4,
		width, height, 4, VIPS_FORMAT_UCHAR);
	if (!t)
		return -1;
	int r = vips_copy(t, out, "interpretation", VIPS_INTERPRETATION_sRGB, NULL);
	g_object_unref(t);
	return r;
}

// px_overlay_prepare converts an overlay to 8-bit sRGB with alpha, scales
// it to width (premultiplied, so edges do not get dark fringes), and
// multiplies the alpha band by opacity.
static int px_overlay_prepare(VipsImage *in, VipsImage **out, int width, double opacity) {
	VipsImage *t[7] = { NULL };
	int r = -1;
	if (vips_colourspace(in, &t[0], VIPS_INTERPRETATION_sRGB, NULL))
		goto done;
	if (vips_image_hasalpha(t[0])) {
		g_object_ref(t[0]);
		t[1] = t[0];
	} else if (vips_bandjoin_const1(t[0], &t[1], 255.0, NULL))
		goto done;
	if (vips_image_get_bands(t[1]) != 4) {
		vips_error("pixsheaf", "unexpected watermark bands");
		goto done;
	}
	if (vips_premultiply(t[1], &t[2], NULL))
		goto done;
	double scale = (double) width / vips_image_get_width(t[2]);
	if (vips_resize(t[2], &t[3], scale, NULL))
		goto done;
	if (vips_unpremultiply(t[3], &t[4], NULL))
		goto done;
	double mul[4] = { 1.0, 1.0, 1.0, opacity };
	double add[4] = { 0.0, 0.0, 0.0, 0.0 };
	if (vips_linear(t[4], &t[5], mul, add, 4, "uchar", TRUE, NULL))
		goto done;
	if (vips_copy(t[5], &t[6], "interpretation", VIPS_INTERPRETATION_sRGB, NULL))
		goto done;
	*out = t[6];
	t[6] = NULL;
	r = 0;
done:
	for (int i = 0; i < 7; i++)
		if (t[i])
			g_object_unref(t[i]);
	return r;
}

// px_composite places overlay over base at (x, y). A base without alpha
// stays without alpha.
static int px_composite(VipsImage *base, VipsImage *overlay, VipsImage **out, int x, int y) {
	VipsImage *t = NULL;
	int alpha = vips_image_hasalpha(base);
	if (vips_composite2(base, overlay, &t, VIPS_BLEND_MODE_OVER, "x", x, "y", y, NULL))
		return -1;
	if (alpha) {
		*out = t;
		return 0;
	}
	int r = vips_extract_band(t, out, 0, "n", vips_image_get_bands(t) - 1, NULL);
	g_object_unref(t);
	return r;
}
*/
import "C"

import (
	"encoding/base64"
	"fmt"
	"math"
	"unsafe"

	"github.com/benerages/pixsheaf/internal/pipeline"
)

// maxOverlayPixels bounds a decoded image watermark (4096 x 4096).
const maxOverlayPixels = 4096 * 4096

// overlayLoaders are the loaders allowed for image watermarks.
var overlayLoaders = map[string]bool{
	"VipsForeignLoadPngBuffer":  true,
	"VipsForeignLoadWebpBuffer": true,
}

// watermark draws w onto im. Size is a percentage of the image width,
// margin a percentage of the shorter side; the watermark is shrunk further
// if it would not fit inside the margins.
func watermark(ses *session, im *image, w pipeline.Watermark, cur pipeline.Size) error {
	margin := int(math.Round(float64(min(cur.Width, cur.Height)) * w.Margin / 100))
	availW, availH := cur.Width-2*margin, cur.Height-2*margin
	if availW < 1 || availH < 1 {
		return fmt.Errorf("%w: the image is too small for the margin", ErrInvalidInput)
	}
	targetW := min(max(int(math.Round(float64(cur.Width)*w.Size/100)), 1), availW)

	ov := &image{}
	defer ov.close()
	if w.Image != "" {
		if err := loadOverlay(ses, ov, w.Image); err != nil {
			return err
		}
	} else {
		c, err := ParseColor(w.Color, 1)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		img, err := RenderText(w.Text, c, targetW, availH)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		b := img.Bounds()
		if C.px_from_nrgba(unsafe.Pointer(&img.Pix[0]), C.int(b.Dx()), C.int(b.Dy()), &ov.p) != 0 {
			return fmt.Errorf("watermark text: %s", lastError())
		}
	}

	src := ov.size()
	scale := min(float64(targetW)/float64(src.Width), float64(availH)/float64(src.Height))
	finalW := max(int(float64(src.Width)*scale), 1)
	if err := ov.apply("prepare watermark", func(in *C.VipsImage, out **C.VipsImage) C.int {
		return C.px_overlay_prepare(in, out, C.int(finalW), C.double(w.Opacity))
	}); err != nil {
		return err
	}

	// Composite works in sRGB; convert CMYK first so that the embedded
	// CMYK profile does not stay attached to sRGB pixels.
	if im.isCMYK() {
		if err := im.toSRGB(); err != nil {
			return err
		}
	}
	box := pipeline.Place(cur, ov.size(), w.Position, margin)
	return im.apply("watermark", func(in *C.VipsImage, out **C.VipsImage) C.int {
		return C.px_composite(in, ov.p, out, C.int(box.Left), C.int(box.Top))
	})
}

// loadOverlay decodes a base64 PNG or WebP watermark into ov.
func loadOverlay(ses *session, ov *image, b64 string) error {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("%w: watermark image is not valid base64", ErrInvalidInput)
	}
	loader, err := FindLoader(data)
	if err != nil || !overlayLoaders[loader] {
		return fmt.Errorf("%w: watermark image must be PNG or WebP", ErrInvalidInput)
	}
	info, err := Inspect(data)
	if err != nil {
		return fmt.Errorf("%w: watermark image: %v", ErrInvalidInput, err)
	}
	if int64(info.Width)*int64(info.Height) > maxOverlayPixels {
		return fmt.Errorf("%w: watermark image has %dx%d pixels, more than %d", ErrInvalidInput, info.Width, info.Height, maxOverlayPixels)
	}
	// libvips reads lazily from the buffer until the output is encoded.
	buf := ses.cbytes(data)
	if C.px_load_overlay(buf, C.size_t(len(data)), &ov.p) != 0 {
		return fmt.Errorf("%w: watermark image: %s", ErrInvalidInput, lastError())
	}
	return nil
}
