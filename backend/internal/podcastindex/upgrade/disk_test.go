package upgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvaluateDiskGateRequiresArchiveDatabaseAndReserve(t *testing.T) {
	giB := int64(1024 * 1024 * 1024)
	probe := func(path string) (DiskStats, error) {
		return DiskStats{Path: path, FilesystemID: "volume-a", CapacityBytes: 100 * giB, AvailableBytes: 25 * giB}, nil
	}
	report, err := EvaluateDiskGate(probe, "/staging", 2*giB, 2*giB)
	if err != nil {
		t.Fatalf("EvaluateDiskGate() error = %v", err)
	}
	if !report.Passed || report.RequiredBytes != 24*giB {
		t.Fatalf("report = %+v", report)
	}
}

func TestEvaluateDiskGateRejectsInsufficientReserve(t *testing.T) {
	giB := int64(1024 * 1024 * 1024)
	probe := func(path string) (DiskStats, error) {
		return DiskStats{Path: path, FilesystemID: "volume-a", CapacityBytes: 100 * giB, AvailableBytes: 23 * giB}, nil
	}
	report, err := EvaluateDiskGate(probe, "/staging", 2*giB, 2*giB)
	if err == nil || report.Passed {
		t.Fatalf("report = %+v, err = %v; expected insufficient reserve", report, err)
	}
	if !strings.Contains(report.Reason, "insufficient disk space") {
		t.Fatalf("reason = %q", report.Reason)
	}
}

func TestEnsureSameFilesystemRejectsDifferentStagingVolume(t *testing.T) {
	probe := func(path string) (DiskStats, error) {
		id := "volume-a"
		if filepath.Base(path) == "other" {
			id = "volume-b"
		}
		return DiskStats{Path: path, FilesystemID: id}, nil
	}
	if err := EnsureSameFilesystem(probe, "/staging", "/other"); err == nil {
		t.Fatal("EnsureSameFilesystem() accepted different volumes")
	}
}

func TestEnsureStagingSeparateRejectsLiveDirectory(t *testing.T) {
	if err := EnsureStagingSeparate("/var/lib/magicpodcast/staging", "/var/lib/magicpodcast/podcastindex_feeds.db"); err == nil {
		t.Fatal("EnsureStagingSeparate() accepted staging below live database directory")
	}
	if err := EnsureStagingSeparate("/tmp/magicpodcast-staging", "/var/lib/magicpodcast/podcastindex_feeds.db"); err != nil {
		t.Fatalf("EnsureStagingSeparate() rejected independent staging: %v", err)
	}
}

func TestEnsureStagingSeparateRejectsSymlinkIntoLiveDirectory(t *testing.T) {
	root := t.TempDir()
	liveDir := filepath.Join(root, "live")
	stageLink := filepath.Join(root, "stage-link")
	if err := os.MkdirAll(liveDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(liveDir, stageLink); err != nil {
		t.Fatal(err)
	}
	if err := EnsureStagingSeparate(stageLink, filepath.Join(liveDir, "podcastindex.db")); err == nil {
		t.Fatal("EnsureStagingSeparate() accepted a staging symlink into the live directory")
	}
}

func TestEnsureStagingSeparateKeepsMissingLivePathSuffix(t *testing.T) {
	root := t.TempDir()
	stageDir := filepath.Join(root, "staging")
	livePath := filepath.Join(root, "remote-only", "repo", "backend", "data", "podcastindex.db")
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := EnsureStagingSeparate(stageDir, livePath); err != nil {
		t.Fatalf("EnsureStagingSeparate() rejected independent staging for missing live path: %v", err)
	}
}
