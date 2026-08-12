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
		&models.EpisodeTriageDecision{},
	))
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
			Items      []services.ConsumptionItem `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Equal(t, models.QueueStateInbox, body.Data.QueueState)
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
