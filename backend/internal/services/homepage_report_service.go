package services

import (
	"strings"
	"time"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

const (
	defaultHomepageHistoryLimit = 30
	maxHomepageHistoryLimit     = 100
)

// HomepageReportEpisode is one interactive episode on a discovery homepage report.
type HomepageReportEpisode struct {
	EpisodeID       uint   `json:"episode_id"`
	Order           int    `json:"order"`
	PodcastID       uint   `json:"podcast_id"`
	PodcastTitle    string `json:"podcast_title"`
	PodcastCoverURL string `json:"podcast_cover_url,omitempty"`
	EpisodeTitle    string `json:"episode_title"`
	EpisodeNo       string `json:"episode_no,omitempty"`
	Duration        int    `json:"duration,omitempty"`
	PublishedDate   string `json:"published_date,omitempty"`
	ImageURL        string `json:"image_url,omitempty"`
	Link            string `json:"link,omitempty"`
	// Recommendation is report-authored rationale; never ordinary Show Notes (#93).
	Recommendation string `json:"recommendation,omitempty"`
	// Context is program context (Show Notes excerpt etc.).
	Context string `json:"context,omitempty"`
	// Excerpt is a deprecated alias of Context for older clients.
	Excerpt           string     `json:"excerpt,omitempty"`
	DecisionState     string     `json:"decision_state"`
	DecisionUpdatedAt *time.Time `json:"decision_updated_at,omitempty"`
}

// HomepageReport is a publishable workflow report for the discovery homepage.
// When MetadataOnly is true, Content is omitted from JSON for bounded history lists (#95).
type HomepageReport struct {
	ID           uint                    `json:"id"`
	JobID        uint                    `json:"job_id"`
	WorkflowID   uint                    `json:"workflow_id"`
	WorkflowName string                  `json:"workflow_name"`
	ReportType   string                  `json:"report_type"`
	Title        string                  `json:"title"`
	Theme        string                  `json:"theme"`
	Content      string                  `json:"content,omitempty"`
	Summary      string                  `json:"summary,omitempty"`
	CompletedAt  time.Time               `json:"completed_at"`
	GeneratedAt  time.Time               `json:"generated_at"`
	EpisodeCount int                     `json:"episode_count"`
	Episodes     []HomepageReportEpisode `json:"episodes"`
	// MetadataOnly marks history summary rows without full Markdown body (#95).
	MetadataOnly bool `json:"metadata_only,omitempty"`
}

// HomepageReportsPayload is the discovery reports API body.
type HomepageReportsPayload struct {
	Date     string           `json:"date"`
	Timezone string           `json:"timezone"`
	Today    []HomepageReport `json:"today"`
	// History is metadata-only by default (no full Markdown bodies).
	History []HomepageReport `json:"history,omitempty"`
}

// HomepageReportService lists structured homepage workflow reports.
type HomepageReportService struct {
	db       *gorm.DB
	location *time.Location
	now      func() time.Time
}

func NewHomepageReportService(db *gorm.DB) *HomepageReportService {
	return NewHomepageReportServiceWithLocation(db, time.UTC)
}

func NewHomepageReportServiceWithLocation(db *gorm.DB, location *time.Location) *HomepageReportService {
	if location == nil {
		location = time.UTC
	}
	return &HomepageReportService{
		db:       db,
		location: location,
		now:      time.Now,
	}
}

// ListToday returns today's publishable homepage reports with full content, newest first.
func (s *HomepageReportService) ListToday() ([]HomepageReport, error) {
	start, end := s.todayBounds()
	return s.listPublished(start, end, 0, false)
}

// ListHistory returns past publishable homepage report metadata before today, newest first.
// Bodies are omitted so the first homepage load is not forced to download many Markdown reports (#95).
func (s *HomepageReportService) ListHistory(limit int) ([]HomepageReport, error) {
	if limit <= 0 {
		limit = defaultHomepageHistoryLimit
	}
	if limit > maxHomepageHistoryLimit {
		limit = maxHomepageHistoryLimit
	}
	start, _ := s.todayBounds()
	return s.listPublished(time.Time{}, start, limit, true)
}

// GetPublishedReport returns one publishable report with full body for on-demand history reading (#95).
func (s *HomepageReportService) GetPublishedReport(reportID uint) (*HomepageReport, error) {
	if reportID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	type reportRow struct {
		models.Report
		WorkflowID uint       `gorm:"column:workflow_id"`
		JobStatus  string     `gorm:"column:job_status"`
		JobEndTime *time.Time `gorm:"column:job_end_time"`
	}
	var row reportRow
	err := s.db.Table("reports").
		Select(`reports.*, jobs.workflow_id AS workflow_id, jobs.status AS job_status, jobs.end_time AS job_end_time`).
		Joins("JOIN jobs ON jobs.id = reports.job_id AND jobs.deleted_at IS NULL").
		Where("reports.id = ?", reportID).
		Where("reports.publish_to_homepage = ?", true).
		Where("reports.report_type IN ?", []string{
			string(models.HomepageReportTypeDaily),
			string(models.HomepageReportTypeWeekly),
		}).
		Where("jobs.status = ?", string(models.JobStatusCompleted)).
		First(&row).Error
	if err != nil {
		return nil, err
	}
	return s.toHomepageReport(row.Report, row.WorkflowID, row.JobEndTime, false), nil
}

// ListTodayAndHistory is the homepage combined payload.
// Today rows include full content; history rows are metadata-only (#95).
func (s *HomepageReportService) ListTodayAndHistory(historyLimit int) (HomepageReportsPayload, error) {
	today, err := s.ListToday()
	if err != nil {
		return HomepageReportsPayload{}, err
	}
	history, err := s.ListHistory(historyLimit)
	if err != nil {
		return HomepageReportsPayload{}, err
	}
	start, _ := s.todayBounds()
	return HomepageReportsPayload{
		Date:     start.Format("2006-01-02"),
		Timezone: s.location.String(),
		Today:    today,
		History:  history,
	}, nil
}

func (s *HomepageReportService) todayBounds() (time.Time, time.Time) {
	now := s.now().In(s.location)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)
	end := start.AddDate(0, 0, 1)
	return start, end
}

func (s *HomepageReportService) listPublished(from, to time.Time, limit int, metadataOnly bool) ([]HomepageReport, error) {
	type reportRow struct {
		models.Report
		WorkflowID uint       `gorm:"column:workflow_id"`
		JobStatus  string     `gorm:"column:job_status"`
		JobEndTime *time.Time `gorm:"column:job_end_time"`
	}

	// #93: only completed jobs — partial/failed/cancelled never enter homepage lists.
	query := s.db.Table("reports").
		Select(`reports.*, jobs.workflow_id AS workflow_id, jobs.status AS job_status, jobs.end_time AS job_end_time`).
		Joins("JOIN jobs ON jobs.id = reports.job_id AND jobs.deleted_at IS NULL").
		Where("reports.publish_to_homepage = ?", true).
		Where("reports.report_type IN ?", []string{
			string(models.HomepageReportTypeDaily),
			string(models.HomepageReportTypeWeekly),
		}).
		Where("jobs.status = ?", string(models.JobStatusCompleted))

	// Completion clock: prefer job end_time, fall back to report.generated_at.
	completionExpr := "COALESCE(jobs.end_time, reports.generated_at)"
	if !from.IsZero() {
		query = query.Where(completionExpr+" >= ?", from.UTC())
	}
	if !to.IsZero() {
		query = query.Where(completionExpr+" < ?", to.UTC())
	}
	query = query.Order(completionExpr + " DESC").Order("reports.id DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var rows []reportRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}

	// Batch-resolve live episode IDs once for all rows (#93).
	candidateIDs := make([]uint, 0)
	for _, row := range rows {
		for _, item := range row.StructuredEpisodes.ValidEpisodes() {
			candidateIDs = append(candidateIDs, item.EpisodeID)
		}
	}
	liveIDs, err := s.liveEpisodeIDSet(candidateIDs)
	if err != nil {
		return nil, err
	}

	out := make([]HomepageReport, 0, len(rows))
	for _, row := range rows {
		report := s.toHomepageReportFiltered(row.Report, row.WorkflowID, row.JobEndTime, metadataOnly, liveIDs)
		if report == nil {
			continue
		}
		out = append(out, *report)
	}
	return out, nil
}

func (s *HomepageReportService) liveEpisodeIDSet(ids []uint) (map[uint]struct{}, error) {
	result := make(map[uint]struct{})
	if len(ids) == 0 {
		return result, nil
	}
	// Deduplicate inputs.
	unique := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return result, nil
	}
	// GORM soft-delete scope excludes deleted_at IS NOT NULL automatically.
	var existing []uint
	if err := s.db.Model(&models.Episode{}).
		Where("id IN ?", unique).
		Pluck("id", &existing).Error; err != nil {
		return nil, err
	}
	for _, id := range existing {
		result[id] = struct{}{}
	}
	return result, nil
}

func (s *HomepageReportService) toHomepageReport(
	report models.Report,
	workflowID uint,
	jobEndTime *time.Time,
	metadataOnly bool,
) *HomepageReport {
	ids, err := s.liveEpisodeIDSet(collectStructuredIDs(report.StructuredEpisodes))
	if err != nil {
		return nil
	}
	return s.toHomepageReportFiltered(report, workflowID, jobEndTime, metadataOnly, ids)
}

func collectStructuredIDs(list models.ReportEpisodeList) []uint {
	out := make([]uint, 0, len(list))
	for _, item := range list.ValidEpisodes() {
		out = append(out, item.EpisodeID)
	}
	return out
}

func (s *HomepageReportService) toHomepageReportFiltered(
	report models.Report,
	workflowID uint,
	jobEndTime *time.Time,
	metadataOnly bool,
	liveIDs map[uint]struct{},
) *HomepageReport {
	valid := report.StructuredEpisodes.ValidEpisodes()
	if len(valid) == 0 {
		return nil
	}

	completedAt := report.GeneratedAt
	if jobEndTime != nil && !jobEndTime.IsZero() {
		completedAt = *jobEndTime
	}

	workflowName := report.WorkflowName
	if workflowName == "" {
		workflowName = report.Title
	}

	episodes := make([]HomepageReportEpisode, 0, len(valid))
	for _, item := range valid {
		if _, ok := liveIDs[item.EpisodeID]; !ok {
			// Missing or soft-deleted library episodes never surface on homepage (#93).
			continue
		}
		order := item.Order
		if order <= 0 {
			order = len(episodes) + 1
		}
		contextText := strings.TrimSpace(item.Context)
		if contextText == "" {
			// Legacy rows stored Show Notes under Excerpt; keep as context only.
			contextText = strings.TrimSpace(item.Excerpt)
		}
		recommendation := strings.TrimSpace(item.Recommendation)
		episodes = append(episodes, HomepageReportEpisode{
			EpisodeID:       item.EpisodeID,
			Order:           order,
			PodcastID:       item.PodcastID,
			PodcastTitle:    item.PodcastTitle,
			PodcastCoverURL: item.PodcastCoverURL,
			EpisodeTitle:    item.EpisodeTitle,
			EpisodeNo:       item.EpisodeNo,
			Duration:        item.Duration,
			PublishedDate:   item.PublishedDate,
			ImageURL:        item.ImageURL,
			Link:            sanitizeHomepageLink(item.Link),
			Recommendation:  recommendation,
			Context:         contextText,
			// Do not re-export legacy excerpt as recommendation; omit unless empty context needs alias.
			Excerpt:       "",
			DecisionState: models.TriageStatePending,
		})
	}
	if len(episodes) == 0 {
		// After live filtering, zero interactive episodes => report not homepage-eligible (#93).
		return nil
	}

	content := report.Content
	summary := report.Summary
	if metadataOnly {
		content = ""
		// Keep a short summary for list cards if present; strip huge bodies.
		if len(summary) > 240 {
			runes := []rune(summary)
			if len(runes) > 240 {
				summary = string(runes[:240]) + "…"
			}
		}
	}

	return &HomepageReport{
		ID:           report.ID,
		JobID:        report.JobID,
		WorkflowID:   workflowID,
		WorkflowName: workflowName,
		ReportType:   report.ReportType,
		Title:        report.Title,
		Theme:        homepageReportTheme(report, episodes, workflowName),
		Content:      content,
		Summary:      summary,
		CompletedAt:  completedAt,
		GeneratedAt:  report.GeneratedAt,
		EpisodeCount: len(episodes),
		Episodes:     episodes,
		MetadataOnly: metadataOnly,
	}
}

const maxHomepageReportThemeRunes = 64

func homepageReportTheme(
	report models.Report,
	episodes []HomepageReportEpisode,
	workflowName string,
) string {
	for _, line := range strings.Split(report.LLMSummary, "\n") {
		theme := strings.TrimSpace(strings.TrimLeft(line, "#> \t"))
		if strings.HasPrefix(theme, "- ") || strings.HasPrefix(theme, "* ") {
			theme = strings.TrimSpace(theme[2:])
		}
		if theme != "" {
			return truncateHomepageReportTheme(theme)
		}
	}

	if len(episodes) > 0 {
		theme := strings.TrimSpace(episodes[0].EpisodeTitle)
		if theme != "" {
			if len(episodes) > 1 {
				theme += " 等精选"
			}
			return truncateHomepageReportTheme(theme)
		}
	}

	title := strings.TrimSpace(report.Title)
	if title != "" && title != workflowName &&
		!strings.HasPrefix(title, workflowName+" - ") {
		return truncateHomepageReportTheme(title)
	}
	return workflowName
}

func truncateHomepageReportTheme(theme string) string {
	runes := []rune(strings.TrimSpace(theme))
	if len(runes) <= maxHomepageReportThemeRunes {
		return string(runes)
	}
	return string(runes[:maxHomepageReportThemeRunes]) + "…"
}

func sanitizeHomepageLink(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "vbscript:") {
		return ""
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return raw
	}
	return ""
}

// AttachDecisions fills triage state for all episodes across reports.
func AttachHomepageReportDecisions(
	reports []HomepageReport,
	decisions map[uint]models.EpisodeTriageDecision,
) {
	if len(reports) == 0 || len(decisions) == 0 {
		return
	}
	for i := range reports {
		for j := range reports[i].Episodes {
			episodeID := reports[i].Episodes[j].EpisodeID
			if decision, ok := decisions[episodeID]; ok {
				reports[i].Episodes[j].DecisionState = decision.State
				decidedAt := decision.DecidedAt
				reports[i].Episodes[j].DecisionUpdatedAt = &decidedAt
			}
		}
	}
}

// CollectHomepageEpisodeIDs gathers unique episode IDs across reports.
func CollectHomepageEpisodeIDs(reports ...[]HomepageReport) []uint {
	seen := make(map[uint]struct{})
	var ids []uint
	for _, group := range reports {
		for _, report := range group {
			for _, episode := range report.Episodes {
				if episode.EpisodeID == 0 {
					continue
				}
				if _, ok := seen[episode.EpisodeID]; ok {
					continue
				}
				seen[episode.EpisodeID] = struct{}{}
				ids = append(ids, episode.EpisodeID)
			}
		}
	}
	return ids
}
