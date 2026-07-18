package upgrade

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DownloadOptions struct {
	URL                    string
	StagingDir             string
	LiveDBPath             string
	ArchiveName            string
	ExpectedExtractedBytes int64
	Probe                  DiskProbe
	Client                 *http.Client
	Now                    func() time.Time
}

func Download(ctx context.Context, options DownloadOptions) (*DownloadResult, error) {
	if options.URL == "" {
		options.URL = DefaultDownloadURL
	}
	if options.StagingDir == "" {
		return nil, fmt.Errorf("staging directory is required")
	}
	if options.ArchiveName == "" {
		options.ArchiveName = "podcastindex_feeds.db.tgz"
	}
	if filepath.Base(options.ArchiveName) != options.ArchiveName {
		return nil, fmt.Errorf("archive name must be a plain file name")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Client == nil {
		options.Client = NewDirectHTTPClient(30 * time.Second)
	}
	if err := EnsureStagingSeparate(options.StagingDir, options.LiveDBPath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(options.StagingDir, 0o700); err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}

	result := &DownloadResult{StagingDir: options.StagingDir}
	before, err := FingerprintURL(ctx, options.Client, options.URL, options.Now)
	result.Before = before
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	if options.Probe == nil {
		options.Probe = DefaultDiskProbe
	}
	initialDisk, err := EvaluateDiskGate(options.Probe, options.StagingDir, before.ContentLength, options.ExpectedExtractedBytes)
	result.Disk = initialDisk
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	archivePath := filepath.Join(options.StagingDir, options.ArchiveName)
	if _, err := os.Lstat(archivePath); err == nil {
		return result, fmt.Errorf("refusing to overwrite existing archive %s", archivePath)
	} else if !os.IsNotExist(err) {
		return result, fmt.Errorf("inspect archive destination: %w", err)
	}

	partial, err := os.CreateTemp(options.StagingDir, ".podcastindex-*.partial")
	if err != nil {
		return result, fmt.Errorf("create partial archive: %w", err)
	}
	partialPath := partial.Name()
	result.PartialPath = partialPath
	defer partial.Close()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, options.URL, nil)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("create download request: %w", err)
	}
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", validatorUserAgent)
	response, err := options.Client.Do(request)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("GET %s: %w", options.URL, err)
	}
	if response.Body != nil {
		defer response.Body.Close()
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		result.Error = fmt.Sprintf("GET returned HTTP %d", response.StatusCode)
		return result, fmt.Errorf("%s", result.Error)
	}

	bytesCopied, err := io.Copy(partial, response.Body)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("download archive: %w", err)
	}
	if before.ContentLength > 0 && bytesCopied != before.ContentLength {
		result.Error = fmt.Sprintf("download size mismatch: got=%d expected=%d", bytesCopied, before.ContentLength)
		return result, fmt.Errorf("%s", result.Error)
	}
	if err := partial.Sync(); err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("sync partial archive: %w", err)
	}
	if err := partial.Close(); err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("close partial archive: %w", err)
	}
	if err := os.Rename(partialPath, archivePath); err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("promote complete archive: %w", err)
	}
	result.ArchivePath = archivePath

	after, err := FingerprintURL(ctx, options.Client, options.URL, options.Now)
	result.After = &after
	if err != nil {
		rejectedPath := rejectArchive(archivePath, "fingerprint-failed.rejected")
		if rejectedPath != "" {
			result.ArchivePath = rejectedPath
		}
		result.Error = err.Error()
		return result, err
	}
	if err := CompareFingerprints(before, after); err != nil {
		if rejectedPath := rejectArchive(archivePath, "metadata-changed.rejected"); rejectedPath != "" {
			result.ArchivePath = rejectedPath
		}
		result.Error = err.Error()
		return result, err
	}

	sha256, sizeBytes, err := SHA256File(archivePath)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	result.SHA256 = sha256
	result.SizeBytes = sizeBytes

	inspection, err := ValidateArchive(archivePath)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	result.Archive = &inspection
	finalDisk, err := EvaluateDiskGate(options.Probe, options.StagingDir, sizeBytes, inspection.ExtractedBytes)
	result.Disk = finalDisk
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	return result, nil
}

func ArchivePathFromDownload(result *DownloadResult) string {
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.ArchivePath)
}

func rejectArchive(path, suffix string) string {
	rejectedPath := path + "." + suffix
	if err := os.Rename(path, rejectedPath); err != nil {
		return ""
	}
	return rejectedPath
}
