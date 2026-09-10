package models

import "time"

type ContentSearchFragment struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	EpisodeID         uint      `gorm:"not null" json:"episode_id"`
	SourceKind        string    `gorm:"size:32;not null" json:"source_kind"`
	SourceVersion     string    `gorm:"size:128;not null" json:"source_version"`
	FragmentOrder     int       `gorm:"not null" json:"fragment_order"`
	StartMS           int64     `gorm:"not null;default:0" json:"start_ms"`
	Text              string    `gorm:"type:text;not null" json:"text"`
	ContextBefore     string    `gorm:"type:text;not null;default:''" json:"context_before"`
	ContextAfter      string    `gorm:"type:text;not null;default:''" json:"context_after"`
	PersonID          *uint     `json:"person_id,omitempty"`
	AttributionStatus string    `gorm:"size:16;not null;default:''" json:"attribution_status"`
	Tokens            string    `gorm:"type:text;not null;default:''" json:"-"`
	PublishedAt       time.Time `gorm:"not null" json:"published_at"`
	Current           bool      `gorm:"not null;default:false" json:"current"`
	CreatedAt         time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

func (ContentSearchFragment) TableName() string { return "content_search_fragments" }

type ContentSearchCoverage struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	EpisodeID     uint      `gorm:"not null" json:"episode_id"`
	SourceKind    string    `gorm:"size:32;not null" json:"source_kind"`
	SourceVersion string    `gorm:"size:128;not null" json:"source_version"`
	Complete      bool      `gorm:"not null;default:false" json:"complete"`
	Reason        string    `gorm:"size:200;not null;default:''" json:"reason"`
	UpdatedAt     time.Time `gorm:"not null" json:"updated_at"`
}

func (ContentSearchCoverage) TableName() string { return "content_search_coverage" }

const ContentSearchFragmentsCreateTableSQL = `
CREATE TABLE IF NOT EXISTS content_search_fragments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	episode_id INTEGER NOT NULL,
	source_kind TEXT NOT NULL,
	source_version TEXT NOT NULL,
	fragment_order INTEGER NOT NULL,
	start_ms INTEGER NOT NULL DEFAULT 0,
	text TEXT NOT NULL,
	context_before TEXT NOT NULL DEFAULT '',
	context_after TEXT NOT NULL DEFAULT '',
	person_id INTEGER,
	attribution_status TEXT NOT NULL DEFAULT '',
	tokens TEXT NOT NULL DEFAULT '',
	published_at DATETIME NOT NULL,
	current NUMERIC NOT NULL DEFAULT false,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	CONSTRAINT fk_content_search_fragments_episode
		FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE,
	CONSTRAINT chk_content_search_fragments_source
		CHECK (source_kind IN ('transcript','show_notes'))
)`

const ContentSearchFragmentsUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_content_search_fragments_identity
ON content_search_fragments(episode_id, source_kind, source_version, fragment_order)`

const ContentSearchFragmentsQueryIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_content_search_fragments_query
ON content_search_fragments(current, episode_id, person_id, attribution_status)`

const ContentSearchCoverageCreateTableSQL = `
CREATE TABLE IF NOT EXISTS content_search_coverage (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	episode_id INTEGER NOT NULL,
	source_kind TEXT NOT NULL,
	source_version TEXT NOT NULL,
	complete NUMERIC NOT NULL DEFAULT false,
	reason TEXT NOT NULL DEFAULT '',
	updated_at DATETIME NOT NULL,
	CONSTRAINT fk_content_search_coverage_episode
		FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE
)`

const ContentSearchCoverageUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_content_search_coverage_source
ON content_search_coverage(episode_id, source_kind)`

// Source edits can occur outside processing. Invalidate derived data at the
// database write boundary; keep manual confirmations for the next preparation.
const ContentSearchShowNotesInvalidationSQL = `
CREATE TRIGGER IF NOT EXISTS invalidate_persona_on_show_notes
AFTER UPDATE OF show_notes ON episodes
WHEN COALESCE(OLD.show_notes, '') != COALESCE(NEW.show_notes, '')
BEGIN
 DELETE FROM content_search_fragments WHERE episode_id = NEW.id;
 DELETE FROM content_search_coverage WHERE episode_id = NEW.id;
 UPDATE episode_appearances SET status = 'pending', status_reason = '来源已更新，请重新识别本集人物' WHERE episode_id = NEW.id;
 UPDATE speech_attributions SET status = 'pending' WHERE episode_id = NEW.id;
END`
