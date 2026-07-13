package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"magicpodcast/internal/database"
)

func TestRuntimeMetadataUsesReleaseEnvironment(t *testing.T) {
	t.Setenv("MAGICPODCAST_RELEASE_ID", "20260712T000000Z-test")
	t.Setenv("MAGICPODCAST_FRONTEND_BUILD_ID", "frontend-build-1")
	t.Setenv("MAGICPODCAST_SERVER_MODE", "release")

	metadata := runtimeMetadata()
	if metadata["release_id"] != "20260712T000000Z-test" {
		t.Fatalf("release_id = %v", metadata["release_id"])
	}
	if metadata["frontend_build_id"] != "frontend-build-1" {
		t.Fatalf("frontend_build_id = %v", metadata["frontend_build_id"])
	}
	if metadata["build_mode"] != "release" {
		t.Fatalf("build_mode = %v", metadata["build_mode"])
	}
}

func TestRuntimeMetadataDefaultsToUnknown(t *testing.T) {
	t.Setenv("MAGICPODCAST_RELEASE_ID", "")
	t.Setenv("MAGICPODCAST_FRONTEND_BUILD_ID", "")
	t.Setenv("MAGICPODCAST_SERVER_MODE", "")

	metadata := runtimeMetadata()
	for _, key := range []string{"release_id", "frontend_build_id", "build_mode"} {
		if metadata[key] != "unknown" {
			t.Fatalf("%s = %v, want unknown", key, metadata[key])
		}
	}
}

func TestReadinessFailsWhenDatabaseIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:health_ready_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	database.SetTestDB(db)
	t.Cleanup(database.ResetDB)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close test db: %v", err)
	}

	router := gin.New()
	router.GET("/ready", NewHealthHandler().Ready)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestLivenessDoesNotRequireDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/live", NewHealthHandler().Live)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/live", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
}
