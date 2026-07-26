package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"magicpodcast/internal/feed"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestAdminFeedDiagnosticsReturnsWhitelistedView verifies the protected admin
// entry returns 200 with the {"success":true,"data":...} shape, surfaces every
// whitelisted counter field, and never leaks a full Feed URL, response body,
// cookie, credential, podcast id, or arbitrary response header.
func TestAdminFeedDiagnosticsReturnsWhitelistedView(t *testing.T) {
	gin.SetMode(gin.TestMode)

	metrics := feed.NewFeedMetrics()
	status200 := http.StatusOK
	status403 := http.StatusForbidden
	metrics.RecordFetch(feed.AccessOutcome{
		HTTPStatus:    &status200,
		ErrorCategory: feed.ErrorCategoryNone,
		TargetDomain:  "feed.xyzfm.space",
		SourceType:    feed.AccessSourcePrimary,
	})
	metrics.RecordFetch(feed.AccessOutcome{
		HTTPStatus:    &status403,
		ErrorCategory: feed.ErrorCategoryAccessDenied,
		TargetDomain:  "feed.xyzfm.space",
		SourceType:    feed.AccessSourcePrimary,
	})
	metrics.RecordConditionalGet("304")
	metrics.RecordConditionalGet("miss")
	metrics.RecordLastGoodHit()
	metrics.RecordRetry("feed.xyzfm.space")
	metrics.RecordCircuitTransition("feed.xyzfm.space", feed.CircuitStateClosed, feed.CircuitStateOpen)
	metrics.SetConfiguredEgressLabel("experiment-xyzfm-egress")

	coord := feed.NewCoordinator(feed.CoordinatorConfig{})
	coord.SetMetrics(metrics)
	handler := NewAdminFeedDiagnosticsHandler(coord).withMetrics(metrics)

	router := gin.New()
	router.GET("/api/v1/admin/feed-diagnostics", handler.GetFeedDiagnostics)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/feed-diagnostics", nil))

	require.Equal(t, http.StatusOK, recorder.Code, "body=%s", recorder.Body.String())

	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.True(t, envelope.Success, "admin entry must use the success envelope")
	require.NotEmpty(t, envelope.Data)

	var view feed.FeedDiagnosticsResponse
	require.NoError(t, json.Unmarshal(envelope.Data, &view))
	require.NotEmpty(t, view.FeedFetchTotal)
	require.NotEmpty(t, view.CircuitTransitions)
	require.NotEmpty(t, view.ConditionalGetTotal)
	require.Equal(t, int64(1), view.LastGoodHitsTotal)
	require.Equal(t, "experiment-xyzfm-egress", view.ConfiguredEgressLabel)
	require.NotNil(t, view.SnapshotStore, "snapshot_store capacity stats must be present")

	body := recorder.Body.String()
	for _, key := range []string{
		`"feed_fetch_total"`, `"feed_fetch_duration_seconds"`, `"circuit_state"`,
		`"circuit_transitions_total"`, `"last_good_hits_total"`, `"conditional_get_total"`,
		`"retry_total"`, `"configured_egress_label"`,
	} {
		require.Contains(t, body, key, "whitelisted field %s must be present", key)
	}

	// No leakage: the bounded view never carries a full URL, raw body, cookie,
	// credential, podcast id, or arbitrary response header.
	for _, forbidden := range []string{"https://", "/feed.xml", "raw_content", "cookie", "authorization", "token", "podcast_id", "set_cookie"} {
		require.NotContains(t, body, forbidden, "forbidden value %s must not appear in diagnostics", forbidden)
	}
}

// TestAdminFeedDiagnosticsNoMetricsRoute asserts the diagnostics surface is the
// admin JSON entry only: the response carries feed_fetch_total counts, never a
// Prometheus-style exposition, and the route name is distinct from /metrics.
func TestAdminFeedDiagnosticsNoMetricsRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAdminFeedDiagnosticsHandler(nil)
	router := gin.New()
	router.GET("/api/v1/admin/feed-diagnostics", handler.GetFeedDiagnostics)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/feed-diagnostics", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	// A Prometheus exposition starts with "# HELP" / "# TYPE"; the JSON envelope
	// must not resemble it.
	require.NotContains(t, recorder.Body.String(), "# HELP")
	require.NotContains(t, recorder.Body.String(), "# TYPE")
	require.Contains(t, recorder.Body.String(), `"success":true`)
}

func TestAdminFeedDiagnosticsIncludesPersistentUserAgentGates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:user_agent_diag_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(feed.FeedUserAgentGatesCreateTableSQL).Error)
	require.NoError(t, db.Exec(feed.FeedUserAgentGatesCreateIndexSQL).Error)
	require.NoError(t, db.Exec(feed.FeedUserAgentGateAuditsCreateTableSQL).Error)
	require.NoError(t, db.Exec(feed.FeedUserAgentGateAuditsCreateIndexSQL).Error)
	require.NoError(t, db.Exec(feed.FeedUserAgentGateRecoveryFeedsCreateTableSQL).Error)
	require.NoError(t, db.Exec(feed.FeedUserAgentGateRecoveryFeedsCreateIndexSQL).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	store, err := feed.NewSQLiteUserAgentGateStore(sqlDB)
	require.NoError(t, err)
	userAgent := "MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)"
	fingerprint := feed.UserAgentFingerprint(userAgent)
	require.NoError(t, store.Block(nil, "feeds.example", fingerprint, time.Now().Add(-time.Hour)))
	approvedAt := time.Now().UTC().Add(25 * time.Hour)
	approval, err := store.ApproveProbe(nil, "feeds.example", fingerprint, "owner", approvedAt, true)
	require.NoError(t, err)
	require.True(t, approval.Applied)
	_, err = store.PreparePrimaryFetchForFeed(nil, "feeds.example", fingerprint, feed.UserAgentGateFeedFingerprint("https://feeds.example/probe.xml"), approvedAt)
	require.NoError(t, err)

	handler := NewAdminFeedDiagnosticsHandlerWithUserAgentGateStore(nil, store)
	router := gin.New()
	router.GET("/api/v1/admin/feed-diagnostics", handler.GetFeedDiagnostics)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/feed-diagnostics", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	var envelope struct {
		Success bool                         `json:"success"`
		Data    feed.FeedDiagnosticsResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	require.True(t, envelope.Success)
	require.Len(t, envelope.Data.UserAgentGates, 1)
	gate := envelope.Data.UserAgentGates[0]
	require.Equal(t, "feeds.example", gate.Domain)
	require.Equal(t, fingerprint[:12], gate.UserAgentFingerprintPrefix)
	require.Equal(t, feed.UserAgentGateStateProbeInFlight, gate.State)
	require.False(t, gate.ProbeEligible)
	require.NotZero(t, gate.DetectedAt)
	require.NotZero(t, gate.ProbeEligibleAt)
	require.Equal(t, "owner", gate.ApprovedBy)
	require.NotNil(t, gate.ApprovedAt)
	require.NotNil(t, gate.LastProbeAt)

	body := recorder.Body.String()
	require.Contains(t, body, fingerprint[:12])
	require.NotContains(t, body, fingerprint)
	require.NotContains(t, body, userAgent)
}
