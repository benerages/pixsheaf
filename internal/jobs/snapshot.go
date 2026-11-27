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
	// Source is the sanitized upload name with its folder path.
	Source string
	// Kind is KindImage or KindArchive; ArchiveID is set for images that
	// came from an archive.
	Kind      string
	ArchiveID string
	// Downloaded is set once the result was downloaded on its own.
	Downloaded bool
	ErrorCode  string
	ErrorMsg   string
}

// status derives the job status from its files.
func (j *job) status() string {
	switch {
	case j.canceled:
		return JobCanceled
	case j.uploads < j.expected || j.pending > 0:
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
		ID: f.id, Index: f.index, Kind: f.kind, ArchiveID: f.archive, Status: f.status, Downloaded: f.downloaded,
		InputBytes: f.inBytes, InputFormat: f.inFormat,
		ErrorCode: f.errCode, ErrorMsg: f.errMsg,
	}
	if f.out != nil {
		out := *f.out
		s.Output, s.OutputBytes, s.OutputName = &out, f.outBytes, f.outputName(uniqueNames{})
	}
	s.Source = SourceName(f.name)
	return s
}

// outputName applies the job's naming to a processed file and makes the
// result unique within names. It includes the folder if the job keeps
// folders.
func (f *file) outputName(names uniqueNames) string {
	folder, base := splitUpload(f.name)
	index, total := 0, 0
	for _, g := range f.job.files {
		if g.kind == KindImage {
			total++
			if g.index <= f.index {
				index++
			}
		}
	}
	name := expandPattern(f.job.naming.Pattern, nameVars{
		name: base, index: index, total: total,
		width: f.out.Width, height: f.out.Height, date: f.job.created, format: f.out.Format,
	})
	if f.job.naming.KeepFolders && folder != "" {
		name = folder + "/" + name
	}
	return names.take(name)
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
