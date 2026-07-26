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

type userAgentIdentityMigrationRequest struct {
	Domain                  string `json:"domain"`
	OldUserAgentFingerprint string `json:"old_user_agent_fingerprint"`
	NewUserAgentFingerprint string `json:"new_user_agent_fingerprint"`
	Actor                   string `json:"actor"`
	Mode                    string `json:"mode"`
}

// userAgentIdentityMigrationResponse exposes only the two fingerprint prefixes
// and the resulting target states. Neither the raw User-Agent nor the full
// fingerprint ever leaves this seam.
type userAgentIdentityMigrationResponse struct {
	Applied                       bool       `json:"applied"`
	Eligible                      bool       `json:"eligible"`
	Domain                        string     `json:"domain"`
	OldUserAgentFingerprintPrefix string     `json:"old_user_agent_fingerprint_prefix"`
	OldState                      string     `json:"old_state"`
	NewUserAgentFingerprintPrefix string     `json:"new_user_agent_fingerprint_prefix"`
	NewState                      string     `json:"new_state"`
	NewProbeEligibleAt            *time.Time `json:"new_probe_eligible_at,omitempty"`
	NewApprovedBy                 string     `json:"new_approved_by,omitempty"`
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

// MigrateIdentity POST /api/v1/admin/feed-user-agent-gates/migrate
// @Summary Migrate a rejected User-Agent identity to a new approved fingerprint
// @Description Protected dry-run/apply action that retires the old fingerprint and admits exactly one probe for the new one. Policy-only; never fetches a Feed.
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/admin/feed-user-agent-gates/migrate [post]
func (h *AdminUserAgentGateHandler) MigrateIdentity(c *gin.Context) {
	if h == nil || h.store == nil {
		middleware.ServiceUnavailableResponse(c, "USER_AGENT_GATE_UNAVAILABLE", "User-Agent恢复策略不可用")
		return
	}
	var request userAgentIdentityMigrationRequest
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
	oldFingerprint, err := feed.NormalizeUserAgentFingerprint(request.OldUserAgentFingerprint)
	if err != nil {
		middleware.BadRequestResponse(c, "INVALID_FINGERPRINT", "必须提供旧 User-Agent 的 SHA-256 指纹")
		return
	}
	newFingerprint, err := feed.NormalizeUserAgentFingerprint(request.NewUserAgentFingerprint)
	if err != nil {
		middleware.BadRequestResponse(c, "INVALID_FINGERPRINT", "必须提供新 User-Agent 的 SHA-256 指纹")
		return
	}
	if oldFingerprint == newFingerprint {
		middleware.BadRequestResponse(c, "INVALID_FINGERPRINT", "新旧 User-Agent 指纹必须不同")
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
	migration, err := h.store.MigrateIdentity(requestContext, strings.TrimSpace(request.Domain), oldFingerprint, newFingerprint, actor, now, apply)
	if err != nil {
		middleware.BadRequestResponse(c, "IDENTITY_MIGRATION_FAILED", "身份迁移未完成")
		return
	}
	middleware.SuccessResponse(c, userAgentIdentityMigrationResponseFromMigration(migration))
}

func userAgentIdentityMigrationResponseFromMigration(migration feed.UserAgentGateIdentityMigration) userAgentIdentityMigrationResponse {
	response := userAgentIdentityMigrationResponse{
		Applied:                       migration.Applied,
		Eligible:                      migration.Eligible,
		Domain:                        migration.Old.Domain,
		OldUserAgentFingerprintPrefix: userAgentFingerprintPrefix(migration.Old.UserAgentFingerprint),
		OldState:                      migration.Old.State,
		NewUserAgentFingerprintPrefix: userAgentFingerprintPrefix(migration.New.UserAgentFingerprint),
		NewState:                      migration.New.State,
		NewApprovedBy:                 migration.New.ApprovedBy,
	}
	if migration.Applied {
		response.OldState = feed.UserAgentGateStateRetired
	}
	// In dry-run the new identity does not exist yet; project its target state so
	// the operator sees what apply would admit.
	if response.NewState == "" {
		response.NewState = feed.UserAgentGateStateProbePending
	}
	if migration.New.ProbeEligibleAt.IsZero() {
		response.NewProbeEligibleAt = nil
	} else {
		eligible := migration.New.ProbeEligibleAt
		response.NewProbeEligibleAt = &eligible
	}
	return response
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
