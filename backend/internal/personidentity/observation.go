package personidentity

import (
	"context"
	"os"
	"time"

	"magicpodcast/internal/codexruntime"
	"magicpodcast/internal/logger"
)

// Runtime phases reported to the caller while one execution is in flight.
// They describe provider activity only: Runtime readiness is not model work,
// and only actual output proves generation. Heartbeats never advance them.
const (
	RuntimePhaseReady        = "ready"
	RuntimePhaseActive       = "active"
	RuntimePhaseGenerating   = "generating"
	RuntimePhaseReconnecting = "reconnecting"
)

// Observation collects the safe, structured facts of one preparation run:
// correlation identifiers, workload sizes, phase timings, and observed
// connection events. It never records transcript text, names, prompts,
// reasoning content, credentials, or raw provider payloads.
//
// The preparation worker owns both updates and the terminal summary. The
// HTTP goroutine never reads it, including after cancellation or disconnect.
type Observation struct {
	RequestID     string
	EpisodeID     uint
	SourceVersion string

	startedAt time.Time

	// Workload facts.
	SegmentCount int
	SpeakerCount int
	PromptBytes  int
	SchemaBytes  int
	ResultBytes  int

	// Correlation with the Runtime execution.
	ExecutionID    string
	RuntimeVersion string

	// Phase timing markers. A phase that was never entered stays zero and is
	// reported as null, never as zero duration.
	sourceReadAt    time.Time
	reservationAt   time.Time
	inputBuiltAt    time.Time
	runtimeReadyAt  time.Time
	firstProviderAt time.Time
	firstOutputAt   time.Time
	streamEndedAt   time.Time
	decodedAt       time.Time
	saveStartAt     time.Time
	saveEndAt       time.Time
	cancelCleanedAt time.Time

	outputDeltas    int
	outputBytes     int
	reconnectEvents int
	lastProviderAt  time.Time

	firstErrorClass string
	lastErrorClass  string

	lastReportedPhase string
	lastReportedAt    time.Time
	finishedAt        time.Time
	failurePhase      string
	currentPhase      string
	connectionClass   string
}

func NewObservation(requestID string, episodeID uint) *Observation {
	return &Observation{RequestID: requestID, EpisodeID: episodeID, startedAt: time.Now(), currentPhase: "read"}
}

type observationKey struct{}

func WithObservation(ctx context.Context, obs *Observation) context.Context {
	return context.WithValue(ctx, observationKey{}, obs)
}

// ObservationFrom returns the request-scoped collector, or nil when the caller
// (processing jobs, maintenance tools) runs without HTTP request scope.
func ObservationFrom(ctx context.Context) *Observation {
	obs, _ := ctx.Value(observationKey{}).(*Observation)
	return obs
}

func (o *Observation) recordSourceRead() {
	if o != nil && o.sourceReadAt.IsZero() {
		o.sourceReadAt = time.Now()
	}
}

func (o *Observation) recordReservation() {
	if o != nil && o.reservationAt.IsZero() {
		o.reservationAt = time.Now()
		o.currentPhase = "input"
	}
}

func (o *Observation) recordInputBuilt(promptBytes, schemaBytes, segments, speakers int) {
	if o == nil {
		return
	}
	if o.inputBuiltAt.IsZero() {
		o.inputBuiltAt = time.Now()
	}
	o.currentPhase = "runtime_start"
	o.PromptBytes = promptBytes
	o.SchemaBytes = schemaBytes
	o.SegmentCount = segments
	o.SpeakerCount = speakers
}

func (o *Observation) recordExecution(id codexruntime.ExecutionID) {
	if o != nil {
		o.ExecutionID = string(id)
	}
}

func (o *Observation) recordRuntimeVersion(version string) {
	if o != nil && version != "" {
		o.RuntimeVersion = version
	}
}

func (o *Observation) recordSourceVersion(version string) {
	if o != nil && o.SourceVersion == "" && version != "" {
		o.SourceVersion = version
	}
}

func (o *Observation) recordResultBytes(size int) {
	if o != nil {
		o.ResultBytes = size
		o.currentPhase = "decode_validate"
	}
}

func (o *Observation) recordDecoded() {
	if o != nil && o.decodedAt.IsZero() {
		o.decodedAt = time.Now()
	}
}

// recordSaveEnd marks the end of the save phase, reached on success and on
// failure alike so a failed save still reports its duration.
func (o *Observation) recordSaveStart() {
	if o != nil {
		o.saveStartAt = time.Now()
		o.currentPhase = "save"
	}
}

func (o *Observation) recordStreamEnd() {
	if o != nil && o.streamEndedAt.IsZero() {
		o.streamEndedAt = time.Now()
	}
}

// Finish is called by the owning worker, after all execution cleanup.
func (o *Observation) Finish(err error) {
	if o == nil {
		return
	}
	o.finishedAt = time.Now()
	if err != nil {
		o.failurePhase = o.currentPhase
		o.recordErrorClass(ClassifyFailure(err).Code)
	}
}

func (o *Observation) recordSaveEnd() {
	if o != nil && o.saveEndAt.IsZero() {
		o.saveEndAt = time.Now()
	}
}

func (o *Observation) recordCancelCleanup() {
	if o != nil && o.cancelCleanedAt.IsZero() {
		o.cancelCleanedAt = time.Now()
	}
}

func (o *Observation) recordErrorClass(class string) {
	if o == nil || class == "" {
		return
	}
	if o.firstErrorClass == "" {
		o.firstErrorClass = class
	}
	o.lastErrorClass = class
}

// observeRuntimeEvent consumes one provider-neutral Runtime event for phase
// timing and connection observation. Events are bounded upstream; nothing is
// logged per event here.
func (o *Observation) observeRuntimeEvent(ctx context.Context, event codexruntime.Event) {
	if o == nil {
		return
	}
	now := time.Now()
	switch event.Type {
	case codexruntime.EventStarted:
		if o.runtimeReadyAt.IsZero() {
			o.runtimeReadyAt = now
			o.currentPhase = "runtime_wait"
			o.reportRuntime(ctx, RuntimePhaseReady, nil)
		}
	case codexruntime.EventProgress:
		if o.firstProviderAt.IsZero() {
			o.firstProviderAt = now
		}
		o.lastProviderAt = now
		if event.Progress != nil && event.Progress.Category == codexruntime.CategoryConnection {
			// Observed reconnect signals only; never presented as the
			// provider's total attempt count.
			o.reconnectEvents++
			var willRetry *bool
			if value, ok := event.Progress.Metadata["will_retry"]; ok && (value == "true" || value == "false") {
				retry := value == "true"
				willRetry = &retry
			}
			class := event.Progress.Metadata["error_class"]
			switch class {
			case "unauthorized", "authentication_failed":
				o.connectionClass = FailureAuthentication
			case "usage_limit_exceeded", "rate_limit_exceeded", "quota_exceeded":
				o.connectionClass = FailureQuota
			case "http_connection_failed", "response_stream_connection_failed", "response_stream_disconnected", "response_too_many_failed_attempts", "upstream_connection_failed":
				o.connectionClass = FailureConnection
			default:
				o.connectionClass = FailureUnknown
			}
			o.recordErrorClass(o.connectionClass)
			o.reportRuntime(ctx, RuntimePhaseReconnecting, willRetry)
			return
		}
		// Local turn/user-message activity is not evidence that an upstream
		// reconnect has recovered.
		if o.lastReportedPhase == RuntimePhaseReconnecting && (event.Progress == nil || event.Progress.Category == codexruntime.CategoryTurn || event.Progress.Category == codexruntime.CategoryGenericItem) {
			return
		}
		o.reportRuntime(ctx, RuntimePhaseActive, nil)
	case codexruntime.EventOutputDelta:
		if o.firstOutputAt.IsZero() {
			o.firstOutputAt = now
		}
		o.outputDeltas++
		o.outputBytes += len(event.Text)
		o.lastProviderAt = now
		o.reportRuntime(ctx, RuntimePhaseGenerating, nil)
	case codexruntime.EventTerminal:
		if o.streamEndedAt.IsZero() {
			o.streamEndedAt = now
		}
	}
}

// reportRuntime forwards a phase change through the existing preparation
// progress seam. Generating wins over active once real output exists;
// reconnecting interrupts any waiting state; heartbeats never reach here.
func (o *Observation) reportRuntime(ctx context.Context, phase string, willRetry *bool) {
	if o == nil {
		return
	}
	if phase == RuntimePhaseActive && o.lastReportedPhase == RuntimePhaseGenerating {
		phase = RuntimePhaseGenerating
	}
	if phase == RuntimePhaseReady && o.lastReportedPhase != "" {
		return
	}
	// Repeat activity is throttled, not discarded: a long generation must
	// keep the frontend's last-activity timestamp truthful.
	if phase == o.lastReportedPhase && phase != RuntimePhaseReconnecting && time.Since(o.lastReportedAt) < time.Second {
		return
	}
	if phase != o.lastReportedPhase || phase == RuntimePhaseReconnecting {
		logger.WithFields(map[string]any{"event": "person_identity_runtime", "request_id": o.RequestID, "execution_id": o.ExecutionID,
			"phase": phase, "elapsed_ms": time.Since(o.startedAt).Milliseconds(), "classification": o.connectionClass,
			"reconnect_events_observed": o.reconnectEvents}).Info("person identity runtime activity")
	}
	o.lastReportedPhase = phase
	o.lastReportedAt = time.Now()
	var age *int64
	if !o.lastProviderAt.IsZero() {
		value := time.Since(o.lastProviderAt).Milliseconds()
		age = &value
	}
	class := ""
	if phase == RuntimePhaseReconnecting {
		class = o.connectionClass
	}
	reportRuntimeProgress(ctx, RuntimeProgress{Phase: phase, WillRetry: willRetry, Classification: class, LastActivityAgeMS: age})
}

// msBetween reports elapsed milliseconds between two markers as a pointer so
// unreached phases stay null instead of masquerading as instant.
func msBetween(from time.Time, to time.Time) *int64 {
	if from.IsZero() || to.IsZero() || to.Before(from) {
		return nil
	}
	value := to.Sub(from).Milliseconds()
	return &value
}

// Summary returns the safe log fields for one finished request. outcome is
// "completed", "failed", "cancelled", or "client_disconnected"; draftSaved
// distinguishes a persisted draft from an attempt that never reached save.
func (o *Observation) Summary(outcome string, draftSaved *bool) map[string]any {
	if o == nil {
		return map[string]any{"event": "person_identity_summary", "outcome": outcome}
	}
	now := o.finishedAt
	if now.IsZero() {
		now = time.Now()
	}
	fields := map[string]any{
		"event":             "person_identity_summary",
		"request_id":        o.RequestID,
		"episode_id":        o.EpisodeID,
		"outcome":           outcome,
		"segment_count":     o.SegmentCount,
		"speaker_count":     o.SpeakerCount,
		"prompt_bytes":      o.PromptBytes,
		"schema_bytes":      o.SchemaBytes,
		"result_bytes":      o.ResultBytes,
		"output_bytes":      o.outputBytes,
		"total_ms":          now.Sub(o.startedAt).Milliseconds(),
		"release_id":        os.Getenv("MAGICPODCAST_RELEASE_ID"),
		"requested_profile": "runtime_default",
		"resolved_model":    "unknown",
		"resolved_effort":   "unknown",
	}
	if o.failurePhase != "" {
		fields["failure_phase"] = o.failurePhase
	}
	if o.SourceVersion != "" {
		fields["source_version"] = o.SourceVersion
	}
	if o.ExecutionID != "" {
		fields["execution_id"] = o.ExecutionID
	}
	if o.RuntimeVersion != "" {
		fields["runtime_version"] = o.RuntimeVersion
	}
	if o.ResultBytes > 0 {
		fields["result_bytes"] = o.ResultBytes
	}
	if o.outputDeltas > 0 {
		fields["output_deltas"] = o.outputDeltas
	}
	for name, value := range map[string]*int64{
		"source_read_ms":          msBetween(o.startedAt, o.sourceReadAt),
		"reservation_ms":          msBetween(o.sourceReadAt, o.reservationAt),
		"input_build_ms":          msBetween(o.reservationAt, o.inputBuiltAt),
		"runtime_ready_ms":        msBetween(o.inputBuiltAt, o.runtimeReadyAt),
		"first_provider_event_ms": msBetween(o.runtimeReadyAt, o.firstProviderAt),
		"first_output_ms":         msBetween(o.runtimeReadyAt, o.firstOutputAt),
		"runtime_wait_ms":         msBetween(o.runtimeReadyAt, o.streamEndedAt),
		"decode_validate_ms":      msBetween(o.streamEndedAt, o.decodedAt),
		"save_ms":                 msBetween(o.saveStartAt, o.saveEndAt),
		"cancel_cleanup_ms":       msBetween(o.streamEndedAt, o.cancelCleanedAt),
	} {
		if value != nil {
			fields[name] = *value
		}
	}
	if !o.lastProviderAt.IsZero() {
		fields["last_provider_event_age_ms"] = now.Sub(o.lastProviderAt).Milliseconds()
	}
	if o.reconnectEvents > 0 {
		fields["reconnect_events_observed"] = o.reconnectEvents
	}
	if o.firstErrorClass != "" {
		fields["first_error_class"] = o.firstErrorClass
	}
	if o.lastErrorClass != "" {
		fields["last_error_class"] = o.lastErrorClass
	}
	if draftSaved != nil {
		fields["draft_saved"] = *draftSaved
	}
	return fields
}
