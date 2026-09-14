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
	// 来源身份带形态命名空间，避免与同字符串的清单 ID 混同。
	assert.Equal(t, "campaign:"+sampleCampaignSlug, result.ExternalID)
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

func TestPreview_CampaignSlugCollidingWithCollectionIDStaysDistinct(t *testing.T) {
	// 复审场景：专题 slug 恰为已有清单的 24 位十六进制 ID 时，
	// 两种来源必须保持各自独立的清单身份。
	hexID := sampleCollectionID
	responses := map[string]string{
		"https://www.xiaoyuzhoufm.com/collection/episode/" + hexID: loadSampleHTML(t),
		"https://collection.xiaoyuzhoufm.com/" + hexID: `{"data":{"slug":"` + hexID + `","config":{"share":{"title":"专题"}},"components":[{"kind":"EPISODE_LIST","items":[` +
			`{"kind":"EPISODE","episode":{"eid":"aaa","title":"一","podcast":{"pid":"p","title":"播"}}}]}]}}`,
	}
	service := newStubService(t, stubFetcher(func(pageURL string) (int, string) {
		body, ok := responses[pageURL]
		if !ok {
			return http.StatusNotFound, ""
		}
		return http.StatusOK, body
	}, nil))

	collectionPreview, err := service.Preview(context.Background(),
		"https://www.xiaoyuzhoufm.com/collection/episode/"+hexID)
	require.NoError(t, err)
	importedCollection, err := service.ConfirmImport(collectionPreview.PreviewID)
	require.NoError(t, err)

	campaignPreview, err := service.Preview(context.Background(),
		"https://collection.xiaoyuzhoufm.com/"+hexID)
	require.NoError(t, err)
	assert.False(t, campaignPreview.Duplicate, "同字符串的专题 slug 不得命中清单身份")
	importedCampaign, err := service.ConfirmImport(campaignPreview.PreviewID)
	require.NoError(t, err)
	assert.NotEqual(t, importedCollection.CollectionID, importedCampaign.CollectionID)
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
