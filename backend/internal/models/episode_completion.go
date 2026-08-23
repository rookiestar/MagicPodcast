package models

import "time"

// EpisodeCompletion is the durable fact that an episode has been explicitly
// completed at least once. EpisodeID is both the primary key and the foreign
// key so repeated completion updates one fact instead of appending events.
type EpisodeCompletion struct {
	EpisodeID   uint      `gorm:"primaryKey;autoIncrement:false" json:"episode_id"`
	Episode     Episode   `gorm:"foreignKey:EpisodeID;constraint:OnDelete:CASCADE" json:"-"`
	CompletedAt time.Time `gorm:"not null;index" json:"completed_at"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null" json:"updated_at"`
}

func (EpisodeCompletion) TableName() string {
	return "episode_completions"
}
