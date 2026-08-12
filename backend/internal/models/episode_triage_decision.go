package models

import "time"

const (
	TriageStatePending     = "pending"
	TriageStateShortlisted = "shortlisted"
	TriageStateDiscarded   = "discarded"

	QueueStateInbox   = "inbox"
	QueueStateFocus   = "focus"
	QueueStateSomeday = "someday"
	QueueStateDone    = "done"
)

type EpisodeTriageDecision struct {
	BaseModel
	EpisodeID      uint       `gorm:"not null;uniqueIndex" json:"episode_id"`
	Episode        Episode    `gorm:"foreignKey:EpisodeID;constraint:OnDelete:CASCADE" json:"-"`
	State          string     `gorm:"size:20;not null;index" json:"state"`
	DecidedAt      time.Time  `gorm:"not null;index" json:"decided_at"`
	QueueState     *string    `gorm:"size:20;index" json:"queue_state,omitempty"`
	DismissedAt    *time.Time `gorm:"index" json:"dismissed_at,omitempty"`
	QueueUpdatedAt *time.Time `gorm:"index" json:"queue_updated_at,omitempty"`
	InProgressAt   *time.Time `gorm:"index" json:"in_progress_at,omitempty"`
	ReadAt         *time.Time `gorm:"index" json:"read_at,omitempty"`
}

func (EpisodeTriageDecision) TableName() string {
	return "episode_triage_decisions"
}
