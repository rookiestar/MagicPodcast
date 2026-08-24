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
	scheduleSkipActiveRun       = "active_run"
	scheduleSkipCurrentArtifact = "current_artifact"
	scheduleSkipNotFocused      = "not_in_focus"
	scheduleSkipAudioNotReady   = "audio_not_ready"
	scheduleSkipEpisodeMissing  = "episode_not_found"
	scheduleSkipTerminalRun     = "previous_terminal_run"
	scheduleSkipStartFailed     = "start_failed"
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

	candidates, err := s.listFocusCandidates(ctx)
	if err != nil {
		return s.failRun(ctx, run.ID, "candidate_list_failed", "unable to list Focus candidates", err)
	}
	if err := s.updateRunProgress(ctx, run.ID, len(candidates), 0, 0); err != nil {
		return s.failRun(ctx, run.ID, "schedule_progress_failed", "unable to record schedule candidates", err)
	}

	started := 0
	skipped := 0
	hadUnexpectedFailure := false
	for _, candidate := range candidates {
		if started >= s.config.BatchSize {
			break
		}
		if err := ctx.Err(); err != nil {
			return s.failRun(
				ctx,
				run.ID,
				"schedule_interrupted",
				"schedule selection was interrupted",
				err,
			)
		}
		outcome, reason, processingRunID, startErr := s.startCandidate(ctx, run.ID, candidate.EpisodeID)
		if startErr != nil {
			outcome = models.ProcessingScheduleItemOutcomeSkipped
			reason = scheduleSkipStartFailed
			hadUnexpectedFailure = true
		}
		item := models.ProcessingScheduleItem{
			ScheduleRunID:   run.ID,
			EpisodeID:       candidate.EpisodeID,
			QueuePosition:   candidate.QueuePosition,
			Outcome:         outcome,
			Reason:          reason,
			ProcessingRunID: processingRunID,
			CreatedAt:       s.now().UTC(),
			UpdatedAt:       s.now().UTC(),
		}
		if err := s.db.WithContext(ctx).Create(&item).Error; err != nil {
			return s.failRun(ctx, run.ID, "schedule_item_record_failed", "unable to record schedule result", err)
		}
		if outcome == models.ProcessingScheduleItemOutcomeStarted {
			started++
		} else {
			skipped++
		}
	}
	if hadUnexpectedFailure {
		return s.finishRun(ctx, run.ID, started, skipped, models.ProcessingScheduleRunStatusFailed, "candidate_start_failed", "one or more candidates could not be started")
	}
	return s.finishRun(ctx, run.ID, started, skipped, models.ProcessingScheduleRunStatusCompleted, "", "")
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
	if !s.config.Enabled {
		return nil
	}
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
		started, skipped, err := s.runCounts(ctx, run.ID)
		if err != nil {
			return err
		}
		update := s.db.WithContext(ctx).
			Model(&models.ProcessingScheduleRun{}).
			Where("id = ? AND status = ?", run.ID, models.ProcessingScheduleRunStatusRunning).
			Updates(map[string]any{
				"status":        models.ProcessingScheduleRunStatusFailed,
				"started_count": started,
				"skipped_count": skipped,
				"error_code":    "schedule_interrupted_by_restart",
				"error_message": "schedule selection was interrupted by service restart",
				"finished_at":   recoveredAt.UTC(),
				"updated_at":    recoveredAt.UTC(),
			})
		if update.Error != nil {
			return fmt.Errorf("close interrupted processing schedule run %d: %w", run.ID, update.Error)
		}
	}
	return nil
}

type scheduleCandidate struct {
	EpisodeID     uint
	QueuePosition int64
}

func (s *Scheduler) listFocusCandidates(ctx context.Context) ([]scheduleCandidate, error) {
	candidates := make([]scheduleCandidate, 0)
	err := s.db.WithContext(ctx).
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

func (s *Scheduler) startCandidate(
	ctx context.Context,
	scheduleRunID uint,
	episodeID uint,
) (string, string, *uint, error) {
	result, err := s.service.StartEpisodeProcessing(ctx, StartRequest{
		EpisodeID:         episodeID,
		TriggerSource:     models.ProcessingTriggerScheduled,
		RequireReadyAudio: true,
		ScheduleRunID:     &scheduleRunID,
	})
	switch {
	case err == nil && result.ReusedActive:
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipActiveRun, &result.Run.ID, nil
	case err == nil && result.ReusedSuccessful:
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipCurrentArtifact, &result.Run.ID, nil
	case err == nil && result.ReusedTerminal:
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipTerminalRun, &result.Run.ID, nil
	case err == nil:
		return models.ProcessingScheduleItemOutcomeStarted, "", &result.Run.ID, nil
	case errors.Is(err, ErrEpisodeNotFocused):
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipNotFocused, nil, nil
	case errors.Is(err, ErrProcessingInputUnavailable):
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipAudioNotReady, nil, nil
	case errors.Is(err, ErrEpisodeNotFound):
		return models.ProcessingScheduleItemOutcomeSkipped, scheduleSkipEpisodeMissing, nil, nil
	default:
		return "", "", nil, err
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

func (s *Scheduler) updateRunProgress(
	ctx context.Context,
	runID uint,
	candidateCount int,
	started int,
	skipped int,
) error {
	update := s.db.WithContext(ctx).
		Model(&models.ProcessingScheduleRun{}).
		Where("id = ? AND status = ?", runID, models.ProcessingScheduleRunStatusRunning).
		Updates(map[string]any{
			"candidate_count": candidateCount,
			"started_count":   started,
			"skipped_count":   skipped,
			"updated_at":      s.now().UTC(),
		})
	if update.Error != nil {
		return fmt.Errorf("update processing schedule run: %w", update.Error)
	}
	if update.RowsAffected == 0 {
		return fmt.Errorf("processing schedule run %d is no longer active", runID)
	}
	return nil
}

func (s *Scheduler) finishRun(
	ctx context.Context,
	runID uint,
	started int,
	skipped int,
	status string,
	errorCode string,
	errorMessage string,
) (ScheduleRunDetail, bool, error) {
	now := s.now().UTC()
	// A cancellation here is normally process shutdown. The selection and its
	// item rows are already durable, so preserve their terminal schedule record
	// instead of leaving a misleading running batch for the next startup.
	durableCtx := context.WithoutCancel(ctx)
	update := s.db.WithContext(durableCtx).
		Model(&models.ProcessingScheduleRun{}).
		Where("id = ? AND status = ?", runID, models.ProcessingScheduleRunStatusRunning).
		Updates(map[string]any{
			"status":        status,
			"started_count": started,
			"skipped_count": skipped,
			"error_code":    errorCode,
			"error_message": errorMessage,
			"finished_at":   now,
			"updated_at":    now,
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
	started, skipped, err := s.runCounts(durableCtx, runID)
	if err != nil {
		return ScheduleRunDetail{}, false, err
	}
	detail, _, finishErr := s.finishRun(
		durableCtx,
		runID,
		started,
		skipped,
		models.ProcessingScheduleRunStatusFailed,
		code,
		message,
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

func (s *Scheduler) runCounts(ctx context.Context, runID uint) (int, int, error) {
	type countRow struct {
		Outcome string
		Count   int
	}
	var rows []countRow
	if err := s.db.WithContext(ctx).
		Model(&models.ProcessingScheduleItem{}).
		Select("outcome, COUNT(*) AS count").
		Where("schedule_run_id = ?", runID).
		Group("outcome").
		Scan(&rows).Error; err != nil {
		return 0, 0, fmt.Errorf("count processing schedule items: %w", err)
	}
	started := 0
	skipped := 0
	for _, row := range rows {
		switch row.Outcome {
		case models.ProcessingScheduleItemOutcomeStarted:
			started = row.Count
		case models.ProcessingScheduleItemOutcomeSkipped:
			skipped = row.Count
		}
	}
	return started, skipped, nil
}

func scheduleTriggerKey(cronExpression string, timezone string, scheduledFor time.Time) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		cronExpression,
		timezone,
		scheduledFor.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}
