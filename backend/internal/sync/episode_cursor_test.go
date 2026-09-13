package sync

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cursorFeedXML 生成包含指定数量单集的 RSS，单集发布于 2024 年 1 月逐日递增。
func cursorFeedXML(items int) string {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Cursor Feed</title><link>https://example.com</link><description>t</description>`
	for i := 1; i <= items; i++ {
		xml += fmt.Sprintf(`<item><title>Episode %d</title><guid>cursor-ep-%d</guid>
<pubDate>Mon, %02d Jan 2024 00:00:00 GMT</pubDate><description>body %d</description></item>`, i, i, i, i)
	}
	xml += `</channel></rss>`
	return xml
}

// cursorFeedXMLWithExtra 在历史单集后追加一个指定发布时间的额外单集。
func cursorFeedXMLWithExtra(items int, extraTitle, extraGUID string, published time.Time) string {
	return cursorFeedXML(items)[:len(cursorFeedXML(items))-len("</channel></rss>")] +
		fmt.Sprintf(`<item><title>%s</title><guid>%s</guid><pubDate>%s</pubDate><description>fresh</description></item>`,
			extraTitle, extraGUID, published.Format(http.TimeFormat)) + `</channel></rss>`
}

func newCursorFeedServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func importCursorPodcast(t *testing.T, service *Service, serverURL string) *models.Podcast {
	t.Helper()
	path := writeTestOPML(t, serverURL+"/feed.xml")
	result, err := service.ImportOPMLFromPodcastIndexOnly(path, &recordingReporter{})
	require.NoError(t, err)
	require.Equal(t, 1, result.SuccessPodcasts)
	var podcast models.Podcast
	require.NoError(t, service.db.First(&podcast).Error)
	return &podcast
}

func parsedCursorItems(t *testing.T, xml string) []*gofeed.Item {
	t.Helper()
	feed, err := gofeed.NewParser().ParseString(xml)
	require.NoError(t, err)
	return feed.Items
}

func TestSmartSyncAfterImportWritesHistoricalEpisodes(t *testing.T) {
	server := newCursorFeedServer(t, cursorFeedXML(3))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	// 导入推进了资料检查时间，但单集游标必须仍为空。
	require.NotNil(t, podcast.LastFetchedAt)
	require.Nil(t, podcast.LastEpisodeSyncAt)

	result, err := service.SyncPodcastEpisodes(podcast.ID, &recordingReporter{}, DefaultEpisodeSyncConfig)
	require.NoError(t, err)
	assert.Equal(t, 3, result.Created)
	assert.False(t, result.Incomplete)

	var count int64
	require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&count).Error)
	assert.Equal(t, int64(3), count)

	var refreshed models.Podcast
	require.NoError(t, db.First(&refreshed, podcast.ID).Error)
	require.NotNil(t, refreshed.LastEpisodeSyncAt)
}

func TestReImportDoesNotAdvanceEpisodeCursor(t *testing.T) {
	server := newCursorFeedServer(t, cursorFeedXML(2))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	_, err = service.SyncPodcastEpisodes(podcast.ID, &recordingReporter{}, DefaultEpisodeSyncConfig)
	require.NoError(t, err)
	var synced models.Podcast
	require.NoError(t, db.First(&synced, podcast.ID).Error)
	cursor := *synced.LastEpisodeSyncAt

	// 重导同一节目：资料写入路径不得推进单集游标。
	result, err := service.ImportOPMLFromPodcastIndexOnly(writeTestOPML(t, server.URL+"/feed.xml"), &recordingReporter{})
	require.NoError(t, err)
	require.Equal(t, 1, result.UnchangedPodcasts)
	require.NoError(t, db.First(&synced, podcast.ID).Error)
	require.NotNil(t, synced.LastEpisodeSyncAt)
	assert.True(t, cursor.Equal(*synced.LastEpisodeSyncAt), "重导不得推进单集同步游标")

	// 游标未动，智能同步以原游标为增量基准，不重复创建。
	result2, err := service.SyncPodcastEpisodes(podcast.ID, &recordingReporter{}, DefaultEpisodeSyncConfig)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.Created)
	var count int64
	require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestEpisodeWriteFailureKeepsCursorAndRetriesWithoutDuplicates(t *testing.T) {
	server := newCursorFeedServer(t, cursorFeedXML(2))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	items := parsedCursorItems(t, cursorFeedXML(2))

	// 让 feed 中 cursor-ep-1 的 GUID 归属另一节目，触发单集写入错误。
	other := models.Podcast{XYZID: "other", Title: "Other", FeedURL: "https://example.com/other.xml"}
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&models.Episode{PodcastID: other.ID, Title: "占用", GUID: "cursor-ep-1"}).Error)

	result, err := service.syncPodcastEpisodeItemsWithContext(t.Context(), podcast, items, DefaultEpisodeSyncConfig, true)
	require.Error(t, err)
	assert.True(t, result.Incomplete)
	assert.Equal(t, 1, result.Created, "同批未受影响的单集仍应写入")

	var refreshed models.Podcast
	require.NoError(t, db.First(&refreshed, podcast.ID).Error)
	assert.Nil(t, refreshed.LastEpisodeSyncAt, "部分失败不得推进单集游标")

	// 重试：不重复创建已写入的单集，游标仍未推进（冲突条目仍失败）。
	result2, err := service.syncPodcastEpisodeItemsWithContext(t.Context(), podcast, items, DefaultEpisodeSyncConfig, true)
	require.Error(t, err)
	assert.Equal(t, 0, result2.Created)
	var count int64
	require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestEpisodeCapTruncationDrainsTailOnNextSync(t *testing.T) {
	server := newCursorFeedServer(t, cursorFeedXML(4))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)
	capped := DefaultEpisodeSyncConfig
	capped.MaxEpisodesPerPodcast = 2

	first, err := service.SyncPodcastEpisodes(podcast.ID, &recordingReporter{}, capped)
	require.NoError(t, err)
	assert.Equal(t, 2, first.Created)
	assert.True(t, first.Incomplete, "截断批次不得报为全部完成")
	assert.Equal(t, 2, first.RemainingItems)
	var refreshed models.Podcast
	require.NoError(t, db.First(&refreshed, podcast.ID).Error)
	assert.Nil(t, refreshed.LastEpisodeSyncAt, "截断批次不得推进游标")

	second, err := service.SyncPodcastEpisodes(podcast.ID, &recordingReporter{}, capped)
	require.NoError(t, err)
	assert.Equal(t, 2, second.Created, "下次同步补齐剩余条目")
	assert.False(t, second.Incomplete)
	require.NoError(t, db.First(&refreshed, podcast.ID).Error)
	require.NotNil(t, refreshed.LastEpisodeSyncAt)

	third, err := service.SyncPodcastEpisodes(podcast.ID, &recordingReporter{}, capped)
	require.NoError(t, err)
	assert.Equal(t, 0, third.Created)
	var count int64
	require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&count).Error)
	assert.Equal(t, int64(4), count)
}

func TestIncrementalRangeModesStayIndependent(t *testing.T) {
	server := newCursorFeedServer(t, cursorFeedXML(3))
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := importCursorPodcast(t, service, server.URL)

	// 「最近 N 天」以窗口起点为基准：3 天窗口取不到 2024 年的旧单集。
	days := 3
	ranged := DefaultEpisodeSyncConfig
	ranged.Mode = SyncModeIncremental
	ranged.TimeRangeDays = &days
	rangedResult, err := service.SyncPodcastEpisodes(podcast.ID, &recordingReporter{}, ranged)
	require.NoError(t, err)
	assert.Equal(t, 0, rangedResult.Created, "最近 N 天范围不得写入窗口外旧单集")

	// 「全部历史」写入全部旧单集。
	full := DefaultEpisodeSyncConfig
	full.Mode = SyncModeFull
	fullResult, err := service.SyncPodcastEpisodes(podcast.ID, &recordingReporter{}, full)
	require.NoError(t, err)
	assert.Equal(t, 3, fullResult.Created)
	var count int64
	require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&count).Error)
	assert.Equal(t, int64(3), count)

	// 游标推进后，「自上次更新」以游标为基准，只取游标之后的新单集。
	future := time.Now().UTC().Add(time.Minute)
	liveServer := newCursorFeedServer(t, cursorFeedXMLWithExtra(3, "Newest", "cursor-ep-new", future))
	result, err := service.SyncPodcastEpisodes(podcast.ID, &recordingReporter{}, DefaultEpisodeSyncConfig)
	require.NoError(t, err)
	_ = liveServer
	_ = result
	sinceLast := DefaultEpisodeSyncConfig
	sinceLast.Mode = SyncModeIncremental

	// 换到提供新单集的源地址后，增量同步只补新条目，不重复创建旧条目。
	require.NoError(t, db.Model(&models.Podcast{}).Where("id = ?", podcast.ID).
		Update("feed_url", liveServer.URL+"/feed.xml").Error)
	incResult, err := service.SyncPodcastEpisodes(podcast.ID, &recordingReporter{}, sinceLast)
	require.NoError(t, err)
	assert.Equal(t, 1, incResult.Created)
	var count2 int64
	require.NoError(t, db.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&count2).Error)
	assert.Equal(t, int64(4), count2)

	repeat, err := service.SyncPodcastEpisodes(podcast.ID, &recordingReporter{}, sinceLast)
	require.NoError(t, err)
	assert.Equal(t, 0, repeat.Created)
}
