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

func TestExecuteStopsSameDomainUserAgentACLRequestsAndKeepsVerifiedAlternative(t *testing.T) {
	var primaryA, primaryB, alternativeHits int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundWorkflow(w, r) {
			return
		}
		switch r.URL.Path {
		case "/a.xml":
			atomic.AddInt32(&primaryA, 1)
			w.Header().Set("X-Tengine-Error", "denied by UA ACL = blacklist")
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, "acl-secret-response-body")
		case "/b.xml":
			atomic.AddInt32(&primaryB, 1)
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(primary.Close)

	alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundWorkflow(w, r) {
			return
		}
		atomic.AddInt32(&alternativeHits, 1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = fmt.Fprint(w, batchRSS("节目 A", "alternative-a-episode"))
	}))
	t.Cleanup(alternative.Close)

	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.Episode{}, &models.Report{}, &models.JobFeedAttempt{}, &models.PodcastAlternativeFeed{}))
	coordinator := feed.NewCoordinator(feed.CoordinatorConfig{
		DomainPolicies: map[string]feed.DomainPolicy{
			feed.TargetDomain(primary.URL): {MaxConcurrency: 1},
		},
	})
	service, err := syncsvc.NewServiceWithFeedCoordinator(db, "", coordinator)
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Close() })

	first := &models.Podcast{
		XYZID:        "ua-acl-a",
		Title:        "节目 A",
		FeedURL:      primary.URL + "/a.xml",
		ITunesID:     "321",
		PodcastGUID:  "ua-acl-guid-a",
		IsSubscribed: true,
	}
	second := &models.Podcast{
		XYZID:        "ua-acl-b",
		Title:        "节目 B",
		FeedURL:      primary.URL + "/b.xml",
		ITunesID:     "654",
		PodcastGUID:  "ua-acl-guid-b",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(first).Error)
	require.NoError(t, db.Create(second).Error)
	require.NoError(t, db.Create(&models.PodcastAlternativeFeed{
		PodcastID:          first.ID,
		MainFeedURL:        feed.CanonicalizeURL(first.FeedURL),
		IdentityKey:        "321|ua-acl-guid-a",
		AlternativeFeedURL: alternative.URL + "/a.xml",
		Status:             models.AlternativeCacheVerified,
		Verification:       feed.IdentityVerificationVerifiedMetadata,
		VerifiedAt:         time.Now(),
	}).Error)

	workflowModel := &models.Workflow{
		Name:        "UA ACL workflow",
		ScopeType:   models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(first.ID), int(second.ID)}},
		RulesConfig: models.RulesConfig{TimeRange: 0},
		IsEnabled:   true,
	}
	require.NoError(t, db.Create(workflowModel).Error)

	executor := NewExecutor(db, service, nil, nil)
	executor.UseInstantBatchClock()
	executor.workerConcurrency = 1
	job, err := executor.Execute(context.Background(), workflowModel, "manual")
	require.NoError(t, err)
	require.Equal(t, models.JobStatusPartial, job.Status)
	require.Equal(t, int32(1), atomic.LoadInt32(&primaryA), "direct UA ACL denial should be one upstream request")
	require.Zero(t, atomic.LoadInt32(&primaryB), "same-domain sibling must be policy-blocked before HTTP")
	require.Equal(t, int32(1), atomic.LoadInt32(&alternativeHits), "verified alternative must remain available")

	var executions []models.JobExecution
	require.NoError(t, db.Where("job_id = ?", job.ID).Order("podcast_id ASC").Find(&executions).Error)
	require.Len(t, executions, 2)
	require.Equal(t, string(feed.AccessSourceAlternative), executions[0].FeedSourceType)
	require.Equal(t, string(feed.ErrorCategory("user_agent_blocked")), executions[1].FeedErrorCategory)
	require.NotContains(t, executions[1].ErrorMessage, "denied by UA ACL")
	require.NotContains(t, executions[1].ErrorMessage, "acl-secret-response-body")

	attempts, err := ListFeedAttempts(db, job.ID)
	require.NoError(t, err)
	var direct, derived, alternativeAttempt *models.JobFeedAttempt
	for i := range attempts {
		attempt := attempts[i]
		switch attempt.ErrorCategory {
		case string(feed.ErrorCategory("user_agent_denied")):
			direct = &attempt
		case string(feed.ErrorCategory("user_agent_blocked")):
			derived = &attempt
		case string(feed.ErrorCategoryNone):
			if attempt.SourceType == string(feed.AccessSourceAlternative) {
				alternativeAttempt = &attempt
			}
		}
	}
	require.NotNil(t, direct)
	require.False(t, direct.IsFinalResult)
	require.Equal(t, "user_agent_denied_no_retry", direct.RetryDecision)
	require.NotNil(t, derived)
	require.True(t, derived.IsFinalResult)
	require.True(t, derived.DerivedPolicy)
	require.Equal(t, "user_agent_blocked_no_retry", derived.RetryDecision)
	require.NotNil(t, alternativeAttempt)
	require.True(t, alternativeAttempt.IsFinalResult)

	summary := BuildRootCauseSummary(attempts)
	require.Equal(t, 1, summary.UpstreamRootCauses[string(feed.ErrorCategory("user_agent_denied"))])
	require.Equal(t, 1, summary.DerivedPolicyActions[string(feed.ErrorCategory("user_agent_blocked"))])
}
