package services

import (
	"errors"
	"fmt"
	"time"

	episodelabel "magicpodcast/internal/episode"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

const FocusSoftLimit = 7

const (
	AttentionNone   = ""
	AttentionStale  = "stale"
	AttentionReview = "review"
)

var (
	ErrInvalidQueueState          = errors.New("invalid queue state")
	ErrConsumptionEpisodeNotFound = errors.New("consumption episode not found")
)

type FocusLimitConfirmationError struct {
	CurrentCount int
}

func (e *FocusLimitConfirmationError) Error() string {
	return fmt.Sprintf("Focus already contains %d items", e.CurrentCount)
}

type QueueWriteOptions struct {
	AcknowledgeFocusLimit bool
}

type QueueSummary struct {
	Counts         map[string]int64 `json:"counts"`
	FocusLimit     int              `json:"focus_limit"`
	FocusOverLimit bool             `json:"focus_over_limit"`
}

type ConsumptionItem struct {
	EpisodeID       uint         `json:"episode_id"`
	PodcastID       uint         `json:"podcast_id"`
	PodcastTitle    string       `json:"podcast_title"`
	PodcastAuthor   string       `json:"podcast_author"`
	PodcastCoverURL string       `json:"podcast_cover_url"`
	EpisodeTitle    string       `json:"episode_title"`
	EpisodeNo       string       `json:"episode_no"`
	Duration        int          `json:"duration"`
	PublishedDate   time.Time    `json:"published_date"`
	ShowNotes       string       `json:"show_notes"`
	OriginalURL     string       `json:"original_url"`
	ImageURL        string       `json:"image_url"`
	Notes           string       `json:"notes"`
	Tags            []models.Tag `json:"tags"`
	QueueState      *string      `json:"queue_state"`
	DismissedAt     *time.Time   `json:"dismissed_at,omitempty"`
	QueueUpdatedAt  *time.Time   `json:"queue_updated_at,omitempty"`
	InProgressAt    *time.Time   `json:"in_progress_at,omitempty"`
	ReadAt          *time.Time   `json:"read_at,omitempty"`
	ActivityAt      *time.Time   `json:"activity_at,omitempty"`
	Attention       string       `json:"attention,omitempty"`
}

type ConsumptionService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewConsumptionService(db *gorm.DB) *ConsumptionService {
	return &ConsumptionService{
		db:  db,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func ValidQueueState(queueState string) bool {
	switch queueState {
	case models.QueueStateInbox, models.QueueStateFocus, models.QueueStateSomeday, models.QueueStateDone:
		return true
	default:
		return false
	}
}

func (s *ConsumptionService) SetQueue(
	episodeID uint,
	queueState string,
	options QueueWriteOptions,
) (*models.EpisodeTriageDecision, error) {
	return s.moveQueueToHead(episodeID, queueState, options)
}

func (s *ConsumptionService) ClearQueue(episodeID uint) (*models.EpisodeTriageDecision, error) {
	return s.clearQueueState(episodeID)
}

func (s *ConsumptionService) SetDismissed(
	episodeID uint,
	dismissed bool,
) (*models.EpisodeTriageDecision, error) {
	return s.setDismissedState(episodeID, dismissed)
}

func (s *ConsumptionService) MarkRead(episodeID uint) (*models.EpisodeTriageDecision, error) {
	return s.updateTimestamp(episodeID, "read_at", false)
}

func (s *ConsumptionService) MarkInProgress(episodeID uint) (*models.EpisodeTriageDecision, error) {
	return s.updateTimestamp(episodeID, "in_progress_at", true)
}

func (s *ConsumptionService) updateTimestamp(
	episodeID uint,
	column string,
	refresh bool,
) (*models.EpisodeTriageDecision, error) {
	var result models.EpisodeTriageDecision
	err := s.db.Transaction(func(tx *gorm.DB) error {
		current, err := s.ensureState(tx, episodeID)
		if err != nil {
			return err
		}
		result = *current
		if !refresh {
			switch column {
			case "read_at":
				if current.ReadAt != nil {
					return nil
				}
			default:
				return fmt.Errorf("unsupported consumption timestamp %q", column)
			}
		}
		if err := tx.Model(current).Update(column, s.now().UTC()).Error; err != nil {
			return err
		}
		result = models.EpisodeTriageDecision{}
		return tx.Where("episode_id = ?", episodeID).First(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *ConsumptionService) ListQueue(queueState string) (QueueSnapshot, error) {
	return s.listQueueSnapshot(queueState)
}

func (s *ConsumptionService) QueueSummary() (QueueSummary, error) {
	type queueCount struct {
		QueueState string
		Count      int64
	}
	var rows []queueCount
	if err := s.db.Model(&models.EpisodeTriageDecision{}).
		Select("queue_state, COUNT(*) AS count").
		Where("queue_state IS NOT NULL").
		Group("queue_state").
		Scan(&rows).Error; err != nil {
		return QueueSummary{}, err
	}
	counts := map[string]int64{
		models.QueueStateInbox:   0,
		models.QueueStateFocus:   0,
		models.QueueStateSomeday: 0,
		models.QueueStateDone:    0,
	}
	for _, row := range rows {
		if ValidQueueState(row.QueueState) {
			counts[row.QueueState] = row.Count
		}
	}
	return QueueSummary{
		Counts:         counts,
		FocusLimit:     FocusSoftLimit,
		FocusOverLimit: counts[models.QueueStateFocus] > FocusSoftLimit,
	}, nil
}

func (s *ConsumptionService) StatesForEpisodes(
	episodeIDs []uint,
) (map[uint]models.EpisodeTriageDecision, error) {
	result := make(map[uint]models.EpisodeTriageDecision, len(episodeIDs))
	if len(episodeIDs) == 0 {
		return result, nil
	}
	var states []models.EpisodeTriageDecision
	if err := s.db.Where("episode_id IN ?", episodeIDs).Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		result[state.EpisodeID] = state
	}
	return result, nil
}

func (s *ConsumptionService) GetItem(episodeID uint) (*ConsumptionItem, error) {
	var state models.EpisodeTriageDecision
	err := s.db.
		Preload("Episode.Podcast").
		Preload("Episode.Tags").
		Where("episode_id = ?", episodeID).
		First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrConsumptionEpisodeNotFound
	}
	if err != nil {
		return nil, err
	}
	item := buildConsumptionItem(state, s.now().UTC())
	return &item, nil
}

func (s *ConsumptionService) ensureState(
	tx *gorm.DB,
	episodeID uint,
) (*models.EpisodeTriageDecision, error) {
	var episodeCount int64
	if err := tx.Model(&models.Episode{}).
		Where("id = ?", episodeID).
		Count(&episodeCount).Error; err != nil {
		return nil, err
	}
	if episodeCount == 0 {
		return nil, ErrConsumptionEpisodeNotFound
	}

	var state models.EpisodeTriageDecision
	err := tx.Where("episode_id = ?", episodeID).First(&state).Error
	if err == nil {
		return &state, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	now := s.now().UTC()
	state = models.EpisodeTriageDecision{
		EpisodeID: episodeID,
		State:     models.TriageStatePending,
		DecidedAt: now,
	}
	if err := tx.Create(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
}

func buildConsumptionItem(
	state models.EpisodeTriageDecision,
	now time.Time,
) ConsumptionItem {
	episode := state.Episode
	coverURL := episode.Podcast.CustomCoverURL
	if coverURL == "" {
		coverURL = episode.Podcast.CoverURL
	}
	activityAt := state.QueueUpdatedAt
	if state.InProgressAt != nil &&
		(activityAt == nil || state.InProgressAt.After(*activityAt)) {
		activityAt = state.InProgressAt
	}
	attention := AttentionNone
	if state.QueueState != nil && *state.QueueState != models.QueueStateDone && activityAt != nil {
		age := now.Sub(activityAt.UTC())
		switch {
		case age >= 30*24*time.Hour:
			attention = AttentionReview
		case age >= 7*24*time.Hour:
			attention = AttentionStale
		}
	}
	tags := episode.Tags
	if tags == nil {
		tags = []models.Tag{}
	}
	return ConsumptionItem{
		EpisodeID:       episode.ID,
		PodcastID:       episode.PodcastID,
		PodcastTitle:    episode.Podcast.Title,
		PodcastAuthor:   episode.Podcast.Author,
		PodcastCoverURL: coverURL,
		EpisodeTitle:    episode.Title,
		EpisodeNo:       episodelabel.Normalize(episode.Title, episode.EpisodeNo),
		Duration:        episode.Duration,
		PublishedDate:   episode.PublishedDate,
		ShowNotes:       episode.ShowNotes,
		OriginalURL:     episode.Link,
		ImageURL:        episode.ImageURL,
		Notes:           episode.Notes,
		Tags:            tags,
		QueueState:      state.QueueState,
		DismissedAt:     state.DismissedAt,
		QueueUpdatedAt:  state.QueueUpdatedAt,
		InProgressAt:    state.InProgressAt,
		ReadAt:          state.ReadAt,
		ActivityAt:      activityAt,
		Attention:       attention,
	}
}
