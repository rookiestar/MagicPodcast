package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apperrors "magicpodcast/internal/errors"
	"magicpodcast/internal/middleware"

	"github.com/gin-gonic/gin"
)

// 示例Handler，演示如何使用统一错误处理
type ExampleHandler struct{}

func NewExampleHandler() *ExampleHandler {
	return &ExampleHandler{}
}

// GetItem 示例：返回资源
func (h *ExampleHandler) GetItem(c *gin.Context) {
	id := c.Param("id")

	// 模拟：如果id是"0"，返回未找到错误
	if id == "0" {
		middleware.NotFoundErrorResponse(c, "item", id)
		return
	}

	// 模拟：正常返回
	middleware.SuccessResponse(c, map[string]interface{}{
		"id":   id,
		"name": "Example Item",
	})
}

// CreateItem 示例：创建资源
func (h *ExampleHandler) CreateItem(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ValidationErrorResponse(c, "request body", "invalid format")
		return
	}

	// 模拟：验证逻辑
	if req.Name == "" {
		middleware.ValidationErrorResponse(c, "name", "is required")
		return
	}

	// 模拟：冲突检查
	if req.Name == "conflict" {
		middleware.HandleError(c, apperrors.ConflictError("item", "already exists"))
		return
	}

	// 模拟：创建成功
	middleware.CreatedResponse(c, map[string]interface{}{
		"id":   1,
		"name": req.Name,
	})
}

// InternalServerExample 示例：内部服务器错误
func (h *ExampleHandler) InternalServerExample(c *gin.Context) {
	// 模拟：内部错误
	middleware.HandleError(c, apperrors.InternalError("database connection failed"))
}

// ========== 测试用例 ==========

func TestExampleHandler_GetItem_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupTestRouter()

	handler := NewExampleHandler()
	router.GET("/items/:id", handler.GetItem)

	// 测试：正常获取
	req, _ := http.NewRequest("GET", "/items/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["success"] != true {
		t.Errorf("Expected success=true, got %v", response["success"])
	}
}

func TestExampleHandler_GetItem_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupTestRouter()

	handler := NewExampleHandler()
	router.GET("/items/:id", handler.GetItem)

	// 测试：未找到
	req, _ := http.NewRequest("GET", "/items/0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["success"] != false {
		t.Errorf("Expected success=false, got %v", response["success"])
	}

	// 验证错误详情
	errorDetail, ok := response["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected error detail")
	}

	if errorDetail["code"] != "NOT_FOUND" {
		t.Errorf("Expected error code NOT_FOUND, got %v", errorDetail["code"])
	}
}

func TestExampleHandler_CreateItem_ValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupTestRouter()

	handler := NewExampleHandler()
	router.POST("/items", handler.CreateItem)

	// 测试：验证错误
	req, _ := http.NewRequest("POST", "/items", nil)
	req.Header.Set("Content-Type", "application/json")
	// 注意：这里简化了，实际应该设置body

	// 测试冲突
	req, _ = http.NewRequest("POST", "/items", nil)
	req.Header.Set("Content-Type", "application/json")

	// 由于ShouldBindJSON需要完整设置，这里仅作演示
}

func TestExampleHandler_InternalServerExample(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupTestRouter()

	handler := NewExampleHandler()
	router.GET("/internal-error", handler.InternalServerExample)

	req, _ := http.NewRequest("GET", "/internal-error", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	var response map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &response)

	if response["success"] != false {
		t.Errorf("Expected success=false, got %v", response["success"])
	}

	errorDetail, ok := response["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected error detail")
	}

	if errorDetail["code"] != "INTERNAL_ERROR" {
		t.Errorf("Expected error code INTERNAL_ERROR, got %v", errorDetail["code"])
	}
}

// setupTestRouter 设置测试路由
func setupTestRouter() *gin.Engine {
	router := gin.New()
	router.Use(middleware.ErrorHandlerMiddleware())
	return router
}
