package handlers

import (
	"bytes"
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

func openAdminUserAgentGateStore(t *testing.T) *feed.SQLiteUserAgentGateStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:admin_user_agent_gate_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	for _, statement := range []string{
		feed.FeedUserAgentGatesCreateTableSQL,
		feed.FeedUserAgentGatesCreateIndexSQL,
		feed.FeedUserAgentGateAuditsCreateTableSQL,
		feed.FeedUserAgentGateAuditsCreateIndexSQL,
		feed.FeedUserAgentGateRecoveryFeedsCreateTableSQL,
		feed.FeedUserAgentGateRecoveryFeedsCreateIndexSQL,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	store, err := feed.NewSQLiteUserAgentGateStore(sqlDB)
	require.NoError(t, err)
	return store
}

func postUserAgentProbeApproval(t *testing.T, router http.Handler, request map[string]string) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(request)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/feed-user-agent-gates/probe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	return recorder.Code, recorder.Body.Bytes()
}

func TestAdminUserAgentGateProbeDryRunApplyAndFingerprintBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := openAdminUserAgentGateStore(t)
	userAgent := "MagicPodcast/1.0 (+https://github.com/rookiestar/MagicPodcast)"
	fingerprint := feed.UserAgentFingerprint(userAgent)
	base := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	require.NoError(t, store.Block(nil, "feeds.example", fingerprint, base))

	now := base.Add(time.Hour)
	handler := NewAdminUserAgentGateHandler(store).withClock(func() time.Time { return now })
	router := gin.New()
	router.POST("/api/v1/admin/feed-user-agent-gates/probe", handler.ApproveProbe)

	status, body := postUserAgentProbeApproval(t, router, map[string]string{
		"domain":                 "feeds.example",
		"user_agent_fingerprint": fingerprint,
		"actor":                  "owner",
		"mode":                   "dry-run",
	})
	require.Equal(t, http.StatusOK, status, string(body))
	var dryRun struct {
		Success bool                           `json:"success"`
		Data    userAgentProbeApprovalResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &dryRun))
	require.True(t, dryRun.Success)
	require.False(t, dryRun.Data.Applied)
	require.Equal(t, feed.UserAgentGateStateBlocked, dryRun.Data.State)
	require.Equal(t, fingerprint[:12], dryRun.Data.UserAgentFingerprintPrefix)
	require.NotContains(t, string(body), fingerprint)
	require.NotContains(t, string(body), userAgent)

	blocked, record, err := store.IsBlocked(nil, "feeds.example", fingerprint, now)
	require.NoError(t, err)
	require.True(t, blocked)
	require.Equal(t, feed.UserAgentGateStateBlocked, record.State, "dry-run must not mutate policy state")

	status, body = postUserAgentProbeApproval(t, router, map[string]string{
		"domain":                 "feeds.example",
		"user_agent_fingerprint": userAgent,
		"actor":                  "owner",
		"mode":                   "apply",
	})
	require.Equal(t, http.StatusBadRequest, status, string(body))

	now = base.Add(25 * time.Hour)
	status, body = postUserAgentProbeApproval(t, router, map[string]string{
		"domain":                 "feeds.example",
		"user_agent_fingerprint": fingerprint,
		"actor":                  "owner",
		"mode":                   "apply",
	})
	require.Equal(t, http.StatusOK, status, string(body))
	var applied struct {
		Success bool                           `json:"success"`
		Data    userAgentProbeApprovalResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &applied))
	require.True(t, applied.Success)
	require.True(t, applied.Data.Applied)
	require.True(t, applied.Data.Eligible)
	require.Equal(t, feed.UserAgentGateStateProbePending, applied.Data.State)
	require.Equal(t, fingerprint[:12], applied.Data.UserAgentFingerprintPrefix)
	require.NotContains(t, string(body), fingerprint)
	require.NotContains(t, string(body), userAgent)

	_, record, err = store.IsBlocked(nil, "feeds.example", fingerprint, now)
	require.NoError(t, err)
	require.Equal(t, feed.UserAgentGateStateProbePending, record.State)
	audits, err := store.ListAudits(nil, "feeds.example", fingerprint)
	require.NoError(t, err)
	require.Len(t, audits, 2, "dry-run and apply must both be auditable")
}
