package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/handlers"
	"magicpodcast/internal/models"
	"magicpodcast/internal/personidentity"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPersonHandlerListsAndCorrectsAttributionWithoutRewritingSource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openIsolatedPersonDB(t)
	service, err := personidentity.NewService(db, nil)
	require.NoError(t, err)

	podcast := models.Podcast{
		XYZID: "person-http", Title: "技术漫谈", FeedURL: "https://example.test/person-http.xml",
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID, Title: "混标签", ShowNotes: "主播张三，嘉宾李明。", GUID: "person-http-ep",
		PublishedDate: time.Date(2025, 8, 11, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&episode).Error)
	_, err = service.Prepare(context.Background(), personidentity.EpisodeSources{
		EpisodeID:     episode.ID,
		ShowNotes:     episode.ShowNotes,
		SourceKind:    personidentity.SourceTranscript,
		SourceVersion: "v1",
		Segments: []personidentity.Segment{
			{Order: 1, SpeakerLabel: "嘉宾", StartMS: 1000, Text: "我不赞成无限制加班。"},
			{Order: 2, SpeakerLabel: "嘉宾", StartMS: 2000, Text: "阶段性冲刺可以接受。"},
		},
	})
	require.NoError(t, err)

	router := gin.New()
	handler := handlers.NewPersonHandler(service)
	router.GET("/api/v1/episodes/:id/people", handler.List)
	router.POST("/api/v1/episodes/:id/people/:personId/corrections", handler.CorrectName)
	router.POST("/api/v1/episodes/:id/attributions/corrections", handler.CorrectAttribution)

	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/episodes/%d/people", episode.ID), nil))
	require.Equal(t, http.StatusOK, list.Code)
	var listed struct {
		Success bool
		Data    personidentity.EpisodePeople
	}
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &listed))
	require.True(t, listed.Success)
	require.GreaterOrEqual(t, len(listed.Data.People), 2)
	var zhangID uint
	for _, person := range listed.Data.People {
		if person.DisplayName == "张三" {
			zhangID = person.ID
			require.Equal(t, "host", person.Role)
		}
	}
	require.NotZero(t, zhangID)
	require.Nil(t, listed.Data.Attributions[0].PersonID)

	body, err := json.Marshal(map[string]any{
		"source_kind":        "transcript",
		"fragment_order":     1,
		"assigned_person_id": zhangID,
		"status":             "confirmed",
	})
	require.NoError(t, err)
	correct := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/api/v1/episodes/%d/attributions/corrections", episode.ID),
		bytes.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(correct, request)
	require.Equal(t, http.StatusOK, correct.Code)
	var corrected struct {
		Success bool
		Data    personidentity.EpisodePeople
	}
	require.NoError(t, json.Unmarshal(correct.Body.Bytes(), &corrected))
	require.Equal(t, zhangID, *corrected.Data.Attributions[0].PersonID)
	require.Equal(t, "confirmed", corrected.Data.Attributions[0].Status)
	require.True(t, corrected.Data.Attributions[0].UserConfirmed)
}

func openIsolatedPersonDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:person_http_%d?mode=memory&cache=shared&_foreign_keys=on", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.ApplyMigrations(db))
	return db
}
