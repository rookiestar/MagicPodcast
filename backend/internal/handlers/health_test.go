package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/runtimeprofile"
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
	if metadata["data_profile"] != "unmanaged" {
		t.Fatalf("data_profile = %v, want unmanaged", metadata["data_profile"])
	}
}

func TestReadinessIncludesManagedProfileWithoutDatabasePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "work", "fixture-instance")
	if err := os.MkdirAll(fixtureDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(fixtureDir, "basic-v1.db")
	db, err := gorm.Open(sqlite.Open(fixturePath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := database.ApplyMigrations(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	database.SetTestDB(db)
	t.Cleanup(database.ResetDB)
	config.SetTestConfig(&config.Config{
		Database: config.DatabaseConfig{Path: fixturePath},
	})
	t.Cleanup(func() { config.SetTestConfig(nil) })
	t.Setenv("MAGICPODCAST_SERVER_MODE", "debug")
	t.Setenv("MAGICPODCAST_DATA_PROFILE", "fixture")
	t.Setenv("MAGICPODCAST_DATA_PROFILE_HOME", root)
	t.Setenv("MAGICPODCAST_DATA_PROFILE_INSTANCE_ID", "fixture-instance")
	t.Setenv("MAGICPODCAST_FIXTURE_VERSION", "basic-v1")

	router := gin.New()
	router.GET("/ready", NewHealthHandler().Ready)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["data_profile"] != "fixture" || body["fixture_version"] != "basic-v1" {
		t.Fatalf("unexpected profile metadata: %#v", body)
	}
	if body["schema_version"] != float64(database.CurrentSchemaVersion) {
		t.Fatalf("schema_version=%v", body["schema_version"])
	}
	if _, found := body["database_path"]; found {
		t.Fatalf("readiness leaked database path: %#v", body)
	}
	if strings.Contains(recorder.Body.String(), fixturePath) {
		t.Fatalf("readiness leaked database path: %s", recorder.Body.String())
	}
}

func TestReadinessFailsWhenManagedProfileMetadataIsInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:health_invalid_profile?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.ApplyMigrations(db); err != nil {
		t.Fatal(err)
	}
	database.SetTestDB(db)
	t.Cleanup(database.ResetDB)
	config.SetTestConfig(&config.Config{
		Server:   config.ServerConfig{Mode: "debug"},
		Database: config.DatabaseConfig{Path: "/outside/production.db"},
	})
	t.Cleanup(func() { config.SetTestConfig(nil) })
	t.Setenv("MAGICPODCAST_DATA_PROFILE", "fixture")
	t.Setenv("MAGICPODCAST_DATA_PROFILE_HOME", t.TempDir())
	t.Setenv("MAGICPODCAST_DATA_PROFILE_INSTANCE_ID", "fixture-instance")
	t.Setenv("MAGICPODCAST_FIXTURE_VERSION", "basic-v1")

	router := gin.New()
	router.GET("/ready", NewHealthHandler().Ready)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503; body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "/outside/production.db") {
		t.Fatalf("readiness leaked database path: %s", recorder.Body.String())
	}
}

func TestHealthFailsWhenManagedProfileMetadataIsInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "health.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	database.SetTestDB(db)
	t.Cleanup(database.ResetDB)
	config.SetTestConfig(&config.Config{
		Server:   config.ServerConfig{Mode: "debug"},
		Database: config.DatabaseConfig{Path: "/outside/production.db"},
	})
	t.Cleanup(func() { config.SetTestConfig(nil) })
	t.Setenv("MAGICPODCAST_DATA_PROFILE", "fixture")
	t.Setenv("MAGICPODCAST_DATA_PROFILE_HOME", t.TempDir())
	t.Setenv("MAGICPODCAST_DATA_PROFILE_INSTANCE_ID", "fixture-instance")
	t.Setenv("MAGICPODCAST_FIXTURE_VERSION", "basic-v1")

	router := gin.New()
	router.GET("/health", NewHealthHandler().Health)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503; body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "/outside/production.db") {
		t.Fatalf("health leaked database path: %s", recorder.Body.String())
	}
}

func TestLivenessDoesNotDependOnManagedProfileMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalLoader := loadRuntimeProfile
	loadRuntimeProfile = func(string, string) (runtimeprofile.Metadata, error) {
		panic("liveness must not validate profile metadata")
	}
	t.Cleanup(func() { loadRuntimeProfile = originalLoader })

	router := gin.New()
	router.GET("/live", NewHealthHandler().Live)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/live", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, found := body["data_profile"]; found {
		t.Fatalf("liveness must contain only process metadata: %#v", body)
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
