package selfcheck

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintCountsFailures(t *testing.T) {
	var buf bytes.Buffer
	failed := Print(&buf, []Result{
		{Name: "a", OK: true, Info: "fine"},
		{Name: "b", OK: false, Info: "broken"},
	})
	if failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
	out := buf.String()
	for _, want := range []string{"PASS  a", "FAIL  b", "1 of 2 checks passed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestProcStatusReadsOwnProcess(t *testing.T) {
	st := procStatus()
	if st["Name"] == "" {
		t.Skip("/proc not available")
	}
	if _, ok := st["CapEff"]; !ok {
		t.Error("CapEff missing from /proc/self/status")
	}
}
