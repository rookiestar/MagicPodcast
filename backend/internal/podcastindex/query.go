package podcastindex

import (
	"database/sql"
	"fmt"
	"magicpodcast/internal/logger"

	_ "modernc.org/sqlite"
)

// Query PodcastIndex数据库查询器
type Query struct {
	db *sql.DB
}

// PodcastInfo PodcastIndex中的播客信息
type PodcastInfo struct {
	ID          int
	Title       string
	Author      string
	Description string
	CoverURL    string
	FeedURL     string
	ITunesID    int
	Language    string
	WebsiteURL  string // 网站链接（对应link字段）

	// PodcastIndex 特有的新字段
	NewestEnclosureURL      string // 最新单集音频URL
	NewestEnclosureDuration int    // 最新单集时长
	LastUpdate              int64  // Feed最后更新时间（Unix时间戳）
	NewestItemPubdate       int64  // 最新单集发布时间
	OldestItemPubdate       int64  // 最旧单集发布时间
	EpisodeCount            int    // 单集总数
	PopularityScore         int    // 受欢迎程度
	Priority                int    // 优先级
	UpdateFrequency         int    // 更新频率
}

// NewQuery 创建PodcastIndex查询器
func NewQuery(dbPath string) (*Query, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(1) // SQLite建议只打开一个连接
	db.SetMaxIdleConns(1)

	// 测试连接
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Query{db: db}, nil
}

// Close 关闭数据库连接
func (q *Query) Close() error {
	if q.db != nil {
		return q.db.Close()
	}
	return nil
}

// FindByFeedURL 根据Feed URL查找播客
func (q *Query) FindByFeedURL(feedURL string) (*PodcastInfo, error) {
	// 使用 CASE 语句处理空字符串的 itunesId
	// 从 v_unique_podcasts 视图查询，自动获取每个title的最优记录
	query := `
		SELECT id, title, itunesAuthor, description, imageUrl, url,
			   CASE
				   WHEN itunesId = '' THEN NULL
				   WHEN typeof(itunesId) = 'text' THEN NULL
				   ELSE CAST(itunesId AS INTEGER)
			   END as itunesId,
			   language, link,
			   newestEnclosureUrl, newestEnclosureDuration, lastUpdate,
			   newestItemPubdate, oldestItemPubdate, popularityScore,
			   priority, updateFrequency, episodeCount
		FROM v_unique_podcasts
		WHERE url = ?
		LIMIT 1
	`

	logger.Infof("  💾 查询 PodcastIndex (去重视图): %s", feedURL)
	row := q.db.QueryRow(query, feedURL)

	var info PodcastInfo
	var itunesID sql.NullInt64
	var coverURL, websiteURL, language sql.NullString
	var newestEnclosureURL sql.NullString

	err := row.Scan(
		&info.ID,
		&info.Title,
		&info.Author,
		&info.Description,
		&coverURL,
		&info.FeedURL,
		&itunesID,
		&language,
		&websiteURL,
		&newestEnclosureURL,
		&info.NewestEnclosureDuration,
		&info.LastUpdate,
		&info.NewestItemPubdate,
		&info.OldestItemPubdate,
		&info.PopularityScore,
		&info.Priority,
		&info.UpdateFrequency,
		&info.EpisodeCount,
	)

	if err == sql.ErrNoRows {
		logger.Infof("  📭 PodcastIndex: 未找到")
		return nil, nil // 未找到
	}
	if err != nil {
		logger.Infof("  ❌ PodcastIndex查询错误: %v", err)
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	logger.Infof("  ✅ PodcastIndex: 找到 - %s", info.Title)

	// 处理NULL值
	if coverURL.Valid {
		info.CoverURL = coverURL.String
	}
	if websiteURL.Valid {
		info.WebsiteURL = websiteURL.String
	}
	if language.Valid {
		info.Language = language.String
	}
	if newestEnclosureURL.Valid {
		info.NewestEnclosureURL = newestEnclosureURL.String
	}
	if itunesID.Valid {
		info.ITunesID = int(itunesID.Int64)
	}

	return &info, nil
}

// FindByTitle 根据标题精准搜索播客（返回多个结果）
func (q *Query) FindByTitle(title string) ([]*PodcastInfo, error) {
	// 使用 CASE 语句处理空字符串的 itunesId
	// 从 v_unique_podcasts 视图查询，自动获取每个title的最优记录
	query := `
		SELECT id, title, itunesAuthor, description, imageUrl, url,
			   CASE
				   WHEN itunesId = '' THEN NULL
				   WHEN typeof(itunesId) = 'text' THEN NULL
				   ELSE CAST(itunesId AS INTEGER)
			   END as itunesId,
			   language, link,
			   newestEnclosureUrl, newestEnclosureDuration, lastUpdate,
			   newestItemPubdate, oldestItemPubdate, popularityScore,
			   priority, updateFrequency, episodeCount
		FROM v_unique_podcasts
		WHERE title = ?
		LIMIT 10
	`

	rows, err := q.db.Query(query, title)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer rows.Close()

	var infos []*PodcastInfo

	for rows.Next() {
		var info PodcastInfo
		var itunesID sql.NullInt64
		var coverURL, websiteURL, language sql.NullString
		var newestEnclosureURL sql.NullString

		err := rows.Scan(
			&info.ID,
			&info.Title,
			&info.Author,
			&info.Description,
			&coverURL,
			&info.FeedURL,
			&itunesID,
			&language,
			&websiteURL,
			&newestEnclosureURL,
			&info.NewestEnclosureDuration,
			&info.LastUpdate,
			&info.NewestItemPubdate,
			&info.OldestItemPubdate,
			&info.PopularityScore,
			&info.Priority,
			&info.UpdateFrequency,
			&info.EpisodeCount,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// 处理NULL值
		if coverURL.Valid {
			info.CoverURL = coverURL.String
		}
		if websiteURL.Valid {
			info.WebsiteURL = websiteURL.String
		}
		if language.Valid {
			info.Language = language.String
		}
		if newestEnclosureURL.Valid {
			info.NewestEnclosureURL = newestEnclosureURL.String
		}
		if itunesID.Valid {
			info.ITunesID = int(itunesID.Int64)
		}

		infos = append(infos, &info)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return infos, nil
}

// FindByITunesID 根据iTunes ID查找播客
func (q *Query) FindByITunesID(itunesID int) (*PodcastInfo, error) {
	// 从 v_unique_podcasts 视图查询，自动获取每个title的最优记录
	query := `
		SELECT id, title, itunesAuthor, description, imageUrl, url, itunesId, language, link,
			   newestEnclosureUrl, newestEnclosureDuration, lastUpdate,
			   newestItemPubdate, oldestItemPubdate, popularityScore,
			   priority, updateFrequency, episodeCount
		FROM v_unique_podcasts
		WHERE itunesId = ?
		LIMIT 1
	`

	row := q.db.QueryRow(query, itunesID)

	var info PodcastInfo
	var dbItunesID sql.NullInt64
	var coverURL, websiteURL, language sql.NullString
	var newestEnclosureURL sql.NullString

	err := row.Scan(
		&info.ID,
		&info.Title,
		&info.Author,
		&info.Description,
		&coverURL,
		&info.FeedURL,
		&dbItunesID,
		&language,
		&websiteURL,
		&newestEnclosureURL,
		&info.NewestEnclosureDuration,
		&info.LastUpdate,
		&info.NewestItemPubdate,
		&info.OldestItemPubdate,
		&info.PopularityScore,
		&info.Priority,
		&info.UpdateFrequency,
		&info.EpisodeCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil // 未找到
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	// 处理NULL值
	if coverURL.Valid {
		info.CoverURL = coverURL.String
	}
	if websiteURL.Valid {
		info.WebsiteURL = websiteURL.String
	}
	if language.Valid {
		info.Language = language.String
	}
	if newestEnclosureURL.Valid {
		info.NewestEnclosureURL = newestEnclosureURL.String
	}
	if dbItunesID.Valid {
		info.ITunesID = int(dbItunesID.Int64)
	}

	return &info, nil
}

// Stats 获取数据库统计信息
func (q *Query) Stats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// 总播客数
	var totalPodcasts int
	err := q.db.QueryRow("SELECT COUNT(*) FROM podcasts").Scan(&totalPodcasts)
	if err != nil {
		return nil, fmt.Errorf("failed to count podcasts: %w", err)
	}
	stats["total_podcasts"] = totalPodcasts

	// 有Feed URL的播客数
	var withFeed int
	err = q.db.QueryRow("SELECT COUNT(*) FROM podcasts WHERE url IS NOT NULL AND url != ''").Scan(&withFeed)
	if err != nil {
		return nil, fmt.Errorf("failed to count podcasts with feed: %w", err)
	}
	stats["with_feed_url"] = withFeed

	return stats, nil
}
