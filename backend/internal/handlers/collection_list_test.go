package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type collectionListSummary struct {
	Covers       []string `json:"covers"`
	ID           uint     `json:"id"`
	Title        string   `json:"title"`
	Platform     string   `json:"platform"`
	ExternalID   string   `json:"external_id"`
	ItemCount    int64    `json:"item_count"`
	AdoptedCount int64    `json:"adopted_count"`
}

type collectionDetailItem struct {
	Position         int    `json:"position"`
	AdoptedEpisodeID *uint  `json:"adopted_episode_id"`
	EpisodeTitle     string `json:"episode_title"`
}

func listCollections(t *testing.T, router http.Handler, search string) []collectionListSummary {
	t.Helper()
	path := "/api/v1/collections"
	if search != "" {
		path += "?search=" + search
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Data []collectionListSummary `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	return body.Data
}

func getCollectionDetailCounts(t *testing.T, router http.Handler, id uint) (int64, int64, []collectionDetailItem) {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/collections/%d", id), nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Data struct {
			ItemCount    int64                  `json:"item_count"`
			AdoptedCount int64                  `json:"adopted_count"`
			Items        []collectionDetailItem `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	return body.Data.ItemCount, body.Data.AdoptedCount, body.Data.Items
}

// seedListScenario 构建覆盖收录判定规则的清单矩阵：命中规则（条目引用、外部
// 身份映射、链接、GUID+节目约束、跨清单共享身份）与排除规则（多候选歧义、
// 跨节目 GUID、软删除单集、软删除节目、空身份），外加空清单与软删除清单。
func seedListScenario(t *testing.T, db *gorm.DB) (matrix, shared, empty uint) {
	t.Helper()

	podcastA := models.Podcast{Title: "节目 A", FeedURL: "https://a.example.com/feed.xml", XYZID: "pod-a", IsSubscribed: true}
	podcastB := models.Podcast{Title: "节目 B", FeedURL: "https://b.example.com/feed.xml", XYZID: "pod-b", IsSubscribed: true}
	deletedPodcast := models.Podcast{Title: "已删节目", FeedURL: "https://sd.example.com/feed.xml", XYZID: "pod-sd", IsSubscribed: true}
	require.NoError(t, db.Create(&podcastA).Error)
	require.NoError(t, db.Create(&podcastB).Error)
	require.NoError(t, db.Create(&deletedPodcast).Error)

	newEpisode := func(podcast models.Podcast, suffix, link string) models.Episode {
		return models.Episode{PodcastID: podcast.ID, Title: "单集 " + suffix, GUID: "guid-" + suffix, Link: link}
	}
	epFK := newEpisode(podcastA, "fk", "https://www.xiaoyuzhoufm.com/episode/fk")
	epRef := newEpisode(podcastA, "ref", "https://www.xiaoyuzhoufm.com/episode/ref")
	epLink := newEpisode(podcastA, "link", "https://www.xiaoyuzhoufm.com/episode/link")
	epGuid := newEpisode(podcastA, "byguid", "https://www.xiaoyuzhoufm.com/episode/byguid")
	epShared := newEpisode(podcastA, "shared", "https://www.xiaoyuzhoufm.com/episode/shared")
	// 同一链接的两条本地单集：制造匹配不唯一。
	epAmbiguous1 := newEpisode(podcastA, "amb1", "https://www.xiaoyuzhoufm.com/episode/ambiguous")
	epAmbiguous2 := newEpisode(podcastA, "amb2", "https://www.xiaoyuzhoufm.com/episode/ambiguous")
	epGuidOtherPodcast := newEpisode(podcastB, "crossguid", "https://www.xiaoyuzhoufm.com/episode/crossguid")
	epSoftDeleted := newEpisode(podcastA, "softep", "https://www.xiaoyuzhoufm.com/episode/softep")
	epDeletedPodcastEpisode := newEpisode(deletedPodcast, "sdpod", "https://www.xiaoyuzhoufm.com/episode/sdpod")
	for _, ep := range []*models.Episode{&epFK, &epRef, &epLink, &epGuid, &epShared,
		&epAmbiguous1, &epAmbiguous2, &epGuidOtherPodcast, &epSoftDeleted, &epDeletedPodcastEpisode} {
		require.NoError(t, db.Create(ep).Error)
	}
	require.NoError(t, db.Delete(&epSoftDeleted).Error)  // 软删除单集
	require.NoError(t, db.Delete(&deletedPodcast).Error) // 软删除节目

	refRow := models.EpisodeExternalRef{SourcePlatform: "xiaoyuzhoufm", ExternalEpisodeID: "ref-eid",
		ExternalPodcastID: "pod-a", EpisodeID: epRef.ID, PodcastID: epRef.PodcastID}
	require.NoError(t, db.Create(&refRow).Error)

	matrixCollection := models.EpisodeCollection{SourcePlatform: "xiaoyuzhoufm", ExternalID: "matrix",
		Title: "矩阵清单", SourceURL: "https://www.xiaoyuzhoufm.com/collection/episode/matrix"}
	sharedCollection := models.EpisodeCollection{SourcePlatform: "xiaoyuzhoufm", ExternalID: "shared",
		Title: "共享清单", SourceURL: "https://www.xiaoyuzhoufm.com/collection/episode/shared"}
	emptyCollection := models.EpisodeCollection{SourcePlatform: "xiaoyuzhoufm", ExternalID: "empty",
		Title: "空清单", SourceURL: "https://www.xiaoyuzhoufm.com/collection/episode/empty"}
	deletedCollection := models.EpisodeCollection{SourcePlatform: "xiaoyuzhoufm", ExternalID: "deleted",
		Title: "已删清单", SourceURL: "https://www.xiaoyuzhoufm.com/collection/episode/deleted"}
	require.NoError(t, db.Create(&matrixCollection).Error)
	require.NoError(t, db.Create(&sharedCollection).Error)
	require.NoError(t, db.Create(&emptyCollection).Error)
	require.NoError(t, db.Create(&deletedCollection).Error)
	require.NoError(t, db.Delete(&deletedCollection).Error) // 软删除清单不出现在列表

	items := []models.EpisodeCollectionItem{
		// 命中：条目引用、外部身份映射、链接、GUID+节目约束、跨清单共享身份。
		{CollectionID: matrixCollection.ID, Position: 0, ExternalEpisodeID: "fk-eid", ExternalPodcastID: "pod-a",
			EpisodeTitle: "命中-条目引用", EpisodeID: &epFK.ID},
		{CollectionID: matrixCollection.ID, Position: 1, ExternalEpisodeID: "ref-eid", ExternalPodcastID: "pod-a",
			EpisodeTitle: "命中-外部映射"},
		{CollectionID: matrixCollection.ID, Position: 2, ExternalEpisodeID: "link-eid", ExternalPodcastID: "pod-a",
			EpisodeTitle: "命中-链接", EpisodeURL: epLink.Link},
		{CollectionID: matrixCollection.ID, Position: 3, ExternalEpisodeID: epGuid.GUID, ExternalPodcastID: "pod-a",
			EpisodeTitle: "命中-GUID"},
		{CollectionID: matrixCollection.ID, Position: 4, ExternalEpisodeID: "shared-eid", ExternalPodcastID: "pod-a",
			EpisodeTitle: "命中-共享", EpisodeURL: epShared.Link},
		// 不命中：匹配不唯一、跨节目 GUID、软删除单集、软删除节目下单集、空身份。
		{CollectionID: matrixCollection.ID, Position: 5, ExternalEpisodeID: "amb-eid", ExternalPodcastID: "pod-a",
			EpisodeTitle: "不命中-多候选", EpisodeURL: epAmbiguous1.Link},
		{CollectionID: matrixCollection.ID, Position: 6, ExternalEpisodeID: epGuidOtherPodcast.GUID, ExternalPodcastID: "pod-a",
			EpisodeTitle: "不命中-跨节目"},
		{CollectionID: matrixCollection.ID, Position: 7, ExternalEpisodeID: "softdel-eid", ExternalPodcastID: "pod-a",
			EpisodeTitle: "不命中-软删除单集", EpisodeURL: epSoftDeleted.Link},
		{CollectionID: matrixCollection.ID, Position: 8, ExternalEpisodeID: "sdpod-eid", ExternalPodcastID: "pod-sd",
			EpisodeTitle: "不命中-软删除节目", EpisodeURL: epDeletedPodcastEpisode.Link},
		{CollectionID: matrixCollection.ID, Position: 9, ExternalEpisodeID: "", ExternalPodcastID: "",
			EpisodeTitle: "不命中-空身份"},
		// 共享身份的第二份清单条目。
		{CollectionID: sharedCollection.ID, Position: 0, ExternalEpisodeID: "shared-eid", ExternalPodcastID: "pod-a",
			EpisodeTitle: "命中-共享", EpisodeURL: epShared.Link},
	}
	for i := range items {
		require.NoError(t, db.Create(&items[i]).Error)
	}
	return matrixCollection.ID, sharedCollection.ID, emptyCollection.ID
}

func TestCollectionHandler_ListAdoptionMatrixMatchesDetail(t *testing.T) {
	fetcher := &stubFetcher{status: http.StatusOK, body: "<html>unused</html>"}
	router, db := newCollectionTestEnv(t, fetcher)
	matrixID, _, _ := seedListScenario(t, db)

	var podcastCount, episodeCount int64
	require.NoError(t, db.Model(&models.Podcast{}).Count(&podcastCount).Error)
	require.NoError(t, db.Model(&models.Episode{}).Count(&episodeCount).Error)

	list := listCollections(t, router, "")
	require.Len(t, list, 3, "软删除清单不出现，其余全部返回")
	byTitle := map[string]collectionListSummary{}
	for _, summary := range list {
		byTitle[summary.Title] = summary
	}

	matrix := byTitle["矩阵清单"]
	assert.Equal(t, int64(10), matrix.ItemCount, "矩阵清单条目数")
	assert.Equal(t, int64(5), matrix.AdoptedCount, "五种命中规则各记一条，排除规则不误标")
	shared := byTitle["共享清单"]
	assert.Equal(t, int64(1), shared.ItemCount)
	assert.Equal(t, int64(1), shared.AdoptedCount, "同一身份在其他清单也正确识别")
	empty := byTitle["空清单"]
	assert.Equal(t, int64(0), empty.ItemCount)
	assert.Equal(t, int64(0), empty.AdoptedCount)
	assert.Empty(t, empty.Covers)

	// 列表与详情一致：条目数、收录数、被收录条目集合。
	itemCount, adoptedCount, detailItems := getCollectionDetailCounts(t, router, matrixID)
	assert.Equal(t, matrix.ItemCount, itemCount)
	assert.Equal(t, matrix.AdoptedCount, adoptedCount)
	adoptedPositions := map[int]bool{}
	for _, item := range detailItems {
		if item.AdoptedEpisodeID != nil {
			adoptedPositions[item.Position] = true
		}
	}
	assert.Equal(t, map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true}, adoptedPositions,
		"收录条目=引用/映射/链接/GUID/共享，歧义与排除规则不误标")

	// 列表读取只读：个人库与清单条目不被改动。
	again := listCollections(t, router, "")
	assert.Equal(t, list, again)
	var podcastAfter, episodeAfter int64
	require.NoError(t, db.Model(&models.Podcast{}).Count(&podcastAfter).Error)
	require.NoError(t, db.Model(&models.Episode{}).Count(&episodeAfter).Error)
	assert.Equal(t, podcastCount, podcastAfter)
	assert.Equal(t, episodeCount, episodeAfter)
	var fkItem, refItem models.EpisodeCollectionItem
	require.NoError(t, db.Where("collection_id = ? AND position = 0", matrixID).First(&fkItem).Error)
	assert.NotNil(t, fkItem.EpisodeID, "列表读取不改写条目引用")
	require.NoError(t, db.Where("collection_id = ? AND position = 1", matrixID).First(&refItem).Error)
	assert.Nil(t, refItem.EpisodeID, "列表读取不把映射结果写回条目")
}

func TestCollectionHandler_ListSearchOrderAndCovers(t *testing.T) {
	fetcher := &stubFetcher{status: http.StatusOK, body: "<html>unused</html>"}
	router, db := newCollectionTestEnv(t, fetcher)

	oldCollection := models.EpisodeCollection{SourcePlatform: "xiaoyuzhoufm", ExternalID: "old",
		Title: "早餐指南", SourceURL: "https://www.xiaoyuzhoufm.com/collection/episode/old"}
	midCollection := models.EpisodeCollection{SourcePlatform: "xiaoyuzhoufm", ExternalID: "mid",
		Title: "深夜电台", SourceURL: "https://www.xiaoyuzhoufm.com/collection/episode/mid"}
	newCollection := models.EpisodeCollection{SourcePlatform: "xiaoyuzhoufm", ExternalID: "new",
		Title: "半导体迷雾", SourceURL: "https://www.xiaoyuzhoufm.com/collection/episode/new"}
	require.NoError(t, db.Create(&oldCollection).Error)
	require.NoError(t, db.Create(&midCollection).Error)
	require.NoError(t, db.Create(&newCollection).Error)
	// 创建时间错开，验证列表按 created_at DESC 排序。
	require.NoError(t, db.Model(&oldCollection).Update("created_at", time.Now().AddDate(0, 0, -3)).Error)
	require.NoError(t, db.Model(&midCollection).Update("created_at", time.Now().AddDate(0, 0, -2)).Error)
	require.NoError(t, db.Model(&newCollection).Update("created_at", time.Now().AddDate(0, 0, -1)).Error)

	// 封面选择：前四条目按原顺序过滤空封面；第五条的封面不参与（不是前四个非空）。
	coverItems := []models.EpisodeCollectionItem{
		{CollectionID: oldCollection.ID, Position: 0, ExternalEpisodeID: "c0", EpisodeTitle: "无封面"},
		{CollectionID: oldCollection.ID, Position: 1, ExternalEpisodeID: "c1", EpisodeTitle: "图一", ImageURL: "https://img/1.jpg"},
		{CollectionID: oldCollection.ID, Position: 2, ExternalEpisodeID: "c2", EpisodeTitle: "图二", PodcastCoverURL: "https://img/2.jpg"},
		{CollectionID: oldCollection.ID, Position: 3, ExternalEpisodeID: "c3", EpisodeTitle: "图三", ImageURL: "https://img/3.jpg"},
		{CollectionID: oldCollection.ID, Position: 4, ExternalEpisodeID: "c4", EpisodeTitle: "图四", ImageURL: "https://img/4.jpg"},
	}
	for i := range coverItems {
		require.NoError(t, db.Create(&coverItems[i]).Error)
	}

	all := listCollections(t, router, "")
	require.Len(t, all, 3)
	assert.Equal(t, []string{"半导体迷雾", "深夜电台", "早餐指南"}, []string{all[0].Title, all[1].Title, all[2].Title},
		"按创建时间倒序")

	hit := listCollections(t, router, "%E6%B7%B1%E5%A4%9C")
	require.Len(t, hit, 1)
	assert.Equal(t, "深夜电台", hit[0].Title)

	miss := listCollections(t, router, "%E4%B8%8D%E5%AD%98%E5%9C%A8")
	assert.Empty(t, miss)

	cleared := listCollections(t, router, "")
	assert.Len(t, cleared, 3, "清空搜索恢复完整列表")

	var breakfast collectionListSummary
	for _, summary := range all {
		if summary.Title == "早餐指南" {
			breakfast = summary
		}
	}
	assert.Equal(t, []string{"https://img/1.jpg", "https://img/2.jpg", "https://img/3.jpg"}, breakfast.Covers,
		"封面取前四条目再过滤空值，不改成前四个非空封面")
}

func TestCollectionHandler_ListIdentityBatchBoundary(t *testing.T) {
	fetcher := &stubFetcher{status: http.StatusOK, body: "<html>unused</html>"}
	router, db := newCollectionTestEnv(t, fetcher)

	podcast := models.Podcast{Title: "边界节目", FeedURL: "https://batch.example.com/feed.xml", XYZID: "pod-batch", IsSubscribed: true}
	require.NoError(t, db.Create(&podcast).Error)
	epFK := models.Episode{PodcastID: podcast.ID, Title: "边界引用", GUID: "guid-batch-fk", Link: "https://www.xiaoyuzhoufm.com/episode/batchfk"}
	epLinkLow := models.Episode{PodcastID: podcast.ID, Title: "边界链接低", GUID: "guid-batch-ll", Link: "https://www.xiaoyuzhoufm.com/episode/batchll"}
	epLinkHigh := models.Episode{PodcastID: podcast.ID, Title: "边界链接高", GUID: "guid-batch-lh", Link: "https://www.xiaoyuzhoufm.com/episode/batchlh"}
	epRefLow := models.Episode{PodcastID: podcast.ID, Title: "边界映射低", GUID: "guid-batch-rl", Link: "https://www.xiaoyuzhoufm.com/episode/batchrl"}
	epRefHigh := models.Episode{PodcastID: podcast.ID, Title: "边界映射高", GUID: "guid-batch-rh", Link: "https://www.xiaoyuzhoufm.com/episode/batchrh"}
	for _, ep := range []*models.Episode{&epFK, &epLinkLow, &epLinkHigh, &epRefLow, &epRefHigh} {
		require.NoError(t, db.Create(ep).Error)
	}
	for _, ref := range []models.EpisodeExternalRef{
		{SourcePlatform: "xiaoyuzhoufm", ExternalEpisodeID: "ref-low", ExternalPodcastID: "pod-batch", EpisodeID: epRefLow.ID, PodcastID: epRefLow.PodcastID},
		{SourcePlatform: "xiaoyuzhoufm", ExternalEpisodeID: "ref-high", ExternalPodcastID: "pod-batch", EpisodeID: epRefHigh.ID, PodcastID: epRefHigh.PodcastID},
	} {
		require.NoError(t, db.Create(&ref).Error)
	}

	collection := models.EpisodeCollection{SourcePlatform: "xiaoyuzhoufm", ExternalID: "batch",
		Title: "分批清单", SourceURL: "https://www.xiaoyuzhoufm.com/collection/episode/batch"}
	require.NoError(t, db.Create(&collection).Error)

	// 520 个条目跨越单批 500 的边界：命中条目分置边界两侧。
	totalItems := 520
	items := make([]models.EpisodeCollectionItem, 0, totalItems)
	for position := 0; position < totalItems; position++ {
		item := models.EpisodeCollectionItem{CollectionID: collection.ID, Position: position,
			ExternalEpisodeID: fmt.Sprintf("batch-%04d", position), ExternalPodcastID: "pod-batch",
			EpisodeTitle: fmt.Sprintf("边界条目 %04d", position),
			EpisodeURL:   fmt.Sprintf("https://www.xiaoyuzhoufm.com/episode/batchitem%04d", position)}
		switch position {
		case 250:
			item.EpisodeID = &epFK.ID
		case 499:
			item.ExternalEpisodeID = "ref-low"
		case 510:
			item.ExternalEpisodeID = "ref-high"
		case 0:
			item.EpisodeURL = epLinkLow.Link
		case 516:
			// 引用和链接的不同候选分处两个查询批次，仍须判为歧义。
			item.EpisodeID = &epFK.ID
			item.EpisodeURL = epLinkHigh.Link
		case 518:
			// 同一候选分别由首批 ID 和后批链接取回，不应误判为歧义。
			item.EpisodeID = &epLinkHigh.ID
			item.EpisodeURL = epLinkHigh.Link
		case 519:
			item.EpisodeURL = epLinkHigh.Link
		}
		items = append(items, item)
	}
	require.NoError(t, db.CreateInBatches(&items, 100).Error)

	list := listCollections(t, router, "")
	require.Len(t, list, 1)
	assert.Equal(t, int64(totalItems), list[0].ItemCount)
	assert.Equal(t, int64(6), list[0].AdoptedCount, "分批边界两侧的引用/映射/链接命中全部识别")

	_, adoptedCount, detailItems := getCollectionDetailCounts(t, router, collection.ID)
	assert.Equal(t, int64(6), adoptedCount)
	adopted := map[int]bool{}
	for _, item := range detailItems {
		if item.AdoptedEpisodeID != nil {
			adopted[item.Position] = true
		}
	}
	assert.Equal(t, map[int]bool{0: true, 250: true, 499: true, 510: true, 518: true, 519: true}, adopted)
}

func TestCollectionHandler_ListEmptySet(t *testing.T) {
	fetcher := &stubFetcher{status: http.StatusOK, body: "<html>unused</html>"}
	router, _ := newCollectionTestEnv(t, fetcher)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/collections", nil))
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"data":[]`, "空清单集合返回空数组而不是 null")
}
