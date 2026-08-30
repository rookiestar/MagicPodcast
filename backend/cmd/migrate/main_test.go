package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequireMigrationMaintenanceWindow(t *testing.T) {
	lockDir := t.TempDir()
	t.Setenv("MAGICPODCAST_DEPLOY_LOCK_DIR", lockDir)
	ownerPID := fmt.Sprintf("%d", os.Getpid())
	ownerStartBytes, err := exec.Command("ps", "-p", ownerPID, "-o", "lstart=").Output()
	require.NoError(t, err)
	ownerStart := strings.TrimSpace(string(ownerStartBytes))
	t.Setenv("MAGICPODCAST_MAINTENANCE_OWNER_PID", ownerPID)
	t.Setenv("MAGICPODCAST_MAINTENANCE_OWNER_START", ownerStart)
	require.Error(t, requireMigrationMaintenanceWindow())

	for name, value := range map[string]string{
		"owner.pid":        ownerPID + "\n",
		"owner.started_at": ownerStart + "\n",
		"operation":        "migration\n",
		"state":            "critical\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(lockDir, name), []byte(value), 0o600))
	}
	require.NoError(t, requireMigrationMaintenanceWindow())

	require.NoError(t, os.WriteFile(filepath.Join(lockDir, "operation"), []byte("deploy\n"), 0o600))
	require.Error(t, requireMigrationMaintenanceWindow())
}

func TestResolveTargetCommitRequiresCleanMatchingCheckout(t *testing.T) {
	repository := t.TempDir()
	t.Chdir(repository)
	for _, args := range [][]string{
		{"init"},
		{"config", "user.name", "Migration Test"},
		{"config", "user.email", "migration-test@example.invalid"},
	} {
		require.NoError(t, exec.Command("git", args...).Run())
	}
	require.NoError(t, os.WriteFile("tracked.txt", []byte("clean\n"), 0o600))
	require.NoError(t, exec.Command("git", "add", "tracked.txt").Run())
	require.NoError(t, exec.Command("git", "commit", "-m", "fixture").Run())
	headBytes, err := exec.Command("git", "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	head := strings.TrimSpace(string(headBytes))
	t.Setenv("MAGICPODCAST_TARGET_COMMIT", head)
	resolved, err := resolveTargetCommit()
	require.NoError(t, err)
	require.Equal(t, head, resolved)

	t.Setenv("MAGICPODCAST_TARGET_COMMIT", "0000000000000000000000000000000000000000")
	_, err = resolveTargetCommit()
	require.ErrorContains(t, err, "does not match")
	t.Setenv("MAGICPODCAST_TARGET_COMMIT", head)
	require.NoError(t, os.WriteFile("tracked.txt", []byte("dirty\n"), 0o600))
	_, err = resolveTargetCommit()
	require.ErrorContains(t, err, "must be clean")
}
