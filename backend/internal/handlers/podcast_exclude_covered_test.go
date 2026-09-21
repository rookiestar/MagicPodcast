package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/database"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupExcludeCoveredRouter 构建隔离库上的节目列表路由。
func setupExcludeCoveredRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:exclude_covered_test_"+strconv.FormatInt(time.Now().UnixNano(), 10)+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Tag{}, &models.Workflow{}, &models.PodcastHistorySyncTask{}))
	database.SetTestDB(db)
	t.Cleanup(database.ResetDB)
	cache.GetCache().Clear()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandlerMiddleware())
	handler := NewPodcastHandler()
	router.GET("/api/v1/podcasts", handler.List)
	return router, db
}

func seedCoveredPodcast(t *testing.T, db *gorm.DB, id int, title string, subscribed bool) models.Podcast {
	t.Helper()
	podcast := models.Podcast{
		XYZID:   "xc-" + strconv.Itoa(id),
		Title:   title,
		FeedURL: "http://f/xc-" + strconv.Itoa(id) + ".xml",
	}
	podcast.ID = uint(id)
	require.NoError(t, db.Create(&podcast).Error)
	require.NoError(t, db.Model(&models.Podcast{}).Where("id = ?", id).
		Update("is_subscribed", subscribed).Error)
	return podcast
}

func listPodcastTitles(t *testing.T, router *gin.Engine, query string) ([]string, int64) {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/podcasts"+query, nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var payload struct {
		Data []struct {
			Title string `json:"title"`
		} `json:"data"`
		Pagination struct {
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	titles := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		titles = append(titles, item.Title)
	}
	return titles, payload.Pagination.Total
}

// TestPodcastListExcludeCoveredAppliesBeforePagination 验证覆盖筛选在分页前
// 与搜索、标签组合，候选与总数一致（#419 AC8）；停用计入、已删除不计入。
func TestPodcastListExcludeCoveredAppliesBeforePagination(t *testing.T) {
	router, db := setupExcludeCoveredRouter(t)

	p1 := seedCoveredPodcast(t, db, 1, "阿尔法", true) // 指定成员覆盖
	p2 := seedCoveredPodcast(t, db, 2, "贝塔", false) // 未订阅，仅被已删除工作流引用：不覆盖
	p3 := seedCoveredPodcast(t, db, 3, "伽马", false) // 自定义源地址覆盖
	p4 := seedCoveredPodcast(t, db, 4, "德尔塔", true) // 停用工作流成员：仍覆盖
	seedCoveredPodcast(t, db, 5, "艾普西隆", false)
	p6 := seedCoveredPodcast(t, db, 6, "泽塔", false)
	seedCoveredPodcast(t, db, 7, "伊塔", true) // 全部订阅覆盖

	specific := models.Workflow{Name: "指定", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(p1.ID)}}}
	require.NoError(t, db.Create(&specific).Error)
	allSub := models.Workflow{Name: "全订阅", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeAllSubscribed}
	require.NoError(t, db.Create(&allSub).Error)
	custom := models.Workflow{Name: "自定义", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeCustomSources,
		ScopeConfig: models.ScopeConfig{CustomURLs: []string{p3.FeedURL}}}
	require.NoError(t, db.Create(&custom).Error)
	disabled := models.Workflow{Name: "停用", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(p4.ID)}}}
	require.NoError(t, db.Create(&disabled).Error)
	deleted := models.Workflow{Name: "已删除", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeSpecificPodcasts,
		ScopeConfig: models.ScopeConfig{PodcastIDs: []int{int(p2.ID)}}}
	require.NoError(t, db.Create(&deleted).Error)
	require.NoError(t, db.Delete(&models.Workflow{}, deleted.ID).Error)

	// 不带筛选：全部 7 条。
	titles, total := listPodcastTitles(t, router, "?exclude_covered=0")
	assert.Len(t, titles, 7)
	assert.Equal(t, int64(7), total)

	// 覆盖筛选：只余未覆盖的 p2/p5/p6。
	titles, total = listPodcastTitles(t, router, "?exclude_covered=1")
	assert.ElementsMatch(t, []string{"贝塔", "艾普西隆", "泽塔"}, titles)
	assert.Equal(t, int64(3), total)

	// 分页一致：page_size=2 两页取全。缺失日期回退创建时间倒序 + id 兜底，
	// 同批种子按稳定顺序返回：泽塔(6) → 艾普西隆(5) → 贝塔(2)（#463）。
	page1, total := listPodcastTitles(t, router, "?exclude_covered=1&page=1&page_size=2")
	page2, _ := listPodcastTitles(t, router, "?exclude_covered=1&page=2&page_size=2")
	assert.Len(t, page1, 2)
	assert.Len(t, page2, 1)
	assert.Equal(t, int64(3), total)
	assert.NotEmpty(t, page1)
	assert.Equal(t, append(page1, page2...), []string{"泽塔", "艾普西隆", "贝塔"})

	// 与搜索组合。
	titles, total = listPodcastTitles(t, router, "?exclude_covered=1&search=艾普西隆")
	assert.Equal(t, []string{"艾普西隆"}, titles)
	assert.Equal(t, int64(1), total)

	// 与标签组合：只给泽塔打标签。
	tag := models.Tag{Name: "覆盖筛选标签"}
	require.NoError(t, db.Create(&tag).Error)
	var p6Record models.Podcast
	require.NoError(t, db.First(&p6Record, p6.ID).Error)
	require.NoError(t, db.Model(&p6Record).Association("Tags").Append(&tag))
	var joinCount int64
	require.NoError(t, db.Table("podcasts_tags").Where("podcast_id = ?", p6.ID).Count(&joinCount).Error)
	require.Equal(t, int64(1), joinCount, "标签关联必须已写入")
	titles, total = listPodcastTitles(t, router, "?exclude_covered=1&tag_id="+strconv.FormatUint(uint64(tag.ID), 10))
	assert.Equal(t, []string{"泽塔"}, titles)
	assert.Equal(t, int64(1), total)
}

func TestCoverageRefreshAfterAppendOverHTTP(t *testing.T) {
	router, db := setupExcludeCoveredRouter(t)
	p := seedCoveredPodcast(t, db, 1, "New", true)
	wf := models.Workflow{Name: "Existing", Schedule: "0 6 * * *", ScopeType: models.ScopeTypeSpecificPodcasts, ScopeConfig: models.ScopeConfig{CustomURLs: []string{"https://keep/feed"}}}
	require.NoError(t, db.Create(&wf).Error)
	router.POST("/workflows/:id/podcasts/append", (&WorkflowHandler{}).AppendPodcasts)
	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()
	readTotal := func() int {
		response, err := client.Get(server.URL + "/api/v1/podcasts?exclude_covered=1&view=summary")
		require.NoError(t, err)
		defer response.Body.Close()
		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "private, no-store", response.Header.Get("Cache-Control"))
		var payload struct {
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		}
		require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
		return payload.Pagination.Total
	}
	require.Equal(t, 1, readTotal())
	require.Equal(t, 1, readTotal(), "cached server response must also forbid browser caching")
	response, err := client.Post(server.URL+"/workflows/"+strconv.Itoa(int(wf.ID))+"/podcasts/append", "application/json", strings.NewReader(`{"podcast_ids":[1]}`))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	response.Body.Close()
	require.Equal(t, 0, readTotal(), "append invalidates the coverage result")
	require.NoError(t, db.First(&wf, wf.ID).Error)
	assert.Equal(t, []int{int(p.ID)}, wf.ScopeConfig.PodcastIDs)
	assert.Equal(t, []string{"https://keep/feed"}, wf.ScopeConfig.CustomURLs)
}
