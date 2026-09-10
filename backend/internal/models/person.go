package models

import "time"

const (
	PersonAppearanceRoleHost    = "host"
	PersonAppearanceRoleGuest   = "guest"
	PersonAppearanceRoleUnknown = "unknown"

	PersonStatusConfirmed = "confirmed"
	PersonStatusPending   = "pending"

	SpeechAttributionStatusConfirmed = "confirmed"
	SpeechAttributionStatusPending   = "pending"
	SpeechAttributionStatusRejected  = "rejected"

	PersonConfirmationKindName        = "person_name"
	PersonConfirmationKindAttribution = "fragment_attribution"

	SpeechSourceTranscript = "transcript"
	SpeechSourceShowNotes  = "show_notes"
)

type Person struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	StableKey    string    `gorm:"size:64;not null" json:"stable_key"`
	DisplayName  string    `gorm:"size:200;not null" json:"display_name"`
	IdentityNote string    `gorm:"type:text;not null;default:''" json:"identity_note"`
	CreatedAt    time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt    time.Time `gorm:"not null" json:"updated_at"`
}

func (Person) TableName() string { return "people" }

type PersonAlias struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PersonID  uint      `gorm:"not null" json:"person_id"`
	Person    Person    `gorm:"foreignKey:PersonID;constraint:OnDelete:CASCADE" json:"-"`
	Alias     string    `gorm:"size:200;not null" json:"alias"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
}

func (PersonAlias) TableName() string { return "person_aliases" }

type EpisodeAppearance struct {
	SourceVersion   string    `gorm:"size:128;not null;default:''" json:"source_version"`
	ID              uint      `gorm:"primaryKey" json:"id"`
	PersonID        uint      `gorm:"not null" json:"person_id"`
	Person          Person    `gorm:"foreignKey:PersonID;constraint:OnDelete:CASCADE" json:"-"`
	EpisodeID       uint      `gorm:"not null" json:"episode_id"`
	Episode         Episode   `gorm:"foreignKey:EpisodeID;constraint:OnDelete:CASCADE" json:"-"`
	Role            string    `gorm:"size:16;not null" json:"role"`
	Status          string    `gorm:"size:16;not null" json:"status"`
	StatusReason    string    `gorm:"size:300;not null;default:''" json:"status_reason"`
	EvidenceKind    string    `gorm:"size:64;not null;default:''" json:"evidence_kind"`
	EvidenceLocator string    `gorm:"size:300;not null;default:''" json:"evidence_locator"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
}

func (EpisodeAppearance) TableName() string { return "episode_appearances" }

type SpeechAttribution struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	AppearanceID    *uint     `json:"appearance_id,omitempty"`
	EpisodeID       uint      `gorm:"not null" json:"episode_id"`
	Episode         Episode   `gorm:"foreignKey:EpisodeID;constraint:OnDelete:CASCADE" json:"-"`
	PersonID        *uint     `json:"person_id,omitempty"`
	SourceKind      string    `gorm:"size:32;not null" json:"source_kind"`
	SourceVersion   string    `gorm:"size:128;not null" json:"source_version"`
	FragmentOrder   int       `gorm:"not null" json:"fragment_order"`
	SpeakerLabel    string    `gorm:"size:200;not null;default:''" json:"speaker_label"`
	StartMS         int64     `gorm:"not null;default:0" json:"start_ms"`
	Text            string    `gorm:"type:text;not null" json:"text"`
	Status          string    `gorm:"size:16;not null" json:"status"`
	EvidenceKind    string    `gorm:"size:64;not null;default:''" json:"evidence_kind"`
	EvidenceLocator string    `gorm:"size:300;not null;default:''" json:"evidence_locator"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time `gorm:"not null" json:"updated_at"`
}

func (SpeechAttribution) TableName() string { return "speech_attributions" }

type PersonUserConfirmation struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	EpisodeID        uint      `gorm:"not null" json:"episode_id"`
	Kind             string    `gorm:"size:32;not null" json:"kind"`
	PersonID         *uint     `json:"person_id,omitempty"`
	SourceVersion    string    `gorm:"size:128;not null;default:''" json:"source_version"`
	SourceText       string    `gorm:"type:text;not null;default:''" json:"source_text"`
	Status           string    `gorm:"size:16;not null;default:'pending'" json:"status"`
	FragmentOrder    int       `gorm:"not null;default:0" json:"fragment_order"`
	SourceKind       string    `gorm:"size:32;not null;default:''" json:"source_kind"`
	SpeakerLabel     string    `gorm:"size:200;not null;default:''" json:"speaker_label"`
	AssignedPersonID *uint     `json:"assigned_person_id,omitempty"`
	DisplayName      string    `gorm:"size:200;not null;default:''" json:"display_name"`
	IdentityNote     string    `gorm:"type:text;not null;default:''" json:"identity_note"`
	Role             string    `gorm:"size:16;not null;default:''" json:"role"`
	CreatedAt        time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt        time.Time `gorm:"not null" json:"updated_at"`
}

func (PersonUserConfirmation) TableName() string { return "person_user_confirmations" }

const PeopleCreateTableSQL = `
CREATE TABLE IF NOT EXISTS people (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	stable_key TEXT NOT NULL,
	display_name TEXT NOT NULL,
	identity_note TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
)`

const PeopleStableKeyIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_people_stable_key ON people(stable_key)`

const PersonAliasesCreateTableSQL = `
CREATE TABLE IF NOT EXISTS person_aliases (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	person_id INTEGER NOT NULL,
	alias TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	CONSTRAINT fk_person_aliases_person
		FOREIGN KEY (person_id) REFERENCES people(id) ON DELETE CASCADE
)`

const PersonAliasesUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_person_aliases_person_alias
ON person_aliases(person_id, alias)`

const PersonAliasesLookupIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_person_aliases_alias ON person_aliases(alias)`

const EpisodeAppearancesCreateTableSQL = `
CREATE TABLE IF NOT EXISTS episode_appearances (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	person_id INTEGER NOT NULL,
	episode_id INTEGER NOT NULL,
	source_version TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL,
	status TEXT NOT NULL,
	status_reason TEXT NOT NULL DEFAULT '',
	evidence_kind TEXT NOT NULL DEFAULT '',
	evidence_locator TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	CONSTRAINT fk_episode_appearances_person
		FOREIGN KEY (person_id) REFERENCES people(id) ON DELETE CASCADE,
	CONSTRAINT fk_episode_appearances_episode
		FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE,
	CONSTRAINT chk_episode_appearances_role
		CHECK (role IN ('host','guest','unknown')),
	CONSTRAINT chk_episode_appearances_status
		CHECK (status IN ('confirmed','pending'))
)`

const EpisodeAppearancesUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_episode_appearances_episode_person
ON episode_appearances(episode_id, person_id)`

const EpisodeAppearancesEpisodeIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_episode_appearances_episode
ON episode_appearances(episode_id, status)`

const SpeechAttributionsCreateTableSQL = `
CREATE TABLE IF NOT EXISTS speech_attributions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	appearance_id INTEGER,
	episode_id INTEGER NOT NULL,
	person_id INTEGER,
	source_kind TEXT NOT NULL,
	source_version TEXT NOT NULL,
	fragment_order INTEGER NOT NULL,
	speaker_label TEXT NOT NULL DEFAULT '',
	start_ms INTEGER NOT NULL DEFAULT 0,
	text TEXT NOT NULL,
	status TEXT NOT NULL,
	evidence_kind TEXT NOT NULL DEFAULT '',
	evidence_locator TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	CONSTRAINT fk_speech_attributions_episode
		FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE,
	CONSTRAINT fk_speech_attributions_person
		FOREIGN KEY (person_id) REFERENCES people(id) ON DELETE SET NULL,
	CONSTRAINT chk_speech_attributions_status
		CHECK (status IN ('confirmed','pending','rejected')),
	CONSTRAINT chk_speech_attributions_source
		CHECK (source_kind IN ('transcript','show_notes'))
)`

const SpeechAttributionsUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_speech_attributions_source_fragment
ON speech_attributions(episode_id, source_kind, source_version, fragment_order)`

const SpeechAttributionsPersonIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_speech_attributions_person_status
ON speech_attributions(person_id, status, episode_id)`

const PersonUserConfirmationsCreateTableSQL = `
CREATE TABLE IF NOT EXISTS person_user_confirmations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	episode_id INTEGER NOT NULL,
	kind TEXT NOT NULL,
	person_id INTEGER,
	source_version TEXT NOT NULL DEFAULT '',
	source_text TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'pending',
	fragment_order INTEGER NOT NULL DEFAULT 0,
	source_kind TEXT NOT NULL DEFAULT '',
	speaker_label TEXT NOT NULL DEFAULT '',
	assigned_person_id INTEGER,
	display_name TEXT NOT NULL DEFAULT '',
	identity_note TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	CONSTRAINT fk_person_user_confirmations_episode
		FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE,
	CONSTRAINT chk_person_user_confirmations_kind
		CHECK (kind IN ('person_name','fragment_attribution'))
)`

const PersonUserConfirmationsUniqueIndexSQL = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_person_user_confirmations_subject
ON person_user_confirmations(episode_id, kind, source_kind, fragment_order, person_id)`
