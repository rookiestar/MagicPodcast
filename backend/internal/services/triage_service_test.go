package services

import (
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTriageService_SetDecision_IsIdempotentAndReversible(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.EpisodeTriageDecision{}))
	podcast := createDiscoveryPodcast(t, db, "个人播客")
	episode := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"待初筛单集",
		time.Date(2026, 7, 29, 8, 0, 0, 0, time.UTC),
		nil,
	)
	service := NewTriageService(db)

	shortlisted, err := service.SetDecision(episode.ID, models.TriageStateShortlisted)
	require.NoError(t, err)
	require.Equal(t, models.TriageStateShortlisted, shortlisted.State)
	firstDecisionTime := shortlisted.DecidedAt

	repeated, err := service.SetDecision(episode.ID, models.TriageStateShortlisted)
	require.NoError(t, err)
	assert.Equal(t, shortlisted.ID, repeated.ID)
	assert.Equal(t, firstDecisionTime, repeated.DecidedAt)

	var rowCount int64
	require.NoError(t, db.Model(&models.EpisodeTriageDecision{}).
		Where("episode_id = ?", episode.ID).
		Count(&rowCount).Error)
	assert.Equal(t, int64(1), rowCount)

	pending, err := service.SetDecision(episode.ID, models.TriageStatePending)
	require.NoError(t, err)
	assert.Equal(t, models.TriageStatePending, pending.State)
	assert.Equal(t, shortlisted.ID, pending.ID)

	discarded, err := service.SetDecision(episode.ID, models.TriageStateDiscarded)
	require.NoError(t, err)
	assert.Equal(t, models.TriageStateDiscarded, discarded.State)
}

func TestTriageService_SetDecision_RejectsInvalidStateAndMissingEpisode(t *testing.T) {
	db := setupDiscoveryTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.EpisodeTriageDecision{}))
	service := NewTriageService(db)

	_, err := service.SetDecision(12345, models.TriageStateShortlisted)
	require.ErrorIs(t, err, ErrTriageEpisodeNotFound)

	podcast := createDiscoveryPodcast(t, db, "个人播客")
	episode := createDiscoveryEpisode(
		t,
		db,
		podcast.ID,
		"待初筛单集",
		time.Now(),
		nil,
	)
	_, err = service.SetDecision(episode.ID, "unknown")
	require.ErrorIs(t, err, ErrInvalidTriageState)
}
