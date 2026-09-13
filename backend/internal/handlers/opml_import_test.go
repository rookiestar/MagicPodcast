package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOPMLUploadRetainsUnavailableSubscription(t *testing.T) {
	t.Chdir(t.TempDir())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	defer upstream.Close()
	for _, streaming := range []bool{false, true} {
		name := "json"
		if streaming {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			defer sqlDB.Close()
			require.NoError(t, db.AutoMigrate(&models.Podcast{}))
			service, err := syncsvc.NewService(db, "")
			require.NoError(t, err)
			defer service.Close()
			handler := &SyncHandler{syncService: service}
			router := gin.New()
			if streaming {
				router.POST("/import", handler.ImportOPMLSSE)
			} else {
				router.POST("/import", handler.ImportOPML)
			}
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, err := writer.CreateFormFile("opml_file", "subscriptions.opml")
			require.NoError(t, err)
			_, err = part.Write([]byte(`<opml version="2.0"><body><outline text="分类"><outline title="正确标题" text="节目简介" xmlUrl="` + upstream.URL + `/feed.xml"/></outline></body></opml>`))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			req := httptest.NewRequest(http.MethodPost, "/import", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("X-MagicPodcast-Confirmation", "IMPORT OPML")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, http.StatusOK, response.Code)
			require.Contains(t, response.Body.String(), `"stub_podcasts":1`)
			if streaming {
				require.True(t, strings.HasPrefix(response.Header().Get("Content-Type"), "text/event-stream"))
				require.Contains(t, response.Body.String(), `"type":"summary"`)
			}
			var podcast models.Podcast
			require.NoError(t, db.First(&podcast).Error)
			require.Equal(t, "正确标题", podcast.Title)
			require.Equal(t, "节目简介", podcast.Description)
			require.True(t, podcast.IsSubscribed)
			require.False(t, podcast.FeedURLValid)
		})
	}
}
