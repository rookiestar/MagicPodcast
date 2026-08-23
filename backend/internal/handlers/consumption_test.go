package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"magicpodcast/internal/handlers"
	"magicpodcast/internal/models"
	"magicpodcast/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupConsumptionHandler(t *testing.T) (*gorm.DB, *gin.Engine, models.Podcast) {
	t.Helper()
	dsn := fmt.Sprintf("file:consumption_handler_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.Tag{},
		&models.EpisodeCompletion{},
		&models.EpisodeTriageDecision{},
		&models.ConsumptionQueueOrder{},
	))
	require.NoError(t, db.Create(&[]models.ConsumptionQueueOrder{
		{QueueState: models.QueueStateInbox, Revision: 1},
		{QueueState: models.QueueStateFocus, Revision: 1},
		{QueueState: models.QueueStateSomeday, Revision: 1},
		{QueueState: models.QueueStateDone, Revision: 1},
	}).Error)
	podcast := models.Podcast{
		Title: "Inbox API", FeedURL: "https://example.com/inbox.xml", XYZID: dsn, IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewDiscoveryHandler(
		services.NewDiscoveryService(db),
		services.NewTriageService(db),
	)
	router.GET("/api/v1/consumption/summary", handler.GetQueueSummary)
	router.GET("/api/v1/consumption/queues/:queue", handler.ListQueue)
	router.GET("/api/v1/consumption/episodes/:episodeID", handler.GetConsumptionItem)
	router.PUT("/api/v1/consumption/episodes/:episodeID/queue", handler.PutQueue)
	router.PUT("/api/v1/consumption/episodes/:episodeID/placement", handler.PutPlacement)
	router.POST("/api/v1/consumption/episodes/:episodeID/completion/undo", handler.UndoCompletion)
	router.DELETE("/api/v1/consumption/episodes/:episodeID/queue", handler.DeleteQueue)
	router.PUT("/api/v1/consumption/episodes/:episodeID/dismissed", handler.PutDismissed)
	router.POST("/api/v1/consumption/episodes/:episodeID/read", handler.MarkRead)
	router.POST("/api/v1/consumption/episodes/:episodeID/in-progress", handler.MarkInProgress)
	return db, router, podcast
}

func createConsumptionHandlerEpisode(
	t *testing.T,
	db *gorm.DB,
	podcastID uint,
	title string,
	publishedAt time.Time,
) models.Episode {
	t.Helper()
	episode := models.Episode{
		PodcastID: podcastID, Title: title,
		GUID:          fmt.Sprintf("%s-%d", title, time.Now().UnixNano()),
		PublishedDate: publishedAt, ShowNotes: "<p>完整 Show Notes</p>",
		Link: "https://example.com/episode",
	}
	require.NoError(t, db.Create(&episode).Error)
	return episode
}

func performJSONRequest(
	t *testing.T,
	router *gin.Engine,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestConsumptionHandler_CollectsIntoCrossDayInboxIdempotently(t *testing.T) {
	db, router, podcast := setupConsumptionHandler(t)
	older := createConsumptionHandlerEpisode(
		t, db, podcast.ID, "跨日历史条目", time.Now().UTC().Add(-14*24*time.Hour),
	)
	newer := createConsumptionHandlerEpisode(
		t, db, podcast.ID, "新收集条目", time.Now().UTC(),
	)
	olderQueue := models.QueueStateInbox
	olderAt := time.Now().UTC().Add(-8 * 24 * time.Hour)
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID: older.ID, State: models.TriageStateShortlisted, DecidedAt: olderAt,
		QueueState: &olderQueue, QueueUpdatedAt: &olderAt,
	}).Error)

	for attempt := 0; attempt < 2; attempt++ {
		response := performJSONRequest(
			t,
			router,
			http.MethodPut,
			fmt.Sprintf("/api/v1/consumption/episodes/%d/queue", newer.ID),
			`{"queue_state":"inbox"}`,
		)
		require.Equal(t, http.StatusOK, response.Code)
	}

	var rowCount int64
	require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).
		Where("episode_id = ?", newer.ID).Count(&rowCount).Error)
	require.Equal(t, int64(1), rowCount)

	response := performJSONRequest(t, router, http.MethodGet, "/api/v1/consumption/queues/inbox", "")
	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			QueueState string                     `json:"queue_state"`
			Revision   int64                      `json:"revision"`
			Items      []services.ConsumptionItem `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Equal(t, models.QueueStateInbox, body.Data.QueueState)
	require.Greater(t, body.Data.Revision, int64(1))
	require.Len(t, body.Data.Items, 2)
	require.Equal(t, newer.ID, body.Data.Items[0].EpisodeID)
	require.Equal(t, older.ID, body.Data.Items[1].EpisodeID)

}

func TestConsumptionHandler_FocusLimitReturnsActionableConflict(t *testing.T) {
	db, router, podcast := setupConsumptionHandler(t)
	for index := 0; index < services.FocusSoftLimit; index++ {
		episode := createConsumptionHandlerEpisode(
			t, db, podcast.ID, fmt.Sprintf("Focus %d", index), time.Now().UTC(),
		)
		response := performJSONRequest(
			t, router, http.MethodPut,
			fmt.Sprintf("/api/v1/consumption/episodes/%d/queue", episode.ID),
			`{"queue_state":"focus"}`,
		)
		require.Equal(t, http.StatusOK, response.Code)
	}
	eighth := createConsumptionHandlerEpisode(t, db, podcast.ID, "第八项", time.Now().UTC())
	response := performJSONRequest(
		t, router, http.MethodPut,
		fmt.Sprintf("/api/v1/consumption/episodes/%d/queue", eighth.ID),
		`{"queue_state":"focus"}`,
	)
	require.Equal(t, http.StatusConflict, response.Code)
	require.Contains(t, response.Body.String(), "FOCUS_LIMIT_CONFIRMATION_REQUIRED")
	require.Contains(t, response.Body.String(), `"focus_limit":7`)

	response = performJSONRequest(
		t, router, http.MethodPut,
		fmt.Sprintf("/api/v1/consumption/episodes/%d/queue", eighth.ID),
		`{"queue_state":"focus","acknowledge_focus_limit":true}`,
	)
	require.Equal(t, http.StatusOK, response.Code)
}

func TestConsumptionHandler_PlacesQueueItemsAndRejectsStaleRevision(t *testing.T) {
	db, router, podcast := setupConsumptionHandler(t)
	first := createConsumptionHandlerEpisode(t, db, podcast.ID, "第一项", time.Now().UTC())
	second := createConsumptionHandlerEpisode(t, db, podcast.ID, "第二项", time.Now().UTC())
	for _, episode := range []models.Episode{first, second} {
		response := performJSONRequest(
			t,
			router,
			http.MethodPut,
			fmt.Sprintf("/api/v1/consumption/episodes/%d/queue", episode.ID),
			`{"queue_state":"inbox"}`,
		)
		require.Equal(t, http.StatusOK, response.Code)
	}

	queueResponse := performJSONRequest(t, router, http.MethodGet, "/api/v1/consumption/queues/inbox", "")
	require.Equal(t, http.StatusOK, queueResponse.Code)
	var queueBody struct {
		Success bool `json:"success"`
		Data    struct {
			Revision int64 `json:"revision"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(queueResponse.Body.Bytes(), &queueBody))
	require.True(t, queueBody.Success)

	response := performJSONRequest(
		t,
		router,
		http.MethodPut,
		fmt.Sprintf("/api/v1/consumption/episodes/%d/placement", first.ID),
		fmt.Sprintf(`{"queue_state":"inbox","before_episode_id":%d,"expected_revisions":{"inbox":%d}}`, second.ID, queueBody.Data.Revision),
	)
	require.Equal(t, http.StatusOK, response.Code)
	var placementBody struct {
		Success bool                          `json:"success"`
		Data    services.QueuePlacementResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &placementBody))
	require.True(t, placementBody.Success)
	require.Equal(t, []uint{first.ID, second.ID}, consumptionHandlerItemIDs(placementBody.Data.Queues[models.QueueStateInbox].Items))

	stale := performJSONRequest(
		t,
		router,
		http.MethodPut,
		fmt.Sprintf("/api/v1/consumption/episodes/%d/placement", second.ID),
		fmt.Sprintf(`{"queue_state":"inbox","expected_revisions":{"inbox":%d}}`, queueBody.Data.Revision),
	)
	require.Equal(t, http.StatusConflict, stale.Code)
	require.Contains(t, stale.Body.String(), "QUEUE_ORDER_CONFLICT")
}

func TestConsumptionHandler_CompletesAndPreciselyUndoesThroughHTTP(t *testing.T) {
	db, router, podcast := setupConsumptionHandler(t)
	first := createConsumptionHandlerEpisode(t, db, podcast.ID, "撤销前项", time.Now().UTC())
	completed := createConsumptionHandlerEpisode(t, db, podcast.ID, "可撤销完成", time.Now().UTC())
	for _, episode := range []models.Episode{first, completed} {
		response := performJSONRequest(
			t,
			router,
			http.MethodPut,
			fmt.Sprintf("/api/v1/consumption/episodes/%d/queue", episode.ID),
			`{"queue_state":"inbox"}`,
		)
		require.Equal(t, http.StatusOK, response.Code)
	}

	queueResponse := performJSONRequest(t, router, http.MethodGet, "/api/v1/consumption/queues/inbox", "")
	require.Equal(t, http.StatusOK, queueResponse.Code)
	var queueBody struct {
		Data struct {
			Revision int64 `json:"revision"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(queueResponse.Body.Bytes(), &queueBody))

	completionResponse := performJSONRequest(
		t,
		router,
		http.MethodPut,
		fmt.Sprintf("/api/v1/consumption/episodes/%d/placement", completed.ID),
		fmt.Sprintf(
			`{"queue_state":"done","before_episode_id":%d,"expected_revisions":{"inbox":%d,"done":1}}`,
			first.ID,
			queueBody.Data.Revision,
		),
	)
	require.Equal(t, http.StatusOK, completionResponse.Code)
	var completionBody struct {
		Success bool                          `json:"success"`
		Data    services.QueuePlacementResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(completionResponse.Body.Bytes(), &completionBody))
	require.True(t, completionBody.Success)
	require.NotNil(t, completionBody.Data.CompletionUndo)
	require.NotEmpty(t, completionBody.Data.CompletionUndo.Token)
	require.WithinDuration(
		t,
		time.Now().UTC().Add(services.CompletionUndoWindow),
		completionBody.Data.CompletionUndo.ExpiresAt,
		2*time.Second,
	)
	require.Equal(
		t,
		[]uint{first.ID},
		consumptionHandlerItemIDs(completionBody.Data.Queues[models.QueueStateInbox].Items),
	)
	require.Equal(
		t,
		[]uint{completed.ID},
		consumptionHandlerItemIDs(completionBody.Data.Queues[models.QueueStateDone].Items),
	)

	undoBody, err := json.Marshal(map[string]string{
		"token": completionBody.Data.CompletionUndo.Token,
	})
	require.NoError(t, err)
	undoResponse := performJSONRequest(
		t,
		router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/consumption/episodes/%d/completion/undo", completed.ID),
		string(undoBody),
	)
	require.Equal(t, http.StatusOK, undoResponse.Code)
	var undoResult struct {
		Success bool                          `json:"success"`
		Data    services.QueuePlacementResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(undoResponse.Body.Bytes(), &undoResult))
	require.True(t, undoResult.Success)
	require.Equal(
		t,
		[]uint{completed.ID, first.ID},
		consumptionHandlerItemIDs(undoResult.Data.Queues[models.QueueStateInbox].Items),
	)
	require.Empty(t, undoResult.Data.Queues[models.QueueStateDone].Items)

	var completionCount int64
	require.NoError(t, db.Model(&models.EpisodeCompletion{}).
		Where("episode_id = ?", completed.ID).
		Count(&completionCount).Error)
	require.Zero(t, completionCount)
}

func TestConsumptionHandler_UndoCompletionReturnsExplicitConflict(t *testing.T) {
	db, router, podcast := setupConsumptionHandler(t)
	episode := createConsumptionHandlerEpisode(
		t,
		db,
		podcast.ID,
		"冲突完成项",
		time.Now().UTC(),
	)
	inboxResponse := performJSONRequest(
		t,
		router,
		http.MethodPut,
		fmt.Sprintf("/api/v1/consumption/episodes/%d/queue", episode.ID),
		`{"queue_state":"inbox"}`,
	)
	require.Equal(t, http.StatusOK, inboxResponse.Code)

	completionResponse := performJSONRequest(
		t,
		router,
		http.MethodPut,
		fmt.Sprintf("/api/v1/consumption/episodes/%d/queue", episode.ID),
		`{"queue_state":"done"}`,
	)
	require.Equal(t, http.StatusOK, completionResponse.Code)
	var completionBody struct {
		Data services.ConsumptionItem `json:"data"`
	}
	require.NoError(t, json.Unmarshal(completionResponse.Body.Bytes(), &completionBody))
	require.NotNil(t, completionBody.Data.CompletionUndo)

	reprocessResponse := performJSONRequest(
		t,
		router,
		http.MethodPut,
		fmt.Sprintf("/api/v1/consumption/episodes/%d/queue", episode.ID),
		`{"queue_state":"someday"}`,
	)
	require.Equal(t, http.StatusOK, reprocessResponse.Code)

	undoBody, err := json.Marshal(map[string]string{
		"token": completionBody.Data.CompletionUndo.Token,
	})
	require.NoError(t, err)
	undoResponse := performJSONRequest(
		t,
		router,
		http.MethodPost,
		fmt.Sprintf("/api/v1/consumption/episodes/%d/completion/undo", episode.ID),
		string(undoBody),
	)
	require.Equal(t, http.StatusConflict, undoResponse.Code)
	require.Contains(t, undoResponse.Body.String(), "COMPLETION_UNDO_CONFLICT")

	var decision models.EpisodeTriageDecision
	require.NoError(t, db.Where("episode_id = ?", episode.ID).First(&decision).Error)
	require.NotNil(t, decision.QueueState)
	require.Equal(t, models.QueueStateSomeday, *decision.QueueState)
	var completionCount int64
	require.NoError(t, db.Model(&models.EpisodeCompletion{}).
		Where("episode_id = ?", episode.ID).
		Count(&completionCount).Error)
	require.Equal(t, int64(1), completionCount)
}

func consumptionHandlerItemIDs(items []services.ConsumptionItem) []uint {
	result := make([]uint, 0, len(items))
	for _, item := range items {
		result = append(result, item.EpisodeID)
	}
	return result
}

func TestConsumptionHandler_RejectsInvalidEpisodeAndQueue(t *testing.T) {
	_, router, podcast := setupConsumptionHandler(t)
	_ = podcast
	response := performJSONRequest(
		t, router, http.MethodPut,
		"/api/v1/consumption/episodes/not-a-number/queue",
		`{"queue_state":"inbox"}`,
	)
	require.Equal(t, http.StatusBadRequest, response.Code)

	response = performJSONRequest(
		t, router, http.MethodPut,
		"/api/v1/consumption/episodes/99999/queue",
		`{"queue_state":"inbox"}`,
	)
	require.Equal(t, http.StatusNotFound, response.Code)

	response = performJSONRequest(
		t, router, http.MethodGet,
		"/api/v1/consumption/queues/later",
		"",
	)
	require.Equal(t, http.StatusBadRequest, response.Code)
}
