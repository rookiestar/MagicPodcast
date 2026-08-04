package models

import "time"

const (
	TriageStatePending     = "pending"
	TriageStateShortlisted = "shortlisted"
	TriageStateDiscarded   = "discarded"
)

type EpisodeTriageDecision struct {
	BaseModel
	EpisodeID uint      `gorm:"not null;uniqueIndex" json:"episode_id"`
	Episode   Episode   `gorm:"foreignKey:EpisodeID;constraint:OnDelete:CASCADE" json:"-"`
	State     string    `gorm:"size:20;not null;index" json:"state"`
	DecidedAt time.Time `gorm:"not null;index" json:"decided_at"`
}

func (EpisodeTriageDecision) TableName() string {
	return "episode_triage_decisions"
}
