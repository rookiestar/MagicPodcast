package services

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	podcastSearchFTSTable = "podcast_search_fts"
	episodeSearchFTSTable = "episode_search_fts"
	minFTSSearchRunes     = 3
)

var searchFTSTokenPattern = regexp.MustCompile(`[A-Za-z0-9]+`)

func canUseSearchFTS(db *gorm.DB, tableName string, keyword string) bool {
	if utf8.RuneCountInString(strings.TrimSpace(keyword)) < minFTSSearchRunes {
		return false
	}
	if buildFTSMatchQuery(keyword) == "" {
		return false
	}

	return searchFTSTableExists(db, tableName)
}

func (s *SearchService) canUseFTS(tableName string, keyword string) bool {
	if utf8.RuneCountInString(strings.TrimSpace(keyword)) < minFTSSearchRunes {
		return false
	}
	if buildFTSMatchQuery(keyword) == "" {
		return false
	}

	return s.ftsTables[tableName]
}

func discoverSearchFTSTables(db *gorm.DB) map[string]bool {
	return map[string]bool{
		podcastSearchFTSTable: searchFTSTableExists(db, podcastSearchFTSTable),
		episodeSearchFTSTable: searchFTSTableExists(db, episodeSearchFTSTable),
	}
}

func searchFTSTableExists(db *gorm.DB, tableName string) bool {
	var count int64
	err := db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
		tableName,
	).Scan(&count).Error

	return err == nil && count > 0
}

func buildFTSMatchQuery(keyword string) string {
	tokens := searchFTSTokenPattern.FindAllString(strings.ToLower(keyword), -1)
	terms := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if len(token) >= minFTSSearchRunes {
			terms = append(terms, token+"*")
		}
	}

	return strings.Join(terms, " ")
}

func buildPodcastFTSQuery(db *gorm.DB, keyword string, tagIDs []uint) *gorm.DB {
	query := db.Unscoped().
		Table("podcast_search_fts").
		Joins("CROSS JOIN podcasts ON podcasts.id = podcast_search_fts.rowid").
		Where("podcasts.deleted_at IS NULL").
		Where("podcast_search_fts MATCH ?", buildFTSMatchQuery(keyword))

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

func buildPodcastFTSOptimizedQuery(db *gorm.DB, keyword string, tagIDs []uint, limit int) *gorm.DB {
	normalizedKeyword := strings.ToLower(keyword)
	query := buildPodcastFTSQuery(db, keyword, tagIDs).Select("podcasts.*")

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
	}).Order(clause.Expr{
		SQL:                "podcasts.id DESC",
		WithoutParentheses: true,
	})

	if limit > 0 {
		query = query.Limit(limit)
	}

	return query
}

func buildEpisodeFTSQuery(db *gorm.DB, keyword string, tagIDs []uint) *gorm.DB {
	query := db.Unscoped().
		Table("episode_search_fts").
		Joins("CROSS JOIN episodes ON episodes.id = episode_search_fts.rowid").
		Joins("CROSS JOIN podcasts ON podcasts.id = episodes.podcast_id").
		Where("episodes.deleted_at IS NULL").
		Where("podcasts.deleted_at IS NULL").
		Where("episode_search_fts MATCH ?", buildFTSMatchQuery(keyword))

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

func buildEpisodeFTSOptimizedQuery(db *gorm.DB, keyword string, tagIDs []uint, limit int) *gorm.DB {
	normalizedKeyword := strings.ToLower(keyword)
	query := buildEpisodeFTSQuery(db, keyword, tagIDs).
		Select("episodes.*, podcasts.title as podcast_title, podcasts.cover_url as podcast_cover_url")

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
	}).Order(clause.Expr{
		SQL:                "episodes.published_date DESC",
		WithoutParentheses: true,
	}).Order("episodes.id DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	return query
}
