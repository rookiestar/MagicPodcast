package personidentity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"magicpodcast/internal/models"
	"magicpodcast/internal/utils"
)

type preparationMetadata struct {
	PodcastTitle       string
	PodcastAuthor      string
	PodcastDescription string
	EpisodeTitle       string
	ShowNotes          string
	PublishedAt        time.Time
}

func readPreparationMetadata(db *gorm.DB, episodeID uint) (preparationMetadata, error) {
	var episode models.Episode
	if err := db.Select("id", "podcast_id", "title", "show_notes", "published_date").First(&episode, episodeID).Error; err != nil {
		return preparationMetadata{}, err
	}
	var podcast models.Podcast
	if err := db.Select("id", "title", "author", "description").First(&podcast, episode.PodcastID).Error; err != nil {
		return preparationMetadata{}, err
	}
	return preparationMetadata{PodcastTitle: podcast.Title, PodcastAuthor: podcast.Author, PodcastDescription: utils.HTMLToMarkdown(podcast.Description), EpisodeTitle: episode.Title, ShowNotes: utils.HTMLToMarkdown(episode.ShowNotes), PublishedAt: episode.PublishedDate}, nil
}

func (m preparationMetadata) digest() string {
	data, _ := json.Marshal(m)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func (s *Service) preparationCurrent(ctx context.Context, episodeID uint, sourceVersion string) (bool, error) {
	var state models.PersonPreparation
	err := s.db.WithContext(ctx).First(&state, "episode_id = ?", episodeID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if state.AlgorithmVersion != models.CurrentIdentityAlgorithm || state.SourceVersion != sourceVersion {
		return false, nil
	}
	metadata, err := readPreparationMetadata(s.db.WithContext(ctx), episodeID)
	if err != nil {
		return false, err
	}
	return state.MetadataDigest == metadata.digest(), nil
}

// reservePreparation fences older Runtime decisions without invalidating a
// previously published result while the replacement is still being computed.
func reservePreparation(tx *gorm.DB, episodeID uint) (uint, error) {
	state := models.PersonPreparation{EpisodeID: episodeID, Revision: 1, UpdatedAt: nowUTC()}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "episode_id"}}, DoUpdates: clause.Assignments(map[string]any{
		"revision": gorm.Expr("person_preparations.revision + 1"), "updated_at": state.UpdatedAt,
	})}).Create(&state).Error; err != nil {
		return 0, err
	}
	if err := tx.First(&state, "episode_id = ?", episodeID).Error; err != nil {
		return 0, err
	}
	return state.Revision, nil
}

func publishPreparation(tx *gorm.DB, episodeID uint, sourceVersion string, metadata preparationMetadata, revision uint) error {
	currentMetadata, err := readPreparationMetadata(tx, episodeID)
	if err != nil {
		return err
	}
	if currentMetadata.digest() != metadata.digest() {
		return ErrSourcesChanged
	}
	var artifact models.EpisodeArtifactSet
	err = tx.Where("episode_id = ? AND is_current = ?", episodeID, true).First(&artifact).Error
	if err == nil {
		if fmt.Sprintf("artifact-%d", artifact.ID) != sourceVersion {
			return ErrSourcesChanged
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		var count int64
		if err := tx.Model(&models.EpisodeArtifactSet{}).Where("episode_id = ?", episodeID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 || strings.HasPrefix(sourceVersion, "artifact-") {
			return ErrSourcesChanged
		}
	} else {
		return err
	}
	result := tx.Model(&models.PersonPreparation{}).Where("episode_id = ? AND revision = ?", episodeID, revision).Updates(map[string]any{
		"published_revision": revision, "source_version": sourceVersion,
		"algorithm_version": models.CurrentIdentityAlgorithm, "metadata_digest": metadata.digest(), "updated_at": nowUTC(),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrSourcesChanged
	}
	return nil
}
