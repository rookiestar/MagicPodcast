package upgrade

import (
	"fmt"
	"path/filepath"
	"syscall"
)

type DiskProbe func(path string) (DiskStats, error)

func DefaultDiskProbe(path string) (DiskStats, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return DiskStats{}, fmt.Errorf("resolve disk path: %w", err)
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(absPath, &stat); err != nil {
		return DiskStats{}, fmt.Errorf("stat filesystem %s: %w", absPath, err)
	}

	return DiskStats{
		Path:           absPath,
		FilesystemID:   fmt.Sprintf("%v", stat.Fsid),
		CapacityBytes:  int64(stat.Blocks) * int64(stat.Bsize),
		AvailableBytes: int64(stat.Bavail) * int64(stat.Bsize),
	}, nil
}

func SafetyReserveBytes(capacityBytes int64) int64 {
	percentReserve := capacityBytes * 15 / 100
	if percentReserve > DefaultReserveFloorBytes {
		return percentReserve
	}
	return DefaultReserveFloorBytes
}

func EvaluateDiskGate(probe DiskProbe, path string, archiveBytes, extractedBytes int64) (DiskReport, error) {
	if probe == nil {
		probe = DefaultDiskProbe
	}
	if archiveBytes < 0 || extractedBytes < 0 {
		return DiskReport{}, fmt.Errorf("archive and extracted sizes must not be negative")
	}

	stats, err := probe(path)
	if err != nil {
		return DiskReport{Path: path, Reason: err.Error()}, err
	}
	reserve := SafetyReserveBytes(stats.CapacityBytes)
	required := archiveBytes + extractedBytes + reserve
	report := DiskReport{
		Path:               stats.Path,
		FilesystemID:       stats.FilesystemID,
		CapacityBytes:      stats.CapacityBytes,
		AvailableBytes:     stats.AvailableBytes,
		ArchiveBytes:       archiveBytes,
		ExtractedBytes:     extractedBytes,
		SafetyReserveBytes: reserve,
		RequiredBytes:      required,
		Passed:             stats.AvailableBytes >= required,
	}
	if !report.Passed {
		report.Reason = fmt.Sprintf("insufficient disk space: available=%d required=%d", stats.AvailableBytes, required)
		return report, fmt.Errorf("%s", report.Reason)
	}
	return report, nil
}

func EnsureSameFilesystem(probe DiskProbe, first, second string) error {
	if probe == nil {
		probe = DefaultDiskProbe
	}
	firstStats, err := probe(first)
	if err != nil {
		return fmt.Errorf("stat first filesystem: %w", err)
	}
	secondStats, err := probe(second)
	if err != nil {
		return fmt.Errorf("stat second filesystem: %w", err)
	}
	if firstStats.FilesystemID == "" || secondStats.FilesystemID == "" {
		return fmt.Errorf("filesystem identity is missing")
	}
	if firstStats.FilesystemID != secondStats.FilesystemID {
		return fmt.Errorf("paths are on different filesystems: %s vs %s", first, second)
	}
	return nil
}
