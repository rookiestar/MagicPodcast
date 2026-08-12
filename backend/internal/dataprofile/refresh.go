package dataprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const RefreshConfirmation = "I_AUTHORIZE_PRODUCTION_SNAPSHOT_READ_TRANSFER_AND_SANITIZATION"

type RefreshRequest struct {
	Source       string
	Confirmation string
	Keep         int
}

type Transfer interface {
	Fetch(context.Context, string) (TransferResult, error)
}

type TransferResult struct {
	Directory string
	Cleanup   func() error
}

type LocalDirectoryTransfer struct{}

func (LocalDirectoryTransfer) Fetch(_ context.Context, source string) (TransferResult, error) {
	if strings.TrimSpace(source) == "" {
		return TransferResult{}, fmt.Errorf("transfer source is required")
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return TransferResult{}, fmt.Errorf("resolve transfer source: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return TransferResult{}, fmt.Errorf("transfer source unavailable: %w", err)
	}
	if !info.IsDir() {
		return TransferResult{}, fmt.Errorf("transfer source must be a real directory, not a symlink or file")
	}
	return TransferResult{Directory: absolute}, nil
}

// CommandTransfer invokes a fixed, operator-configured transfer adapter.
// Credentials belong in that adapter's protected environment/config and are
// never accepted as data-profile command arguments.
type CommandTransfer struct {
	Command []string
}

func (t CommandTransfer) Fetch(ctx context.Context, _ string) (TransferResult, error) {
	if len(t.Command) == 0 {
		return TransferResult{}, fmt.Errorf("transfer adapter is not configured")
	}
	executable, err := requireAbsoluteExecutable(t.Command[0])
	if err != nil {
		return TransferResult{}, fmt.Errorf("transfer adapter rejected: %w", err)
	}
	stagingPath, err := os.MkdirTemp("", "magicpodcast-snapshot-transfer-*")
	if err != nil {
		return TransferResult{}, fmt.Errorf("create owned transfer staging directory: %w", err)
	}
	cleanup := func() error {
		return os.RemoveAll(stagingPath)
	}
	arguments := append(append([]string(nil), t.Command[1:]...), stagingPath)
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Stdout = nil
	command.Stderr = nil
	err = command.Run()
	if err != nil {
		_ = cleanup()
		return TransferResult{}, fmt.Errorf("transfer adapter failed")
	}
	directory, err := requireRealDirectory(stagingPath)
	if err != nil {
		_ = cleanup()
		return TransferResult{}, fmt.Errorf("transfer adapter staging directory is invalid: %w", err)
	}
	return TransferResult{
		Directory: directory,
		Cleanup: func() error {
			if _, err := requireRealDirectory(directory); err != nil {
				return fmt.Errorf("refuse to clean transfer staging directory: %w", err)
			}
			return os.RemoveAll(directory)
		},
	}, nil
}

func (c Controller) RefreshSnapshot(ctx context.Context, request RefreshRequest, transfer Transfer) (Snapshot, error) {
	if err := c.validate(); err != nil {
		return Snapshot{}, err
	}
	unlock, err := c.lockOperation()
	if err != nil {
		return Snapshot{}, err
	}
	defer unlock()
	if request.Confirmation != RefreshConfirmation {
		return Snapshot{}, fmt.Errorf("snapshot refresh requires explicit production read, transfer, and sanitization confirmation")
	}
	if request.Keep < 1 {
		return Snapshot{}, fmt.Errorf("snapshot retention must keep at least one snapshot")
	}
	if transfer == nil {
		return Snapshot{}, fmt.Errorf("snapshot transfer is not configured")
	}
	beforeState, beforeStateErr := c.readState()
	if beforeStateErr != nil && !os.IsNotExist(beforeStateErr) {
		return Snapshot{}, beforeStateErr
	}
	previousLatestPointer, latestErr := readLatest(c.ProfileHome)
	if latestErr != nil && !os.IsNotExist(latestErr) {
		return Snapshot{}, fmt.Errorf("current latest snapshot pointer is invalid: %w", latestErr)
	}
	previousLatest := previousLatestPointer.SnapshotID
	snapshot, err := c.fetchAndPublishPreparedSnapshot(ctx, request.Source, transfer)
	if err != nil {
		return Snapshot{}, err
	}
	if err := c.persistLatest(snapshot); err != nil {
		_ = removePublishedSnapshot(c.ProfileHome, snapshot.ID)
		return Snapshot{}, err
	}
	retention, err := c.stageSnapshotRetention(request.Keep, snapshot.ID, previousLatest, beforeState)
	if err != nil {
		return Snapshot{}, c.rollbackRefresh(snapshot, nil, previousLatest, fmt.Errorf("snapshot retention failed: %w", err))
	}
	if c.afterPublishHook != nil {
		c.afterPublishHook()
	}

	afterState, afterStateErr := c.readState()
	if beforeStateErr == nil {
		if afterStateErr != nil || afterState.InstanceID != beforeState.InstanceID ||
			afterState.Profile != beforeState.Profile || afterState.DatabasePath != beforeState.DatabasePath {
			return Snapshot{}, c.rollbackRefresh(
				snapshot,
				&retention,
				previousLatest,
				fmt.Errorf("snapshot refresh changed the active profile"),
			)
		}
	} else if afterStateErr == nil || !os.IsNotExist(afterStateErr) {
		return Snapshot{}, c.rollbackRefresh(
			snapshot,
			&retention,
			previousLatest,
			fmt.Errorf("snapshot refresh unexpectedly activated a profile"),
		)
	}
	if err := c.commitSnapshotRetention(&retention); err != nil {
		return Snapshot{}, c.rollbackRefresh(
			snapshot,
			&retention,
			previousLatest,
			fmt.Errorf("snapshot retention commit failed: %w", err),
		)
	}
	// Physical deletion happens only after the atomic visibility commit. A
	// cleanup failure leaves an owned hidden garbage directory for a later
	// retry; it never turns a committed refresh into a half-rollback.
	_ = retention.Cleanup()
	return snapshot, nil
}

func (c Controller) fetchAndPublishPreparedSnapshot(
	ctx context.Context,
	source string,
	transfer Transfer,
) (snapshot Snapshot, returnErr error) {
	transferResult, err := transfer.Fetch(ctx, source)
	if err != nil {
		return Snapshot{}, err
	}
	cleaned := transferResult.Cleanup == nil
	defer func() {
		if cleaned {
			return
		}
		if cleanupErr := transferResult.Cleanup(); cleanupErr != nil {
			if returnErr == nil {
				returnErr = fmt.Errorf("clean transfer staging directory: %w", cleanupErr)
			} else {
				returnErr = fmt.Errorf("%v; clean transfer staging directory: %w", returnErr, cleanupErr)
			}
		}
	}()
	stagingPath, err := requireRealDirectory(transferResult.Directory)
	if err != nil {
		return Snapshot{}, fmt.Errorf("transferred snapshot staging directory is invalid: %w", err)
	}
	manifest, databasePath, err := verifyTransferBundle(stagingPath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("transferred snapshot rejected: %w", err)
	}
	snapshot, err = PublishPreparedSnapshot(
		c.ProfileHome,
		manifest.ID,
		databasePath,
		manifest.CapturedAt,
		manifest.SanitizerVersion,
	)
	if err != nil {
		return Snapshot{}, err
	}
	if transferResult.Cleanup != nil {
		if err := transferResult.Cleanup(); err != nil {
			_ = removePublishedSnapshot(c.ProfileHome, snapshot.ID)
			return Snapshot{}, fmt.Errorf("clean transfer staging before latest commit: %w", err)
		}
		cleaned = true
	}
	return snapshot, nil
}

func (c Controller) writeLatest(snapshot Snapshot) error {
	payload := latestSnapshotPointer{
		FormatVersion: ManifestFormatVersion,
		SnapshotID:    snapshot.ID,
		CapturedAt:    snapshot.Manifest.CapturedAt,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(c.ProfileHome, "snapshots", "latest.json"), append(data, '\n'), 0o600)
}

func (c Controller) persistLatest(snapshot Snapshot) error {
	if c.writeLatestHook != nil {
		return c.writeLatestHook(snapshot)
	}
	return c.writeLatest(snapshot)
}

func (c Controller) restoreLatest(previousLatest string) error {
	root, err := ensureRoot(c.ProfileHome)
	if err != nil {
		return err
	}
	latestPath := filepath.Join(root, "snapshots", "latest.json")
	if previousLatest == "" {
		if err := os.Remove(latestPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	snapshot, err := ResolveSnapshot(root, previousLatest)
	if err != nil {
		return err
	}
	return c.writeLatest(snapshot)
}

func (c Controller) rollbackRefresh(
	snapshot Snapshot,
	retention *snapshotRetention,
	previousLatest string,
	cause error,
) error {
	var rollbackErrors []string
	if retention != nil {
		if err := retention.Rollback(); err != nil {
			rollbackErrors = append(rollbackErrors, "retention: "+err.Error())
		}
	}
	if err := c.restoreLatest(previousLatest); err != nil {
		rollbackErrors = append(rollbackErrors, "latest: "+err.Error())
	}
	if err := removePublishedSnapshot(c.ProfileHome, snapshot.ID); err != nil {
		rollbackErrors = append(rollbackErrors, "new snapshot: "+err.Error())
	}
	if len(rollbackErrors) != 0 {
		return fmt.Errorf("%v; rollback incomplete: %s", cause, strings.Join(rollbackErrors, "; "))
	}
	return fmt.Errorf("%w; previous snapshot state restored", cause)
}

func removePublishedSnapshot(profileHome, id string) error {
	if !isSafeIdentifier(id) {
		return fmt.Errorf("snapshot ID is invalid")
	}
	root, err := ensureRoot(profileHome)
	if err != nil {
		return err
	}
	snapshotsRoot := filepath.Join(root, "snapshots")
	directory := filepath.Join(snapshotsRoot, id)
	if _, err := requireExistingPathWithin(directory, snapshotsRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("refuse to remove unmanaged snapshot path: %w", err)
	}
	return os.RemoveAll(directory)
}

type snapshotRetention struct {
	quarantineRoot string
	garbageRoot    string
	moves          []snapshotMove
	removeAll      func(string) error
	committed      bool
}

type snapshotMove struct {
	original   string
	quarantine string
}

func (r *snapshotRetention) Rollback() error {
	if r == nil || r.quarantineRoot == "" {
		return nil
	}
	if r.committed {
		return fmt.Errorf("snapshot retention is already committed")
	}
	var rollbackErrors []string
	for index := len(r.moves) - 1; index >= 0; index-- {
		move := r.moves[index]
		if err := os.Rename(move.quarantine, move.original); err != nil {
			rollbackErrors = append(rollbackErrors, err.Error())
		}
	}
	if r.quarantineRoot != "" {
		if err := os.Remove(r.quarantineRoot); err != nil && !os.IsNotExist(err) {
			rollbackErrors = append(rollbackErrors, err.Error())
		}
	}
	if len(rollbackErrors) != 0 {
		return fmt.Errorf("restore quarantined snapshots: %s", strings.Join(rollbackErrors, "; "))
	}
	return nil
}

func (r *snapshotRetention) Commit() error {
	if r == nil || r.quarantineRoot == "" {
		return nil
	}
	if r.committed {
		return nil
	}
	if r.garbageRoot == "" {
		return fmt.Errorf("snapshot retention garbage path is missing")
	}
	if err := os.Rename(r.quarantineRoot, r.garbageRoot); err != nil {
		return err
	}
	r.committed = true
	for index := range r.moves {
		r.moves[index].quarantine = filepath.Join(r.garbageRoot, filepath.Base(r.moves[index].quarantine))
	}
	return nil
}

func (r *snapshotRetention) Cleanup() error {
	if r == nil || r.garbageRoot == "" || !r.committed {
		return nil
	}
	removeAll := r.removeAll
	if removeAll == nil {
		removeAll = os.RemoveAll
	}
	if err := removeAll(r.garbageRoot); err != nil {
		return err
	}
	return nil
}

func (c Controller) commitSnapshotRetention(retention *snapshotRetention) error {
	if c.retentionHook != nil {
		return c.retentionHook(retention)
	}
	return retention.Commit()
}

func (c Controller) stageSnapshotRetention(keep int, newestID, previousLatest string, active RuntimeState) (snapshotRetention, error) {
	root, err := ensureRoot(c.ProfileHome)
	if err != nil {
		return snapshotRetention{}, err
	}
	snapshotsRoot := filepath.Join(root, "snapshots")
	entries, err := os.ReadDir(snapshotsRoot)
	if err != nil {
		return snapshotRetention{}, err
	}
	var snapshots []Snapshot
	for _, entry := range entries {
		if !entry.IsDir() || !isSafeIdentifier(entry.Name()) {
			continue
		}
		snapshot, resolveErr := resolveSnapshotByID(snapshotsRoot, entry.Name())
		if resolveErr == nil {
			snapshots = append(snapshots, snapshot)
		}
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].Manifest.CapturedAt == snapshots[j].Manifest.CapturedAt {
			return snapshots[i].ID > snapshots[j].ID
		}
		return snapshots[i].Manifest.CapturedAt > snapshots[j].Manifest.CapturedAt
	})
	protected := map[string]struct{}{newestID: {}}
	if previousLatest != "" {
		protected[previousLatest] = struct{}{}
	}
	if active.Profile == "snapshot" && active.SnapshotID != "" {
		protected[active.SnapshotID] = struct{}{}
	}
	kept := 0
	for _, snapshot := range snapshots {
		if kept < keep {
			protected[snapshot.ID] = struct{}{}
			kept++
		}
	}
	var expired []Snapshot
	for _, snapshot := range snapshots {
		if _, found := protected[snapshot.ID]; found {
			continue
		}
		expired = append(expired, snapshot)
	}
	if len(expired) == 0 {
		return snapshotRetention{}, nil
	}
	quarantineRoot, err := os.MkdirTemp(snapshotsRoot, ".retention-quarantine-*")
	if err != nil {
		return snapshotRetention{}, fmt.Errorf("create snapshot retention quarantine: %w", err)
	}
	retention := snapshotRetention{
		quarantineRoot: quarantineRoot,
		garbageRoot: filepath.Join(
			snapshotsRoot,
			".retention-garbage-"+filepath.Base(quarantineRoot),
		),
	}
	for _, snapshot := range expired {
		directory := filepath.Dir(snapshot.DatabasePath)
		if _, err := requireExistingPathWithin(directory, snapshotsRoot); err != nil {
			rollbackErr := retention.Rollback()
			if rollbackErr != nil {
				return snapshotRetention{}, fmt.Errorf("refuse to quarantine unmanaged snapshot path: %v; rollback: %w", err, rollbackErr)
			}
			return snapshotRetention{}, fmt.Errorf("refuse to quarantine unmanaged snapshot path: %w", err)
		}
		quarantinePath := filepath.Join(quarantineRoot, snapshot.ID)
		if err := os.Rename(directory, quarantinePath); err != nil {
			rollbackErr := retention.Rollback()
			if rollbackErr != nil {
				return snapshotRetention{}, fmt.Errorf("quarantine expired snapshot %s: %v; rollback: %w", snapshot.ID, err, rollbackErr)
			}
			return snapshotRetention{}, fmt.Errorf("quarantine expired snapshot %s: %w", snapshot.ID, err)
		}
		retention.moves = append(retention.moves, snapshotMove{
			original:   directory,
			quarantine: quarantinePath,
		})
	}
	return retention, nil
}

func (c Controller) enforceSnapshotRetention(keep int, newestID, previousLatest string, active RuntimeState) error {
	retention, err := c.stageSnapshotRetention(keep, newestID, previousLatest, active)
	if err != nil {
		return err
	}
	if err := c.commitSnapshotRetention(&retention); err != nil {
		return fmt.Errorf("remove snapshot retention quarantine: %w", err)
	}
	_ = retention.Cleanup()
	return nil
}

func DefaultSnapshotID(now time.Time) string {
	return "snapshot-" + now.UTC().Format("20060102T150405Z")
}
