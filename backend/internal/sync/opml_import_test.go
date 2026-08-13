package sync

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestOPML(t *testing.T, xmlURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "feeds.opml")
	content := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0"><head><title>t</title></head><body>
  <outline text="Show" title="Show" type="rss" xmlUrl="` + xmlURL + `"/>
</body></opml>`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func newImportTestServer(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		w.WriteHeader(status)
		if len(body) > 0 {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestImportOPMLFromPodcastIndexOnlyReportsSkipOnce(t *testing.T) {
	server := newImportTestServer(t, http.StatusNotFound, nil)
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	reporter := &recordingReporter{}
	result, err := service.ImportOPMLFromPodcastIndexOnly(writeTestOPML(t, server.URL+"/feed.xml"), reporter)
	require.NoError(t, err)
	assert.Equal(t, 0, result.SuccessPodcasts)

	skips, successes, summaries := reporter.snapshot()
	assert.Len(t, skips, 1, "skip must be reported once, not by both worker and aggregator")
	assert.NotContains(t, successes, "成功导入: Show")
	require.Len(t, summaries, 1)
	assert.Equal(t, "import", summaries[0].Operation)
	assert.Equal(t, 1, summaries[0].TotalPodcasts)
	assert.Equal(t, 0, summaries[0].SuccessPodcasts)
	assert.Equal(t, 1, summaries[0].SkippedPodcasts)
	assert.Equal(t, 0, summaries[0].StubPodcasts)
}

func TestImportOPMLFromPodcastIndexOnlyDoesNotCountStubAsSuccess(t *testing.T) {
	server := newImportTestServer(t, http.StatusInternalServerError, []byte("temporary"))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	reporter := &recordingReporter{}
	result, err := service.ImportOPMLFromPodcastIndexOnly(writeTestOPML(t, server.URL+"/feed.xml"), reporter)
	require.NoError(t, err)
	assert.Equal(t, 0, result.SuccessPodcasts)
	assert.Equal(t, 0, result.FailedPodcasts)

	skips, successes, summaries := reporter.snapshot()
	assert.NotContains(t, successes, "成功导入: Show")
	assert.Len(t, skips, 1)
	assert.Contains(t, skips[0], "待同步")
	require.Len(t, summaries, 1)
	assert.Equal(t, 0, summaries[0].SuccessPodcasts)
	assert.Equal(t, 1, summaries[0].StubPodcasts)

	var count int64
	require.NoError(t, db.Model(&models.Podcast{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestImportOPMLFromPodcastIndexOnlyEmptyFileSendsSummary(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	path := filepath.Join(t.TempDir(), "empty.opml")
	require.NoError(t, os.WriteFile(path, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0"><head><title>t</title></head><body></body></opml>`), 0o600))

	reporter := &recordingReporter{}
	result, err := service.ImportOPMLFromPodcastIndexOnly(path, reporter)
	require.NoError(t, err)
	assert.Equal(t, 0, result.SuccessPodcasts)

	_, _, summaries := reporter.snapshot()
	require.Len(t, summaries, 1)
	assert.Equal(t, "import", summaries[0].Operation)
	assert.Equal(t, 0, summaries[0].TotalPodcasts)
}
