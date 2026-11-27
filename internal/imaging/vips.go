//go:build vips

package imaging

/*
#cgo pkg-config: vips
#include <stdlib.h>
#include <vips/vips.h>

// px_init starts libvips with the hardening from ADR 0008: no operation
// cache, one thread per job, untrusted operations blocked, and every
// loader and saver blocked until the allowlist re-enables them.
static int px_init(void) {
	if (VIPS_INIT("pixsheaf-worker") != 0)
		return -1;
	vips_cache_set_max(0);
	vips_concurrency_set(1);
	vips_block_untrusted_set(TRUE);
	vips_operation_block_set("VipsForeignLoad", TRUE);
	vips_operation_block_set("VipsForeignSave", TRUE);
	return 0;
}

static void px_allow(const char *name) {
	vips_operation_block_set(name, FALSE);
}

// px_header opens buf lazily and reads only the header. Sequential access
// and fail_on=error follow ADR 0008. loader receives the loader nickname
// and compression the HEIF compression ("hevc" or "av1"), if any.
static int px_header(const void *buf, size_t len, int *width, int *height, int *pages,
		char *loader, size_t loader_len, char *compression, size_t compression_len) {
	VipsImage *img = vips_image_new_from_buffer(buf, len, "",
		"access", VIPS_ACCESS_SEQUENTIAL,
		"fail_on", VIPS_FAIL_ON_ERROR,
		NULL);
	if (!img)
		return -1;
	*width = vips_image_get_width(img);
	*height = vips_image_get_height(img);
	*pages = vips_image_get_n_pages(img);
	const char *s = NULL;
	loader[0] = '\0';
	if (vips_image_get_typeof(img, "vips-loader") && !vips_image_get_string(img, "vips-loader", &s) && s)
		g_strlcpy(loader, s, loader_len);
	s = NULL;
	compression[0] = '\0';
	if (vips_image_get_typeof(img, "heif-compression") && !vips_image_get_string(img, "heif-compression", &s) && s)
		g_strlcpy(compression, s, compression_len);
	g_object_unref(img);
	return 0;
}

// px_save_rect encodes a width x height black RGB image with one EXIF
// field, using the saver that matches suffix. The caller frees *buf with
// g_free.
static int px_save_rect(const char *suffix, int width, int height, void **buf, size_t *len) {
	VipsImage *img = NULL;
	if (vips_black(&img, width, height, "bands", 3, NULL))
		return -1;
	vips_image_set_string(img, "exif-ifd0-ImageDescription", "pixsheaf");
	int r = vips_image_write_to_buffer(img, suffix, buf, len, NULL);
	g_object_unref(img);
	return r;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

// allowedLoaders and allowedSavers are the libvips classes re-enabled after
// all loaders and savers are blocked (ADR 0008). The loaders must match the
// API's magic-byte allowlist (ADR 0014).
var (
	allowedLoaders = []string{
		"VipsForeignLoadJpeg",
		"VipsForeignLoadPng",
		"VipsForeignLoadWebp",
		"VipsForeignLoadNsgif",
		"VipsForeignLoadTiff",
		"VipsForeignLoadHeif",
		"VipsForeignLoadJxl",
	}
	allowedSavers = []string{
		"VipsForeignSaveJpeg",
		"VipsForeignSavePng",
		"VipsForeignSaveWebp",
		"VipsForeignSaveCgif",
		"VipsForeignSaveTiff",
		"VipsForeignSaveHeif",
		"VipsForeignSaveJxl",
	}
)

// probeFormats lists one round trip per allowed output format, with the
// loader class that vips_foreign_find_load_buffer reports for it.
var probeFormats = []struct{ suffix, loader string }{
	{".jpg", "VipsForeignLoadJpegBuffer"},
	{".png", "VipsForeignLoadPngBuffer"},
	{".webp", "VipsForeignLoadWebpBuffer"},
	{".gif", "VipsForeignLoadNsgifBuffer"},
	{".tif", "VipsForeignLoadTiffBuffer"},
	{".avif", "VipsForeignLoadHeifBuffer"},
	{".jxl", "VipsForeignLoadJxlBuffer"},
}

var (
	initOnce sync.Once
	errInit  error
)

// Init starts libvips once per process and applies the hardening from
// ADR 0008. It is safe to call more than once.
func Init() error {
	initOnce.Do(func() {
		if C.px_init() != 0 {
			errInit = fmt.Errorf("imaging: libvips init failed: %s", lastError())
			return
		}
		for _, names := range [][]string{allowedLoaders, allowedSavers} {
			for _, name := range names {
				cs := C.CString(name)
				C.px_allow(cs)
				C.free(unsafe.Pointer(cs))
			}
		}
	})
	return errInit
}

// LibraryVersion returns the libvips version, such as "8.18.7".
func LibraryVersion() string {
	return C.GoString(C.vips_version_string())
}

// FindLoader returns the class name of the allowed libvips loader that
// would decode buf (such as "VipsForeignLoadJpegBuffer"), or an error if
// none accepts it.
func FindLoader(buf []byte) (string, error) {
	if err := Init(); err != nil {
		return "", err
	}
	if len(buf) == 0 {
		return "", errors.New("imaging: empty input")
	}
	name := C.vips_foreign_find_load_buffer(unsafe.Pointer(&buf[0]), C.size_t(len(buf)))
	if name == nil {
		C.vips_error_clear()
		return "", ErrUnsupported
	}
	return C.GoString(name), nil
}

// loaderFormats maps loader nickname prefixes to input format names.
var loaderFormats = []struct{ prefix, format string }{
	{"jpegload", "jpeg"},
	{"pngload", "png"},
	{"webpload", "webp"},
	{"gifload", "gif"},
	{"tiffload", "tiff"},
	{"heifload", "heif"},
	{"jxlload", "jxl"},
}

// Inspect reads only the header of buf: format, dimensions, and page
// count. No pixel data is decoded, so it is the place for the size checks
// against decompression bombs (ADR 0008).
func Inspect(buf []byte) (Info, error) {
	if isBMP(buf) {
		return inspectBMP(buf)
	}
	if _, err := FindLoader(buf); err != nil {
		return Info{}, err
	}
	var width, height, pages C.int
	var loader [64]C.char
	var compression [16]C.char
	if C.px_header(unsafe.Pointer(&buf[0]), C.size_t(len(buf)), &width, &height, &pages,
		&loader[0], C.size_t(len(loader)), &compression[0], C.size_t(len(compression))) != 0 {
		return Info{}, fmt.Errorf("%w: %s", ErrDecode, lastError())
	}
	info := Info{Width: int(width), Height: int(height), Pages: max(int(pages), 1)}
	nickname := C.GoString(&loader[0])
	for _, lf := range loaderFormats {
		if strings.HasPrefix(nickname, lf.prefix) {
			info.Format = lf.format
			break
		}
	}
	if info.Format == "" {
		return Info{}, fmt.Errorf("%w: unexpected loader %q", ErrUnsupported, nickname)
	}
	if info.Format == "heif" && C.GoString(&compression[0]) == "av1" {
		info.Format = "avif"
	}
	return info, nil
}

// Probe saves a blank image in every allowed output format and checks that
// the expected loader recognizes the result. The worker self-check uses it
// to prove that all codecs are present and allowed in the runtime image.
func Probe() []ProbeResult {
	if err := Init(); err != nil {
		return []ProbeResult{{Suffix: "libvips", Err: err}}
	}
	rs := make([]ProbeResult, 0, len(probeFormats))
	for _, f := range probeFormats {
		buf, err := saveBlank(f.suffix, 64)
		if err == nil {
			var got string
			got, err = FindLoader(buf)
			if err == nil && got != f.loader {
				err = fmt.Errorf("imaging: loader %q, want %q", got, f.loader)
			}
		}
		rs = append(rs, ProbeResult{Suffix: f.suffix, Loader: f.loader, Err: err})
	}
	return rs
}

// saveBlank encodes a blank size x size image with the saver for suffix.
func saveBlank(suffix string, size int) ([]byte, error) {
	return saveRect(suffix, size, size)
}

// saveRect encodes a blank width x height image with the saver for suffix.
func saveRect(suffix string, width, height int) ([]byte, error) {
	cs := C.CString(suffix)
	defer C.free(unsafe.Pointer(cs))
	var p unsafe.Pointer
	var n C.size_t
	if C.px_save_rect(cs, C.int(width), C.int(height), &p, &n) != 0 {
		return nil, fmt.Errorf("imaging: save %s: %s", suffix, lastError())
	}
	defer C.g_free(C.gpointer(p))
	return C.GoBytes(p, C.int(n)), nil
}

// lastError returns and clears the libvips error buffer.
func lastError() string {
	msg := C.GoString(C.vips_error_buffer())
	C.vips_error_clear()
	return msg
}
