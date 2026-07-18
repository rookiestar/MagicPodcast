package upgrade

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func Cutover(candidatePath, livePath string, manifest Manifest, serviceStopped, dryRun bool, confirmation string) (CutoverRecord, error) {
	record := CutoverRecord{
		Status:         "blocked",
		ServiceStopped: serviceStopped,
		DryRun:         dryRun,
		CandidatePath:  candidatePath,
		LivePath:       livePath,
		StartedAt:      time.Now().UTC(),
	}
	if !serviceStopped {
		return recordWithError(record, fmt.Errorf("refusing cutover while service-stopped confirmation is absent"))
	}
	if !dryRun && confirmation != CutoverConfirmation {
		return recordWithError(record, fmt.Errorf("invalid cutover confirmation"))
	}
	if !manifest.Decision.Go {
		return recordWithError(record, fmt.Errorf("manifest is No-Go; candidate cannot be switched"))
	}
	if !manifest.Candidate.Passed || !manifest.Disk.Passed || !manifest.Archive.GzipValid || !manifest.Archive.TarValid || manifest.Source.SHA256 == "" || manifest.Candidate.SHA256 == "" {
		return recordWithError(record, fmt.Errorf("manifest is missing archive, disk, hash or candidate validation evidence"))
	}
	if manifest.Comparison == nil || manifest.Comparison.ExpectedSamples != 146 || !manifest.Comparison.AccessibilityChecked {
		return recordWithError(record, fmt.Errorf("manifest is missing the required 146-sample accessibility comparison"))
	}
	if !manifest.Cutover.RollbackTested {
		return recordWithError(record, fmt.Errorf("manifest has no successful cutover/rollback self-test"))
	}
	absCandidate, err := filepath.Abs(candidatePath)
	if err != nil {
		return recordWithError(record, fmt.Errorf("resolve candidate path: %w", err))
	}
	absLive, err := filepath.Abs(livePath)
	if err != nil {
		return recordWithError(record, fmt.Errorf("resolve live path: %w", err))
	}
	if absCandidate == absLive {
		return recordWithError(record, fmt.Errorf("candidate and live database paths must be different"))
	}
	if _, err := os.Stat(candidatePath); err != nil {
		return recordWithError(record, fmt.Errorf("candidate database is unavailable: %w", err))
	}
	if _, err := os.Stat(livePath); err != nil {
		return recordWithError(record, fmt.Errorf("live database is unavailable: %w", err))
	}
	if err := EnsureSameFilesystem(nil, candidatePath, livePath); err != nil {
		return recordWithError(record, err)
	}
	if err := ensureNoOpenHandles(livePath); err != nil {
		return recordWithError(record, err)
	}
	if err := ensureNoOpenHandles(candidatePath); err != nil {
		return recordWithError(record, err)
	}
	if manifest.Candidate.SHA256 != "" {
		sha256, _, err := SHA256File(candidatePath)
		if err != nil {
			return recordWithError(record, err)
		}
		if sha256 != manifest.Candidate.SHA256 {
			return recordWithError(record, fmt.Errorf("candidate SHA-256 does not match manifest"))
		}
	}

	record.Status = "ready"
	backupPath := livePath + ".rollback-" + record.StartedAt.Format("20060102T150405Z")
	if _, err := os.Stat(backupPath); err == nil {
		backupPath += "-" + fmt.Sprintf("%d", os.Getpid())
	}
	record.BackupPath = backupPath
	if dryRun {
		record.CompletedAt = time.Now().UTC()
		return record, nil
	}

	liveSidecars, err := moveSidecars(livePath, backupPath)
	if err != nil {
		return recordWithError(record, err)
	}
	candidateSidecars, err := moveSidecars(candidatePath, livePath)
	if err != nil {
		_ = restoreSidecars(liveSidecars)
		return recordWithError(record, err)
	}
	if err := os.Rename(livePath, backupPath); err != nil {
		_ = restoreSidecars(candidateSidecars)
		_ = restoreSidecars(liveSidecars)
		return recordWithError(record, fmt.Errorf("move current PodcastIndex database to rollback copy: %w", err))
	}
	if err := os.Rename(candidatePath, livePath); err != nil {
		_ = os.Rename(backupPath, livePath)
		_ = restoreSidecars(candidateSidecars)
		_ = restoreSidecars(liveSidecars)
		return recordWithError(record, fmt.Errorf("atomically install candidate database: %w", err))
	}
	if err := os.Chmod(backupPath, 0o444); err != nil {
		_, _ = moveSidecars(livePath, candidatePath)
		_ = os.Rename(livePath, candidatePath)
		_ = os.Rename(backupPath, livePath)
		_ = restoreSidecars(liveSidecars)
		return recordWithError(record, fmt.Errorf("protect rollback copy: %w", err))
	}
	backupSHA, _, err := SHA256File(backupPath)
	if err != nil {
		return recordWithError(record, fmt.Errorf("hash rollback copy: %w", err))
	}
	record.BackupSHA256 = backupSHA
	record.Status = "switched"
	record.CompletedAt = time.Now().UTC()
	return record, nil
}

func Rollback(livePath, backupPath string, manifest Manifest, serviceStopped, dryRun bool, confirmation string) (CutoverRecord, error) {
	record := CutoverRecord{
		Status:         "blocked",
		ServiceStopped: serviceStopped,
		DryRun:         dryRun,
		LivePath:       livePath,
		BackupPath:     backupPath,
		StartedAt:      time.Now().UTC(),
	}
	if !serviceStopped {
		return recordWithError(record, fmt.Errorf("refusing rollback while service-stopped confirmation is absent"))
	}
	if !dryRun && confirmation != RollbackConfirmation {
		return recordWithError(record, fmt.Errorf("invalid rollback confirmation"))
	}
	if _, err := os.Stat(backupPath); err != nil {
		return recordWithError(record, fmt.Errorf("rollback copy is unavailable: %w", err))
	}
	if err := EnsureSameFilesystem(nil, livePath, backupPath); err != nil {
		return recordWithError(record, err)
	}
	if err := ensureNoOpenHandles(livePath); err != nil {
		return recordWithError(record, err)
	}
	if err := ensureNoOpenHandles(backupPath); err != nil {
		return recordWithError(record, err)
	}
	if manifest.Cutover.BackupSHA256 != "" {
		sha256, _, err := SHA256File(backupPath)
		if err != nil {
			return recordWithError(record, err)
		}
		if sha256 != manifest.Cutover.BackupSHA256 {
			return recordWithError(record, fmt.Errorf("rollback copy SHA-256 does not match manifest"))
		}
	}
	record.Status = "ready"
	failedPath := livePath + ".failed-" + record.StartedAt.Format("20060102T150405Z")
	if _, err := os.Stat(failedPath); err == nil {
		failedPath += "-" + fmt.Sprintf("%d", os.Getpid())
	}
	record.FailedPath = failedPath
	if dryRun {
		record.CompletedAt = time.Now().UTC()
		return record, nil
	}

	liveSidecars, err := moveSidecars(livePath, failedPath)
	if err != nil {
		return recordWithError(record, err)
	}
	backupSidecars, err := moveSidecars(backupPath, livePath)
	if err != nil {
		_ = restoreSidecars(liveSidecars)
		return recordWithError(record, err)
	}
	if err := os.Rename(livePath, failedPath); err != nil {
		_ = restoreSidecars(backupSidecars)
		_ = restoreSidecars(liveSidecars)
		return recordWithError(record, fmt.Errorf("preserve failed candidate database: %w", err))
	}
	if err := os.Rename(backupPath, livePath); err != nil {
		_ = os.Rename(failedPath, livePath)
		_ = restoreSidecars(backupSidecars)
		_ = restoreSidecars(liveSidecars)
		return recordWithError(record, fmt.Errorf("restore rollback database: %w", err))
	}
	record.Status = "rolled_back"
	record.CompletedAt = time.Now().UTC()
	return record, nil
}

func SelfTestCutoverAndRollback() error {
	temporaryDir, err := os.MkdirTemp("", "podcastindex-cutover-self-test-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporaryDir)
	livePath := filepath.Join(temporaryDir, "live.db")
	candidatePath := filepath.Join(temporaryDir, "candidate.db")
	if err := os.WriteFile(livePath, []byte("old"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(candidatePath, []byte("new"), 0o600); err != nil {
		return err
	}
	candidateSHA, _, err := SHA256File(candidatePath)
	if err != nil {
		return err
	}
	manifest := NewManifest("self-test")
	manifest.Decision.Go = true
	manifest.Source.SHA256 = "self-test-source"
	manifest.Candidate.SHA256 = candidateSHA
	manifest.Candidate.Passed = true
	manifest.Disk.Passed = true
	manifest.Archive.GzipValid = true
	manifest.Archive.TarValid = true
	manifest.Comparison = &SampleComparison{ExpectedSamples: 146, AccessibilityChecked: true}
	manifest.Cutover.RollbackTested = true
	record, err := Cutover(candidatePath, livePath, manifest, true, false, CutoverConfirmation)
	if err != nil || record.Status != "switched" {
		return fmt.Errorf("self-test cutover failed: %v", err)
	}
	manifest.Cutover = record
	record, err = Rollback(livePath, record.BackupPath, manifest, true, false, RollbackConfirmation)
	if err != nil || record.Status != "rolled_back" {
		return fmt.Errorf("self-test rollback failed: %v", err)
	}
	contents, err := os.ReadFile(livePath)
	if err != nil {
		return err
	}
	if string(contents) != "old" {
		return fmt.Errorf("self-test rollback restored wrong contents")
	}
	return nil
}

func recordWithError(record CutoverRecord, err error) (CutoverRecord, error) {
	record.Status = "blocked"
	record.Error = err.Error()
	record.CompletedAt = time.Now().UTC()
	return record, err
}

func ensureNoOpenHandles(path string) error {
	paths := []string{path}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Lstat(path + suffix); err == nil {
			paths = append(paths, path+suffix)
		}
	}
	if _, err := exec.LookPath("lsof"); err != nil {
		return fmt.Errorf("cannot verify open handles for %s: lsof is unavailable", path)
	}
	for _, candidate := range paths {
		output, err := exec.Command("lsof", "-t", "--", candidate).CombinedOutput()
		if err != nil {
			// lsof returns exit status 1 when no process has the file open.
			if len(strings.TrimSpace(string(output))) == 0 {
				continue
			}
			return fmt.Errorf("cannot verify open handles for %s: %s", candidate, strings.TrimSpace(string(output)))
		}
		if strings.TrimSpace(string(output)) != "" {
			return fmt.Errorf("database has open handles; stop the service and close readers first: %s", strings.TrimSpace(string(output)))
		}
	}
	return nil
}

type movedSidecar struct {
	Original string
	Moved    string
}

func moveSidecars(livePath, destinationBase string) ([]movedSidecar, error) {
	var moved []movedSidecar
	for _, suffix := range []string{"-wal", "-shm"} {
		original := livePath + suffix
		if _, err := os.Lstat(original); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return moved, fmt.Errorf("inspect SQLite sidecar %s: %w", original, err)
		}
		movedPath := destinationBase + suffix
		if err := os.Rename(original, movedPath); err != nil {
			_ = restoreSidecars(moved)
			return moved, fmt.Errorf("preserve SQLite sidecar %s: %w", original, err)
		}
		moved = append(moved, movedSidecar{Original: original, Moved: movedPath})
	}
	return moved, nil
}

func restoreSidecars(sidecars []movedSidecar) error {
	var firstErr error
	for index := len(sidecars) - 1; index >= 0; index-- {
		sidecar := sidecars[index]
		if err := os.Rename(sidecar.Moved, sidecar.Original); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
