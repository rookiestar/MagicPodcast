package dataprofile

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/runtimeprofile"
)

const runtimeStateFormatVersion = 1

var managedProfileNames = map[string]struct{}{
	runtimeprofile.ProfileFixture:  {},
	runtimeprofile.ProfileSnapshot: {},
}

type RuntimeState struct {
	FormatVersion      int    `json:"format_version"`
	Profile            string `json:"profile"`
	InstanceID         string `json:"instance_id"`
	SchemaVersion      int    `json:"schema_version"`
	FixtureVersion     string `json:"fixture_version,omitempty"`
	SnapshotID         string `json:"snapshot_id,omitempty"`
	SnapshotCapturedAt string `json:"snapshot_captured_at,omitempty"`
	DatabasePath       string `json:"database_path"`
	ManifestPath       string `json:"manifest_path"`
	CommandPath        string `json:"command_path"`
	PID                int    `json:"pid"`
	Port               int    `json:"port"`
	StartedAt          string `json:"started_at"`
}

type PublicStatus struct {
	Managed            bool   `json:"managed"`
	Profile            string `json:"profile"`
	Ready              bool   `json:"ready"`
	SchemaVersion      int    `json:"schema_version,omitempty"`
	FixtureVersion     string `json:"fixture_version,omitempty"`
	SnapshotID         string `json:"snapshot_id,omitempty"`
	SnapshotCapturedAt string `json:"snapshot_captured_at,omitempty"`
	InstanceID         string `json:"instance_id,omitempty"`
}

type Controller struct {
	ProjectDir  string
	ProfileHome string
	Port        int
	Timeout     time.Duration
	Command     []string

	writeStateHook   func(RuntimeState) error
	writeLatestHook  func(Snapshot) error
	retentionHook    func(*snapshotRetention) error
	afterPublishHook func()
}

func DefaultProfileHome() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("MAGICPODCAST_DATA_PROFILE_HOME")); configured != "" {
		return configured, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(configDir, "MagicPodcast", "data-profiles"), nil
}

func (c Controller) UseFixture(ctx context.Context) (PublicStatus, error) {
	if err := c.validate(); err != nil {
		return PublicStatus{}, err
	}
	unlock, err := c.lockOperation()
	if err != nil {
		return PublicStatus{}, err
	}
	defer unlock()
	if current, err := c.readState(); err == nil &&
		current.Profile == runtimeprofile.ProfileFixture &&
		current.FixtureVersion == CurrentFixtureVersion() {
		if ready, _ := c.probeReady(ctx, current); ready {
			return publicStatusFromState(current), nil
		}
	}
	fixture, err := EnsureFixture(c.ProfileHome)
	if err != nil {
		return PublicStatus{}, err
	}
	instanceID, err := randomIdentifier()
	if err != nil {
		return PublicStatus{}, err
	}
	workPath, err := c.createWorkCopy("fixture", fixture.Version, fixture.DatabasePath, instanceID)
	if err != nil {
		return PublicStatus{}, err
	}
	keepWorkCopy := false
	defer func() {
		if !keepWorkCopy {
			_ = os.Remove(workPath)
			_ = os.Remove(workPath + "-wal")
			_ = os.Remove(workPath + "-shm")
		}
	}()
	target := RuntimeState{
		FormatVersion:  runtimeStateFormatVersion,
		Profile:        runtimeprofile.ProfileFixture,
		InstanceID:     instanceID,
		SchemaVersion:  database.CurrentSchemaVersion,
		FixtureVersion: fixture.Version,
		DatabasePath:   workPath,
		ManifestPath:   fixture.ManifestPath,
		Port:           c.Port,
	}
	status, err := c.switchTo(ctx, target)
	if err != nil {
		return PublicStatus{}, err
	}
	keepWorkCopy = true
	return status, nil
}

func (c Controller) UseSnapshot(ctx context.Context, selector string) (PublicStatus, error) {
	if err := c.validate(); err != nil {
		return PublicStatus{}, err
	}
	unlock, err := c.lockOperation()
	if err != nil {
		return PublicStatus{}, err
	}
	defer unlock()
	snapshot, err := ResolveSnapshot(c.ProfileHome, selector)
	if err != nil {
		return PublicStatus{}, err
	}
	baselineHash, err := SHA256File(snapshot.DatabasePath)
	if err != nil {
		return PublicStatus{}, err
	}
	instanceID, err := randomIdentifier()
	if err != nil {
		return PublicStatus{}, err
	}
	workPath, err := c.createWorkCopy("snapshot", snapshot.ID, snapshot.DatabasePath, instanceID)
	if err != nil {
		return PublicStatus{}, err
	}
	keepWorkCopy := false
	defer func() {
		if !keepWorkCopy {
			_ = os.Remove(workPath)
			_ = os.Remove(workPath + "-wal")
			_ = os.Remove(workPath + "-shm")
		}
	}()
	afterCopyHash, err := SHA256File(snapshot.DatabasePath)
	if err != nil {
		return PublicStatus{}, err
	}
	if afterCopyHash != baselineHash {
		return PublicStatus{}, fmt.Errorf("snapshot baseline changed while creating the work copy")
	}
	target := RuntimeState{
		FormatVersion:      runtimeStateFormatVersion,
		Profile:            runtimeprofile.ProfileSnapshot,
		InstanceID:         instanceID,
		SchemaVersion:      database.CurrentSchemaVersion,
		SnapshotID:         snapshot.ID,
		SnapshotCapturedAt: snapshot.Manifest.CapturedAt,
		DatabasePath:       workPath,
		ManifestPath:       snapshot.ManifestPath,
		Port:               c.Port,
	}
	status, err := c.switchTo(ctx, target)
	if err != nil {
		return PublicStatus{}, err
	}
	keepWorkCopy = true
	return status, nil
}

func (c Controller) Status(ctx context.Context) (PublicStatus, error) {
	if err := c.validate(); err != nil {
		return PublicStatus{}, err
	}
	state, err := c.readState()
	if errors.Is(err, os.ErrNotExist) {
		return PublicStatus{Managed: false, Profile: "none", Ready: false}, nil
	}
	if err != nil {
		return PublicStatus{}, err
	}
	status := publicStatusFromState(state)
	ready, err := c.probeReady(ctx, state)
	if err != nil {
		status.Ready = false
		return status, nil
	}
	status.Ready = ready
	return status, nil
}

func (c Controller) switchTo(ctx context.Context, target RuntimeState) (PublicStatus, error) {
	if _, _, err := ValidateManifest(target.DatabasePath, target.ManifestPath, target.Profile); err != nil {
		return PublicStatus{}, fmt.Errorf("target profile validation failed: %w", err)
	}
	previous, previousErr := c.readState()
	if previousErr == nil && sameTarget(previous, target) {
		if ready, _ := c.probeReady(ctx, previous); ready {
			return publicStatusFromState(previous), nil
		}
	}
	if previousErr != nil && !errors.Is(previousErr, os.ErrNotExist) {
		return PublicStatus{}, previousErr
	}
	if previousErr == nil && previous.Port != c.Port {
		return PublicStatus{}, fmt.Errorf(
			"active profile is managed on port %d; rerun with --port %d",
			previous.Port,
			previous.Port,
		)
	}

	if target.InstanceID == "" {
		instanceID, err := randomIdentifier()
		if err != nil {
			return PublicStatus{}, err
		}
		target.InstanceID = instanceID
	}
	command, err := c.prepareBackend(ctx, target.InstanceID)
	if err != nil {
		return PublicStatus{}, err
	}
	target.CommandPath = command[0]

	previousWasRunning := false
	if previousErr == nil {
		previousWasRunning, err = c.isManagedProcess(previous)
		if err != nil {
			return PublicStatus{}, err
		}
		if previousWasRunning {
			if err := c.stop(previous); err != nil {
				return PublicStatus{}, fmt.Errorf("stop current managed backend: %w", err)
			}
		}
	}

	started, err := c.start(ctx, target, command)
	if err != nil {
		if previousErr == nil && previousWasRunning {
			if rollbackErr := c.restorePrevious(previous); rollbackErr == nil {
				return PublicStatus{}, fmt.Errorf("switch failed; previous profile restored: %w", err)
			} else {
				return PublicStatus{}, fmt.Errorf("switch failed and previous profile restart failed: %v; rollback: %w", err, rollbackErr)
			}
		}
		return PublicStatus{}, err
	}
	if err := c.persistState(started); err != nil {
		stateErr := fmt.Errorf("persist active profile state: %w", err)
		if stopErr := c.stop(started); stopErr != nil {
			return PublicStatus{}, fmt.Errorf("%v; target process could not be stopped: %w", stateErr, stopErr)
		}
		if previousErr == nil && previousWasRunning {
			if rollbackErr := c.restorePrevious(previous); rollbackErr == nil {
				return PublicStatus{}, fmt.Errorf("switch failed; previous profile restored: %w", stateErr)
			} else {
				return PublicStatus{}, fmt.Errorf("switch failed and previous profile restart failed: %v; rollback: %w", stateErr, rollbackErr)
			}
		}
		return PublicStatus{}, stateErr
	}
	if previousErr == nil && previous.InstanceID != started.InstanceID {
		c.cleanupInactiveInstance(previous, started)
	}
	return publicStatusFromState(started), nil
}

func (c Controller) cleanupInactiveInstance(previous, active RuntimeState) {
	if previous.DatabasePath != active.DatabasePath {
		_ = os.Remove(previous.DatabasePath)
		_ = os.Remove(previous.DatabasePath + "-wal")
		_ = os.Remove(previous.DatabasePath + "-shm")
		_ = os.Remove(filepath.Dir(previous.DatabasePath))
	}
	if previous.CommandPath != active.CommandPath {
		_ = os.Remove(previous.CommandPath)
	}
}

func (c Controller) restorePrevious(previous RuntimeState) error {
	if strings.TrimSpace(previous.CommandPath) == "" {
		return fmt.Errorf("previous managed backend command is missing")
	}
	rollbackContext, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	rolledBack, err := c.start(rollbackContext, previous, []string{previous.CommandPath})
	if err != nil {
		return err
	}
	if err := c.persistState(rolledBack); err != nil {
		stopErr := c.stop(rolledBack)
		if stopErr != nil {
			return fmt.Errorf("persist restored profile state: %v; restored process could not be stopped: %w", err, stopErr)
		}
		return fmt.Errorf("persist restored profile state: %w", err)
	}
	return nil
}

func (c Controller) validate() error {
	if strings.TrimSpace(c.ProjectDir) == "" {
		return fmt.Errorf("project directory is required")
	}
	if strings.TrimSpace(c.ProfileHome) == "" {
		return fmt.Errorf("profile home is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid backend port %d", c.Port)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if err := validateProfileHomeLocation(c.ProjectDir, c.ProfileHome); err != nil {
		return err
	}
	return nil
}

func validateProfileHomeLocation(projectDir, profileHome string) error {
	projectRoot, err := requireRealDirectory(projectDir)
	if err != nil {
		return fmt.Errorf("project directory rejected: %w", err)
	}
	profileRoot, err := ensureRoot(profileHome)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(projectRoot, profileRoot)
	if err != nil {
		return fmt.Errorf("compare project and profile directories: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}
	ignoredRoot := ".magicpodcast-data-profiles"
	if relative != ignoredRoot && !strings.HasPrefix(relative, ignoredRoot+string(filepath.Separator)) {
		return fmt.Errorf(
			"profile home inside the repository must be under the ignored %s directory",
			ignoredRoot,
		)
	}
	return nil
}

func (c Controller) lockOperation() (func(), error) {
	root, err := ensureRoot(c.ProfileHome)
	if err != nil {
		return nil, err
	}
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create profile runtime directory: %w", err)
	}
	lockPath := filepath.Join(runtimeDir, "operation.lock")
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("data profile operation lock is not a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect data profile operation lock: %w", err)
	}
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open data profile operation lock: %w", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lockFile.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("another data profile operation is already in progress")
		}
		return nil, fmt.Errorf("lock data profile operation: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}, nil
}

func (c Controller) createWorkCopy(kind, id, sourcePath, instanceID string) (string, error) {
	if !isSafeIdentifier(kind) || !isSafeIdentifier(id) || !isSafeIdentifier(instanceID) {
		return "", fmt.Errorf("invalid work-copy identity")
	}
	root, err := ensureRoot(c.ProfileHome)
	if err != nil {
		return "", err
	}
	workDir := filepath.Join(root, "work", kind+"-"+id)
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return "", fmt.Errorf("create work-copy directory: %w", err)
	}
	targetPath := filepath.Join(workDir, instanceID+".db")
	tempPath := targetPath + ".tmp"
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("open profile baseline: %w", err)
	}
	defer source.Close()
	target, err := os.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create profile work copy: %w", err)
	}
	copyErr := func() error {
		if _, err := io.Copy(target, source); err != nil {
			return err
		}
		if err := target.Sync(); err != nil {
			return err
		}
		return target.Close()
	}()
	if copyErr != nil {
		_ = target.Close()
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("copy profile baseline: %w", copyErr)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("publish profile work copy: %w", err)
	}
	return targetPath, nil
}

func (c Controller) prepareBackend(ctx context.Context, instanceID string) ([]string, error) {
	if len(c.Command) > 0 {
		command := append([]string(nil), c.Command...)
		executable, err := requireAbsoluteExecutable(command[0])
		if err != nil {
			return nil, fmt.Errorf("backend command rejected: %w", err)
		}
		command[0] = executable
		return command, nil
	}
	root, err := ensureRoot(c.ProfileHome)
	if err != nil {
		return nil, err
	}
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return nil, fmt.Errorf("create managed bin directory: %w", err)
	}
	finalPath := filepath.Join(binDir, "magicpodcast-api-"+instanceID)
	tempPath := finalPath + ".tmp"
	command := exec.CommandContext(ctx, "go", "build", "-o", tempPath, "./cmd/api")
	command.Dir = filepath.Join(c.ProjectDir, "backend")
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("build managed backend: %w: %s", err, strings.TrimSpace(output.String()))
	}
	if err := os.Chmod(tempPath, 0o700); err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("set backend binary mode: %w", err)
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("publish backend binary: %w", err)
	}
	return []string{finalPath}, nil
}

func (c Controller) start(ctx context.Context, state RuntimeState, command []string) (RuntimeState, error) {
	if len(command) == 0 {
		return RuntimeState{}, fmt.Errorf("backend command is empty")
	}
	logDir := filepath.Join(c.ProfileHome, "runtime")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return RuntimeState{}, fmt.Errorf("create runtime directory: %w", err)
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, "backend.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return RuntimeState{}, fmt.Errorf("open backend log: %w", err)
	}
	defer logFile.Close()

	process := exec.Command(command[0], command[1:]...)
	process.Dir = filepath.Join(c.ProjectDir, "backend")
	process.Stdout = logFile
	process.Stderr = logFile
	process.Env = c.safeBackendEnvironment(state)
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := process.Start(); err != nil {
		return RuntimeState{}, fmt.Errorf("start managed backend: %w", err)
	}
	state.PID = process.Process.Pid
	state.CommandPath = command[0]
	state.StartedAt = time.Now().UTC().Format(time.RFC3339)
	go func() {
		_ = process.Wait()
	}()

	readyState, err := c.waitUntilReady(ctx, state)
	if err != nil {
		_ = c.stop(state)
		return RuntimeState{}, err
	}
	return readyState, nil
}

func (c Controller) safeBackendEnvironment(state RuntimeState) []string {
	environment := make([]string, 0, 20)
	for _, key := range []string{"PATH", "TMPDIR", "LANG", "LC_ALL", "TZ", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if value, found := os.LookupEnv(key); found {
			environment = append(environment, key+"="+value)
		}
	}
	environment = append(environment,
		"CONFIG_PATH="+filepath.Join(c.ProjectDir, "backend", "configs", "data-profile.yaml"),
		"MAGICPODCAST_SKIP_DOTENV=1",
		"MAGICPODCAST_SERVER_HOST=127.0.0.1",
		"MAGICPODCAST_SERVER_PORT="+strconv.Itoa(state.Port),
		"MAGICPODCAST_SERVER_MODE=debug",
		"MAGICPODCAST_DATABASE_PATH="+state.DatabasePath,
		"MAGICPODCAST_DATABASE_DEBUG=false",
		"MAGICPODCAST_DISABLE_SCHEDULER=1",
		"MAGICPODCAST_DATA_PROFILE_HOME="+c.ProfileHome,
		"MAGICPODCAST_DATA_PROFILE="+state.Profile,
		"MAGICPODCAST_DATA_PROFILE_INSTANCE_ID="+state.InstanceID,
		"MAGICPODCAST_FIXTURE_VERSION="+state.FixtureVersion,
		"MAGICPODCAST_SNAPSHOT_ID="+state.SnapshotID,
		"MAGICPODCAST_SNAPSHOT_CAPTURED_AT="+state.SnapshotCapturedAt,
		"MAGICPODCAST_RELEASE_ID=data-profile-"+state.InstanceID,
		"MAGICPODCAST_FRONTEND_BUILD_ID=not-managed",
	)
	sort.Strings(environment)
	return environment
}

func (c Controller) waitUntilReady(ctx context.Context, state RuntimeState) (RuntimeState, error) {
	deadline := time.NewTimer(c.Timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return RuntimeState{}, ctx.Err()
		case <-deadline.C:
			if lastErr == nil {
				lastErr = fmt.Errorf("readiness endpoint did not become available")
			}
			return RuntimeState{}, fmt.Errorf("managed backend not ready: %w", lastErr)
		case <-ticker.C:
			ready, err := c.probeReady(ctx, state)
			if err == nil && ready {
				return state, nil
			}
			if err != nil {
				lastErr = err
			}
			if running, _ := processExists(state.PID); !running {
				return RuntimeState{}, fmt.Errorf("managed backend exited before readiness")
			}
		}
	}
}

func (c Controller) probeReady(ctx context.Context, state RuntimeState) (bool, error) {
	requestContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d/ready", state.Port),
		nil,
	)
	if err != nil {
		return false, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	var body struct {
		Status             string `json:"status"`
		DataProfile        string `json:"data_profile"`
		InstanceID         string `json:"data_profile_instance_id"`
		SchemaVersion      int    `json:"schema_version"`
		FixtureVersion     string `json:"fixture_version"`
		SnapshotID         string `json:"snapshot_id"`
		SnapshotCapturedAt string `json:"snapshot_captured_at"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return false, err
	}
	if response.StatusCode != http.StatusOK || body.Status != "ok" {
		return false, fmt.Errorf("readiness returned HTTP %d status %q", response.StatusCode, body.Status)
	}
	if body.DataProfile != state.Profile || body.InstanceID != state.InstanceID {
		return false, fmt.Errorf("readiness belongs to a different data profile process")
	}
	if body.SchemaVersion != database.CurrentSchemaVersion {
		return false, fmt.Errorf("readiness schema version %d is not current", body.SchemaVersion)
	}
	if state.Profile == runtimeprofile.ProfileFixture && body.FixtureVersion != state.FixtureVersion {
		return false, fmt.Errorf("readiness fixture version mismatch")
	}
	if state.Profile == runtimeprofile.ProfileSnapshot &&
		(body.SnapshotID != state.SnapshotID || body.SnapshotCapturedAt != state.SnapshotCapturedAt) {
		return false, fmt.Errorf("readiness snapshot metadata mismatch")
	}
	return true, nil
}

func (c Controller) stop(state RuntimeState) error {
	matches, err := c.isManagedProcess(state)
	if err != nil {
		return err
	}
	if !matches {
		return nil
	}
	process, err := os.FindProcess(state.PID)
	if err != nil {
		return fmt.Errorf("find managed backend: %w", err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal managed backend: %w", err)
	}
	for i := 0; i < 50; i++ {
		running, _ := processExists(state.PID)
		if !running {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("force stop managed backend: %w", err)
	}
	for i := 0; i < 20; i++ {
		running, err := processExists(state.PID)
		if err != nil {
			return fmt.Errorf("confirm forced backend stop: %w", err)
		}
		if !running {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("managed backend still running after forced stop")
}

func (c Controller) isManagedProcess(state RuntimeState) (bool, error) {
	if err := c.validateState(state); err != nil {
		return false, fmt.Errorf("refuse to inspect process from invalid active state: %w", err)
	}
	running, err := processExists(state.PID)
	if err != nil || !running {
		return false, err
	}
	output, err := exec.Command("ps", "-p", strconv.Itoa(state.PID), "-o", "command=").Output()
	if err != nil {
		return false, fmt.Errorf("inspect managed backend process: %w", err)
	}
	commandLine := strings.TrimSpace(string(output))
	if !commandLineStartsWithExecutable(commandLine, state.CommandPath) {
		return false, fmt.Errorf("PID %d is not the recorded managed backend; refusing to stop it", state.PID)
	}
	return true, nil
}

func commandLineStartsWithExecutable(commandLine, executable string) bool {
	if commandLine == executable {
		return true
	}
	return strings.HasPrefix(commandLine, executable+" ")
}

func processExists(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	return false, err
}

func (c Controller) statePath() string {
	return filepath.Join(c.ProfileHome, "runtime", "active.json")
}

func (c Controller) readState() (RuntimeState, error) {
	statePath := c.statePath()
	info, err := os.Lstat(statePath)
	if err != nil {
		return RuntimeState{}, err
	}
	if !info.Mode().IsRegular() {
		return RuntimeState{}, fmt.Errorf("active profile state is not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return RuntimeState{}, fmt.Errorf("active profile state permissions are too broad")
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		return RuntimeState{}, err
	}
	var state RuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return RuntimeState{}, fmt.Errorf("decode active profile state: %w", err)
	}
	if err := c.validateState(state); err != nil {
		return RuntimeState{}, fmt.Errorf("active profile state is invalid: %w", err)
	}
	return state, nil
}

func (c Controller) writeState(state RuntimeState) error {
	if err := c.validateState(state); err != nil {
		return fmt.Errorf("refuse to persist invalid active profile state: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode active profile state: %w", err)
	}
	return writeFileAtomic(c.statePath(), append(data, '\n'), 0o600)
}

func (c Controller) persistState(state RuntimeState) error {
	if c.writeStateHook != nil {
		return c.writeStateHook(state)
	}
	return c.writeState(state)
}

func (c Controller) validateState(state RuntimeState) error {
	if state.FormatVersion != runtimeStateFormatVersion {
		return fmt.Errorf("unsupported format %d", state.FormatVersion)
	}
	if _, found := managedProfileNames[state.Profile]; !found {
		return fmt.Errorf("profile %q is not managed", state.Profile)
	}
	if !isSafeIdentifier(state.InstanceID) {
		return fmt.Errorf("instance ID is invalid")
	}
	if state.SchemaVersion != database.CurrentSchemaVersion {
		return fmt.Errorf("schema version %d is not current", state.SchemaVersion)
	}
	if state.PID <= 0 {
		return fmt.Errorf("PID is invalid")
	}
	if state.Port <= 0 || state.Port > 65535 {
		return fmt.Errorf("port is invalid")
	}
	if _, err := parseRFC3339(state.StartedAt); err != nil {
		return fmt.Errorf("start time is invalid: %w", err)
	}

	root, err := ensureRoot(c.ProfileHome)
	if err != nil {
		return err
	}
	workRoot := filepath.Join(root, "work")
	databasePath, err := requireRegularFileWithin(state.DatabasePath, workRoot)
	if err != nil {
		return fmt.Errorf("database path rejected: %w", err)
	}
	databaseAbsolute, err := filepath.Abs(state.DatabasePath)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	databaseAbsoluteResolved, err := filepath.EvalSymlinks(databaseAbsolute)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	if databasePath != databaseAbsoluteResolved {
		return fmt.Errorf("database path is not canonical")
	}
	commandRoot := filepath.Join(root, "bin")
	commandPath, err := requireRegularFileWithin(state.CommandPath, commandRoot)
	if err != nil {
		return fmt.Errorf("command path rejected: %w", err)
	}
	commandInfo, err := os.Lstat(commandPath)
	if err != nil {
		return fmt.Errorf("inspect command path: %w", err)
	}
	if commandInfo.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("command path is not executable")
	}
	commandAbsolute, err := filepath.Abs(state.CommandPath)
	if err != nil {
		return fmt.Errorf("resolve command path: %w", err)
	}
	commandAbsoluteResolved, err := filepath.EvalSymlinks(commandAbsolute)
	if err != nil {
		return fmt.Errorf("resolve command path: %w", err)
	}
	if commandPath != commandAbsoluteResolved {
		return fmt.Errorf("command path is not canonical")
	}

	switch state.Profile {
	case runtimeprofile.ProfileFixture:
		if !isSafeIdentifier(state.FixtureVersion) || state.SnapshotID != "" || state.SnapshotCapturedAt != "" {
			return fmt.Errorf("fixture metadata is invalid")
		}
		expectedManifest := filepath.Join(root, "fixtures", state.FixtureVersion, "manifest.json")
		if err := requireExactManagedPath(state.ManifestPath, expectedManifest); err != nil {
			return fmt.Errorf("fixture manifest path rejected: %w", err)
		}
	case runtimeprofile.ProfileSnapshot:
		if !isSafeIdentifier(state.SnapshotID) || state.FixtureVersion != "" {
			return fmt.Errorf("snapshot metadata is invalid")
		}
		if _, err := parseRFC3339(state.SnapshotCapturedAt); err != nil {
			return fmt.Errorf("snapshot capture time is invalid: %w", err)
		}
		expectedManifest := filepath.Join(root, "snapshots", state.SnapshotID, "manifest.json")
		if err := requireExactManagedPath(state.ManifestPath, expectedManifest); err != nil {
			return fmt.Errorf("snapshot manifest path rejected: %w", err)
		}
	}
	return nil
}

func requireExactManagedPath(path, expected string) error {
	actualResolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	expectedResolved, err := filepath.EvalSymlinks(expected)
	if err != nil {
		return err
	}
	if actualResolved != expectedResolved || path != actualResolved {
		return fmt.Errorf("path does not match the canonical managed target")
	}
	info, err := os.Lstat(actualResolved)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path is not a regular file")
	}
	return nil
}

func publicStatusFromState(state RuntimeState) PublicStatus {
	return PublicStatus{
		Managed:            true,
		Profile:            state.Profile,
		Ready:              true,
		SchemaVersion:      state.SchemaVersion,
		FixtureVersion:     state.FixtureVersion,
		SnapshotID:         state.SnapshotID,
		SnapshotCapturedAt: state.SnapshotCapturedAt,
		InstanceID:         state.InstanceID,
	}
}

func sameTarget(current, target RuntimeState) bool {
	return current.Profile == target.Profile &&
		current.DatabasePath == target.DatabasePath &&
		current.FixtureVersion == target.FixtureVersion &&
		current.SnapshotID == target.SnapshotID &&
		current.SnapshotCapturedAt == target.SnapshotCapturedAt &&
		current.Port == target.Port
}

func randomIdentifier() (string, error) {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("create profile instance ID: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}

func FreeLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
