package services

import (
	"time"

	episodelabel "magicpodcast/internal/episode"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

const (
	defaultDiscoveryCandidateLimit = 500
	maxDiscoveryCandidateLimit     = 1000
	discoverySourceRecentUpdates   = "最近更新"
	discoveryRecentWindow          = 7 * 24 * time.Hour
)

type DiscoveryCandidate struct {
	EpisodeID         uint               `json:"episode_id"`
	PodcastID         uint               `json:"podcast_id"`
	PodcastTitle      string             `json:"podcast_title"`
	PodcastAuthor     string             `json:"podcast_author"`
	PodcastCoverURL   string             `json:"podcast_cover_url"`
	EpisodeTitle      string             `json:"episode_title"`
	EpisodeNo         string             `json:"episode_no"`
	Duration          int                `json:"duration"`
	CandidateTime     time.Time          `json:"candidate_time"`
	TimeBasis         string             `json:"time_basis"`
	Source            string             `json:"source"`
	Excerpt           string             `json:"excerpt,omitempty"`
	MetadataOnly      bool               `json:"metadata_only,omitempty"`
	ShowNotes         string             `json:"show_notes,omitempty"`
	ShowNotesStatus   string             `json:"show_notes_status"`
	OriginalURL       string             `json:"original_url"`
	ImageURL          string             `json:"image_url"`
	DecisionState     string             `json:"decision_state"`
	DecisionUpdatedAt *time.Time         `json:"decision_updated_at,omitempty"`
	QueueState        *string            `json:"queue_state"`
	DismissedAt       *time.Time         `json:"dismissed_at,omitempty"`
	QueueUpdatedAt    *time.Time         `json:"queue_updated_at,omitempty"`
	InProgressAt      *time.Time         `json:"in_progress_at,omitempty"`
	ReadAt            *time.Time         `json:"read_at,omitempty"`
	PreReads          []DiscoveryPreRead `json:"pre_reads,omitempty"`
}

type DiscoveryService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewDiscoveryService(db *gorm.DB) *DiscoveryService {
	return &DiscoveryService{
		db:  db,
		now: time.Now,
	}
}

func (s *DiscoveryService) ListRecentCandidates(limit int) ([]DiscoveryCandidate, error) {
	episodes, err := s.listRecentCandidateEpisodes(limit, true)
	if err != nil {
		return nil, err
	}

	candidates := make([]DiscoveryCandidate, 0, len(episodes))
	preReadGeneratedAt := s.now().UTC()
	for _, episode := range episodes {
		candidates = append(candidates, buildDiscoveryCandidate(episode, preReadGeneratedAt))
	}

	return candidates, nil
}

func (s *DiscoveryService) ListRecentCandidateSummaries(limit int) ([]DiscoveryCandidate, error) {
	episodes, err := s.listRecentCandidateEpisodes(limit, false)
	if err != nil {
		return nil, err
	}

	candidates := make([]DiscoveryCandidate, 0, len(episodes))
	for _, episode := range episodes {
		candidates = append(candidates, buildDiscoveryCandidateSummary(episode))
	}

	return candidates, nil
}

func (s *DiscoveryService) GetCandidate(episodeID uint) (*DiscoveryCandidate, error) {
	var episode models.Episode
	err := s.db.
		Preload("Podcast").
		Preload("Podcast.Tags").
		Preload("Tags").
		Joins("JOIN podcasts ON podcasts.id = episodes.podcast_id").
		Where("podcasts.is_subscribed = ?", true).
		Where("episodes.id = ?", episodeID).
		First(&episode).Error
	if err != nil {
		return nil, err
	}

	candidate := buildDiscoveryCandidate(episode, s.now().UTC())
	return &candidate, nil
}

func (s *DiscoveryService) listRecentCandidateEpisodes(
	limit int,
	includeSignals bool,
) ([]models.Episode, error) {
	if limit <= 0 {
		limit = defaultDiscoveryCandidateLimit
	}
	if limit > maxDiscoveryCandidateLimit {
		limit = maxDiscoveryCandidateLimit
	}
	cutoff := s.now().UTC().Add(-discoveryRecentWindow)
	recencyExpression := "COALESCE(episodes.fetched_at, episodes.created_at)"

	// Discovery only reads episodes already persisted by configured workflows.
	// It must not trigger or broaden podcast synchronization.
	var episodes []models.Episode
	query := s.db.Preload("Podcast")
	if includeSignals {
		query = query.Preload("Podcast.Tags").Preload("Tags")
	}
	err := query.
		Joins("JOIN podcasts ON podcasts.id = episodes.podcast_id").
		Where("podcasts.is_subscribed = ?", true).
		Where(recencyExpression+" >= ?", cutoff).
		Order(recencyExpression + " DESC").
		Order("episodes.id DESC").
		Limit(limit).
		Find(&episodes).Error
	if err != nil {
		return nil, err
	}
	return episodes, nil
}

func AttachConsumptionStateToCandidate(
	candidate *DiscoveryCandidate,
	state models.EpisodeTriageDecision,
) {
	candidate.DecisionState = state.State
	candidate.DecisionUpdatedAt = &state.DecidedAt
	candidate.QueueState = state.QueueState
	candidate.DismissedAt = state.DismissedAt
	candidate.QueueUpdatedAt = state.QueueUpdatedAt
	candidate.InProgressAt = state.InProgressAt
	candidate.ReadAt = state.ReadAt
}

func buildDiscoveryCandidate(episode models.Episode, preReadGeneratedAt time.Time) DiscoveryCandidate {
	candidate := buildDiscoveryCandidateMetadata(episode)
	candidate.ShowNotes = episode.ShowNotes
	candidate.PreReads = buildDiscoveryPreReads(episode, preReadGeneratedAt)
	return candidate
}

func buildDiscoveryCandidateSummary(episode models.Episode) DiscoveryCandidate {
	candidate := buildDiscoveryCandidateMetadata(episode)
	candidate.Excerpt = compactPreReadText(episode.ShowNotes, 220)
	candidate.MetadataOnly = true
	return candidate
}

func buildDiscoveryCandidateMetadata(episode models.Episode) DiscoveryCandidate {
	candidateTime := episode.CreatedAt
	timeBasis := "created_at"
	if episode.FetchedAt != nil && !episode.FetchedAt.IsZero() {
		candidateTime = *episode.FetchedAt
		timeBasis = "fetched_at"
	}

	showNotesStatus := "available"
	if episode.ShowNotes == "" {
		showNotesStatus = "missing"
	}

	candidate := DiscoveryCandidate{
		EpisodeID:       episode.ID,
		PodcastID:       episode.PodcastID,
		PodcastTitle:    episode.Podcast.Title,
		PodcastAuthor:   episode.Podcast.Author,
		PodcastCoverURL: episode.Podcast.CustomCoverURL,
		EpisodeTitle:    episode.Title,
		EpisodeNo:       episodelabel.Normalize(episode.Title, episode.EpisodeNo),
		Duration:        episode.Duration,
		CandidateTime:   candidateTime,
		TimeBasis:       timeBasis,
		Source:          discoverySourceRecentUpdates,
		ShowNotesStatus: showNotesStatus,
		OriginalURL:     episode.Link,
		ImageURL:        episode.ImageURL,
		DecisionState:   models.TriageStatePending,
	}
	if candidate.PodcastCoverURL == "" {
		candidate.PodcastCoverURL = episode.Podcast.CoverURL
	}
	return candidate
}
