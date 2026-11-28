//go:build vips

package jobexec

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"testing"
	"time"

	"github.com/benerages/pixsheaf/internal/jobproto"
)

// largePhotoJPEG returns a 45.4 MP JPEG (8256x5504, the size of a Nikon D850
// photo) with a busy pattern, so that encoders do real work.
func largePhotoJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewYCbCr(image.Rect(0, 0, 8256, 5504), image.YCbCrSubsampleRatio420)
	for y := range img.Rect.Dy() {
		row := img.Y[y*img.YStride:]
		for x := range img.Rect.Dx() {
			row[x] = uint8((x ^ y) + x/7) //nolint:gosec // G115: wraps on purpose
		}
	}
	for i := range img.Cb {
		img.Cb[i], img.Cr[i] = uint8(i), uint8(i/3) //nolint:gosec // G115: wraps on purpose
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// limitedRunner runs the test binary as a job process with the given memory
// limit. Unlike testRunner it sets one: these tests run without the race
// detector, inside the worker image.
func limitedRunner(t *testing.T, memoryMB uint64) Runner {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner(2*time.Minute, memoryMB<<20, 256<<20)
	r.Path = exe
	r.Env = append(r.Env, childEnvVar+"=1")
	return r
}

var toWebP = jobproto.Request{
	Op:       jobproto.OpProcess,
	Pipeline: []byte(`{"version":1,"steps":[{"op":"autoOrient"},{"op":"convert","params":{"format":"webp"}}]}`),
}

// TestLargePhotoFitsDefaultMemory is the regression test for 36 MP camera
// photos that failed as WebP with the old default of 384 MB.
func TestLargePhotoFitsDefaultMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("encodes a 45 MP image")
	}
	resp, out, err := limitedRunner(t, 1024).Run(t.Context(), toWebP, largePhotoJPEG(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil || resp.Output == nil || len(out) == 0 {
		t.Fatalf("response = %+v (%d bytes)", resp, len(out))
	}
	if got := *resp.Output; got.Width != 8256 || got.Height != 5504 || got.Format != "webp" {
		t.Errorf("output = %+v, want webp 8256x5504", got)
	}
}

// TestLargePhotoOverMemoryLimit checks that a job that runs out of memory
// says so instead of reporting a broken image or an encoder failure.
func TestLargePhotoOverMemoryLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("encodes a 45 MP image")
	}
	resp, _, err := limitedRunner(t, 256).Run(t.Context(), toWebP, largePhotoJPEG(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Code != jobproto.CodeMemoryLimit {
		t.Fatalf("response = %+v, want %s", resp, jobproto.CodeMemoryLimit)
	}
}
