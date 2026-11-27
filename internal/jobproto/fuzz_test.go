package jobproto

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func fuzzFrame(s string) []byte {
	return append(binary.BigEndian.AppendUint32(nil, uint32(len(s))), s...) //nolint:gosec // G115: test seeds are short
}

// FuzzReadHeader feeds arbitrary bytes to the header reader, which parses
// data from the other side of a process or socket boundary.
func FuzzReadHeader(f *testing.F) {
	f.Add(fuzzFrame(`{"op":"process","inputBytes":10,"maxPixels":100,"pipeline":{"version":1,"steps":[]}}`))
	f.Add(fuzzFrame(`{"output":{"format":"png","width":1,"height":1,"pages":1},"outputBytes":5}`))
	f.Add(fuzzFrame(`{"error":{"code":"decode_failed","message":"x"},"outputBytes":0}`))
	f.Add(fuzzFrame(`{"op":"x","unknown":1}`))
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		var req Request
		if err := ReadHeader(bytes.NewReader(data), &req); err == nil {
			roundTrip(t, req, &Request{})
		}
		var resp Response
		if err := ReadHeader(bytes.NewReader(data), &resp); err == nil {
			roundTrip(t, resp, &Response{})
		}
	})
}

func roundTrip[T any](t *testing.T, v T, out *T) {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteHeader(&buf, v); err != nil {
		// A header that was read can always be written again.
		t.Fatalf("WriteHeader: %v", err)
	}
	if err := ReadHeader(&buf, out); err != nil {
		t.Fatalf("ReadHeader after WriteHeader: %v", err)
	}
	a, _ := jsonOf(v)
	b, _ := jsonOf(*out)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("round trip changed the header:\n%s\n%s", a, b)
	}
}

func jsonOf(v any) (string, error) {
	var buf bytes.Buffer
	err := WriteHeader(&buf, v)
	return buf.String(), err
}
