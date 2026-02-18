package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			Title:             string(rune('A' + i)) + " Test Podcast",
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
			"custom_cover_url": "https://example.com/custom-cover.jpg",
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
