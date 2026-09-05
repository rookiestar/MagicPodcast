package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	"magicpodcast/internal/services"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func originalLinkFixtureItem(guid, link, title string) string {
	linkTag := ""
	if link != "" {
		linkTag = "<link>" + link + "</link>"
	}
	return "<item><title>" + title + "</title><guid>" + guid + "</guid>" +
		linkTag +
		"<pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>" +
		"<description>original link fixture</description></item>"
}

func originalLinkFixtureFeed(items ...string) string {
	body := ""
	for _, item := range items {
		body += item
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Original Link Fixture</title>
    <link>https://example.com</link>
    <description>deterministic original link fixture</description>` +
		body + `
  </channel>
</rss>`
}

func TestSyncPodcastEpisodeItemsRetainsExistingLinkWhenFeedLinkMissing(t *testing.T) {
	for _, mode := range []EpisodeSyncMode{SyncModeIncremental, SyncModeFull} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupTestDB(t)
			service, err := NewService(db, "")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, service.Close()) })

			podcast := &models.Podcast{
				XYZID:        "retain-link-" + string(mode),
				Title:        "Retain Link Fixture",
				FeedURL:      "https://example.com/feed.xml",
				DataSource:   "rss",
				IsSubscribed: true,
			}
			require.NoError(t, db.Create(podcast).Error)

			existing := &models.Episode{
				PodcastID:     podcast.ID,
				Title:         "旧标题",
				GUID:          "retain-link-guid",
				Link:          "https://example.com/episodes/1",
				PublishedDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			}
			require.NoError(t, db.Create(existing).Error)

			result, err := service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{{
				Title:           "新标题",
				GUID:            "retain-link-guid",
				PublishedParsed: ptrTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			}}, EpisodeSyncConfig{
				Mode:                  mode,
				MaxEpisodesPerPodcast: 1000,
				UpdateExisting:        true,
			})
			require.NoError(t, err)
			assert.Equal(t, 1, result.Updated)
			assert.Equal(t, 0, result.Errors)

			var reloaded models.Episode
			require.NoError(t, db.First(&reloaded, existing.ID).Error)
			assert.Equal(t, "新标题", reloaded.Title, "other content updates still apply")
			assert.Equal(t, "https://example.com/episodes/1", reloaded.Link,
				"an empty feed link must not clear the stored original link")
		})
	}
}

func TestSyncPodcastEpisodeItemsAdoptsNewStandardLink(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := &models.Podcast{
		XYZID:        "adopt-link",
		Title:        "Adopt Link Fixture",
		FeedURL:      "https://example.com/feed.xml",
		DataSource:   "rss",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)

	existing := &models.Episode{
		PodcastID:     podcast.ID,
		Title:         "Standard Link Episode",
		GUID:          "adopt-link-guid",
		Link:          "https://example.com/episodes/old",
		PublishedDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(existing).Error)

	result, err := service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{{
		Title:           "Standard Link Episode",
		GUID:            "adopt-link-guid",
		Link:            "https://example.com/episodes/new",
		PublishedParsed: ptrTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
	}}, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 1000,
		UpdateExisting:        true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Updated)

	var reloaded models.Episode
	require.NoError(t, db.First(&reloaded, existing.ID).Error)
	assert.Equal(t, "https://example.com/episodes/new", reloaded.Link,
		"a new usable standard link must still replace the stored value")
}

func TestSyncPodcastEpisodeItemsDoesNotInventLinksFromGUIDBeforeSourceRules(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := &models.Podcast{
		XYZID:        "no-guid-fallback",
		Title:        "No GUID Fallback Fixture",
		FeedURL:      "https://rss.art19.com/example-show",
		DataSource:   "rss",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)

	result, err := service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{{
		Title:           "URL Shaped GUID Episode",
		GUID:            "https://hosting.wavpub.cn/pie/ep229/",
		PublishedParsed: ptrTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
	}}, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 1000,
		UpdateExisting:        true,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)

	var created models.Episode
	require.NoError(t, db.Where("guid = ?", "https://hosting.wavpub.cn/pie/ep229/").First(&created).Error)
	assert.Empty(t, created.Link,
		"a URL-shaped GUID must not become the original link without a verified source rule")
}

// TestSyncPodcastEpisodesOriginalURLThroughDiscoveryAPI locks the highest
// backend seam: a deterministic RSS fixture really syncs into a temp SQLite
// and the discovery API reads original_url back through retention and update.
func TestSyncPodcastEpisodesOriginalURLThroughDiscoveryAPI(t *testing.T) {
	var fixture atomic.Value
	fixture.Store(originalLinkFixtureFeed(
		originalLinkFixtureItem("fixture-guid", "https://example.com/episodes/1", "第一期"),
	))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(fixture.Load().(string)))
	}))
	t.Cleanup(server.Close)

	coordinator := feed.NewCoordinator(feed.CoordinatorConfig{
		DomainPolicies: map[string]feed.DomainPolicy{
			feed.TargetDomain(server.URL): {MaxConcurrency: 1},
		},
	})
	db := setupTestDB(t)
	service, err := NewServiceWithFeedCoordinator(db, "", coordinator)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := &models.Podcast{
		XYZID:        "original-url-seam",
		Title:        "Original URL Seam",
		FeedURL:      server.URL + "/feed.xml",
		DataSource:   "rss",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)

	config := EpisodeSyncConfig{Mode: SyncModeFull, UpdateExisting: true, MaxEpisodesPerPodcast: 1000}
	discovery := services.NewDiscoveryService(db)
	readBack := func() models.Episode {
		var stored models.Episode
		require.NoError(t, db.Where("guid = ?", "fixture-guid").First(&stored).Error)
		candidate, err := discovery.GetCandidate(stored.ID)
		require.NoError(t, err)
		require.Equal(t, stored.Link, candidate.OriginalURL,
			"the discovery API must read back exactly the stored original link")
		return stored
	}

	// 1. Usable standard link is stored.
	_, err = service.SyncPodcastEpisodesWithContext(context.Background(), podcast.ID, &progressReporter{}, config)
	require.NoError(t, err)
	stored := readBack()
	require.Equal(t, "https://example.com/episodes/1", stored.Link)

	// 2. The feed drops the standard link: the stored value survives a full refresh.
	fixture.Store(originalLinkFixtureFeed(
		originalLinkFixtureItem("fixture-guid", "", "第一期"),
	))
	_, err = service.SyncPodcastEpisodesWithContext(context.Background(), podcast.ID, &progressReporter{}, config)
	require.NoError(t, err)
	stored = readBack()
	require.Equal(t, "https://example.com/episodes/1", stored.Link,
		"an empty feed link must not clear the original link")

	// 3. A new usable standard link updates the stored value again.
	fixture.Store(originalLinkFixtureFeed(
		originalLinkFixtureItem("fixture-guid", "https://example.com/episodes/1-v2", "第一期"),
	))
	_, err = service.SyncPodcastEpisodesWithContext(context.Background(), podcast.ID, &progressReporter{}, config)
	require.NoError(t, err)
	stored = readBack()
	require.Equal(t, "https://example.com/episodes/1-v2", stored.Link)
}

// parseRSSFixture parses a deterministic RSS fixture with the production feed
// parser so WavPub-identity tests can run the real sync pipeline without
// depending on the network.
func parseRSSFixture(t *testing.T, xml string) []*gofeed.Item {
	t.Helper()
	parsed, err := gofeed.NewParser().ParseString(xml)
	require.NoError(t, err)
	return parsed.Items
}

// TestSyncPodcastEpisodesWavPubPageGuidFallback locks the #271 seam: the
// verified WavPub feed identity with a page-shaped GUID restores the original
// page for the 永远的钟鼓楼 episode 229 case, the standard link still wins,
// and the discovery API reads the restored link back.
func TestSyncPodcastEpisodesWavPubPageGuidFallback(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := &models.Podcast{
		XYZID:        "wavpub-guid-fallback",
		Title:        "后互联网时代的乱弹",
		FeedURL:      "https://proxy.wavpub.com/pie.xml",
		DataSource:   "rss",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)

	items := parseRSSFixture(t, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>后互联网时代的乱弹</title>
    <link>https://hosting.wavpub.cn/pie</link>
    <item>
      <title>第230期 标准链接优先</title>
      <link>https://hosting.wavpub.cn/pie/ep230/</link>
      <guid isPermaLink="false">https://hosting.wavpub.cn/pie/ep230/</guid>
      <pubDate>Fri, 28 Aug 2026 07:24:43 +0000</pubDate>
    </item>
    <item>
      <title>第229期 永远的钟鼓楼</title>
      <guid isPermaLink="false">https://hosting.wavpub.cn/pie/?p=822</guid>
      <pubDate>Fri, 21 Aug 2026 16:38:26 +0000</pubDate>
    </item>
  </channel>
</rss>`)

	result, err := service.syncPodcastEpisodeItems(podcast, items, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 1000,
		UpdateExisting:        true,
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.Created)
	require.Equal(t, 0, result.Errors)

	discovery := services.NewDiscoveryService(db)

	var ep229 models.Episode
	require.NoError(t, db.Where("guid = ?", "https://hosting.wavpub.cn/pie/?p=822").First(&ep229).Error)
	require.Equal(t, "229", ep229.EpisodeNo,
		"the chinese 第N期 marker must feed the stored episode label")
	candidate, err := discovery.GetCandidate(ep229.ID)
	require.NoError(t, err)
	require.Equal(t, "https://hosting.wavpub.cn/pie/?p=822", candidate.OriginalURL,
		"the WavPub page GUID must be restored as the original link")
	require.Equal(t, "229", candidate.EpisodeNo,
		"display and identity must consume the same episode label resolution")

	var ep230 models.Episode
	require.NoError(t, db.Where("guid = ?", "https://hosting.wavpub.cn/pie/ep230/").First(&ep230).Error)
	require.Equal(t, "https://hosting.wavpub.cn/pie/ep230/", ep230.Link)

	// The standard link keeps priority even when a page GUID also exists.
	updated := parseRSSFixture(t, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>后互联网时代的乱弹</title>
    <link>https://hosting.wavpub.cn/pie</link>
    <item>
      <title>第230期 标准链接优先</title>
      <link>https://hosting.wavpub.cn/pie/ep230-new/</link>
      <guid isPermaLink="false">https://hosting.wavpub.cn/pie/ep230/</guid>
      <pubDate>Fri, 28 Aug 2026 07:24:43 +0000</pubDate>
    </item>
  </channel>
</rss>`)
	_, err = service.syncPodcastEpisodeItems(podcast, updated, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 1000,
		UpdateExisting:        true,
	})
	require.NoError(t, err)

	require.NoError(t, db.First(&ep230, ep230.ID).Error)
	require.Equal(t, "https://hosting.wavpub.cn/pie/ep230-new/", ep230.Link,
		"a usable standard link must always beat the GUID fallback")
}

// TestSyncPodcastEpisodesWavPubFallbackRejectsNonPageGUIDs proves the rule
// does not leak: only an HTTPS page GUID on the exact verified host qualifies.
func TestSyncPodcastEpisodesWavPubFallbackRejectsNonPageGUIDs(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := &models.Podcast{
		XYZID:        "wavpub-strict",
		Title:        "WavPub Strict Fixture",
		FeedURL:      "https://hosting.wavpub.cn/strict/feed/",
		DataSource:   "rss",
		IsSubscribed: true,
	}
	require.NoError(t, db.Create(podcast).Error)

	items := parseRSSFixture(t, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Strict</title>
    <link>https://hosting.wavpub.cn/strict</link>
    <item>
      <title>http GUID</title>
      <guid>http://hosting.wavpub.cn/strict/ep1/</guid>
      <pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
    </item>
    <item>
      <title>wrong host GUID</title>
      <guid>https://evil.example.com/strict/ep2/</guid>
      <pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
    </item>
    <item>
      <title>root permalink GUID</title>
      <guid>https://hosting.wavpub.cn/?p=822</guid>
      <pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
    </item>
    <item>
      <title>enclosure GUID</title>
      <guid>https://cdn2.wavpub.com/strict/ep4.mp3</guid>
      <pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
    </item>
    <item>
      <title>plain text GUID</title>
      <guid>ep-plain-text</guid>
      <pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
    </item>
    <item>
      <title>第229期 期号不得生成链接</title>
      <guid>ep229-text-only</guid>
      <pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`)

	_, err = service.syncPodcastEpisodeItems(podcast, items, EpisodeSyncConfig{
		Mode:                  SyncModeFull,
		MaxEpisodesPerPodcast: 1000,
		UpdateExisting:        true,
	})
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&models.Episode{}).
		Where("podcast_id = ? AND link <> ''", podcast.ID).
		Count(&count).Error)
	require.Equal(t, int64(0), count,
		"non-page GUIDs must never become original links")
}

// TestSyncPodcastEpisodeItemsKeepsMissingLinksForUnverifiedSources proves the
// ART19/MeldingCloud/荔枝/Libsyn-style sources stay 暂缺 without inventing URLs
// and that existing links survive their empty feed values.
func TestSyncPodcastEpisodeItemsKeepsMissingLinksForUnverifiedSources(t *testing.T) {
	feedURLs := []string{
		"https://rss.art19.com/missing-links",
		"https://media.meldingcloud.com/missing-links/rss",
		"https://www.lizhi.fm/missing-links/rss",
		"https://missing-links.libsyn.com/rss",
	}

	for _, feedURL := range feedURLs {
		t.Run(feedURL, func(t *testing.T) {
			db := setupTestDB(t)
			service, err := NewService(db, "")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, service.Close()) })

			podcast := &models.Podcast{
				XYZID:        "unverified-missing-links",
				Title:        "Unverified Missing Links",
				FeedURL:      feedURL,
				DataSource:   "rss",
				IsSubscribed: true,
			}
			require.NoError(t, db.Create(podcast).Error)

			existing := &models.Episode{
				PodcastID:     podcast.ID,
				Title:         "已有链接单集",
				GUID:          "verified-existing-guid",
				Link:          "https://example.com/episodes/keep",
				PublishedDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			}
			require.NoError(t, db.Create(existing).Error)

			_, err = service.syncPodcastEpisodeItems(podcast, []*gofeed.Item{{
				Title:           "已有链接单集",
				GUID:            "verified-existing-guid",
				PublishedParsed: ptrTime(existing.PublishedDate),
			}, {
				Title:           "一直缺链单集",
				GUID:            "always-missing-guid",
				PublishedParsed: ptrTime(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)),
			}}, EpisodeSyncConfig{
				Mode:                  SyncModeFull,
				MaxEpisodesPerPodcast: 1000,
				UpdateExisting:        true,
			})
			require.NoError(t, err)

			var kept models.Episode
			require.NoError(t, db.First(&kept, existing.ID).Error)
			require.Equal(t, "https://example.com/episodes/keep", kept.Link)

			var missing models.Episode
			require.NoError(t, db.Where("guid = ?", "always-missing-guid").First(&missing).Error)
			require.Empty(t, missing.Link,
				"an unverified source without a standard link must stay 暂缺")
		})
	}
}
