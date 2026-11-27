package jobs

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Name limits.
const (
	// maxBaseName bounds the base of an output name in characters.
	maxBaseName = 100
	// maxFolderDepth and maxSourceLen bound the folder path kept from folder
	// uploads and archive entries.
	maxFolderDepth = 8
	maxSourceLen   = 1024
	// MaxPatternLen bounds a naming pattern.
	MaxPatternLen = 200
)

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

// sanitizeSegment keeps letters, digits, and " ._-" of one name segment
// (others become "_"), at most limit characters, without leading or
// trailing spaces, dots, or underscores. It may return "".
func sanitizeSegment(s string, limit int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == limit {
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
	return strings.Trim(b.String(), " ._")
}

// splitUpload splits an uploaded name into its sanitized folder path and
// its base name without extension (raw, for the {name} placeholder).
// Backslashes count as separators; "." and ".." segments are dropped.
func splitUpload(uploaded string) (folder, base string) {
	parts := strings.Split(strings.ReplaceAll(uploaded, `\`, "/"), "/")
	last := parts[len(parts)-1]
	if ext := path.Ext(last); ext != "" && ext != last {
		last = strings.TrimSuffix(last, ext)
	}
	var dirs []string
	for _, p := range parts[:len(parts)-1] {
		if seg := sanitizeSegment(p, maxBaseName); seg != "" && len(dirs) < maxFolderDepth {
			dirs = append(dirs, seg)
		}
	}
	folder = strings.Join(dirs, "/")
	if len(folder) > maxSourceLen/2 {
		folder = ""
	}
	return folder, last
}

// SourceName is the sanitized upload name with its folder path, for
// display and the report.
func SourceName(uploaded string) string {
	folder, _ := splitUpload(uploaded)
	parts := strings.Split(strings.ReplaceAll(uploaded, `\`, "/"), "/")
	name := sanitizeSegment(parts[len(parts)-1], maxBaseName)
	if name == "" {
		name = "image"
	}
	if folder != "" {
		return folder + "/" + name
	}
	return name
}

// OutputName derives a safe download name from the uploaded file name and
// the output format with the default pattern "{name}". The uploaded name
// is never used as a path and never logged.
func OutputName(uploaded, format string) string {
	_, base := splitUpload(uploaded)
	return finishName(base, format)
}

// finishName sanitizes a base name and appends the extension of format.
func finishName(base, format string) string {
	name := sanitizeSegment(base, maxBaseName)
	if name == "" {
		name = "image"
	}
	ext, ok := extensions[format]
	if !ok {
		ext = "bin"
	}
	return name + "." + ext
}

// Naming is the naming of a job's outputs (api/openapi.yaml).
type Naming struct {
	Pattern     string
	KeepFolders bool
}

// placeholders are the names a pattern may use.
var placeholders = map[string]bool{"name": true, "index": true, "width": true, "height": true, "date": true, "ext": true}

// ErrPattern describes an invalid naming pattern.
var ErrPattern = errors.New("jobs: invalid naming pattern")

// ValidatePattern checks a naming pattern: known placeholders only, no
// stray braces, at most MaxPatternLen characters.
func ValidatePattern(p string) error {
	if p == "" || utf8.RuneCountInString(p) > MaxPatternLen || !utf8.ValidString(p) {
		return fmt.Errorf("%w: must be 1 to %d characters", ErrPattern, MaxPatternLen)
	}
	for rest := p; rest != ""; {
		i := strings.IndexAny(rest, "{}")
		if i < 0 {
			break
		}
		if rest[i] == '}' {
			return fmt.Errorf("%w: unmatched }", ErrPattern)
		}
		end := strings.IndexByte(rest[i:], '}')
		if end < 0 {
			return fmt.Errorf("%w: unmatched {", ErrPattern)
		}
		if name := rest[i+1 : i+end]; !placeholders[name] {
			return fmt.Errorf("%w: unknown placeholder {%s}", ErrPattern, name)
		}
		rest = rest[i+end+1:]
	}
	return nil
}

// nameVars are the values for a pattern.
type nameVars struct {
	name          string
	index, total  int
	width, height int
	date          time.Time
	format        string
}

// expandPattern fills a validated pattern and returns the sanitized name
// with the extension of the output format.
func expandPattern(p string, v nameVars) string {
	digits := len(strconv.Itoa(max(v.total, 1)))
	r := strings.NewReplacer(
		"{name}", v.name,
		"{index}", fmt.Sprintf("%0*d", digits, v.index),
		"{width}", strconv.Itoa(v.width),
		"{height}", strconv.Itoa(v.height),
		"{date}", v.date.UTC().Format("2006-01-02"),
		"{ext}", extensions[v.format],
	)
	return finishName(r.Replace(p), v.format)
}

// uniqueNames hands out names that are unique within a ZIP archive,
// compared case-insensitively.
type uniqueNames map[string]bool

func (u uniqueNames) take(name string) string {
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	candidate := name
	for i := 2; u[strings.ToLower(candidate)]; i++ {
		candidate = fmt.Sprintf("%s (%d)%s", base, i, ext)
	}
	u[strings.ToLower(candidate)] = true
	return candidate
}
