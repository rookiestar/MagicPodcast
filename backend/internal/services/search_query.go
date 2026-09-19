package services

import (
	"fmt"
	"strings"
	"unicode"

	"magicpodcast/internal/config"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// buildPodcastQuery 构建播客搜索查询
func buildPodcastQuery(db *gorm.DB, keyword string, tagIDs []uint) *gorm.DB {
	keywordLower := fmt.Sprintf("%s%s%s", "%", strings.ToLower(keyword), "%")
	query := db.Model(&models.Podcast{}).
		Where("podcasts.deleted_at IS NULL").
		Where(searchPodcastPredicate(db, keyword, keywordLower))

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
	if hasSearchHan(keyword) {
		score := searchScoreExpression(db, keyword, false)
		score.SQL += " DESC, podcasts.id DESC"
		query := buildPodcastQuery(db, keyword, tagIDs).Order(clause.OrderBy{Expression: score})
		if limit > 0 {
			query = query.Limit(limit)
		}
		return query
	}
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
		Where(searchEpisodePredicate(db, keyword, keywordLower))

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
	if hasSearchHan(keyword) {
		score := searchScoreExpression(db, keyword, true)
		score.SQL += " DESC, episodes.published_date DESC, episodes.id DESC"
		query := buildEpisodeQuery(db, keyword, tagIDs).Order(clause.OrderBy{Expression: score})
		if limit > 0 {
			query = query.Limit(limit)
		}
		return query
	}
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

func searchPodcastPredicate(db *gorm.DB, keyword, pattern string) clause.Expression {
	if hasSearchHan(keyword) {
		score := searchScoreExpression(db, keyword, false)
		score.SQL += " > 0"
		return clause.And(searchCandidatePredicate(keyword, false), score)
	}
	return clause.Expr{SQL: "LOWER(podcasts.title) LIKE ? OR LOWER(podcasts.author) LIKE ? OR LOWER(podcasts.description) LIKE ?", Vars: []interface{}{pattern, pattern, pattern}}
}

func searchEpisodePredicate(db *gorm.DB, keyword, pattern string) clause.Expression {
	if hasSearchHan(keyword) {
		score := searchScoreExpression(db, keyword, true)
		score.SQL += " > 0"
		return clause.And(searchCandidatePredicate(keyword, true), score)
	}
	return clause.Expr{SQL: "LOWER(episodes.title) LIKE ? OR LOWER(episodes.show_notes) LIKE ?", Vars: []interface{}{pattern, pattern}}
}

func searchScoreExpression(db *gorm.DB, keyword string, episode bool) clause.Expr {
	cfg := defaultSearchConfig()
	if value, ok := db.Get("search.config"); ok {
		cfg = value.(config.SearchConfig)
	}
	fields := "COALESCE(podcasts.title, ''), COALESCE(podcasts.author, ''), COALESCE(podcasts.description, '')"
	titleWeight, bodyWeight := cfg.Weights.PodcastTitle, cfg.Weights.PodcastDesc
	if episode {
		fields = "COALESCE(episodes.title, ''), '', COALESCE(episodes.show_notes, '')"
		titleWeight, bodyWeight = cfg.Weights.EpisodeTitle, cfg.Weights.EpisodeContent
	}
	return clause.Expr{SQL: "search_score(" + fields + ", ?, ?, ?, ?, ?, ?, ?, ?, ?)", Vars: []interface{}{keyword, episode, titleWeight, cfg.Weights.Author, bodyWeight, cfg.MatchMultipliers.Exact, cfg.MatchMultipliers.Prefix, cfg.MatchMultipliers.Contains, cfg.MatchMultipliers.Occurrence}, WithoutParentheses: true}
}

// A LIKE superset cheaply rejects unrelated rows before the exact normalized
// scorer. Wildcards only replace optional Han/ASCII boundaries; SQL metacharacters
// in the user's query remain literal. The scorer rejects all extra matches.
func searchCandidatePredicate(keyword string, episode bool) clause.Expr {
	runes := []rune(normalizeSearchText(keyword))
	var pattern strings.Builder
	pattern.WriteByte('%')
	for i, r := range runes {
		if i > 0 && ((unicode.Is(unicode.Han, runes[i-1]) && searchASCII(r)) || (searchASCII(runes[i-1]) && unicode.Is(unicode.Han, r))) {
			pattern.WriteByte('%')
		}
		if r == '%' || r == '_' || r == '\\' {
			pattern.WriteByte('\\')
		}
		pattern.WriteRune(r)
	}
	pattern.WriteByte('%')
	fields := []string{"podcasts.title", "podcasts.author", "podcasts.description"}
	if episode {
		fields = []string{"episodes.title", "episodes.show_notes"}
	}
	conditions := make([]string, 0, len(fields))
	values := make([]interface{}, 0, len(fields))
	for _, field := range fields {
		conditions = append(conditions, "LOWER("+field+") LIKE ? ESCAPE '\\'")
		values = append(values, pattern.String())
	}
	return clause.Expr{SQL: "(" + strings.Join(conditions, " OR ") + ")", Vars: values}
}
