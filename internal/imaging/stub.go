//go:build !vips

package imaging

// Init reports that libvips is not available in this build.
func Init() error { return ErrUnavailable }

// LibraryVersion returns "none" in builds without libvips.
func LibraryVersion() string { return "none" }

// FindLoader reports that libvips is not available in this build.
func FindLoader([]byte) (string, error) { return "", ErrUnavailable }

// Probe reports that libvips is not available in this build.
func Probe() []ProbeResult {
	return []ProbeResult{{Suffix: "libvips", Err: ErrUnavailable}}
}
