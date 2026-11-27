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

// px_save_blank encodes a size x size black RGB image with the saver that
// matches suffix. The caller frees *buf with g_free.
static int px_save_blank(const char *suffix, int size, void **buf, size_t *len) {
	VipsImage *img = NULL;
	if (vips_black(&img, size, size, "bands", 3, NULL))
		return -1;
	int r = vips_image_write_to_buffer(img, suffix, buf, len, NULL);
	g_object_unref(img);
	return r;
}
*/
import "C"

import (
	"errors"
	"fmt"
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
		return "", errors.New("imaging: no allowed loader accepts the input")
	}
	return C.GoString(name), nil
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
	cs := C.CString(suffix)
	defer C.free(unsafe.Pointer(cs))
	var p unsafe.Pointer
	var n C.size_t
	if C.px_save_blank(cs, C.int(size), &p, &n) != 0 {
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
