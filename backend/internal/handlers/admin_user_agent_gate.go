package handlers

import (
	"context"
	"strings"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/middleware"

	"github.com/gin-gonic/gin"
)

// AdminUserAgentGateHandler exposes the audited, protected approval action for
// one recovery probe. It shares the existing loopback + Cloudflare Access
// boundary used by Feed diagnostics and never performs network I/O itself.
type AdminUserAgentGateHandler struct {
	store feed.UserAgentGateMaintenanceStore
	now   func() time.Time
}

type userAgentProbeApprovalRequest struct {
	Domain               string `json:"domain"`
	UserAgentFingerprint string `json:"user_agent_fingerprint"`
	Actor                string `json:"actor"`
	Mode                 string `json:"mode"`
}

type userAgentProbeApprovalResponse struct {
	Applied                    bool       `json:"applied"`
	Eligible                   bool       `json:"eligible"`
	Domain                     string     `json:"domain"`
	UserAgentFingerprintPrefix string     `json:"user_agent_fingerprint_prefix"`
	State                      string     `json:"state"`
	ProbeEligibleAt            time.Time  `json:"probe_eligible_at"`
	LastProbeResult            string     `json:"last_probe_result,omitempty"`
	RecoverySuccessCount       int        `json:"recovery_success_count"`
	ApprovedBy                 string     `json:"approved_by,omitempty"`
	ApprovedAt                 *time.Time `json:"approved_at,omitempty"`
	LastProbeAt                *time.Time `json:"last_probe_at,omitempty"`
}

func NewAdminUserAgentGateHandler(store feed.UserAgentGateMaintenanceStore) *AdminUserAgentGateHandler {
	return &AdminUserAgentGateHandler{store: store, now: time.Now}
}

func (h *AdminUserAgentGateHandler) withClock(now func() time.Time) *AdminUserAgentGateHandler {
	h.now = now
	return h
}

// ApproveProbe POST /api/v1/admin/feed-user-agent-gates/probe
// @Summary Approve one User-Agent recovery probe
// @Description Protected dry-run/apply action; the operation changes policy state only and never fetches a Feed.
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/admin/feed-user-agent-gates/probe [post]
func (h *AdminUserAgentGateHandler) ApproveProbe(c *gin.Context) {
	if h == nil || h.store == nil {
		middleware.ServiceUnavailableResponse(c, "USER_AGENT_GATE_UNAVAILABLE", "User-Agent恢复策略不可用")
		return
	}
	var request userAgentProbeApprovalRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		middleware.BadRequestResponse(c, "INVALID_PARAM", "请求参数格式错误")
		return
	}
	mode := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(request.Mode), "-", "_"))
	var apply bool
	switch mode {
	case feed.UserAgentGateAuditModeDryRun:
		apply = false
	case feed.UserAgentGateAuditModeApply:
		apply = true
	default:
		middleware.BadRequestResponse(c, "INVALID_MODE", "mode 必须为 dry_run 或 apply")
		return
	}
	fingerprint, err := feed.NormalizeUserAgentFingerprint(request.UserAgentFingerprint)
	if err != nil {
		middleware.BadRequestResponse(c, "INVALID_FINGERPRINT", "必须提供 User-Agent 的 SHA-256 指纹")
		return
	}
	actor := strings.TrimSpace(request.Actor)
	if actor == "" || len(actor) > 128 {
		middleware.BadRequestResponse(c, "INVALID_ACTOR", "必须提供有效批准人")
		return
	}
	now := time.Now().UTC()
	if h.now != nil {
		now = h.now().UTC()
	}
	requestContext := context.Background()
	if c.Request != nil {
		requestContext = c.Request.Context()
	}
	approval, err := h.store.ApproveProbe(requestContext, strings.TrimSpace(request.Domain), fingerprint, actor, now, apply)
	if err != nil {
		middleware.BadRequestResponse(c, "PROBE_APPROVAL_FAILED", "恢复探测审批未完成")
		return
	}
	middleware.SuccessResponse(c, userAgentProbeApprovalResponseFromApproval(approval))
}

func userAgentProbeApprovalResponseFromApproval(approval feed.UserAgentGateApproval) userAgentProbeApprovalResponse {
	return userAgentProbeApprovalResponse{
		Applied:                    approval.Applied,
		Eligible:                   approval.Eligible,
		Domain:                     approval.Record.Domain,
		UserAgentFingerprintPrefix: userAgentFingerprintPrefix(approval.Record.UserAgentFingerprint),
		State:                      approval.Record.State,
		ProbeEligibleAt:            approval.Record.ProbeEligibleAt,
		LastProbeResult:            approval.Record.LastProbeResult,
		RecoverySuccessCount:       approval.Record.RecoverySuccessCount,
		ApprovedBy:                 approval.Record.ApprovedBy,
		ApprovedAt:                 approval.Record.ApprovedAt,
		LastProbeAt:                approval.Record.LastProbeAt,
	}
}

func userAgentFingerprintPrefix(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
