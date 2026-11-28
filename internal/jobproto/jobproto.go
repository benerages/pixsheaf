// Package jobproto defines the messages between the worker supervisor and a
// job process (ADR 0006). Each direction carries one length-prefixed JSON
// header followed by raw bytes: the request header is followed by the input
// image, the response header by the output.
package jobproto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/benerages/pixsheaf/internal/sandbox"
)

// MaxHeaderBytes bounds a header in either direction. A request carries
// the pipeline, which may embed an image watermark of up to 1 MiB in
// base64 (about 1.4 MB); a response may list the entries of an archive.
const MaxHeaderBytes = 4 << 20

// Op selects what a job process does.
type Op string

// Operations understood by the job process.
const (
	// OpInspect reads only the image header: format, size, and pages.
	OpInspect Op = "inspect"
	// OpSandboxCheck reports the restrictions in effect inside the job
	// process; used by the container self-check.
	OpSandboxCheck Op = "sandbox-check"
	// OpProcess runs a pipeline and returns the encoded result.
	OpProcess Op = "process"
	// OpUnzip checks a ZIP archive and returns its files (ADR 0022): the
	// response lists the entries, and the output is the accepted entries
	// concatenated in that order.
	OpUnzip Op = "unzip"
)

// Request is sent by the supervisor.
type Request struct {
	Op Op `json:"op"`
	// InputBytes is the exact number of input bytes after the header.
	InputBytes int64 `json:"inputBytes"`
	// MaxPixels bounds width x height x pages of the input for OpInspect
	// and OpProcess; zero means no limit.
	MaxPixels int64 `json:"maxPixels,omitempty"`
	// MaxOutputPixels bounds intermediate and final results of OpProcess.
	MaxOutputPixels int64 `json:"maxOutputPixels,omitempty"`
	// MaxFrames bounds the frames of animated GIF and WebP input; zero
	// means the hard limit (imaging.MaxFrames).
	MaxFrames int `json:"maxFrames,omitempty"`
	// Archive limits for OpUnzip (see archive.Limits).
	MaxEntries    int   `json:"maxEntries,omitempty"`
	MaxFiles      int   `json:"maxFiles,omitempty"`
	MaxEntryBytes int64 `json:"maxEntryBytes,omitempty"`
	MaxTotalBytes int64 `json:"maxTotalBytes,omitempty"`
	// DisabledFormats are input formats the operator disabled
	// (PIXSHEAF_DISABLE_FORMATS, ADR 0016); the job process refuses them
	// and keeps their loaders blocked.
	DisabledFormats []string `json:"disabledFormats,omitempty"`
	// Pipeline is the pipeline document for OpProcess (api/openapi.yaml).
	// The job process validates it again.
	Pipeline json.RawMessage `json:"pipeline,omitempty"`
}

// Response is sent by the job process.
type Response struct {
	Error   *Error          `json:"error,omitempty"`
	Image   *ImageInfo      `json:"image,omitempty"`
	Output  *ImageInfo      `json:"output,omitempty"`
	Sandbox *sandbox.Report `json:"sandbox,omitempty"`
	// Entries lists the files of an archive (OpUnzip).
	Entries []ArchiveEntry `json:"entries,omitempty"`
	// OutputBytes is the exact number of output bytes after the header.
	OutputBytes int64 `json:"outputBytes"`
}

// Error codes match the API's FileError codes (api/openapi.yaml).
const (
	CodeUnsupportedFormat = "unsupported_format"
	CodeFormatDisabled    = "format_disabled"
	CodeTooManyPixels     = "too_many_pixels"
	CodeTooManyFrames     = "too_many_frames"
	CodeDecodeFailed      = "decode_failed"
	CodeEncodeFailed      = "encode_failed"
	CodeStepNotApplicable = "step_not_applicable"
	CodeTargetUnreachable = "target_size_unreachable"
	CodeTimeout           = "timeout"
	CodeTooManyFiles      = "too_many_files"
	CodeArchiveInvalid    = "archive_invalid"
	CodeInternal          = "internal"
)

// Error describes why a job failed. Messages never contain file contents.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ArchiveEntry is one file of an unpacked archive. Code is empty for an
// accepted entry and a FileError code for a skipped one.
type ArchiveEntry struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Code  string `json:"code,omitempty"`
}

// ImageInfo describes an image header.
type ImageInfo struct {
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Pages  int    `json:"pages"`
}

// WriteHeader writes v as a length-prefixed JSON header.
func WriteHeader(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > MaxHeaderBytes {
		return fmt.Errorf("jobproto: header of %d bytes exceeds %d", len(b), MaxHeaderBytes)
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(b))) //nolint:gosec // G115: bounded by MaxHeaderBytes above
	if _, err := w.Write(size[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// ReadHeader reads a length-prefixed JSON header into v. Unknown fields
// and trailing data inside the header are rejected.
func ReadHeader(r io.Reader, v any) error {
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return fmt.Errorf("jobproto: reading header size: %w", err)
	}
	n := binary.BigEndian.Uint32(size[:])
	if n == 0 || n > MaxHeaderBytes {
		return fmt.Errorf("jobproto: invalid header size %d", n)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return fmt.Errorf("jobproto: reading header: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("jobproto: decoding header: %w", err)
	}
	if dec.More() {
		return errors.New("jobproto: trailing data in header")
	}
	return nil
}
