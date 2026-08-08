package services

import (
	"time"

	episodelabel "magicpodcast/internal/episode"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

const (
	defaultDiscoveryCandidateLimit = 20
	maxDiscoveryCandidateLimit     = 100
	discoverySourceRecentUpdates   = "最近更新"
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
	ShowNotes         string             `json:"show_notes"`
	ShowNotesStatus   string             `json:"show_notes_status"`
	OriginalURL       string             `json:"original_url"`
	ImageURL          string             `json:"image_url"`
	DecisionState     string             `json:"decision_state"`
	DecisionUpdatedAt *time.Time         `json:"decision_updated_at,omitempty"`
	PreReads          []DiscoveryPreRead `json:"pre_reads"`
}

type DiscoveryService struct {
	db       *gorm.DB
	location *time.Location
	now      func() time.Time
}

func NewDiscoveryService(db *gorm.DB) *DiscoveryService {
	return NewDiscoveryServiceWithLocation(db, time.UTC)
}

func NewDiscoveryServiceWithLocation(db *gorm.DB, location *time.Location) *DiscoveryService {
	if location == nil {
		location = time.UTC
	}
	return &DiscoveryService{
		db:       db,
		location: location,
		now:      time.Now,
	}
}

func (s *DiscoveryService) ListRecentCandidates(limit int) ([]DiscoveryCandidate, error) {
	if limit <= 0 {
		limit = defaultDiscoveryCandidateLimit
	}
	if limit > maxDiscoveryCandidateLimit {
		limit = maxDiscoveryCandidateLimit
	}

	var episodes []models.Episode
	err := s.db.
		Preload("Podcast").
		Preload("Podcast.Tags").
		Preload("Tags").
		Joins("JOIN podcasts ON podcasts.id = episodes.podcast_id").
		Where("podcasts.is_subscribed = ?", true).
		Order(`CASE
			WHEN episodes.published_date > '0001-01-02 00:00:00' THEN episodes.published_date
			WHEN episodes.updated_date IS NOT NULL THEN episodes.updated_date
			ELSE episodes.updated_at
		END DESC`).
		Order("episodes.id DESC").
		Limit(limit).
		Find(&episodes).Error
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

type TodayShortlist struct {
	Date       string               `json:"date"`
	Timezone   string               `json:"timezone"`
	Candidates []DiscoveryCandidate `json:"candidates"`
}

func (s *DiscoveryService) ListTodayShortlisted() (TodayShortlist, error) {
	now := s.now().In(s.location)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.location)
	end := start.AddDate(0, 0, 1)

	var decisions []models.EpisodeTriageDecision
	err := s.db.
		Preload("Episode.Podcast").
		Preload("Episode.Podcast.Tags").
		Preload("Episode.Tags").
		Where(
			"state = ? AND decided_at >= ? AND decided_at < ?",
			models.TriageStateShortlisted,
			start.UTC(),
			end.UTC(),
		).
		Order("decided_at DESC").
		Order("episode_id DESC").
		Find(&decisions).Error
	if err != nil {
		return TodayShortlist{}, err
	}

	candidates := make([]DiscoveryCandidate, 0, len(decisions))
	generatedAt := now.UTC()
	for _, decision := range decisions {
		candidate := buildDiscoveryCandidate(decision.Episode, generatedAt)
		candidate.DecisionState = decision.State
		candidate.DecisionUpdatedAt = &decision.DecidedAt
		candidates = append(candidates, candidate)
	}

	return TodayShortlist{
		Date:       start.Format("2006-01-02"),
		Timezone:   s.location.String(),
		Candidates: candidates,
	}, nil
}

func buildDiscoveryCandidate(episode models.Episode, preReadGeneratedAt time.Time) DiscoveryCandidate {
	candidateTime := episode.PublishedDate
	timeBasis := "published_date"
	if candidateTime.IsZero() {
		timeBasis = "updated_date"
		if episode.UpdatedDate != nil {
			candidateTime = *episode.UpdatedDate
		} else {
			candidateTime = episode.UpdatedAt
		}
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
		ShowNotes:       episode.ShowNotes,
		ShowNotesStatus: showNotesStatus,
		OriginalURL:     episode.Link,
		ImageURL:        episode.ImageURL,
		DecisionState:   models.TriageStatePending,
		PreReads:        buildDiscoveryPreReads(episode, preReadGeneratedAt),
	}
	if candidate.PodcastCoverURL == "" {
		candidate.PodcastCoverURL = episode.Podcast.CoverURL
	}
	return candidate
}
