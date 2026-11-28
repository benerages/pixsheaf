// Package imaging wraps libvips for the worker.
//
// The libvips implementation is compiled only with the "vips" build tag
// (cgo, inside the worker image; see deploy/docker/worker.Dockerfile).
// Without the tag a stub reports that image processing is unavailable, so
// the rest of the module builds and tests without libvips installed.
package imaging

import "errors"

// ErrUnavailable is returned by builds without the "vips" build tag.
var ErrUnavailable = errors.New("imaging: built without libvips (build tag \"vips\")")

// ErrUnsupported means that no allowed loader accepts the input (ADR 0008,
// ADR 0014).
var ErrUnsupported = errors.New("imaging: no allowed loader accepts the input")

// ErrDecode means that an allowed loader rejected the input as broken.
var ErrDecode = errors.New("imaging: the image could not be decoded")

// ErrTooManyPixels means that an input or result exceeds a pixel limit.
var ErrTooManyPixels = errors.New("imaging: too many pixels")

// ErrMemory means that the job ran out of memory: processing the image needs
// more than the job memory limit (PIXSHEAF_JOB_MEMORY_MB) allows.
var ErrMemory = errors.New("imaging: the job memory limit was reached")

// ErrEncode means that the output could not be encoded.
var ErrEncode = errors.New("imaging: the image could not be encoded")

// ErrInvalidInput means that a step does not fit the image, for example a
// crop rectangle outside it.
var ErrInvalidInput = errors.New("imaging: the step does not fit the image")

// ErrTargetUnreachable means that no encoder setting produces an output
// within the requested target size.
var ErrTargetUnreachable = errors.New("imaging: the target size cannot be reached")

// ErrTooManyFrames means that an animation has more frames than allowed.
var ErrTooManyFrames = errors.New("imaging: too many frames")

// ErrNotImplemented marks an output format without a saver; it should not
// occur with a validated pipeline.
var ErrNotImplemented = errors.New("imaging: not implemented")

// MaxFrames is the hard limit for frames of an animation. Operators can
// set a lower limit in ProcessLimits.
const MaxFrames = 1000

// ProcessLimits bound Process. Zero means no limit.
type ProcessLimits struct {
	// MaxInputPixels bounds width x height x pages of the input, checked
	// from the header before decoding.
	MaxInputPixels int64
	// MaxOutputPixels bounds every intermediate and the final result.
	MaxOutputPixels int64
	// MaxFrames bounds the frames of an animation (at most MaxFrames).
	MaxFrames int
}

// Output is an encoded result.
type Output struct {
	Data   []byte
	Format string
	// Width and Height are the size of one frame.
	Width  int
	Height int
	// Pages is the number of frames (1 unless an animation is kept).
	Pages int
}

// Info describes an image header.
type Info struct {
	// Format is an input format name from api/openapi.yaml: jpeg, png,
	// webp, gif, tiff, bmp, avif, heif, or jxl.
	Format string
	Width  int
	Height int
	// Pages is the number of pages or animation frames (at least 1).
	Pages int
}

// ProbeResult is the outcome of one codec round trip in Probe: a blank
// image is saved in the format and the saved bytes must be recognized by
// the expected loader.
type ProbeResult struct {
	Suffix string // output format as a file suffix, such as ".avif"
	Loader string // expected libvips loader class, such as "VipsForeignLoadJpegBuffer"
	Err    error
}
