package transport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/benerages/pixsheaf/internal/jobproto"
	"github.com/benerages/pixsheaf/internal/jobs"
)

// Internal task endpoints (ADR 0007, pull model). A worker claims a task
// with a long poll and receives a jobproto.Request header followed by the
// input bytes. It reports a jobproto.Response header followed by the
// output bytes. Task IDs are random and unrelated to public job and file
// IDs.
const (
	claimPath  = "/internal/v1/tasks/claim"
	resultPath = "/internal/v1/tasks/"
	taskHeader = "Pixsheaf-Task-Id"
	// ClaimWait is how long a claim waits for work before it returns 204.
	ClaimWait = 25 * time.Second
)

// TaskSource hands out and completes tasks (implemented by jobs.Manager).
type TaskSource interface {
	Claim(ctx context.Context) (jobs.Task, error)
	Complete(taskID string, resp jobproto.Response, output io.Reader) error
	Limits() jobs.Limits
}

func claimHandler(tasks TaskSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), ClaimWait)
		defer cancel()
		task, err := tasks.Claim(ctx)
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			w.WriteHeader(http.StatusNoContent)
			return
		case err != nil:
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		defer func() { _ = task.Input.Close() }()
		w.Header().Set(taskHeader, task.ID)
		w.Header().Set("Content-Type", "application/octet-stream")
		if err := jobproto.WriteHeader(w, task.Request); err != nil {
			slog.Warn("sending task", "err", err)
			return
		}
		if _, err := io.CopyN(w, task.Input, task.Request.InputBytes); err != nil {
			// The lease expires and the file fails with a timeout.
			slog.Warn("sending task input", "err", err)
		}
	})
}

func resultHandler(tasks TaskSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lim := tasks.Limits()
		// An archive's output is all of its entries (ADR 0022).
		limit := int64(4+jobproto.MaxHeaderBytes) + max(lim.MaxOutputBytes, lim.MaxJobBytes)
		body := http.MaxBytesReader(w, r.Body, limit)
		var resp jobproto.Response
		if err := jobproto.ReadHeader(body, &resp); err != nil {
			http.Error(w, "bad header", http.StatusBadRequest)
			return
		}
		err := tasks.Complete(r.PathValue("id"), resp, body)
		switch {
		case err == nil:
			w.WriteHeader(http.StatusNoContent)
		case errors.Is(err, jobs.ErrUnknownTask):
			http.Error(w, "unknown task", http.StatusNotFound)
		case errors.Is(err, jobs.ErrInvalidResult):
			slog.Warn("invalid result from worker", "err", err)
			http.Error(w, "invalid result", http.StatusUnprocessableEntity)
		default:
			slog.Error("storing result", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
	})
}

// Task is a claimed task on the worker side.
type Task struct {
	ID      string
	Request jobproto.Request
	Input   []byte
}

// ErrNoTask means that no work arrived during the long poll.
var ErrNoTask = errors.New("transport: no task")

// Claim asks the API for work and waits up to ClaimWait. Inputs larger
// than maxInput bytes are refused.
func Claim(ctx context.Context, c *http.Client, maxInput int64) (Task, error) {
	ctx, cancel := context.WithTimeout(ctx, ClaimWait+10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://pixsheaf"+claimPath, nil)
	if err != nil {
		return Task{}, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return Task{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNoContent:
		return Task{}, ErrNoTask
	default:
		return Task{}, fmt.Errorf("transport: claim: status %d", resp.StatusCode)
	}
	t := Task{ID: resp.Header.Get(taskHeader)}
	if t.ID == "" {
		return Task{}, errors.New("transport: claim without task id")
	}
	if err := jobproto.ReadHeader(resp.Body, &t.Request); err != nil {
		return Task{}, err
	}
	if n := t.Request.InputBytes; n < 1 || n > maxInput {
		return Task{}, fmt.Errorf("transport: task input of %d bytes", n)
	}
	t.Input = make([]byte, t.Request.InputBytes)
	if _, err := io.ReadFull(resp.Body, t.Input); err != nil {
		return Task{}, fmt.Errorf("transport: reading task input: %w", err)
	}
	return t, nil
}

// Report sends the result of a task. resp.OutputBytes is set from output.
func Report(ctx context.Context, c *http.Client, taskID string, resp jobproto.Response, output []byte) error {
	resp.OutputBytes = int64(len(output))
	var header bytes.Buffer
	if err := jobproto.WriteHeader(&header, resp); err != nil {
		return err
	}
	body := io.MultiReader(&header, bytes.NewReader(output))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://pixsheaf"+resultPath+taskID, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(header.Len()) + int64(len(output))
	r, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = r.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1024))
	if r.StatusCode != http.StatusNoContent {
		return fmt.Errorf("transport: report: status %d", r.StatusCode)
	}
	return nil
}
