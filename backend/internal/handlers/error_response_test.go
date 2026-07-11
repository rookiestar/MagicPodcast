package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "magicpodcast/internal/errors"
	"magicpodcast/internal/middleware"

	"github.com/gin-gonic/gin"
)

type responseTestHandler struct{}

func newResponseTestHandler() *responseTestHandler {
	return &responseTestHandler{}
}

func (h *responseTestHandler) GetItem(c *gin.Context) {
	id := c.Param("id")

	if id == "0" {
		middleware.NotFoundErrorResponse(c, "item", id)
		return
	}

	middleware.SuccessResponse(c, map[string]interface{}{
		"id":   id,
		"name": "Example Item",
	})
}

func (h *responseTestHandler) CreateItem(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.ValidationErrorResponse(c, "request body", "invalid format")
		return
	}

	if req.Name == "" {
		middleware.ValidationErrorResponse(c, "name", "is required")
		return
	}

	if req.Name == "conflict" {
		middleware.HandleError(c, apperrors.ConflictError("item", "already exists"))
		return
	}

	middleware.CreatedResponse(c, map[string]interface{}{
		"id":   1,
		"name": req.Name,
	})
}

func (h *responseTestHandler) InternalServerError(c *gin.Context) {
	middleware.HandleError(c, apperrors.InternalError("database connection failed"))
}

func TestErrorResponseHandler_GetItem_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupTestRouter()

	handler := newResponseTestHandler()
	router.GET("/items/:id", handler.GetItem)

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

func TestErrorResponseHandler_GetItem_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupTestRouter()

	handler := newResponseTestHandler()
	router.GET("/items/:id", handler.GetItem)

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

	errorDetail, ok := response["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected error detail")
	}

	if errorDetail["code"] != "NOT_FOUND" {
		t.Errorf("Expected error code NOT_FOUND, got %v", errorDetail["code"])
	}
}

func TestErrorResponseHandler_CreateItem_ValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupTestRouter()

	handler := newResponseTestHandler()
	router.POST("/items", handler.CreateItem)

	req, _ := http.NewRequest("POST", "/items", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestErrorResponseHandler_CreateItem_Conflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupTestRouter()

	handler := newResponseTestHandler()
	router.POST("/items", handler.CreateItem)

	req, _ := http.NewRequest("POST", "/items", strings.NewReader(`{"name":"conflict"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status 409, got %d", w.Code)
	}
}

func TestErrorResponseHandler_CreateItem_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupTestRouter()

	handler := newResponseTestHandler()
	router.POST("/items", handler.CreateItem)

	req, _ := http.NewRequest("POST", "/items", strings.NewReader(`{"name":"created"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}
}

func TestErrorResponseHandler_InternalServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := setupTestRouter()

	handler := newResponseTestHandler()
	router.GET("/internal-error", handler.InternalServerError)

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
