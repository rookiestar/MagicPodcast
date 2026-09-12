package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"magicpodcast/internal/collection"
	"magicpodcast/internal/handlers"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const collectionSampleID = "6a20323b78a52c96d821a769"

// stubFetcher 以可控来源响应替代真实小宇宙读取。
type stubFetcher struct {
	body   string
	status int
	calls  int
}

func (f *stubFetcher) FetchCollectionPage(context.Context, string) ([]byte, error) {
	f.calls++
	if f.status == http.StatusForbidden {
		return nil, collection.ErrSourceForbidden
	}
	if f.status != http.StatusOK {
		return nil, collection.ErrSourceUnavailable
	}
	return []byte(f.body), nil
}

func newCollectionTestEnv(t *testing.T, fetcher collection.SourceFetcher) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "collection_handler.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.EpisodeTriageDecision{},
		&models.EpisodeCollection{},
		&models.EpisodeCollectionItem{},
		&models.EpisodeExternalRef{},
	))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewCollectionHandler(collection.NewServiceWithFetcher(db, fetcher))
	router.POST("/api/v1/collections/preview", handler.Preview)
	router.POST("/api/v1/collections", handler.ConfirmImport)
	router.GET("/api/v1/collections", handler.List)
	router.GET("/api/v1/collections/:id", handler.Get)
	return router, db
}

func loadCollectionSampleHTML(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../collection/testdata/collection_sample.html")
	require.NoError(t, err)
	return string(raw)
}

func postJSON(t *testing.T, router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func errorBody(t *testing.T, response *httptest.ResponseRecorder) (code string, message string) {
	t.Helper()
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	return body.Error.Code, body.Error.Message
}

func previewAndImport(t *testing.T, router *gin.Engine, url string) (duplicate bool, collectionID uint) {
	t.Helper()
	preview := postJSON(t, router, "/api/v1/collections/preview", map[string]string{"url": url})
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	var previewBody struct {
		Data struct {
			PreviewID string `json:"preview_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(preview.Body.Bytes(), &previewBody))
	require.NotEmpty(t, previewBody.Data.PreviewID)

	confirmed := postJSON(t, router, "/api/v1/collections", map[string]string{"preview_id": previewBody.Data.PreviewID})
	require.Equal(t, http.StatusOK, confirmed.Code, confirmed.Body.String())
	var importBody struct {
		Data struct {
			Duplicate    bool `json:"duplicate"`
			CollectionID uint `json:"collection_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(confirmed.Body.Bytes(), &importBody))
	return importBody.Data.Duplicate, importBody.Data.CollectionID
}

func TestCollectionHandler_ImportPreviewAndBrowse(t *testing.T) {
	fetcher := &stubFetcher{body: loadCollectionSampleHTML(t), status: http.StatusOK}
	router, db := newCollectionTestEnv(t, fetcher)

	// 不支持的 URL 拒绝且不读取来源。
	rejected := postJSON(t, router, "/api/v1/collections/preview", map[string]string{"url": "https://evil.example.com/x"})
	require.Equal(t, http.StatusUnprocessableEntity, rejected.Code)
	assert.Zero(t, fetcher.calls)
	code, _ := errorBody(t, rejected)
	assert.Equal(t, "UNSUPPORTED_SOURCE", code)

	url := "https://www.xiaoyuzhoufm.com/collection/episode/" + collectionSampleID
	duplicate, collectionID := previewAndImport(t, router, url)
	assert.False(t, duplicate)
	require.NotZero(t, collectionID)

	// 重复导入：打开已有清单，不新建副本。
	duplicateAgain, sameID := previewAndImport(t, router, url)
	assert.True(t, duplicateAgain)
	assert.Equal(t, collectionID, sameID)

	var collectionCount, itemCount int64
	require.NoError(t, db.Model(&models.EpisodeCollection{}).Count(&collectionCount).Error)
	require.NoError(t, db.Model(&models.EpisodeCollectionItem{}).Count(&itemCount).Error)
	assert.Equal(t, int64(1), collectionCount)
	assert.Equal(t, int64(8), itemCount)

	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet,
		"/api/v1/collections?search=%E5%8D%8A%E5%AF%BC%E4%BD%93", nil))
	require.Equal(t, http.StatusOK, listResponse.Code)
	var listBody struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listResponse.Body.Bytes(), &listBody))
	require.Len(t, listBody.Data, 1)
	assert.Equal(t, float64(8), listBody.Data[0]["item_count"])
	assert.Equal(t, float64(0), listBody.Data[0]["adopted_count"])
	assert.Equal(t, "xiaoyuzhoufm", listBody.Data[0]["platform"])

	detailResponse := httptest.NewRecorder()
	router.ServeHTTP(detailResponse, httptest.NewRequest(http.MethodGet,
		"/api/v1/collections/"+strconv.FormatUint(uint64(collectionID), 10), nil))
	require.Equal(t, http.StatusOK, detailResponse.Code)
	var detailBody struct {
		Data struct {
			Title     string `json:"title"`
			Author    string `json:"author"`
			SourceURL string `json:"source_url"`
			Items     []struct {
				Position         int    `json:"position"`
				EpisodeTitle     string `json:"episode_title"`
				Recommendation   string `json:"recommendation"`
				Shownotes        string `json:"shownotes"`
				EpisodeURL       string `json:"episode_url"`
				AdoptedEpisodeID *uint  `json:"adopted_episode_id"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(detailResponse.Body.Bytes(), &detailBody))
	assert.Equal(t, "穿透半导体迷雾", detailBody.Data.Title)
	assert.Equal(t, "小宇宙领航员", detailBody.Data.Author)
	require.Len(t, detailBody.Data.Items, 8)
	for index, item := range detailBody.Data.Items {
		assert.Equal(t, index, item.Position)
		assert.Nil(t, item.AdoptedEpisodeID, "第 1 票不提供采纳入口，条目不关联个人单集")
		assert.NotEmpty(t, item.EpisodeURL)
	}
	assert.Equal(t, "存储芯片为何五年内持续短缺？", detailBody.Data.Items[0].Recommendation)
	assert.NotEmpty(t, detailBody.Data.Items[0].Shownotes)

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/v1/collections/424242", nil))
	assert.Equal(t, http.StatusNotFound, missing.Code)
}

func TestCollectionHandler_SourceFailuresAreDistinct(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		wantStatus int
		wantCode   string
	}{
		{"403受限", http.StatusForbidden, http.StatusForbidden, "SOURCE_FORBIDDEN"},
		{"网络失败", http.StatusServiceUnavailable, http.StatusBadGateway, "SOURCE_UNAVAILABLE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := &stubFetcher{status: tc.status}
			router, db := newCollectionTestEnv(t, fetcher)
			response := postJSON(t, router, "/api/v1/collections/preview",
				map[string]string{"url": "https://www.xiaoyuzhoufm.com/collection/episode/" + collectionSampleID})
			require.Equal(t, tc.wantStatus, response.Code)
			code, message := errorBody(t, response)
			assert.Equal(t, tc.wantCode, code)
			assert.NotEmpty(t, message)

			var collectionCount int64
			require.NoError(t, db.Model(&models.EpisodeCollection{}).Count(&collectionCount).Error)
			assert.Zero(t, collectionCount, "失败预览不保存任何清单")
		})
	}

	t.Run("未知预览", func(t *testing.T) {
		router, _ := newCollectionTestEnv(t, &stubFetcher{status: http.StatusOK})
		response := postJSON(t, router, "/api/v1/collections", map[string]string{"preview_id": "missing"})
		require.Equal(t, http.StatusNotFound, response.Code)
		code, _ := errorBody(t, response)
		assert.Equal(t, "PREVIEW_NOT_FOUND", code)
	})

	t.Run("损坏来源结构", func(t *testing.T) {
		router, _ := newCollectionTestEnv(t, &stubFetcher{status: http.StatusOK, body: "<html>login required</html>"})
		response := postJSON(t, router, "/api/v1/collections/preview",
			map[string]string{"url": "https://www.xiaoyuzhoufm.com/collection/episode/" + collectionSampleID})
		require.Equal(t, http.StatusUnprocessableEntity, response.Code)
		code, _ := errorBody(t, response)
		assert.Equal(t, "SOURCE_PARSE_FAILED", code)
	})
}

func TestCollectionHandler_ImportKeepsLibraryUntouched(t *testing.T) {
	fetcher := &stubFetcher{body: loadCollectionSampleHTML(t), status: http.StatusOK}
	router, db := newCollectionTestEnv(t, fetcher)

	podcast := models.Podcast{Title: "已有节目", FeedURL: "https://example.com/feed.xml", XYZID: "handler-isolation", IsSubscribed: true}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{PodcastID: podcast.ID, Title: "已有单集", GUID: "handler-isolation-episode"}
	require.NoError(t, db.Create(&episode).Error)

	_, _ = previewAndImport(t, router, "https://www.xiaoyuzhoufm.com/collection/episode/"+collectionSampleID)

	var podcastCount, episodeCount int64
	require.NoError(t, db.Model(&models.Podcast{}).Count(&podcastCount).Error)
	require.NoError(t, db.Model(&models.Episode{}).Count(&episodeCount).Error)
	assert.Equal(t, int64(1), podcastCount)
	assert.Equal(t, int64(1), episodeCount)
}

func TestCollectionHandler_RejectsInvalidIDs(t *testing.T) {
	router, _ := newCollectionTestEnv(t, &stubFetcher{status: http.StatusOK, body: "{}"})

	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/v1/collections/abc", nil))
	assert.Equal(t, http.StatusBadRequest, invalid.Code)

	empty := postJSON(t, router, "/api/v1/collections/preview", map[string]string{"url": "   "})
	assert.Equal(t, http.StatusBadRequest, empty.Code)
}
