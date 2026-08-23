package services

import (
	"errors"
	"fmt"
	"time"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

var (
	ErrQueueOrderConflict    = errors.New("consumption queue order conflict")
	ErrInvalidQueuePlacement = errors.New("invalid consumption queue placement")
)

// QueueSnapshot is the canonical ordered view and revision of one queue.
// Revisions change only when this queue's membership or order changes.
type QueueSnapshot struct {
	QueueState string            `json:"queue_state"`
	Revision   int64             `json:"revision"`
	Items      []ConsumptionItem `json:"items"`
	HasMore    bool              `json:"has_more"`
}

type QueuePlacementOptions struct {
	BeforeEpisodeID       *uint
	ExpectedRevisions     map[string]int64
	AcknowledgeFocusLimit bool
}

type QueuePlacementResult struct {
	Queues         map[string]QueueSnapshot `json:"queues"`
	CompletionUndo *CompletionUndo          `json:"completion_undo,omitempty"`
}

func (s *ConsumptionService) moveQueueToHead(
	episodeID uint,
	queueState string,
	options QueueWriteOptions,
) (*models.EpisodeTriageDecision, error) {
	if !ValidQueueState(queueState) {
		return nil, ErrInvalidQueueState
	}

	var result models.EpisodeTriageDecision
	err := s.db.Transaction(func(tx *gorm.DB) error {
		current, err := s.ensureState(tx, episodeID)
		if err != nil {
			return err
		}
		result = *current

		sourceQueue := queueStateForDecision(current)
		if sourceQueue == queueState && current.DismissedAt == nil {
			return nil
		}
		if err := s.requireFocusAcknowledgement(
			tx,
			episodeID,
			sourceQueue,
			queueState,
			options.AcknowledgeFocusLimit,
		); err != nil {
			return err
		}

		targetIDs, err := queueEpisodeIDs(tx, queueState)
		if err != nil {
			return err
		}
		targetIDs = removeEpisodeID(targetIDs, episodeID)

		var sourceIDs []uint
		if sourceQueue != "" && sourceQueue != queueState {
			sourceIDs, err = queueEpisodeIDs(tx, sourceQueue)
			if err != nil {
				return err
			}
			sourceIDs = removeEpisodeID(sourceIDs, episodeID)
		}

		if err := s.moveDecisionToQueue(tx, current, queueState); err != nil {
			return err
		}
		if sourceQueue != "" && sourceQueue != queueState {
			if err := resequenceQueue(tx, sourceQueue, sourceIDs); err != nil {
				return err
			}
		}
		targetIDs = append([]uint{episodeID}, targetIDs...)
		if err := resequenceQueue(tx, queueState, targetIDs); err != nil {
			return err
		}
		if err := s.bumpQueueRevisions(tx, affectedQueueStates(sourceQueue, queueState)); err != nil {
			return err
		}

		result = models.EpisodeTriageDecision{}
		return tx.Where("episode_id = ?", episodeID).First(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *ConsumptionService) clearQueueState(episodeID uint) (*models.EpisodeTriageDecision, error) {
	var result models.EpisodeTriageDecision
	err := s.db.Transaction(func(tx *gorm.DB) error {
		current, err := s.ensureState(tx, episodeID)
		if err != nil {
			return err
		}
		result = *current
		if current.QueueState == nil && current.DismissedAt == nil {
			return nil
		}

		sourceQueue := queueStateForDecision(current)
		var sourceIDs []uint
		if sourceQueue != "" {
			sourceIDs, err = queueEpisodeIDs(tx, sourceQueue)
			if err != nil {
				return err
			}
			sourceIDs = removeEpisodeID(sourceIDs, episodeID)
		}

		now := s.now().UTC()
		if err := tx.Model(current).Updates(map[string]any{
			"queue_state":      nil,
			"queue_position":   nil,
			"dismissed_at":     nil,
			"queue_updated_at": now,
			"state":            models.TriageStatePending,
			"decided_at":       now,
		}).Error; err != nil {
			return err
		}
		if sourceQueue != "" {
			if err := resequenceQueue(tx, sourceQueue, sourceIDs); err != nil {
				return err
			}
			if err := s.bumpQueueRevisions(tx, []string{sourceQueue}); err != nil {
				return err
			}
		}

		result = models.EpisodeTriageDecision{}
		return tx.Where("episode_id = ?", episodeID).First(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *ConsumptionService) setDismissedState(
	episodeID uint,
	dismissed bool,
) (*models.EpisodeTriageDecision, error) {
	var result models.EpisodeTriageDecision
	err := s.db.Transaction(func(tx *gorm.DB) error {
		current, err := s.ensureState(tx, episodeID)
		if err != nil {
			return err
		}
		result = *current
		if dismissed && current.DismissedAt != nil && current.QueueState == nil {
			return nil
		}
		if !dismissed && current.DismissedAt == nil && current.QueueState == nil {
			return nil
		}

		sourceQueue := queueStateForDecision(current)
		var sourceIDs []uint
		if sourceQueue != "" {
			sourceIDs, err = queueEpisodeIDs(tx, sourceQueue)
			if err != nil {
				return err
			}
			sourceIDs = removeEpisodeID(sourceIDs, episodeID)
		}

		now := s.now().UTC()
		state := models.TriageStatePending
		var dismissedAt any
		if dismissed {
			state = models.TriageStateDiscarded
			dismissedAt = now
		}
		if err := tx.Model(current).Updates(map[string]any{
			"queue_state":    nil,
			"queue_position": nil,
			"dismissed_at":   dismissedAt,
			"state":          state,
			"decided_at":     now,
		}).Error; err != nil {
			return err
		}
		if sourceQueue != "" {
			if err := resequenceQueue(tx, sourceQueue, sourceIDs); err != nil {
				return err
			}
			if err := s.bumpQueueRevisions(tx, []string{sourceQueue}); err != nil {
				return err
			}
		}

		result = models.EpisodeTriageDecision{}
		return tx.Where("episode_id = ?", episodeID).First(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// PlaceQueue applies an exact placement using the revisions that were visible
// to the caller. A revision mismatch leaves all queue data untouched.
func (s *ConsumptionService) PlaceQueue(
	episodeID uint,
	queueState string,
	options QueuePlacementOptions,
) (QueuePlacementResult, error) {
	if !ValidQueueState(queueState) {
		return QueuePlacementResult{}, ErrInvalidQueueState
	}
	if queueState == models.QueueStateDone {
		mutation, err := s.completeEpisode(episodeID, options.ExpectedRevisions)
		if err != nil {
			return QueuePlacementResult{}, err
		}
		return QueuePlacementResult{
			Queues:         mutation.queues,
			CompletionUndo: mutation.undo,
		}, nil
	}

	var result QueuePlacementResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		current, err := s.ensureState(tx, episodeID)
		if err != nil {
			return err
		}
		sourceQueue := queueStateForDecision(current)
		affected := affectedQueueStates(sourceQueue, queueState)
		if err := validateExpectedRevisions(tx, sourceQueue, queueState, options.ExpectedRevisions); err != nil {
			return err
		}
		if err := s.requireFocusAcknowledgement(
			tx,
			episodeID,
			sourceQueue,
			queueState,
			options.AcknowledgeFocusLimit,
		); err != nil {
			return err
		}

		targetIDs, err := queueEpisodeIDs(tx, queueState)
		if err != nil {
			return err
		}
		if options.BeforeEpisodeID != nil {
			if *options.BeforeEpisodeID == episodeID && sourceQueue == queueState {
				result, err = s.snapshotsForQueues(tx, affected)
				return err
			}
			if !containsEpisodeID(targetIDs, *options.BeforeEpisodeID) {
				return fmt.Errorf("%w: target episode is not in %s", ErrInvalidQueuePlacement, queueState)
			}
		}

		targetWithoutMoving := removeEpisodeID(targetIDs, episodeID)
		placedTargetIDs, err := insertEpisodeBefore(
			targetWithoutMoving,
			episodeID,
			options.BeforeEpisodeID,
		)
		if err != nil {
			return err
		}

		if sourceQueue == queueState {
			if sameEpisodeIDs(targetIDs, placedTargetIDs) {
				result, err = s.snapshotsForQueues(tx, affected)
				return err
			}
			if err := resequenceQueue(tx, queueState, placedTargetIDs); err != nil {
				return err
			}
		} else {
			var sourceIDs []uint
			if sourceQueue != "" {
				sourceIDs, err = queueEpisodeIDs(tx, sourceQueue)
				if err != nil {
					return err
				}
				sourceIDs = removeEpisodeID(sourceIDs, episodeID)
			}
			if err := s.moveDecisionToQueue(tx, current, queueState); err != nil {
				return err
			}
			if sourceQueue != "" {
				if err := resequenceQueue(tx, sourceQueue, sourceIDs); err != nil {
					return err
				}
			}
			if err := resequenceQueue(tx, queueState, placedTargetIDs); err != nil {
				return err
			}
		}

		if err := s.bumpQueueRevisions(tx, affected); err != nil {
			return err
		}
		result, err = s.snapshotsForQueues(tx, affected)
		return err
	})
	if err != nil {
		return QueuePlacementResult{}, err
	}
	return result, nil
}

func (s *ConsumptionService) listQueueSnapshot(queueState string) (QueueSnapshot, error) {
	if !ValidQueueState(queueState) {
		return QueueSnapshot{}, ErrInvalidQueueState
	}
	var snapshot QueueSnapshot
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot, err = s.readQueueSnapshot(tx, queueState)
		return err
	})
	if err != nil {
		return QueueSnapshot{}, err
	}
	return snapshot, nil
}

func (s *ConsumptionService) readQueueSnapshot(
	tx *gorm.DB,
	queueState string,
) (QueueSnapshot, error) {
	revision, err := queueRevision(tx, queueState)
	if err != nil {
		return QueueSnapshot{}, err
	}
	if queueState == models.QueueStateDone {
		return s.readRecentCompletionSnapshot(tx, revision)
	}
	var states []models.EpisodeTriageDecision
	if err := tx.
		Joins("JOIN episodes ON episodes.id = episode_triage_decisions.episode_id AND episodes.deleted_at IS NULL").
		Preload("Episode.Podcast").
		Preload("Episode.Tags").
		Where("episode_triage_decisions.queue_state = ?", queueState).
		Order("episode_triage_decisions.queue_position ASC").
		Order("episode_triage_decisions.episode_id ASC").
		Find(&states).Error; err != nil {
		return QueueSnapshot{}, err
	}

	episodeIDs := make([]uint, len(states))
	for index := range states {
		episodeIDs[index] = states[index].EpisodeID
	}
	completionTimes, err := completionTimesForEpisodeIDs(tx, episodeIDs)
	if err != nil {
		return QueueSnapshot{}, err
	}
	now := s.now().UTC()
	items := make([]ConsumptionItem, 0, len(states))
	for index := range states {
		var completedAt *time.Time
		if value, exists := completionTimes[states[index].EpisodeID]; exists {
			valueCopy := value
			completedAt = &valueCopy
		}
		items = append(items, buildConsumptionItem(states[index], completedAt, now))
	}
	return QueueSnapshot{
		QueueState: queueState,
		Revision:   revision,
		Items:      items,
		HasMore:    false,
	}, nil
}

func (s *ConsumptionService) readRecentCompletionSnapshot(
	tx *gorm.DB,
	revision int64,
) (QueueSnapshot, error) {
	var states []models.EpisodeTriageDecision
	if err := tx.
		Joins("JOIN episodes ON episodes.id = episode_triage_decisions.episode_id AND episodes.deleted_at IS NULL").
		Joins("JOIN episode_completions ON episode_completions.episode_id = episode_triage_decisions.episode_id").
		Preload("Episode.Podcast").
		Preload("Episode.Tags").
		Where("episode_triage_decisions.queue_state = ?", models.QueueStateDone).
		Where("episode_completions.completed_at >= ?", s.now().UTC().Add(-RecentCompletionWindow)).
		Order("episode_completions.completed_at DESC").
		Order("episode_triage_decisions.episode_id DESC").
		Limit(RecentCompletionLimit + 1).
		Find(&states).Error; err != nil {
		return QueueSnapshot{}, err
	}

	hasMore := len(states) > RecentCompletionLimit
	if hasMore {
		states = states[:RecentCompletionLimit]
	}
	episodeIDs := make([]uint, len(states))
	for index := range states {
		episodeIDs[index] = states[index].EpisodeID
	}
	completionTimes, err := completionTimesForEpisodeIDs(tx, episodeIDs)
	if err != nil {
		return QueueSnapshot{}, err
	}
	now := s.now().UTC()
	items := make([]ConsumptionItem, 0, len(states))
	for index := range states {
		completedAt := completionTimes[states[index].EpisodeID]
		items = append(items, buildConsumptionItem(states[index], &completedAt, now))
	}
	return QueueSnapshot{
		QueueState: models.QueueStateDone,
		Revision:   revision,
		Items:      items,
		HasMore:    hasMore,
	}, nil
}

func (s *ConsumptionService) snapshotsForQueues(
	tx *gorm.DB,
	queueStates []string,
) (QueuePlacementResult, error) {
	result := QueuePlacementResult{Queues: make(map[string]QueueSnapshot, len(queueStates))}
	for _, queueState := range queueStates {
		snapshot, err := s.readQueueSnapshot(tx, queueState)
		if err != nil {
			return QueuePlacementResult{}, err
		}
		result.Queues[queueState] = snapshot
	}
	return result, nil
}

func (s *ConsumptionService) requireFocusAcknowledgement(
	tx *gorm.DB,
	episodeID uint,
	sourceQueue string,
	targetQueue string,
	acknowledged bool,
) error {
	if targetQueue != models.QueueStateFocus || sourceQueue == models.QueueStateFocus || acknowledged {
		return nil
	}
	var focusCount int64
	if err := tx.Model(&models.EpisodeTriageDecision{}).
		Where("queue_state = ? AND episode_id <> ?", models.QueueStateFocus, episodeID).
		Count(&focusCount).Error; err != nil {
		return err
	}
	if focusCount >= FocusSoftLimit {
		return &FocusLimitConfirmationError{CurrentCount: int(focusCount)}
	}
	return nil
}

func (s *ConsumptionService) moveDecisionToQueue(
	tx *gorm.DB,
	current *models.EpisodeTriageDecision,
	queueState string,
) error {
	now := s.now().UTC()
	position := int64(0)
	updates := map[string]any{
		"queue_state":      queueState,
		"queue_position":   position,
		"dismissed_at":     nil,
		"queue_updated_at": now,
		"state":            models.TriageStateShortlisted,
		"decided_at":       now,
	}
	if queueState == models.QueueStateDone {
		updates["in_progress_at"] = nil
	}
	return tx.Model(current).Updates(updates).Error
}

func (s *ConsumptionService) bumpQueueRevisions(tx *gorm.DB, queueStates []string) error {
	for _, queueState := range queueStates {
		order, err := ensureQueueOrder(tx, queueState)
		if err != nil {
			return err
		}
		if err := tx.Model(&models.ConsumptionQueueOrder{}).
			Where("queue_state = ?", queueState).
			Updates(map[string]any{
				"revision":   order.Revision + 1,
				"updated_at": s.now().UTC(),
			}).Error; err != nil {
			return fmt.Errorf("increment %s queue revision: %w", queueState, err)
		}
	}
	return nil
}

func validateExpectedRevisions(
	tx *gorm.DB,
	sourceQueue string,
	targetQueue string,
	expected map[string]int64,
) error {
	queueStates := affectedQueueStates(sourceQueue, targetQueue)
	allowedQueues := make(map[string]struct{}, len(queueStates))
	for _, queueState := range queueStates {
		allowedQueues[queueState] = struct{}{}
	}
	for queueState := range expected {
		if _, ok := allowedQueues[queueState]; !ok {
			return fmt.Errorf("%w: item is no longer in %s", ErrQueueOrderConflict, queueState)
		}
	}
	for _, queueState := range queueStates {
		expectedRevision, exists := expected[queueState]
		if !exists || expectedRevision < 1 {
			if queueState != targetQueue && sourceQueue != "" {
				return fmt.Errorf("%w: source queue changed to %s", ErrQueueOrderConflict, sourceQueue)
			}
			return fmt.Errorf("%w: expected revision for %s is required", ErrInvalidQueuePlacement, queueState)
		}
		actualRevision, err := queueRevision(tx, queueState)
		if err != nil {
			return err
		}
		if actualRevision != expectedRevision {
			return fmt.Errorf("%w: %s expected=%d actual=%d", ErrQueueOrderConflict, queueState, expectedRevision, actualRevision)
		}
	}
	return nil
}

func ensureQueueOrder(tx *gorm.DB, queueState string) (*models.ConsumptionQueueOrder, error) {
	var order models.ConsumptionQueueOrder
	err := tx.Where("queue_state = ?", queueState).First(&order).Error
	if err == nil {
		return &order, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("read %s queue revision: %w", queueState, err)
	}
	order = models.ConsumptionQueueOrder{QueueState: queueState, Revision: 1}
	if err := tx.Create(&order).Error; err != nil {
		return nil, fmt.Errorf("create %s queue revision: %w", queueState, err)
	}
	return &order, nil
}

func queueRevision(tx *gorm.DB, queueState string) (int64, error) {
	var order models.ConsumptionQueueOrder
	err := tx.Where("queue_state = ?", queueState).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, fmt.Errorf("queue revision is missing for %s", queueState)
	}
	if err != nil {
		return 0, fmt.Errorf("read %s queue revision: %w", queueState, err)
	}
	return order.Revision, nil
}

func queueEpisodeIDs(tx *gorm.DB, queueState string) ([]uint, error) {
	var episodeIDs []uint
	if err := tx.Model(&models.EpisodeTriageDecision{}).
		Where("queue_state = ?", queueState).
		Order("queue_position ASC").
		Order("episode_id ASC").
		Pluck("episode_id", &episodeIDs).Error; err != nil {
		return nil, fmt.Errorf("read %s queue positions: %w", queueState, err)
	}
	return episodeIDs, nil
}

func resequenceQueue(tx *gorm.DB, queueState string, episodeIDs []uint) error {
	for position, episodeID := range episodeIDs {
		if err := tx.Exec(
			"UPDATE episode_triage_decisions SET queue_position = ? WHERE queue_state = ? AND episode_id = ?",
			position,
			queueState,
			episodeID,
		).Error; err != nil {
			return fmt.Errorf("update %s queue position: %w", queueState, err)
		}
	}
	return nil
}

func queueStateForDecision(decision *models.EpisodeTriageDecision) string {
	if decision.QueueState == nil || !ValidQueueState(*decision.QueueState) {
		return ""
	}
	return *decision.QueueState
}

func affectedQueueStates(sourceQueue, targetQueue string) []string {
	if sourceQueue == "" || sourceQueue == targetQueue {
		return []string{targetQueue}
	}
	return []string{sourceQueue, targetQueue}
}

func removeEpisodeID(episodeIDs []uint, episodeID uint) []uint {
	result := make([]uint, 0, len(episodeIDs))
	for _, candidate := range episodeIDs {
		if candidate != episodeID {
			result = append(result, candidate)
		}
	}
	return result
}

func containsEpisodeID(episodeIDs []uint, episodeID uint) bool {
	for _, candidate := range episodeIDs {
		if candidate == episodeID {
			return true
		}
	}
	return false
}

func insertEpisodeBefore(
	episodeIDs []uint,
	episodeID uint,
	beforeEpisodeID *uint,
) ([]uint, error) {
	insertAt := len(episodeIDs)
	if beforeEpisodeID != nil {
		insertAt = -1
		for index, candidate := range episodeIDs {
			if candidate == *beforeEpisodeID {
				insertAt = index
				break
			}
		}
		if insertAt < 0 {
			return nil, fmt.Errorf("%w: target episode is unavailable", ErrInvalidQueuePlacement)
		}
	}
	result := make([]uint, 0, len(episodeIDs)+1)
	result = append(result, episodeIDs[:insertAt]...)
	result = append(result, episodeID)
	result = append(result, episodeIDs[insertAt:]...)
	return result, nil
}

func sameEpisodeIDs(left, right []uint) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
