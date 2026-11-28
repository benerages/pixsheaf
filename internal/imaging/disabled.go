package imaging

import "errors"

// ErrFormatDisabled means the operator disabled the input format
// (PIXSHEAF_DISABLE_FORMATS, ADR 0016).
var ErrFormatDisabled = errors.New("imaging: the input format is disabled on this server")

// disabledFormats holds the input formats refused by this process.
var disabledFormats = map[string]bool{}

// DisableFormats refuses the given input formats ("heif", "avif", ...).
// Call it before the first use of libvips: Init then keeps the loaders of
// these formats blocked, and Inspect and Process reject them even if the
// API's magic-byte check named a different format.
func DisableFormats(formats []string) {
	for _, f := range formats {
		disabledFormats[f] = true
	}
}

// loaderDisabled reports whether every format a loader class reads is
// disabled, so that the loader can stay blocked.
func loaderDisabled(formats []string) bool {
	if len(formats) == 0 {
		return false
	}
	for _, f := range formats {
		if !disabledFormats[f] {
			return false
		}
	}
	return true
}
