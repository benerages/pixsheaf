package sandbox

import "testing"

// Apply is irreversible, so it is tested through a job process in package
// jobexec. Here, Probe must report no restrictions for the test process.
func TestProbeWithoutSandbox(t *testing.T) {
	r := Probe()
	if r.SocketDenied || r.ExecDenied || r.FileCreateDenied || r.OutsideReadDenied {
		t.Errorf("Probe reports restrictions in an unrestricted process: %+v", r)
	}
	if r.OpenFilesLimit == 0 {
		t.Errorf("Probe did not read RLIMIT_NOFILE: %+v", r)
	}
}
