package sync

import (
	"fmt"

	episodelabel "magicpodcast/internal/episode"
	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
)

// episodeIdentityKey is intentionally stricter than title matching. A valid
// cross-feed identity requires both a recognizable episode number and the
// exact publication instant. It is independent of the source-specific GUID.
func episodeIdentityKey(episode *models.Episode) (string, bool) {
	if episode == nil || episode.PodcastID == 0 || episode.PublishedDate.IsZero() {
		return "", false
	}
	episodeNo := episodelabel.FromTitle(episode.Title)
	if episodeNo == "" {
		episodeNo = episodelabel.Normalize("", episode.EpisodeNo)
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
	return episodelabel.FromTitle(item.Title)
}

func episodeNoFromTitle(title string) string {
	return episodelabel.FromTitle(title)
}
