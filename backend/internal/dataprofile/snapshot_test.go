package dataprofile

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"magicpodcast/internal/database"
)

func TestPublishAndResolveSnapshotLatest(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	first, err := publishPreparedTestSnapshot(t, home, "snapshot-001", fixture.DatabasePath, "2026-08-01T08:00:00Z", SanitizerVersion)
	require.NoError(t, err)
	second, err := publishPreparedTestSnapshot(t, home, "snapshot-002", fixture.DatabasePath, "2026-08-02T08:00:00Z", SanitizerVersion)
	require.NoError(t, err)

	latest, err := ResolveSnapshot(home, "latest")
	require.NoError(t, err)
	require.Equal(t, second.ID, latest.ID)
	explicit, err := ResolveSnapshot(home, first.ID)
	require.NoError(t, err)
	require.Equal(t, first.ID, explicit.ID)

	info, err := os.Stat(latest.DatabasePath)
	require.NoError(t, err)
	require.Zero(t, info.Mode().Perm()&0o222)
}

func TestPublishPreparedSnapshotRejectsUnsanitizedInputWithoutChangingSource(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	sourcePath := filepath.Join(t.TempDir(), "unsanitized.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, sourcePath, 0o600))
	db, err := sql.Open("sqlite3", sourcePath)
	require.NoError(t, err)
	_, err = db.Exec(`
		UPDATE episodes
		SET show_notes = 'https://private.example/note?token=TOP-SECRET'
		WHERE id = 2001`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	sourceHash, err := SHA256File(sourcePath)
	require.NoError(t, err)

	_, err = PublishPreparedSnapshot(
		home,
		"snapshot-unsanitized",
		sourcePath,
		"2026-08-01T08:00:00Z",
		SanitizerVersion,
	)
	require.ErrorContains(t, err, "not sanitized")
	afterHash, err := SHA256File(sourcePath)
	require.NoError(t, err)
	require.Equal(t, sourceHash, afterHash)
	_, statErr := os.Stat(filepath.Join(home, "snapshots", "snapshot-unsanitized"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestResolveSnapshotRejectsInvalidInputs(t *testing.T) {
	home := t.TempDir()
	_, err := ResolveSnapshot(home, "latest")
	require.ErrorContains(t, err, "no snapshots")
	_, err = ResolveSnapshot(home, "../../production.db")
	require.ErrorContains(t, err, "selector is invalid")

	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	snapshot, err := publishPreparedTestSnapshot(t, home, "snapshot-bad", fixture.DatabasePath, "2026-08-01T08:00:00Z", SanitizerVersion)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(snapshot.DatabasePath, 0o600))
	_, err = ResolveSnapshot(home, "snapshot-bad")
	require.ErrorContains(t, err, "must be read-only")
}

func TestResolveSnapshotRejectsTamperedLatestPointer(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	snapshot, err := publishPreparedTestSnapshot(t,
		home,
		"snapshot-latest-pointer",
		fixture.DatabasePath,
		"2026-08-01T08:00:00Z",
		SanitizerVersion,
	)
	require.NoError(t, err)
	controller := Controller{ProfileHome: home}
	require.NoError(t, controller.writeLatest(snapshot))
	latestPath := filepath.Join(home, "snapshots", "latest.json")
	require.NoError(t, os.Chmod(latestPath, 0o600))

	target := filepath.Join(t.TempDir(), "latest.json")
	require.NoError(t, os.WriteFile(target, []byte("{}"), 0o600))
	require.NoError(t, os.Remove(latestPath))
	require.NoError(t, os.Symlink(target, latestPath))
	_, err = ResolveSnapshot(home, "latest")
	require.ErrorContains(t, err, "not a regular file")

	require.NoError(t, os.Remove(latestPath))
	require.NoError(t, os.WriteFile(latestPath, []byte(
		`{"format_version":1,"snapshot_id":"`+snapshot.ID+`","captured_at":"2026-08-02T08:00:00Z"}`,
	), 0o600))
	_, err = ResolveSnapshot(home, "latest")
	require.ErrorContains(t, err, "does not match")

	require.NoError(t, os.Chmod(latestPath, 0o644))
	_, err = ResolveSnapshot(home, "latest")
	require.ErrorContains(t, err, "permissions are too broad")
}

func TestResolveSnapshotRejectsManifestCountMismatch(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	snapshot, err := publishPreparedTestSnapshot(t, home, "snapshot-counts", fixture.DatabasePath, "2026-08-01T08:00:00Z", SanitizerVersion)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(snapshot.ManifestPath, 0o600))
	manifest, err := ReadManifest(snapshot.ManifestPath)
	require.NoError(t, err)
	manifest.Counts["podcasts"]++
	require.NoError(t, WriteManifest(snapshot.ManifestPath, manifest))
	require.NoError(t, os.Chmod(snapshot.ManifestPath, 0o400))

	_, err = ResolveSnapshot(home, snapshot.ID)
	require.ErrorContains(t, err, "manifest records")
}

func TestResolveSnapshotRejectsMissingRequiredManifestCount(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	snapshot, err := publishPreparedTestSnapshot(t,
		home,
		"snapshot-missing-count",
		fixture.DatabasePath,
		"2026-08-01T08:00:00Z",
		SanitizerVersion,
	)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(snapshot.ManifestPath, 0o600))
	manifest, err := ReadManifest(snapshot.ManifestPath)
	require.NoError(t, err)
	delete(manifest.Counts, "podcasts")
	require.NoError(t, WriteManifest(snapshot.ManifestPath, manifest))
	require.NoError(t, os.Chmod(snapshot.ManifestPath, 0o400))

	_, err = ResolveSnapshot(home, snapshot.ID)
	require.ErrorContains(t, err, "does not record required table count")
}

func TestResolveSnapshotRejectsUnsupportedSanitizerVersion(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	_, err = publishPreparedTestSnapshot(t,
		home,
		"snapshot-unsupported-sanitizer",
		fixture.DatabasePath,
		"2026-08-01T08:00:00Z",
		"future-v2",
	)
	require.ErrorContains(t, err, "unsupported sanitizer version")
}

func TestResolveSnapshotRejectsCorruptAndOldSchemaSnapshots(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)

	corrupt, err := publishPreparedTestSnapshot(t,
		home,
		"snapshot-corrupt",
		fixture.DatabasePath,
		"2026-08-01T08:00:00Z",
		SanitizerVersion,
	)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(corrupt.DatabasePath, 0o600))
	require.NoError(t, os.WriteFile(corrupt.DatabasePath, []byte("not sqlite"), 0o400))
	_, err = ResolveSnapshot(home, corrupt.ID)
	require.ErrorContains(t, err, "checksum")

	oldDir := filepath.Join(home, "snapshots", "snapshot-old")
	require.NoError(t, os.MkdirAll(oldDir, 0o700))
	oldDatabase := filepath.Join(oldDir, "magicpodcast.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, oldDatabase, 0o400))
	oldHash, err := SHA256File(oldDatabase)
	require.NoError(t, err)
	oldManifest := Manifest{
		FormatVersion:    ManifestFormatVersion,
		Kind:             "snapshot",
		ID:               "snapshot-old",
		SchemaVersion:    database.CurrentSchemaVersion - 1,
		CapturedAt:       "2026-08-01T08:00:00Z",
		SanitizerVersion: SanitizerVersion,
		SHA256:           oldHash,
	}
	require.NoError(t, WriteManifest(filepath.Join(oldDir, "manifest.json"), oldManifest))
	require.NoError(t, os.Chmod(filepath.Join(oldDir, "manifest.json"), 0o400))
	_, err = ResolveSnapshot(home, "snapshot-old")
	require.ErrorContains(t, err, "manifest schema version")
}

func TestResolveSnapshotRejectsMissingRequiredTable(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	directory := filepath.Join(home, "snapshots", "snapshot-missing-table")
	require.NoError(t, os.MkdirAll(directory, 0o700))
	databasePath := filepath.Join(directory, "magicpodcast.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, databasePath, 0o600))
	db, err := sql.Open("sqlite3", databasePath)
	require.NoError(t, err)
	_, err = db.Exec("DROP TABLE sync_configs")
	require.NoError(t, err)
	require.NoError(t, db.Close())
	hash, err := SHA256File(databasePath)
	require.NoError(t, err)
	require.NoError(t, WriteManifest(filepath.Join(directory, "manifest.json"), Manifest{
		FormatVersion:    ManifestFormatVersion,
		Kind:             "snapshot",
		ID:               "snapshot-missing-table",
		SchemaVersion:    database.CurrentSchemaVersion,
		CapturedAt:       "2026-08-01T08:00:00Z",
		SanitizerVersion: SanitizerVersion,
		SHA256:           hash,
		Counts:           fixture.Manifest.Counts,
	}))
	require.NoError(t, os.Chmod(databasePath, 0o400))
	require.NoError(t, os.Chmod(filepath.Join(directory, "manifest.json"), 0o400))

	_, err = ResolveSnapshot(home, "snapshot-missing-table")
	require.ErrorContains(t, err, "required tables missing")
}

func TestResolveSnapshotRejectsSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	snapshotDir := filepath.Join(home, "snapshots", "snapshot-link")
	require.NoError(t, os.MkdirAll(snapshotDir, 0o700))
	require.NoError(t, os.Symlink(fixture.DatabasePath, filepath.Join(snapshotDir, "magicpodcast.db")))
	require.NoError(t, os.Symlink(fixture.ManifestPath, filepath.Join(snapshotDir, "manifest.json")))

	_, err = ResolveSnapshot(home, "snapshot-link")
	require.ErrorContains(t, err, "outside the managed directory")
}

func TestControllerSwitchesFixtureSnapshotFixtureAndPreservesBaseline(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	snapshotSource := prepareSanitizedTestDatabase(t, fixture.DatabasePath)
	snapshotDB, err := sql.Open("sqlite3", snapshotSource)
	require.NoError(t, err)
	_, err = snapshotDB.Exec("UPDATE podcasts SET title = 'Snapshot：独立数据源' WHERE id = 1001")
	require.NoError(t, err)
	require.NoError(t, snapshotDB.Close())
	snapshot, err := PublishPreparedSnapshot(home, "snapshot-001", snapshotSource, "2026-08-01T08:00:00Z", SanitizerVersion)
	require.NoError(t, err)
	baselineHash, err := SHA256File(snapshot.DatabasePath)
	require.NoError(t, err)
	port, err := FreeLoopbackPort()
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  projectDir,
		ProfileHome: home,
		Port:        port,
		Timeout:     30 * time.Second,
	}
	t.Cleanup(func() {
		if state, readErr := controller.readState(); readErr == nil {
			_ = controller.stop(state)
		}
	})

	fixtureStatus, err := controller.UseFixture(context.Background())
	require.NoError(t, err)
	require.Equal(t, "fixture", fixtureStatus.Profile)
	require.Contains(t, getPodcastListBody(t, port), "Fixture：深度科技")
	fixtureState, err := controller.readState()
	require.NoError(t, err)
	snapshotStatus, err := controller.UseSnapshot(context.Background(), "latest")
	require.NoError(t, err)
	require.Equal(t, "snapshot", snapshotStatus.Profile)
	require.Equal(t, "snapshot-001", snapshotStatus.SnapshotID)
	require.Equal(t, "2026-08-01T08:00:00Z", snapshotStatus.SnapshotCapturedAt)
	require.Contains(t, getPodcastListBody(t, port), "Snapshot：独立数据源")
	require.NoFileExists(t, fixtureState.DatabasePath)
	require.NoFileExists(t, fixtureState.CommandPath)
	snapshotState, err := controller.readState()
	require.NoError(t, err)

	coverRequest, err := http.NewRequest(
		http.MethodPut,
		"http://127.0.0.1:"+portString(port)+"/api/v1/podcasts/1001/custom-cover",
		bytes.NewBufferString(`{"custom_cover_url":"https://work-copy.invalid/cover.jpg","confirmation_text":"OVERWRITE COVER 1001"}`),
	)
	require.NoError(t, err)
	coverRequest.Header.Set("Content-Type", "application/json")
	coverResponse, err := http.DefaultClient.Do(coverRequest)
	require.NoError(t, err)
	coverBody, err := io.ReadAll(coverResponse.Body)
	require.NoError(t, err)
	require.NoError(t, coverResponse.Body.Close())
	require.Equal(t, http.StatusOK, coverResponse.StatusCode, string(coverBody))
	afterWriteHash, err := SHA256File(snapshot.DatabasePath)
	require.NoError(t, err)
	require.Equal(t, baselineHash, afterWriteHash, "business writes must not change the snapshot baseline")

	reselected, err := controller.UseSnapshot(context.Background(), "snapshot-001")
	require.NoError(t, err)
	require.NotEqual(t, snapshotStatus.InstanceID, reselected.InstanceID)
	require.NoFileExists(t, snapshotState.DatabasePath)
	require.NoFileExists(t, snapshotState.CommandPath)
	detailResponse, err := http.Get("http://127.0.0.1:" + portString(port) + "/api/v1/podcasts/1001")
	require.NoError(t, err)
	detailBody, err := io.ReadAll(detailResponse.Body)
	require.NoError(t, err)
	require.NoError(t, detailResponse.Body.Close())
	require.Equal(t, http.StatusOK, detailResponse.StatusCode, string(detailBody))
	var detail struct {
		Data struct {
			CustomCoverURL string `json:"custom_cover_url"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(detailBody, &detail))
	require.Empty(t, detail.Data.CustomCoverURL, "reselection must rebuild a clean work copy")

	finalFixture, err := controller.UseFixture(context.Background())
	require.NoError(t, err)
	require.Equal(t, "fixture", finalFixture.Profile)
	require.Contains(t, getPodcastListBody(t, port), "Fixture：深度科技")

	afterHash, err := SHA256File(snapshot.DatabasePath)
	require.NoError(t, err)
	require.Equal(t, baselineHash, afterHash)
}

func TestDataProfileWrapperRefreshesAndSwitchesFixtureSnapshotFixture(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	home := t.TempDir()
	cliDir := t.TempDir()
	port, err := FreeLoopbackPort()
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  projectDir,
		ProfileHome: home,
		Port:        port,
		Timeout:     30 * time.Second,
	}
	t.Cleanup(func() {
		if state, readErr := controller.readState(); readErr == nil {
			_ = controller.stop(state)
		}
	})

	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	sourcePath := filepath.Join(t.TempDir(), "snapshot-source.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, sourcePath, 0o600))
	sourceDB, err := sql.Open("sqlite3", sourcePath)
	require.NoError(t, err)
	_, err = sourceDB.Exec("UPDATE podcasts SET title = 'Snapshot：统一命令数据源' WHERE id = 1001")
	require.NoError(t, err)
	require.NoError(t, sourceDB.Close())
	transferDir := t.TempDir()
	_, err = ExportSanitizedSnapshot(
		sourcePath,
		transferDir,
		"snapshot-command-journey",
		"2026-08-13T00:00:00Z",
	)
	require.NoError(t, err)

	fixtureOutput := runDataProfileWrapper(
		t, projectDir, home, cliDir, port,
		"use", "fixture",
	)
	require.Contains(t, fixtureOutput, "profile=fixture")
	require.Contains(t, fixtureOutput, "ready=true")
	require.Contains(t, getPodcastListBody(t, port), "Fixture：深度科技")

	refreshOutput := runDataProfileWrapper(
		t, projectDir, home, cliDir, port,
		"--transfer-dir", transferDir,
		"--confirm-refresh", RefreshConfirmation,
		"snapshot", "refresh",
	)
	require.Contains(t, refreshOutput, "profile=not-switched")
	require.Contains(t, refreshOutput, "snapshot_id=snapshot-command-journey")
	statusAfterRefresh := runDataProfileWrapper(
		t, projectDir, home, cliDir, port,
		"status",
	)
	require.Contains(t, statusAfterRefresh, "profile=fixture")
	require.Contains(t, statusAfterRefresh, "ready=true")
	require.Contains(t, getPodcastListBody(t, port), "Fixture：深度科技")

	snapshot, err := ResolveSnapshot(home, "latest")
	require.NoError(t, err)
	baselineHash, err := SHA256File(snapshot.DatabasePath)
	require.NoError(t, err)
	snapshotOutput := runDataProfileWrapper(
		t, projectDir, home, cliDir, port,
		"use", "snapshot", "latest",
	)
	require.Contains(t, snapshotOutput, "profile=snapshot")
	require.Contains(t, snapshotOutput, "snapshot_id=snapshot-command-journey")
	require.Contains(t, getPodcastListBody(t, port), "Snapshot：统一命令数据源")

	readyResponse, err := http.Get("http://127.0.0.1:" + portString(port) + "/ready")
	require.NoError(t, err)
	readyBody, err := io.ReadAll(readyResponse.Body)
	require.NoError(t, err)
	require.NoError(t, readyResponse.Body.Close())
	require.Equal(t, http.StatusOK, readyResponse.StatusCode, string(readyBody))
	require.Contains(t, string(readyBody), `"data_profile":"snapshot"`)
	require.Contains(t, string(readyBody), `"snapshot_id":"snapshot-command-journey"`)

	finalOutput := runDataProfileWrapper(
		t, projectDir, home, cliDir, port,
		"use", "fixture",
	)
	require.Contains(t, finalOutput, "profile=fixture")
	require.Contains(t, finalOutput, "ready=true")
	require.Contains(t, getPodcastListBody(t, port), "Fixture：深度科技")
	afterHash, err := SHA256File(snapshot.DatabasePath)
	require.NoError(t, err)
	require.Equal(t, baselineHash, afterHash)
}

func runDataProfileWrapper(
	t *testing.T,
	projectDir string,
	home string,
	cliDir string,
	port int,
	args ...string,
) string {
	t.Helper()
	commandArgs := []string{
		filepath.Join(projectDir, "scripts", "data-profile.sh"),
		"--home", home,
		"--port", portString(port),
		"--timeout", "30s",
	}
	commandArgs = append(commandArgs, args...)
	command := exec.Command("/bin/bash", commandArgs...)
	command.Dir = projectDir
	command.Env = append(
		os.Environ(),
		"MAGICPODCAST_DATA_PROFILE_CLI_DIR="+cliDir,
	)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return string(output)
}

func getPodcastListBody(t *testing.T, port int) string {
	t.Helper()
	response, err := http.Get("http://127.0.0.1:" + portString(port) + "/api/v1/podcasts?page=1&page_size=10")
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	return string(body)
}

func TestSnapshotFailureDoesNotReplaceRunningFixture(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	home := t.TempDir()
	port, err := FreeLoopbackPort()
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  projectDir,
		ProfileHome: home,
		Port:        port,
		Timeout:     30 * time.Second,
	}
	t.Cleanup(func() {
		if state, readErr := controller.readState(); readErr == nil {
			_ = controller.stop(state)
		}
	})
	fixtureStatus, err := controller.UseFixture(context.Background())
	require.NoError(t, err)

	_, err = controller.UseSnapshot(context.Background(), "latest")
	require.ErrorContains(t, err, "no snapshots")
	after, err := controller.Status(context.Background())
	require.NoError(t, err)
	require.True(t, after.Ready)
	require.Equal(t, fixtureStatus.InstanceID, after.InstanceID)
}
