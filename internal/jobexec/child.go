package jobexec

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

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

func handle(req jobproto.Request, input []byte) (jobproto.Response, []byte) {
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
	})
	switch {
	case err == nil:
		return jobproto.Response{Output: &jobproto.ImageInfo{
			Format: out.Format, Width: out.Width, Height: out.Height, Pages: 1,
		}}, out.Data
	case errors.Is(err, imaging.ErrUnsupported):
		return failure(jobproto.CodeUnsupportedFormat, "the file is not an allowed image format"), nil
	case errors.Is(err, imaging.ErrTooManyPixels):
		return failure(jobproto.CodeTooManyPixels, err.Error()), nil
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

func failure(code, message string) jobproto.Response {
	return jobproto.Response{Error: &jobproto.Error{Code: code, Message: message}}
}
