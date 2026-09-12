package models

import "time"

// PersonDraft keeps a reviewable suggestion separate from effective identity facts.
// Previous source versions remain available without becoming current again.
type PersonDraft struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	EpisodeID           uint      `json:"episode_id"`
	Revision            uint      `json:"revision"`
	SourceVersion       string    `json:"source_version"`
	MetadataDigest      string    `json:"-"`
	Sources             string    `json:"-"`
	Matches             string    `json:"-"`
	AppliedRequest      string    `json:"-"`
	LastAppliedRevision uint      `json:"-"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (PersonDraft) TableName() string { return "person_drafts" }

const PersonDraftCreateSQL = `CREATE TABLE person_drafts (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 episode_id INTEGER NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
 revision INTEGER NOT NULL,
 source_version TEXT NOT NULL,
 metadata_digest TEXT NOT NULL,
 sources TEXT NOT NULL,
 matches TEXT NOT NULL,
 last_applied_revision INTEGER NOT NULL DEFAULT 0,
 applied_request TEXT NOT NULL DEFAULT '',
 updated_at DATETIME NOT NULL
)`
const PersonDraftIndexSQL = `CREATE INDEX idx_person_drafts_episode ON person_drafts(episode_id, id)`
