package upgrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadRecordsSHA256AndRejectsChangingObjectMetadata(t *testing.T) {
	archiveBytes := archiveBytesForTest(t, []archiveTestEntry{validArchiveEntry(sqliteHeaderBytes())})
	probe := testDownloadDiskProbe()
	var headCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag := "\"object-a\""
		if r.Method == http.MethodHead {
			if atomic.AddInt32(&headCount, 1) > 1 {
				etag = "\"object-b\""
			}
			w.Header().Set("Content-Length", formatInt64(int64(len(archiveBytes))))
			w.Header().Set("Content-Type", "application/gzip")
			w.Header().Set("ETag", etag)
			w.Header().Set("Last-Modified", "Sat, 18 Jul 2026 10:00:00 GMT")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Length", formatInt64(int64(len(archiveBytes))))
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", "Sat, 18 Jul 2026 10:00:00 GMT")
		_, _ = w.Write(archiveBytes)
	}))
	defer server.Close()

	result, err := Download(context.Background(), DownloadOptions{
		URL:        server.URL + "/podcastindex_feeds.db.tgz",
		StagingDir: t.TempDir(),
		Probe:      probe,
		Client:     NewDirectHTTPClient(5 * time.Second),
	})
	if err == nil {
		t.Fatal("Download() succeeded after object metadata changed")
	}
	if result == nil || result.After == nil {
		t.Fatalf("result = %+v, expected before/after fingerprints", result)
	}
	if result.SHA256 == "" {
		// The archive is intentionally rejected before hashing as an installable object,
		// but the rejected file must remain available for incident inspection.
		rejectedMatches, globErr := filepath.Glob(filepath.Join(result.StagingDir, "*.rejected"))
		if globErr != nil || len(rejectedMatches) != 1 {
			t.Fatalf("rejected archive evidence missing: matches=%v err=%v", rejectedMatches, globErr)
		}
	}
}

func TestDownloadAcceptsStableObjectAndComputesSHA256(t *testing.T) {
	archiveBytes := archiveBytesForTest(t, []archiveTestEntry{validArchiveEntry(sqliteHeaderBytes())})
	probe := testDownloadDiskProbe()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", formatInt64(int64(len(archiveBytes))))
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("ETag", "\"stable\"")
		w.Header().Set("Last-Modified", "Sat, 18 Jul 2026 10:00:00 GMT")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(archiveBytes)
	}))
	defer server.Close()

	result, err := Download(context.Background(), DownloadOptions{
		URL:        server.URL + "/podcastindex_feeds.db.tgz",
		StagingDir: t.TempDir(),
		Probe:      probe,
		Client:     NewDirectHTTPClient(5 * time.Second),
	})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if result.SHA256 == "" || result.Archive == nil || result.Archive.DatabaseEntry.Name != "podcastindex_feeds.db" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(result.ArchivePath); err != nil {
		t.Fatalf("archive missing: %v", err)
	}
}

func TestValidateFingerprintRejectsMissingIdentityMetadata(t *testing.T) {
	err := ValidateFingerprint(Fingerprint{
		StatusCode:    http.StatusOK,
		ContentLength: 10,
		ContentType:   "application/gzip",
	})
	if err == nil {
		t.Fatal("ValidateFingerprint() accepted an object without ETag or Last-Modified")
	}
}

func TestNewHTTPClientRequiresExplicitSafeProxyURL(t *testing.T) {
	if _, err := NewHTTPClient(5*time.Second, "ftp://127.0.0.1:7892"); err == nil {
		t.Fatal("NewHTTPClient() accepted an unsupported proxy scheme")
	}
	if _, err := NewHTTPClient(5*time.Second, "http://user:pass@127.0.0.1:7892"); err == nil {
		t.Fatal("NewHTTPClient() accepted proxy credentials")
	}
	client, err := NewHTTPClient(5*time.Second, "http://127.0.0.1:7892")
	if err != nil || client == nil {
		t.Fatalf("NewHTTPClient() error = %v, client = %v", err, client)
	}
	endpoint, err := ProxyEndpoint("http://127.0.0.1:7892/path?secret=value")
	if err != nil || endpoint != "http://127.0.0.1:7892" {
		t.Fatalf("ProxyEndpoint() = %q, error = %v", endpoint, err)
	}
}

func TestVerifyFileIdentityRejectsChangedArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.tgz")
	if err := os.WriteFile(path, []byte("archive-v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	expectedSHA256, expectedSize, err := SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("archive-v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyFileIdentity(path, expectedSHA256, expectedSize); err == nil {
		t.Fatal("VerifyFileIdentity() accepted a changed archive")
	}
}

func archiveBytesForTest(t *testing.T, entries []archiveTestEntry) []byte {
	t.Helper()
	archivePath := makeArchive(t, entries)
	contents, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func formatInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}

func testDownloadDiskProbe() DiskProbe {
	return func(path string) (DiskStats, error) {
		giB := int64(1024 * 1024 * 1024)
		return DiskStats{
			Path:           path,
			FilesystemID:   "test-volume",
			CapacityBytes:  100 * giB,
			AvailableBytes: 80 * giB,
		}, nil
	}
}
