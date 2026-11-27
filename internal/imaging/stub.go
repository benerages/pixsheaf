//go:build !vips

package imaging

import "github.com/benerages/pixsheaf/internal/pipeline"

// Init reports that libvips is not available in this build.
func Init() error { return ErrUnavailable }

// LibraryVersion returns "none" in builds without libvips.
func LibraryVersion() string { return "none" }

// FindLoader reports that libvips is not available in this build.
func FindLoader([]byte) (string, error) { return "", ErrUnavailable }

// Inspect reports that libvips is not available in this build.
func Inspect([]byte) (Info, error) { return Info{}, ErrUnavailable }

// Process reports that libvips is not available in this build.
func Process([]byte, pipeline.Pipeline, ProcessLimits) (Output, error) {
	return Output{}, ErrUnavailable
}

// Probe reports that libvips is not available in this build.
func Probe() []ProbeResult {
	return []ProbeResult{{Suffix: "libvips", Err: ErrUnavailable}}
}
