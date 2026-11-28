package jobexec

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/benerages/pixsheaf/internal/archive"
	"github.com/benerages/pixsheaf/internal/imaging"
	"github.com/benerages/pixsheaf/internal/jobproto"
	"github.com/benerages/pixsheaf/internal/pipeline"
	"github.com/benerages/pixsheaf/internal/sandbox"
)

// maxInputBytes is a hard ceiling for one input, independent of the
// configured upload limit (PIXSHEAF_MAX_UPLOAD_MB allows up to 1 GiB).
const maxInputBytes = 1 << 30

// Child is the main function of a job process ("pixsheaf-worker job").
// args are the arguments after "job". It returns the process exit code.
func Child(args []string, stdin io.Reader, stdout io.Writer) int {
	fs := flag.NewFlagSet("job", flag.ContinueOnError)
	memory := fs.Uint64("memory", 0, "data-segment limit in bytes (0: unchanged)")
	cpu := fs.Uint64("cpu", 0, "CPU-time limit in seconds (0: unchanged)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Writes to regular files fail with EFBIG (RLIMIT_FSIZE is 0) instead
	// of killing the process.
	signal.Ignore(syscall.SIGXFSZ)
	if err := sandbox.Apply(sandbox.Limits{MemoryBytes: *memory, CPUSeconds: *cpu}); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "job:", err)
		_ = jobproto.WriteHeader(stdout, failure(jobproto.CodeInternal, "sandbox setup failed"))
		return 1
	}

	var req jobproto.Request
	if err := jobproto.ReadHeader(stdin, &req); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "job:", err)
		return 1
	}
	if req.InputBytes < 0 || req.InputBytes > maxInputBytes {
		_ = jobproto.WriteHeader(stdout, failure(jobproto.CodeInternal, "invalid input size"))
		return 1
	}
	input := make([]byte, req.InputBytes)
	if _, err := io.ReadFull(stdin, input); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "job: reading input:", err)
		return 1
	}

	if req.Op == jobproto.OpUnzip {
		return unzip(req, input, stdout)
	}
	resp, output := handle(req, input)
	resp.OutputBytes = int64(len(output))
	if err := jobproto.WriteHeader(stdout, resp); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "job: writing response:", err)
		return 1
	}
	if _, err := stdout.Write(output); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "job: writing output:", err)
		return 1
	}
	return 0
}

// maxArchiveRatio bounds real size against compressed size per entry.
const maxArchiveRatio = 100

// unzip checks an archive and streams its accepted entries (ADR 0022).
// Only the archive is held in memory, never the unpacked entries.
func unzip(req jobproto.Request, input []byte, stdout io.Writer) int {
	entries, err := archive.Scan(input, archive.Limits{
		MaxEntries:    req.MaxEntries,
		MaxFiles:      req.MaxFiles,
		MaxEntryBytes: req.MaxEntryBytes,
		MaxTotalBytes: req.MaxTotalBytes,
		MaxRatio:      maxArchiveRatio,
	})
	var resp jobproto.Response
	switch {
	case errors.Is(err, archive.ErrTooManyFiles):
		resp = failure(jobproto.CodeTooManyFiles, err.Error())
	case err != nil:
		resp = failure(jobproto.CodeArchiveInvalid, "the archive could not be read")
	default:
		for _, e := range entries {
			resp.Entries = append(resp.Entries, jobproto.ArchiveEntry{Name: e.Name, Bytes: e.Bytes, Code: e.Skip})
			resp.OutputBytes += e.Bytes
		}
	}
	if err := jobproto.WriteHeader(stdout, resp); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "job: writing response:", err)
		return 1
	}
	if resp.Error != nil {
		return 0
	}
	if err := archive.Write(input, entries, stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "job: writing entries:", err)
		return 1
	}
	return 0
}

func handle(req jobproto.Request, input []byte) (jobproto.Response, []byte) {
	// Before libvips starts, so that the loaders stay blocked.
	imaging.DisableFormats(req.DisabledFormats)
	switch req.Op {
	case jobproto.OpSandboxCheck:
		r := sandbox.Probe()
		return jobproto.Response{Sandbox: &r}, nil
	case jobproto.OpInspect:
		return inspect(req, input), nil
	case jobproto.OpProcess:
		return process(req, input)
	default:
		return failure(jobproto.CodeInternal, fmt.Sprintf("unknown operation %q", req.Op)), nil
	}
}

func inspect(req jobproto.Request, input []byte) jobproto.Response {
	info, err := imaging.Inspect(input)
	switch {
	case errors.Is(err, imaging.ErrFormatDisabled):
		return failure(jobproto.CodeFormatDisabled, "this image format is disabled on this server")
	case errors.Is(err, imaging.ErrUnsupported):
		return failure(jobproto.CodeUnsupportedFormat, "the file is not an allowed image format")
	case errors.Is(err, imaging.ErrDecode):
		return failure(jobproto.CodeDecodeFailed, "the image header could not be read")
	case err != nil:
		return failure(jobproto.CodeInternal, "image inspection failed")
	}
	pixels := int64(info.Width) * int64(info.Height) * int64(info.Pages)
	if req.MaxPixels > 0 && pixels > req.MaxPixels {
		return failure(jobproto.CodeTooManyPixels, fmt.Sprintf("the image has %d pixels; the limit is %d", pixels, req.MaxPixels))
	}
	maxFrames := imaging.MaxFrames
	if req.MaxFrames > 0 && req.MaxFrames < maxFrames {
		maxFrames = req.MaxFrames
	}
	if (info.Format == "gif" || info.Format == "webp") && info.Pages > maxFrames {
		return failure(jobproto.CodeTooManyFrames, fmt.Sprintf("the animation has %d frames; the limit is %d", info.Pages, maxFrames))
	}
	return jobproto.Response{Image: &jobproto.ImageInfo{
		Format: info.Format,
		Width:  info.Width,
		Height: info.Height,
		Pages:  info.Pages,
	}}
}

func process(req jobproto.Request, input []byte) (jobproto.Response, []byte) {
	// Hard limits only; the API has already applied the operator limits.
	p, err := pipeline.Parse(req.Pipeline, pipeline.Limits{})
	if err != nil {
		// The API validates first, so this means a version mismatch.
		return failure(jobproto.CodeInternal, err.Error()), nil
	}
	out, err := imaging.Process(input, p, imaging.ProcessLimits{
		MaxInputPixels:  req.MaxPixels,
		MaxOutputPixels: req.MaxOutputPixels,
		MaxFrames:       req.MaxFrames,
	})
	switch {
	case err == nil:
		return jobproto.Response{Output: &jobproto.ImageInfo{
			Format: out.Format, Width: out.Width, Height: out.Height, Pages: out.Pages,
		}}, out.Data
	case errors.Is(err, imaging.ErrFormatDisabled):
		return failure(jobproto.CodeFormatDisabled, "this image format is disabled on this server"), nil
	case errors.Is(err, imaging.ErrUnsupported):
		return failure(jobproto.CodeUnsupportedFormat, "the file is not an allowed image format"), nil
	case errors.Is(err, imaging.ErrTooManyPixels):
		return failure(jobproto.CodeTooManyPixels, err.Error()), nil
	case errors.Is(err, imaging.ErrTooManyFrames):
		return failure(jobproto.CodeTooManyFrames, err.Error()), nil
	case errors.Is(err, imaging.ErrMemory):
		return failure(jobproto.CodeMemoryLimit, memoryLimitMessage), nil
	case errors.Is(err, imaging.ErrDecode):
		return failure(jobproto.CodeDecodeFailed, "the image could not be decoded"), nil
	case errors.Is(err, imaging.ErrEncode):
		return failure(jobproto.CodeEncodeFailed, "the image could not be encoded"), nil
	case errors.Is(err, imaging.ErrInvalidInput):
		return failure(jobproto.CodeStepNotApplicable, err.Error()), nil
	case errors.Is(err, imaging.ErrTargetUnreachable):
		return failure(jobproto.CodeTargetUnreachable, err.Error()), nil
	case errors.Is(err, imaging.ErrNotImplemented):
		return failure(jobproto.CodeInternal, err.Error()), nil
	default:
		return failure(jobproto.CodeInternal, "image processing failed"), nil
	}
}

// memoryLimitMessage explains CodeMemoryLimit; operators raise the limit
// with PIXSHEAF_JOB_MEMORY_MB.
const memoryLimitMessage = "processing this image needs more memory than the job memory limit allows"

func failure(code, message string) jobproto.Response {
	return jobproto.Response{Error: &jobproto.Error{Code: code, Message: message}}
}
