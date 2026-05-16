package services

import (
	"fmt"
	"sort"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// SearchService 搜索服务
type SearchService struct {
	db     *gorm.DB
	config config.SearchConfig
}

// NewSearchService 创建搜索服务
func NewSearchService() *SearchService {
	var searchConfig config.SearchConfig
	if cfg := config.Get(); cfg != nil {
		searchConfig = cfg.Search
	} else {
		searchConfig = defaultSearchConfig()
	}

	return NewSearchServiceWithDB(database.GetDB(), searchConfig)
}

func NewSearchServiceWithDB(db *gorm.DB, searchConfig config.SearchConfig) *SearchService {
	return &SearchService{
		db:     db,
		config: searchConfig,
	}
}

func defaultSearchConfig() config.SearchConfig {
	return config.SearchConfig{
		Weights: config.SearchWeights{
			PodcastTitle:   1.0,
			EpisodeTitle:   1.0,
			Author:         0.8,
			PodcastDesc:    0.7,
			EpisodeContent: 0.7,
		},
		MatchMultipliers: config.SearchMatchMultipliers{
			Exact:      1.5,
			Prefix:     1.2,
			Contains:   1.0,
			Occurrence: 0.1,
		},
		DefaultPageSize: 20,
		MaxPageSize:     100,
	}
}

// SearchRequest 搜索请求
type SearchRequest struct {
	Query           string `json:"query" binding:"required"` // 搜索关键词
	Type            string `json:"type"`                     // 搜索类型：all/podcasts/episodes
	TagIDs          []uint `json:"tag_ids"`                  // 标签筛选
	Page            int    `json:"page"`                     // 播客页码
	PageSize        int    `json:"page_size"`                // 播客每页数量
	EpisodePage     int    `json:"episode_page"`             // 单集页码
	EpisodePageSize int    `json:"episode_page_size"`        // 单集每页数量
}

// SearchResponse 搜索响应
type SearchResponse struct {
	Podcasts   []models.PodcastSearchResult `json:"podcasts"`
	Episodes   []models.EpisodeSearchResult `json:"episodes"`
	Pagination models.SearchPagination      `json:"pagination"`
}

// Search 执行搜索
func (s *SearchService) Search(req SearchRequest) (*SearchResponse, error) {
	var (
		podcasts []models.PodcastSearchResult
		episodes []models.EpisodeSearchResult
		podTotal int64
		epiTotal int64
		err      error
	)

	// 搜索播客
	if req.Type == "all" || req.Type == "podcasts" {
		podcasts, podTotal, err = s.searchPodcasts(req)
		if err != nil {
			return nil, fmt.Errorf("search podcasts failed: %w", err)
		}
	}

	// 搜索单集
	if req.Type == "all" || req.Type == "episodes" {
		episodes, epiTotal, err = s.searchEpisodes(req)
		if err != nil {
			return nil, fmt.Errorf("search episodes failed: %w", err)
		}
	}

	// 构建响应
	return &SearchResponse{
		Podcasts: podcasts,
		Episodes: episodes,
		Pagination: models.SearchPagination{
			Podcasts: buildPaginationInfo(podTotal, req.Page, req.PageSize),
			Episodes: buildPaginationInfo(epiTotal, req.EpisodePage, req.EpisodePageSize),
		},
	}, nil
}

type podcastWithScore struct {
	Podcast        models.Podcast
	RelevanceScore float64
}

type episodeResult struct {
	models.Episode
	PodcastTitle    string
	PodcastCoverURL string
}

type episodeWithScore struct {
	Episode        episodeResult
	RelevanceScore float64
}

// searchPodcasts 搜索播客
func (s *SearchService) searchPodcasts(req SearchRequest) ([]models.PodcastSearchResult, int64, error) {
	keyword := req.Query

	// 构建基础查询
	query := buildPodcastQuery(s.db, keyword, req.TagIDs)

	// 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	optimizedQuery := buildPodcastOptimizedQuery(
		s.db,
		keyword,
		req.TagIDs,
		buildSearchCandidateLimit(req.Page, req.PageSize),
	)

	var allPodcasts []models.Podcast
	if err := optimizedQuery.Find(&allPodcasts).Error; err != nil {
		return nil, 0, err
	}

	// 计算相关性得分
	results := make([]podcastWithScore, len(allPodcasts))
	for i, p := range allPodcasts {
		score := calculatePodcastRelevance(p.Title, p.Author, p.Description, keyword, s.config)
		results[i] = podcastWithScore{
			Podcast:        p,
			RelevanceScore: score,
		}
	}

	// 按相关性得分排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].RelevanceScore > results[j].RelevanceScore
	})

	// 过滤低相关性结果
	filteredResults := make([]podcastWithScore, 0)
	for _, r := range results {
		if r.RelevanceScore > 0 {
			filteredResults = append(filteredResults, r)
		}
	}

	// 手动分页
	offset := (req.Page - 1) * req.PageSize
	end := offset + req.PageSize
	if end > len(filteredResults) {
		end = len(filteredResults)
	}
	if offset > len(filteredResults) {
		return []models.PodcastSearchResult{}, total, nil
	}

	// 转换为响应格式
	podcasts := make([]models.PodcastSearchResult, 0, end-offset)
	for i := offset; i < end; i++ {
		r := filteredResults[i]

		matchedFields := extractMatchedFields(r.Podcast.Title, r.Podcast.Author, r.Podcast.Description, keyword, s.config)
		if matchedFields == nil {
			matchedFields = []models.MatchedField{}
		}

		podcasts = append(podcasts, models.PodcastSearchResult{
			ID:                r.Podcast.ID,
			Title:             r.Podcast.Title,
			Author:            r.Podcast.Author,
			Description:       r.Podcast.Description,
			CoverURL:          r.Podcast.CoverURL,
			EpisodeCount:      r.Podcast.EpisodeCount,
			NewestEpisodeDate: r.Podcast.NewestEpisodeDate,
			RelevanceScore:    r.RelevanceScore,
			MatchedFields:     matchedFields,
		})
	}

	return podcasts, total, nil
}

// searchEpisodes 搜索单集
func (s *SearchService) searchEpisodes(req SearchRequest) ([]models.EpisodeSearchResult, int64, error) {
	keyword := req.Query

	// 构建基础查询
	query := buildEpisodeQuery(s.db, keyword, req.TagIDs)

	// 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	optimizedQuery := buildEpisodeOptimizedQuery(
		s.db,
		keyword,
		req.TagIDs,
		buildSearchCandidateLimit(req.EpisodePage, req.EpisodePageSize),
	)

	var allEpisodes []episodeResult
	if err := optimizedQuery.Find(&allEpisodes).Error; err != nil {
		return nil, 0, err
	}

	// 计算相关性得分
	results := make([]episodeWithScore, len(allEpisodes))
	for i := range allEpisodes {
		score := calculateEpisodeRelevance(allEpisodes[i].Title, allEpisodes[i].ShowNotes, keyword, s.config)
		results[i] = episodeWithScore{
			Episode:        allEpisodes[i],
			RelevanceScore: score,
		}
	}

	// 按相关性得分排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].RelevanceScore > results[j].RelevanceScore
	})

	// 过滤低相关性结果
	filteredResults := make([]episodeWithScore, 0)
	for _, r := range results {
		if r.RelevanceScore > 0 {
			filteredResults = append(filteredResults, r)
		}
	}

	// 手动分页
	offset := (req.EpisodePage - 1) * req.EpisodePageSize
	end := offset + req.EpisodePageSize
	if end > len(filteredResults) {
		end = len(filteredResults)
	}
	if offset > len(filteredResults) {
		return []models.EpisodeSearchResult{}, total, nil
	}

	// 转换为响应格式
	episodes := make([]models.EpisodeSearchResult, 0, end-offset)
	for i := offset; i < end; i++ {
		r := filteredResults[i]
		var publishedDate *time.Time
		if !r.Episode.PublishedDate.IsZero() {
			publishedDate = &r.Episode.PublishedDate
		}

		matchedFields := extractMatchedFieldsFromEpisode(r.Episode.Title, r.Episode.ShowNotes, keyword, s.config)
		if matchedFields == nil {
			matchedFields = []models.MatchedField{}
		}

		episodes = append(episodes, models.EpisodeSearchResult{
			ID:              r.Episode.ID,
			PodcastID:       r.Episode.PodcastID,
			PodcastTitle:    r.Episode.PodcastTitle,
			PodcastCoverURL: r.Episode.PodcastCoverURL,
			Title:           r.Episode.Title,
			ShowNotes:       r.Episode.ShowNotes,
			PublishedDate:   publishedDate,
			Duration:        r.Episode.Duration,
			RelevanceScore:  r.RelevanceScore,
			MatchedFields:   matchedFields,
		})
	}

	return episodes, total, nil
}
