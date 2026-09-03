package handlers_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
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

func TestProcessingHandlerStartGetListAndCancel(t *testing.T) {
	db, router, episode, canceler := setupProcessingHandler(t)
	body := `{}`

	response := processingRequest(
		router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/episodes/%d/processing-runs", episode.ID),
		body,
	)
	require.Equal(t, http.StatusConflict, response.Code)
	require.Contains(t, response.Body.String(), "EPISODE_NOT_IN_FOCUS")

	focus := models.QueueStateFocus
	now := time.Date(2026, 8, 24, 15, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID:      episode.ID,
		State:          models.TriageStateShortlisted,
		DecidedAt:      now,
		QueueState:     &focus,
		QueueUpdatedAt: &now,
	}).Error)

	response = processingRequest(
		router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/episodes/%d/processing-runs", episode.ID),
		body,
	)
	require.Equal(t, http.StatusCreated, response.Code)
	var started struct {
		Data struct {
			Run models.EpisodeProcessingRun `json:"run"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &started))
	require.NotZero(t, started.Data.Run.ID)
	require.Equal(t, models.ProcessingRunStatusQueued, started.Data.Run.Status)
	require.Equal(t, strings.Repeat("a", 64), started.Data.Run.AudioDigest)
	require.Equal(t, "pipeline-v1", started.Data.Run.PipelineVersion)

	response = processingRequest(
		router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/episodes/%d/processing-runs", episode.ID),
		body,
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"reused_active":true`)

	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/processing-runs/%d", started.Data.Run.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.NotContains(t, response.Body.String(), "root_path")
	require.NotContains(t, response.Body.String(), "state_json")

	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/episodes/%d/processing-runs", episode.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"pipeline_version":"pipeline-v1"`)

	response = processingRequest(
		router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/processing-runs/%d/cancel", started.Data.Run.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"status":"cancelled"`)
	require.Equal(t, []uint{started.Data.Run.ID}, canceler.RunIDs())

	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/processing-runs/%d", started.Data.Run.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"status":"cancelled"`)

	checkpointState := `{"version":1,"phase":"transcript_stored"}`
	checkpointHash := sha256.Sum256([]byte(checkpointState))
	require.NoError(t, db.Create(&models.ProcessingCheckpoint{
		RunID:          started.Data.Run.ID,
		Step:           processing.StepTranscription,
		Adapter:        "feishu-minutes",
		AdapterVersion: "feishu-minutes-cli-v1",
		Status:         processing.ExternalProgressCompleted,
		StateJSON:      checkpointState,
		StateHash:      fmt.Sprintf("%x", checkpointHash[:]),
		CreatedAt:      now,
		UpdatedAt:      now,
	}).Error)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", started.Data.Run.ID).
		Updates(map[string]any{
			"error_code":      "cancelled_external_result_unknown",
			"error_message":   "legacy cancellation warning",
			"error_retryable": false,
			"updated_at":      now,
		}).Error)
	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/processing-runs/%d", started.Data.Run.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"external_result_unresolved":false`)
	require.Contains(t, response.Body.String(), "可重新转写")

	require.NoError(t, db.Model(&models.ProcessingCheckpoint{}).
		Where(
			"run_id = ? AND step = ?",
			started.Data.Run.ID,
			processing.StepTranscription,
		).
		Update("status", processing.ExternalProgressWaiting).Error)
	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/processing-runs/%d", started.Data.Run.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"external_result_unresolved":true`)
	require.Contains(t, response.Body.String(), "确认前不可重新加工")
}

func TestProcessingHandlerRejectsInvalidRequestAndMissingRun(t *testing.T) {
	_, router, episode, _ := setupProcessingHandler(t)
	response := processingRequest(
		router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/episodes/%d/processing-runs", episode.ID),
		`{"audio_digest":"bad","pipeline_version":"pipeline-v1"}`,
	)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "INVALID_PROCESSING_REQUEST")

	response = processingRequest(
		router,
		http.MethodGet,
		"/api/v1/processing-runs/999999",
		"",
	)
	require.Equal(t, http.StatusNotFound, response.Code)
	require.Contains(t, response.Body.String(), "PROCESSING_RUN_NOT_FOUND")
}

func TestProcessingHandlerFailsClosedWithoutAuthoritativeInputResolver(t *testing.T) {
	db, _, episode, _ := setupProcessingHandler(t)
	focus := models.QueueStateFocus
	now := time.Date(2026, 8, 24, 15, 30, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID:      episode.ID,
		State:          models.TriageStateShortlisted,
		DecidedAt:      now,
		QueueState:     &focus,
		QueueUpdatedAt: &now,
	}).Error)

	router := gin.New()
	handler := handlers.NewProcessingHandler(processing.NewService(db), nil)
	router.POST("/api/v1/episodes/:id/processing-runs", handler.Start)
	response := processingRequest(
		router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/episodes/%d/processing-runs", episode.ID),
		`{}`,
	)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "PROCESSING_INPUT_UNAVAILABLE")
}

func TestProcessingHandlerGetsScheduleStatusWithoutExposingInternalState(t *testing.T) {
	db, _, _, _ := setupProcessingHandler(t)
	next := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	handler := handlers.NewProcessingHandler(
		processing.NewService(db),
		nil,
		staticScheduleStatusProvider{status: processing.ScheduleStatus{
			Enabled:   true,
			Cron:      "0 0 9 * * *",
			Timezone:  "Asia/Shanghai",
			BatchSize: 1,
			NextRunAt: &next,
			LatestRun: &processing.ScheduleRunDetail{
				Run: models.ProcessingScheduleRun{
					ID:           77,
					Status:       models.ProcessingScheduleRunStatusCompleted,
					StartedCount: 1,
					SkippedCount: 1,
				},
				Items: []models.ProcessingScheduleItem{{
					EpisodeID: 9,
					Outcome:   models.ProcessingScheduleItemOutcomeSkipped,
					Reason:    "audio_not_ready",
				}},
			},
		}},
	)
	router := gin.New()
	router.GET("/api/v1/processing-schedule", handler.GetScheduleStatus)

	response := processingRequest(router, http.MethodGet, "/api/v1/processing-schedule", "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"enabled":true`)
	require.Contains(t, response.Body.String(), `"next_run_at"`)
	require.Contains(t, response.Body.String(), `"audio_not_ready"`)
	require.NotContains(t, response.Body.String(), "trigger_key")
}

func TestProcessingHandlerReportsScheduleStatusReadFailure(t *testing.T) {
	db, _, _, _ := setupProcessingHandler(t)
	handler := handlers.NewProcessingHandler(
		processing.NewService(db),
		nil,
		staticScheduleStatusProvider{err: fmt.Errorf("database unavailable")},
	)
	router := gin.New()
	router.GET("/api/v1/processing-schedule", handler.GetScheduleStatus)

	response := processingRequest(router, http.MethodGet, "/api/v1/processing-schedule", "")
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Contains(t, response.Body.String(), "PROCESSING_SCHEDULE_READ_FAILED")
}

func TestProcessingHandlerAudioRetryAndArtifactRoutes(t *testing.T) {
	db, router, episode, _ := setupProcessingHandler(t)
	focus := models.QueueStateFocus
	now := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID:      episode.ID,
		State:          models.TriageStateShortlisted,
		DecidedAt:      now,
		QueueState:     &focus,
		QueueUpdatedAt: &now,
	}).Error)
	require.NoError(t, db.Create(&models.EpisodeAudioAsset{
		EpisodeID:       episode.ID,
		SourceDigest:    strings.Repeat("d", 64),
		Status:          models.EpisodeAudioAssetStatusReady,
		RelativePath:    "private/episode.mp3",
		SHA256:          strings.Repeat("a", 64),
		SizeBytes:       1024,
		DurationSeconds: 60,
		MediaType:       "audio/mpeg",
		Extension:       ".mp3",
		ClaimToken:      "private-claim-token",
		QueuedAt:        now,
		ReadyAt:         &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error)

	response := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/episodes/%d/audio-assets/latest", episode.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"status":"ready"`)
	require.NotContains(t, response.Body.String(), "private/episode.mp3")
	require.NotContains(t, response.Body.String(), "private-claim-token")
	require.NotContains(t, response.Body.String(), strings.Repeat("d", 64))

	response = processingRequest(
		router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/episodes/%d/processing-runs", episode.ID),
		`{}`,
	)
	require.Equal(t, http.StatusCreated, response.Code)
	var started struct {
		Data struct {
			Run models.EpisodeProcessingRun `json:"run"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &started))

	response = processingRequest(
		router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/processing-runs/%d/cancel", started.Data.Run.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	response = processingRequest(
		router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/processing-runs/%d/retry", started.Data.Run.ID),
		"",
	)
	require.Equal(t, http.StatusCreated, response.Code)
	require.Contains(
		t,
		response.Body.String(),
		fmt.Sprintf(`"previous_run_id":%d`, started.Data.Run.ID),
	)

	artifact := models.EpisodeArtifactSet{
		RunID:            started.Data.Run.ID,
		EpisodeID:        episode.ID,
		PipelineVersion:  "pipeline-v1",
		RootPath:         "/private/artifact/root",
		ManifestPath:     "manifest.json",
		ManifestSHA256:   strings.Repeat("1", 64),
		TranscriptSHA256: strings.Repeat("2", 64),
		NotesSHA256:      strings.Repeat("3", 64),
		IsCurrent:        true,
		CreatedAt:        now,
	}
	require.NoError(t, db.Create(&artifact).Error)

	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/transcript", artifact.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "规范逐字稿")
	require.NotContains(t, response.Body.String(), "/private/artifact/root")

	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/manifest", artifact.ID),
		"",
	)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	require.Contains(t, response.Body.String(), "ARTIFACT_INVALID")
}

func TestProcessingArtifactHTTPContractForNativeAndLegacyArtifacts(t *testing.T) {
	db, _, episode, _ := setupProcessingHandler(t)
	now := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	store, err := processing.NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	audioRoot := filepath.Join(t.TempDir(), "managed-audio")
	audioStore, err := processing.NewDiskAudioStore(db, audioRoot)
	require.NoError(t, err)
	service := processing.NewService(
		db,
		processing.WithArtifactReader(store),
		processing.WithAudioPreparer(audioStore),
	)
	handler := handlers.NewProcessingHandler(service, nil)
	router := gin.New()
	router.GET("/api/v1/processing-runs/:id", handler.Get)
	router.GET("/api/v1/artifact-sets/:id/:kind", handler.GetArtifactContent)
	router.HEAD("/api/v1/artifact-sets/:id/audio", handler.GetArtifactAudio)

	createCompletedRun := func(
		targetEpisodeID uint,
		processingKey string,
		audioDigest string,
		pipelineVersion string,
	) models.EpisodeProcessingRun {
		t.Helper()
		run := models.EpisodeProcessingRun{
			EpisodeID:       targetEpisodeID,
			ProcessingKey:   processingKey,
			AudioDigest:     audioDigest,
			PipelineVersion: pipelineVersion,
			TriggerSource:   models.ProcessingTriggerManual,
			Status:          models.ProcessingRunStatusCompleted,
			CurrentStep:     processing.StepArtifactPublish,
			AttemptCount:    1,
			MaxAttempts:     3,
			RetryDeadlineAt: now.Add(24 * time.Hour),
			FinishedAt:      &now,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		require.NoError(t, db.Create(&run).Error)
		return run
	}
	recordArtifact := func(
		run models.EpisodeProcessingRun,
		published processing.ArtifactPublishResult,
	) models.EpisodeArtifactSet {
		t.Helper()
		artifact := models.EpisodeArtifactSet{
			RunID:                    run.ID,
			EpisodeID:                run.EpisodeID,
			PipelineVersion:          run.PipelineVersion,
			RootPath:                 published.RootPath,
			ManifestPath:             published.ManifestPath,
			ManifestSHA256:           published.ManifestSHA256,
			AudioSHA256:              published.AudioSHA256,
			MinutesSummarySHA256:     published.MinutesSummarySHA256,
			TranscriptSHA256:         published.TranscriptSHA256,
			TranscriptTimelineSHA256: published.TranscriptTimelineSHA256,
			NotesSHA256:              published.NotesSHA256,
			IsCurrent:                true,
			CreatedAt:                now,
		}
		require.NoError(t, db.Create(&artifact).Error)
		return artifact
	}

	require.NoError(t, db.Model(&episode).Updates(map[string]any{
		"medium_url": "https://audio.example/native.mp3",
		"duration":   60,
	}).Error)
	queuedAudio, err := audioStore.Enqueue(context.Background(), episode.ID)
	require.NoError(t, err)
	audioBody := []byte("ID3\x04managed-native-audio")
	audioSum := sha256.Sum256(audioBody)
	audioDigest := fmt.Sprintf("%x", audioSum[:])
	audioRelativePath := filepath.Join("ready", "native.mp3")
	audioPath := filepath.Join(audioRoot, audioRelativePath)
	require.NoError(t, os.MkdirAll(filepath.Dir(audioPath), 0o700))
	require.NoError(t, os.WriteFile(audioPath, audioBody, 0o600))
	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", queuedAudio.Asset.ID).
		Updates(map[string]any{
			"status":           models.EpisodeAudioAssetStatusReady,
			"relative_path":    filepath.ToSlash(audioRelativePath),
			"sha256":           audioDigest,
			"size_bytes":       len(audioBody),
			"duration_seconds": 60,
			"media_type":       "audio/mpeg",
			"extension":        ".mp3",
			"ready_at":         &now,
		}).Error)

	nativeRun := createCompletedRun(
		episode.ID,
		strings.Repeat("1", 64),
		audioDigest,
		processing.NativeMinutesPipelineVersion,
	)
	summary := "# 纪要\n\n- 原生妙记纪要\n"
	transcript := "# 逐字稿\n\n说话人 00:00:00.195\n正文\n"
	segments := []processing.TranscriptSegment{{
		Order: 1, Speaker: "说话人", StartMS: 195, Text: "正文",
	}}
	nativePublished, err := store.Publish(
		context.Background(),
		processing.ArtifactPublishRequest{
			RunID:                nativeRun.ID,
			EpisodeID:            nativeRun.EpisodeID,
			AudioDigest:          audioDigest,
			PipelineVersion:      processing.NativeMinutesPipelineVersion,
			NativeMinutes:        true,
			MinutesSummary:       summary,
			Transcript:           transcript,
			TranscriptSegments:   segments,
			TranscriptionAdapter: "feishu-minutes",
			TranscriptionVersion: "feishu-minutes-cli-v1",
			RawArtifacts: map[string][]byte{
				"minutes-detail.json": []byte(`{"file_token":"SECRET"}`),
			},
			GeneratedAt: now,
		},
	)
	require.NoError(t, err)
	nativeArtifact := recordArtifact(nativeRun, nativePublished)

	response := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/processing-runs/%d", nativeRun.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"minutes_summary":true`)
	require.Contains(t, response.Body.String(), `"transcript":true`)
	require.Contains(t, response.Body.String(), `"structured_timeline":true`)
	require.Contains(t, response.Body.String(), `"matching_audio":true`)
	require.Contains(t, response.Body.String(), `"legacy_episode_notes":false`)
	require.NotContains(t, response.Body.String(), nativePublished.RootPath)
	require.NotContains(t, response.Body.String(), "SECRET")

	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/minutes_summary", nativeArtifact.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"kind":"minutes_summary"`)
	require.Contains(t, response.Body.String(), "原生妙记纪要")
	require.Contains(t, response.Body.String(), nativePublished.MinutesSummarySHA256)
	require.NotContains(t, response.Body.String(), nativePublished.RootPath)
	require.NotContains(t, response.Body.String(), audioDigest)
	require.NotContains(t, response.Body.String(), "SECRET")

	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/transcript", nativeArtifact.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"start_ms":195`)
	require.Contains(t, response.Body.String(), nativePublished.TranscriptTimelineSHA256)
	require.Contains(t, response.Body.String(), `"media_available":true`)
	require.NotContains(t, response.Body.String(), nativePublished.RootPath)
	require.NotContains(t, response.Body.String(), audioDigest)
	require.NotContains(t, response.Body.String(), "SECRET")

	requestArtifactAudio := func(method string, rangeHeader string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(
			method,
			fmt.Sprintf("/api/v1/artifact-sets/%d/audio", nativeArtifact.ID),
			nil,
		)
		if rangeHeader != "" {
			request.Header.Set("Range", rangeHeader)
		}
		router.ServeHTTP(recorder, request)
		return recorder
	}
	assertSafeAudioFailure := func(recorder *httptest.ResponseRecorder) {
		t.Helper()
		visible := recorder.Body.String() + fmt.Sprint(recorder.Header())
		require.NotContains(t, visible, audioRoot)
		require.NotContains(t, visible, audioRelativePath)
		require.NotContains(t, visible, audioDigest)
		require.NotContains(t, visible, "SECRET")
	}

	response = requestArtifactAudio(http.MethodGet, "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, audioBody, response.Body.Bytes())
	require.Equal(t, "audio/mpeg", response.Header().Get("Content-Type"))
	require.Equal(t, "bytes", response.Header().Get("Accept-Ranges"))
	require.Equal(t, fmt.Sprint(len(audioBody)), response.Header().Get("Content-Length"))
	require.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	require.Equal(
		t,
		"same-origin",
		response.Header().Get("Cross-Origin-Resource-Policy"),
	)

	response = requestArtifactAudio(http.MethodHead, "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Empty(t, response.Body.Bytes())
	require.Equal(t, "audio/mpeg", response.Header().Get("Content-Type"))
	require.Equal(t, "bytes", response.Header().Get("Accept-Ranges"))
	require.Equal(t, fmt.Sprint(len(audioBody)), response.Header().Get("Content-Length"))

	rangeCases := []struct {
		name         string
		header       string
		start        int
		endExclusive int
		contentRange string
	}{
		{
			name: "first bytes", header: "bytes=0-2", start: 0, endExclusive: 3,
			contentRange: fmt.Sprintf("bytes 0-2/%d", len(audioBody)),
		},
		{
			name: "middle bytes", header: "bytes=4-10", start: 4, endExclusive: 11,
			contentRange: fmt.Sprintf("bytes 4-10/%d", len(audioBody)),
		},
		{
			name:   "tail bytes",
			header: fmt.Sprintf("bytes=%d-%d", len(audioBody)-4, len(audioBody)-1),
			start:  len(audioBody) - 4, endExclusive: len(audioBody),
			contentRange: fmt.Sprintf(
				"bytes %d-%d/%d",
				len(audioBody)-4,
				len(audioBody)-1,
				len(audioBody),
			),
		},
		{
			name: "open tail", header: "bytes=5-", start: 5, endExclusive: len(audioBody),
			contentRange: fmt.Sprintf("bytes 5-%d/%d", len(audioBody)-1, len(audioBody)),
		},
	}
	for _, testCase := range rangeCases {
		t.Run(testCase.name, func(t *testing.T) {
			rangeResponse := requestArtifactAudio(http.MethodGet, testCase.header)
			require.Equal(t, http.StatusPartialContent, rangeResponse.Code)
			require.Equal(
				t,
				audioBody[testCase.start:testCase.endExclusive],
				rangeResponse.Body.Bytes(),
			)
			require.Equal(t, "bytes", rangeResponse.Header().Get("Accept-Ranges"))
			require.Equal(
				t,
				testCase.contentRange,
				rangeResponse.Header().Get("Content-Range"),
			)
			require.Equal(
				t,
				fmt.Sprint(testCase.endExclusive-testCase.start),
				rangeResponse.Header().Get("Content-Length"),
			)
			require.Equal(t, "audio/mpeg", rangeResponse.Header().Get("Content-Type"))
		})
	}

	response = requestArtifactAudio(
		http.MethodGet,
		fmt.Sprintf("bytes=%d-", len(audioBody)),
	)
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, response.Code)
	require.Equal(
		t,
		fmt.Sprintf("bytes */%d", len(audioBody)),
		response.Header().Get("Content-Range"),
	)
	assertSafeAudioFailure(response)

	response = requestArtifactAudio(http.MethodGet, "bytes=invalid")
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, response.Code)
	assertSafeAudioFailure(response)

	assertAudioAvailability := func(target *gin.Engine, expected bool) {
		t.Helper()
		want := "false"
		if expected {
			want = "true"
		}
		runResponse := processingRequest(
			target,
			http.MethodGet,
			fmt.Sprintf("/api/v1/processing-runs/%d", nativeRun.ID),
			"",
		)
		require.Equal(t, http.StatusOK, runResponse.Code)
		require.Contains(
			t,
			runResponse.Body.String(),
			fmt.Sprintf(`"matching_audio":%s`, want),
		)
		transcriptResponse := processingRequest(
			target,
			http.MethodGet,
			fmt.Sprintf("/api/v1/artifact-sets/%d/transcript", nativeArtifact.ID),
			"",
		)
		require.Equal(t, http.StatusOK, transcriptResponse.Code)
		require.Contains(
			t,
			transcriptResponse.Body.String(),
			fmt.Sprintf(`"media_available":%s`, want),
		)
	}

	noAudioHandler := handlers.NewProcessingHandler(
		processing.NewService(db, processing.WithArtifactReader(store)),
		nil,
	)
	noAudioRouter := gin.New()
	noAudioRouter.GET("/api/v1/processing-runs/:id", noAudioHandler.Get)
	noAudioRouter.GET(
		"/api/v1/artifact-sets/:id/:kind",
		noAudioHandler.GetArtifactContent,
	)
	assertAudioAvailability(noAudioRouter, false)

	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", queuedAudio.Asset.ID).
		Update("sha256", strings.Repeat("c", 64)).Error)
	assertAudioAvailability(router, false)
	response = requestArtifactAudio(http.MethodGet, "")
	require.Equal(t, http.StatusConflict, response.Code)
	require.Contains(t, response.Body.String(), "ARTIFACT_AUDIO_MISMATCH")
	assertSafeAudioFailure(response)

	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", queuedAudio.Asset.ID).
		Update("sha256", audioDigest).Error)
	tamperedAudioBody := bytes.Repeat([]byte("x"), len(audioBody))
	require.NotEqual(t, audioBody, tamperedAudioBody)
	require.NoError(t, os.WriteFile(audioPath, tamperedAudioBody, 0o600))
	tamperedAt := time.Now().Add(time.Second)
	require.NoError(t, os.Chtimes(audioPath, tamperedAt, tamperedAt))
	assertAudioAvailability(router, false)
	response = requestArtifactAudio(http.MethodGet, "")
	require.Equal(t, http.StatusConflict, response.Code)
	require.Contains(t, response.Body.String(), "ARTIFACT_AUDIO_MISMATCH")
	assertSafeAudioFailure(response)

	require.NoError(t, os.WriteFile(audioPath, audioBody, 0o600))
	restoredAt := tamperedAt.Add(time.Second)
	require.NoError(t, os.Chtimes(audioPath, restoredAt, restoredAt))
	assertAudioAvailability(router, true)

	require.NoError(t, os.WriteFile(audioPath, append(audioBody, 'x'), 0o600))
	assertAudioAvailability(router, false)
	response = requestArtifactAudio(http.MethodGet, "")
	require.Equal(t, http.StatusNotFound, response.Code)
	require.Contains(t, response.Body.String(), "ARTIFACT_AUDIO_UNAVAILABLE")
	assertSafeAudioFailure(response)

	require.NoError(t, os.WriteFile(audioPath, audioBody, 0o600))
	assertAudioAvailability(router, true)

	require.NoError(t, db.Model(&models.Episode{}).
		Where("id = ?", episode.ID).
		Update("medium_url", "https://audio.example/changed.mp3").Error)
	assertAudioAvailability(router, true)
	response = requestArtifactAudio(http.MethodGet, "")
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, audioBody, response.Body.Bytes())

	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", queuedAudio.Asset.ID).
		Updates(map[string]any{
			"extension":  "avi",
			"media_type": "video/x-msvideo",
		}).Error)
	assertAudioAvailability(router, false)
	response = requestArtifactAudio(http.MethodGet, "")
	require.Equal(t, http.StatusNotFound, response.Code)
	require.Contains(t, response.Body.String(), "ARTIFACT_AUDIO_UNAVAILABLE")
	assertSafeAudioFailure(response)
	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", queuedAudio.Asset.ID).
		Updates(map[string]any{
			"extension":  "mp3",
			"media_type": "audio/mpeg",
		}).Error)
	assertAudioAvailability(router, true)

	externalPath := filepath.Join(t.TempDir(), "private-external-audio.mp3")
	require.NoError(t, os.WriteFile(externalPath, audioBody, 0o600))
	require.NoError(t, os.Remove(audioPath))
	require.NoError(t, os.Symlink(externalPath, audioPath))
	response = requestArtifactAudio(http.MethodGet, "")
	require.Equal(t, http.StatusNotFound, response.Code)
	assertSafeAudioFailure(response)
	require.NotContains(t, response.Body.String(), externalPath)

	require.NoError(t, os.Remove(audioPath))
	require.NoError(t, os.WriteFile(audioPath, audioBody, 0o600))
	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", queuedAudio.Asset.ID).
		Update("relative_path", "../private-external-audio.mp3").Error)
	response = requestArtifactAudio(http.MethodGet, "")
	require.Equal(t, http.StatusNotFound, response.Code)
	assertSafeAudioFailure(response)
	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", queuedAudio.Asset.ID).
		Update("relative_path", filepath.ToSlash(audioRelativePath)).Error)

	require.NoError(t, os.WriteFile(audioPath, nil, 0o600))
	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", queuedAudio.Asset.ID).
		Update("size_bytes", 0).Error)
	response = requestArtifactAudio(http.MethodGet, "")
	require.Equal(t, http.StatusNotFound, response.Code)
	assertSafeAudioFailure(response)
	require.NoError(t, os.WriteFile(audioPath, audioBody, 0o600))
	require.NoError(t, db.Model(&models.EpisodeAudioAsset{}).
		Where("id = ?", queuedAudio.Asset.ID).
		Update("size_bytes", len(audioBody)).Error)

	require.NoError(t, os.Remove(audioPath))
	assertAudioAvailability(router, false)
	response = requestArtifactAudio(http.MethodGet, "")
	require.Equal(t, http.StatusNotFound, response.Code)
	assertSafeAudioFailure(response)

	legacyEpisode := models.Episode{
		PodcastID: episode.PodcastID,
		Title:     "Legacy artifact episode",
		GUID:      "legacy-artifact-http-contract",
	}
	require.NoError(t, db.Create(&legacyEpisode).Error)
	legacyRun := createCompletedRun(
		legacyEpisode.ID,
		strings.Repeat("2", 64),
		strings.Repeat("d", 64),
		"focus-processing-v1",
	)
	legacyPublished, err := store.Publish(
		context.Background(),
		processing.ArtifactPublishRequest{
			RunID:                legacyRun.ID,
			EpisodeID:            legacyRun.EpisodeID,
			AudioDigest:          legacyRun.AudioDigest,
			PipelineVersion:      legacyRun.PipelineVersion,
			Transcript:           "# Transcript\n\nLegacy transcript\n",
			EpisodeNotes:         "# Legacy notes\n\nOld content\n",
			TranscriptionAdapter: "feishu-minutes",
			TranscriptionVersion: "feishu-minutes-cli-v1",
			RuntimeAdapter:       "codex-runtime",
			RuntimeVersion:       "codex-v1",
			PromptVersion:        "episode-notes-v1",
			GeneratedAt:          now,
		},
	)
	require.NoError(t, err)
	legacyArtifact := recordArtifact(legacyRun, legacyPublished)

	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/processing-runs/%d", legacyRun.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"minutes_summary":false`)
	require.Contains(t, response.Body.String(), `"structured_timeline":false`)
	require.Contains(t, response.Body.String(), `"legacy_episode_notes":true`)

	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/episode_notes", legacyArtifact.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"kind":"episode_notes"`)
	require.Contains(t, response.Body.String(), "Legacy notes")
	response = processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/minutes_summary", legacyArtifact.ID),
		"",
	)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	require.Contains(t, response.Body.String(), "ARTIFACT_INVALID")
}

func TestProcessingRichMinutesHTTPContract(t *testing.T) {
	db, _, episode, _ := setupProcessingHandler(t)
	now := time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC)
	store, err := processing.NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	service := processing.NewService(db, processing.WithArtifactReader(store))
	handler := handlers.NewProcessingHandler(service, nil)
	router := gin.New()
	router.GET("/api/v1/artifact-sets/:id/media/:mediaId", handler.GetArtifactMedia)
	router.GET("/api/v1/artifact-sets/:id/:kind", handler.GetArtifactContent)

	run := models.EpisodeProcessingRun{
		EpisodeID:       episode.ID,
		ProcessingKey:   strings.Repeat("3", 64),
		AudioDigest:     strings.Repeat("4", 64),
		PipelineVersion: processing.NativeMinutesPipelineVersion,
		TriggerSource:   models.ProcessingTriggerManual,
		Status:          models.ProcessingRunStatusCompleted,
		CurrentStep:     processing.StepArtifactPublish,
		AttemptCount:    1,
		MaxAttempts:     3,
		RetryDeadlineAt: now.Add(24 * time.Hour),
		FinishedAt:      &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, db.Create(&run).Error)
	var pngBuf bytes.Buffer
	require.NoError(t, png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 3, 2))))
	preview := pngBuf.Bytes()
	previewHash := fmt.Sprintf("%x", sha256.Sum256(preview))
	published, err := store.Publish(context.Background(), processing.ArtifactPublishRequest{
		RunID:              run.ID,
		EpisodeID:          run.EpisodeID,
		AudioDigest:        run.AudioDigest,
		PipelineVersion:    run.PipelineVersion,
		NativeMinutes:      true,
		MinutesSummary:     "# 纪要\n\n原生总结\n",
		Transcript:         "# 逐字稿\n\n说话人 00:00:01.000\n正文\n",
		TranscriptSegments: []processing.TranscriptSegment{{Order: 1, Speaker: "说话人", StartMS: 1000, Text: "正文"}},
		MinutesEnrichment: processing.MinutesEnrichment{
			Chapters: []processing.MinutesChapter{{
				StartMS: 1000, EndMS: 4000, Title: "开场", Summary: "介绍背景",
			}},
			Keywords:  []string{"AI"},
			Decisions: []string{"采用方案 A"},
			Quotes:    []processing.MinutesQuote{{Quote: "长期主义", Explanation: "节奏说明"}},
			Links:     []processing.MinutesLink{{Title: "指南", URL: "https://example.com/guide"}},
			Whiteboard: &processing.MinutesWhiteboard{
				MediaID:   "whiteboard",
				MediaType: "image/png",
				Width:     3,
				Height:    2,
				SHA256:    previewHash,
				Alt:       "飞书智能纪要画板",
			},
		},
		WhiteboardPreview:    preview,
		TranscriptionAdapter: "feishu-minutes",
		TranscriptionVersion: "feishu-minutes-cli-v1",
		RawArtifacts: map[string][]byte{
			"minutes-detail.json": []byte(`{"minutes":[{"minute_token":"obcn_secret","note_id":"note_secret","artifacts":{"summary":"原生总结"}}]}`),
		},
		GeneratedAt: now,
	})
	require.NoError(t, err)
	artifact := models.EpisodeArtifactSet{
		RunID:                    run.ID,
		EpisodeID:                run.EpisodeID,
		PipelineVersion:          run.PipelineVersion,
		RootPath:                 published.RootPath,
		ManifestPath:             published.ManifestPath,
		ManifestSHA256:           published.ManifestSHA256,
		AudioSHA256:              published.AudioSHA256,
		MinutesSummarySHA256:     published.MinutesSummarySHA256,
		TranscriptSHA256:         published.TranscriptSHA256,
		TranscriptTimelineSHA256: published.TranscriptTimelineSHA256,
		IsCurrent:                true,
		CreatedAt:                now,
	}
	require.NoError(t, db.Create(&artifact).Error)

	assertNoLeak := func(body string) {
		t.Helper()
		for _, secret := range []string{
			published.RootPath, "obcn_secret", "note_secret", "minute_token",
			"note_id", "file_token", "/tmp/", "lark-cli",
		} {
			require.NotContains(t, body, secret)
		}
	}

	response := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/minutes_summary", artifact.ID),
		"",
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"kind":"minutes_summary"`)
	require.Contains(t, response.Body.String(), "原生总结")
	require.Contains(t, response.Body.String(), published.MinutesSummarySHA256)
	require.Contains(t, response.Body.String(), `"title":"开场"`)
	require.Contains(t, response.Body.String(), `"start_ms":1000`)
	require.Contains(t, response.Body.String(), `"keywords":["AI"]`)
	require.Contains(t, response.Body.String(), "采用方案 A")
	require.Contains(t, response.Body.String(), "长期主义")
	require.Contains(t, response.Body.String(), "节奏说明")
	require.Contains(t, response.Body.String(), "https://example.com/guide")
	require.Contains(t, response.Body.String(), `"media_id":"whiteboard"`)
	assertNoLeak(response.Body.String())

	transcript := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/transcript", artifact.ID),
		"",
	)
	require.Equal(t, http.StatusOK, transcript.Code)
	require.Contains(t, transcript.Body.String(), `"kind":"transcript"`)
	require.Contains(t, transcript.Body.String(), `"title":"开场"`)
	require.Contains(t, transcript.Body.String(), `"start_ms":1000`)
	assertNoLeak(transcript.Body.String())

	enrichmentPath := filepath.Join(published.RootPath, "minutes-enrichment.json")
	enrichmentBytes, err := os.ReadFile(enrichmentPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(enrichmentPath, []byte(`{"schema_version":"1.0.0","keywords":["tampered"]}`), 0o600))
	corruptedEnrichment := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/minutes_summary", artifact.ID),
		"",
	)
	require.Equal(t, http.StatusUnprocessableEntity, corruptedEnrichment.Code)
	assertNoLeak(corruptedEnrichment.Body.String())
	require.NoError(t, os.WriteFile(enrichmentPath, enrichmentBytes, 0o600))

	media := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/media/whiteboard", artifact.ID),
		"",
	)
	require.Equal(t, http.StatusOK, media.Code)
	require.Equal(t, "image/png", media.Header().Get("Content-Type"))
	require.Equal(t, preview, media.Body.Bytes())
	require.Equal(t, "nosniff", media.Header().Get("X-Content-Type-Options"))
	assertNoLeak(media.Body.String() + fmt.Sprint(media.Header()))

	mediaPath := filepath.Join(published.RootPath, "media", "whiteboard")
	require.NoError(t, os.WriteFile(mediaPath, []byte("not-an-image"), 0o600))
	tampered := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/media/whiteboard", artifact.ID),
		"",
	)
	require.Equal(t, http.StatusUnprocessableEntity, tampered.Code)
	assertNoLeak(tampered.Body.String())
	require.NoError(t, os.WriteFile(mediaPath, preview, 0o600))

	outside := filepath.Join(t.TempDir(), "escaped.png")
	require.NoError(t, os.WriteFile(outside, preview, 0o600))
	require.NoError(t, os.Remove(mediaPath))
	require.NoError(t, os.Symlink(outside, mediaPath))
	symlinked := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/media/whiteboard", artifact.ID),
		"",
	)
	require.Equal(t, http.StatusUnprocessableEntity, symlinked.Code)
	assertNoLeak(symlinked.Body.String())
	require.NotContains(t, symlinked.Body.String(), outside)
	require.NoError(t, os.Remove(mediaPath))
	require.NoError(t, os.WriteFile(mediaPath, preview, 0o600))

	for _, invalid := range []string{
		"unknown",
		"..%2fminutes-summary.md",
		"../raw/minutes-detail.json",
		"minutes-summary.md",
	} {
		denied := processingRequest(
			router,
			http.MethodGet,
			fmt.Sprintf("/api/v1/artifact-sets/%d/media/%s", artifact.ID, invalid),
			"",
		)
		require.NotEqual(t, http.StatusOK, denied.Code, invalid)
		assertNoLeak(denied.Body.String())
	}

	require.NoError(t, db.Model(&artifact).Update("is_current", false).Error)

	coreOnlyRun := models.EpisodeProcessingRun{
		EpisodeID:       episode.ID,
		ProcessingKey:   strings.Repeat("5", 64),
		AudioDigest:     strings.Repeat("6", 64),
		PipelineVersion: processing.NativeMinutesPipelineVersion,
		TriggerSource:   models.ProcessingTriggerManual,
		Status:          models.ProcessingRunStatusCompleted,
		CurrentStep:     processing.StepArtifactPublish,
		AttemptCount:    1,
		MaxAttempts:     3,
		RetryDeadlineAt: now.Add(24 * time.Hour),
		FinishedAt:      &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, db.Create(&coreOnlyRun).Error)
	corePublished, err := store.Publish(context.Background(), processing.ArtifactPublishRequest{
		RunID:                coreOnlyRun.ID,
		EpisodeID:            coreOnlyRun.EpisodeID,
		AudioDigest:          coreOnlyRun.AudioDigest,
		PipelineVersion:      coreOnlyRun.PipelineVersion,
		NativeMinutes:        true,
		MinutesSummary:       "# 纪要\n\n仅核心总结\n",
		Transcript:           "# 逐字稿\n\n说话人 00:00:02.000\n正文\n",
		TranscriptSegments:   []processing.TranscriptSegment{{Order: 1, Speaker: "说话人", StartMS: 2000, Text: "正文"}},
		TranscriptionAdapter: "feishu-minutes",
		TranscriptionVersion: "feishu-minutes-cli-v1",
		GeneratedAt:          now,
	})
	require.NoError(t, err)
	coreArtifact := models.EpisodeArtifactSet{
		RunID:                    coreOnlyRun.ID,
		EpisodeID:                coreOnlyRun.EpisodeID,
		PipelineVersion:          coreOnlyRun.PipelineVersion,
		RootPath:                 corePublished.RootPath,
		ManifestPath:             corePublished.ManifestPath,
		ManifestSHA256:           corePublished.ManifestSHA256,
		AudioSHA256:              corePublished.AudioSHA256,
		MinutesSummarySHA256:     corePublished.MinutesSummarySHA256,
		TranscriptSHA256:         corePublished.TranscriptSHA256,
		TranscriptTimelineSHA256: corePublished.TranscriptTimelineSHA256,
		IsCurrent:                true,
		CreatedAt:                now,
	}
	require.NoError(t, db.Create(&coreArtifact).Error)
	coreResponse := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/minutes_summary", coreArtifact.ID),
		"",
	)
	require.Equal(t, http.StatusOK, coreResponse.Code)
	require.Contains(t, coreResponse.Body.String(), "仅核心总结")
	require.Contains(t, coreResponse.Body.String(), corePublished.MinutesSummarySHA256)
	require.NotContains(t, coreResponse.Body.String(), `"chapters"`)
	require.NotContains(t, coreResponse.Body.String(), `"whiteboard"`)
	transcriptResponse := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/transcript", coreArtifact.ID),
		"",
	)
	require.Equal(t, http.StatusOK, transcriptResponse.Code)
	require.Contains(t, transcriptResponse.Body.String(), `"start_ms":2000`)

	require.NoError(t, db.Model(&coreArtifact).Update("is_current", false).Error)

	restoreRun := models.EpisodeProcessingRun{
		EpisodeID:       episode.ID,
		ProcessingKey:   strings.Repeat("7", 64),
		AudioDigest:     strings.Repeat("8", 64),
		PipelineVersion: processing.NativeMinutesPipelineVersion,
		TriggerSource:   models.ProcessingTriggerManual,
		Status:          models.ProcessingRunStatusCompleted,
		CurrentStep:     processing.StepArtifactPublish,
		AttemptCount:    1,
		MaxAttempts:     3,
		RetryDeadlineAt: now.Add(24 * time.Hour),
		FinishedAt:      &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, db.Create(&restoreRun).Error)
	restorePublished, err := store.Publish(context.Background(), processing.ArtifactPublishRequest{
		RunID:                restoreRun.ID,
		EpisodeID:            restoreRun.EpisodeID,
		AudioDigest:          restoreRun.AudioDigest,
		PipelineVersion:      restoreRun.PipelineVersion,
		NativeMinutes:        true,
		MinutesSummary:       "# 纪要\n\n可恢复总结\n",
		Transcript:           "# 逐字稿\n\n说话人 00:00:03.000\n正文\n",
		TranscriptSegments:   []processing.TranscriptSegment{{Order: 1, Speaker: "说话人", StartMS: 3000, Text: "正文"}},
		TranscriptionAdapter: "feishu-minutes",
		TranscriptionVersion: "feishu-minutes-cli-v1",
		RawArtifacts: map[string][]byte{
			"minutes-detail.json": []byte(`{"minutes":[{"minute_token":"obcn_restore","artifacts":{"summary":"可恢复总结","chapters":[{"start_time":3000,"title":"恢复章节","summary":"来自原始详情"}],"keywords":["恢复词"],"transcript_file":"detail/transcript.txt"}}]}`),
		},
		GeneratedAt: now,
	})
	require.NoError(t, err)
	restoreArtifact := models.EpisodeArtifactSet{
		RunID:                    restoreRun.ID,
		EpisodeID:                restoreRun.EpisodeID,
		PipelineVersion:          restoreRun.PipelineVersion,
		RootPath:                 restorePublished.RootPath,
		ManifestPath:             restorePublished.ManifestPath,
		ManifestSHA256:           restorePublished.ManifestSHA256,
		AudioSHA256:              restorePublished.AudioSHA256,
		MinutesSummarySHA256:     restorePublished.MinutesSummarySHA256,
		TranscriptSHA256:         restorePublished.TranscriptSHA256,
		TranscriptTimelineSHA256: restorePublished.TranscriptTimelineSHA256,
		IsCurrent:                true,
		CreatedAt:                now,
	}
	require.NoError(t, db.Create(&restoreArtifact).Error)
	restoreResponse := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/minutes_summary", restoreArtifact.ID),
		"",
	)
	require.Equal(t, http.StatusOK, restoreResponse.Code)
	require.Contains(t, restoreResponse.Body.String(), "可恢复总结")
	require.Contains(t, restoreResponse.Body.String(), `"title":"恢复章节"`)
	require.Contains(t, restoreResponse.Body.String(), `"keywords":["恢复词"]`)
	require.NotContains(t, restoreResponse.Body.String(), `"decisions"`)
	require.NotContains(t, restoreResponse.Body.String(), `"whiteboard"`)
	require.NotContains(t, restoreResponse.Body.String(), "obcn_restore")
	require.NotContains(t, restoreResponse.Body.String(), restorePublished.RootPath)

	oversize := processingRequest(
		router,
		http.MethodGet,
		fmt.Sprintf("/api/v1/artifact-sets/%d/media/whiteboard", coreArtifact.ID),
		"",
	)
	require.Equal(t, http.StatusUnprocessableEntity, oversize.Code)
}

func setupProcessingHandler(
	t *testing.T,
) (*gorm.DB, *gin.Engine, models.Episode, *recordingRunCanceler) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "processing-handler.db")
	dsn := fmt.Sprintf("%s?_foreign_keys=on&_busy_timeout=5000", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.ApplyMigrations(db))

	podcast := models.Podcast{
		Title: "Processing handler", FeedURL: "https://example.com/handler.xml",
		XYZID: "processing-handler", IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID, Title: "Handler episode",
		GUID: "processing-handler-episode",
	}
	require.NoError(t, db.Create(&episode).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	service := processing.NewService(
		db,
		processing.WithProcessingInputResolver(handlerProcessingInputResolver{}),
		processing.WithArtifactReader(handlerArtifactReader{}),
	)
	canceler := &recordingRunCanceler{service: service}
	handler := handlers.NewProcessingHandler(service, canceler)
	router.POST("/api/v1/episodes/:id/processing-runs", handler.Start)
	router.GET("/api/v1/episodes/:id/processing-runs", handler.ListEpisodeRuns)
	router.GET("/api/v1/episodes/:id/audio-assets/latest", handler.GetLatestAudio)
	router.GET("/api/v1/processing-runs/:id", handler.Get)
	router.POST("/api/v1/processing-runs/:id/cancel", handler.Cancel)
	router.POST("/api/v1/processing-runs/:id/retry", handler.Retry)
	router.GET("/api/v1/artifact-sets/:id/:kind", handler.GetArtifactContent)
	router.POST("/api/v1/artifact-sets/:id/audio/recovery", handler.RecoverArtifactAudio)
	router.HEAD("/api/v1/artifact-sets/:id/audio", handler.GetArtifactAudio)
	return db, router, episode, canceler
}

func processingRequest(
	router *gin.Engine,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

type handlerProcessingInputResolver struct{}

func (handlerProcessingInputResolver) PipelineVersion() string {
	return "pipeline-v1"
}

func (handlerProcessingInputResolver) ResolveProcessingInput(
	_ context.Context,
	_ uint,
) (processing.ProcessingInput, error) {
	return processing.ProcessingInput{
		AudioDigest:     strings.Repeat("a", 64),
		PipelineVersion: "pipeline-v1",
	}, nil
}

type handlerArtifactReader struct{}

func (handlerArtifactReader) ReadText(
	_ context.Context,
	_ models.EpisodeArtifactSet,
	kind string,
) (processing.ArtifactContent, error) {
	if kind != "transcript" && kind != "minutes_summary" && kind != "episode_notes" {
		return processing.ArtifactContent{}, processing.ErrInvalidArtifact
	}
	return processing.ArtifactContent{
		Kind:    kind,
		Content: "# 规范逐字稿",
		SHA256:  strings.Repeat("2", 64),
	}, nil
}

type staticScheduleStatusProvider struct {
	status processing.ScheduleStatus
	err    error
}

func (p staticScheduleStatusProvider) Status(context.Context) (processing.ScheduleStatus, error) {
	return p.status, p.err
}

type recordingRunCanceler struct {
	mu      sync.Mutex
	service *processing.Service
	runIDs  []uint
}

func (c *recordingRunCanceler) Cancel(
	ctx context.Context,
	runID uint,
) (models.EpisodeProcessingRun, error) {
	c.mu.Lock()
	c.runIDs = append(c.runIDs, runID)
	c.mu.Unlock()
	return c.service.CancelProcessingRun(ctx, runID)
}

func (c *recordingRunCanceler) RunIDs() []uint {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]uint(nil), c.runIDs...)
}
