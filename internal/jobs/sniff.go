package jobs

import "bytes"

// FormatZip marks a ZIP archive, which is unpacked in the worker sandbox
// (ADR 0022).
const FormatZip = "zip"

// SniffLen is the number of leading bytes Sniff needs.
const SniffLen = 32

// Sniff detects an allowed input format from the first bytes of a file
// (magic bytes, never the name or Content-Type). It is a cheap early
// check in the API; the worker decodes the header again in its sandbox
// and has the final word. It returns "" for anything else.
func Sniff(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "jpeg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return "gif"
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "webp"
	case bytes.HasPrefix(b, []byte("II*\x00")), bytes.HasPrefix(b, []byte("MM\x00*")):
		return "tiff"
	case bytes.HasPrefix(b, []byte("BM")) && len(b) >= 18:
		return "bmp"
	case bytes.HasPrefix(b, []byte{0xFF, 0x0A}),
		bytes.HasPrefix(b, []byte("\x00\x00\x00\x0cJXL \r\n\x87\n")):
		return "jxl"
	case len(b) >= 12 && bytes.Equal(b[4:8], []byte("ftyp")):
		return isobmff(b)
	case bytes.HasPrefix(b, []byte("PK\x03\x04")), bytes.HasPrefix(b, []byte("PK\x05\x06")):
		return FormatZip
	}
	return ""
}

// isobmff maps the major brand of an ISO base media file to avif or heif.
func isobmff(b []byte) string {
	switch string(b[8:12]) {
	case "avif", "avis":
		return "avif"
	case "heic", "heix", "heim", "heis", "hevc", "hevx", "mif1", "msf1":
		return "heif"
	}
	return ""
}
