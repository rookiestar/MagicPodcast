package collection

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 固定样例来自 2026-09-14 现场读取的《00后的宇宙必听》活动接口真实响应
// （h5.xiaoyuzhoufm.com/xyz-activity/forgenz）；本地归档时仅保留了两个单集
// 列表模块的前几条并截断了图片载荷。活动页由图片模块与单集列表模块组成，
// 单集条目字段比清单/专题更薄（无 Show Notes、发布时间与节目元数据对象）。
const sampleActivityCode = "forgenz"

func loadSampleActivityJSON(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/activity_sample.json")
	require.NoError(t, err)
	return string(raw)
}

func TestParseActivityJSON_SampleFlattensEpisodeListsInPageOrder(t *testing.T) {
	draft, err := ParseActivityJSON(loadSampleActivityJSON(t), "forgenz")
	require.NoError(t, err)
	assert.Equal(t, PlatformXiaoyuzhoufm, draft.Platform)
	assert.Equal(t, "forgenz", draft.ExternalID)
	assert.Equal(t, "00后的宇宙必听｜给正在长大的你", draft.Title)
	assert.False(t, draft.TotalKnown)
	require.Len(t, draft.Items, 3, "图片模块跳过，两个列表模块按页面顺序展平")

	first := draft.Items[0]
	assert.Equal(t, "65e75676d15a20dbcab730ca", first.ExternalEpisodeID)
	assert.Equal(t, "vol.02 我的前半生：如何获得底层自信", first.EpisodeTitle)
	assert.Equal(t, "建立自信，找到不被动摇的内核", first.Recommendation)
	assert.Equal(t, "天真不天真", first.PodcastTitle)
	assert.Equal(t, "65cef9e3cace72dff8d98de3", first.ExternalPodcastID)
	assert.Equal(t, 2913, first.Duration)
	assert.Equal(t, PayTypeFree, first.PayType)
	assert.Equal(t, EpisodeURLForEID(first.ExternalEpisodeID), first.EpisodeURL)
	assert.NotEmpty(t, first.AudioURL, "公开免费单集保留音频地址快照")
	assert.Equal(t, "audio/mp4", first.AudioMimeType)
	assert.NotEmpty(t, first.ImageURL)
	assert.Empty(t, first.PodcastCoverURL, "活动载荷没有节目封面，单集图片不冒充节目封面")
	assert.Empty(t, first.Shownotes, "活动载荷没有 Show Notes，保留空值")
	assert.Nil(t, first.PublishedAt, "活动载荷没有发布时间，不编造日期")
	assert.Empty(t, first.PodcastAuthor, "活动载荷没有节目作者，保留空值")
	assert.Zero(t, first.PodcastEpisodeCount)

	// 展平顺序跨模块保持：第三条来自第二个列表模块。
	third := draft.Items[2]
	assert.Equal(t, "695cdfd921cd486af739424c", third.ExternalEpisodeID)
	assert.Equal(t, "钱婧老师的会客厅", third.PodcastTitle)
}

func TestParseActivityJSON_RejectsUntrustedStructures(t *testing.T) {
	valid := loadSampleActivityJSON(t)

	t.Run("code不匹配", func(t *testing.T) {
		_, err := ParseActivityJSON(valid, "other-code")
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
	t.Run("未知code返回空data", func(t *testing.T) {
		_, err := ParseActivityJSON(`{"data":null}`, sampleActivityCode)
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
	t.Run("响应非JSON", func(t *testing.T) {
		_, err := ParseActivityJSON("<html>404</html>", sampleActivityCode)
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
	t.Run("标题缺失", func(t *testing.T) {
		_, err := ParseActivityJSON(`{"data":{"code":"forgenz","title":" ","modules":[]}}`, sampleActivityCode)
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
	t.Run("重复单集", func(t *testing.T) {
		duplicated := `{"data":{"code":"forgenz","title":"活动","modules":[` +
			`{"type":"EPISODE_VERTICAL_LIST","episodes":[` +
			`{"id":"aaa","title":"一","podcastTitle":"播"},` +
			`{"id":"aaa","title":"二","podcastTitle":"播"}]}]}}`
		draft, err := ParseActivityJSON(duplicated, sampleActivityCode)
		require.NoError(t, err)
		require.Len(t, draft.Items, 1)
		assert.Equal(t, 1, draft.DuplicateItemCount)
	})
	t.Run("条目缺节目标题", func(t *testing.T) {
		missing := `{"data":{"code":"forgenz","title":"活动","modules":[` +
			`{"type":"EPISODE_VERTICAL_LIST","episodes":[{"id":"aaa","title":"一"}]}]}}`
		_, err := ParseActivityJSON(missing, sampleActivityCode)
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
}

func TestParseActivityJSON_NoEpisodeListIsUnsupportedTarget(t *testing.T) {
	imagesOnly := `{"data":{"code":"forgenz","title":"活动","modules":[{"type":"IMAGE","image":{}}]}}`
	_, err := ParseActivityJSON(imagesOnly, sampleActivityCode)
	require.ErrorIs(t, err, ErrUnsupportedTargetType)
}

func TestParseActivityJSON_AllListsEmptyIsEmptyCollection(t *testing.T) {
	empty := `{"data":{"code":"forgenz","title":"活动","modules":[{"type":"EPISODE_VERTICAL_LIST","episodes":[]}]}}`
	draft, err := ParseActivityJSON(empty, sampleActivityCode)
	require.ErrorIs(t, err, ErrEmptyCollection)
	require.NotNil(t, draft)
	assert.Empty(t, draft.Items)
}

func TestParseActivityJSON_PaidOrUnknownPayTypeKeepsNoAudioSnapshot(t *testing.T) {
	paid := `{"data":{"code":"forgenz","title":"活动","modules":[{"type":"EPISODE_VERTICAL_LIST","episodes":[` +
		`{"id":"aaa","title":"一","podcastTitle":"播","payType":"PAY_EPISODE_PODCAST",` +
		`"media":{"source":{"mode":"PUBLIC","url":"https://media.example.com/a.m4a"}}},` +
		`{"id":"bbb","title":"二","podcastTitle":"播","payType":"FREE",` +
		`"media":{"source":{"mode":"LOGIN","url":"https://media.example.com/b.m4a"}}},` +
		`{"id":"ccc","title":"三","podcastTitle":"播",` +
		`"media":{"source":{"mode":"PUBLIC","url":"https://media.example.com/c.m4a"}}}]}]}}`
	draft, err := ParseActivityJSON(paid, sampleActivityCode)
	require.NoError(t, err)
	require.Len(t, draft.Items, 3)
	assert.Empty(t, draft.Items[0].AudioURL, "付费单集不留音频快照")
	assert.Empty(t, draft.Items[1].AudioURL, "非公开来源不留音频快照")
	assert.Empty(t, draft.Items[2].AudioURL, "付费状态未知（缺失）不留音频快照")
}
