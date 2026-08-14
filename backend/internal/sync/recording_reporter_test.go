package sync

import "sync"

type recordingReporter struct {
	mu        sync.Mutex
	infos     []string
	successes []string
	errors    []string
	skips     []string
	summaries []*SyncSummary
}

func (r *recordingReporter) Report(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.infos = append(r.infos, message)
}

func (r *recordingReporter) ReportSuccess(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.successes = append(r.successes, message)
}

func (r *recordingReporter) ReportError(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, message)
}

func (r *recordingReporter) ReportProgress(int, int, string) {}

func (r *recordingReporter) ReportSkip(_ SkipReason, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skips = append(r.skips, message)
}

func (r *recordingReporter) ReportSummary(summary *SyncSummary) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.summaries = append(r.summaries, summary)
}

func (r *recordingReporter) Close() {}

func (r *recordingReporter) snapshot() (skips, successes []string, summaries []*SyncSummary) {
	r.mu.Lock()
	defer r.mu.Unlock()
	skips = append([]string(nil), r.skips...)
	successes = append([]string(nil), r.successes...)
	summaries = append([]*SyncSummary(nil), r.summaries...)
	return skips, successes, summaries
}
