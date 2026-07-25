package workflow

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func serveRobotsNotFoundWorkflow(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/robots.txt" {
		return false
	}
	w.WriteHeader(http.StatusNotFound)
	return true
}

func createWorkflowAlternativeIndex(t *testing.T, primaryURL, alternativeURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "podcastindex.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE podcasts (
  id INTEGER PRIMARY KEY,
  url TEXT NOT NULL,
  title TEXT NOT NULL,
  lastUpdate INTEGER,
  link TEXT,
  lastHttpStatus INTEGER,
  dead INTEGER,
  itunesAuthor TEXT,
  itunesId INTEGER,
  imageUrl TEXT,
  newestItemPubdate INTEGER,
  language TEXT,
  oldestItemPubdate INTEGER,
  episodeCount INTEGER,
  popularityScore INTEGER,
  priority INTEGER,
  updateFrequency INTEGER,
  newestEnclosureUrl TEXT,
  podcastGuid TEXT,
  description TEXT,
  newestEnclosureDuration INTEGER
)`)
	require.NoError(t, err)
	for id, row := range []struct {
		url    string
		status int
	}{
		{primaryURL, http.StatusForbidden},
		{alternativeURL, http.StatusOK},
	} {
		_, err = db.Exec(`INSERT INTO podcasts
 (id, url, title, lastUpdate, link, lastHttpStatus, dead, itunesAuthor, itunesId,
  imageUrl, newestItemPubdate, language, oldestItemPubdate, episodeCount,
  popularityScore, priority, updateFrequency, newestEnclosureUrl, podcastGuid,
  description, newestEnclosureDuration)
 VALUES (?, ?, '工作流稳定节目', 1, 'https://example.com', ?, 0, '作者', 321,
         'https://example.com/image.jpg', 1, 'en', 1, 1, 1, 1, 1,
         'https://example.com/episode.mp3', 'workflow-guid-321', 'description', 60)`,
			id+1, row.url, row.status)
		require.NoError(t, err)
	}
	return path
}

func TestExecutePersistsVerifiedAlternativeSource(t *testing.T) {
	var primaryRequests, alternativeRequests int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundWorkflow(w, r) {
			return
		}
		atomic.AddInt32(&primaryRequests, 1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer primary.Close()
	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundWorkflow(w, r) {
			return
		}
		atomic.AddInt32(&alternativeRequests, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, `<?xml version="1.0"?><rss version="2.0"><channel><title>工作流稳定节目</title><item><title>替代单集</title><guid>workflow-alternative-episode</guid><pubDate>Tue, 14 Jul 2026 08:00:00 GMT</pubDate></item></channel></rss>`)
	}))
	defer alternative.Close()

	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Episode{}, &models.Report{}, &models.JobFeedAttempt{}))
	indexPath := createWorkflowAlternativeIndex(t, primary.URL+"/primary.xml", alternative.URL+"/alternative.xml")
	coordinator := feed.NewCoordinator(feed.CoordinatorConfig{
		DomainPolicies: map[string]feed.DomainPolicy{feed.TargetDomain(primary.URL): {MaxConcurrency: 1}},
		LastGoodStore:  feed.NewMemorySnapshotStore(feed.LastGoodStoreConfig{}),
	})
	service, err := syncsvc.NewServiceWithFeedCoordinator(db, indexPath, coordinator)
	require.NoError(t, err)
	defer service.Close()

	podcast := &models.Podcast{
		XYZID:        "workflow-alternative",
		Title:        "工作流稳定节目",
		Author:       "作者",
		FeedURL:      primary.URL + "/primary.xml",
		ITunesID:     "321",
		PodcastGUID:  "workflow-guid-321",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)
	workflowModel := &models.Workflow{
		Name:        "替代源工作流",
		Schedule:    "0 0 * * *",
		ScopeType:   models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(podcast.ID)}},
		RulesConfig: models.RulesConfig{TimeRange: 1, TimeRangeMode: "days"},
		IsEnabled:   true,
	}
	require.NoError(t, db.Create(workflowModel).Error)

	executor := NewExecutor(db, service, nil, nil)
	executor.UseInstantBatchClock()
	job, err := executor.Execute(t.Context(), workflowModel, "manual")
	require.NoError(t, err)
	var execution models.JobExecution
	require.Eventually(t, func() bool {
		return db.Where("job_id = ?", job.ID).First(&execution).Error == nil
	}, 3*time.Second, 10*time.Millisecond)

	require.Equal(t, models.ExecutionStatusSuccess, execution.Status)
	require.Equal(t, "alternative", execution.FeedSourceType)
	require.Equal(t, alternative.URL+"/alternative.xml", execution.FeedSourceURL)
	require.Equal(t, feed.IdentityVerificationVerifiedMetadata, execution.FeedIdentityVerification)
	require.Equal(t, "live", execution.FeedFreshness)
	require.Equal(t, int32(1), atomic.LoadInt32(&primaryRequests))
	require.Equal(t, int32(1), atomic.LoadInt32(&alternativeRequests))

	attempts, err := ListFeedAttempts(db, job.ID)
	require.NoError(t, err)
	require.Len(t, attempts, 2)
	require.Equal(t, string(feed.AccessSourcePrimary), attempts[0].SourceType)
	require.Equal(t, string(feed.ErrorCategoryAccessDenied), attempts[0].ErrorCategory)
	require.False(t, attempts[0].IsFinalResult)
	require.Equal(t, string(feed.AccessSourceAlternative), attempts[1].SourceType)
	require.Equal(t, string(feed.ErrorCategoryNone), attempts[1].ErrorCategory)
	require.Equal(t, feed.IdentityVerificationVerifiedMetadata, attempts[1].IdentityVerification)
	require.True(t, attempts[1].IsFinalResult)
}
