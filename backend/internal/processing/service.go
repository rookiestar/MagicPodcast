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
	db            *gorm.DB
	inputResolver ProcessingInputResolver
	retryPolicy   RetryPolicy
	now           func() time.Time
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
		return StartResult{}, ErrProcessingInputUnavailable
	}
	input, err = normalizeProcessingInput(input)
	if err != nil {
		return StartResult{}, err
	}
	return s.startResolvedEpisodeProcessing(ctx, normalized, input)
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
	Run        models.EpisodeProcessingRun `json:"run"`
	Artifact   *models.EpisodeArtifactSet  `json:"artifact,omitempty"`
	Deliveries []models.KnowledgeDelivery  `json:"deliveries"`
}

func (s *Service) GetProcessingRun(ctx context.Context, runID uint) (RunDetail, error) {
	run, err := s.getRunModel(ctx, runID)
	if err != nil {
		return RunDetail{}, err
	}
	detail := RunDetail{Run: run, Deliveries: []models.KnowledgeDelivery{}}

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
	return detail, nil
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

type RecoveryResult struct {
	RecoverableRunIDs []uint `json:"recoverable_run_ids"`
	FailedRunIDs      []uint `json:"failed_run_ids"`
	FailedDeliveryIDs []uint `json:"failed_delivery_ids"`
}

// RecoverNonTerminalRuns is called once after a service restart. Queued work
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
