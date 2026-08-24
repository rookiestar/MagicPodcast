package episodecopilot

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
	"magicpodcast/internal/processing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGORMContextLoaderUsesOnlyCurrentEpisodeStateAndWorksReadOnly(
	t *testing.T,
) {
	db := openContextLoaderDatabase(t)
	podcast := models.Podcast{
		Title:        "Read-only podcast",
		FeedURL:      "https://example.com/read-only.xml",
		XYZID:        "episode-copilot-read-only",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID,
		Title:     "Current episode",
		GUID:      "episode-copilot-current",
		ShowNotes: "<p>Current <strong>Show Notes</strong>.</p>",
		Notes:     "Private launch note",
	}
	require.NoError(t, db.Create(&episode).Error)
	other := models.Episode{
		PodcastID: podcast.ID,
		Title:     "Other episode",
		GUID:      "episode-copilot-other",
	}
	require.NoError(t, db.Create(&other).Error)
	now := time.Date(2026, 8, 24, 21, 0, 0, 0, time.UTC)
	run := completedContextLoaderRun(episode.ID, "1", now)
	require.NoError(t, db.Create(&run).Error)
	otherRun := completedContextLoaderRun(other.ID, "4", now)
	require.NoError(t, db.Create(&otherRun).Error)
	artifact := models.EpisodeArtifactSet{
		RunID:            run.ID,
		EpisodeID:        episode.ID,
		PipelineVersion:  "pipeline-v1",
		RootPath:         "/managed/current",
		ManifestPath:     "manifest.json",
		ManifestSHA256:   strings.Repeat("1", 64),
		TranscriptSHA256: strings.Repeat("2", 64),
		NotesSHA256:      strings.Repeat("3", 64),
		IsCurrent:        true,
		CreatedAt:        now,
	}
	require.NoError(t, db.Create(&artifact).Error)
	otherArtifact := artifact
	otherArtifact.ID = 0
	otherArtifact.RunID = otherRun.ID
	otherArtifact.EpisodeID = other.ID
	otherArtifact.RootPath = "/managed/other"
	require.NoError(t, db.Create(&otherArtifact).Error)

	reader := &recordingArtifactReader{
		transcripts: map[uint]string{
			artifact.ID:      "[00:10] Current transcript.",
			otherArtifact.ID: "[00:20] Other transcript.",
		},
	}
	loader, err := NewGORMContextLoader(db, reader)
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA query_only = ON").Error)

	scope, err := loader.Describe(context.Background(), episode.ID)
	require.NoError(t, err)
	require.Equal(t, ContextScope{
		EpisodeID:            episode.ID,
		ShowNotesAvailable:   true,
		TranscriptAvailable:  true,
		PrivateNoteAvailable: true,
	}, scope)

	withoutPrivate, err := loader.Load(
		context.Background(),
		episode.ID,
		false,
	)
	require.NoError(t, err)
	require.Equal(t, "Read-only podcast", withoutPrivate.PodcastTitle)
	require.Equal(t, "Current **Show Notes**.", withoutPrivate.ShowNotes)
	require.Equal(t, "[00:10] Current transcript.", withoutPrivate.Transcript)
	require.Empty(t, withoutPrivate.PrivateNotes)
	require.Equal(t, []uint{artifact.ID}, reader.ReadArtifactIDs())

	withPrivate, err := loader.Load(context.Background(), episode.ID, true)
	require.NoError(t, err)
	require.Equal(t, "Private launch note", withPrivate.PrivateNotes)
	require.Equal(t, []uint{artifact.ID, artifact.ID}, reader.ReadArtifactIDs())

	runtime := newFakeRuntime(
		fakeExecution{
			result: json.RawMessage(
				`{"resources":[],"conflicts":[],"limitations":[]}`,
			),
		},
		fakeExecution{
			deltas: []string{"只读回答 [Show Notes L1-L1]。"},
			result: json.RawMessage(`{"text":"只读回答。"}`),
		},
	)
	service, err := NewService(loader, runtime, t.TempDir())
	require.NoError(t, err)
	events, err := service.Ask(context.Background(), QuestionRequest{
		EpisodeID: episode.ID,
		Question:  "总结当前单集",
	})
	require.NoError(t, err)
	for range events {
	}
}

func completedContextLoaderRun(
	episodeID uint,
	digestRune string,
	now time.Time,
) models.EpisodeProcessingRun {
	finishedAt := now
	return models.EpisodeProcessingRun{
		EpisodeID:       episodeID,
		ProcessingKey:   strings.Repeat(digestRune, 64),
		AudioDigest:     strings.Repeat(digestRune, 64),
		PipelineVersion: "pipeline-v1",
		TriggerSource:   models.ProcessingTriggerManual,
		Status:          models.ProcessingRunStatusCompleted,
		AttemptCount:    1,
		MaxAttempts:     3,
		RetryDeadlineAt: now.Add(time.Hour),
		StartedAt:       &now,
		FinishedAt:      &finishedAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func openContextLoaderDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "episode-copilot.db")
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("%s?_foreign_keys=on&_busy_timeout=5000", path)),
		&gorm.Config{},
	)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.ApplyMigrations(db))
	return db
}

type recordingArtifactReader struct {
	transcripts map[uint]string
	readIDs     []uint
}

func (r *recordingArtifactReader) ReadText(
	_ context.Context,
	artifact models.EpisodeArtifactSet,
	kind string,
) (processing.ArtifactContent, error) {
	if kind != "transcript" {
		return processing.ArtifactContent{}, processing.ErrInvalidArtifact
	}
	r.readIDs = append(r.readIDs, artifact.ID)
	return processing.ArtifactContent{
		Kind:    kind,
		Content: r.transcripts[artifact.ID],
		SHA256:  artifact.TranscriptSHA256,
	}, nil
}

func (r *recordingArtifactReader) ReadArtifactIDs() []uint {
	return append([]uint(nil), r.readIDs...)
}
