package collection

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const campaignURL = "https://collection.xiaoyuzhoufm.com/wavesfilm2026"

func TestPreview_CampaignFetchesCanonicalPageURLAndBindsPreview(t *testing.T) {
	var calls []string
	service := newStubService(t, stubFetcher(func(pageURL string) (int, string) {
		return http.StatusOK, loadSampleCampaignJSON(t)
	}, &calls))

	result, err := service.Preview(context.Background(), campaignURL+"?utm=x")
	require.NoError(t, err)
	require.Len(t, calls, 1)
	// 抓取入口仍是规范化页面地址；接口地址由生产抓取器按来源形态构造。
	assert.Equal(t, campaignURL, calls[0])
	assert.Equal(t, sampleCampaignSlug, result.ExternalID)
	assert.Equal(t, sampleCampaignTitle, result.Title)
	assert.Equal(t, campaignURL, result.SourceURL)
	assert.Len(t, result.Items, len(sampleCampaignEIDs))
	require.Len(t, result.Items, 2)
	assert.Equal(t, "银河影评", result.Items[0].PodcastTitle)
	assert.Equal(t, "感受人与光影的海边共鸣", result.Items[0].Recommendation)
}

func TestConfirmImport_CampaignSavesSourcePageURL(t *testing.T) {
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		return http.StatusOK, loadSampleCampaignJSON(t)
	}, nil))

	preview, err := service.Preview(context.Background(), campaignURL)
	require.NoError(t, err)
	imported, err := service.ConfirmImport(preview.PreviewID)
	require.NoError(t, err)
	require.False(t, imported.Duplicate)

	detail, err := service.GetCollection(imported.CollectionID)
	require.NoError(t, err)
	assert.Equal(t, sampleCampaignTitle, detail.Title)
	assert.Equal(t, campaignURL, detail.SourceURL)
	require.Len(t, detail.Items, len(sampleCampaignEIDs))
	for index, item := range detail.Items {
		assert.Nil(t, item.AdoptedEpisodeID, "导入不收录任何单集")
		assert.Equal(t, index, item.Position)
	}
}

func TestPreview_CampaignDeduplicatesByStableSourceIdentity(t *testing.T) {
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		return http.StatusOK, loadSampleCampaignJSON(t)
	}, nil))

	preview, err := service.Preview(context.Background(), campaignURL)
	require.NoError(t, err)
	imported, err := service.ConfirmImport(preview.PreviewID)
	require.NoError(t, err)

	again, err := service.Preview(context.Background(), campaignURL)
	require.NoError(t, err)
	assert.True(t, again.Duplicate)
	require.NotNil(t, again.ExistingCollectionID)
	assert.Equal(t, imported.CollectionID, *again.ExistingCollectionID)
}

func TestRefreshPreview_CampaignUsesStoredSourceIdentity(t *testing.T) {
	responses := []string{loadSampleCampaignJSON(t)}
	// 刷新时专题新增了一个已发布条目（预告位转正式发布）。
	responses = append(responses, `{"data":{"slug":"wavesfilm2026","config":{"share":{"title":"`+sampleCampaignTitle+`"}},`+
		`"components":[{"kind":"EPISODE_LIST","items":[`+
		`{"kind":"EPISODE","episode":{"eid":"`+sampleCampaignEIDs[0]+`","title":"一","podcast":{"pid":"p1","title":"播一"}}},`+
		`{"kind":"EPISODE","episode":{"eid":"`+sampleCampaignEIDs[1]+`","title":"二","podcast":{"pid":"p2","title":"播二"}}},`+
		`{"kind":"EPISODE","episode":{"eid":"bbb222","title":"三","podcast":{"pid":"p3","title":"播三"}}}]}]}}`)
	call := 0
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		body := responses[call]
		call++
		return http.StatusOK, body
	}, nil))

	preview, err := service.Preview(context.Background(), campaignURL)
	require.NoError(t, err)
	imported, err := service.ConfirmImport(preview.PreviewID)
	require.NoError(t, err)

	refresh, err := service.RefreshPreview(context.Background(), imported.CollectionID)
	require.NoError(t, err)
	assert.Equal(t, 1, refresh.Changes.AddedCount)
	assert.Equal(t, 3, refresh.ReadCount)
	assert.Len(t, refresh.Items, 3)
}
