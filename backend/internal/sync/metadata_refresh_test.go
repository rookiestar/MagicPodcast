package sync

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// revisionFeedXML 生成可控修订版本的 RSS：集数、发布日期、音频 URL 恒定，
// 仅可选择的变更频道简介或旧单集 Show Notes（#398 R6/R7 验收驱动）。
func revisionFeedXML(channelDescription, ep1Notes, ep2Notes string, includeChannelMeta bool) string {
	channelMeta := ""
	if includeChannelMeta {
		channelMeta = fmt.Sprintf(`<description>%s</description><itunes:author>原作者</itunes:author>`,
			channelDescription)
	} else if channelDescription != "" {
		// 仅缺 author：description 保留，用于缺字段不清空场景。
		channelMeta = `<description>` + channelDescription + `</description>`
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd" version="2.0"><channel>
<title>Revision Feed</title>` + channelMeta + `
<item><title>EP1</title><guid>rev-1</guid><pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
<description>` + ep1Notes + `</description><enclosure url="https://cdn.example/ep1.mp3" type="audio/mpeg" length="100"/></item>
<item><title>EP2</title><guid>rev-2</guid><pubDate>Tue, 02 Jan 2024 00:00:00 GMT</pubDate>
<description>` + ep2Notes + `</description><enclosure url="https://cdn.example/ep2.mp3" type="audio/mpeg" length="200"/></item>
</channel></rss>`
}

func newRevisionFeedServer(t *testing.T) (*httptest.Server, *[]byte) {
	t.Helper()
	body := []byte(revisionFeedXML("原始简介", "原始EP1", "原始EP2", true))
	bodyPtr := &body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveRobotsNotFoundSync(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server, bodyPtr
}

func TestMetadataRefreshWritesDescriptionsAndDetectsEpisodeRevisions(t *testing.T) {
	server, bodyPtr := newRevisionFeedServer(t)
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })

	podcast := &models.Podcast{
		XYZID: "revision", Title: "Revision Feed", FeedURL: server.URL,
		DataSource: "rss", IsSubscribed: true, FeedURLValid: true,
		Notes: "我的备注", MyRate: 4, CustomCoverURL: "https://cdn.example/custom.jpg",
	}
	require.NoError(t, db.Create(podcast).Error)

	// 首次检查：建立基线（两集 + 原始简介）。
	err, noUpdate, epResult := service.syncPodcastMetadataWithUpdateCheck(podcast)
	require.NoError(t, err)
	assert.False(t, noUpdate)
	require.NotNil(t, epResult)
	assert.Equal(t, 2, epResult.Created)

	// 只修订旧单集 EP1 的 Show Notes（集数、日期、音频 URL 不变）。
	*bodyPtr = []byte(revisionFeedXML("原始简介", "修订后的EP1 Show Notes", "原始EP2", true))
	err, noUpdate, epResult = service.syncPodcastMetadataWithUpdateCheck(podcast)
	require.NoError(t, err)
	assert.True(t, noUpdate, "简介与标题未变 → 元数据层面无更新；单集仍按 R7 检查修订")
	require.NotNil(t, epResult)
	assert.Equal(t, 1, epResult.Updated, "旧单集 Show Notes 实质修订必须被保存")
	assert.Equal(t, 0, epResult.Created)

	var ep1 models.Episode
	require.NoError(t, db.Where("guid = ?", "rev-1").First(&ep1).Error)
	assert.Equal(t, "修订后的EP1 Show Notes", ep1.ShowNotes)

	// 重复同步保持幂等：无实质变化 → 不再产生写入。
	err, noUpdate, epResult = service.syncPodcastMetadataWithUpdateCheck(podcast)
	require.NoError(t, err)
	assert.True(t, noUpdate)
	require.NotNil(t, epResult)
	assert.Equal(t, 0, epResult.Updated, "重复同步不得重复写入")
	assert.Equal(t, 0, epResult.Created)

	// 只变更节目简介：个人字段保持，新简介写入。
	*bodyPtr = []byte(revisionFeedXML("全新的频道简介", "修订后的EP1 Show Notes", "原始EP2", true))
	err, noUpdate, _ = service.syncPodcastMetadataWithUpdateCheck(podcast)
	require.NoError(t, err)
	assert.False(t, noUpdate)

	var refreshed models.Podcast
	require.NoError(t, db.First(&refreshed, podcast.ID).Error)
	assert.Equal(t, "全新的频道简介", refreshed.Description)
	assert.Equal(t, "我的备注", refreshed.Notes)
	assert.Equal(t, 4, refreshed.MyRate)
	assert.Equal(t, "https://cdn.example/custom.jpg", refreshed.CustomCoverURL)

	// 源站缺失字段（无 author）不得清空已有可信信息。
	*bodyPtr = []byte(revisionFeedXML("全新的频道简介", "修订后的EP1 Show Notes", "原始EP2", false))
	err, _, _ = service.syncPodcastMetadataWithUpdateCheck(podcast)
	require.NoError(t, err)
	var preserved models.Podcast
	require.NoError(t, db.First(&preserved, podcast.ID).Error)
	assert.Equal(t, "原作者", preserved.Author, "Feed 缺失字段不得无依据清空")
}
