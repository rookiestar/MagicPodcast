package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"magicpodcast/internal/config"
)

func TestLLMHealthIsPassiveWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configText := []byte(`server:
  host: 127.0.0.1
  port: 8080
  mode: release
database:
  path: ./data/test.db
xyz_api:
  url: http://127.0.0.1:8081
llm:
  enabled: true
  api_key: test-key-that-is-long-enough
  default_model: test-model
`)
	if err := os.WriteFile(configPath, configText, 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := config.Load(configPath); err != nil {
		t.Fatalf("load config: %v", err)
	}

	router := gin.New()
	router.GET("/health", NewLLMHealthHandler(nil).GetHealth)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		Data LLMHealthResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.ProbePerformed {
		t.Fatal("passive health read must not perform a probe")
	}
	if body.Data.Source != "passive" {
		t.Fatalf("source = %q, want passive", body.Data.Source)
	}
	if body.Data.Status != "unknown" {
		t.Fatalf("status = %q, want unknown", body.Data.Status)
	}
}
