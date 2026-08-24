package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Service struct {
	db             *gorm.DB
	inputResolver  ProcessingInputResolver
	artifactReader ArtifactReader
	audioPreparer  AudioPreparer
	retryPolicy    RetryPolicy
	now            func() time.Time
}

type ServiceOption func(*Service)

func WithRetryPolicy(policy RetryPolicy) ServiceOption {
	return func(service *Service) {
		service.retryPolicy = policy
	}
}

func WithProcessingInputResolver(resolver ProcessingInputResolver) ServiceOption {
	return func(service *Service) {
		service.inputResolver = resolver
	}
}

func WithArtifactReader(reader ArtifactReader) ServiceOption {
	return func(service *Service) {
		service.artifactReader = reader
	}
}

func WithAudioPreparer(preparer AudioPreparer) ServiceOption {
	return func(service *Service) {
		service.audioPreparer = preparer
	}
}

func WithClock(now func() time.Time) ServiceOption {
	return func(service *Service) {
		if now != nil {
			service.now = now
		}
	}
}

func NewService(db *gorm.DB, options ...ServiceOption) *Service {
	service := &Service{
		db:          db,
		retryPolicy: DefaultRetryPolicy(),
		now:         time.Now,
	}
	for _, option := range options {
		option(service)
	}
	if service.retryPolicy.MaxAttempts < 1 {
		service.retryPolicy.MaxAttempts = 1
	}
	if service.retryPolicy.MaxElapsed <= 0 {
		service.retryPolicy.MaxElapsed = 24 * time.Hour
	}
	if service.retryPolicy.BaseDelay < 0 {
		service.retryPolicy.BaseDelay = 0
	}
	return service
}

func (s *Service) StartEpisodeProcessing(
	ctx context.Context,
	request StartRequest,
) (StartResult, error) {
	normalized, err := normalizeStartRequest(request)
	if err != nil {
		return StartResult{}, err
	}
	active, found, err := s.findActiveRun(ctx, normalized.EpisodeID)
	if err != nil {
		return StartResult{}, err
	}
	if found {
		return StartResult{Run: active, ReusedActive: true}, nil
	}
	if err := s.requireFocusedEpisode(ctx, normalized.EpisodeID); err != nil {
		return StartResult{}, err
	}
	if s.inputResolver == nil {
		return StartResult{}, ErrProcessingInputUnavailable
	}
	input, err := s.inputResolver.ResolveProcessingInput(ctx, normalized.EpisodeID)
	if err != nil {
		if normalized.RequireReadyAudio {
			return StartResult{}, ErrProcessingInputUnavailable
		}
		if s.audioPreparer == nil {
			return StartResult{}, ErrProcessingInputUnavailable
		}
		queued, queueErr := s.audioPreparer.Enqueue(ctx, normalized.EpisodeID)
		if queueErr != nil {
			return StartResult{}, queueErr
		}
		if queued.ReusedReady {
			return StartResult{}, ErrProcessingInputUnavailable
		}
		return s.startAudioPreparationRun(ctx, normalized, queued, nil)
	}
	input, err = normalizeProcessingInput(input)
	if err != nil {
		return StartResult{}, err
	}
	return s.startResolvedEpisodeProcessing(ctx, normalized, input)
}

func (s *Service) startAudioPreparationRun(
	ctx context.Context,
	request StartRequest,
	queued AudioEnqueueResult,
	previousRunID *uint,
) (StartResult, error) {
	pipelineVersion := strings.TrimSpace(s.inputResolver.PipelineVersion())
	if pipelineVersion == "" || len(pipelineVersion) > 100 ||
		queued.Asset.ID == 0 ||
		queued.Asset.EpisodeID != request.EpisodeID ||
		!sha256Pattern.MatchString(queued.Asset.SourceDigest) {
		return StartResult{}, ErrProcessingInputUnavailable
	}
	now := s.now().UTC()
	provisionalKey := audioPreparationKey(
		request.EpisodeID,
		queued.Asset.SourceDigest,
		pipelineVersion,
	)
	var result StartResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var active models.EpisodeProcessingRun
		activeErr := tx.
			Where("episode_id = ? AND status IN ?", request.EpisodeID, models.ProcessingRunActiveStatuses).
			Order("created_at DESC, id DESC").
			First(&active).Error
		switch {
		case activeErr == nil:
			result = StartResult{
				Run:            active,
				ReusedActive:   true,
				AudioAsset:     &queued.Asset,
				PreparingAudio: active.CurrentStep == StepAudioPreparation,
			}
			return nil
		case !errors.Is(activeErr, gorm.ErrRecordNotFound):
			return fmt.Errorf("read active processing run: %w", activeErr)
		}

		var focusCount int64
		if err := tx.Model(&models.EpisodeTriageDecision{}).
			Where("episode_id = ? AND queue_state = ?", request.EpisodeID, models.QueueStateFocus).
			Count(&focusCount).Error; err != nil {
			return fmt.Errorf("check Focus eligibility: %w", err)
		}
		if focusCount == 0 {
			return ErrEpisodeNotFocused
		}

		run := models.EpisodeProcessingRun{
			EpisodeID:       request.EpisodeID,
			ProcessingKey:   provisionalKey,
			AudioDigest:     "",
			PipelineVersion: pipelineVersion,
			TriggerSource:   request.TriggerSource,
			ScheduleRunID:   request.ScheduleRunID,
			Status:          models.ProcessingRunStatusQueued,
			CurrentStep:     StepAudioPreparation,
			PreviousRunID:   previousRunID,
			MaxAttempts:     s.retryPolicy.MaxAttempts,
			RetryDeadlineAt: now.Add(s.retryPolicy.MaxElapsed),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(&run).Error; err != nil {
			if isUniqueConstraintError(err) {
				var concurrent models.EpisodeProcessingRun
				if queryErr := tx.
					Where("episode_id = ? AND status IN ?", request.EpisodeID, models.ProcessingRunActiveStatuses).
					Order("created_at DESC, id DESC").
					First(&concurrent).Error; queryErr == nil {
					result = StartResult{
						Run:            concurrent,
						ReusedActive:   true,
						AudioAsset:     &queued.Asset,
						PreparingAudio: concurrent.CurrentStep == StepAudioPreparation,
					}
					return nil
				}
			}
			return fmt.Errorf("create audio preparation run: %w", err)
		}
		result = StartResult{
			Run:            run,
			AudioAsset:     &queued.Asset,
			PreparingAudio: true,
		}
		return nil
	})
	return result, err
}

func (s *Service) completeAudioPreparation(
	ctx context.Context,
	episodeID uint,
	ready ReadyAudio,
) (models.EpisodeProcessingRun, bool, error) {
	input, err := normalizeProcessingInput(ProcessingInput{
		AudioDigest:     ready.SHA256,
		PipelineVersion: s.inputResolver.PipelineVersion(),
	})
	if err != nil {
		return models.EpisodeProcessingRun{}, false, err
	}
	var run models.EpisodeProcessingRun
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		queryErr := tx.
			Where(
				"episode_id = ? AND status = ? AND current_step = ?",
				episodeID,
				models.ProcessingRunStatusQueued,
				StepAudioPreparation,
			).
			Order("created_at DESC, id DESC").
			First(&run).Error
		if errors.Is(queryErr, gorm.ErrRecordNotFound) {
			return nil
		}
		if queryErr != nil {
			return fmt.Errorf("read audio preparation run: %w", queryErr)
		}
		update := tx.Model(&models.EpisodeProcessingRun{}).
			Where(
				"id = ? AND status = ? AND current_step = ?",
				run.ID,
				models.ProcessingRunStatusQueued,
				StepAudioPreparation,
			).
			Updates(map[string]any{
				"processing_key":   processingKey(episodeID, input.AudioDigest, input.PipelineVersion),
				"audio_digest":     input.AudioDigest,
				"pipeline_version": input.PipelineVersion,
				"current_step":     "",
				"updated_at":       s.now().UTC(),
			})
		if update.Error != nil {
			return fmt.Errorf("complete audio preparation run: %w", update.Error)
		}
		if update.RowsAffected == 0 {
			run = models.EpisodeProcessingRun{}
			return nil
		}
		return loadProcessingRun(tx, run.ID, &run)
	})
	return run, run.ID != 0, err
}

func (s *Service) failAudioPreparation(
	ctx context.Context,
	episodeID uint,
	code string,
	message string,
	retryable bool,
) error {
	var run models.EpisodeProcessingRun
	err := s.db.WithContext(ctx).
		Where(
			"episode_id = ? AND status = ? AND current_step = ?",
			episodeID,
			models.ProcessingRunStatusQueued,
			StepAudioPreparation,
		).
		Order("created_at DESC, id DESC").
		First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read audio preparation run: %w", err)
	}
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" {
		code = AudioErrorDownloadFailed
	}
	if message == "" {
		message = "episode audio preparation failed"
	}
	return s.failRun(ctx, run.ID, code, message, retryable, s.now().UTC())
}

func (s *Service) listAudioPreparationRuns(
	ctx context.Context,
) ([]models.EpisodeProcessingRun, error) {
	runs := make([]models.EpisodeProcessingRun, 0)
	if err := s.db.WithContext(ctx).
		Where(
			"status = ? AND current_step = ?",
			models.ProcessingRunStatusQueued,
			StepAudioPreparation,
		).
		Order("created_at ASC, id ASC").
		Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("list audio preparation runs: %w", err)
	}
	return runs, nil
}

func (s *Service) GetLatestEpisodeAudioAsset(
	ctx context.Context,
	episodeID uint,
) (models.EpisodeAudioAsset, error) {
	if episodeID == 0 {
		return models.EpisodeAudioAsset{}, ErrEpisodeNotFound
	}
	var episodeCount int64
	if err := s.db.WithContext(ctx).Model(&models.Episode{}).
		Where("id = ?", episodeID).
		Count(&episodeCount).Error; err != nil {
		return models.EpisodeAudioAsset{}, fmt.Errorf("check audio episode: %w", err)
	}
	if episodeCount == 0 {
		return models.EpisodeAudioAsset{}, ErrEpisodeNotFound
	}
	var asset models.EpisodeAudioAsset
	if err := s.db.WithContext(ctx).
		Where("episode_id = ?", episodeID).
		Order("created_at DESC, id DESC").
		First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.EpisodeAudioAsset{}, ErrProcessingInputUnavailable
		}
		return models.EpisodeAudioAsset{}, fmt.Errorf("read episode audio asset: %w", err)
	}
	return asset, nil
}

func (s *Service) startResolvedEpisodeProcessing(
	ctx context.Context,
	request StartRequest,
	input ProcessingInput,
) (StartResult, error) {
	now := s.now().UTC()
	key := processingKey(request.EpisodeID, input.AudioDigest, input.PipelineVersion)
	var result StartResult

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var episodeCount int64
		if err := tx.Model(&models.Episode{}).
			Where("id = ?", request.EpisodeID).
			Count(&episodeCount).Error; err != nil {
			return fmt.Errorf("check processing episode: %w", err)
		}
		if episodeCount == 0 {
			return ErrEpisodeNotFound
		}

		var active models.EpisodeProcessingRun
		activeErr := tx.
			Where("episode_id = ? AND status IN ?", request.EpisodeID, models.ProcessingRunActiveStatuses).
			Order("created_at DESC, id DESC").
			First(&active).Error
		switch {
		case activeErr == nil:
			result = StartResult{Run: active, ReusedActive: true}
			return nil
		case !errors.Is(activeErr, gorm.ErrRecordNotFound):
			return fmt.Errorf("read active processing run: %w", activeErr)
		}

		var focusCount int64
		if err := tx.Model(&models.EpisodeTriageDecision{}).
			Where("episode_id = ? AND queue_state = ?", request.EpisodeID, models.QueueStateFocus).
			Count(&focusCount).Error; err != nil {
			return fmt.Errorf("check Focus eligibility: %w", err)
		}
		if focusCount == 0 {
			return ErrEpisodeNotFocused
		}

		if !request.Force {
			var completed models.EpisodeProcessingRun
			completedErr := tx.
				Joins("JOIN episode_artifact_sets ON episode_artifact_sets.run_id = episode_processing_runs.id").
				Where(
					"episode_processing_runs.episode_id = ? AND processing_key = ? AND status = ?",
					request.EpisodeID,
					key,
					models.ProcessingRunStatusCompleted,
				).
				Order("finished_at DESC, episode_processing_runs.id DESC").
				First(&completed).Error
			switch {
			case completedErr == nil:
				result = StartResult{Run: completed, ReusedSuccessful: true}
				return nil
			case !errors.Is(completedErr, gorm.ErrRecordNotFound):
				return fmt.Errorf("read reusable processing run: %w", completedErr)
			}

			// The engine owns bounded automatic retry for one processing run. Once
			// it reaches a terminal state, a later cron tick must not create a
			// fresh external request for the same audio/pipeline identity. That is
			// especially important for fail-closed unknown external write results.
			// Manual RetryProcessingRun remains the explicit, reviewable escape
			// hatch after an operator fixes the cause.
			if request.TriggerSource == models.ProcessingTriggerScheduled {
				var terminal models.EpisodeProcessingRun
				terminalErr := tx.
					Where(
						"episode_id = ? AND processing_key = ? AND status IN ?",
						request.EpisodeID,
						key,
						[]string{
							models.ProcessingRunStatusFailed,
							models.ProcessingRunStatusCancelled,
						},
					).
					Order("finished_at DESC, id DESC").
					First(&terminal).Error
				switch {
				case terminalErr == nil:
					result = StartResult{Run: terminal, ReusedTerminal: true}
					return nil
				case !errors.Is(terminalErr, gorm.ErrRecordNotFound):
					return fmt.Errorf("read terminal processing run: %w", terminalErr)
				}
			}
		}

		var previousRunID *uint
		if request.Force {
			var previous models.EpisodeProcessingRun
			previousErr := tx.
				Where("episode_id = ? AND processing_key = ?", request.EpisodeID, key).
				Order("created_at DESC, id DESC").
				First(&previous).Error
			switch {
			case previousErr == nil:
				previousRunID = &previous.ID
			case !errors.Is(previousErr, gorm.ErrRecordNotFound):
				return fmt.Errorf("read previous processing run: %w", previousErr)
			}
		}

		run := models.EpisodeProcessingRun{
			EpisodeID:       request.EpisodeID,
			ProcessingKey:   key,
			AudioDigest:     input.AudioDigest,
			PipelineVersion: input.PipelineVersion,
			TriggerSource:   request.TriggerSource,
			ScheduleRunID:   request.ScheduleRunID,
			Status:          models.ProcessingRunStatusQueued,
			PreviousRunID:   previousRunID,
			MaxAttempts:     s.retryPolicy.MaxAttempts,
			RetryDeadlineAt: now.Add(s.retryPolicy.MaxElapsed),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(&run).Error; err != nil {
			if isUniqueConstraintError(err) {
				var concurrent models.EpisodeProcessingRun
				if queryErr := tx.
					Where("episode_id = ? AND status IN ?", request.EpisodeID, models.ProcessingRunActiveStatuses).
					Order("created_at DESC, id DESC").
					First(&concurrent).Error; queryErr == nil {
					result = StartResult{Run: concurrent, ReusedActive: true}
					return nil
				}
			}
			return fmt.Errorf("create processing run: %w", err)
		}
		result = StartResult{Run: run}
		return nil
	})
	return result, err
}

func (s *Service) findActiveRun(
	ctx context.Context,
	episodeID uint,
) (models.EpisodeProcessingRun, bool, error) {
	var active models.EpisodeProcessingRun
	err := s.db.WithContext(ctx).
		Where("episode_id = ? AND status IN ?", episodeID, models.ProcessingRunActiveStatuses).
		Order("created_at DESC, id DESC").
		First(&active).Error
	switch {
	case err == nil:
		return active, true, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return models.EpisodeProcessingRun{}, false, nil
	default:
		return models.EpisodeProcessingRun{}, false, fmt.Errorf("read active processing run: %w", err)
	}
}

func (s *Service) requireFocusedEpisode(ctx context.Context, episodeID uint) error {
	var episodeCount int64
	if err := s.db.WithContext(ctx).Model(&models.Episode{}).
		Where("id = ?", episodeID).
		Count(&episodeCount).Error; err != nil {
		return fmt.Errorf("check processing episode: %w", err)
	}
	if episodeCount == 0 {
		return ErrEpisodeNotFound
	}
	var focusCount int64
	if err := s.db.WithContext(ctx).Model(&models.EpisodeTriageDecision{}).
		Where("episode_id = ? AND queue_state = ?", episodeID, models.QueueStateFocus).
		Count(&focusCount).Error; err != nil {
		return fmt.Errorf("check Focus eligibility: %w", err)
	}
	if focusCount == 0 {
		return ErrEpisodeNotFocused
	}
	return nil
}

type RunDetail struct {
	Run              models.EpisodeProcessingRun `json:"run"`
	Artifact         *models.EpisodeArtifactSet  `json:"artifact,omitempty"`
	CurrentArtifact  *models.EpisodeArtifactSet  `json:"current_artifact,omitempty"`
	Deliveries       []models.KnowledgeDelivery  `json:"deliveries"`
	ActionSuggestion string                      `json:"action_suggestion,omitempty"`
}

func (s *Service) GetProcessingRun(ctx context.Context, runID uint) (RunDetail, error) {
	run, err := s.getRunModel(ctx, runID)
	if err != nil {
		return RunDetail{}, err
	}
	detail := RunDetail{Run: run, Deliveries: []models.KnowledgeDelivery{}}
	detail.ActionSuggestion = processingActionSuggestion(run)

	var artifact models.EpisodeArtifactSet
	artifactErr := s.db.WithContext(ctx).Where("run_id = ?", runID).First(&artifact).Error
	switch {
	case artifactErr == nil:
		detail.Artifact = &artifact
		if err := s.db.WithContext(ctx).
			Where("artifact_set_id = ?", artifact.ID).
			Order("created_at ASC, id ASC").
			Find(&detail.Deliveries).Error; err != nil {
			return RunDetail{}, fmt.Errorf("list processing deliveries: %w", err)
		}
	case !errors.Is(artifactErr, gorm.ErrRecordNotFound):
		return RunDetail{}, fmt.Errorf("read processing artifact: %w", artifactErr)
	}
	var currentArtifact models.EpisodeArtifactSet
	currentArtifactErr := s.db.WithContext(ctx).
		Where("episode_id = ? AND is_current = ?", run.EpisodeID, true).
		Order("created_at DESC, id DESC").
		First(&currentArtifact).Error
	switch {
	case currentArtifactErr == nil:
		detail.CurrentArtifact = &currentArtifact
	case !errors.Is(currentArtifactErr, gorm.ErrRecordNotFound):
		return RunDetail{}, fmt.Errorf("read current episode artifact: %w", currentArtifactErr)
	}
	return detail, nil
}

func processingActionSuggestion(run models.EpisodeProcessingRun) string {
	switch run.ErrorCode {
	case "audio_not_ready", "audio_digest_mismatch":
		return "请重新准备受管音频后再开始加工。"
	case "lark_cli_unavailable":
		return "请检查生产机上的 lark-cli 安装。"
	case "lark_auth_expired":
		return "请在生产机重新完成飞书用户登录后重试。"
	case "lark_permission_denied":
		return "请补齐飞书云空间与妙记所需的用户权限后重试。"
	case "lark_drive_result_unknown", "lark_minutes_result_unknown", "external_result_unknown":
		return "请先在飞书确认远端资源是否已创建；系统不会自动重复上传。"
	case cancellationExternalResultUnknown:
		return "请先在飞书确认转写是否仍在继续或远端资源是否已创建；确认前不可重新加工。"
	case cancellationRuntimeResultUnknown:
		return "请确认本地 Codex Runtime 已停止后再重新加工。"
	case "runtime_unavailable", "runtime_error":
		return "请检查本地 Codex Runtime 后重试，已完成的逐字稿会继续复用。"
	case "transcript_empty", "empty_transcript":
		return "请等待飞书转写完成或检查妙记产物后重试。"
	}
	if strings.HasPrefix(run.ErrorCode, "audio_") {
		if run.ErrorRetryable {
			return "请检查音频来源或本机存储后从准备阶段重试。"
		}
		return "请修正音频来源、格式、大小或时长后重试。"
	}
	if run.ErrorRetryable {
		return "当前检查点可安全重试。"
	}
	return ""
}

func (s *Service) GetArtifactContent(
	ctx context.Context,
	artifactSetID uint,
	kind string,
) (ArtifactContent, error) {
	if artifactSetID == 0 {
		return ArtifactContent{}, ErrInvalidArtifact
	}
	if s.artifactReader == nil {
		return ArtifactContent{}, ErrInvalidArtifact
	}
	var artifact models.EpisodeArtifactSet
	if err := s.db.WithContext(ctx).First(&artifact, artifactSetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ArtifactContent{}, ErrArtifactNotFound
		}
		return ArtifactContent{}, fmt.Errorf("read artifact set: %w", err)
	}
	return s.artifactReader.ReadText(ctx, artifact, kind)
}

func (s *Service) ListEpisodeProcessingRuns(
	ctx context.Context,
	episodeID uint,
) ([]models.EpisodeProcessingRun, error) {
	var episodeCount int64
	if err := s.db.WithContext(ctx).Model(&models.Episode{}).
		Where("id = ?", episodeID).
		Count(&episodeCount).Error; err != nil {
		return nil, fmt.Errorf("check processing episode: %w", err)
	}
	if episodeCount == 0 {
		return nil, ErrEpisodeNotFound
	}
	runs := make([]models.EpisodeProcessingRun, 0)
	if err := s.db.WithContext(ctx).
		Where("episode_id = ?", episodeID).
		Order("created_at DESC, id DESC").
		Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("list episode processing runs: %w", err)
	}
	return runs, nil
}

func (s *Service) listRunnableRunIDs(
	ctx context.Context,
	now time.Time,
	externalPollInterval time.Duration,
	limit int,
) ([]uint, error) {
	if limit < 1 {
		limit = 1
	}
	pollBefore := now.Add(-externalPollInterval)
	var runIDs []uint
	err := s.db.WithContext(ctx).
		Model(&models.EpisodeProcessingRun{}).
		Select("id").
		Where(
			`(status = ? AND current_step <> ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?))
			 OR
			 (status = ? AND (
			   (next_attempt_at IS NOT NULL AND next_attempt_at <= ?)
			   OR
			   (next_attempt_at IS NULL AND updated_at <= ?)
			))`,
			models.ProcessingRunStatusQueued,
			StepAudioPreparation,
			now,
			models.ProcessingRunStatusWaitingExternal,
			now,
			pollBefore,
		).
		Order("created_at ASC, id ASC").
		Limit(limit).
		Pluck("id", &runIDs).Error
	if err != nil {
		return nil, fmt.Errorf("list runnable processing runs: %w", err)
	}
	return runIDs, nil
}

// cancelQueuedScheduledRunOutsideFocus closes a scheduled run only while it
// is still queued. Once a run has started, moving the episode out of Focus
// must not implicitly cancel its in-flight work.
func (s *Service) cancelQueuedScheduledRunOutsideFocus(
	ctx context.Context,
	runID uint,
) (models.EpisodeProcessingRun, error) {
	now := s.now().UTC()
	var run models.EpisodeProcessingRun
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := loadProcessingRun(tx, runID, &run); err != nil {
			return err
		}
		if run.Status != models.ProcessingRunStatusQueued ||
			run.TriggerSource != models.ProcessingTriggerScheduled {
			return nil
		}
		var focusCount int64
		if err := tx.Model(&models.EpisodeTriageDecision{}).
			Where("episode_id = ? AND queue_state = ?", run.EpisodeID, models.QueueStateFocus).
			Count(&focusCount).Error; err != nil {
			return fmt.Errorf("recheck scheduled Focus eligibility: %w", err)
		}
		if focusCount > 0 {
			return nil
		}
		update := tx.Model(&models.EpisodeProcessingRun{}).
			Where("id = ? AND status = ?", run.ID, models.ProcessingRunStatusQueued).
			Updates(map[string]any{
				"status":          models.ProcessingRunStatusCancelled,
				"current_step":    "",
				"cancelled_at":    now,
				"finished_at":     now,
				"next_attempt_at": nil,
				"error_code":      "scheduled_not_in_focus",
				"error_message":   "episode left Focus before scheduled processing began",
				"error_retryable": false,
				"updated_at":      now,
			})
		if update.Error != nil {
			return fmt.Errorf("cancel queued scheduled processing run: %w", update.Error)
		}
		if update.RowsAffected == 0 {
			return loadProcessingRun(tx, runID, &run)
		}
		if run.ScheduleRunID != nil {
			itemUpdate := tx.Model(&models.ProcessingScheduleItem{}).
				Where(
					"schedule_run_id = ? AND episode_id = ? AND processing_run_id = ? AND outcome = ?",
					*run.ScheduleRunID,
					run.EpisodeID,
					run.ID,
					models.ProcessingScheduleItemOutcomeStarted,
				).
				Updates(map[string]any{
					"outcome":    models.ProcessingScheduleItemOutcomeSkipped,
					"reason":     scheduleSkipNotFocused,
					"updated_at": now,
				})
			if itemUpdate.Error != nil {
				return fmt.Errorf("record skipped scheduled processing item: %w", itemUpdate.Error)
			}
			if itemUpdate.RowsAffected > 0 {
				if err := tx.Model(&models.ProcessingScheduleRun{}).
					Where("id = ?", *run.ScheduleRunID).
					Updates(map[string]any{
						"started_count": gorm.Expr("CASE WHEN started_count > 0 THEN started_count - 1 ELSE 0 END"),
						"skipped_count": gorm.Expr("skipped_count + 1"),
						"updated_at":    now,
					}).Error; err != nil {
					return fmt.Errorf("update scheduled processing counts: %w", err)
				}
			}
		}
		return loadProcessingRun(tx, runID, &run)
	})
	return run, err
}

func (s *Service) CancelProcessingRun(
	ctx context.Context,
	runID uint,
) (models.EpisodeProcessingRun, error) {
	now := s.now().UTC()
	var run models.EpisodeProcessingRun
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := loadProcessingRun(tx, runID, &run); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRunNotFound
			}
			return fmt.Errorf("read processing run: %w", err)
		}
		if models.IsProcessingRunTerminal(run.Status) {
			return nil
		}
		update := tx.Model(&models.EpisodeProcessingRun{}).
			Where("id = ? AND status IN ?", runID, models.ProcessingRunActiveStatuses).
			Updates(map[string]any{
				"status":          models.ProcessingRunStatusCancelled,
				"current_step":    "",
				"cancelled_at":    now,
				"finished_at":     now,
				"next_attempt_at": nil,
				"error_code":      "",
				"error_message":   "",
				"error_retryable": false,
				"updated_at":      now,
			})
		if update.Error != nil {
			return fmt.Errorf("cancel processing run: %w", update.Error)
		}
		if err := loadProcessingRun(tx, runID, &run); err != nil {
			return fmt.Errorf("reload cancelled processing run: %w", err)
		}
		return nil
	})
	return run, err
}

func (s *Service) recordCancellationNotice(
	ctx context.Context,
	runID uint,
	code string,
	message string,
) (models.EpisodeProcessingRun, error) {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" || message == "" {
		return models.EpisodeProcessingRun{}, fmt.Errorf("cancellation notice is incomplete")
	}
	update := s.db.WithContext(ctx).
		Model(&models.EpisodeProcessingRun{}).
		Where("id = ? AND status = ?", runID, models.ProcessingRunStatusCancelled).
		Updates(map[string]any{
			"error_code":      code,
			"error_message":   message,
			"error_retryable": false,
			"updated_at":      s.now().UTC(),
		})
	if update.Error != nil {
		return models.EpisodeProcessingRun{}, fmt.Errorf("record cancellation notice: %w", update.Error)
	}
	run, err := s.getRunModel(ctx, runID)
	if err != nil {
		return models.EpisodeProcessingRun{}, err
	}
	if run.Status != models.ProcessingRunStatusCancelled {
		return run, fmt.Errorf("processing run %d is no longer cancelled", runID)
	}
	return run, nil
}

func (s *Service) RetryProcessingRun(
	ctx context.Context,
	sourceRunID uint,
) (StartResult, error) {
	source, err := s.getRunModel(ctx, sourceRunID)
	if err != nil {
		return StartResult{}, err
	}
	if source.Status != models.ProcessingRunStatusFailed &&
		source.Status != models.ProcessingRunStatusCancelled {
		return StartResult{}, ErrRetryUnsafe
	}
	if strings.Contains(source.ErrorCode, "result_unknown") {
		return StartResult{}, ErrRetryUnsafe
	}
	if active, found, err := s.findActiveRun(ctx, source.EpisodeID); err != nil {
		return StartResult{}, err
	} else if found {
		return StartResult{Run: active, ReusedActive: true}, nil
	}
	if err := s.requireFocusedEpisode(ctx, source.EpisodeID); err != nil {
		return StartResult{}, err
	}
	if s.inputResolver == nil {
		return StartResult{}, ErrProcessingInputUnavailable
	}
	input, err := s.inputResolver.ResolveProcessingInput(ctx, source.EpisodeID)
	if err != nil {
		if s.audioPreparer != nil && strings.HasPrefix(source.ErrorCode, "audio_") {
			queued, queueErr := s.audioPreparer.Enqueue(ctx, source.EpisodeID)
			if queueErr != nil {
				return StartResult{}, queueErr
			}
			if !queued.ReusedReady {
				retryRequest := StartRequest{
					EpisodeID:     source.EpisodeID,
					TriggerSource: models.ProcessingTriggerManual,
				}
				return s.startAudioPreparationRun(
					ctx,
					retryRequest,
					queued,
					&source.ID,
				)
			}
		}
		return StartResult{}, ErrProcessingInputUnavailable
	}
	input, err = normalizeProcessingInput(input)
	if err != nil {
		return StartResult{}, err
	}
	if processingKey(source.EpisodeID, input.AudioDigest, input.PipelineVersion) !=
		source.ProcessingKey {
		return StartResult{}, ErrRetryUnsafe
	}

	now := s.now().UTC()
	var result StartResult
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := loadProcessingRun(tx, sourceRunID, &source); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRunNotFound
			}
			return err
		}
		if source.Status != models.ProcessingRunStatusFailed &&
			source.Status != models.ProcessingRunStatusCancelled {
			return ErrRetryUnsafe
		}
		var active models.EpisodeProcessingRun
		activeErr := tx.
			Where("episode_id = ? AND status IN ?", source.EpisodeID, models.ProcessingRunActiveStatuses).
			Order("created_at DESC, id DESC").
			First(&active).Error
		switch {
		case activeErr == nil:
			result = StartResult{Run: active, ReusedActive: true}
			return nil
		case !errors.Is(activeErr, gorm.ErrRecordNotFound):
			return activeErr
		}

		var checkpoint models.ProcessingCheckpoint
		checkpointErr := tx.
			Where("run_id = ? AND step = ?", sourceRunID, StepTranscription).
			First(&checkpoint).Error
		switch {
		case checkpointErr == nil:
			if !checkpointIsValid(checkpoint) {
				return ErrRetryUnsafe
			}
		case errors.Is(checkpointErr, gorm.ErrRecordNotFound):
			if !source.ErrorRetryable &&
				source.Status != models.ProcessingRunStatusCancelled {
				return ErrRetryUnsafe
			}
		default:
			return checkpointErr
		}

		retry := models.EpisodeProcessingRun{
			EpisodeID:       source.EpisodeID,
			ProcessingKey:   source.ProcessingKey,
			AudioDigest:     source.AudioDigest,
			PipelineVersion: source.PipelineVersion,
			TriggerSource:   models.ProcessingTriggerManual,
			Status:          models.ProcessingRunStatusQueued,
			PreviousRunID:   &source.ID,
			MaxAttempts:     s.retryPolicy.MaxAttempts,
			RetryDeadlineAt: now.Add(s.retryPolicy.MaxElapsed),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Create(&retry).Error; err != nil {
			return fmt.Errorf("create processing retry: %w", err)
		}
		if checkpointErr == nil {
			checkpoint.ID = 0
			checkpoint.RunID = retry.ID
			checkpoint.Run = models.EpisodeProcessingRun{}
			checkpoint.CreatedAt = now
			checkpoint.UpdatedAt = now
			if err := tx.Create(&checkpoint).Error; err != nil {
				return fmt.Errorf("copy processing checkpoint: %w", err)
			}
		}
		result = StartResult{Run: retry}
		return nil
	})
	return result, err
}

type RecoveryResult struct {
	RecoverableRunIDs []uint `json:"recoverable_run_ids"`
	FailedRunIDs      []uint `json:"failed_run_ids"`
	FailedDeliveryIDs []uint `json:"failed_delivery_ids"`
}

// RecoverNonTerminalRuns is called by an explicitly started processing worker
// after a service restart. Normal API setup must remain read-only. Queued work
// and external waits with durable checkpoints remain recoverable. Local work
// and in-flight deliveries cannot survive the process boundary, so they are
// closed explicitly instead of appearing active forever.
func (s *Service) RecoverNonTerminalRuns(
	ctx context.Context,
	recoveredAt time.Time,
) (RecoveryResult, error) {
	var runs []models.EpisodeProcessingRun
	if err := s.db.WithContext(ctx).
		Where("status IN ?", models.ProcessingRunActiveStatuses).
		Order("id ASC").
		Find(&runs).Error; err != nil {
		return RecoveryResult{}, fmt.Errorf("list recoverable processing runs: %w", err)
	}

	result := RecoveryResult{
		RecoverableRunIDs: []uint{},
		FailedRunIDs:      []uint{},
		FailedDeliveryIDs: []uint{},
	}
	for _, run := range runs {
		switch run.Status {
		case models.ProcessingRunStatusQueued:
			result.RecoverableRunIDs = append(result.RecoverableRunIDs, run.ID)
		case models.ProcessingRunStatusWaitingExternal:
			var checkpoint models.ProcessingCheckpoint
			checkpointErr := s.db.WithContext(ctx).
				Where("run_id = ? AND step = ?", run.ID, StepTranscription).
				First(&checkpoint).Error
			if checkpointErr != nil && !errors.Is(checkpointErr, gorm.ErrRecordNotFound) {
				return RecoveryResult{}, fmt.Errorf("check processing checkpoint: %w", checkpointErr)
			}
			if checkpointErr == nil && checkpointIsValid(checkpoint) {
				result.RecoverableRunIDs = append(result.RecoverableRunIDs, run.ID)
				continue
			}
			code := "missing_external_checkpoint"
			message := "external processing state cannot be recovered"
			if checkpointErr == nil {
				code = "invalid_external_checkpoint"
				message = "external processing checkpoint failed integrity validation"
			}
			if err := s.failRun(
				ctx,
				run.ID,
				code,
				message,
				false,
				recoveredAt.UTC(),
			); err != nil {
				return RecoveryResult{}, err
			}
			result.FailedRunIDs = append(result.FailedRunIDs, run.ID)
		case models.ProcessingRunStatusRunning:
			if err := s.failRun(
				ctx,
				run.ID,
				"interrupted_by_restart",
				"local processing step was interrupted by service restart",
				true,
				recoveredAt.UTC(),
			); err != nil {
				return RecoveryResult{}, err
			}
			result.FailedRunIDs = append(result.FailedRunIDs, run.ID)
		}
	}
	var deliveries []models.KnowledgeDelivery
	if err := s.db.WithContext(ctx).
		Where("status = ?", models.DeliveryStatusDelivering).
		Order("id ASC").
		Find(&deliveries).Error; err != nil {
		return RecoveryResult{}, fmt.Errorf("list interrupted knowledge deliveries: %w", err)
	}
	if len(deliveries) > 0 {
		deliveryIDs := make([]uint, 0, len(deliveries))
		for _, delivery := range deliveries {
			deliveryIDs = append(deliveryIDs, delivery.ID)
		}
		update := s.db.WithContext(ctx).Model(&models.KnowledgeDelivery{}).
			Where("id IN ? AND status = ?", deliveryIDs, models.DeliveryStatusDelivering).
			Updates(map[string]any{
				"status":          models.DeliveryStatusFailed,
				"error_code":      "external_result_unknown",
				"error_message":   "knowledge delivery was interrupted before its result was recorded",
				"error_retryable": false,
				"updated_at":      recoveredAt.UTC(),
			})
		if update.Error != nil {
			return RecoveryResult{}, fmt.Errorf("close interrupted knowledge deliveries: %w", update.Error)
		}
		result.FailedDeliveryIDs = deliveryIDs
	}
	return result, nil
}

func (s *Service) getRunModel(
	ctx context.Context,
	runID uint,
) (models.EpisodeProcessingRun, error) {
	var run models.EpisodeProcessingRun
	if err := loadProcessingRun(s.db.WithContext(ctx), runID, &run); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.EpisodeProcessingRun{}, ErrRunNotFound
		}
		return models.EpisodeProcessingRun{}, fmt.Errorf("read processing run: %w", err)
	}
	return run, nil
}

func loadProcessingRun(
	db *gorm.DB,
	runID uint,
	run *models.EpisodeProcessingRun,
) error {
	*run = models.EpisodeProcessingRun{}
	return db.First(run, runID).Error
}

func (s *Service) failRun(
	ctx context.Context,
	runID uint,
	code string,
	message string,
	retryable bool,
	now time.Time,
) error {
	update := s.db.WithContext(ctx).Model(&models.EpisodeProcessingRun{}).
		Where("id = ? AND status IN ?", runID, models.ProcessingRunActiveStatuses).
		Updates(map[string]any{
			"status":          models.ProcessingRunStatusFailed,
			"current_step":    "",
			"finished_at":     now,
			"next_attempt_at": nil,
			"error_code":      code,
			"error_message":   message,
			"error_retryable": retryable,
			"updated_at":      now,
		})
	if update.Error != nil {
		return fmt.Errorf("fail processing run: %w", update.Error)
	}
	return nil
}

func normalizeStartRequest(request StartRequest) (StartRequest, error) {
	request.TriggerSource = strings.TrimSpace(request.TriggerSource)
	if request.EpisodeID == 0 {
		return StartRequest{}, fmt.Errorf("%w: episode id is required", ErrInvalidStart)
	}
	switch request.TriggerSource {
	case models.ProcessingTriggerManual, models.ProcessingTriggerScheduled:
	default:
		return StartRequest{}, fmt.Errorf("%w: invalid trigger source", ErrInvalidStart)
	}
	if request.RequireReadyAudio && request.TriggerSource != models.ProcessingTriggerScheduled {
		return StartRequest{}, fmt.Errorf("%w: ready-audio requirement is scheduled-only", ErrInvalidStart)
	}
	if request.ScheduleRunID != nil {
		if *request.ScheduleRunID == 0 || request.TriggerSource != models.ProcessingTriggerScheduled {
			return StartRequest{}, fmt.Errorf("%w: invalid schedule run", ErrInvalidStart)
		}
	}
	return request, nil
}

func normalizeProcessingInput(input ProcessingInput) (ProcessingInput, error) {
	input.AudioDigest = strings.ToLower(strings.TrimSpace(input.AudioDigest))
	input.PipelineVersion = strings.TrimSpace(input.PipelineVersion)
	if !sha256Pattern.MatchString(input.AudioDigest) {
		return ProcessingInput{}, ErrProcessingInputUnavailable
	}
	if input.PipelineVersion == "" || len(input.PipelineVersion) > 100 {
		return ProcessingInput{}, ErrProcessingInputUnavailable
	}
	return input, nil
}

func processingKey(episodeID uint, audioDigest, pipelineVersion string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%d\x00%s\x00%s",
		episodeID,
		audioDigest,
		pipelineVersion,
	)))
	return hex.EncodeToString(sum[:])
}

func audioPreparationKey(
	episodeID uint,
	sourceDigest string,
	pipelineVersion string,
) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"audio-prepare\x00%d\x00%s\x00%s",
		episodeID,
		sourceDigest,
		pipelineVersion,
	)))
	return hex.EncodeToString(sum[:])
}

func checkpointIsValid(checkpoint models.ProcessingCheckpoint) bool {
	if strings.TrimSpace(checkpoint.Adapter) == "" ||
		strings.TrimSpace(checkpoint.AdapterVersion) == "" {
		return false
	}
	switch checkpoint.Status {
	case ExternalProgressWaiting, ExternalProgressCompleted:
	default:
		return false
	}
	state := []byte(checkpoint.StateJSON)
	if len(state) == 0 || !json.Valid(state) {
		return false
	}
	sum := sha256.Sum256(state)
	return checkpoint.StateHash == hex.EncodeToString(sum[:])
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "is not unique")
}
