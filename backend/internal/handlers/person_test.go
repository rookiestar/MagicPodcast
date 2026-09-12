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
	service, err := personidentity.NewService(db, fixedIdentityFixture{
		{DisplayName: "张三", Role: "host", EvidenceKind: "verified_runtime"},
		{DisplayName: "李明", Role: "guest", EvidenceKind: "verified_runtime"},
	})
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
	_, err = prepareReviewed(service, context.Background(), personidentity.EpisodeSources{
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

// Controlled identity decisions for HTTP behavior tests, never model quality.
type fixedIdentityFixture []personidentity.SuggestedCandidate

func (f fixedIdentityFixture) Suggest(context.Context, personidentity.EpisodeSources) ([]personidentity.SuggestedCandidate, error) {
	return f, nil
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

func TestAppearanceCorrectionHTTPValidatesAndSeparatesRoleFromIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openIsolatedPersonDB(t)
	pod := models.Podcast{XYZID: "appearance-http", Title: "访谈", FeedURL: "https://example.test/appearance-http"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "appearance-http", Title: "访谈"}
	require.NoError(t, db.Create(&ep).Error)
	service, err := personidentity.NewService(db, fixedIdentityFixture{{DisplayName: "林言", Role: "unknown", Status: "pending", EvidenceKind: "verified_runtime"}})
	require.NoError(t, err)
	initial, err := prepareReviewed(service, context.Background(), personidentity.EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []personidentity.Segment{{Order: 1, Text: "投资观点。"}}})
	require.NoError(t, err)
	require.Len(t, initial.People, 1)
	router := gin.New()
	handler := handlers.NewPersonHandler(service)
	router.POST("/episodes/:id/people/:personId/appearance-corrections", handler.CorrectAppearance)
	url := fmt.Sprintf("/episodes/%d/people/%d/appearance-corrections", ep.ID, initial.People[0].ID)
	for _, body := range []string{`{}`, `{"role":"producer"}`, `{"excluded":"yes"}`, `{"role":"host","unexpected":true}`} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(body)))
		require.Equal(t, http.StatusBadRequest, recorder.Code, body)
	}
	for _, body := range []string{`{"role":"host"}`, `{"excluded":true}`, `{"excluded":false}`} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(body)))
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	}
	final, err := service.ListEpisodePeople(context.Background(), ep.ID)
	require.NoError(t, err)
	require.Equal(t, "host", final.People[0].Role)
	require.Equal(t, initial.People[0].Status, final.People[0].Status, "correcting role must not change identity approval")
	require.Zero(t, final.People[0].ConfirmedSpeech)
}

type cancellablePersonPreparation struct {
	personidentity.Module
	started   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
}

func (p *cancellablePersonPreparation) PrepareCurrent(ctx context.Context, _ uint) (personidentity.EpisodePeople, error) {
	close(p.started)
	select {
	case <-ctx.Done():
		close(p.cancelled)
		return personidentity.EpisodePeople{}, ctx.Err()
	case <-p.release:
		return personidentity.EpisodePeople{}, context.Canceled
	}
}

func TestPersonPreparationObservesHTTPClientCancellationWithPOSTBody(t *testing.T) {
	for _, accept := range []string{"application/json", "text/event-stream"} {
		t.Run(accept, func(t *testing.T) { checkPersonPreparationCancellation(t, accept) })
	}
}

func checkPersonPreparationCancellation(t *testing.T, accept string) {
	gin.SetMode(gin.TestMode)
	preparer := &cancellablePersonPreparation{started: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
	engine := gin.New()
	engine.POST("/episodes/:id/people/prepare", handlers.NewPersonHandler(preparer).Prepare)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(preparer.release) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/episodes/1/people/prepare", bytes.NewBufferString("{}"))
	require.NoError(t, err)
	request.Header.Set("Accept", accept)
	done := make(chan struct{})
	go func() {
		defer close(done)
		response, _ := server.Client().Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
	}()
	select {
	case <-preparer.started:
	case <-time.After(time.Second):
		t.Fatal("preparation did not start")
	}
	cancel()
	select {
	case <-preparer.cancelled:
	case <-time.After(time.Second):
		t.Fatal("HTTP client cancellation did not reach preparation")
	}
	<-done
}

func prepareReviewed(s *personidentity.Service, ctx context.Context, src personidentity.EpisodeSources) (personidentity.EpisodePeople, error) {
	p, err := s.Prepare(ctx, src)
	if err != nil || p.Draft == nil {
		return p, err
	}
	for i := range p.Draft.Matches {
		p.Draft.Matches[i].Selected = true
	}
	return s.Review(ctx, src.EpisodeID, personidentity.ReviewRequest{DraftID: p.Draft.ID, Revision: p.Revision, SourceVersion: p.Draft.SourceVersion, Matches: p.Draft.Matches}, true)
}
