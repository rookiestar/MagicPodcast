package services

import (
	"errors"
	"time"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

var (
	ErrInvalidTriageState    = errors.New("invalid triage state")
	ErrTriageEpisodeNotFound = errors.New("triage episode not found")
)

type TriageService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewTriageService(db *gorm.DB) *TriageService {
	return &TriageService{
		db:  db,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func validTriageState(state string) bool {
	switch state {
	case models.TriageStatePending, models.TriageStateShortlisted, models.TriageStateDiscarded:
		return true
	default:
		return false
	}
}

func (s *TriageService) SetDecision(episodeID uint, state string) (*models.EpisodeTriageDecision, error) {
	if !validTriageState(state) {
		return nil, ErrInvalidTriageState
	}

	var result models.EpisodeTriageDecision
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var episodeCount int64
		if err := tx.Model(&models.Episode{}).Where("id = ?", episodeID).Count(&episodeCount).Error; err != nil {
			return err
		}
		if episodeCount == 0 {
			return ErrTriageEpisodeNotFound
		}

		err := tx.Where("episode_id = ?", episodeID).First(&result).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result = models.EpisodeTriageDecision{
				EpisodeID: episodeID,
				State:     state,
				DecidedAt: s.now(),
			}
			return tx.Create(&result).Error
		}
		if err != nil {
			return err
		}
		if result.State == state {
			return nil
		}

		result.State = state
		result.DecidedAt = s.now()
		return tx.Model(&result).Updates(map[string]any{
			"state":      result.State,
			"decided_at": result.DecidedAt,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *TriageService) DecisionsForEpisodes(episodeIDs []uint) (map[uint]models.EpisodeTriageDecision, error) {
	result := make(map[uint]models.EpisodeTriageDecision, len(episodeIDs))
	if len(episodeIDs) == 0 {
		return result, nil
	}

	var decisions []models.EpisodeTriageDecision
	if err := s.db.Where("episode_id IN ?", episodeIDs).Find(&decisions).Error; err != nil {
		return nil, err
	}
	for _, decision := range decisions {
		result[decision.EpisodeID] = decision
	}
	return result, nil
}
