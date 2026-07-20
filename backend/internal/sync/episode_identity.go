package sync

import (
	"fmt"
	"regexp"
	"strings"

	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
)

var (
	episodeMarkerPattern  = regexp.MustCompile(`(?i)(?:^|[\s(\[【|｜])(?:episode|ep|e|vol(?:ume)?[ ._-]*)\s*([0-9]{1,4})(?:$|[\s.。、:：)\]】|_-])`)
	episodePrefixPattern  = regexp.MustCompile(`^\s*([0-9]{1,4})(?:$|[\s.。、:：)\]】_-])`)
	episodeNumericPattern = regexp.MustCompile(`(?i)^(?:episode|ep|e|vol(?:ume)?)[ ._-]*([0-9]{1,4})$`)
	episodeDigitsPattern  = regexp.MustCompile(`^[0-9]+$`)
)

// episodeIdentityKey is intentionally stricter than title matching. A valid
// cross-feed identity requires both a recognizable episode number and the
// exact publication instant. It is independent of the source-specific GUID.
func episodeIdentityKey(episode *models.Episode) (string, bool) {
	if episode == nil || episode.PodcastID == 0 || episode.PublishedDate.IsZero() {
		return "", false
	}
	episodeNo := episodeNoFromTitle(episode.Title)
	if episodeNo == "" {
		episodeNo = canonicalEpisodeNo(episode.EpisodeNo)
	}
	if episodeNo == "" {
		return "", false
	}
	return fmt.Sprintf("%d:%s:%d", episode.PodcastID, episodeNo, episode.PublishedDate.UTC().UnixNano()), true
}

func episodeNoFromItem(item *gofeed.Item) string {
	if item == nil {
		return ""
	}
	// Some alternative feeds expose their own list position as
	// <itunes:episode> (for example, E150 may be position 5). A clear episode
	// marker in the title is the source-independent identity.
	if episodeNo := episodeNoFromTitle(item.Title); episodeNo != "" {
		return episodeNo
	}
	if item.ITunesExt != nil && strings.TrimSpace(item.ITunesExt.Episode) != "" {
		if episodeNo := canonicalEpisodeNo(item.ITunesExt.Episode); episodeNo != "" {
			return episodeNo
		}
	}
	return episodeNoFromTitle(item.Title)
}

func episodeNoFromTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if matches := episodeMarkerPattern.FindStringSubmatch(title); len(matches) == 2 {
		return canonicalEpisodeNo(matches[1])
	}
	if matches := episodePrefixPattern.FindStringSubmatch(title); len(matches) == 2 {
		return canonicalEpisodeNo(matches[1])
	}
	return ""
}

func canonicalEpisodeNo(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.Trim(value, " .。、:：-_()（）[]【】")
	if matches := episodeNumericPattern.FindStringSubmatch(value); len(matches) == 2 {
		value = matches[1]
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ToUpper(value)
	if episodeDigitsPattern.MatchString(value) {
		value = strings.TrimLeft(value, "0")
		if value == "" {
			value = "0"
		}
	}
	return value
}
