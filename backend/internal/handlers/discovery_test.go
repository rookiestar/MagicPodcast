package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
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

func TestDiscoveryHandler_SummaryListAndCandidateDetailSplitHeavyContent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:discovery_handler_summary?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.EpisodeTriageDecision{},
	))

	podcast := models.Podcast{
		Title:        "按需详情节目",
		FeedURL:      "https://example.com/detail.xml",
		XYZID:        "discovery-handler-detail-podcast",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID:     podcast.ID,
		Title:         "按需详情单集",
		GUID:          "discovery-handler-detail-episode",
		PublishedDate: time.Now().UTC(),
		ShowNotes:     "<p>完整正文只应由详情接口返回</p>",
	}
	require.NoError(t, db.Create(&episode).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewDiscoveryHandler(
		services.NewDiscoveryService(db),
		services.NewTriageService(db),
	)
	router.GET("/api/v1/discovery/candidates", handler.ListCandidates)
	router.GET("/api/v1/discovery/candidates/:episodeID", handler.GetCandidate)

	summaryResponse := httptest.NewRecorder()
	summaryRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/discovery/candidates?limit=10&view=summary",
		nil,
	)
	router.ServeHTTP(summaryResponse, summaryRequest)

	require.Equal(t, http.StatusOK, summaryResponse.Code)
	var summaryBody struct {
		Success bool                     `json:"success"`
		Data    []map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(summaryResponse.Body.Bytes(), &summaryBody))
	require.True(t, summaryBody.Success)
	require.Len(t, summaryBody.Data, 1)
	assert.Equal(t, true, summaryBody.Data[0]["metadata_only"])
	assert.Contains(t, summaryBody.Data[0]["excerpt"], "完整正文只应由详情接口返回")
	assert.NotContains(t, summaryBody.Data[0], "show_notes")
	assert.NotContains(t, summaryBody.Data[0], "pre_reads")

	detailResponse := httptest.NewRecorder()
	detailRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/discovery/candidates/"+strconv.FormatUint(uint64(episode.ID), 10),
		nil,
	)
	router.ServeHTTP(detailResponse, detailRequest)

	require.Equal(t, http.StatusOK, detailResponse.Code)
	var detailBody struct {
		Success bool                        `json:"success"`
		Data    services.DiscoveryCandidate `json:"data"`
	}
	require.NoError(t, json.Unmarshal(detailResponse.Body.Bytes(), &detailBody))
	assert.Equal(t, episode.ShowNotes, detailBody.Data.ShowNotes)
	assert.Len(t, detailBody.Data.PreReads, 4)
	assert.False(t, detailBody.Data.MetadataOnly)
}

func TestDiscoveryHandler_ListHomepageReports_ReturnsTodayAndDecisions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "reports.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Workflow{},
		&models.Job{},
		&models.Report{},
		&models.Podcast{},
		&models.Episode{},
		&models.EpisodeTriageDecision{},
	))

	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	localNow := time.Now().In(location)
	// Noon is always inside the service's current local day, including midnight runs.
	completedAt := time.Date(
		localNow.Year(), localNow.Month(), localNow.Day(),
		12, 0, 0, 0, location,
	)

	workflow := models.Workflow{
		Name:              "首页日报",
		Schedule:          "0 0 8 * * *",
		ScopeType:         models.ScopeTypeAllSubscribed,
		IsEnabled:         true,
		PublishToHomepage: true,
		ReportType:        "daily",
	}
	require.NoError(t, db.Create(&workflow).Error)
	end := completedAt.UTC()
	job := models.Job{
		WorkflowID:  workflow.ID,
		Status:      models.JobStatusCompleted,
		TriggeredBy: "cron",
		EndTime:     &end,
	}
	require.NoError(t, db.Create(&job).Error)

	podcast := models.Podcast{
		Title: "报告节目", FeedURL: "https://example.com/r.xml", XYZID: "report-pod", IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID, Title: "报告单集", GUID: "report-ep", PublishedDate: completedAt.UTC(),
	}
	require.NoError(t, db.Create(&episode).Error)
	require.NoError(t, db.Create(&models.EpisodeTriageDecision{
		EpisodeID: episode.ID,
		State:     models.TriageStateShortlisted,
		DecidedAt: completedAt.UTC(),
	}).Error)

	require.NoError(t, db.Create(&models.Report{
		JobID:             job.ID,
		Title:             "首页日报 report",
		Content:           "# 完整正文\n\n内容",
		GeneratedAt:       completedAt.UTC(),
		PublishToHomepage: true,
		ReportType:        "daily",
		WorkflowName:      "首页日报",
		StructuredEpisodes: models.ReportEpisodeList{
			{
				EpisodeID:    episode.ID,
				Order:        1,
				PodcastID:    podcast.ID,
				PodcastTitle: podcast.Title,
				EpisodeTitle: episode.Title,
				Context:      "节目上下文",
			},
		},
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewDiscoveryHandler(
		services.NewDiscoveryService(db),
		services.NewTriageService(db),
		services.NewHomepageReportServiceWithLocation(db, location),
	)
	router.GET("/api/v1/discovery/reports", handler.ListHomepageReports)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/discovery/reports?history_limit=10", nil)
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		Success bool                            `json:"success"`
		Data    services.HomepageReportsPayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data.Today, 1)
	assert.Equal(t, "首页日报", body.Data.Today[0].WorkflowName)
	assert.Equal(t, "daily", body.Data.Today[0].ReportType)
	assert.Contains(t, body.Data.Today[0].Content, "完整正文")
	require.Len(t, body.Data.Today[0].Episodes, 1)
	assert.Equal(t, episode.ID, body.Data.Today[0].Episodes[0].EpisodeID)
	assert.Equal(t, models.TriageStateShortlisted, body.Data.Today[0].Episodes[0].DecisionState)
	assert.Empty(t, body.Data.History)
}
