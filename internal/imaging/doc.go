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

// Info describes an image header.
type Info struct {
	// Format is an input format name from api/openapi.yaml: jpeg, png,
	// webp, gif, tiff, avif, heif, or jxl.
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
