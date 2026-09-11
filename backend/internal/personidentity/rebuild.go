package personidentity

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"magicpodcast/internal/models"
)

// RebuildPreview contains identity metadata only, never transcript text.
type RebuildPreview struct {
	EpisodeID                uint     `json:"episode_id"`
	Title                    string   `json:"title"`
	ArtifactID               uint     `json:"artifact_id"`
	CandidateNames           []string `json:"candidate_names"`
	NameConfirmations        int64    `json:"name_confirmations"`
	AttributionConfirmations int64    `json:"attribution_confirmations"`
	RoleConfirmations        int64    `json:"role_confirmations"`
	Exclusions               int64    `json:"exclusions"`
	NeedsPreparation         bool     `json:"needs_preparation"`
}

// PreviewRebuild is read-only and also supports the deployed pre-repair schema.
// An empty filter inventories episodes with existing identity facts, not the library.
func PreviewRebuild(ctx context.Context, db *gorm.DB, episodeIDs []uint) ([]RebuildPreview, error) {
	var result []RebuildPreview
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var episodes []models.Episode
		query := tx.Select("id", "title").Order("id")
		if len(episodeIDs) > 0 {
			query = query.Where("id IN ?", episodeIDs)
		} else {
			query = query.Where("EXISTS (SELECT 1 FROM episode_appearances a WHERE a.episode_id = episodes.id) OR EXISTS (SELECT 1 FROM person_user_confirmations c WHERE c.episode_id = episodes.id)")
		}
		if err := query.Find(&episodes).Error; err != nil {
			return err
		}
		if len(episodeIDs) > 0 && len(episodes) != len(episodeIDs) {
			return fmt.Errorf("episode list contains missing or duplicate IDs")
		}
		result = make([]RebuildPreview, 0, len(episodes))
		hasPreparation := tx.Migrator().HasTable(&models.PersonPreparation{})
		hasOverrides := tx.Migrator().HasTable(&models.PersonAppearanceOverride{})
		reader := &Service{db: tx}
		for _, episode := range episodes {
			item := RebuildPreview{EpisodeID: episode.ID, Title: episode.Title, CandidateNames: []string{}, NeedsPreparation: true}
			if err := tx.Model(&models.EpisodeArtifactSet{}).Where("episode_id = ? AND is_current = ?", episode.ID, true).Select("id").Scan(&item.ArtifactID).Error; err != nil {
				return err
			}
			if err := tx.Table("episode_appearances a").Joins("JOIN people p ON p.id = a.person_id").Where("a.episode_id = ?", episode.ID).Order("a.id").Pluck("p.display_name", &item.CandidateNames).Error; err != nil {
				return err
			}
			for kind, count := range map[string]*int64{models.PersonConfirmationKindName: &item.NameConfirmations, models.PersonConfirmationKindAttribution: &item.AttributionConfirmations} {
				if err := tx.Model(&models.PersonUserConfirmation{}).Where("episode_id = ? AND kind = ?", episode.ID, kind).Count(count).Error; err != nil {
					return err
				}
			}
			if hasOverrides {
				if err := tx.Model(&models.PersonAppearanceOverride{}).Where("episode_id = ? AND role IS NOT NULL", episode.ID).Count(&item.RoleConfirmations).Error; err != nil {
					return err
				}
				if err := tx.Model(&models.PersonAppearanceOverride{}).Where("episode_id = ? AND excluded = ?", episode.ID, true).Count(&item.Exclusions).Error; err != nil {
					return err
				}
			}
			if hasPreparation && item.ArtifactID != 0 {
				current, err := reader.preparationCurrent(ctx, episode.ID, fmt.Sprintf("artifact-%d", item.ArtifactID))
				if err != nil {
					return err
				}
				item.NeedsPreparation = !current
			}
			result = append(result, item)
		}
		return nil
	})
	return result, err
}

type RebuildResult struct {
	EpisodeID uint     `json:"episode_id"`
	Success   bool     `json:"success"`
	Before    []string `json:"before"`
	After     []string `json:"after"`
	Error     string   `json:"error,omitempty"`
}

// RebuildSelected uses exactly the interactive prepare path, sequentially and
// within its existing per-episode budget. Each failure is reported separately.
func (s *Service) RebuildSelected(ctx context.Context, episodeIDs []uint, report func(RebuildResult) error) error {
	if len(episodeIDs) == 0 || report == nil {
		return fmt.Errorf("explicit episode IDs and a report sink are required")
	}
	preview, err := PreviewRebuild(ctx, s.db, episodeIDs)
	if err != nil {
		return err
	}
	failures := 0
	for _, item := range preview {
		if err := ctx.Err(); err != nil {
			return err
		}
		result := RebuildResult{EpisodeID: item.EpisodeID, Before: item.CandidateNames, After: []string{}}
		episodeCtx, cancel := context.WithTimeout(ctx, 150*time.Second)
		people, err := s.PrepareCurrent(episodeCtx, item.EpisodeID)
		cancel()
		if err != nil {
			result.Error = err.Error()
			failures++
		} else {
			result.Success = true
			for _, person := range people.People {
				result.After = append(result.After, person.DisplayName)
			}
		}
		if err := report(result); err != nil {
			return err
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d episodes failed preparation", failures)
	}
	return nil
}
