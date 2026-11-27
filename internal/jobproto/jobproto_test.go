package jobproto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestHeaderRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := Request{Op: OpProcess, InputBytes: 42, MaxPixels: 1000, Pipeline: []byte(`{"version":1,"steps":[]}`)}
	if err := WriteHeader(&buf, in); err != nil {
		t.Fatal(err)
	}
	buf.WriteString("payload")
	var out Request
	if err := ReadHeader(&buf, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Errorf("got %+v, want %+v", out, in)
	}
	if buf.String() != "payload" {
		t.Errorf("payload = %q; the header reader consumed too much", buf.String())
	}
}

func frame(body string) *bytes.Buffer {
	var buf bytes.Buffer
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(body))) //nolint:gosec // G115: test bodies are tiny
	buf.Write(size[:])
	buf.WriteString(body)
	return &buf
}

func TestReadHeaderRejectsBadInput(t *testing.T) {
	for name, r := range map[string]*bytes.Buffer{
		"unknown field": frame(`{"op":"inspect","inputBytes":1,"extra":true}`),
		"trailing data": frame(`{"op":"inspect"} {}`),
		"not JSON":      frame(`op=inspect`),
		"empty":         frame(``),
		"truncated":     bytes.NewBuffer([]byte{0, 0, 0, 10, '{'}),
		"too large":     bytes.NewBuffer([]byte{0, 2, 0, 0}),
	} {
		var req Request
		if err := ReadHeader(r, &req); err == nil {
			t.Errorf("%s: ReadHeader succeeded, want an error", name)
		}
	}
}

func TestWriteHeaderRejectsOversizedHeader(t *testing.T) {
	err := WriteHeader(&bytes.Buffer{}, Error{Message: strings.Repeat("x", MaxHeaderBytes)})
	if err == nil {
		t.Fatal("WriteHeader succeeded, want an error")
	}
}

func TestHeaderCarriesLargeWatermark(t *testing.T) {
	// A pipeline with an image watermark at the hard limit (1 MiB as base64).
	pipeline := `{"version":1,"steps":[{"op":"watermark","params":{"image":"` + strings.Repeat("A", 1398104) + `"}}]}`
	req := Request{Op: OpProcess, InputBytes: 1, Pipeline: json.RawMessage(pipeline)}
	var buf bytes.Buffer
	if err := WriteHeader(&buf, req); err != nil {
		t.Fatal(err)
	}
	var got Request
	if err := ReadHeader(&buf, &got); err != nil || len(got.Pipeline) != len(pipeline) {
		t.Fatalf("round trip: %v, %d bytes", err, len(got.Pipeline))
	}
}
