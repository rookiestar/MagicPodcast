package workflow

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"

	"github.com/stretchr/testify/require"
)

func TestExecutePersistsUserAgentBlockAcrossJobsAndUsesVerifiedAlternative(t *testing.T) {
	var primaryHits, alternativeHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundWorkflow(w, r) {
			return
		}
		atomic.AddInt32(&primaryHits, 1)
		w.Header().Set("X-Tengine-Error", "denied by UA ACL = blacklist")
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, "persistent-acl-response-body")
	}))
	t.Cleanup(primary.Close)

	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundWorkflow(w, r) {
			return
		}
		atomic.AddInt32(&alternativeHits, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, batchRSS("持久化节目", "persistent-alternative-episode"))
	}))
	t.Cleanup(alternative.Close)

	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Episode{}, &models.Report{}, &models.JobFeedAttempt{}, &models.PodcastAlternativeFeed{}))
	require.NoError(t, db.Exec(feed.FeedUserAgentGatesCreateTableSQL).Error)
	require.NoError(t, db.Exec(feed.FeedUserAgentGatesCreateIndexSQL).Error)
	require.NoError(t, db.Exec(feed.FeedUserAgentGateRecoveryFeedsCreateTableSQL).Error)
	require.NoError(t, db.Exec(feed.FeedUserAgentGateRecoveryFeedsCreateIndexSQL).Error)

	newService := func() *syncsvc.Service {
		coordinator := feed.NewCoordinator(feed.CoordinatorConfig{
			DomainPolicies: map[string]feed.DomainPolicy{
				feed.TargetDomain(primary.URL): {MaxConcurrency: 1},
			},
		})
		service, err := syncsvc.NewServiceWithFeedCoordinator(db, "", coordinator)
		require.NoError(t, err)
		return service
	}

	podcast := &models.Podcast{
		XYZID:        "persistent-ua-gate",
		Title:        "持久化节目",
		FeedURL:      primary.URL + "/primary.xml",
		ITunesID:     "321",
		PodcastGUID:  "persistent-guid",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)
	require.NoError(t, db.Create(&models.PodcastAlternativeFeed{
		PodcastID:          podcast.ID,
		MainFeedURL:        feed.CanonicalizeURL(podcast.FeedURL),
		IdentityKey:        "321|persistent-guid",
		AlternativeFeedURL: alternative.URL + "/alternative.xml",
		Status:             models.AlternativeCacheVerified,
		Verification:       feed.IdentityVerificationVerifiedMetadata,
		VerifiedAt:         time.Now(),
	}).Error)

	workflowModel := &models.Workflow{
		Name:        "持久化 UA 阻断工作流",
		ScopeType:   models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(podcast.ID)}},
		RulesConfig: models.RulesConfig{TimeRange: 0},
		IsEnabled:   true,
	}
	require.NoError(t, db.Create(workflowModel).Error)

	service := newService()
	executor := NewExecutor(db, service, nil, nil)
	executor.UseInstantBatchClock()
	firstJob, err := executor.Execute(context.Background(), workflowModel, "manual")
	require.NoError(t, err)
	require.Equal(t, models.JobStatusCompleted, firstJob.Status)
	require.NoError(t, service.Close())

	// Rebuilding both the coordinator and sync service models a restart. The
	// second Job must not probe the blocked primary again, but may still use the
	// identity-bound verified alternative for the current batch.
	service = newService()
	t.Cleanup(func() { _ = service.Close() })
	executor = NewExecutor(db, service, nil, nil)
	executor.UseInstantBatchClock()
	secondJob, err := executor.Execute(context.Background(), workflowModel, "manual")
	require.NoError(t, err)
	require.Equal(t, models.JobStatusCompleted, secondJob.Status)

	require.Equal(t, int32(1), atomic.LoadInt32(&primaryHits), "the persisted gate must suppress the second Job's primary request")
	require.Equal(t, int32(2), atomic.LoadInt32(&alternativeHits), "each Job may use the verified alternative")

	var execution models.JobExecution
	require.NoError(t, db.Where("job_id = ?", secondJob.ID).First(&execution).Error)
	require.Equal(t, string(feed.AccessSourceAlternative), execution.FeedSourceType)
	require.Equal(t, string(feed.ErrorCategoryNone), execution.FeedErrorCategory)
	require.Equal(t, podcast.FeedURL, func() string {
		var current models.Podcast
		require.NoError(t, db.First(&current, podcast.ID).Error)
		return current.FeedURL
	}(), "alternative success must not replace the configured primary Feed")
}
