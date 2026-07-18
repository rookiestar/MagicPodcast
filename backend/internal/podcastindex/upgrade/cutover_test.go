package upgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelfTestCutoverAndRollback(t *testing.T) {
	if err := SelfTestCutoverAndRollback(); err != nil {
		t.Fatalf("SelfTestCutoverAndRollback() error = %v", err)
	}
}

func TestCutoverDryRunDoesNotReplaceLiveDatabase(t *testing.T) {
	directory := t.TempDir()
	livePath := filepath.Join(directory, "live.db")
	candidatePath := filepath.Join(directory, "candidate.db")
	if err := os.WriteFile(livePath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidatePath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidateSHA, _, err := SHA256File(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := NewManifest("test")
	manifest.Decision.Go = true
	manifest.Source.SHA256 = "test-source"
	manifest.Candidate.SHA256 = candidateSHA
	manifest.Candidate.Passed = true
	manifest.Disk.Passed = true
	manifest.Archive.GzipValid = true
	manifest.Archive.TarValid = true
	manifest.Comparison = &SampleComparison{ExpectedSamples: 146, AccessibilityChecked: true}
	manifest.Cutover.RollbackTested = true
	record, err := Cutover(candidatePath, livePath, manifest, true, true, "")
	if err != nil {
		t.Fatalf("Cutover() dry-run error = %v", err)
	}
	if record.Status != "ready" {
		t.Fatalf("record = %+v", record)
	}
	contents, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "old" {
		t.Fatalf("live contents = %q, dry-run should not mutate", contents)
	}
}

func TestCutoverRejectsNoGoAndRunningService(t *testing.T) {
	directory := t.TempDir()
	livePath := filepath.Join(directory, "live.db")
	candidatePath := filepath.Join(directory, "candidate.db")
	if err := os.WriteFile(livePath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidatePath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := NewManifest("test")
	if _, err := Cutover(candidatePath, livePath, manifest, true, true, ""); err == nil || !strings.Contains(err.Error(), "No-Go") {
		t.Fatalf("No-Go cutover error = %v", err)
	}
	manifest.Decision.Go = true
	if _, err := Cutover(candidatePath, livePath, manifest, false, false, CutoverConfirmation); err == nil {
		t.Fatal("Cutover() accepted a running service")
	}
}

func TestManifestIsWrittenAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	manifest := NewManifest("manifest-test")
	manifest.Blockers = []string{"fixture blocker"}
	if err := WriteManifestAtomic(path, manifest); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Scope != "manifest-test" || len(loaded.Blockers) != 1 {
		t.Fatalf("loaded = %+v", loaded)
	}
	if mode := mustFileMode(t, path); mode.Perm() != 0o600 {
		t.Fatalf("manifest mode = %o", mode.Perm())
	}
}

func TestManifestOmitsUnknownDownloadTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := WriteManifestAtomic(path, NewManifest("manifest-test")); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), `"downloaded_at"`) {
		t.Fatalf("manifest recorded an unknown download time: %s", contents)
	}
}

func TestCutoverAndRollbackMoveSQLiteSidecarsWithDatabase(t *testing.T) {
	directory := t.TempDir()
	livePath := filepath.Join(directory, "live.db")
	candidatePath := filepath.Join(directory, "candidate.db")
	if err := os.WriteFile(livePath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidatePath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, file := range []struct {
		path    string
		content string
	}{
		{livePath + "-wal", "old-wal"},
		{livePath + "-shm", "old-shm"},
		{candidatePath + "-wal", "new-wal"},
		{candidatePath + "-shm", "new-shm"},
	} {
		if err := os.WriteFile(file.path, []byte(file.content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	candidateSHA, _, err := SHA256File(candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := NewManifest("sidecar-test")
	manifest.Decision.Go = true
	manifest.Source.SHA256 = "source"
	manifest.Candidate.SHA256 = candidateSHA
	manifest.Candidate.Passed = true
	manifest.Disk.Passed = true
	manifest.Archive.GzipValid = true
	manifest.Archive.TarValid = true
	manifest.Comparison = &SampleComparison{ExpectedSamples: 146, AccessibilityChecked: true}
	manifest.Cutover.RollbackTested = true

	record, err := Cutover(candidatePath, livePath, manifest, true, false, CutoverConfirmation)
	if err != nil {
		t.Fatalf("Cutover() error = %v", err)
	}
	for _, file := range []struct {
		path    string
		content string
	}{
		{livePath + "-wal", "new-wal"},
		{livePath + "-shm", "new-shm"},
		{record.BackupPath + "-wal", "old-wal"},
		{record.BackupPath + "-shm", "old-shm"},
	} {
		contents, readErr := os.ReadFile(file.path)
		if readErr != nil || string(contents) != file.content {
			t.Fatalf("sidecar %s = %q err=%v", file.path, contents, readErr)
		}
	}

	manifest.Cutover = record
	rollback, err := Rollback(livePath, record.BackupPath, manifest, true, false, RollbackConfirmation)
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if rollback.Status != "rolled_back" {
		t.Fatalf("rollback = %+v", rollback)
	}
	for _, file := range []struct {
		path    string
		content string
	}{
		{livePath + "-wal", "old-wal"},
		{livePath + "-shm", "old-shm"},
		{rollback.FailedPath + "-wal", "new-wal"},
		{rollback.FailedPath + "-shm", "new-shm"},
	} {
		contents, readErr := os.ReadFile(file.path)
		if readErr != nil || string(contents) != file.content {
			t.Fatalf("rolled-back sidecar %s = %q err=%v", file.path, contents, readErr)
		}
	}
}

func mustFileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}
