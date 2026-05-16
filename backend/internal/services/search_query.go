package services

import (
	"fmt"
	"strings"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// buildPodcastQuery 构建播客搜索查询
func buildPodcastQuery(db *gorm.DB, keyword string, tagIDs []uint) *gorm.DB {
	keywordLower := fmt.Sprintf("%s%s%s", "%", strings.ToLower(keyword), "%")
	query := db.Model(&models.Podcast{}).
		Where("podcasts.deleted_at IS NULL").
		Where("LOWER(podcasts.title) LIKE ? OR LOWER(podcasts.author) LIKE ? OR LOWER(podcasts.description) LIKE ?",
			keywordLower, keywordLower, keywordLower)

	// 标签筛选
	if len(tagIDs) > 0 {
		for i, tagID := range tagIDs {
			alias := fmt.Sprintf("pt%d", i)
			query = query.Joins(
				fmt.Sprintf("INNER JOIN podcasts_tags %s ON %s.podcast_id = podcasts.id", alias, alias),
			).Where(fmt.Sprintf("%s.tag_id = ?", alias), tagID)
		}
		query = query.Group("podcasts.id")
	}

	return query
}

// buildPodcastOptimizedQuery 构建优化的播客查询（带排序和限制）
func buildPodcastOptimizedQuery(db *gorm.DB, keyword string, tagIDs []uint, limit int) *gorm.DB {
	normalizedKeyword := strings.ToLower(keyword)
	keywordLower := fmt.Sprintf("%s%s%s", "%", normalizedKeyword, "%")
	query := db.Model(&models.Podcast{}).
		Where("podcasts.deleted_at IS NULL").
		Where("LOWER(podcasts.title) LIKE ? OR LOWER(podcasts.author) LIKE ? OR LOWER(podcasts.description) LIKE ?",
			keywordLower, keywordLower, keywordLower)

	// 标题优先排序
	query = query.Order(clause.Expr{
		SQL: "CASE " +
			"WHEN LOWER(podcasts.title) = ? THEN 1 " +
			"WHEN LOWER(podcasts.title) LIKE ? THEN 2 " +
			"WHEN LOWER(podcasts.author) = ? THEN 3 " +
			"WHEN LOWER(podcasts.author) LIKE ? THEN 4 " +
			"ELSE 5 END",
		Vars: []interface{}{
			normalizedKeyword,
			normalizedKeyword + "%",
			normalizedKeyword,
			normalizedKeyword + "%",
		},
		WithoutParentheses: true,
	}).Order("podcasts.id DESC")

	// 标签筛选
	if len(tagIDs) > 0 {
		for i, tagID := range tagIDs {
			alias := fmt.Sprintf("pt%d", i)
			query = query.Joins(
				fmt.Sprintf("INNER JOIN podcasts_tags %s ON %s.podcast_id = podcasts.id", alias, alias),
			).Where(fmt.Sprintf("%s.tag_id = ?", alias), tagID)
		}
		query = query.Group("podcasts.id")
	}

	// 限制加载数量
	if limit > 0 {
		query = query.Limit(limit)
	}

	return query
}

// buildEpisodeQuery 构建单集搜索查询
func buildEpisodeQuery(db *gorm.DB, keyword string, tagIDs []uint) *gorm.DB {
	keywordLower := fmt.Sprintf("%s%s%s", "%", strings.ToLower(keyword), "%")
	query := db.Model(&models.Episode{}).
		Select("episodes.*, podcasts.title as podcast_title, podcasts.cover_url as podcast_cover_url").
		Joins("JOIN podcasts ON episodes.podcast_id = podcasts.id").
		Where("episodes.deleted_at IS NULL").
		Where("podcasts.deleted_at IS NULL").
		Where("LOWER(episodes.title) LIKE ? OR LOWER(episodes.show_notes) LIKE ?",
			keywordLower, keywordLower)

	// 标签筛选（通过播客的标签）
	if len(tagIDs) > 0 {
		for i, tagID := range tagIDs {
			alias := fmt.Sprintf("pt%d", i)
			query = query.Joins(
				fmt.Sprintf("INNER JOIN podcasts_tags %s ON %s.podcast_id = podcasts.id", alias, alias),
			).Where(fmt.Sprintf("%s.tag_id = ?", alias), tagID)
		}
		query = query.Group("episodes.id")
	}

	return query
}

// buildEpisodeOptimizedQuery 构建优化的单集查询（带排序和限制）
func buildEpisodeOptimizedQuery(db *gorm.DB, keyword string, tagIDs []uint, limit int) *gorm.DB {
	normalizedKeyword := strings.ToLower(keyword)
	keywordLower := fmt.Sprintf("%s%s%s", "%", normalizedKeyword, "%")
	query := db.Model(&models.Episode{}).
		Select("episodes.*, podcasts.title as podcast_title, podcasts.cover_url as podcast_cover_url").
		Joins("JOIN podcasts ON episodes.podcast_id = podcasts.id").
		Where("episodes.deleted_at IS NULL").
		Where("podcasts.deleted_at IS NULL").
		Where("LOWER(episodes.title) LIKE ? OR LOWER(episodes.show_notes) LIKE ?",
			keywordLower, keywordLower)

	// 标题优先排序
	query = query.Order(clause.Expr{
		SQL: "CASE " +
			"WHEN LOWER(episodes.title) = ? THEN 1 " +
			"WHEN LOWER(episodes.title) LIKE ? THEN 2 " +
			"ELSE 3 END",
		Vars: []interface{}{
			normalizedKeyword,
			normalizedKeyword + "%",
		},
		WithoutParentheses: true,
	}).Order("episodes.published_date DESC").Order("episodes.id DESC")

	// 标签筛选（通过播客的标签）
	if len(tagIDs) > 0 {
		for i, tagID := range tagIDs {
			alias := fmt.Sprintf("pt%d", i)
			query = query.Joins(
				fmt.Sprintf("INNER JOIN podcasts_tags %s ON %s.podcast_id = podcasts.id", alias, alias),
			).Where(fmt.Sprintf("%s.tag_id = ?", alias), tagID)
		}
		query = query.Group("episodes.id")
	}

	// 限制加载数量
	if limit > 0 {
		query = query.Limit(limit)
	}

	return query
}
