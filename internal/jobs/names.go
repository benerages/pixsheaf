package jobs

import (
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxBaseName bounds the base of an output name in characters.
const maxBaseName = 100

// extensions maps output formats to file name extensions.
var extensions = map[string]string{
	"jpeg": "jpg", "png": "png", "webp": "webp", "gif": "gif", "tiff": "tif",
	"bmp": "bmp", "avif": "avif", "jxl": "jxl", "pdf": "pdf",
}

// contentTypes maps output formats to media types.
var contentTypes = map[string]string{
	"jpeg": "image/jpeg", "png": "image/png", "webp": "image/webp", "gif": "image/gif",
	"tiff": "image/tiff", "bmp": "image/bmp", "avif": "image/avif", "jxl": "image/jxl",
	"pdf": "application/pdf",
}

// ContentType returns the media type for an output format.
func ContentType(format string) string {
	if ct, ok := contentTypes[format]; ok {
		return ct
	}
	return "application/octet-stream"
}

// OutputName derives a safe download name from the uploaded file name and
// the output format: the base name without directories and extension,
// with only letters, digits, and " ._-" kept (others become "_"), at most
// 100 characters, and "image" if nothing is left. The uploaded name is
// never used as a path and never logged.
func OutputName(uploaded, format string) string {
	base := strings.ReplaceAll(uploaded, `\`, "/")
	base = path.Base(base)
	if ext := path.Ext(base); ext != "" && ext != base {
		base = strings.TrimSuffix(base, ext)
	}
	var b strings.Builder
	n := 0
	for _, r := range base {
		if n == maxBaseName {
			break
		}
		switch {
		case r == utf8.RuneError:
			continue
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_', r == '.', r == ' ':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
		n++
	}
	name := strings.Trim(b.String(), " ._")
	if name == "" {
		name = "image"
	}
	ext, ok := extensions[format]
	if !ok {
		ext = "bin"
	}
	return name + "." + ext
}
