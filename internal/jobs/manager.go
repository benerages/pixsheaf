// Package jobs keeps the jobs of the API in memory: their files on a
// tmpfs directory, the queue of files waiting for a worker, and the leases
// of files a worker is processing (ADR 0007, ADR 0019).
//
// Nothing survives a restart. Files are stored under random names, never
// under the uploaded name, and are deleted as soon as they are no longer
// needed: inputs after processing, outputs after the download, everything
// when a job is deleted or expires.
package jobs

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/benerages/pixsheaf/internal/jobproto"
)

// Errors returned by Manager. The HTTP layer maps them to problem codes.
var (
	ErrNotFound         = errors.New("jobs: not found")
	ErrUnauthorized     = errors.New("jobs: wrong or missing job token")
	ErrNotOpen          = errors.New("jobs: the job accepts no more uploads")
	ErrTooLarge         = errors.New("jobs: file too large")
	ErrJobTooLarge      = errors.New("jobs: job too large")
	ErrUnsupported      = errors.New("jobs: not an allowed image format")
	ErrBusy             = errors.New("jobs: the queue is full")
	ErrNotReady         = errors.New("jobs: the job is not done")
	ErrNoResult         = errors.New("jobs: the job has no result")
	ErrUnknownTask      = errors.New("jobs: unknown or expired task")
	ErrInvalidResult    = errors.New("jobs: invalid result from worker")
	ErrTooManyJobs      = errors.New("jobs: too many open jobs")
	ErrShuttingDown     = errors.New("jobs: shutting down")
	ErrBatchUnavailable = errors.New("jobs: jobs with more than one file are not available yet")
)

// Limits are the operator limits enforced by the Manager.
type Limits struct {
	MaxUploadBytes  int64
	MaxJobBytes     int64
	MaxFiles        int
	MaxOutputBytes  int64
	MaxInputPixels  int64
	MaxOutputPixels int64
	MaxFrames       int
	// MaxConcurrency is the number of files processed at the same time
	// across all workers.
	MaxConcurrency int
	// MaxQueued is the number of files waiting for a worker; uploads beyond
	// it get ErrBusy.
	MaxQueued int
	// MaxJobs bounds the jobs kept in memory.
	MaxJobs int
	// MaxStoredBytes bounds all stored uploads and results together. It
	// must stay below the size of the tmpfs, so that a full disk shows up
	// as ErrBusy (503 with Retry-After) instead of a failed upload.
	MaxStoredBytes int64
	// TTL is the lifetime of a job that is not downloaded.
	TTL time.Duration
	// LeaseTimeout is how long a worker may take for one file before it is
	// failed with a timeout; it covers the job timeout plus transfer time.
	LeaseTimeout time.Duration
}

// Job and file states (api/openapi.yaml).
const (
	JobOpen       = "open"
	JobProcessing = "processing"
	JobDone       = "done"
	JobCanceled   = "canceled"

	FileQueued     = "queued"
	FileProcessing = "processing"
	FileDone       = "done"
	FileFailed     = "failed"
	FileCanceled   = "canceled"
)

// Manager holds all jobs. It is safe for concurrent use.
type Manager struct {
	// root confines every file operation to dir, so that no blob name can
	// reach a file outside it.
	root *os.Root
	lim  Limits
	now  func() time.Time

	mu       sync.Mutex
	jobs     map[string]*job
	queue    []*file
	leases   map[string]*lease
	inFlight int
	// stored counts the bytes of all blobs; reserved the upper bound of
	// uploads in progress.
	stored   int64
	reserved int64
	wake     chan struct{}
	closed   bool
}

type job struct {
	id          string
	tokenHash   [sha256.Size]byte
	pipeline    json.RawMessage
	expected    int
	created     time.Time
	expires     time.Time
	files       []*file
	pending     int // uploads in progress
	bytes       int64
	canceled    bool
	downloading bool
}

type file struct {
	job      *job
	id       string
	index    int
	name     string // uploaded name, for OutputName only
	status   string
	input    string // blob name in Manager.root
	inBytes  int64
	inFormat string
	output   string // blob name in Manager.root
	out      *jobproto.ImageInfo
	outBytes int64
	errCode  string
	errMsg   string
}

type lease struct {
	file     *file
	deadline time.Time
}

// New returns a Manager that stores files in dir (a tmpfs directory that
// only this process uses; it is created with mode 0700 and emptied).
func New(dir string, lim Limits) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	if err := clearDir(root); err != nil {
		_ = root.Close()
		return nil, err
	}
	return &Manager{
		root:   root,
		lim:    lim,
		now:    time.Now,
		jobs:   map[string]*job{},
		leases: map[string]*lease{},
		wake:   make(chan struct{}),
	}, nil
}

// Limits returns the configured limits.
func (m *Manager) Limits() Limits { return m.lim }

// signal wakes all waiting Claim calls. m.mu must be held.
func (m *Manager) signal() {
	close(m.wake)
	m.wake = make(chan struct{})
}

// Create registers a job for an already validated pipeline and returns it
// with its token. The token is not stored, only its hash.
func (m *Manager) Create(pipeline json.RawMessage, expected int) (Job, string, error) {
	if expected < 1 || expected > m.lim.MaxFiles {
		return Job{}, "", fmt.Errorf("%w: expectedFiles must be between 1 and %d", ErrJobTooLarge, m.lim.MaxFiles)
	}
	if expected > 1 {
		return Job{}, "", ErrBatchUnavailable
	}
	id, token := newID(), newToken()
	now := m.now()
	j := &job{
		id:        id,
		tokenHash: sha256.Sum256([]byte(token)),
		pipeline:  slices.Clone(pipeline),
		expected:  expected,
		created:   now,
		expires:   now.Add(m.lim.TTL),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Job{}, "", ErrShuttingDown
	}
	if len(m.jobs) >= m.lim.MaxJobs {
		return Job{}, "", ErrTooManyJobs
	}
	m.jobs[id] = j
	return j.snapshot(), token, nil
}

// Authorize checks the token of a job in constant time. A missing job and a
// wrong token give different errors; the HTTP layer answers both with 404
// or 401 as the specification says.
func (m *Manager) Authorize(id, token string) error {
	m.mu.Lock()
	j, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return ErrNotFound
	}
	h := sha256.Sum256([]byte(token))
	if token == "" || subtle.ConstantTimeCompare(h[:], j.tokenHash[:]) != 1 {
		return ErrUnauthorized
	}
	return nil
}

// Get returns a snapshot of a job. The caller must have called Authorize.
func (m *Manager) Get(id string) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	return j.snapshot(), nil
}

// AddFile stores an upload and queues it. r must already be limited by
// the caller to about MaxUploadBytes; AddFile enforces the exact limit.
func (m *Manager) AddFile(id, uploadedName string, r io.Reader) (File, error) {
	m.mu.Lock()
	j, ok := m.jobs[id]
	switch {
	case !ok:
		m.mu.Unlock()
		return File{}, ErrNotFound
	case j.canceled || len(j.files)+j.pending >= j.expected:
		m.mu.Unlock()
		return File{}, ErrNotOpen
	case len(m.queue) >= m.lim.MaxQueued,
		m.stored+m.reserved+m.lim.MaxUploadBytes > m.lim.MaxStoredBytes:
		m.mu.Unlock()
		return File{}, ErrBusy
	}
	j.pending++
	m.reserved += m.lim.MaxUploadBytes
	m.mu.Unlock()

	path, n, format, err := m.store(r)

	m.mu.Lock()
	defer m.mu.Unlock()
	j.pending--
	m.reserved -= m.lim.MaxUploadBytes
	if err != nil {
		return File{}, err
	}
	if _, ok := m.jobs[id]; !ok || j.canceled {
		_ = m.root.Remove(path)
		return File{}, ErrNotFound
	}
	if j.bytes+n > m.lim.MaxJobBytes {
		_ = m.root.Remove(path)
		return File{}, ErrJobTooLarge
	}
	f := &file{
		job: j, id: newID(), index: len(j.files), name: uploadedName,
		status: FileQueued, input: path, inBytes: n, inFormat: format,
	}
	j.bytes += n
	m.stored += n
	j.files = append(j.files, f)
	m.queue = append(m.queue, f)
	m.signal()
	return f.snapshot(), nil
}

// store writes an upload to a new blob and sniffs its format.
func (m *Manager) store(r io.Reader) (path string, n int64, format string, err error) {
	head := make([]byte, SniffLen)
	k, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", 0, "", err
	}
	head = head[:k]
	if format = Sniff(head); format == "" {
		return "", 0, "", ErrUnsupported
	}
	path, w, err := m.create()
	if err != nil {
		return "", 0, "", err
	}
	n, err = io.Copy(w, io.MultiReader(bytesReader(head), io.LimitReader(r, m.lim.MaxUploadBytes-int64(k)+1)))
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > m.lim.MaxUploadBytes {
		err = ErrTooLarge
	}
	if err != nil {
		_ = m.root.Remove(path)
		return "", 0, "", err
	}
	return path, n, format, nil
}

// create opens a new blob with a random name, mode 0600, and returns its
// name.
func (m *Manager) create() (string, *os.File, error) {
	name := newID()
	f, err := m.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	return name, f, err
}

// Task is one file handed to a worker.
type Task struct {
	ID      string
	Request jobproto.Request
	// Input is open for reading; the caller closes it.
	Input *os.File
}

// Claim waits until a file is queued and a processing slot is free, and
// leases it to the caller. It returns ctx.Err() when ctx ends first.
func (m *Manager) Claim(ctx context.Context) (Task, error) {
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return Task{}, ErrShuttingDown
		}
		if len(m.queue) > 0 && m.inFlight < m.lim.MaxConcurrency {
			f := m.queue[0]
			m.queue = m.queue[1:]
			in, err := m.root.Open(f.input)
			if err != nil {
				m.failLocked(f, jobproto.CodeInternal, "the upload could not be read")
				m.mu.Unlock()
				continue
			}
			id := newID()
			f.status = FileProcessing
			m.leases[id] = &lease{file: f, deadline: m.now().Add(m.lim.LeaseTimeout)}
			m.inFlight++
			t := Task{ID: id, Input: in, Request: jobproto.Request{
				Op:              jobproto.OpProcess,
				InputBytes:      f.inBytes,
				MaxPixels:       m.lim.MaxInputPixels,
				MaxOutputPixels: m.lim.MaxOutputPixels,
				MaxFrames:       m.lim.MaxFrames,
				Pipeline:        f.job.pipeline,
			}}
			m.mu.Unlock()
			return t, nil
		}
		wake := m.wake
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return Task{}, ctx.Err()
		case <-wake:
		}
	}
}

// Complete records a worker's result for a task. output must deliver
// exactly resp.OutputBytes bytes when resp.Error is nil.
func (m *Manager) Complete(taskID string, resp jobproto.Response, output io.Reader) error {
	m.mu.Lock()
	l, ok := m.leases[taskID]
	if ok {
		delete(m.leases, taskID)
		m.inFlight--
		m.signal()
	}
	m.mu.Unlock()
	if !ok {
		return ErrUnknownTask
	}
	f := l.file

	if resp.Error != nil {
		m.mu.Lock()
		m.failLocked(f, resp.Error.Code, resp.Error.Message)
		m.mu.Unlock()
		return nil
	}
	if err := validResult(resp, m.lim.MaxOutputBytes); err != nil {
		m.mu.Lock()
		m.failLocked(f, jobproto.CodeInternal, "the worker returned an invalid result")
		m.mu.Unlock()
		return err
	}
	path, w, err := m.create()
	if err != nil {
		m.mu.Lock()
		m.failLocked(f, jobproto.CodeInternal, "the result could not be stored")
		m.mu.Unlock()
		return err
	}
	n, err := io.Copy(w, io.LimitReader(output, resp.OutputBytes))
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != resp.OutputBytes {
		err = fmt.Errorf("%w: %d of %d output bytes", ErrInvalidResult, n, resp.OutputBytes)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		_ = m.root.Remove(path)
		m.failLocked(f, jobproto.CodeInternal, "the result could not be stored")
		return err
	}
	if f.status != FileProcessing || m.jobs[f.job.id] != f.job {
		// Canceled, deleted, or timed out in the meantime.
		_ = m.root.Remove(path)
		return nil
	}
	m.removeInput(f)
	f.status = FileDone
	f.output, f.out, f.outBytes = path, resp.Output, n
	m.stored += n
	m.signal()
	return nil
}

func validResult(resp jobproto.Response, maxBytes int64) error {
	o := resp.Output
	switch {
	case o == nil:
		return fmt.Errorf("%w: no output info", ErrInvalidResult)
	case extensions[o.Format] == "":
		return fmt.Errorf("%w: output format %q", ErrInvalidResult, o.Format)
	case o.Width < 1 || o.Height < 1 || o.Pages < 1:
		return fmt.Errorf("%w: output size", ErrInvalidResult)
	case resp.OutputBytes < 1 || resp.OutputBytes > maxBytes:
		return fmt.Errorf("%w: %d output bytes", ErrInvalidResult, resp.OutputBytes)
	}
	return nil
}

// failLocked marks a file failed and drops its input. m.mu must be held.
func (m *Manager) failLocked(f *file, code, msg string) {
	if f.status == FileCanceled || f.status == FileDone {
		return
	}
	m.removeInput(f)
	f.status, f.errCode, f.errMsg = FileFailed, code, msg
	m.signal()
}

func (m *Manager) removeInput(f *file) {
	if f.input != "" {
		_ = m.root.Remove(f.input)
		f.input = ""
		m.stored -= f.inBytes
	}
}

func (m *Manager) removeOutput(f *file) {
	if f.output != "" {
		_ = m.root.Remove(f.output)
		f.output = ""
		m.stored -= f.outBytes
	}
}

// Download is the result of a finished single-file job.
type Download struct {
	File        *os.File
	Size        int64
	Name        string
	ContentType string
}

// TakeDownload hands out the result of a done job once and deletes the job
// from the Manager; the caller streams Download.File and must call Release
// afterwards, which deletes the result.
func (m *Manager) TakeDownload(id string) (Download, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok || j.downloading {
		return Download{}, ErrNotFound
	}
	if j.status() != JobDone {
		return Download{}, ErrNotReady
	}
	f := j.files[0]
	if f.status != FileDone {
		return Download{}, ErrNoResult
	}
	out, err := m.root.Open(f.output)
	if err != nil {
		return Download{}, err
	}
	j.downloading = true
	m.deleteLocked(j)
	// The open descriptor keeps the data readable after the unlink.
	return Download{
		File:        out,
		Size:        f.outBytes,
		Name:        OutputName(f.name, f.out.Format),
		ContentType: ContentType(f.out.Format),
	}, nil
}

// Delete cancels a job and removes all of its data.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return ErrNotFound
	}
	m.deleteLocked(j)
	return nil
}

// deleteLocked removes a job, its queued files, and its blobs. Files a
// worker holds are discarded when the worker reports. m.mu must be held.
func (m *Manager) deleteLocked(j *job) {
	j.canceled = true
	delete(m.jobs, j.id)
	m.queue = slices.DeleteFunc(m.queue, func(f *file) bool { return f.job == j })
	for _, f := range j.files {
		m.removeInput(f)
		m.removeOutput(f)
		if f.status == FileQueued || f.status == FileProcessing {
			f.status = FileCanceled
		}
	}
	m.signal()
}

// GetFile returns a snapshot of one file of a job.
func (m *Manager) GetFile(jobID, fileID string) (File, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, err := m.fileLocked(jobID, fileID)
	if err != nil {
		return File{}, err
	}
	return f.snapshot(), nil
}

// CancelFile stops one file and deletes its data. A file that is already
// finished keeps its status; its result is deleted if it had one, so that
// it is not part of the download.
func (m *Manager) CancelFile(jobID, fileID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, err := m.fileLocked(jobID, fileID)
	if err != nil {
		return err
	}
	m.queue = slices.DeleteFunc(m.queue, func(q *file) bool { return q == f })
	m.removeInput(f)
	m.removeOutput(f)
	if f.status != FileFailed {
		f.status = FileCanceled
	}
	m.signal()
	return nil
}

func (m *Manager) fileLocked(jobID, fileID string) (*file, error) {
	j, ok := m.jobs[jobID]
	if !ok {
		return nil, ErrNotFound
	}
	for _, f := range j.files {
		if f.id == fileID {
			return f, nil
		}
	}
	return nil, ErrNotFound
}

// Changes returns a channel that is closed at the next change of any job.
// Callers take a snapshot after receiving it and call Changes again.
func (m *Manager) Changes() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.wake
}

// Sweep expires jobs past their lifetime and leases past their deadline.
// The API calls it periodically.
func (m *Manager) Sweep() {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, l := range m.leases {
		if now.After(l.deadline) {
			delete(m.leases, id)
			m.inFlight--
			m.failLocked(l.file, jobproto.CodeTimeout, "processing took too long")
		}
	}
	for _, j := range m.jobs {
		if now.After(j.expires) {
			m.deleteLocked(j)
		}
	}
	m.signal()
}

// Run calls Sweep every interval until ctx ends.
func (m *Manager) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Sweep()
		}
	}
}

// Close deletes all jobs and their files and wakes all waiting claims.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for _, j := range m.jobs {
		m.deleteLocked(j)
	}
	m.signal()
	return clearDir(m.root)
}

func clearDir(root *os.Root) error {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := root.Remove(e.Name()); err != nil {
			return err
		}
	}
	return nil
}
