package jobs

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"io"
	"time"

	"github.com/benerages/pixsheaf/internal/jobproto"
)

// Job is a snapshot of a job for the HTTP layer.
type Job struct {
	ID            string
	Status        string
	ExpectedFiles int
	CreatedAt     time.Time
	ExpiresAt     time.Time
	Files         []File
}

// File is a snapshot of a file. OutputName is the sanitized download name.
type File struct {
	ID          string
	Index       int
	Status      string
	InputBytes  int64
	InputFormat string
	Output      *jobproto.ImageInfo
	OutputBytes int64
	OutputName  string
	ErrorCode   string
	ErrorMsg    string
}

// status derives the job status from its files.
func (j *job) status() string {
	switch {
	case j.canceled:
		return JobCanceled
	case len(j.files) < j.expected || j.pending > 0:
		return JobOpen
	}
	for _, f := range j.files {
		if f.status == FileQueued || f.status == FileProcessing {
			return JobProcessing
		}
	}
	return JobDone
}

func (j *job) snapshot() Job {
	s := Job{
		ID: j.id, Status: j.status(), ExpectedFiles: j.expected,
		CreatedAt: j.created, ExpiresAt: j.expires,
		Files: make([]File, 0, len(j.files)),
	}
	for _, f := range j.files {
		s.Files = append(s.Files, f.snapshot())
	}
	return s
}

func (f *file) snapshot() File {
	s := File{
		ID: f.id, Index: f.index, Status: f.status,
		InputBytes: f.inBytes, InputFormat: f.inFormat,
		ErrorCode: f.errCode, ErrorMsg: f.errMsg,
	}
	if f.out != nil {
		out := *f.out
		s.Output, s.OutputBytes, s.OutputName = &out, f.outBytes, OutputName(f.name, out.Format)
	}
	return s
}

// newID returns 128 random bits, base64url without padding.
func newID() string { return randomString(16) }

// newToken returns 256 random bits, base64url without padding.
func newToken() string { return randomString(32) }

func randomString(n int) string {
	b := make([]byte, n)
	// crypto/rand.Read never returns an error (Go 1.24 and later).
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }
