package handlers_test

import (
	"encoding/json"
	"fmt"
	"magicpodcast/internal/services"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/config"
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
	previous := config.Get()
	config.SetTestConfig(nil)
	t.Cleanup(func() { config.SetTestConfig(previous) })

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

func TestSearchHandler_WhitespaceTolerance(t *testing.T) {
	for _, indexed := range []bool{false, true} {
		t.Run(fmt.Sprintf("fts=%t", indexed), func(t *testing.T) {
			for _, tc := range []struct {
				query, target string
				hit           bool
			}{
				{"42 章经", "42章经", true}, {"42章经", "42 章经", true},
				{"42　章经", "42章经", true}, {"42   章经", "42章经", true},
				{"聊聊 AI 创业", "聊聊AI创业", true}, {"AI创业", "AI 创业", true},
				{"AI创业", "AI\t\n\u00a0创业", true}, {"a bowl", "abowl", false},
				{"4 2章经", "42章经", false}, {"章 经", "章经", false},
				{"AI 创业", "AI 投资", false}, {"42 章经", "42其他章经", false},
				{"42%章经", "42其他章经", false}, {"42%章经", "42%章经", true}, {"OpenAI 创业", "OpenAI 投资", false},
			} {
				t.Run(tc.query+"/"+tc.target, func(t *testing.T) {
					db := setupSearchTestDB(t)
					podcast := createSearchFixture(t, db)
					require.NoError(t, db.Model(&podcast).Updates(map[string]interface{}{"title": tc.target, "author": "无关", "description": "无关"}).Error)
					require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Updates(map[string]interface{}{"title": tc.target, "show_notes": "无关"}).Error)
					if indexed {
						require.NoError(t, db.Exec("CREATE VIRTUAL TABLE podcast_search_fts USING fts4(title, author, description, content='podcasts', tokenize=unicode61)").Error)
						require.NoError(t, db.Exec("CREATE VIRTUAL TABLE episode_search_fts USING fts4(title, show_notes, content='episodes', tokenize=unicode61)").Error)
						require.NoError(t, db.Exec("INSERT INTO podcast_search_fts(podcast_search_fts) VALUES('rebuild')").Error)
						require.NoError(t, db.Exec("INSERT INTO episode_search_fts(episode_search_fts) VALUES('rebuild')").Error)
					}
					response := httptest.NewRecorder()
					setupSearchTestRouter().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/search?q="+url.QueryEscape(tc.query), nil))
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					var body searchHandlerTestResponse
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
					expected := 0
					if tc.hit {
						expected = 1
					}
					require.Len(t, body.Data.Podcasts, expected)
					require.Len(t, body.Data.Episodes, expected)
					if tc.hit {
						assert.Equal(t, tc.target, body.Data.Podcasts[0].Title)
					}
				})
			}
		})
	}
}

func searchWhitespaceResponse(t *testing.T, router *gin.Engine, query string) services.SearchResponse {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/search?"+query, nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Data services.SearchResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	return body.Data
}

func TestSearchHandler_WhitespaceOrderingAndPagination(t *testing.T) {
	db := setupSearchTestDB(t)
	// The oldest exact title must survive a candidate window filled with newer body hits.
	for i := 0; i < 86; i++ {
		title, body := fmt.Sprintf("无关 %d", i), "42章经"
		if i == 0 {
			title = "42章经"
		}
		if i == 1 {
			title = "42 章经"
		}
		if i == 2 {
			title = "42章经访谈"
		}
		p := models.Podcast{XYZID: fmt.Sprintf("page-%d", i), Title: title, Description: body, FeedURL: fmt.Sprintf("https://example.com/%d", i)}
		require.NoError(t, db.Create(&p).Error)
		e := models.Episode{PodcastID: p.ID, Title: title, ShowNotes: body, GUID: fmt.Sprintf("page-%d", i), PublishedDate: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC)}
		require.NoError(t, db.Create(&e).Error)
	}
	router := setupSearchTestRouter()
	first := searchWhitespaceResponse(t, router, "q=42+章经&page_size=2&episode_page_size=2")
	require.Len(t, first.Podcasts, 2)
	assert.Equal(t, "42 章经", first.Podcasts[0].Title)
	assert.Equal(t, "42章经", first.Podcasts[1].Title)
	assert.Equal(t, first.Podcasts[0].Title, first.Episodes[0].Title)
	assert.Equal(t, 86, first.Pagination.Podcasts.Total)
	assert.Equal(t, 86, first.Pagination.Episodes.Total)
	seenP, seenE := map[uint]bool{}, map[uint]bool{}
	for page := 1; page <= 9; page++ {
		params := fmt.Sprintf("q=42+章经&page_size=10&episode_page_size=10&page=%d&episode_page=%d", page, page)
		result := searchWhitespaceResponse(t, router, params)
		repeated := searchWhitespaceResponse(t, router, params)
		assert.Equal(t, result, repeated)
		for _, p := range result.Podcasts {
			require.False(t, seenP[p.ID])
			seenP[p.ID] = true
		}
		for _, e := range result.Episodes {
			require.False(t, seenE[e.ID])
			seenE[e.ID] = true
		}
	}
	assert.Len(t, seenP, 86)
	assert.Len(t, seenE, 86)
	noTotals := searchWhitespaceResponse(t, router, "q=42+章经&page_size=2&episode_page_size=2&include_totals=false")
	assert.Equal(t, first.Podcasts, noTotals.Podcasts)
	assert.Equal(t, first.Episodes, noTotals.Episodes)
	podcasts := searchWhitespaceResponse(t, router, "q=42+章经&type=podcasts&page_size=2")
	episodes := searchWhitespaceResponse(t, router, "q=42+章经&type=episodes&episode_page_size=2")
	assert.Equal(t, first.Podcasts, podcasts.Podcasts)
	assert.Equal(t, first.Episodes, episodes.Episodes)
}

func TestSearchHandler_WhitespaceFieldsAndSnippets(t *testing.T) {
	for _, field := range []string{"title", "author", "description", "show_notes"} {
		t.Run(field, func(t *testing.T) {
			db := setupSearchTestDB(t)
			p := createSearchFixture(t, db)
			long := strings.Repeat("前文", 200) + "42　章经" + strings.Repeat("后文", 200)
			if field == "show_notes" {
				require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", p.ID).Update(field, "<p>"+long+"</p>").Error)
			} else {
				require.NoError(t, db.Model(&p).Update(field, long).Error)
			}
			result := searchWhitespaceResponse(t, setupSearchTestRouter(), "q=42+章经")
			if field == "show_notes" {
				require.Len(t, result.Episodes, 1)
				assert.Contains(t, result.Episodes[0].ShowNotes, "42　章经")
				assert.LessOrEqual(t, len([]rune(strings.Split(result.Episodes[0].ShowNotes, "42　章经")[0])), 38)
				assert.NotContains(t, result.Episodes[0].ShowNotes, "<p>")
				require.NotEmpty(t, result.Episodes[0].MatchedFields)
			} else {
				require.Len(t, result.Podcasts, 1)
				require.NotEmpty(t, result.Podcasts[0].MatchedFields)
				assert.Equal(t, field, result.Podcasts[0].MatchedFields[0].Field)
				assert.Contains(t, result.Podcasts[0].MatchedFields[0].Snippet, "42　章经")
			}
		})
	}
}

func TestSearchHandler_UsesDefaultsForMissingSearchConfig(t *testing.T) {
	db := setupSearchTestDB(t)
	config.SetTestConfig(&config.Config{Search: config.SearchConfig{DefaultPageSize: 20}})
	p := createSearchFixture(t, db)
	require.NoError(t, db.Model(&p).Update("title", "42章经").Error)
	result := searchWhitespaceResponse(t, setupSearchTestRouter(), "q=42+章经")
	require.Len(t, result.Podcasts, 1)
	assert.Positive(t, result.Podcasts[0].RelevanceScore)
}

func TestSearchHandler_WhitespaceTagsAndDeletion(t *testing.T) {
	db := setupSearchTestDB(t)
	require.NoError(t, db.Exec("CREATE TABLE IF NOT EXISTS podcasts_tags (podcast_id INTEGER, tag_id INTEGER)").Error)
	p := createSearchFixture(t, db)
	require.NoError(t, db.Model(&p).Update("title", "42章经").Error)
	require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", p.ID).Update("title", "42章经").Error)
	require.NoError(t, db.Exec("INSERT INTO podcasts_tags (podcast_id, tag_id) VALUES (?, 7)", p.ID).Error)
	router := setupSearchTestRouter()
	result := searchWhitespaceResponse(t, router, "q=42+章经&tag_id=7")
	require.Len(t, result.Podcasts, 1)
	require.Len(t, result.Episodes, 1)
	result = searchWhitespaceResponse(t, router, "q=42+章经&tag_id=8")
	require.Empty(t, result.Podcasts)
	require.Empty(t, result.Episodes)
	require.NoError(t, db.Delete(&p).Error)
	result = searchWhitespaceResponse(t, router, "q=42+章经")
	require.Empty(t, result.Podcasts)
	require.Empty(t, result.Episodes)
}

func TestSearchHandler_PreservesSignificantWhitespaceInSnippet(t *testing.T) {
	db := setupSearchTestDB(t)
	p := createSearchFixture(t, db)
	phrase := "汉\n字AI"
	notes := strings.Repeat("前文", 200) + phrase + strings.Repeat("后文", 200)
	require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", p.ID).Update("show_notes", notes).Error)
	response := searchWhitespaceResponse(t, setupSearchTestRouter(), "q="+url.QueryEscape("汉\n字 AI"))
	require.Len(t, response.Episodes, 1)
	require.Contains(t, response.Episodes[0].ShowNotes, phrase)
	require.Contains(t, response.Episodes[0].MatchedFields[0].Snippet, phrase)
}
