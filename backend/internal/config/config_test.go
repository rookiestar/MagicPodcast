package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadAppliesRuntimeEnvOverrides(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("MAGICPODCAST_SERVER_MODE", "release")
	t.Setenv("MAGICPODCAST_SERVER_PORT", "18080")
	t.Setenv("MAGICPODCAST_DATABASE_DEBUG", "false")

	writeTestConfig(t, configPath, `
server:
  port: 8080
  mode: debug
database:
  path: ./data/test.db
  debug: true
xyz_api:
  url: http://localhost:8081
`)

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Server.Mode != "release" {
		t.Fatalf("Server.Mode = %q, want release", loaded.Server.Mode)
	}
	if loaded.Server.Port != 18080 {
		t.Fatalf("Server.Port = %d, want 18080", loaded.Server.Port)
	}
	if loaded.Database.Debug {
		t.Fatal("Database.Debug = true, want false")
	}
}

func writeTestConfig(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
}
