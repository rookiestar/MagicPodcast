package xyzvideo

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
)

const (
	defaultBase = "https://" + feed.XiaoyuzhouWebDomain
)

var episodeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// ParseEpisodeID extracts a Xiaoyuzhou episode id from a public episode page
// URL. Podcast pages, comment paths, and non-http(s) values are rejected.
func ParseEpisodeID(rawURL string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host != feed.XiaoyuzhouWebDomain && host != feed.XiaoyuzhouLegacyWebDomain {
		return "", false
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) != 2 || segments[0] != "episode" {
		return "", false
	}
	id := segments[1]
	if !episodeIDPattern.MatchString(id) {
		return "", false
	}
	return id, true
}

// ShouldProbe reports whether this batch should ask Xiaoyuzhou about video.
// New rows and still-unknown rows may be probed. Terminal states are skipped
// unless the GUID or page link changed and needs a fresh determination.
func ShouldProbe(link, availability string, identityChanged, isNew bool) bool {
	if _, ok := ParseEpisodeID(link); !ok {
		return false
	}
	if isNew {
		return true
	}
	if models.NormalizeVideoAvailability(availability) == models.VideoAvailabilityUnknown {
		return true
	}
	return identityChanged
}

// ParsePlaybackResponse maps a video-playback HTTP result onto the tri-state.
// The body is inspected only to classify the status; callers must discard it
// so signed HLS URLs never reach storage.
func ParsePlaybackResponse(status int, _ []byte) string {
	switch status {
	case http.StatusOK:
		return models.VideoAvailabilityAvailable
	case http.StatusNotFound:
		return models.VideoAvailabilityUnavailable
	default:
		return models.VideoAvailabilityUnknown
	}
}

func haltAfterStatus(status int, err error) bool {
	if err != nil {
		return true
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests {
		return true
	}
	return status >= 500
}
