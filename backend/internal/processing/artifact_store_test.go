package processing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
)

func TestDiskArtifactStorePublishesCompleteImmutableSet(t *testing.T) {
	root := t.TempDir()
	store, err := NewDiskArtifactStore(root)
	require.NoError(t, err)
	request := artifactTestRequest(11)

	result, err := store.Publish(context.Background(), request)
	require.NoError(t, err)
	canonicalRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	require.Equal(
		t,
		filepath.Join(canonicalRoot, "episodes", "7", "sets", "run-11"),
		result.RootPath,
	)
	require.Len(t, result.ManifestSHA256, 64)
	require.Len(t, result.TranscriptSHA256, 64)
	require.Len(t, result.NotesSHA256, 64)

	for _, relative := range []string{
		"manifest.json",
		"transcript.md",
		"episode-notes.md",
		filepath.Join("raw", "minutes-transcript.json"),
	} {
		info, statErr := os.Stat(filepath.Join(result.RootPath, relative))
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	rawInfo, err := os.Stat(filepath.Join(result.RootPath, "raw"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), rawInfo.Mode().Perm())

	manifestBytes, err := os.ReadFile(filepath.Join(result.RootPath, "manifest.json"))
	require.NoError(t, err)
	var manifest artifactManifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	require.Equal(t, "1.0.0", manifest.SchemaVersion)
	require.Equal(t, request.RunID, manifest.RunID)
	require.Equal(t, request.EpisodeID, manifest.EpisodeID)
	require.Equal(t, request.TranscriptionAdapter, manifest.TranscriptionAdapter)
	require.Equal(t, request.TranscriptionVersion, manifest.TranscriptionVersion)
	require.Equal(t, request.RuntimeAdapter, manifest.RuntimeAdapter)
	require.Equal(t, request.RuntimeVersion, manifest.RuntimeVersion)
	require.Equal(t, request.PromptVersion, manifest.PromptVersion)
	require.Equal(t, request.SkillVersions, manifest.SkillVersions)
	require.Len(t, manifest.Files, 3)
	require.NotContains(t, string(manifestBytes), result.RootPath)

	_, err = store.Publish(context.Background(), request)
	require.ErrorIs(t, err, ErrArtifactExists)
	transcript, err := os.ReadFile(filepath.Join(result.RootPath, "transcript.md"))
	require.NoError(t, err)
	require.Equal(t, request.Transcript, string(transcript))
}

func TestDiskArtifactStorePublishesAndReadsNativeMinutesSet(t *testing.T) {
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	request := nativeArtifactTestRequest(13)

	published, err := store.Publish(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, published.AudioSHA256, 64)
	require.Len(t, published.MinutesSummarySHA256, 64)
	require.Len(t, published.TranscriptSHA256, 64)
	require.Len(t, published.TranscriptTimelineSHA256, 64)
	require.Empty(t, published.NotesSHA256)

	for _, relative := range []string{
		"manifest.json",
		"minutes-summary.md",
		"transcript.md",
		"transcript.json",
		filepath.Join("raw", "minutes-detail.json"),
	} {
		info, statErr := os.Stat(filepath.Join(published.RootPath, relative))
		require.NoError(t, statErr)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	_, err = os.Stat(filepath.Join(published.RootPath, "episode-notes.md"))
	require.True(t, os.IsNotExist(err))

	artifact := models.EpisodeArtifactSet{
		ID:                       1,
		RunID:                    request.RunID,
		EpisodeID:                request.EpisodeID,
		PipelineVersion:          request.PipelineVersion,
		RootPath:                 published.RootPath,
		ManifestPath:             published.ManifestPath,
		ManifestSHA256:           published.ManifestSHA256,
		AudioSHA256:              published.AudioSHA256,
		MinutesSummarySHA256:     published.MinutesSummarySHA256,
		TranscriptSHA256:         published.TranscriptSHA256,
		TranscriptTimelineSHA256: published.TranscriptTimelineSHA256,
	}
	summary, err := store.ReadText(context.Background(), artifact, "minutes_summary")
	require.NoError(t, err)
	require.Equal(t, request.MinutesSummary, summary.Content)
	require.Equal(t, published.MinutesSummarySHA256, summary.SHA256)
	transcript, err := store.ReadText(context.Background(), artifact, "transcript")
	require.NoError(t, err)
	require.Equal(t, request.TranscriptSegments, transcript.Segments)
	require.Equal(t, published.TranscriptTimelineSHA256, transcript.TimelineSHA256)
	require.False(t, transcript.MediaAvailable)

	require.NoError(t, os.WriteFile(
		filepath.Join(published.RootPath, "transcript.json"),
		[]byte(`{"tampered":true}`),
		0o600,
	))
	_, err = store.ReadText(context.Background(), artifact, "transcript")
	require.ErrorIs(t, err, ErrInvalidArtifact)
}

func TestDiskArtifactStorePublishesAndReadsOrderedVisualItems(t *testing.T) {
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	request := nativeArtifactTestRequest(131)
	first := mustTestPNG(t)
	second := mustTestPNG(t)
	firstHash := digestBytes(first)
	secondHash := digestBytes(second)
	request.MinutesEnrichment = MinutesEnrichment{
		VisualItems: []MinutesVisualItem{
			{
				Type: "image", MediaID: "image-1", MediaType: "image/png",
				Width: 2, Height: 2, SHA256: firstHash, Alt: "第一张",
			},
			{
				Type: "image", MediaID: "image-2", MediaType: "image/png",
				Width: 2, Height: 2, SHA256: secondHash, Alt: "第二张",
			},
		},
		InlineImages: []MinutesInlineImage{
			{MediaID: "image-1", Section: "summary", SectionStart: true},
			{MediaID: "image-2", Section: "decisions", AnchorText: "第二段", AnchorOccurrence: 1},
		},
	}
	request.VisualPreviews = []ManagedMinutesVisual{
		{Item: request.MinutesEnrichment.VisualItems[0], Bytes: first},
		{Item: request.MinutesEnrichment.VisualItems[1], Bytes: second},
	}

	published, err := store.Publish(context.Background(), request)
	require.NoError(t, err)
	artifact := models.EpisodeArtifactSet{
		ID:                       1,
		RunID:                    request.RunID,
		EpisodeID:                request.EpisodeID,
		PipelineVersion:          request.PipelineVersion,
		RootPath:                 published.RootPath,
		ManifestPath:             published.ManifestPath,
		ManifestSHA256:           published.ManifestSHA256,
		AudioSHA256:              published.AudioSHA256,
		MinutesSummarySHA256:     published.MinutesSummarySHA256,
		TranscriptSHA256:         published.TranscriptSHA256,
		TranscriptTimelineSHA256: published.TranscriptTimelineSHA256,
	}
	content, err := store.ReadText(context.Background(), artifact, "minutes_summary")
	require.NoError(t, err)
	require.Equal(t, []string{"image-1", "image-2"}, []string{
		content.VisualItems[0].MediaID,
		content.VisualItems[1].MediaID,
	})
	require.Equal(t, request.MinutesEnrichment.InlineImages, content.InlineImages)
	require.NotContains(t, string(mustJSON(t, content)), "filecn_")
	for _, visual := range content.VisualItems {
		media, mediaErr := store.ReadMedia(context.Background(), artifact, visual.MediaID)
		require.NoError(t, mediaErr)
		require.Equal(t, visual.MediaID, media.MediaID)
		require.Equal(t, visual.SHA256, media.SHA256)
	}
}

func TestDiskArtifactStoreRejectsOversizedGeneratedTimelineBeforePublish(t *testing.T) {
	root := t.TempDir()
	store, err := NewDiskArtifactStore(root)
	require.NoError(t, err)
	request := nativeArtifactTestRequest(14)
	request.TranscriptSegments[0].Text = strings.Repeat("x", maxArtifactTextBytes)

	_, err = store.Publish(context.Background(), request)
	require.ErrorIs(t, err, ErrInvalidArtifact)
	require.ErrorContains(t, err, "transcript.json exceeds the public read limit")
	var adapterErr *AdapterError
	require.ErrorAs(t, err, &adapterErr)
	require.Equal(t, artifactPublicReadLimitExceededCode, adapterErr.ErrorCode)
	require.False(t, adapterErr.CanRetry)

	finalPath := filepath.Join(root, "episodes", "7", "sets", "run-14")
	_, statErr := os.Stat(finalPath)
	require.True(t, os.IsNotExist(statErr))
	entries, err := os.ReadDir(filepath.Dir(finalPath))
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, strings.HasPrefix(entry.Name(), ".run-14-"))
	}
}

func TestDiskArtifactStoreRejectsTraversalWithoutPartialPublish(t *testing.T) {
	root := t.TempDir()
	store, err := NewDiskArtifactStore(root)
	require.NoError(t, err)
	request := artifactTestRequest(12)
	request.RawArtifacts = map[string][]byte{"../secret.txt": []byte("secret")}

	_, err = store.Publish(context.Background(), request)
	require.ErrorIs(t, err, ErrInvalidArtifact)
	finalPath := filepath.Join(root, "episodes", "7", "sets", "run-12")
	_, statErr := os.Stat(finalPath)
	require.True(t, os.IsNotExist(statErr))
	entries, err := os.ReadDir(filepath.Dir(finalPath))
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, strings.HasPrefix(entry.Name(), ".run-12-"))
	}
}

func TestDiskArtifactStoreFailurePreservesPreviousSuccess(t *testing.T) {
	root := t.TempDir()
	store, err := NewDiskArtifactStore(root)
	require.NoError(t, err)
	first := artifactTestRequest(21)
	firstResult, err := store.Publish(context.Background(), first)
	require.NoError(t, err)

	second := artifactTestRequest(22)
	second.EpisodeNotes = ""
	_, err = store.Publish(context.Background(), second)
	require.ErrorIs(t, err, ErrInvalidArtifact)

	firstTranscript, err := os.ReadFile(filepath.Join(firstResult.RootPath, "transcript.md"))
	require.NoError(t, err)
	require.Equal(t, first.Transcript, string(firstTranscript))
	_, statErr := os.Stat(filepath.Join(root, "episodes", "7", "sets", "run-22"))
	require.True(t, os.IsNotExist(statErr))
}

func TestDiskArtifactStoreHonorsCancellationBeforePublish(t *testing.T) {
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = store.Publish(ctx, artifactTestRequest(31))
	require.True(t, errors.Is(err, context.Canceled))
}

func TestDiskArtifactStoreDiscardsOnlyVerifiedOwnedSet(t *testing.T) {
	root := t.TempDir()
	store, err := NewDiskArtifactStore(root)
	require.NoError(t, err)
	published, err := store.Publish(context.Background(), artifactTestRequest(41))
	require.NoError(t, err)

	require.NoError(t, store.Discard(context.Background(), published))
	_, err = os.Stat(published.RootPath)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, store.Discard(context.Background(), published))

	outside := t.TempDir()
	invalid := published
	invalid.RootPath = outside
	require.ErrorIs(t, store.Discard(context.Background(), invalid), ErrInvalidArtifact)
	_, err = os.Stat(outside)
	require.NoError(t, err)
}

func TestDiskArtifactStoreRefusesToDiscardTamperedSet(t *testing.T) {
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	published, err := store.Publish(context.Background(), artifactTestRequest(42))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(published.RootPath, "manifest.json"),
		[]byte(`{"tampered":true}`),
		0o600,
	))

	require.ErrorIs(t, store.Discard(context.Background(), published), ErrInvalidArtifact)
	_, err = os.Stat(published.RootPath)
	require.NoError(t, err)
}

func TestDiskArtifactStoreReadsOnlyVerifiedNormalizedText(t *testing.T) {
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	request := artifactTestRequest(51)
	published, err := store.Publish(context.Background(), request)
	require.NoError(t, err)
	artifact := models.EpisodeArtifactSet{
		ID:               1,
		RunID:            request.RunID,
		EpisodeID:        request.EpisodeID,
		PipelineVersion:  request.PipelineVersion,
		RootPath:         published.RootPath,
		ManifestPath:     published.ManifestPath,
		ManifestSHA256:   published.ManifestSHA256,
		TranscriptSHA256: published.TranscriptSHA256,
		NotesSHA256:      published.NotesSHA256,
	}

	transcript, err := store.ReadText(context.Background(), artifact, "transcript")
	require.NoError(t, err)
	require.Equal(t, request.Transcript, transcript.Content)
	require.Equal(t, published.TranscriptSHA256, transcript.SHA256)
	notes, err := store.ReadText(context.Background(), artifact, "episode_notes")
	require.NoError(t, err)
	require.Equal(t, request.EpisodeNotes, notes.Content)

	_, err = store.ReadText(context.Background(), artifact, "../raw")
	require.ErrorIs(t, err, ErrInvalidArtifact)

	require.NoError(t, os.WriteFile(
		filepath.Join(published.RootPath, "transcript.md"),
		[]byte("tampered"),
		0o600,
	))
	_, err = store.ReadText(context.Background(), artifact, "transcript")
	require.ErrorIs(t, err, ErrInvalidArtifact)
}

func artifactTestRequest(runID uint) ArtifactPublishRequest {
	return ArtifactPublishRequest{
		RunID:                runID,
		EpisodeID:            7,
		AudioDigest:          strings.Repeat("a", 64),
		PipelineVersion:      "pipeline-v1",
		Transcript:           "# Transcript\n\n00:00 内容\n",
		EpisodeNotes:         "# Episode notes\n\n要点\n",
		TranscriptionAdapter: "fake-minutes",
		TranscriptionVersion: "fake-minutes-v1",
		RuntimeAdapter:       "fake-runtime",
		RuntimeVersion:       "fake-runtime-v1",
		PromptVersion:        "notes-v1",
		SkillVersions:        map[string]string{"minutes": "skill-v1"},
		Sources:              map[string]string{"episode": "https://example.com/episodes/7"},
		RawArtifacts: map[string][]byte{
			"minutes-transcript.json": []byte(`{"text":"内容"}`),
		},
		GeneratedAt: time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
	}
}

func nativeArtifactTestRequest(runID uint) ArtifactPublishRequest {
	return ArtifactPublishRequest{
		RunID:           runID,
		EpisodeID:       7,
		AudioDigest:     strings.Repeat("a", 64),
		PipelineVersion: NativeMinutesPipelineVersion,
		NativeMinutes:   true,
		MinutesSummary:  "# 纪要\n\n原生妙记总结\n",
		Transcript:      "# 逐字稿\n\n张三 00:00:00.195\n内容\n",
		TranscriptSegments: []TranscriptSegment{{
			Order: 1, Speaker: "张三", StartMS: 195, Text: "内容",
		}},
		TranscriptionAdapter: "fake-minutes",
		TranscriptionVersion: "fake-minutes-v1",
		SkillVersions:        map[string]string{"minutes": "skill-v1"},
		Sources:              map[string]string{"episode": "https://example.com/episodes/7"},
		RawArtifacts: map[string][]byte{
			"minutes-detail.json": []byte(`{"ok":true}`),
		},
		GeneratedAt: time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC),
	}
}
