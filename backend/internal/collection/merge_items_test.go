package collection

import (
	"context"
	"net/http"
	"os"
	"testing"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDuplicateItemsImportRefreshAndAdopt(t *testing.T) {
	a := ItemDraft{ExternalEpisodeID: "a", ExternalPodcastID: "p", PodcastTitle: "播客", EpisodeTitle: "同标题", Recommendation: "第一段\n内部换行", PayType: PayTypeFree}
	b := a
	b.ExternalEpisodeID = "b"
	b.Recommendation = ""
	second := a
	second.Recommendation = " 第二条 "
	blank := a
	blank.Recommendation = "  "
	draft := &Draft{ExternalID: sampleCollectionID, Title: "清单", Items: []ItemDraft{a, b, second, a, blank}}
	body := nextDataHTMLFromDraft(t, draft)
	s := newStubService(t, stubFetcher(func(string) (int, string) { return http.StatusOK, body }, nil))
	p := samplePreview(t, s)
	assert.Equal(t, 5, p.SourceItemCount)
	assert.Equal(t, 3, p.DuplicateItemCount)
	require.Len(t, p.Items, 2)
	assert.Equal(t, "第一段\n内部换行\n\n第二条", p.Items[0].Recommendation)
	saved, err := s.ConfirmImport(p.PreviewID)
	require.NoError(t, err)
	detail, err := s.GetCollection(saved.CollectionID)
	require.NoError(t, err)
	assert.Equal(t, p.Items[0].Recommendation, detail.Items[0].Recommendation)
	adopted, err := serviceAdopt(t, s, saved.CollectionID, detail.Items[0].ID)
	require.NoError(t, err)
	// Personal queue must survive both source updates and repeated adoption.
	require.NoError(t, s.db.Model(&models.EpisodeTriageDecision{}).Where("episode_id = ?", adopted.EpisodeID).Update("queue_state", "focus").Error)
	again, err := serviceAdopt(t, s, saved.CollectionID, detail.Items[0].ID)
	require.NoError(t, err)
	assert.Equal(t, adopted.EpisodeID, again.EpisodeID)
	duplicate := samplePreview(t, s)
	assert.True(t, duplicate.Duplicate)
	for _, tc := range []struct {
		items            []ItemDraft
		changed, removed int
		text             string
	}{
		{[]ItemDraft{a, b, second}, 0, 0, "第一段\n内部换行\n\n第二条"},
		{[]ItemDraft{second, b, a}, 1, 0, "第二条\n\n第一段\n内部换行"},
		{[]ItemDraft{a, b}, 1, 0, "第一段\n内部换行"},
		{[]ItemDraft{b}, 0, 1, ""},
	} {
		draft.Items = tc.items
		body = nextDataHTMLFromDraft(t, draft)
		refresh, err := s.RefreshPreview(context.Background(), saved.CollectionID)
		require.NoError(t, err)
		assert.Equal(t, tc.changed, refresh.Changes.RecommendationChanged)
		assert.Equal(t, tc.removed, refresh.Changes.RemovedCount)
		_, err = s.ApplyRefresh(saved.CollectionID, refresh.PreviewID, refresh.BaseRev)
		require.NoError(t, err)
		got, err := s.GetCollection(saved.CollectionID)
		require.NoError(t, err)
		if tc.text != "" {
			assert.Equal(t, tc.text, got.Items[0].Recommendation)
		}
	}
	var episode models.EpisodeTriageDecision
	require.NoError(t, s.db.Where("episode_id = ?", adopted.EpisodeID).First(&episode).Error)
	require.NotNil(t, episode.QueueState)
	assert.Equal(t, "focus", *episode.QueueState)
}

func TestDuplicateItemsPreserveMediaRestrictions(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		first := ItemDraft{ExternalEpisodeID: "a", ExternalPodcastID: "p", AudioURL: "https://example.com/a", PayType: PayTypeFree}
		restricted := first
		restricted.AudioURL = ""
		restricted.IsPrivateMedia = true
		items := []ItemDraft{first, restricted}
		if reverse {
			items = []ItemDraft{restricted, first}
		}
		draft, err := mergeDraftItems(&Draft{Items: items})
		require.NoError(t, err)
		assert.Empty(t, draft.Items[0].AudioURL)
		assert.True(t, draft.Items[0].IsPrivateMedia)
	}
	// Remember non-empty identity even when the first occurrence lacks it.
	_, err := mergeDraftItems(&Draft{Items: []ItemDraft{{ExternalEpisodeID: "a"}, {ExternalEpisodeID: "a", ExternalPodcastID: "p"}, {ExternalEpisodeID: "a", ExternalPodcastID: "other"}}})
	require.ErrorIs(t, err, ErrIncompleteSource)
}

func TestRealDuplicateCollectionSample(t *testing.T) {
	body, err := os.ReadFile("testdata/duplicate_collection.html")
	require.NoError(t, err)
	s := newStubService(t, stubFetcher(func(string) (int, string) { return http.StatusOK, string(body) }, nil))
	p, err := s.Preview(context.Background(), "https://www.xiaoyuzhoufm.com/collection/episode/69aaa2becc9f90f6da4d4d3c?s=share")
	require.NoError(t, err)
	assert.Equal(t, 6, p.SourceItemCount)
	assert.Equal(t, 1, p.DuplicateItemCount)
	require.Len(t, p.Items, 5)
	assert.Equal(t, "微观视角中看到真实的女性勇气\n\n巴列维时期伊朗女性自由的真相", p.Items[2].Recommendation)
	saved, err := s.ConfirmImport(p.PreviewID)
	require.NoError(t, err)
	got, err := s.GetCollection(saved.CollectionID)
	require.NoError(t, err)
	require.Len(t, got.Items, 5)
	assert.Equal(t, p.Items[2].Recommendation, got.Items[2].Recommendation)
}

func TestDuplicateItemsDoNotMaskIncompleteSource(t *testing.T) {
	entry := map[string]any{"eid": "a", "title": "一", "podcast": map[string]any{"title": "播"}, "recommendation": "甲"}
	payload := map[string]any{"id": sampleCollectionID, "title": "清单", "targetType": "EPISODE", "target": []any{entry, entry}, "totalCount": 2}
	got, err := ParsePageHTML(nextDataHTML(t, payload), sampleCollectionID)
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	assert.Equal(t, "甲", got.Items[0].Recommendation)
	payload["totalCount"] = 1
	_, err = ParsePageHTML(nextDataHTML(t, payload), sampleCollectionID)
	require.ErrorIs(t, err, ErrIncompleteSource)
	payload["totalCount"] = 2
	payload["target"] = []any{entry, map[string]any{"eid": "a", "title": "二"}}
	_, err = ParsePageHTML(nextDataHTML(t, payload), sampleCollectionID)
	require.ErrorIs(t, err, ErrIncompleteSource)
}
