//go:build vips

package imaging

/*
#cgo pkg-config: vips
#include <stdlib.h>
#include <string.h>
#include <vips/vips.h>

// PX_BMP and PX_PDF are encoded in Go (bmp.go, pdf.go), not by px_save.
enum { PX_JPEG, PX_PNG, PX_WEBP, PX_GIF, PX_TIFF, PX_AVIF, PX_JXL, PX_BMP, PX_PDF };

typedef struct {
	int format;
	int q;
	int lossless;
	int optimize;
	int progressive;
	int keep;
} px_save_opts;

// First page only; animation follows in a later step.
static int px_load(const void *buf, size_t len, VipsImage **out) {
	*out = vips_image_new_from_buffer(buf, len, "", "fail_on", VIPS_FAIL_ON_ERROR, NULL);
	return *out ? 0 : -1;
}

// All pages, as one tall image with page-height set (animations).
static int px_load_all(const void *buf, size_t len, VipsImage **out) {
	*out = vips_image_new_from_buffer(buf, len, "", "n", -1, "fail_on", VIPS_FAIL_ON_ERROR, NULL);
	return *out ? 0 : -1;
}

// px_join stacks equally sized frames into one animation. Frame delays and
// the loop count come from the first frame, which carries the metadata of
// the loaded animation.
static int px_join(VipsImage **frames, int n, VipsImage **out) {
	VipsImage *t = NULL;
	if (vips_arrayjoin(frames, &t, n, "across", 1, NULL))
		return -1;
	int r = vips_copy(t, out, NULL);
	g_object_unref(t);
	if (r)
		return r;
	vips_image_set_int(*out, VIPS_META_PAGE_HEIGHT, vips_image_get_height(frames[0]));
	vips_image_set_int(*out, VIPS_META_N_PAGES, n);
	return 0;
}

// px_decodes reports whether the input decodes completely (0) or not (-1):
// it loads it again and reads every pixel. Sequential access streams the
// pixels, so the check needs little memory.
static int px_decodes(const void *buf, size_t len, int all_pages) {
	VipsImage *img = all_pages
		? vips_image_new_from_buffer(buf, len, "", "n", -1, "fail_on", VIPS_FAIL_ON_ERROR,
			"access", VIPS_ACCESS_SEQUENTIAL, NULL)
		: vips_image_new_from_buffer(buf, len, "", "fail_on", VIPS_FAIL_ON_ERROR,
			"access", VIPS_ACCESS_SEQUENTIAL, NULL);
	if (!img)
		return -1;
	double avg;
	int r = vips_avg(img, &avg, NULL);
	g_object_unref(img);
	return r;
}

static int px_autorot(VipsImage *in, VipsImage **out) {
	return vips_autorot(in, out, NULL);
}

static int px_resize(VipsImage *in, VipsImage **out, double hscale, double vscale) {
	return vips_resize(in, out, hscale, "vscale", vscale, NULL);
}

static int px_extract(VipsImage *in, VipsImage **out, int left, int top, int width, int height) {
	return vips_extract_area(in, out, left, top, width, height, NULL);
}

static int px_smartcrop(VipsImage *in, VipsImage **out, int width, int height, int attention) {
	return vips_smartcrop(in, out, width, height, "interesting",
		attention ? VIPS_INTERESTING_ATTENTION : VIPS_INTERESTING_ENTROPY, NULL);
}

static int px_rot(VipsImage *in, VipsImage **out, int degrees) {
	VipsAngle a = degrees == 90 ? VIPS_ANGLE_D90 : degrees == 180 ? VIPS_ANGLE_D180 : VIPS_ANGLE_D270;
	return vips_rot(in, out, a, NULL);
}

static int px_flip(VipsImage *in, VipsImage **out, int vertical) {
	return vips_flip(in, out, vertical ? VIPS_DIRECTION_VERTICAL : VIPS_DIRECTION_HORIZONTAL, NULL);
}

// px_to_srgb converts with the embedded ICC profile when there is a usable
// one, and by interpretation otherwise. Grayscale stays grayscale.
static int px_to_srgb(VipsImage *in, VipsImage **out) {
	if (vips_image_get_typeof(in, VIPS_META_ICC_NAME)) {
		if (!vips_icc_transform(in, out, "srgb", "embedded", TRUE,
				"intent", VIPS_INTENT_PERCEPTUAL, NULL))
			return 0;
		vips_error_clear(); // broken profile: fall back to the interpretation
	}
	VipsInterpretation i = vips_image_guess_interpretation(in);
	if (i == VIPS_INTERPRETATION_sRGB || i == VIPS_INTERPRETATION_B_W) {
		g_object_ref(in);
		*out = in;
		return 0;
	}
	return vips_colourspace(in, out, VIPS_INTERPRETATION_sRGB, NULL);
}

// px_strip_location returns a copy without GPS EXIF fields, XMP, and IPTC.
static int px_strip_location(VipsImage *in, VipsImage **out) {
	if (vips_copy(in, out, NULL))
		return -1;
	gchar **fields = vips_image_get_fields(*out);
	for (int i = 0; fields[i]; i++) {
		if (g_str_has_prefix(fields[i], "exif-ifd3-") ||
				!strcmp(fields[i], VIPS_META_XMP_NAME) ||
				!strcmp(fields[i], VIPS_META_IPTC_NAME))
			vips_image_remove(*out, fields[i]);
	}
	g_strfreev(fields);
	return 0;
}

// px_flatten removes alpha by compositing over white.
static int px_flatten(VipsImage *in, VipsImage **out) {
	if (!vips_image_hasalpha(in)) {
		g_object_ref(in);
		*out = in;
		return 0;
	}
	double white = vips_image_get_format(in) == VIPS_FORMAT_USHORT ? 65535.0 : 255.0;
	VipsArrayDouble *bg = vips_array_double_newv(1, white);
	int r = vips_flatten(in, out, "background", bg, NULL);
	vips_area_unref(VIPS_AREA(bg));
	return r;
}

// px_copy_memory decodes the image once into memory, so that repeated
// encodes (target size search) do not decode the input again.
static int px_copy_memory(VipsImage *in, VipsImage **out) {
	*out = vips_image_copy_memory(in);
	return *out ? 0 : -1;
}

static int px_save(VipsImage *in, px_save_opts o, void **buf, size_t *len) {
	switch (o.format) {
	case PX_JPEG:
		return vips_jpegsave_buffer(in, buf, len, "Q", o.q, "optimize_coding", o.optimize,
			"interlace", o.progressive, "keep", o.keep, NULL);
	case PX_PNG:
		if (o.q > 0)
			return vips_pngsave_buffer(in, buf, len, "compression", o.optimize ? 9 : 6,
				"interlace", o.progressive, "palette", TRUE, "Q", o.q, "keep", o.keep, NULL);
		return vips_pngsave_buffer(in, buf, len, "compression", o.optimize ? 9 : 6,
			"interlace", o.progressive, "keep", o.keep, NULL);
	case PX_WEBP:
		return vips_webpsave_buffer(in, buf, len, "Q", o.q, "lossless", o.lossless,
			"effort", o.optimize ? 6 : 4, "keep", o.keep, NULL);
	case PX_GIF:
		return vips_gifsave_buffer(in, buf, len, "effort", o.optimize ? 10 : 7,
			"interlace", o.progressive, "keep", o.keep, NULL);
	case PX_TIFF:
		return vips_tiffsave_buffer(in, buf, len, "compression", VIPS_FOREIGN_TIFF_COMPRESSION_DEFLATE,
			"keep", o.keep, NULL);
	case PX_AVIF:
		return vips_heifsave_buffer(in, buf, len, "compression", VIPS_FOREIGN_HEIF_COMPRESSION_AV1,
			"Q", o.q, "lossless", o.lossless, "effort", o.optimize ? 6 : 4, "keep", o.keep, NULL);
	case PX_JXL:
		return vips_jxlsave_buffer(in, buf, len, "Q", o.q, "lossless", o.lossless,
			"effort", o.optimize ? 8 : 7, "keep", o.keep, NULL);
	}
	vips_error("pixsheaf", "unsupported output format");
	return -1;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime/debug"
	"unsafe"

	"github.com/benerages/pixsheaf/internal/pipeline"
)

// saveFormats maps output formats to the C saver and its default quality.
var saveFormats = map[string]struct {
	id             C.int
	defaultQuality int
	alpha          bool
}{
	"jpeg": {C.PX_JPEG, 82, false},
	"png":  {C.PX_PNG, 0, true},
	"webp": {C.PX_WEBP, 80, true},
	"gif":  {C.PX_GIF, 0, true},
	"tiff": {C.PX_TIFF, 0, true},
	"avif": {C.PX_AVIF, 55, true},
	"jxl":  {C.PX_JXL, 80, true},
	"bmp":  {C.PX_BMP, 0, false},
	"pdf":  {C.PX_PDF, 82, false},
}

// image owns one VipsImage reference.
type image struct {
	p *C.VipsImage
}

func (im *image) close() {
	if im.p != nil {
		C.g_object_unref(C.gpointer(im.p))
		im.p = nil
	}
}

func (im *image) size() pipeline.Size {
	return pipeline.Size{Width: int(C.vips_image_get_width(im.p)), Height: int(C.vips_image_get_height(im.p))}
}

// apply runs op on the current image and replaces it with the result.
func (im *image) apply(name string, op func(in *C.VipsImage, out **C.VipsImage) C.int) error {
	var out *C.VipsImage
	if op(im.p, &out) != 0 {
		return vipsError(nil, name)
	}
	im.close()
	im.p = out
	return nil
}

// toSRGB converts the image to sRGB (see px_to_srgb).
func (im *image) toSRGB() error {
	return im.apply("to sRGB", func(in *C.VipsImage, out **C.VipsImage) C.int { return C.px_to_srgb(in, out) })
}

// isCMYK reports whether the image is in CMYK.
func (im *image) isCMYK() bool {
	return C.vips_image_guess_interpretation(im.p) == C.VIPS_INTERPRETATION_CMYK
}

// session owns the C buffers libvips reads lazily from until the output is
// encoded, and per-step state shared by all frames of an animation.
type session struct {
	bufs []unsafe.Pointer
	// animated disables content-aware cropping, which would pick a
	// different area in every frame.
	animated bool
	// step is the index of the running step; overlays caches the prepared
	// watermark of each step.
	step     int
	overlays map[int]*image
}

// cbytes returns a C copy of b that lives until free.
func (s *session) cbytes(b []byte) unsafe.Pointer {
	p := C.CBytes(b)
	s.bufs = append(s.bufs, p)
	return p
}

func (s *session) free() {
	for _, ov := range s.overlays {
		ov.close()
	}
	s.overlays = nil
	for _, p := range s.bufs {
		C.free(p)
	}
	s.bufs = nil
}

// Process decodes input, runs the pipeline, and encodes the result. The
// header is checked against lim before any pixel data is decoded.
func Process(input []byte, p pipeline.Pipeline, lim ProcessLimits) (Output, error) {
	info, err := Inspect(input)
	if err != nil {
		return Output{}, err
	}
	if lim.MaxInputPixels > 0 && int64(info.Width)*int64(info.Height)*int64(info.Pages) > lim.MaxInputPixels {
		return Output{}, fmt.Errorf("%w: input has %dx%d pixels in %d pages", ErrTooManyPixels, info.Width, info.Height, info.Pages)
	}

	// GIF and WebP animations are processed frame by frame; for every other
	// multi-page input (TIFF, HEIF) only the first page is used.
	animated := info.Pages > 1 && (info.Format == "gif" || info.Format == "webp")
	maxFrames := MaxFrames
	if lim.MaxFrames > 0 && lim.MaxFrames < maxFrames {
		maxFrames = lim.MaxFrames
	}
	if animated && info.Pages > maxFrames {
		return Output{}, fmt.Errorf("%w: %d frames, the limit is %d", ErrTooManyFrames, info.Pages, maxFrames)
	}

	s := &session{animated: animated}
	defer s.free()
	im := &image{}
	if info.Format == "bmp" {
		if im, err = loadBMP(input, info); err != nil {
			return Output{}, err
		}
		// Return the decoded Go copy to the OS before libvips allocates.
		debug.FreeOSMemory()
	} else {
		// libvips decodes lazily and keeps reading the buffer until the
		// output is encoded, so it gets a C copy instead of a Go pointer.
		buf, n := s.cbytes(input), C.size_t(len(input))
		var rc C.int
		if animated {
			rc = C.px_load_all(buf, n, &im.p)
		} else {
			rc = C.px_load(buf, n, &im.p)
		}
		if rc != 0 {
			return Output{}, vipsError(ErrDecode, "load")
		}
	}
	frames := []*image{im}
	if animated {
		frames, err = splitFrames(im, maxFrames)
		im.close()
		if err != nil {
			return Output{}, err
		}
	}
	defer func() {
		for _, f := range frames {
			f.close()
		}
	}()

	format := pipeline.DefaultOutputFormat(info.Format)
	var conv pipeline.Convert
	for i, step := range p.Steps {
		cur := frames[0].size()
		if !step.When.Matches(pipeline.Facts{Size: cur, SourceFormat: info.Format}) {
			continue
		}
		s.step = i
		for _, f := range frames {
			if err := runStep(s, f, step, cur, lim); err != nil {
				return Output{}, fmt.Errorf("step %d (%s): %w", i, step.Op, err)
			}
		}
		if step.Convert != nil {
			conv = *step.Convert
			format = conv.Format
		}
	}

	// Only GIF and WebP output keep the animation; other formats get the
	// first frame (the other frames are never decoded).
	frameSize, pages := frames[0].size(), 1
	im = frames[0]
	if len(frames) > 1 && (format == "gif" || format == "webp") {
		joined, err := joinFrames(frames)
		if err != nil {
			return Output{}, err
		}
		defer joined.close()
		im, pages = joined, len(frames)
	}

	saver, ok := saveFormats[format]
	if !ok {
		return Output{}, fmt.Errorf("%w: output format %s", ErrNotImplemented, format)
	}
	// PDF embeds the image as DeviceRGB or DeviceGray, so it is always sRGB.
	if p.ColorSpace == "srgb" || format == "pdf" {
		if err := im.toSRGB(); err != nil {
			return Output{}, asDecode(err)
		}
	}
	keep := C.int(C.VIPS_FOREIGN_KEEP_NONE)
	switch {
	case p.Metadata.Keep:
		keep = C.VIPS_FOREIGN_KEEP_ALL
		if !p.Metadata.KeepLocation {
			if err := im.apply("strip location", func(in *C.VipsImage, out **C.VipsImage) C.int { return C.px_strip_location(in, out) }); err != nil {
				return Output{}, err
			}
		}
	case p.ColorSpace == "keep":
		// Without its profile the unconverted image would show wrong colors.
		keep = C.VIPS_FOREIGN_KEEP_ICC
	}
	if !saver.alpha {
		if err := im.apply("flatten", func(in *C.VipsImage, out **C.VipsImage) C.int { return C.px_flatten(in, out) }); err != nil {
			return Output{}, err
		}
	}

	final := im.size()
	if lim.MaxOutputPixels > 0 && final.Pixels() > lim.MaxOutputPixels {
		return Output{}, fmt.Errorf("%w: output would have %dx%d pixels", ErrTooManyPixels, final.Width, final.Height)
	}
	var data []byte
	if conv.TargetSizeKb > 0 {
		data, err = encodeToTarget(im, saver.id, conv, keep, conv.TargetSizeKb*1024)
	} else {
		quality := conv.Quality
		if quality == 0 {
			quality = saver.defaultQuality
		}
		data, err = encode(im, saver.id, conv, quality, keep)
	}
	if err != nil {
		if errors.Is(err, ErrEncode) && info.Format != "bmp" {
			// Release the pipeline first, so that the check below gets the
			// memory the encoder used.
			for _, f := range frames {
				f.close()
			}
			im.close()
			s.free()
			inPixels := int64(info.Width) * int64(info.Height) * int64(pages)
			need := EstimateBytes(inPixels, final.Pixels()*int64(pages), format, conv.Lossless)
			return Output{}, encodeFailure(s, input, animated, err, need)
		}
		return Output{}, err
	}
	return Output{Data: data, Format: format, Width: frameSize.Width, Height: frameSize.Height, Pages: pages}, nil
}

// splitFrames cuts a loaded animation into its frames.
func splitFrames(strip *image, maxFrames int) ([]*image, error) {
	size := strip.size()
	pageHeight := int(C.vips_image_get_page_height(strip.p))
	if pageHeight < 1 || size.Height%pageHeight != 0 {
		return nil, fmt.Errorf("%w: inconsistent frame height", ErrDecode)
	}
	n := size.Height / pageHeight
	if n > maxFrames {
		return nil, fmt.Errorf("%w: %d frames, the limit is %d", ErrTooManyFrames, n, maxFrames)
	}
	frames := make([]*image, 0, n)
	for i := range n {
		f := &image{}
		if C.px_extract(strip.p, &f.p, 0, C.int(i*pageHeight), C.int(size.Width), C.int(pageHeight)) != 0 {
			for _, g := range frames {
				g.close()
			}
			return nil, vipsError(ErrDecode, fmt.Sprintf("frame %d", i))
		}
		frames = append(frames, f)
	}
	return frames, nil
}

// joinFrames stacks processed frames into one animation.
func joinFrames(frames []*image) (*image, error) {
	ptrs := make([]*C.VipsImage, len(frames))
	for i, f := range frames {
		ptrs[i] = f.p
	}
	joined := &image{}
	if C.px_join(&ptrs[0], C.int(len(ptrs)), &joined.p) != 0 {
		return nil, fmt.Errorf("join frames: %s", lastError())
	}
	return joined, nil
}

// checkDecodes reports whether input decodes completely: nil, an error
// wrapping ErrDecode, or one wrapping ErrMemory.
func checkDecodes(s *session, input []byte, allPages bool) error {
	if C.px_decodes(s.cbytes(input), C.size_t(len(input)), cbool(allPages)) == 0 {
		return nil
	}
	return vipsError(ErrDecode, "decode")
}

// encodeFailure tells apart the three reasons for a failed encode. libvips
// decodes lazily, so a broken input often fails only while the output is
// written; a decode of its own shows that. An input that decodes but whose
// encode needs at least half of the job's memory limit most likely ran out
// of memory: libwebp, for example, reports that only as "unable to encode".
// need is the estimated memory of the job (EstimateBytes).
func encodeFailure(s *session, input []byte, animated bool, encErr error, need int64) error {
	if err := checkDecodes(s, input, animated); err != nil {
		if errors.Is(err, ErrMemory) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrDecode, encErr)
	}
	if limit := memoryLimit(); limit > 0 && need >= limit/2 {
		return fmt.Errorf("%w: %v (estimated need %d MB, limit %d MB)", ErrMemory, encErr, need>>20, limit>>20)
	}
	return encErr
}

// vipsError turns the pending libvips error into a Go error: one wrapping
// ErrMemory when an allocation failed, otherwise one wrapping base (or a
// plain error named after the operation when base is nil).
func vipsError(base error, op string) error {
	msg := lastError()
	switch {
	case IsMemoryMessage(msg):
		return fmt.Errorf("%w: %s: %s", ErrMemory, op, msg)
	case base != nil:
		return fmt.Errorf("%w: %s", base, msg)
	default:
		return fmt.Errorf("%s: %s", op, msg)
	}
}

// asDecode wraps err in ErrDecode unless it already reports a memory
// failure.
func asDecode(err error) error {
	if errors.Is(err, ErrMemory) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrDecode, err)
}

// loadBMP decodes a BMP in Go and copies it into libvips. The Go image is
// garbage once this returns.
func loadBMP(input []byte, info Info) (*image, error) {
	src, err := decodeBMP(input, info)
	if err != nil {
		return nil, err
	}
	return fromGo(src)
}

// encode runs the saver once. quality 0 means the saver's default; for PNG
// a quality enables palette quantization.
func encode(im *image, format C.int, conv pipeline.Convert, quality int, keep C.int) ([]byte, error) {
	switch format {
	case C.PX_BMP:
		img, free, err := toNRGBA(im)
		if err != nil {
			return nil, err
		}
		defer free()
		return encodeBMP(img)
	case C.PX_PDF:
		jpg, err := encode(im, C.PX_JPEG, conv, quality, keep)
		if err != nil {
			return nil, err
		}
		s := im.size()
		return wrapJPEG(jpg, s.Width, s.Height, C.vips_image_get_bands(im.p) < 3), nil
	}
	opts := C.px_save_opts{
		format:      format,
		q:           C.int(quality),
		lossless:    cbool(conv.Lossless),
		optimize:    cbool(conv.OptimizeOrDefault()),
		progressive: cbool(conv.Progressive),
		keep:        keep,
	}
	var out unsafe.Pointer
	var n C.size_t
	if C.px_save(im.p, opts, &out, &n) != 0 {
		return nil, vipsError(ErrEncode, "save")
	}
	defer C.g_free(C.gpointer(out))
	return C.GoBytes(out, C.int(n)), nil
}

// targetCopyBudget bounds the decoded copy kept for the target size search.
const targetCopyBudget = 128 << 20

// memoryBytes is the size of the decoded image in bytes.
func (im *image) memoryBytes() int64 {
	return int64(C.vips_image_get_width(im.p)) * int64(C.vips_image_get_height(im.p)) *
		int64(C.vips_image_get_bands(im.p)) * int64(C.vips_format_sizeof(C.vips_image_get_format(im.p)))
}

// encodeToTarget finds the highest quality whose output fits in limit
// bytes, with a binary search over quality 1 to 95 (at most 7 encodes).
// Lossless formats are encoded once; PNG falls back to palette
// quantization with the same search.
func encodeToTarget(im *image, format C.int, conv pipeline.Convert, keep C.int, limit int) ([]byte, error) {
	// Decode once when the pixels fit in the budget; otherwise every encode
	// runs the (lazy) pipeline again, which is slower but bounded in memory.
	if im.memoryBytes() <= targetCopyBudget {
		if err := im.apply("copy to memory", func(in *C.VipsImage, out **C.VipsImage) C.int { return C.px_copy_memory(in, out) }); err != nil {
			return nil, asDecode(err)
		}
	}
	switch format {
	case C.PX_JPEG, C.PX_WEBP, C.PX_AVIF, C.PX_JXL, C.PX_PDF:
	case C.PX_PNG:
		data, err := encode(im, format, conv, 0, keep)
		if err != nil || len(data) <= limit {
			return data, err
		}
	default:
		data, err := encode(im, format, conv, 0, keep)
		if err != nil {
			return nil, err
		}
		if len(data) > limit {
			return nil, fmt.Errorf("%w: %d bytes at the only setting", ErrTargetUnreachable, len(data))
		}
		return data, nil
	}
	var best []byte
	lo, hi := 1, 95
	for lo <= hi {
		q := (lo + hi) / 2
		data, err := encode(im, format, conv, q, keep)
		if err != nil {
			return nil, err
		}
		if len(data) <= limit {
			best, lo = data, q+1
		} else {
			hi = q - 1
		}
	}
	if best == nil {
		return nil, fmt.Errorf("%w: more than %d bytes even at the lowest quality", ErrTargetUnreachable, limit)
	}
	return best, nil
}

func runStep(ses *session, im *image, s pipeline.Step, cur pipeline.Size, lim ProcessLimits) error {
	switch s.Op {
	case pipeline.OpAutoOrient:
		return im.apply("autorot", func(in *C.VipsImage, out **C.VipsImage) C.int { return C.px_autorot(in, out) })
	case pipeline.OpResize:
		plan := s.Resize.Plan(cur)
		res, scaled := plan.Result(cur), plan.Scaled(cur)
		if err := checkSize(scaled, lim); err != nil {
			return err
		}
		if plan.HScale != 1 || plan.VScale != 1 {
			if err := im.apply("resize", func(in *C.VipsImage, out **C.VipsImage) C.int {
				return C.px_resize(in, out, C.double(plan.HScale), C.double(plan.VScale))
			}); err != nil {
				return err
			}
		}
		if plan.Crop != nil {
			return crop(im, *plan.Crop, smartFor(ses, plan.Smart), res)
		}
		return nil
	case pipeline.OpCrop:
		box, smart, ok := s.Crop.Box(cur)
		if !ok {
			return fmt.Errorf("%w: the crop rectangle lies outside the %dx%d image", ErrInvalidInput, cur.Width, cur.Height)
		}
		return crop(im, box, smartFor(ses, smart), pipeline.Size{Width: box.Width, Height: box.Height})
	case pipeline.OpRotate:
		return im.apply("rotate", func(in *C.VipsImage, out **C.VipsImage) C.int { return C.px_rot(in, out, C.int(s.Rotate)) })
	case pipeline.OpFlip:
		vertical := s.Flip == "vertical"
		return im.apply("flip", func(in *C.VipsImage, out **C.VipsImage) C.int { return C.px_flip(in, out, cbool(vertical)) })
	case pipeline.OpWatermark:
		return watermark(ses, im, *s.Watermark, cur)
	case pipeline.OpConvert:
		return nil
	}
	return errors.New("unknown operation")
}

// smartFor turns content-aware cropping off for animations, where the
// centered box (which Plan and Box return for attention and entropy) keeps
// all frames aligned.
func smartFor(ses *session, smart string) string {
	if ses.animated {
		return ""
	}
	return smart
}

func crop(im *image, box pipeline.Box, smart string, size pipeline.Size) error {
	if smart != "" {
		attention := smart == "attention"
		return im.apply("smartcrop", func(in *C.VipsImage, out **C.VipsImage) C.int {
			return C.px_smartcrop(in, out, C.int(size.Width), C.int(size.Height), cbool(attention))
		})
	}
	return im.apply("crop", func(in *C.VipsImage, out **C.VipsImage) C.int {
		return C.px_extract(in, out, C.int(box.Left), C.int(box.Top), C.int(box.Width), C.int(box.Height))
	})
}

func checkSize(s pipeline.Size, lim ProcessLimits) error {
	if s.Width > pipeline.MaxDimension || s.Height > pipeline.MaxDimension {
		return fmt.Errorf("%w: %dx%d exceeds %d pixels per side", ErrTooManyPixels, s.Width, s.Height, pipeline.MaxDimension)
	}
	if lim.MaxOutputPixels > 0 && s.Pixels() > lim.MaxOutputPixels {
		return fmt.Errorf("%w: %dx%d exceeds the output limit", ErrTooManyPixels, s.Width, s.Height)
	}
	return nil
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}
