package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/handlers"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type searchHandlerTestResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Podcasts []struct {
			ID    uint   `json:"id"`
			Title string `json:"title"`
		} `json:"podcasts"`
		Episodes []struct {
			ID    uint   `json:"id"`
			Title string `json:"title"`
		} `json:"episodes"`
	} `json:"data"`
}

func setupSearchTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbName := fmt.Sprintf("file:search_handler_%d?mode=memory&cache=shared", time.Now().UnixNano())
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

func setupSearchTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	searchHandler := handlers.NewSearchHandler()
	router.GET("/api/v1/search", searchHandler.Search)
	return router
}

func createSearchFixture(t *testing.T, db *gorm.DB) models.Podcast {
	t.Helper()

	podcast := models.Podcast{
		Title:        "AI Frontiers",
		Author:       "AI Team",
		Description:  "A practical AI podcast",
		FeedURL:      fmt.Sprintf("https://example.com/search-%d.xml", time.Now().UnixNano()),
		XYZID:        fmt.Sprintf("search_xyz_%d", time.Now().UnixNano()),
		EpisodeCount: 1,
	}
	require.NoError(t, db.Create(&podcast).Error)

	episode := models.Episode{
		PodcastID:     podcast.ID,
		Title:         "AI workflow stability",
		ShowNotes:     "How to keep search stable while typing",
		PublishedDate: time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
		GUID:          fmt.Sprintf("search-guid-%d", time.Now().UnixNano()),
	}
	require.NoError(t, db.Create(&episode).Error)

	return podcast
}

func TestSearchHandler_RejectsBlankQuery(t *testing.T) {
	setupSearchTestDB(t)
	router := setupSearchTestRouter()

	request, _ := http.NewRequest(http.MethodGet, "/api/v1/search?q=%20%20%20", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestSearchHandler_RejectsInvalidType(t *testing.T) {
	setupSearchTestDB(t)
	router := setupSearchTestRouter()

	request, _ := http.NewRequest(http.MethodGet, "/api/v1/search?q=ai&type=bad", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestSearchHandler_TrimsQueryBeforeSearching(t *testing.T) {
	db := setupSearchTestDB(t)
	createSearchFixture(t, db)
	router := setupSearchTestRouter()

	request, _ := http.NewRequest(http.MethodGet, "/api/v1/search?q=%20%20AI%20%20&type=all", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)

	var body searchHandlerTestResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data.Podcasts, 1)
	require.Len(t, body.Data.Episodes, 1)
	assert.Equal(t, "AI Frontiers", body.Data.Podcasts[0].Title)
	assert.Equal(t, "AI workflow stability", body.Data.Episodes[0].Title)
}
