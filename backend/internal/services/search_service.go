package services

import (
	"fmt"
	"sort"
	"strings"
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
	cfg := config.Get()
	return &SearchService{
		db:     database.GetDB(),
		config: cfg.Search,
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
	EpisodePageSize int    `json:"episode_page_size"`         // 单集每页数量
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
			Podcasts: s.buildPaginationInfo(podTotal, req.Page, req.PageSize),
			Episodes: s.buildPaginationInfo(epiTotal, req.EpisodePage, req.EpisodePageSize),
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
	Episode         episodeResult
	RelevanceScore  float64
}

// searchPodcasts 搜索播客
func (s *SearchService) searchPodcasts(req SearchRequest) ([]models.PodcastSearchResult, int64, error) {
	keyword := req.Query
	keywordLower := strings.ToLower(keyword)

	// 构建基础查询
	query := s.db.Model(&models.Podcast{}).
		Where("podcasts.deleted_at IS NULL").
		Where("LOWER(podcasts.title) LIKE ? OR LOWER(podcasts.author) LIKE ? OR LOWER(podcasts.description) LIKE ?",
			"%"+keywordLower+"%", "%"+keywordLower+"%", "%"+keywordLower+"%")

	// 标签筛选
	if len(req.TagIDs) > 0 {
		for i, tagID := range req.TagIDs {
			alias := fmt.Sprintf("pt%d", i)
			query = query.Joins(
				fmt.Sprintf("INNER JOIN podcasts_tags %s ON %s.podcast_id = podcasts.id", alias, alias),
			).Where(fmt.Sprintf("%s.tag_id = ?", alias), tagID)
		}
		query = query.Group("podcasts.id")
	}

	// 获取总数
	var total int64
	query.Count(&total)

	// 查询结果（不分页，先获取所有匹配的结果用于排序）
	var allPodcasts []models.Podcast
	if err := query.Find(&allPodcasts).Error; err != nil {
		return nil, 0, err
	}

	// 计算相关性得分并转换为响应格式
	results := make([]podcastWithScore, len(allPodcasts))
	for i, p := range allPodcasts {
		score := s.calculatePodcastRelevance(p.Title, p.Author, p.Description, keyword)
		results[i] = podcastWithScore{
			Podcast:        p,
			RelevanceScore: score,
		}
	}

	// 按相关性得分排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].RelevanceScore > results[j].RelevanceScore
	})

	// 手动分页
	offset := (req.Page - 1) * req.PageSize
	end := offset + req.PageSize
	if end > len(results) {
		end = len(results)
	}
	if offset > len(results) {
		return []models.PodcastSearchResult{}, total, nil
	}

	// 转换为响应格式
	podcasts := make([]models.PodcastSearchResult, 0, end-offset)
	for i := offset; i < end; i++ {
		r := results[i]

		matchedFields := s.extractMatchedFields(r.Podcast.Title, r.Podcast.Author, r.Podcast.Description, keyword)
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
	keywordLower := strings.ToLower(keyword)

	// 构建查询
	query := s.db.Model(&models.Episode{}).
		Select("episodes.*, podcasts.title as podcast_title, podcasts.cover_url as podcast_cover_url").
		Joins("JOIN podcasts ON episodes.podcast_id = podcasts.id").
		Where("episodes.deleted_at IS NULL").
		Where("podcasts.deleted_at IS NULL").
		Where("LOWER(episodes.title) LIKE ? OR LOWER(episodes.show_notes) LIKE ?",
			"%"+keywordLower+"%", "%"+keywordLower+"%")

	// 标签筛选（通过播客的标签）
	if len(req.TagIDs) > 0 {
		for i, tagID := range req.TagIDs {
			alias := fmt.Sprintf("pt%d", i)
			query = query.Joins(
				fmt.Sprintf("INNER JOIN podcasts_tags %s ON %s.podcast_id = podcasts.id", alias, alias),
			).Where(fmt.Sprintf("%s.tag_id = ?", alias), tagID)
		}
		query = query.Group("episodes.id")
	}

	// 获取总数
	var total int64
	query.Count(&total)

	// 查询结果（不分页，先获取所有匹配的结果用于排序）
	var allEpisodes []episodeResult
	if err := query.Find(&allEpisodes).Error; err != nil {
		return nil, 0, err
	}

	// 计算相关性得分并转换为响应格式
	results := make([]episodeWithScore, len(allEpisodes))
	for i := range allEpisodes {
		score := s.calculateEpisodeRelevance(allEpisodes[i].Title, allEpisodes[i].ShowNotes, keyword)
		results[i] = episodeWithScore{
			Episode:         allEpisodes[i],
			RelevanceScore:  score,
		}
	}

	// 按相关性得分排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].RelevanceScore > results[j].RelevanceScore
	})

	// 手动分页
	offset := (req.EpisodePage - 1) * req.EpisodePageSize
	end := offset + req.EpisodePageSize
	if end > len(results) {
		end = len(results)
	}
	if offset > len(results) {
		return []models.EpisodeSearchResult{}, total, nil
	}

	// 转换为响应格式
	episodes := make([]models.EpisodeSearchResult, 0, end-offset)
	for i := offset; i < end; i++ {
		r := results[i]
		var publishedDate *time.Time
		if !r.Episode.PublishedDate.IsZero() {
			publishedDate = &r.Episode.PublishedDate
		}

		matchedFields := s.extractMatchedFieldsFromEpisode(r.Episode.Title, r.Episode.ShowNotes, keyword)
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

// calculatePodcastRelevance 计算播客相关性得分
func (s *SearchService) calculatePodcastRelevance(title, author, description, keyword string) float64 {
	keywordLower := strings.ToLower(keyword)
	var score float64

	// 标题匹配
	titleLower := strings.ToLower(title)
	if titleLower == keywordLower {
		score += s.config.Weights.PodcastTitle * s.config.MatchMultipliers.Exact
	} else if strings.HasPrefix(titleLower, keywordLower) {
		score += s.config.Weights.PodcastTitle * s.config.MatchMultipliers.Prefix
	} else if strings.Contains(titleLower, keywordLower) {
		score += s.config.Weights.PodcastTitle * s.config.MatchMultipliers.Contains
	}

	// 作者匹配
	authorLower := strings.ToLower(author)
	if authorLower == keywordLower {
		score += s.config.Weights.Author * s.config.MatchMultipliers.Exact
	} else if strings.HasPrefix(authorLower, keywordLower) {
		score += s.config.Weights.Author * s.config.MatchMultipliers.Prefix
	} else if strings.Contains(authorLower, keywordLower) {
		score += s.config.Weights.Author * s.config.MatchMultipliers.Contains
	}

	// 简介匹配
	descLower := strings.ToLower(description)
	if strings.Contains(descLower, keywordLower) {
		occurrences := strings.Count(descLower, keywordLower)
		descScore := s.config.Weights.PodcastDesc * s.config.MatchMultipliers.Contains
		if occurrences > 1 {
			descScore *= (1 + float64(occurrences-1) * s.config.MatchMultipliers.Occurrence)
		}
		score += descScore
	}

	return score
}

// calculateEpisodeRelevance 计算单集相关性得分
func (s *SearchService) calculateEpisodeRelevance(title, showNotes, keyword string) float64 {
	keywordLower := strings.ToLower(keyword)
	var score float64

	// 标题匹配
	titleLower := strings.ToLower(title)
	if titleLower == keywordLower {
		score += s.config.Weights.EpisodeTitle * s.config.MatchMultipliers.Exact
	} else if strings.HasPrefix(titleLower, keywordLower) {
		score += s.config.Weights.EpisodeTitle * s.config.MatchMultipliers.Prefix
	} else if strings.Contains(titleLower, keywordLower) {
		score += s.config.Weights.EpisodeTitle * s.config.MatchMultipliers.Contains
	}

	// 内容匹配
	notesLower := strings.ToLower(showNotes)
	if strings.Contains(notesLower, keywordLower) {
		occurrences := strings.Count(notesLower, keywordLower)
		notesScore := s.config.Weights.EpisodeContent * s.config.MatchMultipliers.Contains
		if occurrences > 1 {
			notesScore *= (1 + float64(occurrences-1) * s.config.MatchMultipliers.Occurrence)
		}
		score += notesScore
	}

	return score
}

// extractMatchedFields 提取匹配字段（播客）
func (s *SearchService) extractMatchedFields(title, author, description, keyword string) []models.MatchedField {
	var fields []models.MatchedField
	keywordLower := strings.ToLower(keyword)

	if strings.Contains(strings.ToLower(title), keywordLower) {
		fields = append(fields, models.MatchedField{
			Field:   "title",
			Score:   s.config.Weights.PodcastTitle,
			Snippet: s.generateSnippet(title, keyword),
		})
	}

	if strings.Contains(strings.ToLower(author), keywordLower) {
		fields = append(fields, models.MatchedField{
			Field:   "author",
			Score:   s.config.Weights.Author,
			Snippet: s.generateSnippet(author, keyword),
		})
	}

	if strings.Contains(strings.ToLower(description), keywordLower) {
		fields = append(fields, models.MatchedField{
			Field:   "description",
			Score:   s.config.Weights.PodcastDesc,
			Snippet: s.generateSnippet(description, keyword),
		})
	}

	return fields
}

// extractMatchedFieldsFromEpisode 提取匹配字段（单集）
func (s *SearchService) extractMatchedFieldsFromEpisode(title, showNotes, keyword string) []models.MatchedField {
	var fields []models.MatchedField
	keywordLower := strings.ToLower(keyword)
	titleLower := strings.ToLower(title)
	showNotesLower := strings.ToLower(showNotes)

	if strings.Contains(titleLower, keywordLower) {
		fields = append(fields, models.MatchedField{
			Field:   "title",
			Score:   s.config.Weights.EpisodeTitle,
			Snippet: s.generateSnippet(title, keyword),
		})
	}

	if strings.Contains(showNotesLower, keywordLower) {
		fields = append(fields, models.MatchedField{
			Field:   "show_notes",
			Score:   s.config.Weights.EpisodeContent,
			Snippet: s.generateSnippet(showNotes, keyword),
		})
	}

	return fields
}

// stripHTML 移除 HTML 标签
func (s *SearchService) stripHTML(text string) string {
	// 简单的 HTML 标签移除
	var result strings.Builder
	inTag := false

	for _, r := range text {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// generateSnippet 生成匹配片段（用于高亮显示）
func (s *SearchService) generateSnippet(text, keyword string) string {
	// 清理文本：先移除 HTML 标签，再清理换行符
	text = s.stripHTML(text)
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.TrimSpace(text)

	if len(text) <= 150 {
		return text
	}

	keywordLower := strings.ToLower(keyword)
	textLower := strings.ToLower(text)

	// 查找关键词第一次出现的位置
	idx := strings.Index(textLower, keywordLower)
	if idx == -1 {
		// 如果没找到关键词，返回前150个字符
		if len(text) > 150 {
			return text[:150] + "..."
		}
		return text
	}

	// 生成以关键词为中心的片段
	// 策略：让关键词出现在片段的前 1/4 处，这样用户能更快看到关键词
	snippetLength := 150
	prefixLength := 35 // 关键词前保留约 35 个字符

	start := idx - prefixLength
	if start < 0 {
		start = 0
	}

	end := start + snippetLength
	if end > len(text) {
		end = len(text)
		// 如果接近文本末尾，调整 start 以保持 snippet 长度
		start = end - snippetLength
		if start < 0 {
			start = 0
		}
	}

	// 最终验证：确保 snippet 包含完整的关键词
	snippet := text[start:end]
	if !strings.Contains(strings.ToLower(snippet), keywordLower) {
		// 如果因为某种原因 snippet 不包含关键词，使用最简单的策略
		start = idx
		end = idx + len(keyword) + 100
		if end > len(text) {
			end = len(text)
		}
		if start > 0 {
			start = start - 20
			if start < 0 {
				start = 0
			}
		}
		snippet = text[start:end]
	}

	// 添加省略号
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(text) {
		snippet = snippet + "..."
	}

	return snippet
}

// buildPaginationInfo 构建分页信息
func (s *SearchService) buildPaginationInfo(total int64, page, pageSize int) models.PaginationInfo {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return models.PaginationInfo{
		Page:       page,
		PageSize:   pageSize,
		Total:      int(total),
		TotalPages: totalPages,
	}
}
