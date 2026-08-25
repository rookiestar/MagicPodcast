package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	if kind != "transcript" && kind != "episode_notes" {
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
