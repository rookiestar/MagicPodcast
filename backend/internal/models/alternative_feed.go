package models

import "time"

// Alternative verification cache statuses. These are local policy outcomes,
// not upstream HTTP codes.
const (
	AlternativeCacheVerified   = "verified"
	AlternativeCacheUnavailable = "unavailable"
)

// PodcastAlternativeFeed stores a pre-verified (or rejected) alternative Feed
// for one podcast, keyed by the current main Feed URL and stable identity.
// It never replaces podcasts.feed_url; it only serves the current batch after
// primary failure (#35/#37).
type PodcastAlternativeFeed struct {
	BaseModel

	PodcastID          uint      `gorm:"uniqueIndex:idx_alt_feed_podcast;not null" json:"podcast_id"`
	MainFeedURL        string    `gorm:"size:1000;not null;default:''" json:"main_feed_url"`
	IdentityKey        string    `gorm:"size:255;not null;default:''" json:"identity_key"`
	AlternativeFeedURL string    `gorm:"size:1000;not null;default:''" json:"alternative_feed_url"`
	Status             string    `gorm:"size:32;not null;default:''" json:"status"`
	Verification       string    `gorm:"size:80;not null;default:''" json:"verification"`
	UnavailableReason  string    `gorm:"size:120;not null;default:''" json:"unavailable_reason"`
	VerifiedAt         time.Time `gorm:"not null" json:"verified_at"`
}

// TableName pins the durable table name used by schema migrations and tests.
func (PodcastAlternativeFeed) TableName() string {
	return "podcast_alternative_feeds"
}
