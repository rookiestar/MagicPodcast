package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/database"
	"magicpodcast/internal/handlers"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupPodcastTestDB 设置测试数据库
func setupPodcastTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file:podcast_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	// 自动迁移
	err = db.AutoMigrate(&models.Podcast{}, &models.Tag{}, &models.Episode{})
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	// 设置测试数据库为全局数据库
	database.SetTestDB(db)

	return db
}

// cleanupPodcastTestDB 清理测试数据库
func cleanupPodcastTestDB() {
	database.ResetDB()
	cache.GetCache().Clear()
}

// setupPodcastTestRouter 设置测试路由
func setupPodcastTestRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandlerMiddleware())

	return router
}

// createTestPodcasts 创建测试数据
func createTestPodcasts(db *gorm.DB, count int) []models.Podcast {
	podcasts := make([]models.Podcast, count)
	now := time.Now()

	for i := 0; i < count; i++ {
		podcasts[i] = models.Podcast{
			XYZID:             string(rune('A' + i)),
			Title:             string(rune('A'+i)) + " Test Podcast",
			Description:       "Description for podcast " + string(rune('A'+i)),
			Author:            "Author " + string(rune('A'+i)),
			CoverURL:          "https://example.com/cover" + string(rune('A'+i)) + ".jpg",
			FeedURL:           "https://example.com/feed" + string(rune('A'+i)) + ".xml",
			EpisodeCount:      (i + 1) * 10,
			NewestEpisodeDate: now.Add(-time.Duration(i*24) * time.Hour),
			IsSubscribed:      i%2 == 0,
			IsDead:            i == 2, // 第三个播客标记为失效
			AddedDate:         now.Add(-time.Duration(i*48) * time.Hour),
		}
		db.Create(&podcasts[i])
	}

	return podcasts
}

func TestPodcastHandler_ListSummaryUsesCardDescription(t *testing.T) {
	db := setupPodcastTestDB(t)
	defer cleanupPodcastTestDB()

	router := setupPodcastTestRouter(db)
	handler := handlers.NewPodcastHandler()
	router.GET("/api/v1/podcasts", handler.List)

	cache.GetCache().Clear()
	podcast := createTestPodcasts(db, 1)[0]
	longDescription := "<p>" + strings.Repeat("Podcast summary description ", 40) + "</p>"
	if err := db.Model(&podcast).Update("description", longDescription).Error; err != nil {
		t.Fatalf("Failed to update podcast description: %v", err)
	}

	req, _ := http.NewRequest("GET", "/api/v1/podcasts?page=1&page_size=1&view=summary", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	data := response["data"].([]interface{})
	description := data[0].(map[string]interface{})["description"].(string)
	if len([]rune(description)) > 163 {
		t.Fatalf("Expected summary description to stay compact, got %d runes", len([]rune(description)))
	}
	if strings.Contains(description, "<") || strings.Contains(description, ">") {
		t.Fatalf("Expected summary description to be plain text, got %q", description)
	}

	req, _ = http.NewRequest("GET", "/api/v1/podcasts?page=1&page_size=1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	data = response["data"].([]interface{})
	fullDescription := data[0].(map[string]interface{})["description"].(string)
	if len([]rune(fullDescription)) <= len([]rune(description)) {
		t.Fatal("Expected default list response to keep a longer description than summary view")
	}
}

// createTestTags 创建测试标签
func createTestTags(db *gorm.DB, count int) []models.Tag {
	tags := make([]models.Tag, count)

	for i := 0; i < count; i++ {
		tags[i] = models.Tag{
			Name:  "Tag " + string(rune('A'+i)),
			Color: "#FF0000",
		}
		db.Create(&tags[i])
	}

	return tags
}

// TestPodcastHandler_List 测试播客列表接口
func TestPodcastHandler_List(t *testing.T) {
	db := setupPodcastTestDB(t)
	defer cleanupPodcastTestDB()

	router := setupPodcastTestRouter(db)
	handler := handlers.NewPodcastHandler()

	// 清理缓存
	cache.GetCache().Clear()

	// 创建测试数据
	podcasts := createTestPodcasts(db, 5)

	router.GET("/api/v1/podcasts", handler.List)

	t.Run("Success - Basic List", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/podcasts?page=1&page_size=10", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		if response["success"] != true {
			t.Error("Expected success to be true")
		}

		data := response["data"].([]interface{})
		if len(data) != 5 {
			t.Errorf("Expected 5 podcasts, got %d", len(data))
		}

		pagination := response["pagination"].(map[string]interface{})
		if pagination["total"].(float64) != 5 {
			t.Errorf("Expected total 5, got %v", pagination["total"])
		}
	})

	t.Run("Success - Search Filter", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/podcasts?search=Test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		data := response["data"].([]interface{})
		if len(data) < 1 {
			t.Error("Expected at least 1 podcast matching search")
		}
	})

	t.Run("Success - Pagination", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/podcasts?page=1&page_size=2", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		data := response["data"].([]interface{})
		if len(data) != 2 {
			t.Errorf("Expected 2 podcasts (page_size=2), got %d", len(data))
		}

		pagination := response["pagination"].(map[string]interface{})
		if pagination["total_pages"].(float64) != 3 {
			t.Errorf("Expected 3 total pages, got %v", pagination["total_pages"])
		}
	})

	_ = podcasts // 使用变量避免编译警告
}

// TestPodcastHandler_Get 测试获取单个播客
func TestPodcastHandler_Get(t *testing.T) {
	db := setupPodcastTestDB(t)
	defer cleanupPodcastTestDB()

	router := setupPodcastTestRouter(db)
	handler := handlers.NewPodcastHandler()

	// 创建测试数据
	podcasts := createTestPodcasts(db, 3)

	router.GET("/api/v1/podcasts/:id", handler.Get)

	t.Run("Success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/podcasts/1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		if response["success"] != true {
			t.Error("Expected success to be true")
		}

		data := response["data"].(map[string]interface{})
		if data["id"].(float64) != 1 {
			t.Errorf("Expected id 1, got %v", data["id"])
		}

		if data["title"] != podcasts[0].Title {
			t.Errorf("Expected title %s, got %v", podcasts[0].Title, data["title"])
		}
	})

	t.Run("Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/podcasts/999", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", w.Code)
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		if response["success"] != false {
			t.Error("Expected success to be false")
		}
	})
}

// TestPodcastHandler_BatchGet 测试批量获取播客
func TestPodcastHandler_BatchGet(t *testing.T) {
	db := setupPodcastTestDB(t)
	defer cleanupPodcastTestDB()

	router := setupPodcastTestRouter(db)
	handler := handlers.NewPodcastHandler()

	// 创建测试数据
	createTestPodcasts(db, 5)

	router.POST("/api/v1/podcasts/batch", handler.BatchGet)

	t.Run("Success", func(t *testing.T) {
		body := map[string]interface{}{
			"ids": []uint{1, 2, 3},
		}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest("POST", "/api/v1/podcasts/batch", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		if response["success"] != true {
			t.Error("Expected success to be true")
		}

		data := response["data"].([]interface{})
		if len(data) != 3 {
			t.Errorf("Expected 3 podcasts, got %d", len(data))
		}
	})

	t.Run("Invalid Request - Empty IDs", func(t *testing.T) {
		body := map[string]interface{}{
			"ids": []uint{},
		}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest("POST", "/api/v1/podcasts/batch", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	t.Run("Invalid Request - Missing IDs", func(t *testing.T) {
		body := map[string]interface{}{}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest("POST", "/api/v1/podcasts/batch", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})
}

// TestPodcastHandler_UpdateCustomCover 测试更新自定义封面
func TestPodcastHandler_UpdateCustomCover(t *testing.T) {
	db := setupPodcastTestDB(t)
	defer cleanupPodcastTestDB()

	router := setupPodcastTestRouter(db)
	handler := handlers.NewPodcastHandler()

	// 创建测试数据
	createTestPodcasts(db, 3)

	router.PUT("/api/v1/podcasts/:id/custom-cover", handler.UpdateCustomCover)

	t.Run("Success", func(t *testing.T) {
		body := map[string]interface{}{
			"custom_cover_url":  "https://example.com/custom-cover.jpg",
			"confirmation_text": "OVERWRITE COVER 1",
		}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest("PUT", "/api/v1/podcasts/1/custom-cover", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &response)

		if response["success"] != true {
			t.Error("Expected success to be true")
		}

		data := response["data"].(map[string]interface{})
		if data["custom_cover_url"] != "https://example.com/custom-cover.jpg" {
			t.Errorf("Expected custom_cover_url, got %v", data["custom_cover_url"])
		}
	})

	t.Run("Missing confirmation does not write", func(t *testing.T) {
		body := map[string]interface{}{
			"custom_cover_url": "https://example.com/should-not-write.jpg",
		}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest("PUT", "/api/v1/podcasts/1/custom-cover", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusPreconditionRequired {
			t.Fatalf("Expected status 428, got %d: %s", w.Code, w.Body.String())
		}
		var podcast models.Podcast
		if err := db.First(&podcast, 1).Error; err != nil {
			t.Fatalf("load podcast: %v", err)
		}
		if podcast.CustomCoverURL == "https://example.com/should-not-write.jpg" {
			t.Fatal("missing confirmation must not update the database")
		}
	})

	t.Run("Not Found", func(t *testing.T) {
		body := map[string]interface{}{
			"custom_cover_url": "https://example.com/custom-cover.jpg",
		}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest("PUT", "/api/v1/podcasts/999/custom-cover", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", w.Code)
		}
	})

	t.Run("Invalid URL - Too Long", func(t *testing.T) {
		// 创建一个超过512字符的URL
		longURL := "https://example.com/"
		for i := 0; i < 600; i++ {
			longURL += "a"
		}

		body := map[string]interface{}{
			"custom_cover_url": longURL,
		}
		jsonBody, _ := json.Marshal(body)

		req, _ := http.NewRequest("PUT", "/api/v1/podcasts/1/custom-cover", bytes.NewBuffer(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400 for too long URL, got %d", w.Code)
		}
	})
}

// TestPodcastHandler_ListSupportsSubscriptionFilterAndExternalCount 验证
// “我的播客”关注筛选与源站总数字段（#377）。
func TestPodcastHandler_ListSupportsSubscriptionFilterAndExternalCount(t *testing.T) {
	db := setupPodcastTestDB(t)
	defer cleanupPodcastTestDB()

	router := setupPodcastTestRouter(db)
	handler := handlers.NewPodcastHandler()
	router.GET("/api/v1/podcasts", handler.List)

	cache.GetCache().Clear()
	// 共享内存库跨测试存活：先清空节目表，保证计数断言只针对本用例数据。
	if err := db.Exec("DELETE FROM podcasts_tags").Error; err != nil {
		t.Fatalf("clear podcasts_tags: %v", err)
	}
	if err := db.Exec("DELETE FROM podcasts").Error; err != nil {
		t.Fatalf("clear podcasts: %v", err)
	}
	// is_subscribed=false 受 GORM default:true 零值覆盖影响，必须以 map 显式落库。
	subscribed := models.Podcast{Title: "已关注节目", XYZID: "sub-pid", FeedURL: "https://example.com/sub.xml", IsSubscribed: true, EpisodeCount: 20}
	if err := db.Create(&subscribed).Error; err != nil {
		t.Fatalf("create subscribed: %v", err)
	}
	unsubscribedValues := map[string]any{
		"xyz_id": "unsub-pid", "title": "清单收录节目",
		"feed_url": "https://example.com/unsub.xml", "is_subscribed": false,
		"episode_count": 1, "external_episode_count": 200,
		"feed_url_valid": false,
	}
	if err := db.Model(&models.Podcast{}).Create(unsubscribedValues).Error; err != nil {
		t.Fatalf("create unsubscribed: %v", err)
	}
	feedlessValues := map[string]any{
		"xyz_id": "feedless-pid", "title": "无 Feed 节目",
		"feed_url": nil, "is_subscribed": false,
		"episode_count": 2, "feed_url_valid": false,
	}
	if err := db.Model(&models.Podcast{}).Create(feedlessValues).Error; err != nil {
		t.Fatalf("create feedless: %v", err)
	}

	fetchTitles := func(url string) []string {
		cache.GetCache().Clear()
		req, _ := http.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("Expected 200 for %s, got %d", url, w.Code)
		}
		var response struct {
			Data []struct {
				Title                string `json:"title"`
				IsSubscribed         bool   `json:"is_subscribed"`
				ExternalEpisodeCount int    `json:"external_episode_count"`
				FeedURL              string `json:"feed_url"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		titles := make([]string, 0, len(response.Data))
		for _, item := range response.Data {
			titles = append(titles, item.Title)
		}
		return titles
	}

	allTitles := fetchTitles("/api/v1/podcasts?subscription=all")
	if len(allTitles) != 3 {
		t.Fatalf("expected 3 podcasts, got %v", allTitles)
	}
	subscribedTitles := fetchTitles("/api/v1/podcasts?subscription=subscribed")
	if len(subscribedTitles) != 1 || subscribedTitles[0] != "已关注节目" {
		t.Fatalf("subscribed filter mismatch: %v", subscribedTitles)
	}
	unsubscribedTitles := fetchTitles("/api/v1/podcasts?subscription=unsubscribed")
	if len(unsubscribedTitles) != 2 {
		t.Fatalf("unsubscribed filter mismatch: %v", unsubscribedTitles)
	}

	// summary 视图带 external_episode_count；无 Feed 节目的 feed_url 输出为空。
	req, _ := http.NewRequest("GET", "/api/v1/podcasts?view=summary&subscription=unsubscribed", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var summary struct {
		Data []struct {
			Title                string `json:"title"`
			ExternalEpisodeCount int    `json:"external_episode_count"`
			FeedURL              string `json:"feed_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if len(summary.Data) != 2 {
		t.Fatalf("expected 2 unsubscribed summaries, got %d", len(summary.Data))
	}
	for _, item := range summary.Data {
		if item.Title == "清单收录节目" && item.ExternalEpisodeCount != 200 {
			t.Fatalf("external_episode_count mismatch: %+v", item)
		}
	}

	invalid := httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/podcasts?subscription=bogus", nil)
	router.ServeHTTP(invalid, req)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid subscription, got %d", invalid.Code)
	}
}
