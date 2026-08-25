package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"magicpodcast/internal/cronexpr"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

const (
	scheduleSkipActiveRun        = "active_run"
	scheduleSkipCurrentArtifact  = "current_artifact"
	scheduleSkipNotFocused       = "not_in_focus"
	scheduleSkipAudioNotReady    = "audio_not_ready"
	scheduleSkipEpisodeMissing   = "episode_not_found"
	scheduleSkipTerminalRun      = "previous_terminal_run"
	scheduleSkipStartFailed      = "start_failed"
	scheduleSkipBatchLimit       = "batch_limit"
	scheduleSelectionPending     = "selection_pending"
	scheduleSkipSelectionStopped = "selection_interrupted"
	scheduleSkipSelectionRestart = "selection_interrupted_by_restart"
)

// SchedulerConfig is startup-loaded operator configuration. It deliberately
// does not persist an enable switch: disabling the scheduler must remain an
// explicit deployment configuration change.
type SchedulerConfig struct {
	Enabled   bool
	Cron      string
	Timezone  string
	BatchSize int
}

type ScheduleStatusProvider interface {
	Status(context.Context) (ScheduleStatus, error)
}

type ScheduleStatus struct {
	Enabled   bool               `json:"enabled"`
	Cron      string             `json:"cron"`
	Timezone  string             `json:"timezone"`
	BatchSize int                `json:"batch_size"`
	NextRunAt *time.Time         `json:"next_run_at,omitempty"`
	LatestRun *ScheduleRunDetail `json:"latest_run,omitempty"`
}

type ScheduleRunDetail struct {
	Run   models.ProcessingScheduleRun    `json:"run"`
	Items []models.ProcessingScheduleItem `json:"items"`
}

type Scheduler struct {
	db       *gorm.DB
	service  *Service
	config   SchedulerConfig
	schedule cron.Schedule
	location *time.Location
	now      func() time.Time
}

type SchedulerOption func(*Scheduler)

func WithSchedulerClock(now func() time.Time) SchedulerOption {
	return func(scheduler *Scheduler) {
		if now != nil {
			scheduler.now = now
		}
	}
}

func NewScheduler(
	db *gorm.DB,
	service *Service,
	config SchedulerConfig,
	options ...SchedulerOption,
) (*Scheduler, error) {
	if db == nil || service == nil {
		return nil, fmt.Errorf("processing scheduler requires database and service")
	}
	normalized, schedule, location, err := parseSchedulerConfig(config)
	if err != nil {
		return nil, err
	}
	scheduler := &Scheduler{
		db:       db,
		service:  service,
		config:   normalized,
		schedule: schedule,
		location: location,
		now:      time.Now,
	}
	for _, option := range options {
		option(scheduler)
	}
	return scheduler, nil
}

// ValidateSchedulerConfig is shared by startup configuration and the runtime
// scheduler so an enabled cron cannot pass one path and fail another.
func ValidateSchedulerConfig(config SchedulerConfig) (SchedulerConfig, error) {
	normalized, _, _, err := parseSchedulerConfig(config)
	return normalized, err
}

func parseSchedulerConfig(
	config SchedulerConfig,
) (SchedulerConfig, cron.Schedule, *time.Location, error) {
	config.Cron = strings.TrimSpace(config.Cron)
	config.Timezone = strings.TrimSpace(config.Timezone)
	if !config.Enabled {
		return config, nil, nil, nil
	}
	if config.Cron == "" || len(config.Cron) > 120 {
		return SchedulerConfig{}, nil, nil, fmt.Errorf("processing schedule cron is required")
	}
	if config.Timezone == "" || len(config.Timezone) > 80 {
		return SchedulerConfig{}, nil, nil, fmt.Errorf("processing schedule timezone is required")
	}
	if config.BatchSize < 1 {
		return SchedulerConfig{}, nil, nil, fmt.Errorf("processing schedule batch_size must be at least 1")
	}
	location, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return SchedulerConfig{}, nil, nil, fmt.Errorf("invalid processing schedule timezone %q: %w", config.Timezone, err)
	}
	normalizedCron, schedule, err := cronexpr.Parse(config.Cron)
	if err != nil {
		return SchedulerConfig{}, nil, nil, fmt.Errorf("invalid processing schedule cron %q: %w", config.Cron, err)
	}
	config.Cron = normalizedCron
	return config, schedule, location, nil
}

// Run waits for future configured schedule instants. It intentionally does not
// replay missed cron instants after a restart: partial batches are closed by
// recovery and the next configured instant makes a fresh, durable selection.
func (s *Scheduler) Run(ctx context.Context) error {
	if !s.config.Enabled {
		return nil
	}
	if err := s.RecoverIncompleteRuns(ctx, s.now().UTC()); err != nil {
		return err
	}
	next := s.schedule.Next(s.now().In(s.location))
	for {
		wait := time.Until(next)
		if wait < 0 {
			next = s.schedule.Next(s.now().In(s.location))
			continue
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil
		case <-timer.C:
			if _, _, err := s.RunAt(ctx, next); err != nil && !errors.Is(err, context.Canceled) {
				logger.Errorf("Focus processing schedule failed: %v", err)
			}
			next = s.schedule.Next(next)
		}
	}
}

// RunAt executes one known planned instant. It is exported for deterministic
// tests and controlled operator diagnostics; normal production triggering is
// only the future-cron loop in Run.
func (s *Scheduler) RunAt(
	ctx context.Context,
	scheduledFor time.Time,
) (ScheduleRunDetail, bool, error) {
	if !s.config.Enabled {
		return ScheduleRunDetail{}, false, fmt.Errorf("processing schedule is disabled")
	}
	if scheduledFor.IsZero() {
		return ScheduleRunDetail{}, false, fmt.Errorf("processing schedule instant is required")
	}
	canonical := scheduledFor.In(s.location).Truncate(time.Second)
	run, duplicate, err := s.claimRun(ctx, canonical)
	if err != nil {
		return ScheduleRunDetail{}, false, err
	}
	if duplicate {
		detail, loadErr := s.loadRunDetail(ctx, run.ID)
		return detail, true, loadErr
	}

	candidates, err := s.planCandidates(ctx, run.ID)
	if err != nil {
		return s.failRun(ctx, run.ID, "candidate_plan_failed", "unable to persist Focus schedule candidates", err)
	}

	started := 0
	hadUnexpectedFailure := false
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return s.failRun(
				ctx,
				run.ID,
				"schedule_interrupted",
				"schedule selection was interrupted",
				err,
			)
		}
		outcome := models.ProcessingScheduleItemOutcomeSkipped
		reason := scheduleSkipBatchLimit
		var processingRunID *uint
		if started < s.config.BatchSize {
			var startErr error
			var itemRecorded bool
			outcome, reason, processingRunID, itemRecorded, startErr = s.startCandidate(
				ctx,
				run.ID,
				candidate,
			)
			if startErr != nil {
				outcome = models.ProcessingScheduleItemOutcomeSkipped
				reason = scheduleSkipStartFailed
				hadUnexpectedFailure = true
			}
			if itemRecorded {
				if outcome == models.ProcessingScheduleItemOutcomeStarted {
					started++
				}
				continue
			}
		}
		if err := s.recordCandidateOutcome(ctx, run.ID, candidate, outcome, reason, processingRunID); err != nil {
			return s.failRun(ctx, run.ID, "schedule_item_record_failed", "unable to record schedule result", err)
		}
		if outcome == models.ProcessingScheduleItemOutcomeStarted {
			started++
		}
	}
	if hadUnexpectedFailure {
		return s.finishRun(ctx, run.ID, models.ProcessingScheduleRunStatusFailed, "candidate_start_failed", "one or more candidates could not be started", s.now().UTC())
	}
	return s.finishRun(ctx, run.ID, models.ProcessingScheduleRunStatusCompleted, "", "", s.now().UTC())
}

func (s *Scheduler) Status(ctx context.Context) (ScheduleStatus, error) {
	status := ScheduleStatus{
		Enabled:   s.config.Enabled,
		Cron:      s.config.Cron,
		Timezone:  s.config.Timezone,
		BatchSize: s.config.BatchSize,
	}
	if s.config.Enabled && s.schedule != nil && s.location != nil {
		next := s.schedule.Next(s.now().In(s.location)).UTC()
		status.NextRunAt = &next
	}
	var latest models.ProcessingScheduleRun
	err := s.db.WithContext(ctx).
		Order("scheduled_for DESC, id DESC").
		First(&latest).Error
	switch {
	case err == nil:
		detail, loadErr := s.loadRunDetail(ctx, latest.ID)
		if loadErr != nil {
			return ScheduleStatus{}, loadErr
		}
		status.LatestRun = &detail
	case errors.Is(err, gorm.ErrRecordNotFound):
	case err != nil:
		return ScheduleStatus{}, fmt.Errorf("read latest processing schedule run: %w", err)
	}
	return status, nil
}

// RecoverIncompleteRuns never repeats a partially planned batch. It preserves
// any already-created processing run, records it when possible, and closes the
// schedule record so the next cron instant can proceed honestly.
func (s *Scheduler) RecoverIncompleteRuns(ctx context.Context, recoveredAt time.Time) error {
	var runs []models.ProcessingScheduleRun
	if err := s.db.WithContext(ctx).
		Where("status = ?", models.ProcessingScheduleRunStatusRunning).
		Order("id ASC").
		Find(&runs).Error; err != nil {
		return fmt.Errorf("list interrupted processing schedule runs: %w", err)
	}
	for _, run := range runs {
		if err := s.backfillStartedItems(ctx, run.ID, recoveredAt.UTC()); err != nil {
			return err
		}
		if err := s.markPendingCandidatesSkipped(
			ctx,
			run.ID,
			scheduleSkipSelectionRestart,
			recoveredAt.UTC(),
		); err != nil {
			return err
		}
		if _, _, err := s.finishRun(
			ctx,
			run.ID,
			models.ProcessingScheduleRunStatusFailed,
			"schedule_interrupted_by_restart",
			"schedule selection was interrupted by service restart",
			recoveredAt.UTC(),
		); err != nil {
			return fmt.Errorf("close interrupted processing schedule run %d: %w", run.ID, err)
		}
	}
	return nil
}

type scheduleCandidate struct {
	EpisodeID     uint
	QueuePosition int64
}

func listFocusCandidates(db *gorm.DB) ([]scheduleCandidate, error) {
	candidates := make([]scheduleCandidate, 0)
	err := db.
		Table("episode_triage_decisions").
		Select("episode_triage_decisions.episode_id, COALESCE(episode_triage_decisions.queue_position, -1) AS queue_position").
		Joins("JOIN episodes ON episodes.id = episode_triage_decisions.episode_id AND episodes.deleted_at IS NULL").
		Where("episode_triage_decisions.queue_state = ?", models.QueueStateFocus).
		Order("episode_triage_decisions.queue_position ASC").
		Order("episode_triage_decisions.episode_id ASC").
		Scan(&candidates).Error
	if err != nil {
		return nil, fmt.Errorf("list Focus schedule candidates: %w", err)
	}
	return candidates, nil
}

// planCandidates atomically snapshots the current Focus candidates and records
// an unresolved row for each before any start attempt. A crash can then be
// represented honestly during recovery instead of losing a skip decision.
func (s *Scheduler) planCandidates(
	ctx context.Context,
	runID uint,
) ([]scheduleCandidate, error) {
	candidates := make([]scheduleCandidate, 0)
	now := s.now().UTC()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var listErr error
		candidates, listErr = listFocusCandidates(tx)
		if listErr != nil {
			return listErr
		}
		for _, candidate := range candidates {
			item := models.ProcessingScheduleItem{
				ScheduleRunID: runID,
				EpisodeID:     candidate.EpisodeID,
				QueuePosition: candidate.QueuePosition,
				Outcome:       models.ProcessingScheduleItemOutcomeSkipped,
				Reason:        scheduleSelectionPending,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			if err := tx.Create(&item).Error; err != nil {
				return fmt.Errorf("reserve Focus schedule candidate: %w", err)
			}
		}
		update := tx.Model(&models.ProcessingScheduleRun{}).
			Where("id = ? AND status = ?", runID, models.ProcessingScheduleRunStatusRunning).
			Updates(map[string]any{
				"candidate_count": len(candidates),
				"started_count":   0,
				"skipped_count":   0,
				"updated_at":      now,
			})
		if update.Error != nil {
			return fmt.Errorf("record processing schedule candidates: %w", update.Error)
		}
		if update.RowsAffected == 0 {
			return fmt.Errorf("processing schedule run %d is no longer active", runID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return candidates, nil
}

func (s *Scheduler) recordCandidateOutcome(
	ctx context.Context,
	runID uint,
	candidate scheduleCandidate,
	outcome string,
	reason string,
	processingRunID *uint,
) error {
	update := s.db.WithContext(ctx).
		Model(&models.ProcessingScheduleItem{}).
		Where(
			"schedule_run_id = ? AND episode_id = ? AND outcome = ? AND reason = ?",
			runID,
			candidate.EpisodeID,
			models.ProcessingScheduleItemOutcomeSkipped,
			scheduleSelectionPending,
		).
		Updates(map[string]any{
			"outcome":           outcome,
			"reason":            reason,
			"processing_run_id": processingRunID,
			"updated_at":        s.now().UTC(),
		})
	if update.Error != nil {
		return fmt.Errorf("record Focus schedule candidate outcome: %w", update.Error)
	}
	if update.RowsAffected == 0 {
		return fmt.Errorf("Focus schedule candidate %d is no longer pending", candidate.EpisodeID)
	}
	return nil
}

func (s *Scheduler) startCandidate(
	ctx context.Context,
	scheduleRunID uint,
	candidate scheduleCandidate,
) (string, string, *uint, bool, error) {
	queuePosition := candidate.QueuePosition
	result, err := s.service.StartEpisodeProcessing(ctx, StartRequest{
		EpisodeID:             candidate.EpisodeID,
		TriggerSource:         models.ProcessingTriggerScheduled,
		RequireReadyAudio:     true,
		ScheduleRunID:         &scheduleRunID,
		ScheduleQueuePosition: &queuePosition,
	})
	switch {
	case err == nil && result.ReusedActive:
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipActiveRun, &result.Run.ID, false, nil
	case err == nil && result.ReusedSuccessful:
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipCurrentArtifact, &result.Run.ID, false, nil
	case err == nil && result.ReusedTerminal:
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipTerminalRun, &result.Run.ID, false, nil
	case err == nil:
		return models.ProcessingScheduleItemOutcomeStarted, "", &result.Run.ID, true, nil
	case errors.Is(err, ErrEpisodeNotFocused):
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipNotFocused, nil, false, nil
	case errors.Is(err, ErrProcessingInputUnavailable):
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipAudioNotReady, nil, false, nil
	case errors.Is(err, ErrEpisodeNotFound):
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipEpisodeMissing, nil, false, nil
	default:
		return "", "", nil, false, err
	}
}

func (s *Scheduler) claimRun(
	ctx context.Context,
	scheduledFor time.Time,
) (models.ProcessingScheduleRun, bool, error) {
	now := s.now().UTC()
	canonical := scheduledFor.UTC()
	run := models.ProcessingScheduleRun{
		TriggerKey:     scheduleTriggerKey(s.config.Cron, s.config.Timezone, canonical),
		ScheduledFor:   canonical,
		CronExpression: s.config.Cron,
		Timezone:       s.config.Timezone,
		BatchSize:      s.config.BatchSize,
		Status:         models.ProcessingScheduleRunStatusRunning,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.db.WithContext(ctx).Create(&run).Error; err != nil {
		if !isUniqueConstraintError(err) {
			return models.ProcessingScheduleRun{}, false, fmt.Errorf("claim processing schedule run: %w", err)
		}
		var existing models.ProcessingScheduleRun
		if queryErr := s.db.WithContext(ctx).
			Where("trigger_key = ?", run.TriggerKey).
			First(&existing).Error; queryErr != nil {
			return models.ProcessingScheduleRun{}, false, fmt.Errorf("read duplicate processing schedule run: %w", queryErr)
		}
		return existing, true, nil
	}
	return run, false, nil
}

func (s *Scheduler) finishRun(
	ctx context.Context,
	runID uint,
	status string,
	errorCode string,
	errorMessage string,
	finishedAt time.Time,
) (ScheduleRunDetail, bool, error) {
	// A cancellation here is normally process shutdown. The selection and its
	// item rows are already durable, so preserve their terminal schedule record
	// instead of leaving a misleading running batch for the next startup.
	durableCtx := context.WithoutCancel(ctx)
	update := s.db.WithContext(durableCtx).
		Model(&models.ProcessingScheduleRun{}).
		Where("id = ? AND status = ?", runID, models.ProcessingScheduleRunStatusRunning).
		Updates(map[string]any{
			"status": status,
			// Count from durable items in the same UPDATE statement. A queued worker
			// can turn a just-started item into a skip before or after this terminal
			// write, but can no longer be overwritten by a stale local counter.
			"started_count": gorm.Expr("(SELECT COUNT(*) FROM processing_schedule_items WHERE schedule_run_id = ? AND outcome = ?)", runID, models.ProcessingScheduleItemOutcomeStarted),
			"skipped_count": gorm.Expr("(SELECT COUNT(*) FROM processing_schedule_items WHERE schedule_run_id = ? AND outcome = ?)", runID, models.ProcessingScheduleItemOutcomeSkipped),
			"error_code":    errorCode,
			"error_message": errorMessage,
			"finished_at":   finishedAt,
			"updated_at":    finishedAt,
		})
	if update.Error != nil {
		return ScheduleRunDetail{}, false, fmt.Errorf("finish processing schedule run: %w", update.Error)
	}
	if update.RowsAffected == 0 {
		return ScheduleRunDetail{}, false, fmt.Errorf("processing schedule run %d is no longer active", runID)
	}
	detail, err := s.loadRunDetail(durableCtx, runID)
	return detail, false, err
}

func (s *Scheduler) failRun(
	ctx context.Context,
	runID uint,
	code string,
	message string,
	cause error,
) (ScheduleRunDetail, bool, error) {
	durableCtx := context.WithoutCancel(ctx)
	now := s.now().UTC()
	if err := s.backfillStartedItems(durableCtx, runID, now); err != nil {
		return ScheduleRunDetail{}, false, err
	}
	if err := s.markPendingCandidatesSkipped(durableCtx, runID, scheduleSkipSelectionStopped, now); err != nil {
		return ScheduleRunDetail{}, false, err
	}
	detail, _, finishErr := s.finishRun(
		durableCtx,
		runID,
		models.ProcessingScheduleRunStatusFailed,
		code,
		message,
		now,
	)
	if finishErr != nil {
		return ScheduleRunDetail{}, false, finishErr
	}
	return detail, false, cause
}

func (s *Scheduler) loadRunDetail(ctx context.Context, runID uint) (ScheduleRunDetail, error) {
	var run models.ProcessingScheduleRun
	if err := s.db.WithContext(ctx).First(&run, runID).Error; err != nil {
		return ScheduleRunDetail{}, fmt.Errorf("read processing schedule run: %w", err)
	}
	items := make([]models.ProcessingScheduleItem, 0)
	if err := s.db.WithContext(ctx).
		Where("schedule_run_id = ?", runID).
		Order("queue_position ASC, id ASC").
		Find(&items).Error; err != nil {
		return ScheduleRunDetail{}, fmt.Errorf("list processing schedule items: %w", err)
	}
	return ScheduleRunDetail{Run: run, Items: items}, nil
}

func (s *Scheduler) backfillStartedItems(
	ctx context.Context,
	scheduleRunID uint,
	now time.Time,
) error {
	var processingRuns []models.EpisodeProcessingRun
	if err := s.db.WithContext(ctx).
		Where("schedule_run_id = ?", scheduleRunID).
		Order("id ASC").
		Find(&processingRuns).Error; err != nil {
		return fmt.Errorf("list scheduled processing runs for recovery: %w", err)
	}
	for _, processingRun := range processingRuns {
		promoted := s.db.WithContext(ctx).
			Model(&models.ProcessingScheduleItem{}).
			Where(
				"schedule_run_id = ? AND episode_id = ? AND outcome = ? AND reason = ? AND processing_run_id IS NULL",
				scheduleRunID,
				processingRun.EpisodeID,
				models.ProcessingScheduleItemOutcomeSkipped,
				scheduleSelectionPending,
			).
			Updates(map[string]any{
				"outcome":           models.ProcessingScheduleItemOutcomeStarted,
				"reason":            "recovered_after_restart",
				"processing_run_id": processingRun.ID,
				"updated_at":        now,
			})
		if promoted.Error != nil {
			return fmt.Errorf("promote recovered schedule item: %w", promoted.Error)
		}
		if promoted.RowsAffected > 0 {
			continue
		}
		var existing int64
		if err := s.db.WithContext(ctx).
			Model(&models.ProcessingScheduleItem{}).
			Where("schedule_run_id = ? AND episode_id = ?", scheduleRunID, processingRun.EpisodeID).
			Count(&existing).Error; err != nil {
			return fmt.Errorf("check recovered schedule item: %w", err)
		}
		if existing > 0 {
			continue
		}
		item := models.ProcessingScheduleItem{
			ScheduleRunID:   scheduleRunID,
			EpisodeID:       processingRun.EpisodeID,
			QueuePosition:   -1,
			Outcome:         models.ProcessingScheduleItemOutcomeStarted,
			Reason:          "recovered_after_restart",
			ProcessingRunID: &processingRun.ID,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := s.db.WithContext(ctx).Create(&item).Error; err != nil && !isUniqueConstraintError(err) {
			return fmt.Errorf("record recovered schedule item: %w", err)
		}
	}
	return nil
}

func (s *Scheduler) markPendingCandidatesSkipped(
	ctx context.Context,
	runID uint,
	reason string,
	now time.Time,
) error {
	update := s.db.WithContext(ctx).
		Model(&models.ProcessingScheduleItem{}).
		Where(
			"schedule_run_id = ? AND outcome = ? AND reason = ?",
			runID,
			models.ProcessingScheduleItemOutcomeSkipped,
			scheduleSelectionPending,
		).
		Updates(map[string]any{
			"reason":     reason,
			"updated_at": now,
		})
	if update.Error != nil {
		return fmt.Errorf("mark interrupted Focus schedule candidates: %w", update.Error)
	}
	return nil
}

func scheduleTriggerKey(cronExpression string, timezone string, scheduledFor time.Time) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		cronExpression,
		timezone,
		scheduledFor.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}
