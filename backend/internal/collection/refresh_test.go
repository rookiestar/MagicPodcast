package collection

import (
	"context"
	"net/http"
	"testing"
	"time"

	"magicpodcast/internal/models"
	"magicpodcast/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// modifiedSampleHTML 在固定样例基础上模拟源清单编辑：移出第 8 集、新增一集、
// 调整第 2 集推荐语、交换前两集顺序。
func modifiedSampleHTML(t *testing.T) string {
	t.Helper()
	draft, err := ParsePageHTML(loadSampleHTML(t), sampleCollectionID)
	require.NoError(t, err)

	// 移出最后一集
	draft.Items = draft.Items[:7]
	// 交换前两集顺序
	draft.Items[0], draft.Items[1] = draft.Items[1], draft.Items[0]
	// 修改新的第一条（原第 2 集）推荐语
	draft.Items[0].Recommendation = "更新后的推荐语"
	// 新增一集
	draft.Items = append(draft.Items, ItemDraft{
		ExternalEpisodeID: "aaaaaaaaaaaaaaaaaaaaaaaa",
		ExternalPodcastID: "pnew",
		PodcastTitle:      "新节目",
		EpisodeTitle:      "新增单集",
		Recommendation:    "新增推荐语",
		EpisodeURL:        EpisodeURLForEID("aaaaaaaaaaaaaaaaaaaaaaaa"),
		PayType:           PayTypeFree,
	})
	return nextDataHTMLFromDraft(t, draft)
}

func nextDataHTMLFromDraft(t *testing.T, draft *Draft) string {
	t.Helper()
	target := make([]map[string]any, 0, len(draft.Items))
	for _, item := range draft.Items {
		target = append(target, map[string]any{
			"eid": item.ExternalEpisodeID, "pid": item.ExternalPodcastID,
			"title": item.EpisodeTitle, "recommendation": item.Recommendation,
			"podcast": map[string]any{"pid": item.ExternalPodcastID, "title": item.PodcastTitle},
			"payType": item.PayType,
		})
	}
	payload := map[string]any{
		"id": draft.ExternalID, "title": draft.Title, "description": draft.Description,
		"targetType": "EPISODE", "target": target,
		"author": map[string]any{"nickname": draft.Author},
	}
	return nextDataHTML(t, payload)
}

func seedAdoptedFirstItem(t *testing.T, service *Service) (collectionID uint, itemID uint) {
	t.Helper()
	imported, err := service.ConfirmImport(samplePreview(t, service).PreviewID)
	require.NoError(t, err)
	detail, err := service.GetCollection(imported.CollectionID)
	require.NoError(t, err)
	return imported.CollectionID, detail.Items[0].ID
}

func TestRefreshPreview_ShowsAddedRemovedReorderedAndRecommendationChanges(t *testing.T) {
	calls := 0
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		calls++
		if calls == 1 {
			return http.StatusOK, loadSampleHTML(t)
		}
		return http.StatusOK, modifiedSampleHTML(t)
	}, nil))
	collectionID, itemID := seedAdoptedFirstItem(t, service)

	// 先收录第 1 集（原始顺序第一条）。
	adopted, err := serviceAdopt(t, service, collectionID, itemID)
	require.NoError(t, err)

	preview, err := service.RefreshPreview(context.Background(), collectionID)
	require.NoError(t, err)
	changes := preview.Changes
	assert.Equal(t, 1, changes.AddedCount, "新增 1 集")
	assert.Equal(t, 1, changes.RemovedCount, "移出 1 集")
	assert.Equal(t, 2, changes.ReorderedCount, "前两集顺序互换")
	assert.Equal(t, 1, changes.RecommendationChanged, "1 条推荐语变化")
	require.Len(t, preview.Removed, 1)
	assert.Equal(t, sampleEIDs[7], preview.Removed[0].ExternalEpisodeID)
	assert.False(t, preview.Removed[0].Adopted)

	// 确认应用实际看过的版本。
	applied, err := service.ApplyRefresh(collectionID, preview.PreviewID, preview.BaseRev)
	require.NoError(t, err)
	assert.True(t, applied.Applied)
	assert.False(t, applied.NoChanges)
	assert.Equal(t, 8, applied.ItemCount, "移出 1 新增 1，总数不变")
	assert.Equal(t, 1, applied.AdoptedKeptCount, "已收录条目的关联保留")
	assert.Equal(t, 0, applied.RemovedAdopted, "被移出条目未被收录")

	// 已收录单集的关联与个人状态在刷新后保持。
	detail, err := service.GetCollection(collectionID)
	require.NoError(t, err)
	var kept *CollectionItemDetail
	for index := range detail.Items {
		if detail.Items[index].ExternalEpisodeID == sampleEIDs[0] {
			kept = &detail.Items[index]
		}
	}
	require.NotNil(t, kept)
	require.NotNil(t, kept.AdoptedEpisodeID)
	assert.Equal(t, adopted.EpisodeID, *kept.AdoptedEpisodeID, "已收录单集关联保留")

	// 新增条目保持未采纳。
	for _, item := range detail.Items {
		if item.ExternalEpisodeID == "aaaaaaaaaaaaaaaaaaaaaaaa" {
			assert.Nil(t, item.AdoptedEpisodeID, "刷新新增条目不自动采纳")
		}
	}
}

func TestRefreshPreview_NoChangesOnlyRecordsCheck(t *testing.T) {
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		return http.StatusOK, loadSampleHTML(t)
	}, nil))
	collectionID, _ := seedAdoptedFirstItem(t, service)

	preview, err := service.RefreshPreview(context.Background(), collectionID)
	require.NoError(t, err)
	assert.False(t, preview.Changes.HasChanges())

	applied, err := service.ApplyRefresh(collectionID, preview.PreviewID, preview.BaseRev)
	require.NoError(t, err)
	assert.True(t, applied.NoChanges)
	assert.Equal(t, preview.BaseRev, applied.Revision, "无变化不递增修订")
}

func TestApplyRefresh_RejectsStaleRevision(t *testing.T) {
	calls := 0
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		calls++
		if calls <= 1 {
			return http.StatusOK, loadSampleHTML(t)
		}
		return http.StatusOK, modifiedSampleHTML(t)
	}, nil))
	collectionID, _ := seedAdoptedFirstItem(t, service)

	first, err := service.RefreshPreview(context.Background(), collectionID)
	require.NoError(t, err)
	// 另一个页面先应用了一次刷新（修订+1）。
	second, err := service.RefreshPreview(context.Background(), collectionID)
	require.NoError(t, err)
	_, err = service.ApplyRefresh(collectionID, second.PreviewID, second.BaseRev)
	require.NoError(t, err)

	// 第一个页面的过期预览再提交：修订冲突，不覆盖较新结果。
	_, err = service.ApplyRefresh(collectionID, first.PreviewID, first.BaseRev)
	require.ErrorIs(t, err, ErrRefreshConflict)

	detail, err := service.GetCollection(collectionID)
	require.NoError(t, err)
	assert.Equal(t, second.BaseRev+1, detail.Revision)
}

func TestDeleteCollection_KeepsAdoptedEpisodesAndSources(t *testing.T) {
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		return http.StatusOK, loadSampleHTML(t)
	}, nil))
	collectionID, itemID := seedAdoptedFirstItem(t, service)
	adopted, err := serviceAdopt(t, service, collectionID, itemID)
	require.NoError(t, err)

	// 收录后用户添加的个人笔记属于 Episode 账本，删除清单不得影响。
	require.NoError(t, service.db.Model(&models.Episode{}).Where("id = ?", adopted.EpisodeID).
		Update("notes", "我的个人笔记").Error)

	result, err := service.DeleteCollection(collectionID)
	require.NoError(t, err)
	assert.True(t, result.Deleted)
	assert.Equal(t, 8, result.ItemCount)
	assert.Equal(t, 1, result.AdoptedDetached)

	// 清单与条目已删除。
	_, err = service.GetCollection(collectionID)
	require.ErrorIs(t, err, ErrCollectionNotFound)

	// 已收录单集、队列、笔记与采纳来源摘要全部保留。
	var episode models.Episode
	require.NoError(t, service.db.First(&episode, adopted.EpisodeID).Error)
	assert.Equal(t, "我的个人笔记", episode.Notes)
	var decision models.EpisodeTriageDecision
	require.NoError(t, service.db.Where("episode_id = ?", adopted.EpisodeID).
		First(&decision).Error)
	require.NotNil(t, decision.QueueState)
	assert.Equal(t, models.QueueStateInbox, *decision.QueueState)
	var adoptions []models.EpisodeCollectionAdoption
	require.NoError(t, service.db.Where("episode_id = ?", adopted.EpisodeID).
		Find(&adoptions).Error)
	require.Len(t, adoptions, 1, "采纳来源摘要保留")
	assert.Equal(t, sampleTitle, adoptions[0].CollectionTitle)
	assert.NotEmpty(t, adoptions[0].CollectionURL, "来源仍可解释并打开源站")

	// 清单已删除后重复删除返回不存在。
	_, err = service.DeleteCollection(collectionID)
	require.ErrorIs(t, err, ErrCollectionNotFound)
}

// serviceAdopt 通过服务的事务收录路径收录一条条目（测试辅助）。
func serviceAdopt(t *testing.T, service *Service, collectionID, itemID uint) (*services.AdoptResult, error) {
	t.Helper()
	return services.NewCollectionAdoptionService(service.db).
		AdoptCollectionItem(collectionID, itemID)
}

func TestRefreshPreview_FailureKeepsLastGoodCollection(t *testing.T) {
	var failNext bool
	service := newStubService(t, stubFetcher(func(string) (int, string) {
		if failNext {
			return http.StatusForbidden, ""
		}
		return http.StatusOK, loadSampleHTML(t)
	}, nil))
	collectionID, _ := seedAdoptedFirstItem(t, service)

	failNext = true
	_, err := service.RefreshPreview(context.Background(), collectionID)
	require.ErrorIs(t, err, ErrSourceForbidden)

	// 失败保留上一份成功清单。
	detail, err := service.GetCollection(collectionID)
	require.NoError(t, err)
	assert.Len(t, detail.Items, 8)
	assert.Equal(t, 1, detail.Revision)
	assert.False(t, detail.LastRefreshedAt == nil || detail.LastRefreshedAt.After(time.Now().Add(time.Minute)))
}

func TestReviewImportPersistsAudioForAdoption(t *testing.T) {
	s := newStubService(t, stubFetcher(func(string) (int, string) { return http.StatusOK, loadSampleHTML(t) }, nil))
	id, itemID := seedAdoptedFirstItem(t, s)
	draft, err := ParsePageHTML(loadSampleHTML(t), sampleCollectionID)
	require.NoError(t, err)
	require.NotEmpty(t, draft.Items[0].AudioURL, "fixture must carry a public audio source")
	adopted, err := serviceAdopt(t, s, id, itemID)
	require.NoError(t, err)
	var e models.Episode
	require.NoError(t, s.db.First(&e, adopted.EpisodeID).Error)
	assert.Equal(t, draft.Items[0].AudioURL, e.MediumURL)
	assert.Equal(t, draft.Items[0].AudioMimeType, e.EnclosureType)
	assert.Equal(t, draft.Items[0].AudioSize, e.EnclosureLength)
}

func TestReviewRefreshKeepsItemAddressAndUpdatesMetadata(t *testing.T) {
	s := newStubService(t, stubFetcher(func(string) (int, string) { return http.StatusOK, loadSampleHTML(t) }, nil))
	id, itemID := seedAdoptedFirstItem(t, s)
	draft, err := ParsePageHTML(loadSampleHTML(t), sampleCollectionID)
	require.NoError(t, err)
	draft.Title = "新主题"
	draft.Items[0].Recommendation = "新推荐语"
	s.fetch = stubFetcher(func(string) (int, string) { return http.StatusOK, nextDataHTMLFromDraft(t, draft) }, nil)
	p, err := s.RefreshPreview(context.Background(), id)
	require.NoError(t, err)
	_, err = s.ApplyRefresh(id, p.PreviewID, p.BaseRev)
	require.NoError(t, err)
	d, err := s.GetCollection(id)
	require.NoError(t, err)
	assert.Equal(t, itemID, d.Items[0].ID, "刷新不能让仍在清单内的条目地址失效")
	draft.Title = "只有标题变化"
	p, err = s.RefreshPreview(context.Background(), id)
	require.NoError(t, err)
	_, err = s.ApplyRefresh(id, p.PreviewID, p.BaseRev)
	require.NoError(t, err)
	d, err = s.GetCollection(id)
	require.NoError(t, err)
	assert.Equal(t, draft.Title, d.Title)
}

func TestReviewReadbackUsesSharedLibraryAndIgnoresDeletedEpisode(t *testing.T) {
	s := newStubService(t, stubFetcher(func(string) (int, string) { return http.StatusOK, loadSampleHTML(t) }, nil))
	id, itemID := seedAdoptedFirstItem(t, s)
	adopted, err := serviceAdopt(t, s, id, itemID)
	require.NoError(t, err)
	other := models.EpisodeCollection{SourcePlatform: PlatformXiaoyuzhoufm, ExternalID: "other", Title: "Other", SourceURL: "https://www.xiaoyuzhoufm.com/collection/episode/bbbbbbbbbbbbbbbbbbbbbbbb", Revision: 1}
	require.NoError(t, s.db.Create(&other).Error)
	var original models.EpisodeCollectionItem
	require.NoError(t, s.db.First(&original, itemID).Error)
	original.BaseModel = models.BaseModel{}
	original.CollectionID = other.ID
	original.EpisodeID = nil
	require.NoError(t, s.db.Create(&original).Error)
	detail, err := s.GetCollection(other.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), detail.AdoptedCount)
	require.NotNil(t, detail.Items[0].AdoptedEpisodeID)
	assert.Equal(t, adopted.EpisodeID, *detail.Items[0].AdoptedEpisodeID)
	draft, err := ParsePageHTML(loadSampleHTML(t), sampleCollectionID)
	require.NoError(t, err)
	draft.ExternalID = other.ExternalID
	draft.Items = nil
	s.fetch = stubFetcher(func(string) (int, string) { return http.StatusOK, nextDataHTMLFromDraft(t, draft) }, nil)
	preview, err := s.RefreshPreview(context.Background(), other.ID)
	require.NoError(t, err)
	require.Len(t, preview.Removed, 1)
	assert.True(t, preview.Removed[0].Adopted)
	require.NoError(t, s.db.Delete(&models.Episode{}, adopted.EpisodeID).Error)
	for _, cid := range []uint{id, other.ID} {
		detail, err = s.GetCollection(cid)
		require.NoError(t, err)
		assert.Zero(t, detail.AdoptedCount)
		assert.Nil(t, detail.Items[0].AdoptedEpisodeID)
	}
}

func TestReviewRefreshEmptySourceRetainsPersonalEpisode(t *testing.T) {
	s := newStubService(t, stubFetcher(func(string) (int, string) { return http.StatusOK, loadSampleHTML(t) }, nil))
	id, itemID := seedAdoptedFirstItem(t, s)
	adopted, err := serviceAdopt(t, s, id, itemID)
	require.NoError(t, err)
	draft, err := ParsePageHTML(loadSampleHTML(t), sampleCollectionID)
	require.NoError(t, err)
	draft.Items = nil
	s.fetch = stubFetcher(func(string) (int, string) { return http.StatusOK, nextDataHTMLFromDraft(t, draft) }, nil)
	preview, err := s.RefreshPreview(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, 8, preview.Changes.RemovedCount)
	_, err = s.ApplyRefresh(id, preview.PreviewID, preview.BaseRev)
	require.NoError(t, err)
	detail, err := s.GetCollection(id)
	require.NoError(t, err)
	assert.Empty(t, detail.Items)
	var episode models.Episode
	require.NoError(t, s.db.First(&episode, adopted.EpisodeID).Error)
}
