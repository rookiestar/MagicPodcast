package dataprofile

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	sharedBackendOnce sync.Once
	sharedBackendDir  string
	sharedBackendPath string
	sharedBackendErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedBackendDir != "" {
		_ = os.RemoveAll(sharedBackendDir)
	}
	os.Exit(code)
}

func withSharedTestBackend(t *testing.T, controller Controller) Controller {
	t.Helper()
	sharedBackendOnce.Do(func() {
		projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
		if err != nil {
			sharedBackendErr = fmt.Errorf("resolve project directory: %w", err)
			return
		}
		sharedBackendDir, err = os.MkdirTemp("", "magicpodcast-dataprofile-backend-")
		if err != nil {
			sharedBackendErr = fmt.Errorf("create shared backend directory: %w", err)
			return
		}
		sharedBackendDir, err = filepath.EvalSymlinks(sharedBackendDir)
		if err != nil {
			sharedBackendErr = fmt.Errorf("resolve shared backend directory: %w", err)
			return
		}
		sharedBackendPath = filepath.Join(sharedBackendDir, "magicpodcast-api")
		command := exec.Command("go", "build", "-o", sharedBackendPath, "./cmd/api")
		command.Dir = filepath.Join(projectDir, "backend")
		output, err := command.CombinedOutput()
		if err != nil {
			sharedBackendErr = fmt.Errorf(
				"build shared test backend: %w: %s",
				err,
				strings.TrimSpace(string(output)),
			)
		}
	})
	require.NoError(t, sharedBackendErr)
	controller.prepareBackendHook = func(_ context.Context, instanceID string) ([]string, error) {
		profileRoot, err := ensureRoot(controller.ProfileHome)
		if err != nil {
			return nil, err
		}
		binDir := filepath.Join(profileRoot, "bin")
		if err := os.MkdirAll(binDir, 0o700); err != nil {
			return nil, err
		}
		commandPath := filepath.Join(binDir, "magicpodcast-api-"+instanceID)
		if err := copyRegularFile(sharedBackendPath, commandPath, 0o700); err != nil {
			return nil, err
		}
		return []string{commandPath}, nil
	}
	return controller
}

func prepareSanitizedTestDatabase(t *testing.T, sourcePath string) string {
	t.Helper()
	preparedPath := filepath.Join(t.TempDir(), "prepared.db")
	require.NoError(t, copyRegularFile(sourcePath, preparedPath, 0o600))
	db, err := sql.Open("sqlite3", "file:"+preparedPath+"?_foreign_keys=on&_busy_timeout=5000")
	require.NoError(t, err)
	require.NoError(t, SanitizeSnapshot(db))
	require.NoError(t, db.Close())
	return preparedPath
}

func publishPreparedTestSnapshot(
	t *testing.T,
	profileHome string,
	id string,
	sourcePath string,
	capturedAt string,
	sanitizerVersion string,
) (Snapshot, error) {
	t.Helper()
	preparedPath := sourcePath
	if sanitizerVersion == SanitizerVersion {
		preparedPath = prepareSanitizedTestDatabase(t, sourcePath)
	}
	return PublishPreparedSnapshot(
		profileHome,
		id,
		preparedPath,
		capturedAt,
		sanitizerVersion,
	)
}
