package feed

import (
	"net/url"
	"strings"
	"time"
)

// ErrorCategory is the stable, user-visible classification of a Feed access.
type ErrorCategory string

const (
	ErrorCategoryNotObserved        ErrorCategory = "not_observed"
	ErrorCategoryNone               ErrorCategory = "none"
	ErrorCategoryAccessDenied       ErrorCategory = "access_denied"
	ErrorCategoryRateLimited        ErrorCategory = "rate_limited"
	ErrorCategoryServiceUnavailable ErrorCategory = "service_unavailable"
	ErrorCategoryHTTP               ErrorCategory = "http_error"
	ErrorCategoryTimeout            ErrorCategory = "timeout"
	ErrorCategoryCancelled          ErrorCategory = "cancelled"
	ErrorCategoryNetwork            ErrorCategory = "network_error"
	ErrorCategoryParse              ErrorCategory = "parse_error"
	ErrorCategoryInvalidRequest     ErrorCategory = "invalid_request"
	// ErrorCategoryPolicyRejected marks a Feed path blocked by an advisory
	// robots.txt rule (a client-side admission decision) BEFORE any upstream
	// request left the process. It is distinct from ErrorCategoryAccessDenied
	// (an upstream HTTP 403/401) so logs, execution history, and metrics can
	// tell a local policy skip apart from an upstream refusal — and so it never
	// trips the Xiaoyuzhou first-403-immediate-open circuit invariant.
	ErrorCategoryPolicyRejected ErrorCategory = "policy_rejected"
	ErrorCategoryCircuitOpen    ErrorCategory = "circuit_open"
	// ErrorCategoryUnattempted is a synthetic batch outcome: the Feed never
	// reached a network attempt before the hard batch deadline.
	ErrorCategoryUnattempted ErrorCategory = "unattempted"
	ErrorCategoryUnknown     ErrorCategory = "unknown"
)

type CircuitState string

const (
	CircuitStateNotUsed CircuitState = "not_used"
	CircuitStateOpen    CircuitState = "open"
	CircuitStateProbe   CircuitState = "probe"
	CircuitStateClosed  CircuitState = "closed"
)

// AccessSource identifies where the content used by a workflow came from.
// Later resilience tickets extend these values without changing the execution
// history contract introduced here.
type AccessSource string

const (
	AccessSourceUnknown     AccessSource = "unknown"
	AccessSourcePrimary     AccessSource = "primary"
	AccessSourceAlternative AccessSource = "alternative"
	AccessSourceSharedCache AccessSource = "shared_cache"
	AccessSourceLocalCache  AccessSource = "local_cache"
	AccessSourceLastGood    AccessSource = "last_good"
)

// IdentityVerification is the bounded explanation for whether a
// PodcastIndex alternative was safe to use. It is persisted with the
// execution outcome so an unavailable or ambiguous candidate is visible
// without reading server logs.
const (
	IdentityVerificationNotChecked                   = "not_checked"
	IdentityVerificationVerifiedMetadata             = "verified_stable_identity_and_metadata"
	IdentityVerificationVerifiedEpisode              = "verified_stable_identity_and_episode"
	IdentityVerificationRejectedNoStableID           = "rejected_no_stable_identity"
	IdentityVerificationRejectedAmbiguous            = "rejected_ambiguous"
	IdentityVerificationRejectedConflict             = "rejected_identity_conflict"
	IdentityVerificationRejectedInsufficientEvidence = "rejected_insufficient_evidence"
	IdentityVerificationUnavailable                  = "alternative_unavailable"
)

type CacheStatus string

const (
	CacheStatusNotUsed     CacheStatus = "not_used"
	CacheStatusHit         CacheStatus = "hit"
	CacheStatusMiss        CacheStatus = "miss"
	CacheStatusNotModified CacheStatus = "not_modified"
)

// FailurePhase names the connection stage a Feed fetch reached before it
// failed. It is derived from httptrace callbacks plus body-read wrapping, so a
// fast HTTP refusal (e.g. 403) is always response_header/body_read — never
// connect. It is a classification aid only and cannot by itself prove WAF/CDN
// or IP-root-cause.
type FailurePhase string

const (
	FailurePhaseNotObserved    FailurePhase = "not_observed"
	FailurePhaseDNS            FailurePhase = "dns"
	FailurePhaseConnect        FailurePhase = "connect"
	FailurePhaseTLS            FailurePhase = "tls"
	FailurePhaseResponseHeader FailurePhase = "response_header"
	FailurePhaseBodyRead       FailurePhase = "body_read"
)

type Freshness string

const (
	FreshnessLive    Freshness = "live"
	FreshnessFresh   Freshness = "fresh"
	FreshnessStale   Freshness = "stale"
	FreshnessUnknown Freshness = "unknown"
)

const (
	EgressDirect  = "direct"
	EgressUnknown = "unknown"
)

// AccessOutcome contains only bounded, whitelisted metadata. It deliberately
// has no response body, cookies, credentials, or arbitrary response headers.
type AccessOutcome struct {
	HTTPStatus           *int          `json:"http_status"`
	ErrorCategory        ErrorCategory `json:"error_category"`
	TargetDomain         string        `json:"target_domain"`
	ResponseTimeMs       int           `json:"response_time_ms"`
	RetryAfter           string        `json:"retry_after,omitempty"`
	ETag                 string        `json:"etag,omitempty"`
	LastModified         string        `json:"last_modified,omitempty"`
	CacheControl         string        `json:"cache_control,omitempty"`
	Expires              string        `json:"expires,omitempty"`
	Age                  string        `json:"age,omitempty"`
	ResponseBytes        int64         `json:"response_bytes"`
	SourceType           AccessSource  `json:"source_type"`
	SourceURL            string        `json:"source_url"`
	IdentityVerification string        `json:"identity_verification"`
	CacheStatus          CacheStatus   `json:"cache_status"`
	Freshness            Freshness     `json:"freshness"`
	EgressID             string        `json:"egress_id"`
	CircuitState         CircuitState  `json:"circuit_state"`
	FailurePhase         FailurePhase  `json:"failure_phase,omitempty"`
	RetrievedAt          *time.Time    `json:"retrieved_at,omitempty"`
}

func newPrimaryAccessOutcome(feedURL string) AccessOutcome {
	return AccessOutcome{
		ErrorCategory:        ErrorCategoryNotObserved,
		TargetDomain:         TargetDomain(feedURL),
		SourceType:           AccessSourcePrimary,
		SourceURL:            SanitizeFeedURL(feedURL),
		IdentityVerification: IdentityVerificationNotChecked,
		CacheStatus:          CacheStatusNotUsed,
		Freshness:            FreshnessUnknown,
		EgressID:             EgressDirect,
		CircuitState:         CircuitStateNotUsed,
	}
}

// TargetDomain returns the lower-case hostname only. It never includes user
// info, query values, paths, or a port.
func TargetDomain(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

// SanitizeFeedURL removes URL user info and redacts query values that commonly
// carry credentials. It is safe for logs and user-facing execution summaries.
func SanitizeFeedURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid-feed-url>"
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		if sensitiveQueryKey(key) {
			query.Set(key, "[REDACTED]")
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func sensitiveQueryKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), ".", "_"))
	for _, marker := range []string{
		"token", "secret", "password", "passwd", "credential", "authorization", "cookie", "api_key", "apikey", "access_key", "access_token", "signature",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return normalized == "auth" || strings.HasPrefix(normalized, "auth_") || strings.HasSuffix(normalized, "_auth") ||
		normalized == "sig" || strings.HasPrefix(normalized, "sig_") || strings.HasSuffix(normalized, "_sig")
}

func errorCategoryForStatus(status int) ErrorCategory {
	switch status {
	case 401, 403:
		return ErrorCategoryAccessDenied
	case 429:
		return ErrorCategoryRateLimited
	case 503:
		return ErrorCategoryServiceUnavailable
	case 400, 404, 405, 406, 408, 410, 451:
		return ErrorCategoryHTTP
	default:
		if status >= 400 && status < 500 {
			return ErrorCategoryHTTP
		}
		if status >= 500 {
			return ErrorCategoryServiceUnavailable
		}
		return ErrorCategoryUnknown
	}
}
