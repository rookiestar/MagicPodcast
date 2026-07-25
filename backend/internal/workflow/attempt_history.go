package workflow

import (
	"strings"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// ErrorCategoryUserLabel maps stable machine categories to user-facing copy.
// Keep in sync with frontend display for timeout / network / 403 / parse / policy.
func ErrorCategoryUserLabel(category string) string {
	switch feed.ErrorCategory(category) {
	case feed.ErrorCategoryAccessDenied:
		return "访问被拒绝 (403/401)"
	case feed.ErrorCategoryUserAgentDenied:
		return "User-Agent 被上游 ACL 拒绝"
	case feed.ErrorCategoryUserAgentBlocked:
		return "User-Agent 已被同域策略阻断"
	case feed.ErrorCategoryTimeout:
		return "超时"
	case feed.ErrorCategoryNetwork:
		return "网络错误"
	case feed.ErrorCategoryParse:
		return "解析失败"
	case feed.ErrorCategoryPolicyRejected:
		return "策略拒绝"
	case feed.ErrorCategoryRateLimited:
		return "限流 (429)"
	case feed.ErrorCategoryServiceUnavailable:
		return "服务不可用 (5xx)"
	case feed.ErrorCategoryCircuitOpen:
		return "断路器打开（派生策略）"
	case feed.ErrorCategoryUnattempted:
		return "未尝试（批次截止）"
	case feed.ErrorCategoryNone, feed.ErrorCategoryNotObserved:
		if category == string(feed.ErrorCategoryNotObserved) {
			return "未观测"
		}
		return "成功"
	default:
		if category == "" {
			return "未观测"
		}
		return category
	}
}

// PersistFeedAttempt appends one safe attempt row. Never stores body/cookie/token.
func PersistFeedAttempt(db *gorm.DB, attempt *models.JobFeedAttempt) error {
	if db == nil || attempt == nil {
		return nil
	}
	if !db.Migrator().HasTable(&models.JobFeedAttempt{}) {
		return nil
	}
	if attempt.AttemptedAt.IsZero() {
		attempt.AttemptedAt = time.Now()
	}
	// Sanitize URL if present.
	if attempt.SourceURL != "" {
		attempt.SourceURL = feed.SanitizeFeedURL(attempt.SourceURL)
	}
	attempt.DerivedPolicy = isDerivedPolicyCategory(attempt.ErrorCategory)
	return db.Create(attempt).Error
}

func isDerivedPolicyCategory(category string) bool {
	switch feed.ErrorCategory(category) {
	case feed.ErrorCategoryCircuitOpen, feed.ErrorCategoryUserAgentBlocked:
		return true
	default:
		return false
	}
}

// RootCauseSummary aggregates attempt rows without double-counting derived
// policy actions (e.g. circuit_open/user_agent_blocked) as independent
// upstream failures.
type RootCauseSummary struct {
	TotalFeeds           int               `json:"total_feeds"`
	AttemptedFeeds       int               `json:"attempted_feeds"`
	UnattemptedFeeds     int               `json:"unattempted_feeds"`
	PrimarySuccesses     int               `json:"primary_successes"`
	AlternativeSuccesses int               `json:"alternative_successes"`
	FinalSuccesses       int               `json:"final_successes"`
	FinalFailures        int               `json:"final_failures"`
	UpstreamRootCauses   map[string]int    `json:"upstream_root_causes"`
	DerivedPolicyActions map[string]int    `json:"derived_policy_actions"`
	UserLabels           map[string]string `json:"user_labels"`
}

// BuildRootCauseSummary inspects attempt history for a job.
func BuildRootCauseSummary(attempts []models.JobFeedAttempt) RootCauseSummary {
	summary := RootCauseSummary{
		UpstreamRootCauses:   map[string]int{},
		DerivedPolicyActions: map[string]int{},
		UserLabels:           map[string]string{},
	}
	// Track final result per podcast (last final-flagged or last attempt).
	finalByPodcast := map[uint]models.JobFeedAttempt{}
	for _, a := range attempts {
		pid := uint(0)
		if a.PodcastID != nil {
			pid = *a.PodcastID
		}
		if a.IsFinalResult || finalByPodcast[pid].ID == 0 || a.AttemptNo >= finalByPodcast[pid].AttemptNo {
			if a.IsFinalResult {
				finalByPodcast[pid] = a
			} else if !finalByPodcast[pid].IsFinalResult {
				finalByPodcast[pid] = a
			}
		}
		if a.DerivedPolicy || isDerivedPolicyCategory(a.ErrorCategory) {
			summary.DerivedPolicyActions[a.ErrorCategory]++
			summary.UserLabels[a.ErrorCategory] = ErrorCategoryUserLabel(a.ErrorCategory)
			continue
		}
		if a.ErrorCategory == string(feed.ErrorCategoryUnattempted) {
			summary.UserLabels[a.ErrorCategory] = ErrorCategoryUserLabel(a.ErrorCategory)
			continue
		}
		if a.ErrorCategory != "" && a.ErrorCategory != string(feed.ErrorCategoryNone) &&
			a.ErrorCategory != string(feed.ErrorCategoryNotObserved) {
			// Count each attempt's upstream category once; finals summarized below.
			summary.UpstreamRootCauses[a.ErrorCategory]++
			summary.UserLabels[a.ErrorCategory] = ErrorCategoryUserLabel(a.ErrorCategory)
		}
		if a.IsFinalResult && a.SourceType == string(feed.AccessSourcePrimary) && a.ErrorCategory == string(feed.ErrorCategoryNone) {
			summary.PrimarySuccesses++
		}
		if a.IsFinalResult && a.SourceType == string(feed.AccessSourceAlternative) && a.ErrorCategory == string(feed.ErrorCategoryNone) {
			summary.AlternativeSuccesses++
		}
	}
	for _, final := range finalByPodcast {
		summary.TotalFeeds++
		if final.ErrorCategory == string(feed.ErrorCategoryUnattempted) {
			summary.UnattemptedFeeds++
			continue
		}
		summary.AttemptedFeeds++
		if final.ErrorCategory == string(feed.ErrorCategoryNone) {
			summary.FinalSuccesses++
		} else if final.ErrorCategory != "" && final.ErrorCategory != string(feed.ErrorCategoryNotObserved) {
			// Do not re-add derived finals into upstream map if already classified.
			if isDerivedPolicyCategory(final.ErrorCategory) {
				summary.FinalFailures++
				continue
			}
			summary.FinalFailures++
		} else if final.ErrorCategory != string(feed.ErrorCategoryNone) {
			summary.FinalFailures++
		}
	}
	return summary
}

// ListFeedAttempts returns attempts for a job ordered by podcast and attempt no.
func ListFeedAttempts(db *gorm.DB, jobID uint) ([]models.JobFeedAttempt, error) {
	if db == nil {
		return nil, nil
	}
	if !db.Migrator().HasTable(&models.JobFeedAttempt{}) {
		return []models.JobFeedAttempt{}, nil
	}
	var rows []models.JobFeedAttempt
	err := db.Where("job_id = ?", jobID).
		Order("podcast_id ASC, attempt_no ASC, id ASC").
		Find(&rows).Error
	return rows, err
}

// attemptFromExecution builds a final-result attempt projection from JobExecution
// when no granular history exists (legacy jobs).
func attemptFromExecution(exec *models.JobExecution) models.JobFeedAttempt {
	if exec == nil {
		return models.JobFeedAttempt{}
	}
	return models.JobFeedAttempt{
		JobID:                exec.JobID,
		PodcastID:            exec.PodcastID,
		AttemptNo:            1,
		SourceType:           exec.FeedSourceType,
		AttemptedAt:          exec.UpdatedAt,
		HTTPStatus:           exec.FeedHTTPStatus,
		ErrorCategory:        exec.FeedErrorCategory,
		IdentityVerification: exec.FeedIdentityVerification,
		TargetDomain:         exec.FeedTargetDomain,
		SourceURL:            exec.FeedSourceURL,
		IsFinalResult:        true,
		DerivedPolicy:        isDerivedPolicyCategory(exec.FeedErrorCategory),
	}
}

// SanitizeAttemptForAPI strips any accidental sensitive fields before JSON.
// Current model is already safe; this guards source_url.
func SanitizeAttemptForAPI(a models.JobFeedAttempt) models.JobFeedAttempt {
	a.SourceURL = feed.SanitizeFeedURL(a.SourceURL)
	// Never include raw headers/bodies — none are on the struct.
	if strings.Contains(strings.ToLower(a.SourceURL), "cookie") {
		a.SourceURL = feed.SanitizeFeedURL(a.SourceURL)
	}
	return a
}
