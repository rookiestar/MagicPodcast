package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"magicpodcast/internal/handlers"
	"magicpodcast/internal/models"
	"magicpodcast/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDiscoveryHandler_ListCandidates_ReturnsRecentLibraryEpisodes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:discovery_handler?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Episode{}, &models.EpisodeTriageDecision{}))

	podcast := models.Podcast{
		Title:        "真实个人库节目",
		Author:       "主播",
		FeedURL:      "https://example.com/feed.xml",
		XYZID:        "discovery-handler-podcast",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)

	episode := models.Episode{
		PodcastID:     podcast.ID,
		Title:         "最近更新单集",
		GUID:          "discovery-handler-episode",
		PublishedDate: time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC),
		Duration:      2700,
	}
	require.NoError(t, db.Create(&episode).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewDiscoveryHandler(
		services.NewDiscoveryService(db),
		services.NewTriageService(db),
	)
	router.GET("/api/v1/discovery/candidates", handler.ListCandidates)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/discovery/candidates?limit=10", nil)
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		Success bool                          `json:"success"`
		Data    []services.DiscoveryCandidate `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data, 1)
	assert.Equal(t, episode.ID, body.Data[0].EpisodeID)
	assert.Equal(t, "真实个人库节目", body.Data[0].PodcastTitle)
	assert.Equal(t, "最近更新", body.Data[0].Source)
	assert.Equal(t, "missing", body.Data[0].ShowNotesStatus)
	require.Len(t, body.Data[0].PreReads, 4)
	assert.Equal(t, services.PreReadStatusMissing, body.Data[0].PreReads[0].Status)
	assert.Equal(t, services.PreReadStatusInsufficient, body.Data[0].PreReads[2].Status)
	assert.NotEmpty(t, body.Data[0].PreReads[0].GeneratedAt)
	assert.NotEmpty(t, body.Data[0].PreReads[0].Version)
}

func TestDiscoveryHandler_ListCandidates_ExcludesUnsubscribedPodcasts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:discovery_handler_subscriptions?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Episode{}, &models.EpisodeTriageDecision{}))

	subscribed := models.Podcast{
		Title:        "已订阅节目",
		FeedURL:      "https://example.com/subscribed.xml",
		XYZID:        "discovery-handler-subscribed-podcast",
		IsSubscribed: true,
	}
	unsubscribed := models.Podcast{
		Title:        "未订阅节目",
		FeedURL:      "https://example.com/unsubscribed.xml",
		XYZID:        "discovery-handler-unsubscribed-podcast",
		IsSubscribed: false,
	}
	require.NoError(t, db.Create(&subscribed).Error)
	require.NoError(t, db.Create(&unsubscribed).Error)
	require.NoError(t, db.Model(&unsubscribed).Update("is_subscribed", false).Error)

	subscribedEpisode := models.Episode{
		PodcastID:     subscribed.ID,
		Title:         "已订阅的最近更新",
		GUID:          "discovery-handler-subscribed-episode",
		PublishedDate: time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC),
	}
	unsubscribedEpisode := models.Episode{
		PodcastID:     unsubscribed.ID,
		Title:         "未订阅的不应出现",
		GUID:          "discovery-handler-unsubscribed-episode",
		PublishedDate: time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&subscribedEpisode).Error)
	require.NoError(t, db.Create(&unsubscribedEpisode).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewDiscoveryHandler(
		services.NewDiscoveryService(db),
		services.NewTriageService(db),
	)
	router.GET("/api/v1/discovery/candidates", handler.ListCandidates)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/discovery/candidates?limit=10", nil)
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		Success bool                          `json:"success"`
		Data    []services.DiscoveryCandidate `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data, 1)
	assert.Equal(t, subscribedEpisode.ID, body.Data[0].EpisodeID)
	assert.Equal(t, "已订阅节目", body.Data[0].PodcastTitle)
}

func TestDiscoveryHandler_ListCandidates_RejectsInvalidLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:discovery_handler_invalid?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewDiscoveryHandler(
		services.NewDiscoveryService(db),
		services.NewTriageService(db),
	)
	router.GET("/api/v1/discovery/candidates", handler.ListCandidates)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/discovery/candidates?limit=invalid", nil)
	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestDiscoveryHandler_ListTodayShortlist_ReturnsSharedCollection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:discovery_handler_today?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.EpisodeTriageDecision{},
	))
	podcast := models.Podcast{
		Title:        "今日备选节目",
		FeedURL:      "https://example.com/today.xml",
		XYZID:        "discovery-handler-today-podcast",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID:     podcast.ID,
		Title:         "今日保留单集",
		GUID:          "discovery-handler-today-episode",
		PublishedDate: time.Now().UTC(),
		ShowNotes:     "<p>今日备选摘要来源</p>",
	}
	require.NoError(t, db.Create(&episode).Error)
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID: episode.ID,
		State:     models.TriageStateShortlisted,
		DecidedAt: time.Now().UTC(),
	}).Error)
	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewDiscoveryHandler(
		services.NewDiscoveryServiceWithLocation(db, location),
		services.NewTriageService(db),
	)
	router.GET("/api/v1/discovery/shortlist/today", handler.ListTodayShortlist)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/discovery/shortlist/today", nil)
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		Success bool                    `json:"success"`
		Data    services.TodayShortlist `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.True(t, body.Success)
	assert.Equal(t, "Asia/Shanghai", body.Data.Timezone)
	require.Len(t, body.Data.Candidates, 1)
	assert.Equal(t, episode.ID, body.Data.Candidates[0].EpisodeID)
	assert.Equal(t, models.TriageStateShortlisted, body.Data.Candidates[0].DecisionState)
}

func TestDiscoveryHandler_PutDecision_PersistsIdempotentServerState(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:discovery_handler_decision?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.EpisodeTriageDecision{},
	))

	podcast := models.Podcast{
		Title:        "真实个人库节目",
		FeedURL:      "https://example.com/decision.xml",
		XYZID:        "discovery-handler-decision-podcast",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID:     podcast.ID,
		Title:         "可决定单集",
		GUID:          "discovery-handler-decision-episode",
		PublishedDate: time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&episode).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewDiscoveryHandler(
		services.NewDiscoveryService(db),
		services.NewTriageService(db),
	)
	router.GET("/api/v1/discovery/candidates", handler.ListCandidates)
	router.PUT("/api/v1/discovery/candidates/:episodeID/decision", handler.PutDecision)

	for attempt := 0; attempt < 2; attempt++ {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/discovery/candidates/1/decision",
			bytes.NewBufferString(`{"state":"shortlisted"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
	}

	var decisionCount int64
	require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).Count(&decisionCount).Error)
	assert.Equal(t, int64(1), decisionCount)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/discovery/candidates", nil)
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)

	var body struct {
		Data []services.DiscoveryCandidate `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, models.TriageStateShortlisted, body.Data[0].DecisionState)
	require.NotNil(t, body.Data[0].DecisionUpdatedAt)
	assert.False(t, body.Data[0].DecisionUpdatedAt.IsZero())
}
