package episodecopilot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"magicpodcast/internal/models"
	"magicpodcast/internal/processing"
	"magicpodcast/internal/utils"

	"gorm.io/gorm"
)

type GORMContextLoader struct {
	db             *gorm.DB
	artifactReader processing.ArtifactReader
}

func NewGORMContextLoader(
	db *gorm.DB,
	artifactReader processing.ArtifactReader,
) (*GORMContextLoader, error) {
	if db == nil || artifactReader == nil {
		return nil, fmt.Errorf(
			"%w: database and artifact reader are required",
			ErrContextUnavailable,
		)
	}
	return &GORMContextLoader{
		db:             db,
		artifactReader: artifactReader,
	}, nil
}

func (l *GORMContextLoader) Describe(
	ctx context.Context,
	episodeID uint,
) (ContextScope, error) {
	if episodeID == 0 {
		return ContextScope{}, ErrInvalidQuestion
	}
	var projection struct {
		ID                   uint
		ShowNotesAvailable   bool
		PrivateNoteAvailable bool
	}
	if err := l.db.WithContext(ctx).
		Model(&models.Episode{}).
		Select(
			"id",
			"CASE WHEN length(trim(show_notes)) > 0 THEN 1 ELSE 0 END AS show_notes_available",
			"CASE WHEN length(trim(notes)) > 0 THEN 1 ELSE 0 END AS private_note_available",
		).
		Where("id = ?", episodeID).
		Take(&projection).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ContextScope{}, ErrEpisodeNotFound
		}
		return ContextScope{}, fmt.Errorf("read episode copilot scope: %w", err)
	}
	var transcriptCount int64
	if err := l.db.WithContext(ctx).
		Model(&models.EpisodeArtifactSet{}).
		Where("episode_id = ? AND is_current = ?", episodeID, true).
		Count(&transcriptCount).Error; err != nil {
		return ContextScope{}, fmt.Errorf(
			"read episode copilot transcript scope: %w",
			err,
		)
	}
	return ContextScope{
		EpisodeID:            projection.ID,
		ShowNotesAvailable:   projection.ShowNotesAvailable,
		TranscriptAvailable:  transcriptCount > 0,
		PrivateNoteAvailable: projection.PrivateNoteAvailable,
	}, nil
}

func (l *GORMContextLoader) Load(
	ctx context.Context,
	episodeID uint,
	includePrivateNote bool,
) (EpisodeContext, error) {
	if episodeID == 0 {
		return EpisodeContext{}, ErrInvalidQuestion
	}
	var episode models.Episode
	fields := []string{"id", "podcast_id", "title", "show_notes"}
	if includePrivateNote {
		fields = append(fields, "notes")
	}
	query := l.db.WithContext(ctx).Select(fields).Preload("Podcast")
	if err := query.First(&episode, episodeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return EpisodeContext{}, ErrEpisodeNotFound
		}
		return EpisodeContext{}, fmt.Errorf("read episode copilot context: %w", err)
	}
	result := EpisodeContext{
		EpisodeID:    episode.ID,
		EpisodeTitle: episode.Title,
		PodcastTitle: episode.Podcast.Title,
		ShowNotes:    utils.HTMLToMarkdown(episode.ShowNotes),
	}
	if includePrivateNote {
		result.PrivateNotes = strings.TrimSpace(episode.Notes)
	}

	var artifact models.EpisodeArtifactSet
	err := l.db.WithContext(ctx).
		Where("episode_id = ? AND is_current = ?", episodeID, true).
		Order("created_at DESC, id DESC").
		First(&artifact).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return result, nil
	case err != nil:
		return EpisodeContext{}, fmt.Errorf(
			"read current episode artifact: %w",
			err,
		)
	}
	content, err := l.artifactReader.ReadText(ctx, artifact, "transcript")
	if err != nil {
		return EpisodeContext{}, fmt.Errorf(
			"%w: current transcript failed integrity validation",
			ErrContextUnavailable,
		)
	}
	result.Transcript = strings.TrimSpace(content.Content)
	return result, nil
}

var _ ContextLoader = (*GORMContextLoader)(nil)
