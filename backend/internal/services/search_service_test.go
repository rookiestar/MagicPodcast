package services

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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

	t.Run("generateSnippet preserves multibyte characters", func(t *testing.T) {
		longText := strings.Repeat("前置内容", 30) +
			"四大科技巨头同晚交卷，市场反应却截然不同。谷歌云增速飙升。" +
			strings.Repeat("后续内容", 30)

		snippet := generateSnippet(longText, "科技")

		assert.True(t, utf8.ValidString(snippet))
		assert.NotContains(t, snippet, "\uFFFD")
		assert.Contains(t, snippet, "科技")
		assert.LessOrEqual(t, utf8.RuneCountInString(strings.Trim(snippet, ".")), 150)
	})
}

// 测试相关性计算
func TestRelevanceCalculation(t *testing.T) {
	cfg := defaultSearchConfig()

	t.Run("calculatePodcastRelevance", func(t *testing.T) {
		// 完全匹配
		score1 := calculatePodcastRelevance("科技播客", "作者", "描述", "科技播客", cfg)
		score2 := calculatePodcastRelevance("科技播客", "作者", "描述", "科技", cfg)
		assert.GreaterOrEqual(t, score1, score2)

		// 前缀匹配
		score3 := calculatePodcastRelevance("科技播客", "作者", "描述", "科技", cfg)
		score4 := calculatePodcastRelevance("这是科技播客", "作者", "描述", "科技", cfg)
		assert.Greater(t, score3, score4)
	})

	t.Run("calculateEpisodeRelevance", func(t *testing.T) {
		// 完全匹配
		score1 := calculateEpisodeRelevance("科技节目", "内容", "科技节目", cfg)
		score2 := calculateEpisodeRelevance("科技节目", "内容", "科技", cfg)
		assert.GreaterOrEqual(t, score1, score2)
	})
}

func TestSearchAllMatchesSeparateSearches(t *testing.T) {
	service := newSearchServiceForAllEquivalenceTest(t)
	baseReq := SearchRequest{
		Query:           "podcast",
		Page:            1,
		PageSize:        10,
		EpisodePage:     1,
		EpisodePageSize: 10,
	}

	allResp, err := service.Search(withSearchType(baseReq, "all"))
	require.NoError(t, err)

	podcastResp, err := service.Search(withSearchType(baseReq, "podcasts"))
	require.NoError(t, err)

	episodeResp, err := service.Search(withSearchType(baseReq, "episodes"))
	require.NoError(t, err)

	assert.Equal(t, podcastResp.Podcasts, allResp.Podcasts)
	assert.Equal(t, episodeResp.Episodes, allResp.Episodes)
	assert.Equal(t, podcastResp.Pagination.Podcasts, allResp.Pagination.Podcasts)
	assert.Equal(t, episodeResp.Pagination.Episodes, allResp.Pagination.Episodes)
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

	t.Run("buildSearchCandidateLimit", func(t *testing.T) {
		assert.Equal(t, 100, buildSearchCandidateLimit(1, 50))
		assert.Equal(t, 150, buildSearchCandidateLimit(2, 50))
		assert.Equal(t, 600, buildSearchCandidateLimit(11, 50))
		assert.Equal(t, 51, buildSearchCandidateLimit(0, 0))
	})
}

// 基准测试 - 验证性能
func BenchmarkSearchService(b *testing.B) {
	searchService := newBenchmarkSearchService(b)
	req := SearchRequest{
		Query:           "podcast",
		Type:            "all",
		Page:            1,
		PageSize:        20,
		EpisodePage:     1,
		EpisodePageSize: 20,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := searchService.Search(req); err != nil {
			b.Fatal(err)
		}
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
	cfg := defaultSearchConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = calculatePodcastRelevance("科技播客", "科技作者", "这是科技相关的描述", "科技", cfg)
	}
}

func newBenchmarkSearchService(b *testing.B) *SearchService {
	b.Helper()

	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(b, err)
	require.NoError(b, db.AutoMigrate(&models.Podcast{}, &models.Episode{}))

	now := time.Now()
	for podcastIndex := 0; podcastIndex < 50; podcastIndex++ {
		podcast := models.Podcast{
			XYZID:       fmt.Sprintf("benchmark-podcast-%02d", podcastIndex),
			Title:       fmt.Sprintf("Podcast %02d", podcastIndex),
			FeedURL:     fmt.Sprintf("https://example.com/podcast-%02d.xml", podcastIndex),
			Description: "A benchmark podcast about technology and audio.",
			Author:      "Benchmark",
		}
		require.NoError(b, db.Create(&podcast).Error)

		for episodeIndex := 0; episodeIndex < 20; episodeIndex++ {
			require.NoError(b, db.Create(&models.Episode{
				PodcastID:     podcast.ID,
				Title:         fmt.Sprintf("Podcast %02d episode %02d", podcastIndex, episodeIndex),
				ShowNotes:     "A compact podcast benchmark note about search performance.",
				PublishedDate: now.Add(-time.Duration(podcastIndex*20+episodeIndex) * time.Hour),
				GUID:          fmt.Sprintf("benchmark-%02d-%02d", podcastIndex, episodeIndex),
			}).Error)
		}
	}

	require.NoError(b, db.Exec(`
		CREATE VIRTUAL TABLE podcast_search_fts
		USING fts4(
			title,
			author,
			description,
			content='podcasts',
			tokenize=unicode61
		)
	`).Error)

	require.NoError(b, db.Exec(`
		CREATE VIRTUAL TABLE episode_search_fts
		USING fts4(
			title,
			show_notes,
			content='episodes',
			tokenize=unicode61
		)
	`).Error)

	require.NoError(b, db.Exec("INSERT INTO podcast_search_fts(podcast_search_fts) VALUES('rebuild')").Error)
	require.NoError(b, db.Exec("INSERT INTO episode_search_fts(episode_search_fts) VALUES('rebuild')").Error)

	return NewSearchServiceWithDB(db, defaultSearchConfig())
}

func newSearchServiceForAllEquivalenceTest(t *testing.T) *SearchService {
	t.Helper()

	dsn := fmt.Sprintf("file:search_all_equivalence_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.Episode{}))

	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	podcasts := []models.Podcast{
		{
			XYZID:       "equivalence-tech",
			Title:       "Podcast Systems",
			FeedURL:     "https://example.com/systems.xml",
			Description: "A podcast about maintainable systems.",
			Author:      "Example",
		},
		{
			XYZID:       "equivalence-audio",
			Title:       "Audio Notes",
			FeedURL:     "https://example.com/audio.xml",
			Description: "Production notes for podcast teams.",
			Author:      "Podcast Lab",
		},
	}

	for podcastIndex := range podcasts {
		require.NoError(t, db.Create(&podcasts[podcastIndex]).Error)
		for episodeIndex := 0; episodeIndex < 2; episodeIndex++ {
			require.NoError(t, db.Create(&models.Episode{
				PodcastID:     podcasts[podcastIndex].ID,
				Title:         fmt.Sprintf("Podcast episode %d", episodeIndex+1),
				ShowNotes:     "A podcast note used to compare all-search and type-specific search.",
				PublishedDate: now.Add(-time.Duration(podcastIndex*2+episodeIndex) * time.Hour),
				GUID:          fmt.Sprintf("equivalence-%d-%d", podcastIndex, episodeIndex),
			}).Error)
		}
	}

	createSearchFTSTablesForTest(t, db)
	return NewSearchServiceWithDB(db, defaultSearchConfig())
}

func withSearchType(req SearchRequest, searchType string) SearchRequest {
	req.Type = searchType
	return req
}
