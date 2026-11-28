package imaging

import "strings"

// memoryMarkers are lower-case fragments of the messages that libvips, GLib,
// the codec libraries, and the Go runtime print when an allocation fails,
// for example because the job process reached RLIMIT_DATA.
var memoryMarkers = []string{
	"out of memory",            // libvips vips_tracked_malloc, Go runtime, libspng, libheif
	"failed to allocate",       // GLib g_malloc, which then aborts the process
	"cannot allocate memory",   // strerror(ENOMEM)
	"insufficient memory",      // libjpeg-turbo JERR_OUT_OF_MEMORY
	"memory error",             // libwebp picture import in libvips webpsave
	"unable to allocate",       // several libvips operations
	"memory allocation failed", // libtiff, libjxl
}

// IsMemoryMessage reports whether msg says that an allocation failed.
func IsMemoryMessage(msg string) bool {
	msg = strings.ToLower(msg)
	for _, m := range memoryMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// baseJobBytes is the memory a job process needs before it touches pixels:
// the Go runtime, libvips, the codec libraries, and the thread stacks. Measured
// with libvips and a cgo job process like ours: about 110 MB.
const baseJobBytes = 112 << 20

// encoderBytesPerPixel approximates the memory per output pixel that a saver
// needs on top of the decoded image, which libvips streams at about one byte
// per input pixel. JPEG, PNG, TIFF, and WebP are measured (36.5 MP photo, one
// libvips thread); the others are estimates from the encoders' frame buffers.
var encoderBytesPerPixel = map[string]float64{
	"jpeg":          1,
	"pdf":           1, // a JPEG inside the PDF
	"png":           2.5,
	"tiff":          4.5,
	"webp":          5.5,
	"webp-lossless": 24,
	"bmp":           7, // Go NRGBA copy plus the encoded file
	"gif":           6,
	"avif":          12,
	"heif":          12,
	"jxl":           20,
}

// EstimateBytes estimates the peak memory of a job that decodes in pixels and
// encodes out pixels as format. It is a rough figure, used to tell a job that
// ran out of memory from one whose encoder failed for another reason.
func EstimateBytes(in, out int64, format string, lossless bool) int64 {
	key := format
	if lossless && format == "webp" {
		key = "webp-lossless"
	}
	bpp, ok := encoderBytesPerPixel[key]
	if !ok {
		bpp = 12
	}
	return baseJobBytes + in + int64(bpp*float64(out))
}
