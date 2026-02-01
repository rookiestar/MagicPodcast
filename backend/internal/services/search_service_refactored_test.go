package services

import (
	"testing"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestSearchServiceRefactored(t *testing.T) {
	t.Skip("SearchService requires full database and config initialization. Run as integration test instead.")

	// 配置和数据库已通过其他方式初始化
	// 创建搜索服务
	searchService := NewSearchService()

	t.Run("Search_AllTypes", func(t *testing.T) {
		req := SearchRequest{
			Query:           "科技",
			Type:            "all",
			Page:            1,
			PageSize:        10,
			EpisodePage:     1,
			EpisodePageSize: 10,
		}

		resp, err := searchService.Search(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Pagination)
	})

	t.Run("Search_OnlyPodcasts", func(t *testing.T) {
		req := SearchRequest{
			Query:           "科技",
			Type:            "podcasts",
			Page:            1,
			PageSize:        10,
			EpisodePage:     1,
			EpisodePageSize: 10,
		}

		resp, err := searchService.Search(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Pagination)
		assert.Empty(t, resp.Episodes) // 只搜索播客
	})

	t.Run("Search_OnlyEpisodes", func(t *testing.T) {
		req := SearchRequest{
			Query:           "科技",
			Type:            "episodes",
			Page:            1,
			PageSize:        10,
			EpisodePage:     1,
			EpisodePageSize: 10,
		}

		resp, err := searchService.Search(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Pagination)
		assert.Empty(t, resp.Podcasts) // 只搜索单集
	})

	t.Run("Search_WithTags", func(t *testing.T) {
		// 首先获取一些标签
		db := database.GetDB()
		var tags []models.Tag
		if err := db.Limit(2).Find(&tags).Error; err != nil {
			t.Skip("需要数据库中有标签")
		}

		if len(tags) < 2 {
			t.Skip("需要至少2个标签进行测试")
		}

		tagIDs := []uint{tags[0].ID, tags[1].ID}

		req := SearchRequest{
			Query:           "",
			Type:            "all",
			TagIDs:          tagIDs,
			Page:            1,
			PageSize:        10,
			EpisodePage:     1,
			EpisodePageSize: 10,
		}

		resp, err := searchService.Search(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotNil(t, resp.Pagination)
	})
}

// 测试文本处理函数
func TestTextProcessing(t *testing.T) {
	t.Run("isPureNumber", func(t *testing.T) {
		assert.True(t, isPureNumber("123"))
		assert.True(t, isPureNumber("42"))
		assert.False(t, isPureNumber("12a3"))
		assert.False(t, isPureNumber(""))
		assert.False(t, isPureNumber("abc"))
	})

	t.Run("isStandaloneNumber", func(t *testing.T) {
		assert.True(t, isStandaloneNumber("hello 123 world", "123"))
		assert.True(t, isStandaloneNumber("123 world", "123"))
		assert.True(t, isStandaloneNumber("hello 123", "123"))
		assert.False(t, isStandaloneNumber("hello123world", "123"))
	})

	t.Run("stripHTML", func(t *testing.T) {
		assert.Equal(t, "hello world", stripHTML("<p>hello world</p>"))
		assert.Equal(t, "hello world", stripHTML("<div>hello</div> <span>world</span>"))
		assert.Equal(t, "hello", stripHTML("hello"))
	})

	t.Run("generateSnippet", func(t *testing.T) {
		longText := "This is a very long text that should be truncated to 150 characters when the snippet is generated. This allows us to show only the relevant part of the content to the user."

		snippet := generateSnippet(longText, "long")
		assert.True(t, len(snippet) <= 153) // 150 + "..."
		assert.Contains(t, snippet, "long")
	})
}

// 测试相关性计算
func TestRelevanceCalculation(t *testing.T) {
	t.Skip("需要配置初始化，在实际运行环境中测试")

	cfg := config.Get().Search

	t.Run("calculatePodcastRelevance", func(t *testing.T) {
		// 完全匹配
		score1 := calculatePodcastRelevance("科技播客", "作者", "描述", "科技播客", cfg)
		score2 := calculatePodcastRelevance("科技播客", "作者", "描述", "科技", cfg)
		assert.Greater(t, score1, score2)

		// 前缀匹配
		score3 := calculatePodcastRelevance("科技播客", "作者", "描述", "科技", cfg)
		score4 := calculatePodcastRelevance("这是科技播客", "作者", "描述", "科技", cfg)
		assert.Greater(t, score3, score4)
	})

	t.Run("calculateEpisodeRelevance", func(t *testing.T) {
		// 完全匹配
		score1 := calculateEpisodeRelevance("科技节目", "内容", "科技节目", cfg)
		score2 := calculateEpisodeRelevance("科技节目", "内容", "科技", cfg)
		assert.Greater(t, score1, score2)
	})
}

// 测试分页构建
func TestPaginationBuilder(t *testing.T) {
	t.Run("buildPaginationInfo", func(t *testing.T) {
		info := buildPaginationInfo(100, 1, 10)
		assert.Equal(t, 1, info.Page)
		assert.Equal(t, 10, info.PageSize)
		assert.Equal(t, 100, info.Total)
		assert.Equal(t, 10, info.TotalPages)

		info2 := buildPaginationInfo(95, 1, 10)
		assert.Equal(t, 10, info2.TotalPages) // 95/10 = 9, 余5 -> 10页
	})
}

// 基准测试 - 验证性能
func BenchmarkSearchService(b *testing.B) {
	searchService := NewSearchService()
	req := SearchRequest{
		Query:           "科技",
		Type:            "all",
		Page:            1,
		PageSize:        20,
		EpisodePage:     1,
		EpisodePageSize: 20,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = searchService.Search(req)
	}
}

func BenchmarkTextProcessing(b *testing.B) {
	keyword := "科技"
	text := "这是一个关于科技的播客节目，我们讨论最新的科技趋势和科技新闻。"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generateSnippet(text, keyword)
	}
}

func BenchmarkRelevanceCalculation(b *testing.B) {
	cfg := config.Get().Search

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = calculatePodcastRelevance("科技播客", "科技作者", "这是科技相关的描述", "科技", cfg)
	}
}
