package handlers

import (
	"magicpodcast/internal/feed"
	"magicpodcast/internal/middleware"

	"github.com/gin-gonic/gin"
)

// AdminFeedDiagnosticsHandler exposes the protected Feed fetch diagnostics view.
// It is intentionally read-only and surfaces only bounded, whitelisted
// aggregations: fetch counts, latency buckets, circuit state/transitions,
// conditional-GET results, last-good hits, retry (probe admission) counts, and
// last-good store capacity — never full Feed URLs, response bodies, cookies,
// credentials, or arbitrary response headers.
//
// Access control relies on loopback binding (config.Validate rejects a
// non-loopback bind address) plus Cloudflare Access in front of the API. There
// is no in-app token: a single-user service keeps the trust boundary at the
// network edge rather than introducing a credential to manage. Do NOT add a
// /metrics (Prometheus) endpoint; this is the only diagnostics surface.
type AdminFeedDiagnosticsHandler struct {
	coordinator *feed.Coordinator
	metrics     *feed.FeedMetrics
	gateStore   feed.UserAgentGateStore
}

// NewAdminFeedDiagnosticsHandler constructs the handler against the shared
// Coordinator and the process-wide metrics registry. A nil coordinator falls
// back to feed.SharedCoordinator() so the handler stays usable even when wired
// without an explicit dependency.
func NewAdminFeedDiagnosticsHandler(coordinator *feed.Coordinator) *AdminFeedDiagnosticsHandler {
	if coordinator == nil {
		coordinator = feed.SharedCoordinator()
	}
	return &AdminFeedDiagnosticsHandler{
		coordinator: coordinator,
		metrics:     feed.SharedFeedMetrics(),
	}
}

// NewAdminFeedDiagnosticsHandlerWithUserAgentGateStore wires the already
// migrated durable gate store. The constructor is intentionally explicit so
// tests and the router can prove which database-backed policy state is exposed.
func NewAdminFeedDiagnosticsHandlerWithUserAgentGateStore(coordinator *feed.Coordinator, store feed.UserAgentGateStore) *AdminFeedDiagnosticsHandler {
	h := NewAdminFeedDiagnosticsHandler(coordinator)
	h.gateStore = store
	return h
}

// withMetrics is a test seam that injects an isolated metrics registry so a
// handler test can populate known counters and assert the whitelisted output
// without touching the process-wide singleton.
func (h *AdminFeedDiagnosticsHandler) withMetrics(metrics *feed.FeedMetrics) *AdminFeedDiagnosticsHandler {
	h.metrics = metrics
	return h
}

// GetFeedDiagnostics GET /api/v1/admin/feed-diagnostics
// @Summary Feed fetch diagnostics
// @Description Protected, whitelisted Feed fetch diagnostics (counts, latency, circuits, conditional-GET, last-good). No /metrics endpoint.
// @Tags Admin
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/admin/feed-diagnostics [get]
func (h *AdminFeedDiagnosticsHandler) GetFeedDiagnostics(c *gin.Context) {
	response := feed.BuildFeedDiagnosticsWithUserAgentGate(h.coordinator, h.metrics, h.gateStore)
	middleware.SuccessResponse(c, response)
}
