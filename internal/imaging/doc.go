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

// ProbeResult is the outcome of one codec round trip in Probe: a blank
// image is saved in the format and the saved bytes must be recognized by
// the expected loader.
type ProbeResult struct {
	Suffix string // output format as a file suffix, such as ".avif"
	Loader string // expected libvips loader class, such as "VipsForeignLoadJpegBuffer"
	Err    error
}
