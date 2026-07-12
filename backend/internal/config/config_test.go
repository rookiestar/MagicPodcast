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
	t.Setenv("MAGICPODCAST_SERVER_HOST", "127.0.0.1")
	t.Setenv("MAGICPODCAST_SERVER_PORT", "18080")
	t.Setenv("MAGICPODCAST_DATABASE_DEBUG", "false")

	writeTestConfig(t, configPath, `
server:
  host: 127.0.0.1
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
	if loaded.Server.Host != "127.0.0.1" {
		t.Fatalf("Server.Host = %q, want 127.0.0.1", loaded.Server.Host)
	}
	if loaded.Database.Debug {
		t.Fatal("Database.Debug = true, want false")
	}
}

func TestLoadDefaultsServerHostToLoopback(t *testing.T) {
	t.Cleanup(func() {
		cfg = nil
		viper.Reset()
	})
	viper.Reset()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeTestConfig(t, configPath, `
server:
  port: 8080
  mode: release
database:
  path: ./data/test.db
xyz_api:
  url: http://localhost:8081
`)

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Server.Host != "127.0.0.1" {
		t.Fatalf("Server.Host = %q, want 127.0.0.1", loaded.Server.Host)
	}
}

func TestValidateRejectsNonLoopbackServerHost(t *testing.T) {
	base := Config{
		Server:   ServerConfig{Port: 8080, Mode: "release"},
		Database: DatabaseConfig{Path: "./data/test.db"},
		XYZAPI:   XYZAPIConfig{URL: "http://127.0.0.1:8081"},
	}

	for _, host := range []string{"", "localhost", "0.0.0.0", "192.168.1.10", "example.com"} {
		t.Run(host, func(t *testing.T) {
			config := base
			config.Server.Host = host
			if err := config.Validate(); err == nil {
				t.Fatalf("Validate() succeeded for non-loopback host %q", host)
			}
		})
	}

	for _, host := range []string{"127.0.0.1", "::1"} {
		t.Run(host, func(t *testing.T) {
			config := base
			config.Server.Host = host
			if err := config.Validate(); err != nil {
				t.Fatalf("Validate() error for loopback host %q: %v", host, err)
			}
		})
	}
}

func writeTestConfig(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
}
