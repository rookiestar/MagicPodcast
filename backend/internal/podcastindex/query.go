package podcastindex

import (
	"context"
	"database/sql"
	"fmt"
	"magicpodcast/internal/logger"
	"strings"

	_ "modernc.org/sqlite"
)

// Query is the read-only PodcastIndex database query surface used by imports
// and the verified alternative-feed fallback. The database is optional and is
// never mutated by this package.
type Query struct {
	db *sql.DB
}

// PodcastInfo contains the bounded metadata needed to identify and rank a
// PodcastIndex feed. PodcastGUID is read from the raw podcasts table because
// the legacy v_unique_podcasts view does not expose it.
type PodcastInfo struct {
	ID          int
	Title       string
	Author      string
	Description string
	CoverURL    string
	FeedURL     string
	ITunesID    int
	PodcastGUID string
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
	Dead                    int    // PodcastIndex dead flag
	LastHTTPStatus          int    // PodcastIndex last HTTP status
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
	if q != nil && q.db != nil {
		return q.db.Close()
	}
	return nil
}

type lookupSource struct {
	from       string
	alias      string
	guidColumn string
}

// sourceForLookup keeps existing imports compatible with the deduplicating
// view, but falls back to the raw table for upgraded databases that have not
// yet created that view. The alternative-feed path deliberately queries the
// raw table so duplicate/conflicting identities remain visible.
func (q *Query) sourceForLookup() (lookupSource, error) {
	if q == nil || q.db == nil {
		return lookupSource{}, fmt.Errorf("PodcastIndex database is not open")
	}

	viewExists, err := q.objectExists("v_unique_podcasts", "view")
	if err != nil {
		return lookupSource{}, err
	}
	hasGUID, err := q.columnExists("podcasts", "podcastGuid")
	if err != nil {
		return lookupSource{}, err
	}
	guidColumn := "''"
	if hasGUID {
		guidColumn = "p.podcastGuid"
	}
	if viewExists {
		return lookupSource{
			from:       "v_unique_podcasts AS v LEFT JOIN podcasts AS p ON p.id = v.id",
			alias:      "v",
			guidColumn: guidColumn,
		}, nil
	}
	return lookupSource{
		from:       "podcasts AS p",
		alias:      "p",
		guidColumn: guidColumn,
	}, nil
}

func (q *Query) rawSource() (lookupSource, error) {
	return q.rawSourceContext(context.Background())
}

func (q *Query) rawSourceContext(ctx context.Context) (lookupSource, error) {
	if q == nil || q.db == nil {
		return lookupSource{}, fmt.Errorf("PodcastIndex database is not open")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	hasGUID, err := q.columnExistsContext(ctx, "podcasts", "podcastGuid")
	if err != nil {
		return lookupSource{}, err
	}
	guidColumn := "''"
	if hasGUID {
		guidColumn = "p.podcastGuid"
	}
	return lookupSource{
		from:       "podcasts AS p",
		alias:      "p",
		guidColumn: guidColumn,
	}, nil
}

func (q *Query) objectExists(name, objectType string) (bool, error) {
	return q.objectExistsContext(context.Background(), name, objectType)
}

func (q *Query) objectExistsContext(ctx context.Context, name, objectType string) (bool, error) {
	var count int
	if ctx == nil {
		ctx = context.Background()
	}
	err := q.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = ? AND name = ?`, objectType, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check PodcastIndex %s %q: %w", objectType, name, err)
	}
	return count > 0, nil
}

func (q *Query) columnExists(table, column string) (bool, error) {
	return q.columnExistsContext(context.Background(), table, column)
}

func (q *Query) columnExistsContext(ctx context.Context, table, column string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := q.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, fmt.Errorf("inspect PodcastIndex table %q: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, declaredType, defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &declaredType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scan PodcastIndex table %q: %w", table, err)
		}
		if strings.EqualFold(name.String, column) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read PodcastIndex table %q: %w", table, err)
	}
	return false, nil
}

func podcastInfoSelect(source lookupSource) string {
	prefix := source.alias
	selectSQL := `
		%s.id,
		%s.title,
		%s.itunesAuthor,
		%s.description,
		%s.imageUrl,
		%s.url,
		CASE
			WHEN typeof(%s.itunesId) IN ('integer', 'real') THEN CAST(%s.itunesId AS INTEGER)
			ELSE NULL
		END AS itunesId,
		%s.language,
		%s.link,
		%s.newestEnclosureUrl,
		%s.newestEnclosureDuration,
		%s.lastUpdate,
		%s.newestItemPubdate,
		%s.oldestItemPubdate,
		%s.popularityScore,
		%s.priority,
		%s.updateFrequency,
		%s.episodeCount,
		%s.dead,
		%s.lastHttpStatus,
		%s AS podcastGuid`
	args := make([]any, 21)
	for index := range args {
		args[index] = prefix
	}
	args = append(args, source.guidColumn)
	return fmt.Sprintf(selectSQL, args...)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPodcastInfo(scanner rowScanner) (*PodcastInfo, error) {
	var info PodcastInfo
	var title, author, description, coverURL, feedURL, language, websiteURL, newestEnclosureURL, podcastGUID sql.NullString
	var itunesID, newestDuration, lastUpdate, newestItem, oldestItem, popularity, priority, updateFrequency, episodeCount, dead, lastHTTPStatus sql.NullInt64

	if err := scanner.Scan(
		&info.ID,
		&title,
		&author,
		&description,
		&coverURL,
		&feedURL,
		&itunesID,
		&language,
		&websiteURL,
		&newestEnclosureURL,
		&newestDuration,
		&lastUpdate,
		&newestItem,
		&oldestItem,
		&popularity,
		&priority,
		&updateFrequency,
		&episodeCount,
		&dead,
		&lastHTTPStatus,
		&podcastGUID,
	); err != nil {
		return nil, err
	}

	info.Title = title.String
	info.Author = author.String
	info.Description = description.String
	info.CoverURL = coverURL.String
	info.FeedURL = feedURL.String
	info.Language = language.String
	info.WebsiteURL = websiteURL.String
	info.NewestEnclosureURL = newestEnclosureURL.String
	info.PodcastGUID = strings.TrimSpace(podcastGUID.String)
	if itunesID.Valid {
		info.ITunesID = int(itunesID.Int64)
	}
	if newestDuration.Valid {
		info.NewestEnclosureDuration = int(newestDuration.Int64)
	}
	if lastUpdate.Valid {
		info.LastUpdate = lastUpdate.Int64
	}
	if newestItem.Valid {
		info.NewestItemPubdate = newestItem.Int64
	}
	if oldestItem.Valid {
		info.OldestItemPubdate = oldestItem.Int64
	}
	if popularity.Valid {
		info.PopularityScore = int(popularity.Int64)
	}
	if priority.Valid {
		info.Priority = int(priority.Int64)
	}
	if updateFrequency.Valid {
		info.UpdateFrequency = int(updateFrequency.Int64)
	}
	if episodeCount.Valid {
		info.EpisodeCount = int(episodeCount.Int64)
	}
	if dead.Valid {
		info.Dead = int(dead.Int64)
	}
	if lastHTTPStatus.Valid {
		info.LastHTTPStatus = int(lastHTTPStatus.Int64)
	}
	return &info, nil
}

// FindByFeedURL 根据Feed URL查找播客。导入链路保留去重视图语义；批次
// 失败窗口使用下面的 FindByFeedURLContext 原始表快路径。
func (q *Query) FindByFeedURL(feedURL string) (*PodcastInfo, error) {
	source, err := q.sourceForLookup()
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s.url = ? LIMIT 1", podcastInfoSelect(source), source.from, source.alias)
	logger.Infof("  💾 查询 PodcastIndex: %s", feedURL)
	info, err := scanPodcastInfo(q.db.QueryRow(query, feedURL))
	if err == sql.ErrNoRows {
		logger.Infof("  📭 PodcastIndex: 未找到")
		return nil, nil
	}
	if err != nil {
		logger.Infof("  ❌ PodcastIndex查询错误: %v", err)
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}
	logger.Infof("  ✅ PodcastIndex: 找到 - %s", info.Title)
	return info, nil
}

// FindByFeedURLContext is the cancellable, index-friendly URL lookup used by
// failure-window identity resolution. The context covers both schema probing
// and the SQLite query so a large optional dataset cannot hold a workflow
// worker past its live-query budget.
func (q *Query) FindByFeedURLContext(ctx context.Context, feedURL string) (*PodcastInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source, err := q.rawSourceContext(ctx)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s.url = ? LIMIT 1", podcastInfoSelect(source), source.from, source.alias)
	logger.Infof("  💾 查询 PodcastIndex: %s", feedURL)
	info, err := scanPodcastInfo(q.db.QueryRowContext(ctx, query, feedURL))
	if err == sql.ErrNoRows {
		logger.Infof("  📭 PodcastIndex: 未找到")
		return nil, nil
	}
	if err != nil {
		logger.Infof("  ❌ PodcastIndex查询错误: %v", err)
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}
	logger.Infof("  ✅ PodcastIndex: 找到 - %s", info.Title)
	return info, nil
}

// FindByTitle 根据标题精准搜索播客（返回多个结果）
func (q *Query) FindByTitle(title string) ([]*PodcastInfo, error) {
	source, err := q.sourceForLookup()
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s.title = ? LIMIT 10", podcastInfoSelect(source), source.from, source.alias)
	rows, err := q.db.Query(query, title)
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer rows.Close()

	var infos []*PodcastInfo
	for rows.Next() {
		info, err := scanPodcastInfo(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		infos = append(infos, info)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return infos, nil
}

// FindByITunesID 根据iTunes ID查找播客
func (q *Query) FindByITunesID(itunesID int) (*PodcastInfo, error) {
	source, err := q.sourceForLookup()
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT %s FROM %s
		WHERE typeof(%s.itunesId) IN ('integer', 'real')
		  AND CAST(%s.itunesId AS INTEGER) = ?
		LIMIT 1`, podcastInfoSelect(source), source.from, source.alias, source.alias)
	info, err := scanPodcastInfo(q.db.QueryRow(query, itunesID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}
	return info, nil
}

// FindCandidatesByIdentity returns every raw-table candidate matching a
// stable identity. It intentionally does not collapse same-identity rows: a
// caller must see conflicts instead of silently choosing a title-ranked row.
func (q *Query) FindCandidatesByIdentity(itunesID int, podcastGUID string) ([]*PodcastInfo, error) {
	return q.FindCandidatesByIdentityContext(context.Background(), itunesID, podcastGUID)
}

// FindCandidatesByIdentityContext returns identity candidates without the
// CAST/lower/trim expressions that disabled PodcastIndex indexes. Exact
// equality is the fast path; a compatibility fallback keeps older datasets
// with textual IDs usable, still under the caller's context deadline.
func (q *Query) FindCandidatesByIdentityContext(ctx context.Context, itunesID int, podcastGUID string) ([]*PodcastInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source, err := q.rawSourceContext(ctx)
	if err != nil {
		return nil, err
	}
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if itunesID > 0 {
		conditions = append(conditions, "p.itunesId = ?")
		args = append(args, itunesID)
	}
	if guid := strings.ToLower(strings.TrimSpace(podcastGUID)); guid != "" && source.guidColumn != "''" {
		conditions = append(conditions, "p.podcastGuid = ? COLLATE NOCASE")
		args = append(args, guid)
	}
	if len(conditions) == 0 {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT %s FROM %s
		WHERE (%s) AND p.url IS NOT NULL AND p.url <> ''
		ORDER BY p.dead ASC,
		         CASE WHEN p.lastHttpStatus = 200 THEN 0 ELSE 1 END,
		         p.newestItemPubdate DESC,
		         p.episodeCount DESC,
		         p.id ASC`, podcastInfoSelect(source), source.from, strings.Join(conditions, " OR "))
	rows, err := q.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find PodcastIndex identity candidates: %w", err)
	}
	defer rows.Close()

	var infos []*PodcastInfo
	for rows.Next() {
		info, err := scanPodcastInfo(rows)
		if err != nil {
			return nil, fmt.Errorf("scan PodcastIndex identity candidate: %w", err)
		}
		infos = append(infos, info)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read PodcastIndex identity candidates: %w", err)
	}
	if len(infos) > 0 {
		return infos, nil
	}
	// Compatibility fallback for legacy exports that stored numeric IDs as
	// text or padded GUIDs. It is intentionally reached only after the indexed
	// exact path misses and remains cancellable.
	legacyConditions := make([]string, 0, 2)
	legacyArgs := make([]any, 0, 2)
	if itunesID > 0 {
		legacyConditions = append(legacyConditions, "(typeof(p.itunesId) IN ('integer', 'real', 'text') AND CAST(p.itunesId AS INTEGER) = ?)")
		legacyArgs = append(legacyArgs, itunesID)
	}
	if guid := strings.ToLower(strings.TrimSpace(podcastGUID)); guid != "" && source.guidColumn != "''" {
		legacyConditions = append(legacyConditions, "lower(trim(COALESCE(p.podcastGuid, ''))) = ?")
		legacyArgs = append(legacyArgs, guid)
	}
	if len(legacyConditions) == 0 {
		return nil, nil
	}
	legacyQuery := fmt.Sprintf(`SELECT %s FROM %s
		WHERE (%s) AND p.url IS NOT NULL AND p.url <> ''
		ORDER BY p.dead ASC,
		         CASE WHEN p.lastHttpStatus = 200 THEN 0 ELSE 1 END,
		         p.newestItemPubdate DESC,
		         p.episodeCount DESC,
		         p.id ASC`, podcastInfoSelect(source), source.from, strings.Join(legacyConditions, " OR "))
	legacyRows, err := q.db.QueryContext(ctx, legacyQuery, legacyArgs...)
	if err != nil {
		return nil, fmt.Errorf("find legacy PodcastIndex identity candidates: %w", err)
	}
	defer legacyRows.Close()
	for legacyRows.Next() {
		info, scanErr := scanPodcastInfo(legacyRows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan legacy PodcastIndex identity candidate: %w", scanErr)
		}
		infos = append(infos, info)
	}
	if err := legacyRows.Err(); err != nil {
		return nil, fmt.Errorf("read legacy PodcastIndex identity candidates: %w", err)
	}
	return infos, nil
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
