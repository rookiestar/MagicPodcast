package collection

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 固定样例来自 2026-09-12 现场读取的《穿透半导体迷雾》公开清单页；本地归档时
// 仅截断了超长 Show Notes 正文。现场内容如变化（计数、互动数）不属于断言范围。
const (
	sampleCollectionID = "6a20323b78a52c96d821a769"
	sampleTitle        = "穿透半导体迷雾"
	sampleAuthor       = "小宇宙领航员"
)

var sampleFirstRecommendations = []string{
	"存储芯片为何五年内持续短缺？",
	"中国半导体产业如何启程？",
	"英伟达对下一阶段AI时代的判断",
	"存储芯片为何堪比一套房？",
	"芯片行业还有哪些机会？",
	"功率半导体出海的真实挑战",
	"汽车芯片会是未来最大赛道？",
	"工业体系决定芯片战走向",
}

var sampleEIDs = []string{
	"6a1c07b0ac7bdb080c3397a9",
	"636274672e925d51c119478e",
	"6a1e8760122ade5348e85027",
	"699ecd45de29766da97fbf61",
	"635b5fce1d21a50d56b196ef",
	"60d8f65625da4f997b8ba4ba",
	"64b8c16e5680f4d4a83f251a",
	"6926b1a9f8a9e1d16282e8e8",
}

func loadSampleHTML(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/collection_sample.html")
	require.NoError(t, err)
	return string(raw)
}

func TestParsePageHTML_SampleParsesEightDistinctEpisodesInOrder(t *testing.T) {
	draft, err := ParsePageHTML(loadSampleHTML(t), sampleCollectionID)
	require.NoError(t, err)

	assert.Equal(t, PlatformXiaoyuzhoufm, draft.Platform)
	assert.Equal(t, sampleCollectionID, draft.ExternalID)
	assert.Equal(t, sampleTitle, draft.Title)
	assert.Equal(t, sampleAuthor, draft.Author)
	assert.NotEmpty(t, draft.Description)
	// 当前源格式没有总数或分页标记，只能如实表示已读取条数。
	assert.False(t, draft.TotalKnown)
	require.Len(t, draft.Items, 8)

	seen := make(map[string]struct{}, len(draft.Items))
	for index, item := range draft.Items {
		assert.Equal(t, sampleEIDs[index], item.ExternalEpisodeID, "原始顺序必须保留")
		assert.NotEmpty(t, item.ExternalPodcastID)
		assert.NotEmpty(t, item.PodcastTitle)
		assert.NotEmpty(t, item.EpisodeTitle)
		assert.Equal(t, sampleFirstRecommendations[index], item.Recommendation)
		assert.Equal(t, EpisodeURLForEID(item.ExternalEpisodeID), item.EpisodeURL)
		assert.Equal(t, PayTypeFree, item.PayType)
		assert.False(t, item.IsPrivateMedia)
		seen[item.ExternalEpisodeID] = struct{}{}
	}
	assert.Len(t, seen, 8, "8 个条目必须是 8 个不同单集身份")

	first := draft.Items[0]
	assert.Equal(t, 4692, first.Duration)
	require.NotNil(t, first.PublishedAt)
	assert.Equal(t, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), *first.PublishedAt)
	assert.NotEmpty(t, first.PodcastAuthor, "投资实战派")
}

func TestParsePageHTML_MissingRecommendationStaysEmpty(t *testing.T) {
	html := nextDataHTML(t, map[string]any{
		"id": "aaaaaaaaaaaaaaaaaaaaaaaa", "title": "缺推荐语清单", "targetType": "EPISODE",
		"target": []map[string]any{{
			"eid": "bbbbbbbbbbbbbbbbbbbbbbbb", "pid": "cccccccccccccccccccccccc",
			"title": "没有推荐语的单集", "podcast": map[string]any{"pid": "cccccccccccccccccccccccc", "title": "某节目"},
		}},
	})
	draft, err := ParsePageHTML(html, "")
	require.NoError(t, err)
	require.Len(t, draft.Items, 1)
	assert.Empty(t, draft.Items[0].Recommendation, "推荐语缺失时保留空值，不编造")
	assert.Empty(t, draft.Items[0].PublishedAt)
	assert.Equal(t, 0, draft.Items[0].Duration)
}

func TestParsePageHTML_RejectsUnsupportedAndBrokenStructures(t *testing.T) {
	base := map[string]any{
		"id": "aaaaaaaaaaaaaaaaaaaaaaaa", "title": "清单", "targetType": "EPISODE",
		"target": []map[string]any{{"eid": "bbbbbbbbbbbbbbbbbbbbbbbb", "title": "t", "podcast": map[string]any{"title": "p"}}},
	}

	cases := []struct {
		name    string
		payload map[string]any
		wantErr error
	}{
		{"节目型清单", func() map[string]any {
			p := cloneMap(base)
			p["targetType"] = "PODCAST"
			return p
		}(), ErrUnsupportedTargetType},
		{"缺target键", func() map[string]any {
			p := cloneMap(base)
			delete(p, "target")
			return p
		}(), ErrIncompleteSource},
		{"缺标题", func() map[string]any {
			p := cloneMap(base)
			p["title"] = ""
			return p
		}(), ErrIncompleteSource},
		{"清单内重复单集", func() map[string]any {
			p := cloneMap(base)
			p["target"] = []map[string]any{
				{"eid": "bbbbbbbbbbbbbbbbbbbbbbbb", "title": "a", "podcast": map[string]any{"title": "p"}},
				{"eid": "bbbbbbbbbbbbbbbbbbbbbbbb", "title": "b", "podcast": map[string]any{"title": "p"}},
			}
			return p
		}(), ErrDuplicateItems},
		{"条目缺节目名", func() map[string]any {
			p := cloneMap(base)
			p["target"] = []map[string]any{{"eid": "bbbbbbbbbbbbbbbbbbbbbbbb", "title": "a"}}
			return p
		}(), ErrIncompleteSource},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePageHTML(nextDataHTML(t, tc.payload), "")
			require.ErrorIs(t, err, tc.wantErr)
		})
	}

	t.Run("页面缺结构化数据", func(t *testing.T) {
		_, err := ParsePageHTML("<html><body>登录后查看</body></html>", "")
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
	t.Run("JSON损坏", func(t *testing.T) {
		_, err := ParsePageHTML(`<script id="__NEXT_DATA__" type="application/json">{broken</script>`, "")
		require.ErrorIs(t, err, ErrIncompleteSource)
	})
}

func TestParsePageHTML_EmptyTargetIsDistinctEmptyCollection(t *testing.T) {
	html := nextDataHTML(t, map[string]any{
		"id": "aaaaaaaaaaaaaaaaaaaaaaaa", "title": "空清单", "targetType": "EPISODE",
		"target": []map[string]any{},
	})
	_, err := ParsePageHTML(html, "")
	require.ErrorIs(t, err, ErrEmptyCollection)
}

func TestParsePageHTML_IDMismatchRejected(t *testing.T) {
	_, err := ParsePageHTML(loadSampleHTML(t), "ffffffffffffffffffffffff")
	require.ErrorIs(t, err, ErrIncompleteSource)
}

// nextDataHTML 用与源站相同的脚本结构包装测试数据。
func nextDataHTML(t *testing.T, payload map[string]any) string {
	t.Helper()
	wrapped := map[string]any{
		"props": map[string]any{
			"pageProps": map[string]any{"collection": payload},
		},
	}
	raw, err := json.Marshal(wrapped)
	require.NoError(t, err)
	return `<html><head><script id="__NEXT_DATA__" type="application/json">` +
		string(raw) + `</script></head><body></body></html>`
}

func cloneMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func TestReviewParserRejectsExplicitIncompletePage(t *testing.T) {
	for _, extra := range []map[string]any{{"hasMore": true}, {"totalCount": 2}, {"loadMoreKey": "next"}} {
		p := map[string]any{"id": sampleCollectionID, "title": "partial", "targetType": "EPISODE", "target": []map[string]any{{"eid": sampleEIDs[0], "pid": "p1", "title": "ep", "podcast": map[string]any{"title": "podcast"}}}}
		for k, v := range extra {
			p[k] = v
		}
		_, err := ParsePageHTML(nextDataHTML(t, p), sampleCollectionID)
		require.ErrorIs(t, err, ErrIncompleteSource)
	}
}
