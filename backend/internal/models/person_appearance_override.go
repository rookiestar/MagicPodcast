package models

import "time"

// PersonAppearanceOverride contains only explicit episode-scoped user choices.
// A nil role leaves the automatic role untouched; exclusion never deletes facts.
type PersonAppearanceOverride struct {
	EpisodeID uint `gorm:"primaryKey"`
	PersonID  uint `gorm:"primaryKey"`
	Role      *string
	Excluded  bool      `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

func (PersonAppearanceOverride) TableName() string { return "person_appearance_overrides" }

const PersonAppearanceOverridesCreateSQL = `CREATE TABLE IF NOT EXISTS person_appearance_overrides (
 episode_id INTEGER NOT NULL,
 person_id INTEGER NOT NULL,
 role TEXT CHECK (role IS NULL OR role IN ('host','guest','unknown')),
 excluded INTEGER NOT NULL DEFAULT 0 CHECK (excluded IN (0,1)),
 updated_at DATETIME NOT NULL,
 PRIMARY KEY (episode_id, person_id),
 FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE,
 FOREIGN KEY (person_id) REFERENCES people(id) ON DELETE CASCADE
)`
