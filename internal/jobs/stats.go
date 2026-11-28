package jobs

import "github.com/benerages/pixsheaf/internal/jobproto"

// Stats is a snapshot of the manager for the metrics endpoint. It holds
// counts only: no IDs, names, or client keys.
type Stats struct {
	Jobs        int
	QueuedFiles int
	InFlight    int
	StoredBytes int64
	// FilesDone counts images processed since the start.
	FilesDone uint64
	// FilesFailed counts failed files per error code since the start.
	FilesFailed map[string]uint64
}

// Stats returns the current counts.
func (m *Manager) Stats() Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	failed := make(map[string]uint64, len(m.filesFailed))
	for code, n := range m.filesFailed {
		failed[code] = n
	}
	return Stats{
		Jobs:        len(m.jobs),
		QueuedFiles: m.queue.len(),
		InFlight:    m.inFlight,
		StoredBytes: m.stored,
		FilesDone:   m.filesDone,
		FilesFailed: failed,
	}
}

// metricCodes are the error codes counted under their own label; anything
// else (a code from a misbehaving worker, for example) counts as "other",
// so that the number of metric series stays bounded.
var metricCodes = map[string]bool{
	jobproto.CodeUnsupportedFormat: true, jobproto.CodeFormatDisabled: true, jobproto.CodeTooManyPixels: true,
	jobproto.CodeTooManyFrames: true, jobproto.CodeDecodeFailed: true,
	jobproto.CodeEncodeFailed: true, jobproto.CodeStepNotApplicable: true,
	jobproto.CodeTargetUnreachable: true, jobproto.CodeTimeout: true,
	jobproto.CodeTooManyFiles: true, jobproto.CodeArchiveInvalid: true,
	jobproto.CodeInternal: true, "too_large": true, "archive_entry_unsupported": true,
}

func metricCode(code string) string {
	if metricCodes[code] {
		return code
	}
	return "other"
}
