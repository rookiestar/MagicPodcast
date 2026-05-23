package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestLLMGetModelsIncludesDeepSeekFlash(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := NewLLMConfigHandler(nil)
	router.GET("/models", handler.GetModels)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/models", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Available []ModelInfo `json:"available"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)

	found := false
	for _, model := range body.Data.Available {
		if model.ID == "deepseek-v4-flash" {
			found = true
			require.Equal(t, "DeepSeek V4 Flash", model.Name)
			require.True(t, model.Available)
			break
		}
	}
	require.True(t, found, "deepseek-v4-flash should be listed")
}

func TestLLMValidateKeyRejectsShortKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := NewLLMConfigHandler(nil)
	router.POST("/validate-key", handler.ValidateKey)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/validate-key",
		strings.NewReader(`{"api_key":"short","model":"deepseek-v4-flash"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)

	var body struct {
		Success bool                `json:"success"`
		Data    ValidateKeyResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.False(t, body.Data.Valid)
	require.NotEmpty(t, body.Data.TestError)
}
