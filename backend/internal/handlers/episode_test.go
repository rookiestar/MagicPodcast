package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/database"
	"magicpodcast/internal/handlers"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type episodeListTestResponse struct {
	Success    bool                     `json:"success"`
	Data       []map[string]interface{} `json:"data"`
	Pagination struct {
		Page       int   `json:"page"`
		PageSize   int   `json:"page_size"`
		Total      int64 `json:"total"`
		TotalPages int64 `json:"total_pages"`
		HasMore    bool  `json:"has_more"`
	} `json:"pagination"`
}

func setupEpisodeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	cache.GetCache().Clear()

	dbName := fmt.Sprintf("file:episode_handler_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Episode{}))

	database.SetTestDB(db)
	t.Cleanup(func() {
		database.ResetDB()
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})

	return db
}

func setupEpisodeTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	episodeHandler := handlers.NewEpisodeHandler()
	router.GET("/api/v1/podcasts/:id/episodes", episodeHandler.ListByPodcast)
	return router
}

func createEpisodeHandlerPodcast(t *testing.T, db *gorm.DB) models.Podcast {
	t.Helper()

	podcast := models.Podcast{
		Title:       "Episode Handler Test Podcast",
		Author:      "Tester",
		Description: "Test",
		FeedURL:     fmt.Sprintf("https://example.com/%d.xml", time.Now().UnixNano()),
		XYZID:       fmt.Sprintf("xyz_%d", time.Now().UnixNano()),
	}
	require.NoError(t, db.Create(&podcast).Error)
	return podcast
}

func createEpisodeHandlerEpisode(t *testing.T, db *gorm.DB, podcastID uint, sequence int, publishedDate time.Time) models.Episode {
	t.Helper()

	episode := models.Episode{
		PodcastID:       podcastID,
		EpisodeNo:       fmt.Sprintf("E%d", sequence),
		Title:           fmt.Sprintf("Episode %d", sequence),
		MediumURL:       fmt.Sprintf("https://example.com/%d.mp3", sequence),
		ShowNotes:       fmt.Sprintf("<p>Show notes %d</p>", sequence),
		PublishedDate:   publishedDate,
		Duration:        sequence * 60,
		Link:            fmt.Sprintf("https://example.com/episodes/%d", sequence),
		Content:         "large content should not be returned by list endpoint",
		ImageURL:        fmt.Sprintf("https://example.com/%d.jpg", sequence),
		EnclosureType:   "audio/mpeg",
		EnclosureLength: int64(sequence * 1024),
		GUID:            fmt.Sprintf("episode-handler-guid-%d-%d", time.Now().UnixNano(), sequence),
		Notes:           "private note should not be returned by list endpoint",
	}
	require.NoError(t, db.Create(&episode).Error)
	return episode
}

func TestEpisodeHandler_ListByPodcast_SummaryViewUsesShorterShowNotes(t *testing.T) {
	db := setupEpisodeTestDB(t)
	router := setupEpisodeTestRouter()
	podcast := createEpisodeHandlerPodcast(t, db)
	publishedDate := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	longNotes := "<p>" + strings.Repeat("Long show notes paragraph ", 50) + "</p>"
	episode := createEpisodeHandlerEpisode(t, db, podcast.ID, 1, publishedDate)
	require.NoError(t, db.Model(&episode).Update("show_notes", longNotes).Error)

	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/podcasts/%d/episodes?page=1&page_size=1&view=summary", podcast.ID), nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)

	var body episodeListTestResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "1", body.Data[0]["episode_no"])

	summaryShowNotes, ok := body.Data[0]["show_notes"].(string)
	require.True(t, ok)
	assert.LessOrEqual(t, len([]rune(summaryShowNotes)), 323)
	assert.Contains(t, summaryShowNotes, "...")

	request, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/podcasts/%d/episodes?page=1&page_size=1", podcast.ID), nil)
	response = httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)

	fullShowNotes, ok := body.Data[0]["show_notes"].(string)
	require.True(t, ok)
	assert.Greater(t, len([]rune(fullShowNotes)), len([]rune(summaryShowNotes)))
	assert.Equal(t, "unknown", body.Data[0]["video_availability"])
}

func TestEpisodeHandler_ListByPodcast_IncludesVideoAvailability(t *testing.T) {
	db := setupEpisodeTestDB(t)
	router := setupEpisodeTestRouter()
	podcast := createEpisodeHandlerPodcast(t, db)
	publishedDate := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	available := createEpisodeHandlerEpisode(t, db, podcast.ID, 1, publishedDate)
	require.NoError(t, db.Model(&available).Update("video_availability", models.VideoAvailabilityAvailable).Error)
	unavailable := createEpisodeHandlerEpisode(t, db, podcast.ID, 2, publishedDate.Add(-time.Hour))
	require.NoError(t, db.Model(&unavailable).Update("video_availability", models.VideoAvailabilityUnavailable).Error)
	createEpisodeHandlerEpisode(t, db, podcast.ID, 3, publishedDate.Add(-2*time.Hour))

	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/podcasts/%d/episodes?page=1&page_size=10", podcast.ID), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)

	var body episodeListTestResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Data, 3)
	assert.Equal(t, "available", body.Data[0]["video_availability"])
	assert.Equal(t, "unavailable", body.Data[1]["video_availability"])
	assert.Equal(t, "unknown", body.Data[2]["video_availability"])
	assert.NotContains(t, response.Body.String(), "m3u8")
	assert.NotContains(t, response.Body.String(), "auth_key")
}

func TestEpisodeHandler_HidesUnreliableStoredEpisodeNumber(t *testing.T) {
	db := setupEpisodeTestDB(t)
	router := setupEpisodeTestRouter()
	podcast := createEpisodeHandlerPodcast(t, db)
	episode := createEpisodeHandlerEpisode(
		t,
		db,
		podcast.ID,
		1,
		time.Date(2026, 8, 6, 14, 9, 44, 0, time.UTC),
	)
	require.NoError(t, db.Model(&episode).Updates(map[string]interface{}{
		"title":      "昆山杜克大学周忆粟：AI 来了，年轻人的梯子被抽掉了",
		"episode_no": "1",
	}).Error)

	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/podcasts/%d/episodes?page=1&page_size=1", podcast.ID), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var body episodeListTestResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "", body.Data[0]["episode_no"])
}

func TestEpisodeHandler_ListByPodcast_PaginatesWithStableOrder(t *testing.T) {
	db := setupEpisodeTestDB(t)
	router := setupEpisodeTestRouter()
	podcast := createEpisodeHandlerPodcast(t, db)
	publishedDate := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	firstCreated := createEpisodeHandlerEpisode(t, db, podcast.ID, 1, publishedDate)
	secondCreated := createEpisodeHandlerEpisode(t, db, podcast.ID, 2, publishedDate)
	thirdCreated := createEpisodeHandlerEpisode(t, db, podcast.ID, 3, publishedDate)

	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/podcasts/%d/episodes?page=1&page_size=2", podcast.ID), nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)

	var body episodeListTestResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))

	require.True(t, body.Success)
	require.Len(t, body.Data, 2)
	assert.Equal(t, float64(thirdCreated.ID), body.Data[0]["id"])
	assert.Equal(t, float64(secondCreated.ID), body.Data[1]["id"])
	assert.Equal(t, int64(3), body.Pagination.Total)
	assert.Equal(t, int64(2), body.Pagination.TotalPages)
	assert.True(t, body.Pagination.HasMore)

	_, hasContent := body.Data[0]["content"]
	assert.False(t, hasContent)
	_, hasUpdatedDate := body.Data[0]["updated_date"]
	assert.False(t, hasUpdatedDate)
	assert.Equal(t, "Show notes 3", body.Data[0]["show_notes"])
	assert.Equal(t, "", body.Data[0]["notes"])

	request, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/podcasts/%d/episodes?page=2&page_size=2", podcast.ID), nil)
	response = httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, float64(firstCreated.ID), body.Data[0]["id"])
	assert.False(t, body.Pagination.HasMore)
}

func TestEpisodeHandler_ListByPodcast_ReturnsNotFoundForMissingPodcast(t *testing.T) {
	setupEpisodeTestDB(t)
	router := setupEpisodeTestRouter()

	request, _ := http.NewRequest(http.MethodGet, "/api/v1/podcasts/9999/episodes", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
}
