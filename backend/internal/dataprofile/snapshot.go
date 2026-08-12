package dataprofile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Snapshot struct {
	ID           string
	DatabasePath string
	ManifestPath string
	Manifest     Manifest
}

type latestSnapshotPointer struct {
	FormatVersion int    `json:"format_version"`
	SnapshotID    string `json:"snapshot_id"`
	CapturedAt    string `json:"captured_at"`
}

func ResolveSnapshot(profileHome, selector string) (Snapshot, error) {
	root, err := ensureRoot(profileHome)
	if err != nil {
		return Snapshot{}, err
	}
	snapshotsRoot := filepath.Join(root, "snapshots")
	if selector == "" {
		return Snapshot{}, fmt.Errorf("snapshot selector is required")
	}
	if selector == "latest" {
		latest, latestErr := readLatest(root)
		if latestErr == nil {
			snapshot, resolveErr := resolveSnapshotByID(snapshotsRoot, latest.SnapshotID)
			if resolveErr != nil {
				return Snapshot{}, resolveErr
			}
			if snapshot.Manifest.CapturedAt != latest.CapturedAt {
				return Snapshot{}, fmt.Errorf("latest snapshot pointer metadata does not match its manifest")
			}
			return snapshot, nil
		}
		if !os.IsNotExist(latestErr) {
			return Snapshot{}, fmt.Errorf("latest snapshot pointer is invalid: %w", latestErr)
		}
		entries, readErr := os.ReadDir(snapshotsRoot)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return Snapshot{}, fmt.Errorf("no snapshots are available")
			}
			return Snapshot{}, fmt.Errorf("read snapshots directory: %w", readErr)
		}
		var candidates []Snapshot
		for _, entry := range entries {
			if !entry.IsDir() || !isSafeIdentifier(entry.Name()) {
				continue
			}
			candidate, candidateErr := resolveSnapshotByID(snapshotsRoot, entry.Name())
			if candidateErr == nil {
				candidates = append(candidates, candidate)
			}
		}
		if len(candidates) == 0 {
			return Snapshot{}, fmt.Errorf("no valid snapshots are available")
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Manifest.CapturedAt == candidates[j].Manifest.CapturedAt {
				return candidates[i].ID > candidates[j].ID
			}
			return candidates[i].Manifest.CapturedAt > candidates[j].Manifest.CapturedAt
		})
		return candidates[0], nil
	}
	if !isSafeIdentifier(selector) {
		return Snapshot{}, fmt.Errorf("snapshot selector is invalid")
	}
	return resolveSnapshotByID(snapshotsRoot, selector)
}

func readLatest(root string) (latestSnapshotPointer, error) {
	resolvedRoot, err := ensureRoot(root)
	if err != nil {
		return latestSnapshotPointer{}, err
	}
	latestPath := filepath.Join(resolvedRoot, "snapshots", "latest.json")
	info, err := os.Lstat(latestPath)
	if err != nil {
		return latestSnapshotPointer{}, err
	}
	if !info.Mode().IsRegular() {
		return latestSnapshotPointer{}, fmt.Errorf("latest snapshot pointer is not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return latestSnapshotPointer{}, fmt.Errorf("latest snapshot pointer permissions are too broad")
	}
	data, err := os.ReadFile(latestPath)
	if err != nil {
		return latestSnapshotPointer{}, err
	}
	var latest latestSnapshotPointer
	if err := json.Unmarshal(data, &latest); err != nil {
		return latestSnapshotPointer{}, err
	}
	if latest.FormatVersion != ManifestFormatVersion || !isSafeIdentifier(latest.SnapshotID) {
		return latestSnapshotPointer{}, fmt.Errorf("latest snapshot pointer is invalid")
	}
	if _, err := parseRFC3339(latest.CapturedAt); err != nil {
		return latestSnapshotPointer{}, fmt.Errorf("latest snapshot pointer capture time is invalid: %w", err)
	}
	return latest, nil
}

func resolveSnapshotByID(snapshotsRoot, id string) (Snapshot, error) {
	directory := filepath.Join(snapshotsRoot, id)
	databasePath := filepath.Join(directory, "magicpodcast.db")
	manifestPath := filepath.Join(directory, "manifest.json")
	if _, err := requireExistingPathWithin(databasePath, snapshotsRoot); err != nil {
		return Snapshot{}, fmt.Errorf("snapshot database path rejected: %w", err)
	}
	if _, err := requireExistingPathWithin(manifestPath, snapshotsRoot); err != nil {
		return Snapshot{}, fmt.Errorf("snapshot manifest path rejected: %w", err)
	}
	manifest, _, err := ValidateManifest(databasePath, manifestPath, "snapshot")
	if err != nil {
		return Snapshot{}, fmt.Errorf("snapshot %s is invalid: %w", id, err)
	}
	if manifest.ID != id {
		return Snapshot{}, fmt.Errorf("snapshot directory and manifest ID do not match")
	}
	if _, err := parseRFC3339(manifest.CapturedAt); err != nil {
		return Snapshot{}, err
	}
	if manifest.SanitizerVersion != SanitizerVersion {
		return Snapshot{}, fmt.Errorf("snapshot sanitizer version %q is not supported", manifest.SanitizerVersion)
	}
	info, err := os.Lstat(databasePath)
	if err != nil {
		return Snapshot{}, err
	}
	if info.Mode().Perm()&0o222 != 0 {
		return Snapshot{}, fmt.Errorf("snapshot baseline must be read-only")
	}
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return Snapshot{}, err
	}
	if manifestInfo.Mode().Perm()&0o222 != 0 {
		return Snapshot{}, fmt.Errorf("snapshot manifest must be read-only")
	}
	return Snapshot{
		ID:           id,
		DatabasePath: databasePath,
		ManifestPath: manifestPath,
		Manifest:     manifest,
	}, nil
}

func PublishPreparedSnapshot(profileHome, id, databasePath, capturedAt, sanitizerVersion string) (Snapshot, error) {
	if !isSafeIdentifier(id) {
		return Snapshot{}, fmt.Errorf("snapshot ID is invalid")
	}
	if _, err := parseRFC3339(capturedAt); err != nil {
		return Snapshot{}, err
	}
	if sanitizerVersion != SanitizerVersion {
		return Snapshot{}, fmt.Errorf("unsupported sanitizer version %q", sanitizerVersion)
	}
	if _, err := ValidateDatabase(databasePath); err != nil {
		return Snapshot{}, fmt.Errorf("prepared snapshot database is invalid: %w", err)
	}
	root, err := ensureRoot(profileHome)
	if err != nil {
		return Snapshot{}, err
	}
	snapshotsRoot := filepath.Join(root, "snapshots")
	if err := os.MkdirAll(snapshotsRoot, 0o700); err != nil {
		return Snapshot{}, fmt.Errorf("create snapshots directory: %w", err)
	}
	finalDir := filepath.Join(snapshotsRoot, id)
	if _, err := os.Stat(finalDir); err == nil {
		return Snapshot{}, fmt.Errorf("snapshot %s already exists", id)
	} else if !os.IsNotExist(err) {
		return Snapshot{}, fmt.Errorf("inspect snapshot destination: %w", err)
	}
	tempDir, err := os.MkdirTemp(snapshotsRoot, "."+id+".tmp-*")
	if err != nil {
		return Snapshot{}, fmt.Errorf("create snapshot staging directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	stagedDatabase := filepath.Join(tempDir, "magicpodcast.db")
	if err := copyRegularFile(databasePath, stagedDatabase, 0o600); err != nil {
		return Snapshot{}, err
	}
	stagedDB, err := openSQLDatabase(stagedDatabase, true)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open prepared snapshot for sanitizer verification: %w", err)
	}
	verifyErr := VerifySanitizedSnapshot(stagedDB)
	closeErr := stagedDB.Close()
	if verifyErr != nil {
		return Snapshot{}, fmt.Errorf("prepared snapshot is not sanitized: %w", verifyErr)
	}
	if closeErr != nil {
		return Snapshot{}, fmt.Errorf("close verified prepared snapshot: %w", closeErr)
	}
	status, err := ValidateDatabase(stagedDatabase)
	if err != nil {
		return Snapshot{}, fmt.Errorf("prepared snapshot is invalid: %w", err)
	}
	hash, err := SHA256File(stagedDatabase)
	if err != nil {
		return Snapshot{}, err
	}
	manifest := Manifest{
		FormatVersion:    ManifestFormatVersion,
		Kind:             "snapshot",
		ID:               id,
		SchemaVersion:    status.SchemaVersion,
		CapturedAt:       capturedAt,
		SanitizerVersion: sanitizerVersion,
		SHA256:           hash,
		Counts:           status.Counts,
	}
	if err := WriteManifest(filepath.Join(tempDir, "manifest.json"), manifest); err != nil {
		return Snapshot{}, err
	}
	if err := os.Chmod(stagedDatabase, 0o400); err != nil {
		return Snapshot{}, fmt.Errorf("set snapshot database mode: %w", err)
	}
	if err := os.Chmod(filepath.Join(tempDir, "manifest.json"), 0o400); err != nil {
		return Snapshot{}, fmt.Errorf("set snapshot manifest mode: %w", err)
	}
	if err := os.Rename(tempDir, finalDir); err != nil {
		return Snapshot{}, fmt.Errorf("publish snapshot: %w", err)
	}
	snapshot, err := resolveSnapshotByID(snapshotsRoot, id)
	if err != nil {
		_ = os.RemoveAll(finalDir)
		return Snapshot{}, err
	}
	return snapshot, nil
}

func copyRegularFile(sourcePath, targetPath string, mode os.FileMode) error {
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return fmt.Errorf("inspect source file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer source.Close()
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	if _, err := target.ReadFrom(source); err != nil {
		_ = target.Close()
		return fmt.Errorf("copy file: %w", err)
	}
	if err := target.Sync(); err != nil {
		_ = target.Close()
		return fmt.Errorf("sync copied file: %w", err)
	}
	if err := target.Close(); err != nil {
		return fmt.Errorf("close copied file: %w", err)
	}
	return nil
}
