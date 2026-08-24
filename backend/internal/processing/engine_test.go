package processing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEngineRecoversExternalWaitPublishesAndKeepsDeliveryIndependent(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 13, 0, 0, 0, time.UTC)
	service := newProcessingService(db, WithClock(func() time.Time { return now }))
	episode := createProcessingEpisode(t, db, true, "engine-success")
	require.NoError(t, db.Model(&models.Episode{}).
		Where("id = ?", episode.ID).
		Updates(map[string]any{
			"link":       "https://example.com/episode",
			"show_notes": "<p>公开 Show Notes</p>",
			"notes":      "PRIVATE-NOTE",
		}).Error)
	completedAt := now.Add(-24 * time.Hour)
	require.NoError(t, db.Create(&models.EpisodeCompletion{
		EpisodeID:   episode.ID,
		CompletedAt: completedAt,
		CreatedAt:   completedAt,
		UpdatedAt:   completedAt,
	}).Error)
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)

	firstTranscriber := &fakeTranscriber{
		beginProgress: []TranscriptionProgress{{
			Status:     ExternalProgressWaiting,
			Checkpoint: json.RawMessage(`{"minute_ref":"restricted"}`),
		}},
	}
	firstEngine, err := NewEngine(
		service,
		firstTranscriber,
		&fakeRuntime{},
		store,
		nil,
	)
	require.NoError(t, err)
	waiting, err := firstEngine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusWaitingExternal, waiting.Status)

	recovery, err := service.RecoverNonTerminalRuns(context.Background(), now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, []uint{run.ID}, recovery.RecoverableRunIDs)
	require.Empty(t, recovery.FailedRunIDs)

	bridge := &fakeBridge{
		target:  "fake-knowledge",
		version: "fake-v1",
		err: NewAdapterError(
			"knowledge_rate_limited",
			"knowledge target is temporarily unavailable",
			true,
		),
	}
	secondTranscriber := &fakeTranscriber{
		resumeProgress: []TranscriptionProgress{{
			Status:        ExternalProgressCompleted,
			Checkpoint:    json.RawMessage(`{"minute_ref":"restricted","state":"complete"}`),
			Transcript:    "# Transcript\n\n[00:00] 可核对内容\n",
			RawArtifacts:  map[string][]byte{"minutes.json": []byte(`{"ok":true}`)},
			SourceRefs:    map[string]string{"episode": "https://example.com/episode"},
			SkillVersions: map[string]string{"lark-minutes": "fake-skill-v1"},
		}},
	}
	runtime := &fakeRuntime{result: RuntimeResult{
		EpisodeNotes:   "# Episode notes\n\n- 关键观点\n",
		RuntimeVersion: "fake-runtime-v1",
		PromptVersion:  "notes-v1",
		SkillVersions:  map[string]string{"episode-notes": "fake-skill-v1"},
	}}
	secondEngine, err := NewEngine(
		service,
		secondTranscriber,
		runtime,
		store,
		[]BridgeBinding{{Destination: "kb-test", Adapter: bridge}},
	)
	require.NoError(t, err)
	completed, err := secondEngine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCompleted, completed.Status)

	detail, err := service.GetProcessingRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.NotNil(t, detail.Artifact)
	require.True(t, detail.Artifact.IsCurrent)
	require.Len(t, detail.Deliveries, 1)
	require.Equal(t, models.DeliveryStatusFailed, detail.Deliveries[0].Status)
	require.Equal(t, models.ProcessingRunStatusCompleted, detail.Run.Status)
	deliveredPackage := bridge.LastPackage()
	require.Equal(t, episode.Title, deliveredPackage.EpisodeTitle)
	require.Equal(t, "Processing engine-success", deliveredPackage.PodcastTitle)
	require.Equal(t, "https://example.com/episode", deliveredPackage.SourceURL)
	require.Equal(t, "公开 Show Notes", deliveredPackage.ShowNotes)
	require.NotContains(t, deliveredPackage.ShowNotes, "PRIVATE-NOTE")
	for _, name := range []string{"manifest.json", "transcript.md", "episode-notes.md"} {
		_, statErr := os.Stat(filepath.Join(detail.Artifact.RootPath, name))
		require.NoError(t, statErr)
	}

	bridge.err = nil
	bridge.receipt = DeliveryReceipt{
		RemoteRef: "remote-1",
		PublicURL: "https://example.com/knowledge/remote-1",
	}
	pkg := KnowledgePackage{
		EpisodeID:       episode.ID,
		PipelineVersion: completed.PipelineVersion,
		ManifestSHA256:  detail.Artifact.ManifestSHA256,
		Transcript:      "# Transcript\n\n[00:00] 可核对内容\n",
		EpisodeNotes:    "# Episode notes\n\n- 关键观点\n",
	}
	require.NoError(t, secondEngine.deliver(
		context.Background(),
		*detail.Artifact,
		pkg,
		BridgeBinding{Destination: "kb-test", Adapter: bridge},
	))
	require.NoError(t, secondEngine.deliver(
		context.Background(),
		*detail.Artifact,
		pkg,
		BridgeBinding{Destination: "kb-test", Adapter: bridge},
	))
	detail, err = service.GetProcessingRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.Len(t, detail.Deliveries, 1)
	require.Equal(t, models.DeliveryStatusDelivered, detail.Deliveries[0].Status)
	require.Equal(t, 2, detail.Deliveries[0].AttemptCount)
	require.Equal(t, 2, bridge.CallCount())
	require.Len(t, bridge.DeliveryKeys(), 2)
	require.Equal(t, bridge.DeliveryKeys()[0], bridge.DeliveryKeys()[1])
	require.Len(t, bridge.DeliveryKeys()[0], 64)
	require.Equal(t, models.ProcessingRunStatusCompleted, detail.Run.Status)
	require.Zero(t, secondTranscriber.BeginCallCount())
	require.Equal(t, 1, secondTranscriber.ResumeCallCount())

	forceRequest := processingStartRequest(episode.ID)
	forceRequest.Force = true
	forced, err := service.StartEpisodeProcessing(context.Background(), forceRequest)
	require.NoError(t, err)
	failingEngine, err := NewEngine(
		service,
		&fakeTranscriber{},
		&fakeRuntime{err: NewAdapterError(
			"notes_generation_failed",
			"episode notes generation failed",
			false,
		)},
		store,
		nil,
	)
	require.NoError(t, err)
	failed, err := failingEngine.Advance(context.Background(), forced.Run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusFailed, failed.Status)
	var currentArtifacts []models.EpisodeArtifactSet
	require.NoError(t, db.Where("episode_id = ? AND is_current = ?", episode.ID, true).
		Find(&currentArtifacts).Error)
	require.Len(t, currentArtifacts, 1)
	require.Equal(t, detail.Artifact.ID, currentArtifacts[0].ID)
	var decision models.EpisodeTriageDecision
	require.NoError(t, db.Where("episode_id = ?", episode.ID).First(&decision).Error)
	require.NotNil(t, decision.QueueState)
	require.Equal(t, models.QueueStateFocus, *decision.QueueState)
	var completion models.EpisodeCompletion
	require.NoError(t, db.First(&completion, "episode_id = ?", episode.ID).Error)
	require.Equal(t, completedAt, completion.CompletedAt)
}

func TestNewEngineRejectsIncompleteAdapterIdentity(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)

	_, err = NewEngine(
		service,
		&fakeTranscriber{name: " "},
		&fakeRuntime{},
		store,
		nil,
	)
	require.ErrorContains(t, err, "transcription adapter identity")

	_, err = NewEngine(
		service,
		&fakeTranscriber{version: " "},
		&fakeRuntime{},
		store,
		nil,
	)
	require.ErrorContains(t, err, "transcription adapter identity")

	_, err = NewEngine(
		service,
		&fakeTranscriber{},
		&fakeRuntime{name: " "},
		store,
		nil,
	)
	require.ErrorContains(t, err, "runtime adapter identity")

	_, err = NewEngine(
		service,
		&fakeTranscriber{},
		&fakeRuntime{},
		store,
		[]BridgeBinding{{
			Destination: "knowledge",
			Adapter:     &fakeBridge{target: "", version: "v1"},
		}},
	)
	require.ErrorContains(t, err, "knowledge bridge identity")

	bridge := &fakeBridge{target: "knowledge", version: "v1"}
	_, err = NewEngine(
		service,
		&fakeTranscriber{},
		&fakeRuntime{},
		store,
		[]BridgeBinding{
			{Destination: "destination", Adapter: bridge},
			{Destination: "destination", Adapter: bridge},
		},
	)
	require.ErrorContains(t, err, "knowledge bridge identity is duplicated")
}

func TestEngineUnknownExternalResultFailsClosed(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "engine-unknown")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	engine, err := NewEngine(
		service,
		&fakeTranscriber{
			beginProgress: []TranscriptionProgress{{
				Status:     ExternalProgressUnknown,
				Checkpoint: json.RawMessage(`{"remote_ref":"known-but-result-unknown"}`),
			}},
		},
		&fakeRuntime{},
		store,
		nil,
	)
	require.NoError(t, err)

	failed, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusFailed, failed.Status)
	require.Equal(t, "external_result_unknown", failed.ErrorCode)
	require.False(t, failed.ErrorRetryable)
	checkpoint, err := engine.loadCheckpoint(context.Background(), run.ID, StepTranscription)
	require.NoError(t, err)
	require.True(t, checkpointIsValid(checkpoint))
	require.Equal(t, ExternalProgressWaiting, checkpoint.Status)
}

func TestEngineInvalidCompletedCheckpointFailsClosedWithoutStuckRun(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "engine-invalid-completed-checkpoint")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	engine, err := NewEngine(
		service,
		&fakeTranscriber{
			beginProgress: []TranscriptionProgress{{
				Status:     ExternalProgressCompleted,
				Checkpoint: json.RawMessage(`{"invalid"`),
				Transcript: "# Transcript\n",
			}},
		},
		&fakeRuntime{},
		store,
		nil,
	)
	require.NoError(t, err)

	failed, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusFailed, failed.Status)
	require.Equal(t, "invalid_external_checkpoint", failed.ErrorCode)
	require.Empty(t, failed.CurrentStep)
}

func TestEngineRedactsUnclassifiedAdapterErrors(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "engine-safe-error")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	engine, err := NewEngine(
		service,
		&fakeTranscriber{
			beginErrors: []error{errors.New("provider token TOP-SECRET leaked")},
		},
		&fakeRuntime{},
		store,
		nil,
	)
	require.NoError(t, err)

	failed, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusFailed, failed.Status)
	require.Equal(t, "adapter_error", failed.ErrorCode)
	require.Equal(t, "processing adapter failed", failed.ErrorMessage)
	require.NotContains(t, failed.ErrorMessage, "TOP-SECRET")
}

func TestEngineRejectsCheckpointFromDifferentAdapterVersion(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "engine-adapter-mismatch")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	first, err := NewEngine(
		service,
		&fakeTranscriber{beginProgress: []TranscriptionProgress{{
			Status:     ExternalProgressWaiting,
			Checkpoint: json.RawMessage(`{"minute_ref":"restricted"}`),
		}}},
		&fakeRuntime{},
		store,
		nil,
	)
	require.NoError(t, err)
	waiting, err := first.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusWaitingExternal, waiting.Status)

	replacement := &fakeTranscriber{version: "fake-minutes-v2"}
	second, err := NewEngine(service, replacement, &fakeRuntime{}, store, nil)
	require.NoError(t, err)
	failed, err := second.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusFailed, failed.Status)
	require.Equal(t, "checkpoint_adapter_mismatch", failed.ErrorCode)
	require.Zero(t, replacement.ResumeCallCount())
}

func TestEngineRetryIsBoundedByAttempts(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 14, 0, 0, 0, time.UTC)
	service := newProcessingService(
		db,
		WithClock(func() time.Time { return now }),
		WithRetryPolicy(RetryPolicy{
			MaxAttempts: 3,
			MaxElapsed:  time.Hour,
			BaseDelay:   0,
		}),
	)
	episode := createProcessingEpisode(t, db, true, "engine-retry")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	retryErr := NewAdapterError("temporary_network", "temporary network failure", true)
	transcriber := &fakeTranscriber{
		beginErrors: []error{retryErr, retryErr, retryErr},
	}
	engine, err := NewEngine(service, transcriber, &fakeRuntime{}, store, nil)
	require.NoError(t, err)

	first, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusQueued, first.Status)
	require.Equal(t, 1, first.AttemptCount)
	require.True(t, first.ErrorRetryable)

	second, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusQueued, second.Status)
	require.Equal(t, 2, second.AttemptCount)

	third, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusFailed, third.Status)
	require.Equal(t, 3, third.AttemptCount)
	require.Equal(t, "temporary_network", third.ErrorCode)
	require.Equal(t, 3, transcriber.BeginCallCount())
}

func TestEngineRetryDelayUsesBoundedDeterministicJitter(t *testing.T) {
	engine := &Engine{service: &Service{retryPolicy: RetryPolicy{BaseDelay: 10 * time.Second}}}

	first := engine.retryDelay(41, 1)
	require.Equal(t, first, engine.retryDelay(41, 1))
	require.GreaterOrEqual(t, first, 10*time.Second)
	require.LessOrEqual(t, first, 12*time.Second)

	second := engine.retryDelay(41, 2)
	require.GreaterOrEqual(t, second, 20*time.Second)
	require.LessOrEqual(t, second, 24*time.Second)

	capped := &Engine{service: &Service{retryPolicy: RetryPolicy{BaseDelay: time.Hour}}}
	require.Equal(t, time.Hour, capped.retryDelay(41, 1))
}

func TestEngineRetryPreservesExternalCheckpointWithoutRecreatingRequest(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 14, 15, 0, 0, time.UTC)
	service := newProcessingService(
		db,
		WithClock(func() time.Time { return now }),
		WithRetryPolicy(RetryPolicy{
			MaxAttempts: 3,
			MaxElapsed:  time.Hour,
			BaseDelay:   0,
		}),
	)
	episode := createProcessingEpisode(t, db, true, "engine-resume-retry")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	transcriber := &fakeTranscriber{
		beginProgress: []TranscriptionProgress{{
			Status:     ExternalProgressWaiting,
			Checkpoint: json.RawMessage(`{"remote_ref":"stable"}`),
		}},
		resumeErrors: []error{
			NewAdapterError("temporary_network", "temporary network failure", true),
		},
	}
	engine, err := NewEngine(service, transcriber, &fakeRuntime{}, store, nil)
	require.NoError(t, err)

	waiting, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusWaitingExternal, waiting.Status)

	retrying, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusWaitingExternal, retrying.Status)
	require.Equal(t, 1, retrying.AttemptCount)
	require.NotNil(t, retrying.NextAttemptAt)
	require.Equal(t, 1, transcriber.BeginCallCount())
	require.Equal(t, 1, transcriber.ResumeCallCount())

	checkpoint, err := engine.loadCheckpoint(context.Background(), run.ID, StepTranscription)
	require.NoError(t, err)
	require.True(t, checkpointIsValid(checkpoint))
	require.JSONEq(t, `{"remote_ref":"stable"}`, checkpoint.StateJSON)
}

func TestEngineRuntimeRetryReusesCompletedTranscriptionCheckpoint(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 14, 20, 0, 0, time.UTC)
	service := newProcessingService(
		db,
		WithClock(func() time.Time { return now }),
		WithRetryPolicy(RetryPolicy{
			MaxAttempts: 3,
			MaxElapsed:  time.Hour,
			BaseDelay:   0,
		}),
	)
	episode := createProcessingEpisode(t, db, true, "engine-runtime-retry")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	transcriber := &fakeTranscriber{
		beginProgress: []TranscriptionProgress{{
			Status:     ExternalProgressWaiting,
			Checkpoint: json.RawMessage(`{"remote_ref":"stable"}`),
		}},
		resumeProgress: []TranscriptionProgress{{
			Status:     ExternalProgressCompleted,
			Transcript: "# Transcript\n",
		}},
	}
	runtime := &fakeRuntime{
		err: NewAdapterError("runtime_busy", "runtime is temporarily busy", true),
	}
	engine, err := NewEngine(service, transcriber, runtime, store, nil)
	require.NoError(t, err)

	waiting, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusWaitingExternal, waiting.Status)

	retrying, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusWaitingExternal, retrying.Status)
	require.Equal(t, 1, transcriber.BeginCallCount())
	require.Equal(t, 1, transcriber.ResumeCallCount())
	checkpoint, err := engine.loadCheckpoint(context.Background(), run.ID, StepTranscription)
	require.NoError(t, err)
	require.True(t, checkpointIsValid(checkpoint))
	require.Equal(t, ExternalProgressCompleted, checkpoint.Status)
}

func TestEngineRejectsCompletedTranscriptionWithoutRecoverableCheckpoint(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "engine-missing-completed-checkpoint")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	transcriber := &fakeTranscriber{
		beginProgress: []TranscriptionProgress{{
			Status:     ExternalProgressCompleted,
			Transcript: "# Transcript\n",
		}},
	}
	engine, err := NewEngine(service, transcriber, &fakeRuntime{}, store, nil)
	require.NoError(t, err)

	failed, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusFailed, failed.Status)
	require.Equal(t, "missing_completed_checkpoint", failed.ErrorCode)
	require.False(t, failed.ErrorRetryable)
	require.Equal(t, 1, transcriber.BeginCallCount())

	terminal, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusFailed, terminal.Status)
	require.Equal(t, 1, transcriber.BeginCallCount())
}

func TestEngineContextCancellationCannotLeaveRunStuckRunning(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(
		db,
		WithRetryPolicy(RetryPolicy{
			MaxAttempts: 3,
			MaxElapsed:  time.Hour,
			BaseDelay:   0,
		}),
	)
	episode := createProcessingEpisode(t, db, true, "engine-context-cancel")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	transcriber := &fakeTranscriber{
		onBegin: cancel,
		beginProgress: []TranscriptionProgress{{
			Status:     ExternalProgressCompleted,
			Checkpoint: json.RawMessage(`{"transcript_ref":"cancelled-context"}`),
			Transcript: "# Transcript\n",
		}},
	}
	engine, err := NewEngine(service, transcriber, &fakeRuntime{}, store, nil)
	require.NoError(t, err)

	retrying, err := engine.Advance(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusWaitingExternal, retrying.Status)
	require.Equal(t, "processing_state_update_failed", retrying.ErrorCode)
	require.True(t, retrying.ErrorRetryable)
	require.Equal(t, StepTranscription, retrying.CurrentStep)
}

func TestEngineDoesNotCompleteWhenKnowledgePackageLoadFails(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "knowledge-load-failure")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	bridge := &fakeBridge{target: "fake-knowledge", version: "fake-v1"}
	engine, err := NewEngine(
		service,
		&fakeTranscriber{},
		&fakeRuntime{},
		store,
		[]BridgeBinding{{Destination: "kb-test", Adapter: bridge}},
	)
	require.NoError(t, err)

	callbackName := "test:fail_knowledge_package_episode_query"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(
		callbackName,
		func(tx *gorm.DB) {
			if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "episodes" {
				tx.AddError(errors.New("injected episode query failure"))
			}
		},
	))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
	})

	retrying, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusWaitingExternal, retrying.Status)
	require.Equal(t, "knowledge_package_load_failed", retrying.ErrorCode)
	require.True(t, retrying.ErrorRetryable)
	require.Zero(t, bridge.CallCount())

	var artifactCount int64
	require.NoError(t, db.Model(&models.EpisodeArtifactSet{}).
		Where("run_id = ?", run.ID).
		Count(&artifactCount).Error)
	require.Zero(t, artifactCount)
	var deliveryCount int64
	require.NoError(t, db.Model(&models.KnowledgeDelivery{}).
		Count(&deliveryCount).Error)
	require.Zero(t, deliveryCount)
}

func TestEngineUsesEnclosureSourceWhenEpisodeLinkIsMissing(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "knowledge-source-fallback")
	enclosureURL := "https://media.example.com/episode.mp3"
	require.NoError(t, db.Model(&models.Episode{}).
		Where("id = ?", episode.ID).
		Updates(map[string]any{
			"link":       "",
			"medium_url": enclosureURL,
		}).Error)
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	bridge := &fakeBridge{target: "fake-knowledge", version: "fake-v1"}
	engine, err := NewEngine(
		service,
		&fakeTranscriber{},
		&fakeRuntime{},
		store,
		[]BridgeBinding{{Destination: "kb-test", Adapter: bridge}},
	)
	require.NoError(t, err)

	completed, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCompleted, completed.Status)
	pkg := bridge.LastPackage()
	require.Equal(t, enclosureURL, pkg.SourceURL)
	require.Equal(t, enclosureURL, pkg.Sources["episode"])
}

func TestEngineRetryIsBoundedByTotalElapsedTime(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 14, 30, 0, 0, time.UTC)
	service := newProcessingService(
		db,
		WithClock(func() time.Time { return now }),
		WithRetryPolicy(RetryPolicy{
			MaxAttempts: 5,
			MaxElapsed:  time.Second,
			BaseDelay:   0,
		}),
	)
	episode := createProcessingEpisode(t, db, true, "engine-deadline")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	transcriber := &fakeTranscriber{
		beginErrors: []error{
			NewAdapterError("temporary_network", "temporary network failure", true),
		},
	}
	engine, err := NewEngine(service, transcriber, &fakeRuntime{}, store, nil)
	require.NoError(t, err)

	retrying, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusQueued, retrying.Status)
	now = now.Add(2 * time.Second)
	failed, err := engine.Advance(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusFailed, failed.Status)
	require.Equal(t, "retry_exhausted", failed.ErrorCode)
	require.Empty(t, failed.CurrentStep)
	require.Nil(t, failed.NextAttemptAt)
	require.Equal(t, 1, transcriber.BeginCallCount())
}

func TestEngineCancellationCannotBeOverwrittenByWorker(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "engine-cancel")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	transcriber := &fakeTranscriber{
		beginProgress: []TranscriptionProgress{{
			Status:     ExternalProgressCompleted,
			Checkpoint: json.RawMessage(`{"transcript_ref":"cancel-race"}`),
			Transcript: "# Transcript\n",
		}},
	}
	runtime := newBlockingFakeRuntime()
	engine, err := NewEngine(service, transcriber, runtime, store, nil)
	require.NoError(t, err)

	type advanceResult struct {
		run models.EpisodeProcessingRun
		err error
	}
	finished := make(chan advanceResult, 1)
	go func() {
		advanced, advanceErr := engine.Advance(context.Background(), run.ID)
		finished <- advanceResult{run: advanced, err: advanceErr}
	}()
	select {
	case <-runtime.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("runtime did not start")
	}

	cancelled, err := engine.Cancel(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCancelled, cancelled.Status)
	select {
	case result := <-finished:
		require.NoError(t, result.err)
		require.Equal(t, models.ProcessingRunStatusCancelled, result.run.Status)
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
	require.True(t, runtime.WasCancelled())
	require.True(t, transcriber.WasCancelled())

	var artifactCount int64
	require.NoError(t, db.Model(&models.EpisodeArtifactSet{}).
		Where("run_id = ?", run.ID).
		Count(&artifactCount).Error)
	require.Zero(t, artifactCount)
}

func TestEngineCancellationPersistsExternalContinuationNotice(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "engine-cancel-external")
	run := startProcessingRun(t, service, episode.ID)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	transcriber := &fakeTranscriber{
		cancellationDisposition: TranscriptionCancellationDisposition{
			RemoteMayContinue: true,
			Message:           "已取消本机加工；飞书端任务可能继续，已创建的远端资源会保留。",
		},
	}
	engine, err := NewEngine(service, transcriber, &fakeRuntime{}, store, nil)
	require.NoError(t, err)

	cancelled, err := engine.Cancel(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCancelled, cancelled.Status)
	require.Equal(t, cancellationExternalResultUnknown, cancelled.ErrorCode)
	require.Contains(t, cancelled.ErrorMessage, "飞书端任务可能继续")
	require.False(t, cancelled.ErrorRetryable)
	_, err = service.RetryProcessingRun(context.Background(), run.ID)
	require.ErrorIs(t, err, ErrRetryUnsafe)
}

func TestEngineCancellationRecordsAdapterAndRuntimeCancelFailures(t *testing.T) {
	t.Run("transcriber", func(t *testing.T) {
		db := openProcessingTestDB(t)
		service := newProcessingService(db)
		episode := createProcessingEpisode(t, db, true, "engine-cancel-transcriber-failure")
		run := startProcessingRun(t, service, episode.ID)
		store, err := NewDiskArtifactStore(t.TempDir())
		require.NoError(t, err)
		engine, err := NewEngine(
			service,
			&fakeTranscriber{cancelErr: errors.New("remote cancellation unavailable")},
			&fakeRuntime{},
			store,
			nil,
		)
		require.NoError(t, err)

		cancelled, err := engine.Cancel(context.Background(), run.ID)
		require.NoError(t, err)
		require.Equal(t, cancellationExternalResultUnknown, cancelled.ErrorCode)
		require.Contains(t, cancelled.ErrorMessage, "外部转写状态无法确认")
	})

	t.Run("runtime", func(t *testing.T) {
		db := openProcessingTestDB(t)
		service := newProcessingService(db)
		episode := createProcessingEpisode(t, db, true, "engine-cancel-runtime-failure")
		run := startProcessingRun(t, service, episode.ID)
		store, err := NewDiskArtifactStore(t.TempDir())
		require.NoError(t, err)
		engine, err := NewEngine(
			service,
			&fakeTranscriber{},
			&fakeRuntime{cancelErr: errors.New("runtime cancellation unavailable")},
			store,
			nil,
		)
		require.NoError(t, err)

		cancelled, err := engine.Cancel(context.Background(), run.ID)
		require.NoError(t, err)
		require.Equal(t, cancellationRuntimeResultUnknown, cancelled.ErrorCode)
		require.Contains(t, cancelled.ErrorMessage, "Codex Runtime")
	})
}

func TestEngineCancellationAfterFilePublishDiscardsUnrecordedSet(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "engine-cancel-after-publish")
	run := startProcessingRun(t, service, episode.ID)
	diskStore, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	store := &postPublishBlockingStore{
		delegate:  diskStore,
		published: make(chan ArtifactPublishResult, 1),
		release:   make(chan struct{}),
	}
	engine, err := NewEngine(
		service,
		&fakeTranscriber{},
		&fakeRuntime{},
		store,
		nil,
	)
	require.NoError(t, err)

	type advanceResult struct {
		run models.EpisodeProcessingRun
		err error
	}
	finished := make(chan advanceResult, 1)
	go func() {
		advanced, advanceErr := engine.Advance(context.Background(), run.ID)
		finished <- advanceResult{run: advanced, err: advanceErr}
	}()

	var published ArtifactPublishResult
	select {
	case published = <-store.published:
	case <-time.After(3 * time.Second):
		t.Fatal("artifact store did not publish")
	}
	cancelled, err := engine.Cancel(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCancelled, cancelled.Status)
	close(store.release)

	select {
	case result := <-finished:
		require.NoError(t, result.err)
		require.Equal(t, models.ProcessingRunStatusCancelled, result.run.Status)
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not reconcile published artifact")
	}
	_, err = os.Stat(published.RootPath)
	require.True(t, os.IsNotExist(err))
	var artifactCount int64
	require.NoError(t, db.Model(&models.EpisodeArtifactSet{}).
		Where("run_id = ?", run.ID).
		Count(&artifactCount).Error)
	require.Zero(t, artifactCount)
}

func TestEngineExternalWaitCheckpointAndStatusCommitAtomically(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	episode := createProcessingEpisode(t, db, true, "engine-wait-race")
	run := startProcessingRun(t, service, episode.ID)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", run.ID).
		Update("status", models.ProcessingRunStatusRunning).Error)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	engine, err := NewEngine(service, &fakeTranscriber{}, &fakeRuntime{}, store, nil)
	require.NoError(t, err)

	cancelled, err := service.CancelProcessingRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCancelled, cancelled.Status)

	current, err := engine.persistExternalWait(
		context.Background(),
		run.ID,
		json.RawMessage(`{"minute_ref":"must-not-commit"}`),
	)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCancelled, current.Status)
	var checkpointCount int64
	require.NoError(t, db.Model(&models.ProcessingCheckpoint{}).
		Where("run_id = ?", run.ID).
		Count(&checkpointCount).Error)
	require.Zero(t, checkpointCount)
}

func TestEngineClaimsEachDurableRunOnlyOnceAcrossInstances(t *testing.T) {
	db := openProcessingTestDB(t)
	service := newProcessingService(db)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	first, err := NewEngine(service, &fakeTranscriber{}, &fakeRuntime{}, store, nil)
	require.NoError(t, err)
	second, err := NewEngine(service, &fakeTranscriber{}, &fakeRuntime{}, store, nil)
	require.NoError(t, err)

	queuedEpisode := createProcessingEpisode(t, db, true, "engine-queued-claim")
	queued := startProcessingRun(t, service, queuedEpisode.ID)
	claimed, err := first.beginQueuedAttempt(context.Background(), queued.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusRunning, claimed.Status)

	duplicate, err := second.beginQueuedAttempt(context.Background(), queued.ID)
	require.ErrorIs(t, err, ErrRunBusy)
	require.Equal(t, models.ProcessingRunStatusRunning, duplicate.Status)

	waitingEpisode := createProcessingEpisode(t, db, true, "engine-waiting-claim")
	waiting := startProcessingRun(t, service, waitingEpisode.ID)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", waiting.ID).
		Update("status", models.ProcessingRunStatusWaitingExternal).Error)

	claimed, err = first.claimExternalWait(context.Background(), waiting.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusRunning, claimed.Status)

	duplicate, err = second.claimExternalWait(context.Background(), waiting.ID)
	require.ErrorIs(t, err, ErrRunBusy)
	require.Equal(t, models.ProcessingRunStatusRunning, duplicate.Status)
}

func TestEngineKnowledgeDeliveriesAreIndependentPerTargetAndDestination(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC)
	service := newProcessingService(db, WithClock(func() time.Time { return now }))
	episode := createProcessingEpisode(t, db, true, "independent-deliveries")
	run := startProcessingRun(t, service, episode.ID)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", run.ID).
		Updates(map[string]any{
			"status":      models.ProcessingRunStatusCompleted,
			"finished_at": now,
			"updated_at":  now,
		}).Error)
	artifact := models.EpisodeArtifactSet{
		RunID:            run.ID,
		EpisodeID:        episode.ID,
		PipelineVersion:  run.PipelineVersion,
		RootPath:         "/managed/artifacts",
		ManifestPath:     "manifest.json",
		ManifestSHA256:   strings.Repeat("1", 64),
		TranscriptSHA256: strings.Repeat("2", 64),
		NotesSHA256:      strings.Repeat("3", 64),
		IsCurrent:        true,
		CreatedAt:        now,
	}
	require.NoError(t, db.Create(&artifact).Error)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	engine, err := NewEngine(service, &fakeTranscriber{}, &fakeRuntime{}, store, nil)
	require.NoError(t, err)
	failing := &fakeBridge{
		target:  "gemini",
		version: "fake-gemini-v1",
		err: NewAdapterError(
			"knowledge_rate_limited",
			"knowledge target is temporarily unavailable",
			true,
		),
	}
	successful := &fakeBridge{
		target:  "ima",
		version: "fake-ima-v1",
		receipt: DeliveryReceipt{RemoteRef: "package-1"},
	}
	pkg := KnowledgePackage{
		EpisodeID:       episode.ID,
		PipelineVersion: run.PipelineVersion,
		ManifestSHA256:  artifact.ManifestSHA256,
		Transcript:      "# Transcript\n",
		EpisodeNotes:    "# Episode notes\n",
	}

	require.NoError(t, engine.deliver(
		context.Background(),
		artifact,
		pkg,
		BridgeBinding{Destination: "notebook-a", Adapter: failing},
	))
	require.NoError(t, engine.deliver(
		context.Background(),
		artifact,
		pkg,
		BridgeBinding{Destination: "manual-package", Adapter: successful},
	))

	detail, err := service.GetProcessingRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.Equal(t, models.ProcessingRunStatusCompleted, detail.Run.Status)
	require.Len(t, detail.Deliveries, 2)
	statusByTarget := make(map[string]string, len(detail.Deliveries))
	keyByTarget := make(map[string]string, len(detail.Deliveries))
	for _, delivery := range detail.Deliveries {
		statusByTarget[delivery.Target] = delivery.Status
		keyByTarget[delivery.Target] = delivery.DeliveryKey
	}
	require.Equal(t, models.DeliveryStatusFailed, statusByTarget["gemini"])
	require.Equal(t, models.DeliveryStatusDelivered, statusByTarget["ima"])
	require.NotEqual(t, keyByTarget["gemini"], keyByTarget["ima"])
}

func TestEnginePersistsSuccessfulDeliveryAfterContextCancellation(t *testing.T) {
	db := openProcessingTestDB(t)
	now := time.Date(2026, 8, 24, 16, 30, 0, 0, time.UTC)
	service := newProcessingService(db, WithClock(func() time.Time { return now }))
	episode := createProcessingEpisode(t, db, true, "delivery-success-after-cancel")
	run := startProcessingRun(t, service, episode.ID)
	require.NoError(t, db.Model(&models.EpisodeProcessingRun{}).
		Where("id = ?", run.ID).
		Updates(map[string]any{
			"status":      models.ProcessingRunStatusCompleted,
			"finished_at": now,
			"updated_at":  now,
		}).Error)
	artifact := models.EpisodeArtifactSet{
		RunID:            run.ID,
		EpisodeID:        episode.ID,
		PipelineVersion:  run.PipelineVersion,
		RootPath:         "/managed/artifacts",
		ManifestPath:     "manifest.json",
		ManifestSHA256:   strings.Repeat("1", 64),
		TranscriptSHA256: strings.Repeat("2", 64),
		NotesSHA256:      strings.Repeat("3", 64),
		IsCurrent:        true,
		CreatedAt:        now,
	}
	require.NoError(t, db.Create(&artifact).Error)
	store, err := NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	engine, err := NewEngine(service, &fakeTranscriber{}, &fakeRuntime{}, store, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	bridge := &fakeBridge{
		target:       "gemini",
		version:      "fake-gemini-v1",
		receipt:      DeliveryReceipt{RemoteRef: "source-1"},
		afterDeliver: cancel,
	}

	require.NoError(t, engine.deliver(
		ctx,
		artifact,
		KnowledgePackage{
			EpisodeID:       episode.ID,
			PipelineVersion: run.PipelineVersion,
			ManifestSHA256:  artifact.ManifestSHA256,
			Transcript:      "# Transcript\n",
			EpisodeNotes:    "# Episode notes\n",
		},
		BridgeBinding{Destination: "notebook-a", Adapter: bridge},
	))

	detail, err := service.GetProcessingRun(context.Background(), run.ID)
	require.NoError(t, err)
	require.Len(t, detail.Deliveries, 1)
	require.Equal(t, models.DeliveryStatusDelivered, detail.Deliveries[0].Status)
	require.Equal(t, "source-1", detail.Deliveries[0].RemoteRef)
	require.NotNil(t, detail.Deliveries[0].DeliveredAt)
}

type fakeTranscriber struct {
	mu                         sync.Mutex
	name                       string
	version                    string
	beginProgress              []TranscriptionProgress
	beginErrors                []error
	resumeProgress             []TranscriptionProgress
	resumeErrors               []error
	beginCalls                 int
	resumeCalls                int
	cancelled                  bool
	cancelErr                  error
	cancellationDisposition    TranscriptionCancellationDisposition
	cancellationDispositionErr error
	onBegin                    func()
}

func (f *fakeTranscriber) Name() string {
	if f.name == "" {
		return "fake-minutes"
	}
	return f.name
}
func (f *fakeTranscriber) Version() string {
	if f.version == "" {
		return "fake-minutes-v1"
	}
	return f.version
}

func (f *fakeTranscriber) Begin(
	_ context.Context,
	_ TranscriptionRequest,
) (TranscriptionProgress, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index := f.beginCalls
	f.beginCalls++
	onBegin := f.onBegin
	if index < len(f.beginErrors) && f.beginErrors[index] != nil {
		return TranscriptionProgress{}, f.beginErrors[index]
	}
	if index < len(f.beginProgress) {
		if onBegin != nil {
			onBegin()
		}
		return f.beginProgress[index], nil
	}
	if onBegin != nil {
		onBegin()
	}
	return TranscriptionProgress{
		Status:     ExternalProgressCompleted,
		Checkpoint: json.RawMessage(`{"transcript_ref":"default"}`),
		Transcript: "# Transcript\n",
	}, nil
}

func (f *fakeTranscriber) Resume(
	_ context.Context,
	_ TranscriptionRequest,
	_ json.RawMessage,
) (TranscriptionProgress, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index := f.resumeCalls
	f.resumeCalls++
	if index < len(f.resumeErrors) && f.resumeErrors[index] != nil {
		return TranscriptionProgress{}, f.resumeErrors[index]
	}
	if index < len(f.resumeProgress) {
		return f.resumeProgress[index], nil
	}
	return TranscriptionProgress{
		Status:     ExternalProgressCompleted,
		Transcript: "# Transcript\n",
	}, nil
}

func (f *fakeTranscriber) Cancel(
	_ context.Context,
	_ uint,
	_ json.RawMessage,
) error {
	f.mu.Lock()
	f.cancelled = true
	err := f.cancelErr
	f.mu.Unlock()
	return err
}

func (f *fakeTranscriber) CancellationDisposition(
	json.RawMessage,
) (TranscriptionCancellationDisposition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cancellationDisposition, f.cancellationDispositionErr
}

func (f *fakeTranscriber) BeginCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.beginCalls
}

func (f *fakeTranscriber) ResumeCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resumeCalls
}

func (f *fakeTranscriber) WasCancelled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cancelled
}

type fakeRuntime struct {
	mu        sync.Mutex
	name      string
	result    RuntimeResult
	err       error
	cancelErr error
	entered   chan struct{}
	block     bool
	cancelled bool
	enterOnce sync.Once
}

func newBlockingFakeRuntime() *fakeRuntime {
	return &fakeRuntime{
		entered: make(chan struct{}),
		block:   true,
	}
}

func (f *fakeRuntime) Name() string {
	if f.name == "" {
		return "fake-runtime"
	}
	return f.name
}

func (f *fakeRuntime) Execute(
	ctx context.Context,
	_ RuntimeRequest,
) (RuntimeResult, error) {
	if f.entered != nil {
		f.enterOnce.Do(func() { close(f.entered) })
	}
	if f.block {
		<-ctx.Done()
		return RuntimeResult{}, ctx.Err()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return RuntimeResult{}, f.err
	}
	result := f.result
	if result.EpisodeNotes == "" {
		result.EpisodeNotes = "# Episode notes\n"
	}
	if result.RuntimeVersion == "" {
		result.RuntimeVersion = "fake-runtime-v1"
	}
	if result.PromptVersion == "" {
		result.PromptVersion = "fake-prompt-v1"
	}
	return result, nil
}

func (f *fakeRuntime) Cancel(_ context.Context, _ uint) error {
	f.mu.Lock()
	f.cancelled = true
	err := f.cancelErr
	f.mu.Unlock()
	return err
}

func (f *fakeRuntime) WasCancelled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cancelled
}

type fakeBridge struct {
	mu           sync.Mutex
	target       string
	version      string
	receipt      DeliveryReceipt
	err          error
	calls        int
	deliveryKeys []string
	packages     []KnowledgePackage
	afterDeliver func()
}

func (f *fakeBridge) Target() string         { return f.target }
func (f *fakeBridge) AdapterVersion() string { return f.version }

func (f *fakeBridge) Deliver(
	_ context.Context,
	request DeliveryRequest,
) (DeliveryReceipt, error) {
	f.mu.Lock()
	f.calls++
	f.deliveryKeys = append(f.deliveryKeys, request.DeliveryKey)
	f.packages = append(f.packages, request.Package)
	err := f.err
	receipt := f.receipt
	afterDeliver := f.afterDeliver
	f.mu.Unlock()
	if afterDeliver != nil {
		afterDeliver()
	}
	if err != nil {
		return DeliveryReceipt{}, err
	}
	return receipt, nil
}

func (f *fakeBridge) Cancel(_ context.Context, _ uint) error { return nil }

func (f *fakeBridge) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeBridge) DeliveryKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deliveryKeys...)
}

func (f *fakeBridge) LastPackage() KnowledgePackage {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.packages) == 0 {
		return KnowledgePackage{}
	}
	return f.packages[len(f.packages)-1]
}

type postPublishBlockingStore struct {
	delegate  ArtifactStore
	published chan ArtifactPublishResult
	release   chan struct{}
}

func (s *postPublishBlockingStore) Publish(
	ctx context.Context,
	request ArtifactPublishRequest,
) (ArtifactPublishResult, error) {
	published, err := s.delegate.Publish(ctx, request)
	if err != nil {
		return ArtifactPublishResult{}, err
	}
	s.published <- published
	<-s.release
	return published, nil
}

func (s *postPublishBlockingStore) Discard(
	ctx context.Context,
	published ArtifactPublishResult,
) error {
	return s.delegate.Discard(ctx, published)
}

var _ TranscriptionAdapter = (*fakeTranscriber)(nil)
var _ RuntimeAdapter = (*fakeRuntime)(nil)
var _ KnowledgeBridge = (*fakeBridge)(nil)
var _ ArtifactStore = (*postPublishBlockingStore)(nil)
