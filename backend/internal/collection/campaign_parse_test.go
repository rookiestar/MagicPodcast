package collection

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 固定样例来自 2026-09-14 现场读取的《海浪电影周》专题接口真实响应
// （collection.xiaoyuzhoufm.com/wavesfilm2026）；本地归档时仅截断了超长
// Show Notes 正文。专题同时包含已发布条目与未发布预告位，这是专题页的正常结构。
const (
	sampleCampaignSlug  = "wavesfilm2026"
	sampleCampaignTitle = "海浪电影周 播客特别企划：世界在每个清晨重启"
)

var sampleCampaignEIDs = []string{
	"6a905ad0ef65145dfcc67d3c",
	"6aa717c2bc0db5338980c34c",
}

func loadSampleCampaignJSON(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/campaign_sample.json")
	require.NoError(t, err)
	return string(raw)
}

func TestParseCampaignJSON_SampleParsesPublishedEpisodesSkippingTeasers(t *testing.T) {
	draft, err := ParseCampaignJSON(loadSampleCampaignJSON(t), sampleCampaignSlug)
	require.NoError(t, err)
	assert.Equal(t, PlatformXiaoyuzhoufm, draft.Platform)
	assert.Equal(t, sampleCampaignSlug, draft.ExternalID)
	assert.Equal(t, sampleCampaignTitle, draft.Title)
	assert.False(t, draft.TotalKnown)
	require.Len(t, draft.Items, len(sampleCampaignEIDs), "未发布预告位跳过，不进入清单")
	for index, item := range draft.Items {
		assert.Equal(t, sampleCampaignEIDs[index], item.ExternalEpisodeID)
		assert.Equal(t, EpisodeURLForEID(sampleCampaignEIDs[index]), item.EpisodeURL)
	}

	first := draft.Items[0]
	assert.Equal(t, "Ep151 海浪电影周前瞻：当经典与新锐交织，当此岸与世界对话", first.EpisodeTitle)
	assert.Equal(t, "感受人与光影的海边共鸣", first.Recommendation)
	assert.Equal(t, "银河影评", first.PodcastTitle)
	assert.Equal(t, "银河影音空间", first.PodcastAuthor)
	assert.Equal(t, "665d2080109557d1486237b9", first.ExternalPodcastID)
	assert.Equal(t, 157, first.PodcastEpisodeCount)
	assert.Equal(t, 1772, first.Duration)
	assert.Equal(t, PayTypeFree, first.PayType)
	assert.False(t, first.IsPrivateMedia)
	assert.NotEmpty(t, first.AudioURL, "公开免费单集保留音频地址快照")
	assert.NotEmpty(t, first.ImageURL)
	require.NotNil(t, first.PublishedAt)
	assert.Equal(t, "2026-08-28", first.PublishedAt.UTC().Format("2006-01-02"))

	second := draft.Items[1]
	assert.Equal(t, "Vibration 歪波音室", second.PodcastTitle)
	assert.Equal(t, "聆听浪潮与影视旋律的浪漫共振", second.Recommendation)
}

func TestParseCampaignJSON_RejectsUntrustedStructures(t *testing.T) {
	valid := loadSampleCampaignJSON(t)

	t.Run("slug不匹配", func(t *testing.T) {
		_, err := ParseCampaignJSON(valid, "other-slug")
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
	t.Run("响应非JSON", func(t *testing.T) {
		_, err := ParseCampaignJSON("<html>404</html>", sampleCampaignSlug)
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
	t.Run("data缺失", func(t *testing.T) {
		_, err := ParseCampaignJSON(`{"data":null}`, sampleCampaignSlug)
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
	t.Run("标题缺失", func(t *testing.T) {
		_, err := ParseCampaignJSON(`{"data":{"slug":"wavesfilm2026","config":{"share":{"title":" "}},"components":[]}}`, sampleCampaignSlug)
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
	t.Run("重复eid", func(t *testing.T) {
		duplicated := `{"data":{"slug":"wavesfilm2026","config":{"share":{"title":"专题"}},"components":[{"kind":"EPISODE_LIST","items":[` +
			`{"kind":"EPISODE","episode":{"eid":"aaa","title":"一","podcast":{"pid":"p","title":"播"}}},` +
			`{"kind":"EPISODE","episode":{"eid":"aaa","title":"二","podcast":{"pid":"p","title":"播"}}}]}]}}`
		draft, err := ParseCampaignJSON(duplicated, sampleCampaignSlug)
		require.NoError(t, err)
		require.Len(t, draft.Items, 1)
		assert.Equal(t, 1, draft.DuplicateItemCount)
	})
	t.Run("已发布条目缺节目标题", func(t *testing.T) {
		missing := `{"data":{"slug":"wavesfilm2026","config":{"share":{"title":"专题"}},"components":[{"kind":"EPISODE_LIST","items":[` +
			`{"kind":"EPISODE","episode":{"eid":"aaa","title":"一"}}}]}}`
		_, err := ParseCampaignJSON(missing, sampleCampaignSlug)
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
}

func TestParseCampaignJSON_NoEpisodeListIsUnsupportedTarget(t *testing.T) {
	podcastOnly := `{"data":{"slug":"wavesfilm2026","config":{"share":{"title":"专题"}},"components":[{"kind":"PODCAST_LIST","items":[]}]}}`
	_, err := ParseCampaignJSON(podcastOnly, sampleCampaignSlug)
	require.ErrorIs(t, err, ErrUnsupportedTargetType)
}

func TestParseCampaignJSON_AllTeasersIsEmptyCollection(t *testing.T) {
	teasersOnly := `{"data":{"slug":"wavesfilm2026","config":{"share":{"title":"专题"}},"components":[{"kind":"EPISODE_LIST","items":[` +
		`{"kind":"EPISODE","quote":"即将上线"},` +
		`{"kind":"EPISODE","episode":null,"quote":"即将上线"}]}]}}`
	draft, err := ParseCampaignJSON(teasersOnly, sampleCampaignSlug)
	require.ErrorIs(t, err, ErrEmptyCollection)
	require.NotNil(t, draft)
	assert.Empty(t, draft.Items)
}

func TestParseCampaignJSON_PrivateMediaKeepsNoAudioSnapshot(t *testing.T) {
	private := `{"data":{"slug":"wavesfilm2026","config":{"share":{"title":"专题"}},"components":[{"kind":"EPISODE_LIST","items":[` +
		`{"kind":"EPISODE","quote":"q","episode":{"eid":"aaa","title":"一","podcast":{"pid":"p","title":"播"},` +
		`"isPrivateMedia":true,"enclosure":{"url":"https://media.example.com/a.m4a"}}}]}]}}`
	draft, err := ParseCampaignJSON(private, sampleCampaignSlug)
	require.NoError(t, err)
	require.Len(t, draft.Items, 1)
	assert.Empty(t, draft.Items[0].AudioURL)
}

func TestParseCampaignJSON_InvalidPubDateKeepsNil(t *testing.T) {
	invalid := `{"data":{"slug":"wavesfilm2026","config":{"share":{"title":"专题"}},"components":[{"kind":"EPISODE_LIST","items":[` +
		`{"kind":"EPISODE","episode":{"eid":"aaa","title":"一","podcast":{"pid":"p","title":"播"},"pubDate":"not-a-time"}}]}]}}`
	draft, err := ParseCampaignJSON(invalid, sampleCampaignSlug)
	require.NoError(t, err)
	require.Len(t, draft.Items, 1)
	assert.Nil(t, draft.Items[0].PublishedAt)
}
