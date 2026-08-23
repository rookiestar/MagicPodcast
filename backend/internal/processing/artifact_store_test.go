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
