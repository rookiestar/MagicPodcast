package dataprofile

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"magicpodcast/internal/database"
)

func TestProfileStopHelperProcess(t *testing.T) {
	if os.Getenv("MAGICPODCAST_PROFILE_STOP_HELPER") != "1" {
		return
	}
	signal.Ignore(syscall.SIGTERM)
	marker := os.Getenv("MAGICPODCAST_PROFILE_STOP_MARKER")
	if err := os.WriteFile(marker, []byte("ready"), 0o600); err != nil {
		os.Exit(2)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestControllerFixtureEndToEndUsesRealBackend(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	port, err := FreeLoopbackPort()
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  projectDir,
		ProfileHome: t.TempDir(),
		Port:        port,
		Timeout:     30 * time.Second,
	}
	fixture, err := EnsureFixture(controller.ProfileHome)
	require.NoError(t, err)
	baselineHash, err := SHA256File(fixture.DatabasePath)
	require.NoError(t, err)
	t.Cleanup(func() {
		if state, readErr := controller.readState(); readErr == nil {
			_ = controller.stop(state)
		}
	})

	first, err := controller.UseFixture(context.Background())
	require.NoError(t, err)
	require.True(t, first.Managed)
	require.True(t, first.Ready)
	require.Equal(t, "fixture", first.Profile)
	require.Equal(t, CurrentFixtureVersion(), first.FixtureVersion)

	response, err := http.Get("http://127.0.0.1:" + portString(port) + "/api/v1/podcasts?page=1&page_size=10")
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, string(body))
	require.Contains(t, string(body), "Fixture：深度科技")

	second, err := controller.UseFixture(context.Background())
	require.NoError(t, err)
	require.Equal(t, first.InstanceID, second.InstanceID, "repeat use fixture should keep the ready process")

	status, err := controller.Status(context.Background())
	require.NoError(t, err)
	require.True(t, status.Ready)
	require.Equal(t, first.InstanceID, status.InstanceID)
	afterHash, err := SHA256File(fixture.DatabasePath)
	require.NoError(t, err)
	require.Equal(t, baselineHash, afterHash)
}

func TestDataProfileWrapperExitLeavesManagedBackendReady(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	port, err := FreeLoopbackPort()
	require.NoError(t, err)
	home := t.TempDir()
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

	command := exec.Command(
		"/bin/bash",
		filepath.Join(projectDir, "scripts", "data-profile.sh"),
		"--home", home,
		"--port", portString(port),
		"--timeout", "30s",
		"use", "fixture",
	)
	command.Dir = projectDir
	command.Env = append(
		os.Environ(),
		"MAGICPODCAST_DATA_PROFILE_CLI_DIR="+filepath.Join(t.TempDir(), "cli"),
	)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	require.Contains(t, string(output), "profile=fixture")
	require.Contains(t, string(output), "ready=true")

	// Both the wrapper and CLI have exited. A separate controller and direct
	// readiness request must still observe the detached managed backend.
	status, err := controller.Status(context.Background())
	require.NoError(t, err)
	require.True(t, status.Ready)
	require.Equal(t, "fixture", status.Profile)

	response, err := http.Get("http://127.0.0.1:" + portString(port) + "/ready")
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
}

func TestControllerStartFailurePreservesFixtureAndDoesNotPublishState(t *testing.T) {
	projectDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(projectDir, "backend"), 0o700))
	home := t.TempDir()
	port, err := FreeLoopbackPort()
	require.NoError(t, err)
	controller := Controller{
		ProjectDir:  projectDir,
		ProfileHome: home,
		Port:        port,
		Timeout:     2 * time.Second,
		Command:     []string{"/usr/bin/false"},
	}

	_, err = controller.UseFixture(context.Background())
	require.Error(t, err)
	_, stateErr := os.Stat(controller.statePath())
	require.ErrorIs(t, stateErr, os.ErrNotExist)

	fixture, fixtureErr := EnsureFixture(home)
	require.NoError(t, fixtureErr)
	_, _, validationErr := ValidateManifest(fixture.DatabasePath, fixture.ManifestPath, "fixture")
	require.NoError(t, validationErr)
}

func TestControllerRejectsUnsafeBackendCommandBeforeSwitch(t *testing.T) {
	home := t.TempDir()
	controller := Controller{
		ProjectDir:  t.TempDir(),
		ProfileHome: home,
		Port:        18080,
		Timeout:     time.Second,
		Command:     []string{"relative-command"},
	}

	_, err := controller.UseFixture(context.Background())
	require.ErrorContains(t, err, "must be absolute")
	_, stateErr := os.Stat(controller.statePath())
	require.ErrorIs(t, stateErr, os.ErrNotExist)
}

func TestControllerStateCommitFailureRestoresRunningFixture(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	_, err = publishPreparedTestSnapshot(t,
		home,
		"snapshot-rollback",
		fixture.DatabasePath,
		"2026-08-12T03:00:00Z",
		SanitizerVersion,
	)
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
	failNextCommit := true
	controller.writeStateHook = func(state RuntimeState) error {
		if failNextCommit {
			failNextCommit = false
			return errors.New("injected state commit failure")
		}
		return controller.writeState(state)
	}

	_, err = controller.UseSnapshot(context.Background(), "latest")
	require.ErrorContains(t, err, "previous profile restored")
	require.ErrorContains(t, err, "injected state commit failure")

	status, err := controller.Status(context.Background())
	require.NoError(t, err)
	require.True(t, status.Ready)
	require.Equal(t, "fixture", status.Profile)
	require.Equal(t, fixtureStatus.InstanceID, status.InstanceID)
}

func TestSnapshotStartFailureRestoresRunningFixture(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	snapshot, err := publishPreparedTestSnapshot(
		t,
		home,
		"snapshot-start-failure",
		fixture.DatabasePath,
		"2026-08-13T02:00:00Z",
		SanitizerVersion,
	)
	require.NoError(t, err)
	snapshotHash, err := SHA256File(snapshot.DatabasePath)
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
	require.Contains(t, getPodcastListBody(t, port), "Fixture：深度科技")

	controller.Command = []string{"/usr/bin/false"}
	_, err = controller.UseSnapshot(context.Background(), "latest")
	require.ErrorContains(t, err, "previous profile restored")

	status, err := controller.Status(context.Background())
	require.NoError(t, err)
	require.True(t, status.Ready)
	require.Equal(t, "fixture", status.Profile)
	require.Equal(t, fixtureStatus.InstanceID, status.InstanceID)
	require.Contains(t, getPodcastListBody(t, port), "Fixture：深度科技")
	afterHash, err := SHA256File(snapshot.DatabasePath)
	require.NoError(t, err)
	require.Equal(t, snapshotHash, afterHash)
}

func TestControllerPortConflictDoesNotPublishProfileState(t *testing.T) {
	projectDir, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	controller := Controller{
		ProjectDir:  projectDir,
		ProfileHome: t.TempDir(),
		Port:        port,
		Timeout:     10 * time.Second,
	}

	_, err = controller.UseFixture(context.Background())
	require.Error(t, err)
	_, stateErr := os.Stat(controller.statePath())
	require.ErrorIs(t, stateErr, os.ErrNotExist)
}

func TestCreateWorkCopyReportsDestinationFailure(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "source.db")
	require.NoError(t, os.WriteFile(source, []byte("source"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(home, "work"), []byte("not-a-directory"), 0o600))
	controller := Controller{ProfileHome: home}

	_, err := controller.createWorkCopy("snapshot", "snapshot-copy-failure", source, "instance")
	require.ErrorContains(t, err, "create work-copy directory")
}

func TestControllerRejectsConcurrentProfileOperations(t *testing.T) {
	controller := Controller{ProfileHome: t.TempDir()}
	unlock, err := controller.lockOperation()
	require.NoError(t, err)
	defer unlock()

	_, err = controller.lockOperation()
	require.ErrorContains(t, err, "already in progress")
}

func TestSafeBackendEnvironmentDoesNotInheritCredentials(t *testing.T) {
	t.Setenv("MAGICPODCAST_LLM_API_KEY", "must-not-leak")
	t.Setenv("MAGICPODCAST_USER_ACCESS_TOKEN", "must-not-leak")
	t.Setenv("MAGICPODCAST_DATABASE_PATH", "/known/production.db")
	controller := Controller{
		ProjectDir:  t.TempDir(),
		ProfileHome: t.TempDir(),
		Port:        18080,
		Timeout:     time.Second,
	}
	environment := controller.safeBackendEnvironment(RuntimeState{
		Profile:        "fixture",
		InstanceID:     "test-instance",
		FixtureVersion: CurrentFixtureVersion(),
		DatabasePath:   filepath.Join(controller.ProfileHome, "fixtures", CurrentFixtureVersion(), "magicpodcast.db"),
		Port:           18080,
	})
	joined := strings.Join(environment, "\n")

	require.NotContains(t, joined, "must-not-leak")
	require.NotContains(t, joined, "/known/production.db")
	require.Contains(t, joined, "MAGICPODCAST_SKIP_DOTENV=1")
	require.Contains(t, joined, "MAGICPODCAST_DISABLE_SCHEDULER=1")
}

func TestReadStateRejectsTamperedPIDCommandAndPaths(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	workDir := filepath.Join(home, "work", "fixture-"+fixture.Version)
	require.NoError(t, os.MkdirAll(workDir, 0o700))
	workPath := filepath.Join(workDir, "instance.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, workPath, 0o600))
	binDir := filepath.Join(home, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o700))
	commandPath := filepath.Join(binDir, "magicpodcast-api-instance")
	require.NoError(t, os.WriteFile(commandPath, []byte("binary"), 0o700))
	controller := Controller{ProfileHome: home}
	valid := RuntimeState{
		FormatVersion:  runtimeStateFormatVersion,
		Profile:        "fixture",
		InstanceID:     "instance",
		SchemaVersion:  database.CurrentSchemaVersion,
		FixtureVersion: fixture.Version,
		DatabasePath:   workPath,
		ManifestPath:   fixture.ManifestPath,
		CommandPath:    commandPath,
		PID:            os.Getpid(),
		Port:           18080,
		StartedAt:      "2026-08-13T00:00:00Z",
	}
	writeRaw := func(state RuntimeState) {
		data, marshalErr := json.Marshal(state)
		require.NoError(t, marshalErr)
		require.NoError(t, os.MkdirAll(filepath.Dir(controller.statePath()), 0o700))
		require.NoError(t, os.WriteFile(controller.statePath(), data, 0o600))
	}

	writeRaw(valid)
	_, err = controller.readState()
	require.NoError(t, err)

	tampered := valid
	tampered.PID = 0
	writeRaw(tampered)
	_, err = controller.readState()
	require.ErrorContains(t, err, "PID is invalid")

	tampered = valid
	tampered.CommandPath = "/bin/true"
	writeRaw(tampered)
	_, err = controller.readState()
	require.ErrorContains(t, err, "command path rejected")

	require.NoError(t, os.Chmod(commandPath, 0o600))
	writeRaw(valid)
	_, err = controller.readState()
	require.ErrorContains(t, err, "not executable")
	require.NoError(t, os.Chmod(commandPath, 0o700))

	tampered = valid
	tampered.DatabasePath = fixture.DatabasePath
	writeRaw(tampered)
	_, err = controller.readState()
	require.ErrorContains(t, err, "database path rejected")
}

func TestReadStateRejectsSymlinkAndBroadPermissions(t *testing.T) {
	home := t.TempDir()
	controller := Controller{ProfileHome: home}
	require.NoError(t, os.MkdirAll(filepath.Dir(controller.statePath()), 0o700))
	target := filepath.Join(t.TempDir(), "active.json")
	require.NoError(t, os.WriteFile(target, []byte("{}"), 0o600))
	require.NoError(t, os.Symlink(target, controller.statePath()))

	_, err := controller.readState()
	require.ErrorContains(t, err, "not a regular file")

	require.NoError(t, os.Remove(controller.statePath()))
	require.NoError(t, os.WriteFile(controller.statePath(), []byte("{}"), 0o644))
	_, err = controller.readState()
	require.ErrorContains(t, err, "permissions are too broad")
}

func TestManagedCommandRequiresExactExecutablePrefix(t *testing.T) {
	require.True(t, commandLineStartsWithExecutable("/managed/api", "/managed/api"))
	require.True(t, commandLineStartsWithExecutable("/managed/api --flag", "/managed/api"))
	require.False(t, commandLineStartsWithExecutable("/bin/sh /managed/api", "/managed/api"))
	require.False(t, commandLineStartsWithExecutable("/tmp/not-managed/api", "/managed/api"))
	require.False(t, commandLineStartsWithExecutable("/managed/api-evil", "/managed/api"))
}

func TestControllerStopWaitsForForcedProcessExit(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	workDir := filepath.Join(home, "work", "fixture-"+fixture.Version)
	require.NoError(t, os.MkdirAll(workDir, 0o700))
	workPath := filepath.Join(workDir, "forced-stop.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, workPath, 0o600))
	binDir := filepath.Join(home, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o700))
	commandPath := filepath.Join(binDir, "magicpodcast-api-forced-stop")
	testExecutable, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, copyRegularFile(testExecutable, commandPath, 0o700))

	marker := filepath.Join(t.TempDir(), "ready")
	command := exec.Command(commandPath, "-test.run=^TestProfileStopHelperProcess$")
	command.Env = append(
		os.Environ(),
		"MAGICPODCAST_PROFILE_STOP_HELPER=1",
		"MAGICPODCAST_PROFILE_STOP_MARKER="+marker,
	)
	require.NoError(t, command.Start())
	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		select {
		case <-waitDone:
		case <-time.After(time.Second):
		}
	})
	require.Eventually(t, func() bool {
		_, statErr := os.Stat(marker)
		return statErr == nil
	}, 5*time.Second, 20*time.Millisecond)

	controller := Controller{ProfileHome: home}
	state := RuntimeState{
		FormatVersion:  runtimeStateFormatVersion,
		Profile:        "fixture",
		InstanceID:     "forced-stop",
		SchemaVersion:  database.CurrentSchemaVersion,
		FixtureVersion: fixture.Version,
		DatabasePath:   workPath,
		ManifestPath:   fixture.ManifestPath,
		CommandPath:    commandPath,
		PID:            command.Process.Pid,
		Port:           18080,
		StartedAt:      "2026-08-13T00:00:00Z",
	}
	require.NoError(t, controller.stop(state))
	running, err := processExists(state.PID)
	require.NoError(t, err)
	require.False(t, running)
}

func TestControllerRejectsChangingManagedPortWhileStateExists(t *testing.T) {
	home := t.TempDir()
	fixture, err := EnsureFixture(home)
	require.NoError(t, err)
	workDir := filepath.Join(home, "work", "fixture-"+fixture.Version)
	require.NoError(t, os.MkdirAll(workDir, 0o700))
	workPath := filepath.Join(workDir, "instance.db")
	require.NoError(t, copyRegularFile(fixture.DatabasePath, workPath, 0o600))
	binDir := filepath.Join(home, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o700))
	commandPath := filepath.Join(binDir, "magicpodcast-api-instance")
	require.NoError(t, os.WriteFile(commandPath, []byte("binary"), 0o700))
	controller := Controller{
		ProjectDir:  t.TempDir(),
		ProfileHome: home,
		Port:        18081,
		Timeout:     time.Second,
	}
	require.NoError(t, controller.writeState(RuntimeState{
		FormatVersion:  runtimeStateFormatVersion,
		Profile:        "fixture",
		InstanceID:     "instance",
		SchemaVersion:  database.CurrentSchemaVersion,
		FixtureVersion: fixture.Version,
		DatabasePath:   workPath,
		ManifestPath:   fixture.ManifestPath,
		CommandPath:    commandPath,
		PID:            os.Getpid(),
		Port:           18080,
		StartedAt:      "2026-08-13T00:00:00Z",
	}))

	_, err = controller.UseFixture(context.Background())
	require.ErrorContains(t, err, "active profile is managed on port 18080")
}

func TestControllerRejectsUnignoredProfileHomeInsideRepository(t *testing.T) {
	projectDir := t.TempDir()
	controller := Controller{
		ProjectDir:  projectDir,
		ProfileHome: filepath.Join(projectDir, "local-data-profiles"),
		Port:        18080,
		Timeout:     time.Second,
		Command:     []string{"/usr/bin/false"},
	}

	_, err := controller.UseFixture(context.Background())
	require.ErrorContains(t, err, ".magicpodcast-data-profiles")
	_, statErr := os.Stat(filepath.Join(projectDir, "local-data-profiles", "fixtures"))
	require.ErrorIs(t, statErr, os.ErrNotExist)

	controller.ProfileHome = filepath.Join(projectDir, ".magicpodcast-data-profiles", "test")
	_, err = controller.UseFixture(context.Background())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "profile home inside the repository")
}

func portString(port int) string {
	return strconv.Itoa(port)
}
