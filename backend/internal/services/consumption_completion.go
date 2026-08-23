package services

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	RecentCompletionWindow = 168 * time.Hour
	RecentCompletionLimit  = 20
	CompletionUndoWindow   = 15 * time.Second
)

var (
	ErrInvalidCompletionUndo  = errors.New("invalid completion undo token")
	ErrCompletionUndoExpired  = errors.New("completion undo expired")
	ErrCompletionUndoConflict = errors.New(
		"completion undo conflicts with newer state",
	)
)

type CompletionUndo struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type QueueWriteResult struct {
	Decision       *models.EpisodeTriageDecision
	CompletionUndo *CompletionUndo
}

type completionMutation struct {
	decision *models.EpisodeTriageDecision
	queues   map[string]QueueSnapshot
	undo     *CompletionUndo
}

type completionUndoDecision struct {
	State          string     `json:"state"`
	DecidedAt      time.Time  `json:"decided_at"`
	QueueState     *string    `json:"queue_state,omitempty"`
	QueuePosition  *int64     `json:"queue_position,omitempty"`
	DismissedAt    *time.Time `json:"dismissed_at,omitempty"`
	QueueUpdatedAt *time.Time `json:"queue_updated_at,omitempty"`
	InProgressAt   *time.Time `json:"in_progress_at,omitempty"`
}

type completionUndoPayload struct {
	Version                 int                    `json:"version"`
	EpisodeID               uint                   `json:"episode_id"`
	CompletedAt             time.Time              `json:"completed_at"`
	ExpiresAt               time.Time              `json:"expires_at"`
	Original                completionUndoDecision `json:"original"`
	OriginalBeforeEpisodeID *uint                  `json:"original_before_episode_id,omitempty"`
	PreviousCompletedAt     *time.Time             `json:"previous_completed_at,omitempty"`
	ExpectedRevisions       map[string]int64       `json:"expected_revisions"`
}

func newCompletionUndoKey() []byte {
	key := make([]byte, sha256.Size)
	if _, err := rand.Read(key); err == nil {
		return key
	}
	fallback := sha256.Sum256([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	return fallback[:]
}

func (s *ConsumptionService) SetQueueWithResult(
	episodeID uint,
	queueState string,
	options QueueWriteOptions,
) (QueueWriteResult, error) {
	if queueState != models.QueueStateDone {
		decision, err := s.moveQueueToHead(episodeID, queueState, options)
		return QueueWriteResult{Decision: decision}, err
	}
	if !ValidQueueState(queueState) {
		return QueueWriteResult{}, ErrInvalidQueueState
	}
	mutation, err := s.completeEpisode(episodeID, nil)
	if err != nil {
		return QueueWriteResult{}, err
	}
	return QueueWriteResult{
		Decision:       mutation.decision,
		CompletionUndo: mutation.undo,
	}, nil
}

func (s *ConsumptionService) completeEpisode(
	episodeID uint,
	expectedRevisions map[string]int64,
) (completionMutation, error) {
	var mutation completionMutation
	err := s.db.Transaction(func(tx *gorm.DB) error {
		current, err := s.ensureState(tx, episodeID)
		if err != nil {
			return err
		}
		sourceQueue := queueStateForDecision(current)
		if expectedRevisions != nil {
			if err := validateExpectedRevisions(
				tx,
				sourceQueue,
				models.QueueStateDone,
				expectedRevisions,
			); err != nil {
				return err
			}
		}

		original := snapshotCompletionUndoDecision(current)

		var sourceIDs []uint
		var originalBeforeEpisodeID *uint
		if sourceQueue != "" {
			sourceIDs, err = queueEpisodeIDs(tx, sourceQueue)
			if err != nil {
				return err
			}
			originalBeforeEpisodeID = episodeAfter(sourceIDs, episodeID)
		}

		previousCompletedAt, err := completionTimeForEpisode(tx, episodeID)
		if err != nil {
			return err
		}
		completedAt := s.now().UTC()
		if sourceQueue == models.QueueStateDone {
			if err := tx.Model(current).Updates(map[string]any{
				"dismissed_at":     nil,
				"queue_updated_at": completedAt,
				"in_progress_at":   nil,
				"state":            models.TriageStateShortlisted,
				"decided_at":       completedAt,
			}).Error; err != nil {
				return err
			}
		} else {
			targetIDs, err := queueEpisodeIDs(tx, models.QueueStateDone)
			if err != nil {
				return err
			}
			targetIDs = removeEpisodeID(targetIDs, episodeID)
			if sourceQueue != "" {
				sourceIDs = removeEpisodeID(sourceIDs, episodeID)
			}
			if err := s.moveDecisionToQueue(tx, current, models.QueueStateDone); err != nil {
				return err
			}
			if sourceQueue != "" {
				if err := resequenceQueue(tx, sourceQueue, sourceIDs); err != nil {
					return err
				}
			}
			targetIDs = append([]uint{episodeID}, targetIDs...)
			if err := resequenceQueue(tx, models.QueueStateDone, targetIDs); err != nil {
				return err
			}
		}

		if err := upsertCompletion(tx, episodeID, completedAt); err != nil {
			return err
		}
		affected := affectedQueueStates(sourceQueue, models.QueueStateDone)
		if err := s.bumpQueueRevisions(tx, affected); err != nil {
			return err
		}
		snapshots, err := s.snapshotsForQueues(tx, affected)
		if err != nil {
			return err
		}
		var decision models.EpisodeTriageDecision
		if err := tx.Where("episode_id = ?", episodeID).First(&decision).Error; err != nil {
			return err
		}
		revisions := make(map[string]int64, len(snapshots.Queues))
		for queueState, snapshot := range snapshots.Queues {
			revisions[queueState] = snapshot.Revision
		}
		expiresAt := completedAt.Add(CompletionUndoWindow)
		token, err := s.signCompletionUndo(completionUndoPayload{
			Version:                 1,
			EpisodeID:               episodeID,
			CompletedAt:             completedAt,
			ExpiresAt:               expiresAt,
			Original:                original,
			OriginalBeforeEpisodeID: originalBeforeEpisodeID,
			PreviousCompletedAt:     previousCompletedAt,
			ExpectedRevisions:       revisions,
		})
		if err != nil {
			return err
		}
		mutation = completionMutation{
			decision: &decision,
			queues:   snapshots.Queues,
			undo:     &CompletionUndo{Token: token, ExpiresAt: expiresAt},
		}
		return nil
	})
	if err != nil {
		return completionMutation{}, err
	}
	return mutation, nil
}

func (s *ConsumptionService) UndoCompletion(
	episodeID uint,
	token string,
) (QueuePlacementResult, error) {
	payload, err := s.verifyCompletionUndo(token)
	if err != nil {
		return QueuePlacementResult{}, err
	}
	if payload.EpisodeID != episodeID {
		return QueuePlacementResult{}, ErrInvalidCompletionUndo
	}
	if s.now().UTC().After(payload.ExpiresAt) {
		return QueuePlacementResult{}, ErrCompletionUndoExpired
	}

	var result QueuePlacementResult
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var current models.EpisodeTriageDecision
		if err := tx.Where("episode_id = ?", episodeID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCompletionUndoConflict
			}
			return err
		}
		if current.QueueState == nil || *current.QueueState != models.QueueStateDone {
			return ErrCompletionUndoConflict
		}
		currentCompletedAt, err := completionTimeForEpisode(tx, episodeID)
		if err != nil {
			return err
		}
		if currentCompletedAt == nil || !currentCompletedAt.Equal(payload.CompletedAt) {
			return ErrCompletionUndoConflict
		}
		for queueState, expectedRevision := range payload.ExpectedRevisions {
			actualRevision, err := queueRevision(tx, queueState)
			if err != nil {
				return err
			}
			if actualRevision != expectedRevision {
				return ErrCompletionUndoConflict
			}
		}

		originalQueue := ""
		if payload.Original.QueueState != nil {
			originalQueue = *payload.Original.QueueState
		}
		doneIDs, err := queueEpisodeIDs(tx, models.QueueStateDone)
		if err != nil {
			return err
		}
		doneIDs = removeEpisodeID(doneIDs, episodeID)

		var originalQueueIDs []uint
		if originalQueue != "" && originalQueue != models.QueueStateDone {
			originalQueueIDs, err = queueEpisodeIDs(tx, originalQueue)
			if err != nil {
				return err
			}
			originalQueueIDs, err = insertEpisodeBefore(
				originalQueueIDs,
				episodeID,
				payload.OriginalBeforeEpisodeID,
			)
			if err != nil {
				return ErrCompletionUndoConflict
			}
		}

		if err := tx.Model(&current).Updates(map[string]any{
			"state":            payload.Original.State,
			"decided_at":       payload.Original.DecidedAt,
			"queue_state":      payload.Original.QueueState,
			"queue_position":   payload.Original.QueuePosition,
			"dismissed_at":     payload.Original.DismissedAt,
			"queue_updated_at": payload.Original.QueueUpdatedAt,
			"in_progress_at":   payload.Original.InProgressAt,
		}).Error; err != nil {
			return err
		}
		if originalQueue != models.QueueStateDone {
			if err := resequenceQueue(tx, models.QueueStateDone, doneIDs); err != nil {
				return err
			}
			if originalQueue != "" {
				if err := resequenceQueue(tx, originalQueue, originalQueueIDs); err != nil {
					return err
				}
			}
		}

		if payload.PreviousCompletedAt == nil {
			if err := tx.Where("episode_id = ?", episodeID).
				Delete(&models.EpisodeCompletion{}).Error; err != nil {
				return err
			}
		} else if err := upsertCompletion(tx, episodeID, *payload.PreviousCompletedAt); err != nil {
			return err
		}

		affected := affectedQueueStates(models.QueueStateDone, originalQueue)
		if originalQueue == "" {
			affected = []string{models.QueueStateDone}
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

func (s *ConsumptionService) signCompletionUndo(
	payload completionUndoPayload,
) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode completion undo: %w", err)
	}
	mac := hmac.New(sha256.New, s.undoKey)
	_, _ = mac.Write(body)
	return base64.RawURLEncoding.EncodeToString(body) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *ConsumptionService) verifyCompletionUndo(
	token string,
) (completionUndoPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return completionUndoPayload{}, ErrInvalidCompletionUndo
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return completionUndoPayload{}, ErrInvalidCompletionUndo
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return completionUndoPayload{}, ErrInvalidCompletionUndo
	}
	mac := hmac.New(sha256.New, s.undoKey)
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return completionUndoPayload{}, ErrInvalidCompletionUndo
	}
	var payload completionUndoPayload
	if err := json.Unmarshal(body, &payload); err != nil || payload.Version != 1 {
		return completionUndoPayload{}, ErrInvalidCompletionUndo
	}
	return payload, nil
}

func completionTimeForEpisode(db *gorm.DB, episodeID uint) (*time.Time, error) {
	var completion models.EpisodeCompletion
	err := db.Where("episode_id = ?", episodeID).First(&completion).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	completedAt := completion.CompletedAt
	return &completedAt, nil
}

func completionTimesForEpisodeIDs(
	db *gorm.DB,
	episodeIDs []uint,
) (map[uint]time.Time, error) {
	result := make(map[uint]time.Time, len(episodeIDs))
	if len(episodeIDs) == 0 {
		return result, nil
	}
	var completions []models.EpisodeCompletion
	if err := db.Where("episode_id IN ?", episodeIDs).Find(&completions).Error; err != nil {
		return nil, err
	}
	for _, completion := range completions {
		result[completion.EpisodeID] = completion.CompletedAt
	}
	return result, nil
}

func upsertCompletion(db *gorm.DB, episodeID uint, completedAt time.Time) error {
	completion := models.EpisodeCompletion{
		EpisodeID:   episodeID,
		CompletedAt: completedAt,
		CreatedAt:   completedAt,
		UpdatedAt:   completedAt,
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "episode_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"completed_at": completedAt,
			"updated_at":   completedAt,
		}),
	}).Create(&completion).Error
}

func episodeAfter(episodeIDs []uint, episodeID uint) *uint {
	for index, candidate := range episodeIDs {
		if candidate != episodeID || index+1 >= len(episodeIDs) {
			continue
		}
		next := episodeIDs[index+1]
		return &next
	}
	return nil
}

func snapshotCompletionUndoDecision(
	decision *models.EpisodeTriageDecision,
) completionUndoDecision {
	return completionUndoDecision{
		State:          decision.State,
		DecidedAt:      decision.DecidedAt,
		QueueState:     cloneStringPointer(decision.QueueState),
		QueuePosition:  cloneInt64Pointer(decision.QueuePosition),
		DismissedAt:    cloneTimePointer(decision.DismissedAt),
		QueueUpdatedAt: cloneTimePointer(decision.QueueUpdatedAt),
		InProgressAt:   cloneTimePointer(decision.InProgressAt),
	}
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
