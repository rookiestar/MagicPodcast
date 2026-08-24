package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/models"
	"magicpodcast/internal/utils"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Engine struct {
	service       *Service
	transcriber   TranscriptionAdapter
	runtime       RuntimeAdapter
	artifactStore ArtifactStore
	bridges       []BridgeBinding

	activeMu sync.Mutex
	active   map[uint]context.CancelFunc
}

const (
	cancellationExternalResultUnknown = "cancelled_external_result_unknown"
	cancellationRuntimeResultUnknown  = "cancelled_runtime_result_unknown"
)

func NewEngine(
	service *Service,
	transcriber TranscriptionAdapter,
	runtime RuntimeAdapter,
	artifactStore ArtifactStore,
	bridges []BridgeBinding,
) (*Engine, error) {
	if service == nil || service.db == nil {
		return nil, fmt.Errorf("processing service is required")
	}
	if transcriber == nil || runtime == nil || artifactStore == nil {
		return nil, fmt.Errorf("all required processing adapters are required")
	}
	if !validAdapterIdentityPart(transcriber.Name()) ||
		!validAdapterIdentityPart(transcriber.Version()) {
		return nil, fmt.Errorf("transcription adapter identity is incomplete")
	}
	if !validAdapterIdentityPart(runtime.Name()) {
		return nil, fmt.Errorf("runtime adapter identity is incomplete")
	}
	seenBridges := make(map[string]struct{}, len(bridges))
	for _, binding := range bridges {
		if binding.Adapter == nil ||
			!validAdapterIdentityPart(binding.Destination) ||
			!validAdapterIdentityPart(binding.Adapter.Target()) ||
			!validAdapterIdentityPart(binding.Adapter.AdapterVersion()) {
			return nil, fmt.Errorf("knowledge bridge identity is incomplete")
		}
		identity := strings.Join([]string{
			binding.Adapter.Target(),
			binding.Destination,
			binding.Adapter.AdapterVersion(),
		}, "\x00")
		if _, exists := seenBridges[identity]; exists {
			return nil, fmt.Errorf("knowledge bridge identity is duplicated")
		}
		seenBridges[identity] = struct{}{}
	}
	return &Engine{
		service:       service,
		transcriber:   transcriber,
		runtime:       runtime,
		artifactStore: artifactStore,
		bridges:       append([]BridgeBinding(nil), bridges...),
		active:        make(map[uint]context.CancelFunc),
	}, nil
}

func validAdapterIdentityPart(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && value == trimmed
}

// Advance performs bounded work for one run. External waiting returns
// immediately after persisting the checkpoint; a later call resumes it.
func (e *Engine) Advance(
	ctx context.Context,
	runID uint,
) (models.EpisodeProcessingRun, error) {
	run, err := e.service.getRunModel(ctx, runID)
	if err != nil {
		return models.EpisodeProcessingRun{}, err
	}
	if models.IsProcessingRunTerminal(run.Status) {
		return run, nil
	}
	now := e.service.now().UTC()
	if run.NextAttemptAt != nil && run.NextAttemptAt.After(now) {
		return run, ErrRetryNotReady
	}

	runCtx, cancel := context.WithCancel(ctx)
	if !e.registerActive(runID, cancel) {
		cancel()
		return run, ErrRunBusy
	}
	defer func() {
		cancel()
		e.unregisterActive(runID)
	}()

	request := TranscriptionRequest{
		RunID:           run.ID,
		EpisodeID:       run.EpisodeID,
		AudioDigest:     run.AudioDigest,
		PipelineVersion: run.PipelineVersion,
		PersistCheckpoint: func(
			checkpointCtx context.Context,
			status string,
			state json.RawMessage,
		) error {
			if checkpointCtx == nil {
				checkpointCtx = context.Background()
			}
			return e.saveCheckpoint(
				context.WithoutCancel(checkpointCtx),
				run.ID,
				StepTranscription,
				e.transcriber.Name(),
				e.transcriber.Version(),
				status,
				state,
			)
		},
	}

	var progress TranscriptionProgress
	switch run.Status {
	case models.ProcessingRunStatusQueued:
		if run.TriggerSource == models.ProcessingTriggerScheduled {
			run, err = e.service.cancelQueuedScheduledRunOutsideFocus(runCtx, run.ID)
			if err != nil || models.IsProcessingRunTerminal(run.Status) {
				return run, err
			}
		}
		run, err = e.beginQueuedAttempt(runCtx, run.ID)
		if err != nil || models.IsProcessingRunTerminal(run.Status) {
			return run, err
		}
		var checkpoint models.ProcessingCheckpoint
		checkpoint, err = e.loadCheckpoint(runCtx, run.ID, StepTranscription)
		switch {
		case err == nil:
			if !checkpointIsValid(checkpoint) ||
				checkpoint.Adapter != e.transcriber.Name() ||
				checkpoint.AdapterVersion != e.transcriber.Version() {
				return e.handleStepError(
					runCtx,
					run.ID,
					NewAdapterError(
						"checkpoint_adapter_mismatch",
						"external processing checkpoint cannot be resumed by the configured adapter",
						false,
					),
					nil,
					"",
				)
			}
			progress, err = e.transcriber.Resume(
				runCtx,
				request,
				json.RawMessage(checkpoint.StateJSON),
			)
			if len(progress.Checkpoint) == 0 {
				progress.Checkpoint = append(
					json.RawMessage(nil),
					[]byte(checkpoint.StateJSON)...,
				)
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			progress, err = e.transcriber.Begin(runCtx, request)
		default:
			return run, err
		}
		if err != nil {
			return e.handleStepError(
				runCtx,
				run.ID,
				err,
				progress.Checkpoint,
				ExternalProgressWaiting,
			)
		}
	case models.ProcessingRunStatusWaitingExternal:
		var checkpoint models.ProcessingCheckpoint
		checkpoint, err = e.loadCheckpoint(runCtx, run.ID, StepTranscription)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if failErr := e.service.failRun(
					runCtx,
					run.ID,
					"missing_external_checkpoint",
					"external processing state cannot be recovered",
					false,
					now,
				); failErr != nil {
					return run, failErr
				}
				return e.service.getRunModel(runCtx, run.ID)
			}
			return run, err
		}
		if !checkpointIsValid(checkpoint) {
			if failErr := e.service.failRun(
				context.WithoutCancel(runCtx),
				run.ID,
				"invalid_external_checkpoint",
				"external processing checkpoint failed integrity validation",
				false,
				now,
			); failErr != nil {
				return run, failErr
			}
			return e.service.getRunModel(context.WithoutCancel(runCtx), run.ID)
		}
		if checkpoint.Adapter != e.transcriber.Name() ||
			checkpoint.AdapterVersion != e.transcriber.Version() {
			if failErr := e.service.failRun(
				context.WithoutCancel(runCtx),
				run.ID,
				"checkpoint_adapter_mismatch",
				"external processing checkpoint belongs to a different adapter version",
				false,
				now,
			); failErr != nil {
				return run, failErr
			}
			return e.service.getRunModel(context.WithoutCancel(runCtx), run.ID)
		}
		run, err = e.claimExternalWait(runCtx, run.ID)
		if err != nil || models.IsProcessingRunTerminal(run.Status) {
			return run, err
		}
		progress, err = e.transcriber.Resume(
			runCtx,
			request,
			json.RawMessage(checkpoint.StateJSON),
		)
		if len(progress.Checkpoint) == 0 {
			progress.Checkpoint = append(
				json.RawMessage(nil),
				[]byte(checkpoint.StateJSON)...,
			)
		}
		if err != nil {
			errorCheckpoint := progress.Checkpoint
			errorCheckpointStatus := ExternalProgressWaiting
			if checkpoint.Status == ExternalProgressCompleted {
				errorCheckpointStatus = ExternalProgressCompleted
			}
			return e.handleStepError(
				runCtx,
				run.ID,
				err,
				errorCheckpoint,
				errorCheckpointStatus,
			)
		}
	case models.ProcessingRunStatusRunning:
		return run, ErrRunBusy
	default:
		return run, fmt.Errorf("unsupported processing status %q", run.Status)
	}

	if len(progress.Checkpoint) > 0 && !json.Valid(progress.Checkpoint) {
		return e.handleStepError(
			runCtx,
			run.ID,
			NewUnknownExternalResultError(
				"invalid_external_checkpoint",
				"external processing checkpoint failed integrity validation",
			),
			nil,
			"",
		)
	}
	switch progress.Status {
	case ExternalProgressWaiting:
		if len(progress.Checkpoint) == 0 {
			return e.handleStepError(
				runCtx,
				run.ID,
				NewUnknownExternalResultError(
					"invalid_external_checkpoint",
					"external processing did not return a recoverable checkpoint",
				),
				nil,
				"",
			)
		}
		return e.persistExternalWait(runCtx, run.ID, progress.Checkpoint)
	case ExternalProgressUnknown:
		return e.handleStepError(
			runCtx,
			run.ID,
			NewUnknownExternalResultError(
				"external_result_unknown",
				"external processing result is unknown",
			),
			progress.Checkpoint,
			ExternalProgressWaiting,
		)
	case ExternalProgressCompleted:
		if progress.Transcript == "" {
			return e.handleStepError(
				runCtx,
				run.ID,
				NewAdapterError(
					"empty_transcript",
					"transcription completed without transcript content",
					false,
				),
				progress.Checkpoint,
				ExternalProgressCompleted,
			)
		}
		if len(progress.Checkpoint) == 0 {
			return e.handleStepError(
				runCtx,
				run.ID,
				NewAdapterError(
					"missing_completed_checkpoint",
					"completed transcription did not return a recoverable checkpoint",
					false,
				),
				nil,
				ExternalProgressCompleted,
			)
		}
	default:
		return e.handleStepError(
			runCtx,
			run.ID,
			NewUnknownExternalResultError(
				"invalid_external_status",
				"external processing returned an unknown status",
			),
			progress.Checkpoint,
			ExternalProgressWaiting,
		)
	}

	if len(progress.Checkpoint) > 0 {
		if err := e.saveCheckpoint(
			context.WithoutCancel(runCtx),
			run.ID,
			StepTranscription,
			e.transcriber.Name(),
			e.transcriber.Version(),
			ExternalProgressCompleted,
			progress.Checkpoint,
		); err != nil {
			return e.currentAfterConditionalFailure(runCtx, run.ID, err)
		}
	}
	if err := e.setCurrentStep(runCtx, run.ID, StepEpisodeNotes); err != nil {
		return e.currentAfterConditionalFailure(runCtx, run.ID, err)
	}
	runtimeResult, err := e.runtime.Execute(runCtx, RuntimeRequest{
		RunID:           run.ID,
		EpisodeID:       run.EpisodeID,
		PipelineVersion: run.PipelineVersion,
		Transcript:      progress.Transcript,
	})
	if err != nil {
		return e.handleStepError(
			runCtx,
			run.ID,
			err,
			progress.Checkpoint,
			ExternalProgressCompleted,
		)
	}
	if runtimeResult.EpisodeNotes == "" {
		return e.handleStepError(
			runCtx,
			run.ID,
			NewAdapterError(
				"empty_episode_notes",
				"runtime completed without episode notes",
				false,
			),
			progress.Checkpoint,
			ExternalProgressCompleted,
		)
	}
	skillVersions, err := mergeVersionMaps(
		progress.SkillVersions,
		runtimeResult.SkillVersions,
	)
	if err != nil {
		return e.handleStepError(
			runCtx,
			run.ID,
			NewAdapterError(
				"skill_version_conflict",
				"processing adapters reported conflicting skill versions",
				false,
			),
			progress.Checkpoint,
			ExternalProgressCompleted,
		)
	}

	if err := e.setCurrentStep(runCtx, run.ID, StepArtifactPublish); err != nil {
		return e.currentAfterConditionalFailure(runCtx, run.ID, err)
	}
	var packageEpisode models.Episode
	if len(e.bridges) > 0 {
		packageEpisode, err = e.loadKnowledgePackageEpisode(runCtx, run.EpisodeID)
		if err != nil {
			return e.handleStepError(
				runCtx,
				run.ID,
				NewAdapterError(
					"knowledge_package_load_failed",
					"knowledge package metadata could not be loaded",
					true,
				),
				progress.Checkpoint,
				ExternalProgressCompleted,
			)
		}
	}
	published, err := e.artifactStore.Publish(runCtx, ArtifactPublishRequest{
		RunID:                run.ID,
		EpisodeID:            run.EpisodeID,
		AudioDigest:          run.AudioDigest,
		PipelineVersion:      run.PipelineVersion,
		Transcript:           progress.Transcript,
		EpisodeNotes:         runtimeResult.EpisodeNotes,
		TranscriptionAdapter: e.transcriber.Name(),
		TranscriptionVersion: e.transcriber.Version(),
		RuntimeAdapter:       e.runtime.Name(),
		RuntimeVersion:       runtimeResult.RuntimeVersion,
		PromptVersion:        runtimeResult.PromptVersion,
		SkillVersions:        skillVersions,
		Sources:              progress.SourceRefs,
		RawArtifacts:         progress.RawArtifacts,
		GeneratedAt:          e.service.now().UTC(),
	})
	if err != nil {
		return e.handleStepError(
			runCtx,
			run.ID,
			err,
			progress.Checkpoint,
			ExternalProgressCompleted,
		)
	}

	run, artifact, err := e.completeWithArtifact(
		context.WithoutCancel(runCtx),
		run.ID,
		published,
	)
	if err != nil {
		var recorded bool
		run, artifact, recorded, err = e.reconcilePublishedArtifact(
			runCtx,
			run.ID,
			published,
			progress.Checkpoint,
			err,
		)
		if err != nil || !recorded {
			return run, err
		}
	}
	if len(e.bridges) == 0 {
		return run, nil
	}
	pkg := buildKnowledgePackage(
		run,
		artifact,
		packageEpisode,
		progress.Transcript,
		runtimeResult.EpisodeNotes,
		progress.SourceRefs,
	)
	for _, binding := range e.bridges {
		if err := e.deliver(runCtx, artifact, pkg, binding); err != nil {
			// Delivery state is persisted independently; local processing
			// remains completed even when a target fails.
			continue
		}
	}
	return run, nil
}

func (e *Engine) loadKnowledgePackageEpisode(
	ctx context.Context,
	episodeID uint,
) (models.Episode, error) {
	var episode models.Episode
	if err := e.service.db.WithContext(ctx).
		Preload("Podcast").
		First(&episode, episodeID).Error; err != nil {
		return models.Episode{}, fmt.Errorf("load knowledge package episode: %w", err)
	}
	return episode, nil
}

func buildKnowledgePackage(
	run models.EpisodeProcessingRun,
	artifact models.EpisodeArtifactSet,
	episode models.Episode,
	transcript string,
	episodeNotes string,
	sourceRefs map[string]string,
) KnowledgePackage {
	sourceURL := strings.TrimSpace(episode.Link)
	if sourceURL == "" {
		sourceURL = strings.TrimSpace(episode.MediumURL)
	}
	sources := cloneStringMap(sourceRefs)
	if sourceURL != "" {
		sources["episode"] = sourceURL
	}
	return KnowledgePackage{
		RunID:               run.ID,
		EpisodeID:           run.EpisodeID,
		EpisodeTitle:        episode.Title,
		PodcastTitle:        episode.Podcast.Title,
		PublishedAt:         episode.PublishedDate,
		SourceURL:           sourceURL,
		ShowNotes:           utils.HTMLToMarkdown(episode.ShowNotes),
		PipelineVersion:     run.PipelineVersion,
		ArtifactGeneratedAt: artifact.CreatedAt,
		ManifestSHA256:      artifact.ManifestSHA256,
		TranscriptSHA256:    artifact.TranscriptSHA256,
		EpisodeNotesSHA256:  artifact.NotesSHA256,
		Transcript:          transcript,
		EpisodeNotes:        episodeNotes,
		Sources:             sources,
	}
}

func (e *Engine) reconcilePublishedArtifact(
	ctx context.Context,
	runID uint,
	published ArtifactPublishResult,
	checkpoint json.RawMessage,
	commitErr error,
) (
	models.EpisodeProcessingRun,
	models.EpisodeArtifactSet,
	bool,
	error,
) {
	durableCtx := context.WithoutCancel(ctx)
	detail, readErr := e.service.GetProcessingRun(durableCtx, runID)
	if readErr != nil {
		return models.EpisodeProcessingRun{}, models.EpisodeArtifactSet{}, false, errors.Join(
			commitErr,
			fmt.Errorf("read artifact commit result: %w", readErr),
		)
	}
	if detail.Artifact != nil {
		if detail.Run.Status == models.ProcessingRunStatusCompleted &&
			artifactMatchesPublished(*detail.Artifact, published) {
			return detail.Run, *detail.Artifact, true, nil
		}
		failErr := e.service.failRun(
			durableCtx,
			runID,
			"artifact_state_inconsistent",
			"published artifact state is inconsistent and requires review",
			false,
			e.service.now().UTC(),
		)
		return detail.Run, *detail.Artifact, false, errors.Join(
			commitErr,
			failErr,
		)
	}

	if discardErr := e.artifactStore.Discard(durableCtx, published); discardErr != nil {
		failErr := e.service.failRun(
			durableCtx,
			runID,
			"artifact_reconciliation_failed",
			"unrecorded artifact cleanup failed and requires review",
			false,
			e.service.now().UTC(),
		)
		current, currentErr := e.service.getRunModel(durableCtx, runID)
		return current, models.EpisodeArtifactSet{}, false, errors.Join(
			commitErr,
			discardErr,
			failErr,
			currentErr,
		)
	}
	if models.IsProcessingRunTerminal(detail.Run.Status) {
		return detail.Run, models.EpisodeArtifactSet{}, false, nil
	}
	retrying, retryErr := e.handleStepError(
		durableCtx,
		runID,
		NewAdapterError(
			"artifact_record_failed",
			"artifact publication could not be recorded",
			true,
		),
		checkpoint,
		ExternalProgressCompleted,
	)
	return retrying, models.EpisodeArtifactSet{}, false, retryErr
}

func artifactMatchesPublished(
	artifact models.EpisodeArtifactSet,
	published ArtifactPublishResult,
) bool {
	return artifact.RootPath == published.RootPath &&
		artifact.ManifestPath == published.ManifestPath &&
		artifact.ManifestSHA256 == published.ManifestSHA256 &&
		artifact.TranscriptSHA256 == published.TranscriptSHA256 &&
		artifact.NotesSHA256 == published.NotesSHA256
}

func (e *Engine) Cancel(
	ctx context.Context,
	runID uint,
) (models.EpisodeProcessingRun, error) {
	run, err := e.service.CancelProcessingRun(ctx, runID)
	if err != nil {
		return models.EpisodeProcessingRun{}, err
	}
	if run.Status != models.ProcessingRunStatusCancelled {
		return run, nil
	}
	e.activeMu.Lock()
	cancel := e.active[runID]
	e.activeMu.Unlock()
	if cancel != nil {
		cancel()
	}

	durableCtx := context.WithoutCancel(ctx)
	checkpoint, checkpointErr := e.loadCheckpoint(durableCtx, runID, StepTranscription)
	var state json.RawMessage
	noticeCode := ""
	noticeMessages := make([]string, 0, 2)
	addNotice := func(code string, message string) {
		if noticeCode == "" {
			noticeCode = code
		}
		for _, existing := range noticeMessages {
			if existing == message {
				return
			}
		}
		noticeMessages = append(noticeMessages, message)
	}
	if checkpointErr == nil {
		state = json.RawMessage(checkpoint.StateJSON)
	} else if !errors.Is(checkpointErr, gorm.ErrRecordNotFound) {
		addNotice(
			cancellationExternalResultUnknown,
			"已取消本机加工；外部转写状态无法确认，任务可能继续。",
		)
	}
	if err := e.transcriber.Cancel(durableCtx, runID, state); err != nil {
		addNotice(
			cancellationExternalResultUnknown,
			"已取消本机加工；外部转写状态无法确认，任务可能继续。",
		)
	} else if reporter, ok := e.transcriber.(TranscriptionCancellationReporter); ok {
		disposition, dispositionErr := reporter.CancellationDisposition(state)
		switch {
		case dispositionErr != nil:
			addNotice(
				cancellationExternalResultUnknown,
				"已取消本机加工；外部转写状态无法确认，任务可能继续。",
			)
		case disposition.RemoteMayContinue:
			message := strings.TrimSpace(disposition.Message)
			if message == "" {
				message = "已取消本机加工；外部转写任务可能继续，已创建的远端资源会保留。"
			}
			addNotice(cancellationExternalResultUnknown, message)
		}
	}
	if err := e.runtime.Cancel(durableCtx, runID); err != nil {
		addNotice(
			cancellationRuntimeResultUnknown,
			"已取消本机加工；本地 Codex Runtime 的取消状态无法确认，可能仍在运行。",
		)
	}
	if noticeCode != "" {
		updated, recordErr := e.service.recordCancellationNotice(
			durableCtx,
			runID,
			noticeCode,
			strings.Join(noticeMessages, " "),
		)
		if recordErr != nil {
			return run, recordErr
		}
		return updated, nil
	}
	return run, nil
}

func (e *Engine) beginQueuedAttempt(
	ctx context.Context,
	runID uint,
) (models.EpisodeProcessingRun, error) {
	now := e.service.now().UTC()
	var run models.EpisodeProcessingRun
	err := e.service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := loadProcessingRun(tx, runID, &run); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRunNotFound
			}
			return err
		}
		if run.Status != models.ProcessingRunStatusQueued {
			if models.IsProcessingRunTerminal(run.Status) {
				return nil
			}
			return ErrRunBusy
		}
		if run.AttemptCount >= run.MaxAttempts || !now.Before(run.RetryDeadlineAt) {
			if err := tx.Model(&models.EpisodeProcessingRun{}).
				Where("id = ? AND status = ?", runID, models.ProcessingRunStatusQueued).
				Updates(map[string]any{
					"status":          models.ProcessingRunStatusFailed,
					"current_step":    "",
					"finished_at":     now,
					"next_attempt_at": nil,
					"error_code":      "retry_exhausted",
					"error_message":   "processing retry limit was exhausted",
					"error_retryable": false,
					"updated_at":      now,
				}).Error; err != nil {
				return err
			}
			return loadProcessingRun(tx, runID, &run)
		}
		attemptCount := run.AttemptCount + 1
		updates := map[string]any{
			"status":          models.ProcessingRunStatusRunning,
			"current_step":    StepTranscription,
			"attempt_count":   attemptCount,
			"next_attempt_at": nil,
			"error_code":      "",
			"error_message":   "",
			"error_retryable": false,
			"updated_at":      now,
		}
		if run.StartedAt == nil {
			updates["started_at"] = now
		}
		update := tx.Model(&models.EpisodeProcessingRun{}).
			Where("id = ? AND status = ?", runID, models.ProcessingRunStatusQueued).
			Updates(updates)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			if err := loadProcessingRun(tx, runID, &run); err != nil {
				return err
			}
			if models.IsProcessingRunTerminal(run.Status) {
				return nil
			}
			return ErrRunBusy
		}
		return loadProcessingRun(tx, runID, &run)
	})
	return run, err
}

func (e *Engine) claimExternalWait(
	ctx context.Context,
	runID uint,
) (models.EpisodeProcessingRun, error) {
	now := e.service.now().UTC()
	var run models.EpisodeProcessingRun
	err := e.service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := loadProcessingRun(tx, runID, &run); err != nil {
			return err
		}
		if run.Status != models.ProcessingRunStatusWaitingExternal {
			if models.IsProcessingRunTerminal(run.Status) {
				return nil
			}
			return ErrRunBusy
		}
		attemptCount := run.AttemptCount
		if run.NextAttemptAt != nil {
			if attemptCount >= run.MaxAttempts || !now.Before(run.RetryDeadlineAt) {
				if err := tx.Model(&models.EpisodeProcessingRun{}).
					Where("id = ? AND status = ?", runID, models.ProcessingRunStatusWaitingExternal).
					Updates(map[string]any{
						"status":          models.ProcessingRunStatusFailed,
						"current_step":    "",
						"finished_at":     now,
						"next_attempt_at": nil,
						"error_code":      "retry_exhausted",
						"error_message":   "processing retry limit was exhausted",
						"error_retryable": false,
						"updated_at":      now,
					}).Error; err != nil {
					return err
				}
				return loadProcessingRun(tx, runID, &run)
			}
			attemptCount++
		}
		update := tx.Model(&models.EpisodeProcessingRun{}).
			Where("id = ? AND status = ?", runID, models.ProcessingRunStatusWaitingExternal).
			Updates(map[string]any{
				"status":          models.ProcessingRunStatusRunning,
				"current_step":    StepTranscription,
				"attempt_count":   attemptCount,
				"next_attempt_at": nil,
				"updated_at":      now,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			if err := loadProcessingRun(tx, runID, &run); err != nil {
				return err
			}
			if models.IsProcessingRunTerminal(run.Status) {
				return nil
			}
			return ErrRunBusy
		}
		return loadProcessingRun(tx, runID, &run)
	})
	return run, err
}

func (e *Engine) persistExternalWait(
	ctx context.Context,
	runID uint,
	checkpoint json.RawMessage,
) (models.EpisodeProcessingRun, error) {
	durableCtx := context.WithoutCancel(ctx)
	now := e.service.now().UTC()
	err := e.service.db.WithContext(durableCtx).Transaction(func(tx *gorm.DB) error {
		if err := e.upsertCheckpoint(
			tx,
			runID,
			StepTranscription,
			e.transcriber.Name(),
			e.transcriber.Version(),
			ExternalProgressWaiting,
			checkpoint,
		); err != nil {
			return err
		}
		update := tx.Model(&models.EpisodeProcessingRun{}).
			Where("id = ? AND status = ?", runID, models.ProcessingRunStatusRunning).
			Updates(map[string]any{
				"status":       models.ProcessingRunStatusWaitingExternal,
				"current_step": StepTranscription,
				"updated_at":   now,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return fmt.Errorf("processing run is no longer active")
		}
		return nil
	})
	if err != nil {
		return e.currentAfterConditionalFailure(durableCtx, runID, err)
	}
	return e.service.getRunModel(durableCtx, runID)
}

func (e *Engine) saveCheckpoint(
	ctx context.Context,
	runID uint,
	step string,
	adapter string,
	adapterVersion string,
	status string,
	state json.RawMessage,
) error {
	return e.upsertCheckpoint(
		e.service.db.WithContext(ctx),
		runID,
		step,
		adapter,
		adapterVersion,
		status,
		state,
	)
}

func (e *Engine) upsertCheckpoint(
	db *gorm.DB,
	runID uint,
	step string,
	adapter string,
	adapterVersion string,
	status string,
	state json.RawMessage,
) error {
	if adapter == "" || adapterVersion == "" || len(state) == 0 || !json.Valid(state) {
		return fmt.Errorf("invalid processing checkpoint")
	}
	sum := sha256.Sum256(state)
	now := e.service.now().UTC()
	checkpoint := models.ProcessingCheckpoint{
		RunID:          runID,
		Step:           step,
		Adapter:        adapter,
		AdapterVersion: adapterVersion,
		Status:         status,
		StateJSON:      string(state),
		StateHash:      hex.EncodeToString(sum[:]),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "run_id"}, {Name: "step"}},
		DoUpdates: clause.Assignments(map[string]any{
			"adapter":         adapter,
			"adapter_version": adapterVersion,
			"status":          status,
			"state_json":      string(state),
			"state_hash":      checkpoint.StateHash,
			"updated_at":      now,
		}),
	}).Create(&checkpoint).Error
}

func (e *Engine) loadCheckpoint(
	ctx context.Context,
	runID uint,
	step string,
) (models.ProcessingCheckpoint, error) {
	var checkpoint models.ProcessingCheckpoint
	err := e.service.db.WithContext(ctx).
		Where("run_id = ? AND step = ?", runID, step).
		First(&checkpoint).Error
	return checkpoint, err
}

func (e *Engine) setCurrentStep(
	ctx context.Context,
	runID uint,
	step string,
) error {
	update := e.service.db.WithContext(ctx).Model(&models.EpisodeProcessingRun{}).
		Where("id = ? AND status = ?", runID, models.ProcessingRunStatusRunning).
		Updates(map[string]any{
			"current_step": step,
			"updated_at":   e.service.now().UTC(),
		})
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected == 0 {
		return fmt.Errorf("processing run is no longer active")
	}
	return nil
}

func (e *Engine) handleStepError(
	ctx context.Context,
	runID uint,
	stepErr error,
	checkpoint json.RawMessage,
	checkpointStatus string,
) (models.EpisodeProcessingRun, error) {
	classified := classifyAdapterError(stepErr)
	if len(checkpoint) > 0 {
		if !json.Valid(checkpoint) ||
			(checkpointStatus != ExternalProgressWaiting &&
				checkpointStatus != ExternalProgressCompleted) {
			classified = classifiedError{
				code:          "invalid_external_checkpoint",
				message:       "external processing checkpoint failed integrity validation",
				resultUnknown: true,
			}
			checkpoint = nil
			checkpointStatus = ""
		}
	}
	durableCtx := context.WithoutCancel(ctx)
	now := e.service.now().UTC()
	var run models.EpisodeProcessingRun
	err := e.service.db.WithContext(durableCtx).Transaction(func(tx *gorm.DB) error {
		if err := loadProcessingRun(tx, runID, &run); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRunNotFound
			}
			return err
		}
		if models.IsProcessingRunTerminal(run.Status) {
			return nil
		}
		if len(checkpoint) > 0 {
			if err := e.upsertCheckpoint(
				tx,
				runID,
				StepTranscription,
				e.transcriber.Name(),
				e.transcriber.Version(),
				checkpointStatus,
				checkpoint,
			); err != nil {
				return err
			}
		}

		updates := map[string]any{
			"current_step":    "",
			"next_attempt_at": nil,
			"error_code":      classified.code,
			"error_message":   classified.message,
			"error_retryable": classified.retryable,
			"updated_at":      now,
		}
		if !classified.resultUnknown &&
			classified.retryable &&
			run.AttemptCount < run.MaxAttempts &&
			now.Before(run.RetryDeadlineAt) {
			nextAttempt := now.Add(e.retryDelay(run.ID, run.AttemptCount))
			if nextAttempt.After(run.RetryDeadlineAt) {
				nextAttempt = run.RetryDeadlineAt
			}
			nextStatus := models.ProcessingRunStatusQueued
			if len(checkpoint) > 0 {
				nextStatus = models.ProcessingRunStatusWaitingExternal
			}
			updates["status"] = nextStatus
			updates["current_step"] = StepTranscription
			updates["next_attempt_at"] = nextAttempt
			updates["error_retryable"] = true
		} else {
			updates["status"] = models.ProcessingRunStatusFailed
			updates["finished_at"] = now
			if classified.resultUnknown {
				updates["error_retryable"] = false
			}
		}
		update := tx.Model(&models.EpisodeProcessingRun{}).
			Where("id = ? AND status IN ?", runID, models.ProcessingRunActiveStatuses).
			Updates(updates)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			if err := loadProcessingRun(tx, runID, &run); err != nil {
				return err
			}
			if models.IsProcessingRunTerminal(run.Status) {
				return nil
			}
			return fmt.Errorf("processing run is no longer active")
		}
		return loadProcessingRun(tx, runID, &run)
	})
	if err != nil {
		return run, err
	}
	return run, nil
}

func (e *Engine) retryDelay(runID uint, attempt int) time.Duration {
	delay := e.service.retryPolicy.BaseDelay
	for index := 1; index < attempt && delay < time.Hour; index++ {
		delay *= 2
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	if delay <= 0 || delay == time.Hour {
		return delay
	}

	// Deterministic jitter spreads simultaneous failures without making retry
	// timing non-reproducible after a restart. It stays in [0, 20%] of the
	// exponential delay and never exceeds the one-hour cap.
	seed := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%d", runID, attempt)))
	jitter := time.Duration(int64(delay) * int64(seed[0]) / (255 * 5))
	if delay > time.Hour-jitter {
		return time.Hour
	}
	return delay + jitter
}

func (e *Engine) completeWithArtifact(
	ctx context.Context,
	runID uint,
	published ArtifactPublishResult,
) (models.EpisodeProcessingRun, models.EpisodeArtifactSet, error) {
	now := e.service.now().UTC()
	var run models.EpisodeProcessingRun
	var artifact models.EpisodeArtifactSet
	err := e.service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := loadProcessingRun(tx, runID, &run); err != nil {
			return err
		}
		if run.Status != models.ProcessingRunStatusRunning {
			return fmt.Errorf("processing run is no longer active")
		}
		if err := tx.Model(&models.EpisodeArtifactSet{}).
			Where("episode_id = ? AND is_current = ?", run.EpisodeID, true).
			Update("is_current", false).Error; err != nil {
			return fmt.Errorf("retire previous artifact set: %w", err)
		}
		artifact = models.EpisodeArtifactSet{
			RunID:            run.ID,
			EpisodeID:        run.EpisodeID,
			PipelineVersion:  run.PipelineVersion,
			RootPath:         published.RootPath,
			ManifestPath:     published.ManifestPath,
			ManifestSHA256:   published.ManifestSHA256,
			TranscriptSHA256: published.TranscriptSHA256,
			NotesSHA256:      published.NotesSHA256,
			IsCurrent:        true,
			CreatedAt:        now,
		}
		if err := tx.Create(&artifact).Error; err != nil {
			return fmt.Errorf("record artifact set: %w", err)
		}
		update := tx.Model(&models.EpisodeProcessingRun{}).
			Where("id = ? AND status = ?", runID, models.ProcessingRunStatusRunning).
			Updates(map[string]any{
				"status":          models.ProcessingRunStatusCompleted,
				"current_step":    "",
				"finished_at":     now,
				"next_attempt_at": nil,
				"error_code":      "",
				"error_message":   "",
				"error_retryable": false,
				"updated_at":      now,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return fmt.Errorf("processing run is no longer active")
		}
		return loadProcessingRun(tx, runID, &run)
	})
	return run, artifact, err
}

func (e *Engine) deliver(
	ctx context.Context,
	artifact models.EpisodeArtifactSet,
	pkg KnowledgePackage,
	binding BridgeBinding,
) error {
	key := deliveryKey(
		artifact.ID,
		binding.Adapter.Target(),
		binding.Destination,
		binding.Adapter.AdapterVersion(),
	)
	now := e.service.now().UTC()
	var delivery models.KnowledgeDelivery
	err := e.service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		findErr := tx.Where("delivery_key = ?", key).First(&delivery).Error
		switch {
		case findErr == nil && delivery.Status == models.DeliveryStatusDelivered:
			return nil
		case findErr == nil:
			return tx.Model(&delivery).Updates(map[string]any{
				"status":          models.DeliveryStatusDelivering,
				"attempt_count":   delivery.AttemptCount + 1,
				"error_code":      "",
				"error_message":   "",
				"error_retryable": false,
				"updated_at":      now,
			}).Error
		case !errors.Is(findErr, gorm.ErrRecordNotFound):
			return findErr
		}
		delivery = models.KnowledgeDelivery{
			ArtifactSetID:  artifact.ID,
			Target:         binding.Adapter.Target(),
			Destination:    binding.Destination,
			AdapterVersion: binding.Adapter.AdapterVersion(),
			DeliveryKey:    key,
			Status:         models.DeliveryStatusDelivering,
			AttemptCount:   1,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		return tx.Create(&delivery).Error
	})
	if err != nil || delivery.Status == models.DeliveryStatusDelivered {
		return err
	}

	receipt, deliverErr := binding.Adapter.Deliver(ctx, DeliveryRequest{
		ArtifactSetID: artifact.ID,
		DeliveryKey:   key,
		Destination:   binding.Destination,
		Package:       pkg,
	})
	if deliverErr != nil {
		classified := classifyAdapterError(deliverErr)
		return e.service.db.WithContext(context.WithoutCancel(ctx)).
			Model(&models.KnowledgeDelivery{}).
			Where("id = ?", delivery.ID).
			Updates(map[string]any{
				"status":          models.DeliveryStatusFailed,
				"error_code":      classified.code,
				"error_message":   classified.message,
				"error_retryable": classified.retryable,
				"updated_at":      e.service.now().UTC(),
			}).Error
	}
	receiptStatus := receipt.Status
	if receiptStatus == "" {
		receiptStatus = models.DeliveryStatusDelivered
	}
	updatedAt := e.service.now().UTC()
	switch receiptStatus {
	case models.DeliveryStatusPending:
		return e.service.db.WithContext(context.WithoutCancel(ctx)).
			Model(&models.KnowledgeDelivery{}).
			Where("id = ?", delivery.ID).
			Updates(map[string]any{
				"status":          models.DeliveryStatusPending,
				"remote_ref":      receipt.RemoteRef,
				"public_url":      receipt.PublicURL,
				"delivered_at":    nil,
				"error_code":      "",
				"error_message":   "",
				"error_retryable": false,
				"updated_at":      updatedAt,
			}).Error
	case models.DeliveryStatusDelivered:
		return e.service.db.WithContext(context.WithoutCancel(ctx)).
			Model(&models.KnowledgeDelivery{}).
			Where("id = ?", delivery.ID).
			Updates(map[string]any{
				"status":          models.DeliveryStatusDelivered,
				"remote_ref":      receipt.RemoteRef,
				"public_url":      receipt.PublicURL,
				"delivered_at":    updatedAt,
				"error_code":      "",
				"error_message":   "",
				"error_retryable": false,
				"updated_at":      updatedAt,
			}).Error
	default:
		return e.service.db.WithContext(context.WithoutCancel(ctx)).
			Model(&models.KnowledgeDelivery{}).
			Where("id = ?", delivery.ID).
			Updates(map[string]any{
				"status":          models.DeliveryStatusFailed,
				"error_code":      "invalid_delivery_receipt",
				"error_message":   "knowledge bridge returned an invalid delivery status",
				"error_retryable": false,
				"updated_at":      updatedAt,
			}).Error
	}
}

func deliveryKey(
	artifactSetID uint,
	target string,
	destination string,
	adapterVersion string,
) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%d\x00%s\x00%s\x00%s",
		artifactSetID,
		target,
		destination,
		adapterVersion,
	)))
	return hex.EncodeToString(sum[:])
}

func mergeVersionMaps(inputs ...map[string]string) (map[string]string, error) {
	merged := make(map[string]string)
	for _, input := range inputs {
		for name, version := range input {
			if existing, found := merged[name]; found && existing != version {
				return nil, fmt.Errorf("conflicting version for %s", name)
			}
			merged[name] = version
		}
	}
	return merged, nil
}

func (e *Engine) currentAfterConditionalFailure(
	ctx context.Context,
	runID uint,
	operationErr error,
) (models.EpisodeProcessingRun, error) {
	durableCtx := context.WithoutCancel(ctx)
	current, err := e.service.getRunModel(durableCtx, runID)
	if err == nil && models.IsProcessingRunTerminal(current.Status) {
		return current, nil
	}
	if err != nil {
		return models.EpisodeProcessingRun{}, err
	}
	if current.Status == models.ProcessingRunStatusRunning {
		var (
			checkpoint       json.RawMessage
			checkpointStatus string
		)
		stored, checkpointErr := e.loadCheckpoint(
			durableCtx,
			runID,
			StepTranscription,
		)
		if checkpointErr == nil && checkpointIsValid(stored) {
			checkpoint = json.RawMessage(stored.StateJSON)
			checkpointStatus = stored.Status
		}
		retrying, retryErr := e.handleStepError(
			durableCtx,
			runID,
			NewAdapterError(
				"processing_state_update_failed",
				"processing state could not be advanced",
				true,
			),
			checkpoint,
			checkpointStatus,
		)
		if retryErr == nil {
			return retrying, nil
		}
		return retrying, errors.Join(operationErr, retryErr)
	}
	return current, operationErr
}

func (e *Engine) registerActive(runID uint, cancel context.CancelFunc) bool {
	e.activeMu.Lock()
	defer e.activeMu.Unlock()
	if _, exists := e.active[runID]; exists {
		return false
	}
	e.active[runID] = cancel
	return true
}

func (e *Engine) unregisterActive(runID uint) {
	e.activeMu.Lock()
	delete(e.active, runID)
	e.activeMu.Unlock()
}
