package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestImageHandler_Health(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	imageHandler := NewImageHandler()
	router.GET("/images/health", imageHandler.Health)

	req, _ := http.NewRequest("GET", "/images/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "image-proxy")
}
