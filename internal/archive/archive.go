// Package archive unpacks uploaded ZIP archives inside a job process
// (ADR 0022). It never writes to a file system: entries are streamed to
// the job's output, and entry names are only cleaned for display.
//
// Unpacking takes two passes over the archive, which is held in memory.
// Scan decompresses every entry into a counter to learn and check its real
// size; Write then streams the accepted entries. Memory stays bounded by
// the archive itself, independent of what the entries expand to.
package archive

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

// Reasons for skipped entries; they are FileError codes of the API.
const (
	SkipUnsupported = "archive_entry_unsupported"
	SkipTooLarge    = "too_large"
	SkipInvalid     = "archive_invalid"
)

// Errors that fail the whole archive.
var (
	ErrInvalid      = errors.New("archive: not a readable ZIP archive")
	ErrTooManyFiles = errors.New("archive: too many entries")
)

// maxNameLen bounds an entry name before cleaning.
const maxNameLen = 1024

// Limits bound unpacking.
type Limits struct {
	// MaxEntries bounds all entries, folders included.
	MaxEntries int
	// MaxFiles bounds file entries (accepted or skipped).
	MaxFiles int
	// MaxEntryBytes bounds the real size of one entry.
	MaxEntryBytes int64
	// MaxTotalBytes bounds the accepted entries together; entries beyond
	// it are skipped as too large.
	MaxTotalBytes int64
	// MaxRatio bounds real size divided by compressed size per entry.
	MaxRatio int64
}

// Entry is one file of the archive. Skip is empty for accepted entries.
type Entry struct {
	Name  string
	Bytes int64
	Skip  string
	index int
}

// Scan checks every entry and returns the files in archive order.
func Scan(data []byte, lim Limits) ([]Entry, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if len(zr.File) > lim.MaxEntries {
		return nil, fmt.Errorf("%w: %d entries, the limit is %d", ErrTooManyFiles, len(zr.File), lim.MaxEntries)
	}
	var entries []Entry
	var total int64
	for i, f := range zr.File {
		if f.Mode().IsDir() || strings.HasSuffix(f.Name, "/") {
			continue
		}
		if len(entries) == lim.MaxFiles {
			return nil, fmt.Errorf("%w: more than %d files", ErrTooManyFiles, lim.MaxFiles)
		}
		e := Entry{Name: CleanName(f.Name), index: i}
		e.Bytes, e.Skip = check(f, lim)
		if e.Skip == "" && total+e.Bytes > lim.MaxTotalBytes {
			e.Skip = SkipTooLarge
		}
		if e.Skip == "" {
			total += e.Bytes
		} else {
			e.Bytes = 0
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// check decompresses one entry into a counter and returns its real size,
// or the reason to skip it.
func check(f *zip.File, lim Limits) (int64, string) {
	switch {
	case f.Mode()&fs.ModeSymlink != 0, f.Flags&0x1 != 0:
		// Symbolic links and encrypted entries.
		return 0, SkipUnsupported
	case f.Method != zip.Store && f.Method != zip.Deflate:
		return 0, SkipUnsupported
	}
	limit := lim.MaxEntryBytes
	if bound := int64(f.CompressedSize64) * lim.MaxRatio; f.Method == zip.Deflate && bound < limit { //nolint:gosec // G115: compressed sizes are bounded by the archive size
		limit = bound
	}
	rc, err := f.Open()
	if err != nil {
		return 0, SkipInvalid
	}
	defer func() { _ = rc.Close() }()
	head := make([]byte, 8)
	k, err := io.ReadFull(rc, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return 0, SkipInvalid
	}
	if isArchive(head[:k]) {
		return 0, SkipUnsupported
	}
	// Reading to the end also verifies the size and CRC-32 from the header.
	n, err := io.Copy(io.Discard, io.LimitReader(rc, limit-int64(k)+1))
	n += int64(k)
	switch {
	case n > limit:
		return 0, SkipTooLarge
	case err != nil:
		return 0, SkipInvalid
	}
	return n, ""
}

// isArchive detects nested archives and compressed streams.
func isArchive(b []byte) bool {
	for _, magic := range [][]byte{
		[]byte("PK\x03\x04"), []byte("PK\x05\x06"), []byte("PK\x07\x08"), // ZIP
		{0x1f, 0x8b},                     // gzip
		[]byte("7z\xbc\xaf\x27\x1c"),     // 7-Zip
		[]byte("Rar!\x1a\x07"),           // RAR
		[]byte("BZh"),                    // bzip2
		{0xfd, '7', 'z', 'X', 'Z', 0x00}, // xz
		{0x28, 0xb5, 0x2f, 0xfd},         // zstd
	} {
		if bytes.HasPrefix(b, magic) {
			return true
		}
	}
	return false
}

// Write streams the accepted entries of a scanned archive to w, in order.
// Every entry must have exactly the size Scan found.
func Write(data []byte, entries []Entry, w io.Writer) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	for _, e := range entries {
		if e.Skip != "" {
			continue
		}
		rc, err := zr.File[e.index].Open()
		if err != nil {
			return err
		}
		n, err := io.Copy(w, io.LimitReader(rc, e.Bytes))
		var extra [1]byte
		k, _ := rc.Read(extra[:])
		_ = rc.Close()
		switch {
		case err != nil:
			return err
		case n != e.Bytes || k != 0:
			return fmt.Errorf("archive: entry %d changed size between passes", e.index)
		}
	}
	return nil
}

// CleanName turns an entry name into a relative path without "." and ".."
// segments, empty segments, or a leading slash. It is used for display and
// the folder structure of downloads only, never to open a file.
func CleanName(name string) string {
	if len(name) > maxNameLen {
		name = name[len(name)-maxNameLen:]
	}
	name = strings.ReplaceAll(name, `\`, "/")
	var parts []string
	for _, p := range strings.Split(name, "/") {
		switch p {
		case "", ".", "..":
			continue
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return "file"
	}
	return path.Join(parts...)
}
