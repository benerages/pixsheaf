package jobexec

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/benerages/pixsheaf/internal/imaging"
	"github.com/benerages/pixsheaf/internal/jobproto"
)

// childEnvVar makes the test binary act as a job process.
const childEnvVar = "PIXSHEAF_JOBEXEC_TEST_CHILD"

// crashEnvVar makes the test binary act as a job process that dies the way
// GLib does when an allocation fails, or with another message.
const crashEnvVar = "PIXSHEAF_JOBEXEC_TEST_CRASH"

func TestMain(m *testing.M) {
	if msg := os.Getenv(crashEnvVar); msg != "" && len(os.Args) > 1 && os.Args[1] == "job" {
		_, _ = fmt.Fprintln(os.Stderr, msg)
		os.Exit(133)
	}
	if os.Getenv(childEnvVar) == "1" && len(os.Args) > 1 && os.Args[1] == "job" {
		os.Exit(Child(os.Args[2:], os.Stdin, os.Stdout))
	}
	os.Exit(m.Run())
}

// testRunner runs the test binary itself as the job process. No memory
// limit: the race detector reserves large mappings that RLIMIT_DATA counts.
func testRunner(t *testing.T) Runner {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner(20*time.Second, 0, 1<<20)
	r.Path = exe
	r.Env = append(r.Env, childEnvVar+"=1")
	return r
}

func samplePNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSandboxIsEnforcedInJobProcess(t *testing.T) {
	resp, out, err := testRunner(t).Run(t.Context(), jobproto.Request{Op: jobproto.OpSandboxCheck}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil || resp.Sandbox == nil || len(out) != 0 {
		t.Fatalf("unexpected response: %+v, %d output bytes", resp, len(out))
	}
	s := resp.Sandbox
	if !s.SocketDenied || !s.ExecDenied {
		t.Errorf("seccomp filter not effective: %+v", s)
	}
	if s.CoreLimit != 0 || s.FileSizeLimit != 0 || s.OpenFilesLimit != 64 || s.CPULimit != 21 {
		t.Errorf("resource limits not set: %+v", s)
	}
	if s.LandlockABI == 0 {
		t.Log("Landlock is unavailable here; filesystem checks skipped (the container self-check requires it)")
		return
	}
	if !s.FileCreateDenied || !s.OutsideReadDenied {
		t.Errorf("Landlock not effective (ABI %d): %+v", s.LandlockABI, s)
	}
}

func TestInspectInJobProcess(t *testing.T) {
	resp, _, err := testRunner(t).Run(t.Context(), jobproto.Request{Op: jobproto.OpInspect}, samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if imaging.LibraryVersion() == "none" {
		if resp.Error == nil || resp.Error.Code != jobproto.CodeInternal {
			t.Fatalf("without libvips: response = %+v, want an internal error", resp)
		}
		return
	}
	if resp.Error != nil || resp.Image == nil {
		t.Fatalf("response = %+v", resp)
	}
	if got := *resp.Image; got != (jobproto.ImageInfo{Format: "png", Width: 2, Height: 3, Pages: 1}) {
		t.Errorf("image = %+v, want png 2x3, 1 page", got)
	}
}

func TestInspectLimitsAndRejects(t *testing.T) {
	if imaging.LibraryVersion() == "none" {
		t.Skip("needs libvips")
	}
	r := testRunner(t)
	resp, _, err := r.Run(t.Context(), jobproto.Request{Op: jobproto.OpInspect, MaxPixels: 5}, samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Code != jobproto.CodeTooManyPixels {
		t.Errorf("6 pixels with limit 5: response = %+v, want too_many_pixels", resp)
	}
	resp, _, err = r.Run(t.Context(), jobproto.Request{Op: jobproto.OpInspect}, []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Code != jobproto.CodeUnsupportedFormat {
		t.Errorf("SVG: response = %+v, want unsupported_format", resp)
	}
}

func TestUnknownOperation(t *testing.T) {
	resp, _, err := testRunner(t).Run(t.Context(), jobproto.Request{Op: "format-disk"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Code != jobproto.CodeInternal {
		t.Errorf("response = %+v, want an internal error", resp)
	}
}

func TestTimeoutKillsProcessGroup(t *testing.T) {
	// The shell and its background child share the process group; both
	// must be gone for Run to return promptly.
	r := Runner{
		Path:           "/bin/sh",
		Args:           []string{"-c", "sleep 30 & sleep 30"},
		Timeout:        500 * time.Millisecond,
		MaxOutputBytes: 1,
	}
	start := time.Now()
	_, _, err := r.Run(t.Context(), jobproto.Request{Op: jobproto.OpInspect}, nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Run returned after %v; the process group was not killed", d)
	}
}

func TestOversizedOutputIsRejected(t *testing.T) {
	r := testRunner(t)
	r.MaxOutputBytes = -1 // any announced output is too large
	_, _, err := r.Run(t.Context(), jobproto.Request{Op: jobproto.OpSandboxCheck}, nil)
	if err == nil {
		t.Fatal("Run succeeded, want an output size error")
	}
}

func TestProcessInJobProcess(t *testing.T) {
	req := jobproto.Request{
		Op:       jobproto.OpProcess,
		Pipeline: []byte(`{"version":1,"steps":[{"op":"rotate","params":{"angle":90}},{"op":"convert","params":{"format":"webp"}}]}`),
	}
	resp, out, err := testRunner(t).Run(t.Context(), req, samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if imaging.LibraryVersion() == "none" {
		if resp.Error == nil || resp.Error.Code != jobproto.CodeInternal {
			t.Fatalf("without libvips: response = %+v, want an internal error", resp)
		}
		return
	}
	if resp.Error != nil || resp.Output == nil {
		t.Fatalf("response = %+v", resp)
	}
	if got := *resp.Output; got != (jobproto.ImageInfo{Format: "webp", Width: 3, Height: 2, Pages: 1}) {
		t.Errorf("output = %+v, want webp 3x2", got)
	}
	info, err := imaging.Inspect(out)
	if err != nil || info.Format != "webp" || info.Width != 3 || info.Height != 2 {
		t.Errorf("returned bytes: %+v, %v", info, err)
	}
}

func TestProcessRejectsInvalidPipeline(t *testing.T) {
	req := jobproto.Request{Op: jobproto.OpProcess, Pipeline: []byte(`{"version":1,"steps":[{"op":"explode"}]}`)}
	resp, _, err := testRunner(t).Run(t.Context(), req, samplePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Code != jobproto.CodeInternal {
		t.Errorf("response = %+v, want an internal error", resp)
	}
}

func TestCrashAfterFailedAllocationIsMemoryLimit(t *testing.T) {
	r := testRunner(t)
	r.Env = append(r.Env, crashEnvVar+"=GLib-ERROR **: ../glib/gmem.c:177: failed to allocate 67108864 bytes")
	resp, out, err := r.Run(t.Context(), jobproto.Request{Op: jobproto.OpProcess}, samplePNG(t))
	if err != nil {
		t.Fatalf("Run: %v, want a job-level failure", err)
	}
	if resp.Error == nil || resp.Error.Code != jobproto.CodeMemoryLimit || out != nil {
		t.Fatalf("response = %+v, want %s", resp, jobproto.CodeMemoryLimit)
	}
}

func TestCrashWithOtherMessageIsProcessError(t *testing.T) {
	r := testRunner(t)
	r.Env = append(r.Env, crashEnvVar+"=segmentation fault")
	if _, _, err := r.Run(t.Context(), jobproto.Request{Op: jobproto.OpProcess}, samplePNG(t)); err == nil {
		t.Fatal("Run succeeded for a crashed job process")
	}
}
