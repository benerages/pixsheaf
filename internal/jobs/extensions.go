package jobs

import "strings"

// allowedExtensions are the file name extensions accepted for uploads and
// archive entries (ASVS 5.0 5.2.2, ADR 0023). The content still decides
// the format: it is sniffed from the magic bytes and always re-encoded, so
// a mismatch between extension and content is accepted.
var allowedExtensions = map[string]bool{
	"jpg": true, "jpeg": true, "jpe": true, "jfif": true,
	"png": true, "webp": true, "avif": true, "gif": true,
	"tif": true, "tiff": true, "bmp": true, "jxl": true,
	"heic": true, "heif": true, "hif": true,
	"zip": true,
}

// AllowedExtension reports whether the last segment of an uploaded name
// ends in an allowed extension, compared case-insensitively.
func AllowedExtension(name string) bool {
	base := name[strings.LastIndexAny(name, `/\`)+1:]
	dot := strings.LastIndexByte(base, '.')
	if dot <= 0 || dot == len(base)-1 {
		return false
	}
	return allowedExtensions[strings.ToLower(base[dot+1:])]
}
