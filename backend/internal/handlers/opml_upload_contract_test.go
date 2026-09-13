package handlers

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newUploadContractRouter 构建独立隔离库上的单个导入入口路由。
func newUploadContractRouter(t *testing.T, streaming bool) (*gin.Engine, *gorm.DB) {
	t.Helper()
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.ImportTask{}))
	service, err := syncsvc.NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	handler := &SyncHandler{syncService: service, db: db}
	router := gin.New()
	if streaming {
		router.POST("/import", handler.ImportOPMLSSE)
	} else {
		router.POST("/import", handler.ImportOPML)
	}
	return router, db
}

func buildOPMLUploadRequest(t *testing.T, router *gin.Engine, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("opml_file", filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-MagicPodcast-Confirmation", "IMPORT OPML")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

const uploadContractFeedXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Contract Feed</title><link>https://example.com</link>
<item><title>Episode</title><guid>ep-1</guid><pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate></item>
</channel></rss>`

// TestOPMLUploadEntriesShareOneContract 用同一文件分别走普通与流式入口，
// 断言两套入口的校验结论、业务结果与持久数据一致（#398 R10/R11）。
func TestOPMLUploadEntriesShareOneContract(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(uploadContractFeedXML))
	}))
	defer upstream.Close()

	opmlContent := `<opml version="2.0"><body><outline title="Contract Show" text="Tom&#39;s 简介与&CDATA" type="rss" xmlUrl="` + upstream.URL + `/feed.xml?a=1&amp;b=2"/></body></opml>`

	for _, streaming := range []bool{false, true} {
		name := "json"
		if streaming {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			router, db := newUploadContractRouter(t, streaming)
			response := buildOPMLUploadRequest(t, router, "subs.opml", []byte(opmlContent))
			require.Equal(t, http.StatusOK, response.Code)

			var podcast models.Podcast
			require.NoError(t, db.First(&podcast).Error)
			// 导入成功后资料以 RSS 为准；OPML 标题/描述仅用于待同步空壳。
			require.Equal(t, "Contract Feed", podcast.Title)
			require.Equal(t, upstream.URL+"/feed.xml?a=1&b=2", podcast.FeedURL)
			require.True(t, podcast.FeedURLValid)
			var count int64
			require.NoError(t, db.Model(&models.Podcast{}).Count(&count).Error)
			require.Equal(t, int64(1), count)
		})
	}
}

func TestOPMLUploadExtensionCaseInsensitiveOnBothEntries(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	opmlContent := `<opml version="2.0"><body><outline title="S" text="d" type="rss" xmlUrl="` + upstream.URL + `/f"/></body></opml>`
	for _, streaming := range []bool{false, true} {
		router, _ := newUploadContractRouter(t, streaming)
		response := buildOPMLUploadRequest(t, router, "subs.OPML", []byte(opmlContent))
		require.Equal(t, http.StatusOK, response.Code)
		require.Contains(t, response.Body.String(), `"stub_podcasts":1`)
	}
}

func TestOPMLUploadRejectsInvalidExtensionOnBothEntries(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		router, _ := newUploadContractRouter(t, streaming)
		response := buildOPMLUploadRequest(t, router, "subs.txt", []byte(`<opml/>`))
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Contains(t, response.Body.String(), "INVALID_FILE_FORMAT")
	}
}

func TestOPMLUploadBoundaryAndOversizeConsistent(t *testing.T) {
	// 恰好 8 MiB 的合法 OPML（用注释填充到边界）必须被接受。
	opmlContent := `<opml version="2.0"><body><outline title="Big" text="d" type="rss" xmlUrl="https://example.com/f"/></body></opml>`
	padding := middleware.MaxOPMLFileBytes - int64(len(opmlContent)) - 7 // "<!--" + "-->"
	opmlContent = opmlContent + "<!--" + strings.Repeat("p", int(padding)) + "-->"
	require.Equal(t, middleware.MaxOPMLFileBytes, int64(len(opmlContent)))

	for _, streaming := range []bool{false, true} {
		name := "json"
		if streaming {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			router, _ := newUploadContractRouter(t, streaming)
			response := buildOPMLUploadRequest(t, router, "big.opml", []byte(opmlContent))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"stub_podcasts":1`)

			oversize := append([]byte(nil), opmlContent...)
			oversize = append(oversize, 'x')
			response = buildOPMLUploadRequest(t, router, "big.opml", oversize)
			require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
		})
	}
}

func TestOPMLUploadEmptyAndMalformedFileRejectedBeforeWrite(t *testing.T) {
	cases := []struct {
		name    string
		content string
		code    string
	}{
		{"empty", "", "INVALID_OPML"},
		{"malformed", `<?xml version="1.0"?><opml><body><outline`, "INVALID_OPML"},
	}
	for _, streaming := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%s", map[bool]string{false: "json", true: "sse"}[streaming], tc.name), func(t *testing.T) {
				router, db := newUploadContractRouter(t, streaming)
				response := buildOPMLUploadRequest(t, router, "bad.opml", []byte(tc.content))
				require.Equal(t, http.StatusBadRequest, response.Code)
				require.Contains(t, response.Body.String(), tc.code)
				// 非法文件不得写入个人播客库。
				var count int64
				require.NoError(t, db.Model(&models.Podcast{}).Count(&count).Error)
				require.Zero(t, count)
			})
		}
	}
}

func TestOPMLUploadEmptyOutlineReportsZeroEntries(t *testing.T) {
	emptyOPML := `<?xml version="1.0" encoding="UTF-8"?><opml version="2.0"><head><title>t</title></head><body></body></opml>`

	router, _ := newUploadContractRouter(t, false)
	response := buildOPMLUploadRequest(t, router, "empty.opml", []byte(emptyOPML))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "没有订阅条目")

	sseRouter, _ := newUploadContractRouter(t, true)
	response = buildOPMLUploadRequest(t, sseRouter, "empty.opml", []byte(emptyOPML))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "没有订阅条目")
}

// TestOPMLUploadCleansTempFiles 保证导入完成后临时文件不残留。
func TestOPMLUploadCleansTempFiles(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	router, _ := newUploadContractRouter(t, false)
	workDir, err := os.Getwd()
	require.NoError(t, err)
	opmlContent := `<opml version="2.0"><body><outline title="S" text="d" type="rss" xmlUrl="` + upstream.URL + `/f"/></body></opml>`
	response := buildOPMLUploadRequest(t, router, "cleanup.opml", []byte(opmlContent))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	entries, err := os.ReadDir(filepath.Join(workDir, "data", "temp"))
	if err == nil {
		require.Empty(t, entries)
	}
}
