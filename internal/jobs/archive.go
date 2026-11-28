package jobs

import (
	"bytes"
	"fmt"
	"io"

	"github.com/benerages/pixsheaf/internal/jobproto"
)

// Archive uploads (ADR 0022): the worker unpacks them in its sandbox and
// returns the list of entries and the accepted entries' bytes; the API
// stores each entry as a new file and queues the images.

// skipCodes are the reasons a worker may give for a skipped entry.
var skipCodes = map[string]bool{
	"archive_entry_unsupported": true,
	"too_large":                 true,
	"archive_invalid":           true,
}

// unzipRequest builds the task for an archive. m.mu must be held.
func (m *Manager) unzipRequest(f *file) jobproto.Request {
	j := f.job
	images := 0
	for _, g := range j.files {
		if g.kind == KindImage {
			images++
		}
	}
	return jobproto.Request{
		Op:            jobproto.OpUnzip,
		InputBytes:    f.inBytes,
		MaxEntries:    m.lim.MaxArchiveEntries,
		MaxFiles:      m.lim.MaxFiles - images,
		MaxEntryBytes: m.lim.MaxUploadBytes,
		// The archive itself stops counting once it is unpacked.
		MaxTotalBytes: max(m.lim.MaxJobBytes-(j.bytes-f.inBytes), 0),
	}
}

// entryFile is an unpacked entry before it is attached to its job.
type entryFile struct {
	name   string
	blob   string
	bytes  int64
	format string
	code   string
	msg    string
}

// completeArchive stores the entries of an unpacked archive and queues
// the images among them.
func (m *Manager) completeArchive(f *file, resp jobproto.Response, output io.Reader) error {
	if resp.Error != nil {
		m.mu.Lock()
		m.failLocked(f, resp.Error.Code, resp.Error.Message)
		m.mu.Unlock()
		return nil
	}
	m.mu.Lock()
	maxFiles := m.unzipRequest(f).MaxFiles
	full := m.stored+resp.OutputBytes > m.lim.MaxStoredBytes
	m.mu.Unlock()
	if err := m.validEntries(resp, maxFiles); err != nil {
		m.mu.Lock()
		m.failLocked(f, jobproto.CodeInternal, "the worker returned an invalid archive listing")
		m.mu.Unlock()
		return err
	}
	if full {
		m.mu.Lock()
		m.failLocked(f, "too_large", "the server's storage is full; retry later")
		m.mu.Unlock()
		return nil
	}

	entries, err := m.storeEntries(resp.Entries, output)
	m.mu.Lock()
	defer m.mu.Unlock()
	j := f.job
	if err != nil || f.status != FileProcessing || m.jobs[j.id] != j {
		for _, e := range entries {
			if e.blob != "" {
				_ = m.root.Remove(e.blob)
			}
		}
		if err != nil {
			m.failLocked(f, jobproto.CodeInternal, "the archive entries could not be stored")
		}
		return err
	}
	j.bytes -= f.inBytes
	m.removeInput(f)
	f.status = FileDone
	for _, e := range entries {
		g := &file{
			job: j, id: newID(), kind: KindImage, archive: f.id, index: len(j.files),
			name: e.name, input: e.blob, inBytes: e.bytes, inFormat: e.format,
		}
		j.files = append(j.files, g)
		switch {
		case e.code != "":
			g.status, g.errCode, g.errMsg = FileFailed, e.code, e.msg
		default:
			g.status = FileQueued
			j.bytes += e.bytes
			m.stored += e.bytes
			m.queue.push(g)
		}
	}
	m.signal()
	return nil
}

// validEntries checks the listing from the worker, which is untrusted.
func (m *Manager) validEntries(resp jobproto.Response, maxFiles int) error {
	if len(resp.Entries) > maxFiles {
		return fmt.Errorf("%w: %d entries for %d files", ErrInvalidResult, len(resp.Entries), maxFiles)
	}
	var total int64
	for _, e := range resp.Entries {
		switch {
		case e.Code != "" && (!skipCodes[e.Code] || e.Bytes != 0):
			return fmt.Errorf("%w: skipped entry with code %q and %d bytes", ErrInvalidResult, e.Code, e.Bytes)
		case e.Code == "" && (e.Bytes < 0 || e.Bytes > m.lim.MaxUploadBytes):
			return fmt.Errorf("%w: entry of %d bytes", ErrInvalidResult, e.Bytes)
		}
		total += e.Bytes
	}
	if total != resp.OutputBytes {
		return fmt.Errorf("%w: entries add up to %d, output has %d bytes", ErrInvalidResult, total, resp.OutputBytes)
	}
	return nil
}

// storeEntries reads the accepted entries from output into new blobs and
// sniffs them. Entries that are not allowed images become failed files.
func (m *Manager) storeEntries(listing []jobproto.ArchiveEntry, output io.Reader) ([]entryFile, error) {
	out := make([]entryFile, 0, len(listing))
	for _, e := range listing {
		ef := entryFile{name: e.Name, code: e.Code}
		if ef.code == "" && !AllowedExtension(e.Name) {
			ef.code = jobproto.CodeUnsupportedFormat
		}
		if ef.code != "" {
			ef.msg = skipMessage(ef.code)
			out = append(out, ef)
			// The worker streams every entry it accepted; skip the bytes
			// of an entry that is rejected here.
			if e.Code == "" {
				if _, err := io.CopyN(io.Discard, output, e.Bytes); err != nil {
					return out, err
				}
			}
			continue
		}
		name, w, err := m.create()
		if err != nil {
			return out, err
		}
		var head bytes.Buffer
		n, err := io.Copy(io.MultiWriter(w, &limitedWriter{w: &head, n: SniffLen}), io.LimitReader(output, e.Bytes))
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		ef.blob, ef.bytes = name, n
		if err == nil && n != e.Bytes {
			err = fmt.Errorf("%w: entry has %d of %d bytes", ErrInvalidResult, n, e.Bytes)
		}
		if err != nil {
			out = append(out, ef)
			return out, err
		}
		switch ef.format = Sniff(head.Bytes()); {
		case ef.format == "":
			ef.code, ef.msg = jobproto.CodeUnsupportedFormat, "the file is not an allowed image format"
		case m.formatDisabled(ef.format):
			ef.code, ef.msg = jobproto.CodeFormatDisabled, "this image format is disabled on this server"
		case ef.format == FormatZip:
			ef.code, ef.msg = "archive_entry_unsupported", skipMessage("archive_entry_unsupported")
		}
		if ef.code != "" {
			_ = m.root.Remove(name)
			ef.blob, ef.bytes = "", 0
		}
		out = append(out, ef)
	}
	return out, nil
}

func skipMessage(code string) string {
	switch code {
	case "archive_entry_unsupported":
		return "symbolic links, encrypted entries, and nested archives are not supported"
	case "too_large":
		return "the entry is too large or compressed too strongly"
	case jobproto.CodeUnsupportedFormat:
		return "the file name must end in an image extension such as .jpg or .png"
	default:
		return "the entry could not be read"
	}
}

// limitedWriter keeps the first n bytes written to it.
type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		if _, err := l.w.Write(p[:k]); err != nil {
			return 0, err
		}
		l.n -= k
	}
	return len(p), nil
}
