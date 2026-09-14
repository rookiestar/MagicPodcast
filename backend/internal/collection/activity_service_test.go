package collection

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const activityURL = "https://h5.xiaoyuzhoufm.com/xyz-activity/forgenz"

func TestPreview_ActivityFetchesCanonicalPageURLAndBindsPreview(t *testing.T) {
	var calls []string
	service := newStubService(t, stubFetcher(func(pageURL string) (int, string) {
		return http.StatusOK, loadSampleActivityJSON(t)
	}, &calls))

	result, err := service.Preview(context.Background(), activityURL+"?utm=x")
	require.NoError(t, err)
	require.Len(t, calls, 1)
	// 抓取入口仍是规范化页面地址；接口地址与 POST 请求体由生产抓取器构造。
	assert.Equal(t, activityURL, calls[0])
	assert.Equal(t, "activity:forgenz", result.ExternalID, "来源身份带形态命名空间")
	assert.Equal(t, "00后的宇宙必听｜给正在长大的你", result.Title)
	assert.Equal(t, activityURL, result.SourceURL)
	assert.Len(t, result.Items, 3)
	assert.Equal(t, "天真不天真", result.Items[0].PodcastTitle)
}

func TestConfirmImport_ActivitySavesSourcePageURLAndIdentity(t *testing.T) {
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		return http.StatusOK, loadSampleActivityJSON(t)
	}, nil))

	preview, err := service.Preview(context.Background(), activityURL)
	require.NoError(t, err)
	imported, err := service.ConfirmImport(preview.PreviewID)
	require.NoError(t, err)
	require.False(t, imported.Duplicate)

	detail, err := service.GetCollection(imported.CollectionID)
	require.NoError(t, err)
	assert.Equal(t, "activity:forgenz", detail.ExternalID)
	assert.Equal(t, activityURL, detail.SourceURL)
	require.Len(t, detail.Items, 3)
	for index, item := range detail.Items {
		assert.Nil(t, item.AdoptedEpisodeID, "导入不收录任何单集")
		assert.Equal(t, index, item.Position)
	}
}

func TestPreview_ActivityDeduplicatesByStableSourceIdentity(t *testing.T) {
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		return http.StatusOK, loadSampleActivityJSON(t)
	}, nil))

	preview, err := service.Preview(context.Background(), activityURL)
	require.NoError(t, err)
	imported, err := service.ConfirmImport(preview.PreviewID)
	require.NoError(t, err)

	again, err := service.Preview(context.Background(), activityURL)
	require.NoError(t, err)
	assert.True(t, again.Duplicate)
	require.NotNil(t, again.ExistingCollectionID)
	assert.Equal(t, imported.CollectionID, *again.ExistingCollectionID)
}

func TestRefreshPreview_ActivityUsesStoredSourceIdentity(t *testing.T) {
	responses := []string{loadSampleActivityJSON(t)}
	// 刷新时活动页新增了一个单集列表条目。
	responses = append(responses, `{"data":{"code":"forgenz","title":"00后的宇宙必听｜给正在长大的你","modules":[`+
		`{"type":"EPISODE_VERTICAL_LIST","episodes":[`+
		`{"id":"65e75676d15a20dbcab730ca","title":"一","podcastTitle":"播一"},`+
		`{"id":"6a0ae93ce1eb34a939a093d8","title":"二","podcastTitle":"播二"},`+
		`{"id":"695cdfd921cd486af739424c","title":"三","podcastTitle":"播三"},`+
		`{"id":"ccc111","title":"四","podcastTitle":"播四"}]}]}}`)
	call := 0
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		body := responses[call]
		call++
		return http.StatusOK, body
	}, nil))

	preview, err := service.Preview(context.Background(), activityURL)
	require.NoError(t, err)
	imported, err := service.ConfirmImport(preview.PreviewID)
	require.NoError(t, err)

	refresh, err := service.RefreshPreview(context.Background(), imported.CollectionID)
	require.NoError(t, err)
	assert.Equal(t, 1, refresh.Changes.AddedCount)
	assert.Equal(t, 4, refresh.ReadCount)
	assert.Len(t, refresh.Items, 4)
}
