package router

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

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
		"GET /api/v1/discovery/candidates/:episodeID",
		"GET /api/v1/discovery/reports",
		"GET /api/v1/discovery/reports/:id",
		"GET /api/v1/consumption/summary",
		"GET /api/v1/consumption/queues/:queue",
		"GET /api/v1/consumption/episodes/:episodeID",
		"PUT /api/v1/consumption/episodes/:episodeID/queue",
		"PUT /api/v1/consumption/episodes/:episodeID/placement",
		"DELETE /api/v1/consumption/episodes/:episodeID/queue",
		"PUT /api/v1/consumption/episodes/:episodeID/dismissed",
		"POST /api/v1/consumption/episodes/:episodeID/read",
		"POST /api/v1/consumption/episodes/:episodeID/in-progress",
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

func TestSetupRouterLeavesProcessingRecoveryToExplicitWorker(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	configPath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "configs", "config.example.yaml")
	if _, err := config.Load(configPath); err != nil {
		t.Fatalf("load test config: %v", err)
	}
	t.Setenv("MAGICPODCAST_DISABLE_SCHEDULER", "1")

	dsn := fmt.Sprintf("file:router_read_only_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open router database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get router database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	if err := database.ApplyMigrations(db); err != nil {
		t.Fatalf("apply router schema: %v", err)
	}

	now := time.Date(2026, 8, 24, 17, 0, 0, 0, time.UTC)
	podcast := models.Podcast{
		XYZID:        "router-processing-recovery",
		Title:        "Router processing recovery",
		FeedURL:      "https://example.com/router-processing-recovery.xml",
		IsSubscribed: true,
	}
	if err := db.Create(&podcast).Error; err != nil {
		t.Fatalf("create podcast: %v", err)
	}
	episode := models.Episode{
		PodcastID:     podcast.ID,
		Title:         "Router processing recovery episode",
		GUID:          "router-processing-recovery-episode",
		PublishedDate: now,
	}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}
	run := models.EpisodeProcessingRun{
		EpisodeID:       episode.ID,
		ProcessingKey:   strings.Repeat("1", 64),
		AudioDigest:     strings.Repeat("2", 64),
		PipelineVersion: "pipeline-v1",
		TriggerSource:   models.ProcessingTriggerManual,
		Status:          models.ProcessingRunStatusRunning,
		CurrentStep:     "episode_notes",
		AttemptCount:    1,
		MaxAttempts:     3,
		RetryDeadlineAt: now.Add(time.Hour),
		StartedAt:       &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create processing run: %v", err)
	}

	database.SetTestDB(db)
	t.Cleanup(func() {
		database.ResetDB()
		_ = sqlDB.Close()
	})
	if err := db.Exec("PRAGMA query_only = ON").Error; err != nil {
		t.Fatalf("enable query-only database: %v", err)
	}

	_ = SetupRouter()

	var reloaded models.EpisodeProcessingRun
	if err := db.First(&reloaded, run.ID).Error; err != nil {
		t.Fatalf("reload processing run: %v", err)
	}
	if reloaded.Status != models.ProcessingRunStatusRunning {
		t.Fatalf("processing status = %q, want unchanged running", reloaded.Status)
	}
	if reloaded.CurrentStep != "episode_notes" {
		t.Fatalf("processing step = %q, want unchanged episode_notes", reloaded.CurrentStep)
	}
}
