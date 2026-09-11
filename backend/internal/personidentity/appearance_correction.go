package personidentity

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"magicpodcast/internal/models"
)

type AppearanceCorrection struct {
	PersonID uint
	Role     *string
	Excluded *bool
}

func loadAppearanceOverrides(db *gorm.DB, episodeID uint) (map[uint]models.PersonAppearanceOverride, error) {
	var rows []models.PersonAppearanceOverride
	if err := db.Where("episode_id = ?", episodeID).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[uint]models.PersonAppearanceOverride, len(rows))
	for _, row := range rows {
		result[row.PersonID] = row
	}
	return result, nil
}

func (s *Service) CorrectAppearance(ctx context.Context, episodeID uint, correction AppearanceCorrection) (EpisodePeople, error) {
	if correction.PersonID == 0 || (correction.Role == nil && correction.Excluded == nil) {
		return EpisodePeople{}, ErrInvalidCorrection
	}
	if correction.Role != nil && *correction.Role != RoleHost && *correction.Role != RoleGuest && *correction.Role != RoleUnknown {
		return EpisodePeople{}, ErrInvalidCorrection
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var appearance models.EpisodeAppearance
		if err := tx.Where("episode_id = ? AND person_id = ?", episodeID, correction.PersonID).First(&appearance).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotEpisodeParticipant
			}
			return err
		}
		row := models.PersonAppearanceOverride{EpisodeID: episodeID, PersonID: correction.PersonID}
		if err := tx.First(&row, "episode_id = ? AND person_id = ?", episodeID, correction.PersonID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if correction.Role != nil {
			row.Role = correction.Role
		}
		if correction.Excluded != nil {
			row.Excluded = *correction.Excluded
		}
		row.UpdatedAt = nowUTC()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if _, err := reservePreparation(tx, episodeID); err != nil {
			return err
		}
		return s.reindexSearchTransaction(ctx, tx, episodeID)
	})
	if err != nil {
		return EpisodePeople{}, err
	}
	return s.ListEpisodePeople(ctx, episodeID)
}
