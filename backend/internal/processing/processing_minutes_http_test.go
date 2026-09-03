package processing_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/handlers"
	"magicpodcast/internal/models"
	"magicpodcast/internal/processing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type minutesTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *minutesTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *minutesTestClock) Add(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

type scriptedMinutesCall struct {
	args []string
}

type scriptedMinutesStep struct {
	output       []byte
	err          error
	beforeReturn func(string)
}

type scriptedMinutesRunner struct {
	mu    sync.Mutex
	steps []scriptedMinutesStep
	calls []scriptedMinutesCall
}

func (r *scriptedMinutesRunner) Run(_ context.Context, cwd string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, scriptedMinutesCall{args: append([]string(nil), args...)})
	if len(r.steps) == 0 {
		return nil, errors.New("unexpected lark-cli call")
	}
	step := r.steps[0]
	r.steps = r.steps[1:]
	if step.beforeReturn != nil {
		step.beforeReturn(cwd)
	}
	return append([]byte(nil), step.output...), step.err
}

type stubRuntime struct{}

func (stubRuntime) Name() string { return "stub-runtime" }
func (stubRuntime) Execute(context.Context, processing.RuntimeRequest) (processing.RuntimeResult, error) {
	return processing.RuntimeResult{}, errors.New("runtime must not execute")
}
func (stubRuntime) Cancel(context.Context, uint) error { return nil }

type nativeMinutesResolver struct {
	digest string
}

func (r nativeMinutesResolver) PipelineVersion() string {
	return processing.NativeMinutesPipelineVersion
}
func (r nativeMinutesResolver) ResolveProcessingInput(context.Context, uint) (processing.ProcessingInput, error) {
	return processing.ProcessingInput{
		AudioDigest:     r.digest,
		PipelineVersion: processing.NativeMinutesPipelineVersion,
	}, nil
}

type minutesHTTPHarness struct {
	t        *testing.T
	clock    *minutesTestClock
	db       *gorm.DB
	service  *processing.Service
	engine   *processing.Engine
	router   *gin.Engine
	adapter  *processing.FeishuMinutesAdapter
	runner   *scriptedMinutesRunner
	store    *processing.DiskArtifactStore
	workRoot string
	episode  models.Episode
	digest   string
}

func newMinutesHTTPHarness(t *testing.T, steps []scriptedMinutesStep) *minutesHTTPHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	clock := &minutesTestClock{now: time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)}
	path := filepath.Join(t.TempDir(), "minutes-http.db")
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("%s?_foreign_keys=on&_busy_timeout=5000", path)), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.ApplyMigrations(db))

	digest := strings.Repeat("c", 64)
	store, err := processing.NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	resolver := nativeMinutesResolver{digest: digest}
	service := processing.NewService(
		db,
		processing.WithClock(clock.Now),
		processing.WithProcessingInputResolver(resolver),
		processing.WithArtifactReader(store),
	)
	runner := &scriptedMinutesRunner{steps: steps}
	workRoot := t.TempDir()
	adapter, err := processing.NewFeishuMinutesAdapterForTest(
		runner.Run,
		workRoot,
		func(context.Context, uint) (string, string, error) {
			return "", "", errors.New("unused")
		},
		clock.Now,
	)
	require.NoError(t, err)
	engine, err := processing.NewEngine(service, adapter, stubRuntime{}, store, nil)
	require.NoError(t, err)
	handler := handlers.NewProcessingHandler(service, nil)
	router := gin.New()
	router.POST("/api/v1/episodes/:id/processing-runs", handler.Start)
	router.GET("/api/v1/processing-runs/:id", handler.Get)
	router.POST("/api/v1/processing-runs/:id/retry", handler.Retry)
	router.GET("/api/v1/artifact-sets/:id/:kind", handler.GetArtifactContent)

	podcast := models.Podcast{
		Title: "Minutes HTTP", FeedURL: "https://example.com/minutes-http.xml",
		XYZID: "minutes-http-" + t.Name(), IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID, Title: "Minutes HTTP episode",
		GUID: "minutes-http-" + t.Name(), MediumURL: "https://example.com/minutes.mp3",
	}
	require.NoError(t, db.Create(&episode).Error)
	focus := models.QueueStateFocus
	now := clock.Now()
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID: episode.ID, State: models.TriageStateShortlisted,
		DecidedAt: now, QueueState: &focus, QueueUpdatedAt: &now,
	}).Error)

	return &minutesHTTPHarness{
		t: t, clock: clock, db: db, service: service, engine: engine,
		router: router, adapter: adapter, runner: runner, store: store,
		workRoot: workRoot, episode: episode, digest: digest,
	}
}

func (h *minutesHTTPHarness) restartEngine() *processing.Engine {
	h.t.Helper()
	resolver := nativeMinutesResolver{digest: h.digest}
	service := processing.NewService(
		h.db,
		processing.WithClock(h.clock.Now),
		processing.WithProcessingInputResolver(resolver),
		processing.WithArtifactReader(h.store),
	)
	recovered, err := service.RecoverNonTerminalRuns(context.Background(), h.clock.Now())
	require.NoError(h.t, err)
	require.NotEmpty(h.t, recovered.RecoverableRunIDs)
	adapter, err := processing.NewFeishuMinutesAdapterForTest(
		h.runner.Run,
		h.workRoot,
		func(context.Context, uint) (string, string, error) {
			return "", "", errors.New("unused")
		},
		h.clock.Now,
	)
	require.NoError(h.t, err)
	engine, err := processing.NewEngine(service, adapter, stubRuntime{}, h.store, nil)
	require.NoError(h.t, err)
	return engine
}

func (h *minutesHTTPHarness) request(method, path, body string) *httptest.ResponseRecorder {
	h.t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	h.router.ServeHTTP(recorder, request)
	return recorder
}

func (h *minutesHTTPHarness) assertNoLeak(body string) {
	h.t.Helper()
	for _, secret := range []string{
		"obcn_", "docx_", "wbcn_", "boxcn_",
		"minute_token", "note_id", "file_token", "/tmp/", "lark-cli",
	} {
		require.NotContains(h.t, body, secret)
	}
}

func (h *minutesHTTPHarness) start(trigger string) models.EpisodeProcessingRun {
	h.t.Helper()
	if trigger == models.ProcessingTriggerScheduled {
		result, err := h.service.StartEpisodeProcessing(context.Background(), processing.StartRequest{
			EpisodeID:     h.episode.ID,
			TriggerSource: models.ProcessingTriggerScheduled,
		})
		require.NoError(h.t, err)
		return result.Run
	}
	response := h.request(
		http.MethodPost,
		fmt.Sprintf("/api/v1/episodes/%d/processing-runs", h.episode.ID),
		"{}",
	)
	require.Equal(h.t, http.StatusCreated, response.Code)
	h.assertNoLeak(response.Body.String())
	var started struct {
		Data struct {
			Run models.EpisodeProcessingRun `json:"run"`
		} `json:"data"`
	}
	require.NoError(h.t, json.Unmarshal(response.Body.Bytes(), &started))
	return started.Data.Run
}

func (h *minutesHTTPHarness) seedCoreReady(run models.EpisodeProcessingRun, minuteToken string) {
	h.t.Helper()
	state, err := json.Marshal(map[string]any{
		"version":      1,
		"phase":        "minutes_created",
		"audio_digest": h.digest,
		"file_token":   "boxcn_http_123",
		"minute_token": minuteToken,
		"minute_url":   "https://example.feishu.cn/minutes/" + minuteToken,
	})
	require.NoError(h.t, err)
	sum := sha256.Sum256(state)
	require.NoError(h.t, h.db.Create(&models.ProcessingCheckpoint{
		RunID:          run.ID,
		Step:           processing.StepTranscription,
		Adapter:        h.adapter.Name(),
		AdapterVersion: h.adapter.Version(),
		Status:         processing.ExternalProgressWaiting,
		StateJSON:      string(state),
		StateHash:      fmt.Sprintf("%x", sum[:]),
		CreatedAt:      h.clock.Now(),
		UpdatedAt:      h.clock.Now(),
	}).Error)
}

func (h *minutesHTTPHarness) getRun(runID uint) *httptest.ResponseRecorder {
	h.t.Helper()
	response := h.request(http.MethodGet, fmt.Sprintf("/api/v1/processing-runs/%d", runID), "")
	h.assertNoLeak(response.Body.String())
	return response
}

func writeHTTPTranscript(cwd string) {
	_ = os.MkdirAll(filepath.Join(cwd, "detail"), 0o700)
	_ = os.WriteFile(
		filepath.Join(cwd, "detail", "transcript.txt"),
		[]byte("张三 00:00:01.500\n开场\n"),
		0o600,
	)
}

func assertNoMinutesWriteCommands(t *testing.T, calls []scriptedMinutesCall) {
	t.Helper()
	for _, call := range calls {
		if len(call.args) < 2 {
			continue
		}
		resource, action := call.args[0], call.args[1]
		require.NotEqual(t, "+upload", action, call.args)
		require.NotEqual(t, "+create", action, call.args)
		require.NotEqual(t, "drive", resource, call.args)
	}
}

func TestProcessingMinutesCompletenessHTTPContract(t *testing.T) {
	pendingNote := errors.New("note still processing")
	t.Run("late note publishes atomically and exposes transcript chapters", func(t *testing.T) {
		minuteDetail := []byte(`{"minutes":[{"minute_token":"obcn_http_late","note_id":"note_http_late","artifacts":{"summary":"完整纪要","chapters":[{"start_time":1500,"title":"开场","summary":"介绍背景"}],"keywords":["AI"],"transcript_file":"detail/transcript.txt"}}]}`)
		h := newMinutesHTTPHarness(t, []scriptedMinutesStep{
			{output: minuteDetail, beforeReturn: writeHTTPTranscript},
			{err: pendingNote},
			{output: minuteDetail, beforeReturn: writeHTTPTranscript},
			{output: []byte(`{"note_doc_token":"docx_http_late"}`)},
			{output: []byte(`{"data":{"document":{"content":"<h1>总结</h1><p></p><h1>关键决策</h1><ul><li>采用方案 A</li></ul><h1>相关链接</h1><p><a href=\"https://example.com/guide\">minute_token=obcn_sensitive_title</a></p>"}}}`)},
		})
		run := h.start(models.ProcessingTriggerManual)
		h.seedCoreReady(run, "obcn_http_late")

		waiting, err := h.engine.Advance(context.Background(), run.ID)
		require.NoError(t, err)
		require.Equal(t, models.ProcessingRunStatusWaitingExternal, waiting.Status)
		require.Equal(t, processing.StepMinutesEnrichment, waiting.CurrentStep)
		waitingBody := h.getRun(waiting.ID)
		require.Contains(t, waitingBody.Body.String(), `"status":"waiting_external"`)
		require.Contains(t, waitingBody.Body.String(), `"current_step":"minutes_enrichment"`)

		restartedEngine := h.restartEngine()
		completed, err := restartedEngine.Advance(context.Background(), run.ID)
		require.NoError(t, err)
		require.Equal(t, models.ProcessingRunStatusCompleted, completed.Status)
		completedBody := h.getRun(completed.ID)
		require.Contains(t, completedBody.Body.String(), `"status":"completed"`)
		require.Contains(t, completedBody.Body.String(), `"is_current":true`)
		var envelope struct {
			Data processing.RunDetail `json:"data"`
		}
		require.NoError(t, json.Unmarshal(completedBody.Body.Bytes(), &envelope))
		require.NotNil(t, envelope.Data.Artifact)
		summary := h.request(http.MethodGet, fmt.Sprintf("/api/v1/artifact-sets/%d/minutes_summary", envelope.Data.Artifact.ID), "")
		require.Equal(t, http.StatusOK, summary.Code)
		require.Contains(t, summary.Body.String(), "完整纪要")
		require.Contains(t, summary.Body.String(), `"title":"开场"`)
		require.Contains(t, summary.Body.String(), "采用方案 A")
		require.Contains(t, summary.Body.String(), `"title":"https://example.com/guide"`)
		h.assertNoLeak(summary.Body.String())
		transcript := h.request(http.MethodGet, fmt.Sprintf("/api/v1/artifact-sets/%d/transcript", envelope.Data.Artifact.ID), "")
		require.Equal(t, http.StatusOK, transcript.Code)
		require.Contains(t, transcript.Body.String(), `"title":"开场"`)
		require.Contains(t, transcript.Body.String(), `"start_ms":1500`)
		h.assertNoLeak(transcript.Body.String())
		assertNoMinutesWriteCommands(t, h.runner.calls)
	})

	t.Run("malformed chapter or keyword metadata cannot complete", func(t *testing.T) {
		for _, minuteDetail := range []string{
			`{"minutes":[{"minute_token":"obcn_http_malformed","note_id":"note_http_malformed","artifacts":{"summary":"完整纪要","chapters":[{"title":"章节","start_time":{"bad":true}}],"transcript_file":"detail/transcript.txt"}}]}`,
			`{"minutes":[{"minute_token":"obcn_http_malformed","note_id":"note_http_malformed","artifacts":{"summary":"完整纪要","keywords":{"unexpected":"shape"},"transcript_file":"detail/transcript.txt"}}]}`,
		} {
			h := newMinutesHTTPHarness(t, []scriptedMinutesStep{{
				output:       []byte(minuteDetail),
				beforeReturn: writeHTTPTranscript,
			}})
			run := h.start(models.ProcessingTriggerManual)
			h.seedCoreReady(run, "obcn_http_malformed")

			failed, err := h.engine.Advance(context.Background(), run.ID)
			require.NoError(t, err)
			require.Equal(t, models.ProcessingRunStatusFailed, failed.Status)
			require.Equal(t, "minutes_section_unparsed", failed.ErrorCode)
		}
	})

	t.Run("empty optional sections can complete for scheduled runs", func(t *testing.T) {
		h := newMinutesHTTPHarness(t, []scriptedMinutesStep{
			{
				output:       []byte(`{"minutes":[{"minute_token":"obcn_http_empty","note_id":"note_http_empty","artifacts":{"summary":"仅总结","transcript_file":"detail/transcript.txt"}}]}`),
				beforeReturn: writeHTTPTranscript,
			},
			{output: []byte(`{"note_doc_token":"docx_http_empty"}`)},
			{output: []byte(`{"data":{"document":{"content":"<h1>总结</h1><p></p><h1>关键决策</h1><p>暂无</p>"}}}`)},
		})
		run := h.start(models.ProcessingTriggerScheduled)
		h.seedCoreReady(run, "obcn_http_empty")
		completed, err := h.engine.Advance(context.Background(), run.ID)
		require.NoError(t, err)
		require.Equal(t, models.ProcessingRunStatusCompleted, completed.Status)
		body := h.getRun(completed.ID)
		var envelope struct {
			Data processing.RunDetail `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body.Body.Bytes(), &envelope))
		summary := h.request(http.MethodGet, fmt.Sprintf("/api/v1/artifact-sets/%d/minutes_summary", envelope.Data.Artifact.ID), "")
		require.Equal(t, http.StatusOK, summary.Code)
		require.Contains(t, summary.Body.String(), "仅总结")
		require.NotContains(t, summary.Body.String(), `"decisions"`)
		require.NotContains(t, summary.Body.String(), `"whiteboard"`)
	})

	t.Run("unknown template does not replace the current artifact", func(t *testing.T) {
		h := newMinutesHTTPHarness(t, []scriptedMinutesStep{
			{
				output:       []byte(`{"minutes":[{"minute_token":"obcn_http_fail","note_id":"note_http_fail","artifacts":{"summary":"新总结","transcript_file":"detail/transcript.txt"}}]}`),
				beforeReturn: writeHTTPTranscript,
			},
			{output: []byte(`{"note_doc_token":"docx_http_fail"}`)},
			{output: []byte(`{"data":{"document":{"content":"<h1>未知模板</h1><p>不能发布</p>"}}}`)},
		})
		previousRun := models.EpisodeProcessingRun{
			EpisodeID: h.episode.ID, ProcessingKey: strings.Repeat("9", 64),
			AudioDigest: h.digest, PipelineVersion: processing.NativeMinutesPipelineVersion,
			TriggerSource: models.ProcessingTriggerManual, Status: models.ProcessingRunStatusCompleted,
			AttemptCount: 1, MaxAttempts: 3, RetryDeadlineAt: h.clock.Now().Add(24 * time.Hour),
			FinishedAt: ptrTimeHTTP(h.clock.Now()), CreatedAt: h.clock.Now(), UpdatedAt: h.clock.Now(),
		}
		require.NoError(t, h.db.Create(&previousRun).Error)
		previous, err := h.store.Publish(context.Background(), processing.ArtifactPublishRequest{
			RunID: previousRun.ID, EpisodeID: h.episode.ID, AudioDigest: h.digest,
			PipelineVersion: processing.NativeMinutesPipelineVersion, NativeMinutes: true,
			MinutesSummary: "# 纪要\n\n上一成功版本\n",
			Transcript:     "# 逐字稿\n\n说话人 00:00:01.000\n旧正文\n",
			TranscriptSegments: []processing.TranscriptSegment{
				{Order: 1, Speaker: "说话人", StartMS: 1000, Text: "旧正文"},
			},
			TranscriptionAdapter: h.adapter.Name(),
			TranscriptionVersion: h.adapter.Version(),
			GeneratedAt:          h.clock.Now(),
		})
		require.NoError(t, err)
		require.NoError(t, h.db.Create(&models.EpisodeArtifactSet{
			RunID: previousRun.ID, EpisodeID: h.episode.ID,
			PipelineVersion: processing.NativeMinutesPipelineVersion,
			RootPath:        previous.RootPath, ManifestPath: previous.ManifestPath,
			ManifestSHA256: previous.ManifestSHA256, AudioSHA256: previous.AudioSHA256,
			MinutesSummarySHA256:     previous.MinutesSummarySHA256,
			TranscriptSHA256:         previous.TranscriptSHA256,
			TranscriptTimelineSHA256: previous.TranscriptTimelineSHA256,
			IsCurrent:                true, CreatedAt: h.clock.Now(),
		}).Error)

		run := h.start(models.ProcessingTriggerManual)
		h.seedCoreReady(run, "obcn_http_fail")
		failed, err := h.engine.Advance(context.Background(), run.ID)
		require.NoError(t, err)
		require.Equal(t, models.ProcessingRunStatusFailed, failed.Status)
		require.Equal(t, "minutes_template_unrecognized", failed.ErrorCode)
		body := h.getRun(failed.ID)
		require.Contains(t, body.Body.String(), `"status":"failed"`)
		require.Contains(t, body.Body.String(), "minutes_template_unrecognized")
		require.Contains(t, body.Body.String(), "可重新同步同一条妙记")
		var envelope struct {
			Data processing.RunDetail `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body.Body.Bytes(), &envelope))
		require.NotNil(t, envelope.Data.CurrentArtifact)
		require.Equal(t, previous.ManifestSHA256, envelope.Data.CurrentArtifact.ManifestSHA256)
		require.Nil(t, envelope.Data.Artifact)
	})

	t.Run("timeout preserves deadline and retry stays read-only", func(t *testing.T) {
		minuteDetail := []byte(`{"minutes":[{"minute_token":"obcn_http_timeout","note_id":"note_http_timeout","artifacts":{"summary":"完整纪要","transcript_file":"detail/transcript.txt"}}]}`)
		retryDetail := []byte(`{"minutes":[{"minute_token":"obcn_http_timeout","note_id":"note_http_timeout","artifacts":{"summary":"重同步后的完整纪要","transcript_file":"detail/transcript.txt"}}]}`)
		h := newMinutesHTTPHarness(t, []scriptedMinutesStep{
			{output: minuteDetail, beforeReturn: writeHTTPTranscript},
			{err: pendingNote},
			{output: minuteDetail, beforeReturn: writeHTTPTranscript},
			{err: pendingNote},
			{output: retryDetail, beforeReturn: writeHTTPTranscript},
			// An expired window performs one final read so credential failures
			// remain actionable; the retry then reads the Minute again.
			{output: retryDetail, beforeReturn: writeHTTPTranscript},
			{output: []byte(`{"note_doc_token":"docx_http_retry"}`)},
			{output: []byte(`{"data":{"document":{"content":"<h1>总结</h1><h1>关键决策</h1><ul><li>继续推进</li></ul>"}}}`)},
		})
		run := h.start(models.ProcessingTriggerManual)
		h.seedCoreReady(run, "obcn_http_timeout")
		waiting, err := h.engine.Advance(context.Background(), run.ID)
		require.NoError(t, err)
		require.Equal(t, models.ProcessingRunStatusWaitingExternal, waiting.Status)
		var source models.ProcessingCheckpoint
		require.NoError(t, h.db.Where("run_id = ? AND step = ?", waiting.ID, processing.StepTranscription).First(&source).Error)
		var waitingState map[string]any
		require.NoError(t, json.Unmarshal([]byte(source.StateJSON), &waitingState))
		deadline, _ := waitingState["enrichment_deadline_at"].(string)
		require.NotEmpty(t, deadline)

		h.clock.Add(10 * time.Minute)
		stillWaiting, err := h.engine.Advance(context.Background(), run.ID)
		require.NoError(t, err)
		require.Equal(t, models.ProcessingRunStatusWaitingExternal, stillWaiting.Status)
		require.NoError(t, h.db.Where("run_id = ? AND step = ?", stillWaiting.ID, processing.StepTranscription).First(&source).Error)
		require.NoError(t, json.Unmarshal([]byte(source.StateJSON), &waitingState))
		require.Equal(t, deadline, waitingState["enrichment_deadline_at"])

		h.clock.Add(25 * time.Minute)
		timedOut, err := h.engine.Advance(context.Background(), run.ID)
		require.NoError(t, err)
		require.Equal(t, models.ProcessingRunStatusFailed, timedOut.Status)
		require.Equal(t, "minutes_enrichment_timeout", timedOut.ErrorCode)
		failedBody := h.getRun(timedOut.ID)
		require.Contains(t, failedBody.Body.String(), "minutes_enrichment_timeout")
		require.Contains(t, failedBody.Body.String(), "可重新同步同一条妙记")

		retry := h.request(http.MethodPost, fmt.Sprintf("/api/v1/processing-runs/%d/retry", timedOut.ID), "")
		require.Equal(t, http.StatusCreated, retry.Code)
		h.assertNoLeak(retry.Body.String())
		var retryEnvelope struct {
			Data struct {
				Run models.EpisodeProcessingRun `json:"run"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(retry.Body.Bytes(), &retryEnvelope))
		var copied models.ProcessingCheckpoint
		require.NoError(t, h.db.Where("run_id = ? AND step = ?", retryEnvelope.Data.Run.ID, processing.StepTranscription).First(&copied).Error)
		var copiedState map[string]any
		require.NoError(t, json.Unmarshal([]byte(copied.StateJSON), &copiedState))
		require.Equal(t, "minutes_enrichment_waiting", copiedState["phase"])
		require.Empty(t, copiedState["core_ready_at"])
		require.Empty(t, copiedState["enrichment_deadline_at"])
		require.Equal(t, "obcn_http_timeout", copiedState["minute_token"])

		callsBefore := len(h.runner.calls)
		completed, err := h.engine.Advance(context.Background(), retryEnvelope.Data.Run.ID)
		require.NoError(t, err)
		require.Equal(t, models.ProcessingRunStatusCompleted, completed.Status)
		assertNoMinutesWriteCommands(t, h.runner.calls[callsBefore:])
		completedBody := h.getRun(completed.ID)
		require.Contains(t, completedBody.Body.String(), `"status":"completed"`)
		require.Contains(t, completedBody.Body.String(), `"is_current":true`)

		var completedCheckpoint models.ProcessingCheckpoint
		require.NoError(t, h.db.Where(
			"run_id = ? AND step = ?",
			completed.ID,
			processing.StepTranscription,
		).First(&completedCheckpoint).Error)
		require.NoError(t, json.Unmarshal([]byte(completedCheckpoint.StateJSON), &copiedState))
		enrichmentPath, _ := copiedState["enrichment_relative_path"].(string)
		require.Contains(t, enrichmentPath, fmt.Sprintf("run-%d/", completed.ID))
		transcriptPath, _ := copiedState["transcript_relative_path"].(string)
		detailPath, _ := copiedState["detail_relative_path"].(string)
		require.Contains(t, transcriptPath, fmt.Sprintf("run-%d/", completed.ID))
		require.Contains(t, detailPath, fmt.Sprintf("run-%d/", completed.ID))

		callsBeforeReplay := len(h.runner.calls)
		restartedAdapter, adapterErr := processing.NewFeishuMinutesAdapterForTest(
			h.runner.Run,
			h.workRoot,
			func(context.Context, uint) (string, string, error) {
				return "", "", errors.New("unused")
			},
			h.clock.Now,
		)
		require.NoError(t, adapterErr)
		replayed, replayErr := restartedAdapter.Resume(
			context.Background(),
			processing.TranscriptionRequest{
				RunID:           completed.ID,
				EpisodeID:       h.episode.ID,
				AudioDigest:     h.digest,
				PipelineVersion: processing.NativeMinutesPipelineVersion,
				PersistCheckpoint: func(context.Context, string, json.RawMessage) error {
					return nil
				},
			},
			json.RawMessage(completedCheckpoint.StateJSON),
		)
		require.NoError(t, replayErr)
		require.Equal(t, processing.ExternalProgressCompleted, replayed.Status)
		require.Contains(t, replayed.MinutesSummary, "重同步后的完整纪要")
		require.Equal(t, []string{"继续推进"}, replayed.MinutesEnrichment.Decisions)
		require.Equal(t, callsBeforeReplay, len(h.runner.calls), "completed replay must stay local")
	})
}

func ptrTimeHTTP(value time.Time) *time.Time {
	copied := value
	return &copied
}
