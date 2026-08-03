package router

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestFrontendAPIContractRoutesRegistered keeps the frontend's production
// request surface tied to the real backend router. It intentionally exercises
// route registration rather than the deleted frontend Mock handlers.
func TestFrontendAPIContractRoutesRegistered(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	configPath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "configs", "config.example.yaml")
	if _, err := config.Load(configPath); err != nil {
		t.Fatalf("load test config: %v", err)
	}

	dsn := fmt.Sprintf("file:frontend_api_contract_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open contract database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get contract database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := database.ApplyMigrations(db); err != nil {
		t.Fatalf("apply contract schema: %v", err)
	}

	database.SetTestDB(db)
	t.Cleanup(func() {
		database.ResetDB()
		_ = sqlDB.Close()
	})

	registered := make(map[string]struct{})
	for _, route := range SetupRouter().Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	frontendRoutes := []string{
		"GET /api/v1/discovery/candidates",
		"GET /api/v1/discovery/shortlist/today",
		"PUT /api/v1/discovery/candidates/:episodeID/decision",
		"GET /api/v1/podcasts",
		"POST /api/v1/podcasts/batch",
		"GET /api/v1/podcasts/:id",
		"GET /api/v1/podcasts/:id/episodes",
		"PUT /api/v1/podcasts/:id/custom-cover",
		"GET /api/v1/search",
		"GET /api/v1/tags",
		"POST /api/v1/tags",
		"GET /api/v1/tags/:id",
		"PUT /api/v1/tags/:id",
		"DELETE /api/v1/tags/:id",
		"GET /api/v1/podcasts/:id/notes",
		"PUT /api/v1/podcasts/:id/notes",
		"GET /api/v1/episodes/:id/notes",
		"PUT /api/v1/episodes/:id/notes",
		"GET /api/v1/podcasts/:id/tags",
		"POST /api/v1/podcasts/:id/tags",
		"DELETE /api/v1/podcasts/:id/tags/:tagId",
		"GET /api/v1/episodes/:id/tags",
		"POST /api/v1/episodes/:id/tags",
		"DELETE /api/v1/episodes/:id/tags/:tagId",
		"POST /api/v1/sync/import-sse",
		"POST /api/v1/sync/podcasts/metadata-sse",
		"GET /api/v1/sync/status",
		"GET /api/v1/workflows",
		"POST /api/v1/workflows",
		"GET /api/v1/workflows/:id",
		"PUT /api/v1/workflows/:id",
		"DELETE /api/v1/workflows/:id",
		"POST /api/v1/workflows/:id/toggle",
		"GET /api/v1/workflows/:id/jobs",
		"POST /api/v1/workflows/:id/trigger",
		"GET /api/v1/jobs/:id",
		"GET /api/v1/jobs/:id/report",
		"POST /api/v1/jobs/:id/regenerate-llm",
		"POST /api/v1/jobs/:id/compensate-failed",
		"GET /api/v1/scheduler/status",
		"POST /api/v1/scheduler/reload",
		"POST /api/v1/scheduler/workflows/:id/pause",
		"POST /api/v1/scheduler/workflows/:id/resume",
		"POST /api/v1/cache/clear",
	}

	for _, route := range frontendRoutes {
		if _, ok := registered[route]; !ok {
			t.Errorf("frontend API route is not registered by backend: %s", route)
		}
	}
}
