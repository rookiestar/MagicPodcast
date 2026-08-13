package dataprofile

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mattn/go-sqlite3"
)

type ExportResult struct {
	DatabasePath string
	ManifestPath string
	Manifest     Manifest
}

func ExportSanitizedSnapshot(sourcePath, outputDir, snapshotID, capturedAt string) (ExportResult, error) {
	return ExportSanitizedSnapshotContext(
		context.Background(),
		sourcePath,
		outputDir,
		snapshotID,
		capturedAt,
	)
}

func ExportSanitizedSnapshotContext(
	ctx context.Context,
	sourcePath,
	outputDir,
	snapshotID,
	capturedAt string,
) (ExportResult, error) {
	if !isSafeIdentifier(snapshotID) {
		return ExportResult{}, fmt.Errorf("snapshot ID is invalid")
	}
	if _, err := parseRFC3339(capturedAt); err != nil {
		return ExportResult{}, err
	}
	sourceInfo, err := os.Lstat(sourcePath)
	if err != nil {
		return ExportResult{}, fmt.Errorf("inspect production database: %w", err)
	}
	if !sourceInfo.Mode().IsRegular() {
		return ExportResult{}, fmt.Errorf("production database must be a regular file")
	}
	outputDir, err = requireEmptySecureDirectory(outputDir)
	if err != nil {
		return ExportResult{}, fmt.Errorf("export staging directory rejected: %w", err)
	}
	databasePath := filepath.Join(outputDir, "magicpodcast.db")
	manifestPath := filepath.Join(outputDir, "manifest.json")
	if _, err := os.Stat(databasePath); err == nil {
		return ExportResult{}, fmt.Errorf("export destination already exists")
	} else if !os.IsNotExist(err) {
		return ExportResult{}, err
	}

	tempPath := databasePath + ".tmp"
	defer os.Remove(tempPath)
	if err := consistentSQLiteBackupContext(ctx, sourcePath, tempPath, 10*time.Second); err != nil {
		return ExportResult{}, err
	}
	exportDB, err := sql.Open("sqlite3", "file:"+tempPath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return ExportResult{}, fmt.Errorf("open exported snapshot: %w", err)
	}
	if err := SanitizeSnapshot(exportDB); err != nil {
		exportDB.Close()
		return ExportResult{}, err
	}
	if err := exportDB.Close(); err != nil {
		return ExportResult{}, err
	}
	if _, err := ValidateDatabase(tempPath); err != nil {
		return ExportResult{}, fmt.Errorf("validate sanitized export: %w", err)
	}
	hash, err := SHA256File(tempPath)
	if err != nil {
		return ExportResult{}, err
	}
	status, err := ValidateDatabase(tempPath)
	if err != nil {
		return ExportResult{}, err
	}
	manifest := Manifest{
		FormatVersion:    ManifestFormatVersion,
		Kind:             "transfer",
		ID:               snapshotID,
		SchemaVersion:    status.SchemaVersion,
		CapturedAt:       capturedAt,
		SanitizerVersion: SanitizerVersion,
		SHA256:           hash,
		Counts:           status.Counts,
	}
	if err := os.Chmod(tempPath, 0o400); err != nil {
		return ExportResult{}, fmt.Errorf("set exported snapshot read-only: %w", err)
	}
	if err := os.Rename(tempPath, databasePath); err != nil {
		return ExportResult{}, fmt.Errorf("publish exported database: %w", err)
	}
	if err := WriteManifest(manifestPath, manifest); err != nil {
		_ = os.Remove(databasePath)
		return ExportResult{}, err
	}
	if err := os.Chmod(manifestPath, 0o400); err != nil {
		_ = os.Remove(databasePath)
		_ = os.Remove(manifestPath)
		return ExportResult{}, err
	}
	return ExportResult{DatabasePath: databasePath, ManifestPath: manifestPath, Manifest: manifest}, nil
}

func requireEmptySecureDirectory(path string) (string, error) {
	resolved, err := requireRealDirectory(path)
	if err != nil {
		return "", err
	}
	if err := os.Chmod(resolved, 0o700); err != nil {
		return "", fmt.Errorf("secure directory permissions: %w", err)
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return "", fmt.Errorf("read directory: %w", err)
	}
	if len(entries) != 0 {
		return "", fmt.Errorf("directory must be empty")
	}
	return resolved, nil
}

func consistentSQLiteBackup(sourcePath, destinationPath string) error {
	return consistentSQLiteBackupContext(
		context.Background(),
		sourcePath,
		destinationPath,
		10*time.Second,
	)
}

func consistentSQLiteBackupContext(
	ctx context.Context,
	sourcePath,
	destinationPath string,
	stallTimeout time.Duration,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("SQLite consistent backup canceled: %w", err)
	}
	if stallTimeout <= 0 {
		return fmt.Errorf("SQLite backup stall timeout must be positive")
	}
	timeoutMilliseconds := stallTimeout.Milliseconds()
	if timeoutMilliseconds < 1 {
		timeoutMilliseconds = 1
	}
	sourceURL := (&url.URL{Scheme: "file", Path: sourcePath}).String() +
		"?mode=ro&_busy_timeout=" + strconv.FormatInt(timeoutMilliseconds, 10)
	destinationURL := (&url.URL{Scheme: "file", Path: destinationPath}).String() +
		"?_busy_timeout=" + strconv.FormatInt(timeoutMilliseconds, 10)
	source, err := sql.Open("sqlite3", sourceURL)
	if err != nil {
		return fmt.Errorf("open production database read-only: %w", err)
	}
	defer source.Close()
	destination, err := sql.Open("sqlite3", destinationURL)
	if err != nil {
		return fmt.Errorf("open backup destination: %w", err)
	}
	defer destination.Close()

	sourceConn, err := source.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("acquire production database connection: %w", err)
	}
	defer sourceConn.Close()
	destinationConn, err := destination.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("acquire backup destination connection: %w", err)
	}
	defer destinationConn.Close()

	return destinationConn.Raw(func(destinationDriver any) error {
		destinationSQLite, ok := destinationDriver.(*sqlite3.SQLiteConn)
		if !ok {
			return fmt.Errorf("unexpected destination SQLite driver")
		}
		return sourceConn.Raw(func(sourceDriver any) error {
			sourceSQLite, ok := sourceDriver.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("unexpected source SQLite driver")
			}
			backup, err := destinationSQLite.Backup("main", sourceSQLite, "main")
			if err != nil {
				return fmt.Errorf("initialize SQLite consistent backup: %w", err)
			}
			lastRemaining := -1
			var noProgressSince time.Time
			for {
				if err := ctx.Err(); err != nil {
					_ = backup.Close()
					return fmt.Errorf("SQLite consistent backup canceled: %w", err)
				}
				done, err := backup.Step(256)
				if err != nil {
					_ = backup.Close()
					return fmt.Errorf("copy SQLite backup pages: %w", err)
				}
				if done {
					return backup.Close()
				}
				pageCount := backup.PageCount()
				remaining := backup.Remaining()
				madeProgress := false
				if pageCount > 0 {
					madeProgress = remaining < pageCount &&
						(lastRemaining < 0 || remaining < lastRemaining)
					lastRemaining = remaining
				}
				retryDelay := 10 * time.Millisecond
				if madeProgress {
					noProgressSince = time.Time{}
					continue
				}
				if noProgressSince.IsZero() {
					noProgressSince = time.Now()
				}
				idleFor := time.Since(noProgressSince)
				if idleFor >= stallTimeout {
					_ = backup.Close()
					return fmt.Errorf("SQLite consistent backup made no progress for %s", stallTimeout)
				}
				retryDelay = min(retryDelay, stallTimeout-idleFor)
				timer := time.NewTimer(retryDelay)
				select {
				case <-ctx.Done():
					timer.Stop()
					_ = backup.Close()
					return fmt.Errorf("SQLite consistent backup canceled: %w", ctx.Err())
				case <-timer.C:
				}
			}
		})
	})
}

func verifyTransferBundle(directory string) (Manifest, string, error) {
	databasePath := filepath.Join(directory, "magicpodcast.db")
	manifestPath := filepath.Join(directory, "manifest.json")
	for _, item := range []struct {
		label string
		path  string
	}{
		{label: "database", path: databasePath},
		{label: "manifest", path: manifestPath},
	} {
		info, err := os.Lstat(item.path)
		if err != nil {
			return Manifest{}, "", fmt.Errorf("inspect transfer %s: %w", item.label, err)
		}
		if !info.Mode().IsRegular() {
			return Manifest{}, "", fmt.Errorf("transfer %s must be a regular file", item.label)
		}
	}
	manifest, _, err := ValidateManifest(databasePath, manifestPath, "transfer")
	if err != nil {
		return Manifest{}, "", err
	}
	if manifest.SanitizerVersion != SanitizerVersion {
		return Manifest{}, "", fmt.Errorf("unsupported sanitizer version %q", manifest.SanitizerVersion)
	}
	db, err := sql.Open("sqlite3", "file:"+databasePath+"?mode=ro&_query_only=1")
	if err != nil {
		return Manifest{}, "", err
	}
	defer db.Close()
	if err := VerifySanitizedSnapshot(db); err != nil {
		return Manifest{}, "", err
	}
	return manifest, databasePath, nil
}
