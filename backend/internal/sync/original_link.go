package sync

import (
	"magicpodcast/internal/models"
	"magicpodcast/internal/originallink"

	"github.com/mmcdole/gofeed"
)

// resolveOriginalEpisodeLink is the single seam between RSS items and the
// stored 原节目链接. Every episode write path asks the shared originallink
// entry instead of reading item.Link directly, so the standard-link
// priority, the verified WavPub GUID fallback, and the keep-existing
// retention rule cannot drift between incremental and full syncs.
func resolveOriginalEpisodeLink(podcast *models.Podcast, item *gofeed.Item, existingLink string) originallink.Decision {
	return originallink.Resolve(originallink.Input{
		Feed:         originallink.FeedIdentity{FeedURL: podcast.FeedURL},
		RSSLink:      item.Link,
		GUID:         item.GUID,
		ExistingLink: existingLink,
	})
}
