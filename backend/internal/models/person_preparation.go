package models

import "time"

// CurrentIdentityAlgorithm identifies results which passed the current
// evidence contract. It is independent of the original transcript version.
const CurrentIdentityAlgorithm = "identity-evidence-v3"

type PersonPreparation struct {
	EpisodeID         uint      `gorm:"primaryKey" json:"episode_id"`
	Revision          uint      `gorm:"not null" json:"revision"`
	PublishedRevision uint      `gorm:"not null" json:"published_revision"`
	SourceVersion     string    `gorm:"not null" json:"source_version"`
	AlgorithmVersion  string    `gorm:"not null" json:"algorithm_version"`
	MetadataDigest    string    `gorm:"not null" json:"metadata_digest"`
	UpdatedAt         time.Time `gorm:"not null" json:"updated_at"`
}

func (PersonPreparation) TableName() string { return "person_preparations" }

const PersonPreparationsCreateSQL = `
CREATE TABLE IF NOT EXISTS person_preparations (
 episode_id INTEGER PRIMARY KEY,
 revision INTEGER NOT NULL DEFAULT 0,
 published_revision INTEGER NOT NULL DEFAULT 0,
 source_version TEXT NOT NULL DEFAULT '',
 algorithm_version TEXT NOT NULL DEFAULT '',
 metadata_digest TEXT NOT NULL DEFAULT '',
 updated_at DATETIME NOT NULL,
 FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE
)`

// Keep original transcript text searchable while invalidating only identity
// conclusions derived from changed public metadata.
const PersonPreparationEpisodeInvalidationSQL = `
CREATE TRIGGER invalidate_person_preparation_on_episode_metadata
AFTER UPDATE OF title, show_notes, podcast_id, published_date ON episodes
WHEN OLD.title IS NOT NEW.title OR OLD.show_notes IS NOT NEW.show_notes
 OR OLD.podcast_id IS NOT NEW.podcast_id OR OLD.published_date IS NOT NEW.published_date
BEGIN
 UPDATE person_preparations SET algorithm_version = '', revision = revision + 1 WHERE episode_id = NEW.id;
 DELETE FROM content_search_fragments WHERE episode_id = NEW.id AND source_kind = 'show_notes';
 UPDATE content_search_coverage SET complete = 0 WHERE episode_id = NEW.id AND source_kind = 'show_notes';
END`

const PersonPreparationPodcastInvalidationSQL = `
CREATE TRIGGER invalidate_person_preparation_on_podcast_metadata
AFTER UPDATE OF title, author, description ON podcasts
WHEN OLD.title IS NOT NEW.title OR OLD.author IS NOT NEW.author OR OLD.description IS NOT NEW.description
BEGIN
 UPDATE person_preparations SET algorithm_version = '', revision = revision + 1
 WHERE episode_id IN (SELECT id FROM episodes WHERE podcast_id = NEW.id);
END`
