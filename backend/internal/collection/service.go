package collection

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// 预览与导入的生命周期错误。
var (
	ErrPreviewNotFound    = errors.New("collection preview not found")
	ErrPreviewExpired     = errors.New("collection preview expired")
	ErrCollectionNotFound = errors.New("collection not found")
)

const (
	// previewTTL 预览在服务端短期保存；过期后要求重新读取，确认保存的始终
	// 是用户实际看过的那一份。
	previewTTL = 30 * time.Minute
)

// previewEntry 是服务端绑定的待提交清单版本。
type previewEntry struct {
	draft     *Draft
	expiresAt time.Time
}

// previewStore 保存待提交预览。单用户单进程实例使用进程内存储即可，
// 不为放弃的预览写入数据库。
type previewStore struct {
	mu      sync.Mutex
	entries map[string]previewEntry
	now     func() time.Time
}

func newPreviewStore(now func() time.Time) *previewStore {
	return &previewStore{
		entries: make(map[string]previewEntry),
		now:     now,
	}
}

func (s *previewStore) put(draft *Draft) (string, error) {
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate preview token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked()
	s.entries[token] = previewEntry{draft: draft, expiresAt: s.now().Add(previewTTL)}
	return token, nil
}

// take 取出并删除预览；确认导入单次有效，避免同一预览被重复提交成多份。
func (s *previewStore) take(token string) (*Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[token]
	if !exists {
		return nil, ErrPreviewNotFound
	}
	delete(s.entries, token)
	if s.now().After(entry.expiresAt) {
		return nil, ErrPreviewExpired
	}
	return entry.draft, nil
}

func (s *previewStore) evictExpiredLocked() {
	now := s.now()
	for token, entry := range s.entries {
		if now.After(entry.expiresAt) {
			delete(s.entries, token)
		}
	}
}

// Service 清单导入与浏览的领域服务。网络读取全部在事务外完成。
type Service struct {
	db        *gorm.DB
	sourceURL func(rawURL string) (*CollectionURL, error)
	fetch     fetchFunc
	parse     func(html string, expectedExternalID string) (*Draft, error)
	previews  *previewStore
	now       func() time.Time
}

// NewService 创建清单服务；生产抓取器带完整安全边界。
func NewService(db *gorm.DB) *Service {
	now := func() time.Time { return time.Now().UTC() }
	return &Service{
		db:        db,
		sourceURL: ParseCollectionURL,
		fetch:     newProductionFetcher(),
		parse:     ParsePageHTML,
		previews:  newPreviewStore(now),
		now:       now,
	}
}

// SourceFetcher 是清单页读取的可控入口；生产实现自带私网隔离与逐跳校验，
// 测试注入可控来源响应，避免真实网络。
type SourceFetcher interface {
	FetchCollectionPage(ctx context.Context, pageURL string) ([]byte, error)
}

// NewServiceWithFetcher 以指定来源读取器创建清单服务（仅测试与验收使用）。
func NewServiceWithFetcher(db *gorm.DB, fetcher SourceFetcher) *Service {
	service := NewService(db)
	if fetcher != nil {
		service.fetch = fetcher.FetchCollectionPage
	}
	return service
}

// PreviewResult 返回给前端的预览内容；预览不含 Show Notes 正文，保持响应轻量。
type PreviewResult struct {
	PreviewID   string             `json:"preview_id"`
	Platform    string             `json:"platform"`
	ExternalID  string             `json:"external_id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Author      string             `json:"author"`
	SourceURL   string             `json:"source_url"`
	TotalKnown  bool               `json:"total_known"`
	ReadCount   int                `json:"read_count"`
	Items       []PreviewItemBrief `json:"items"`
}

// PreviewItemBrief 预览条目：标题、节目与原始推荐语足以确认导入对象。
type PreviewItemBrief struct {
	Position          int        `json:"position"`
	ExternalEpisodeID string     `json:"external_episode_id"`
	EpisodeTitle      string     `json:"episode_title"`
	PodcastTitle      string     `json:"podcast_title"`
	PodcastAuthor     string     `json:"podcast_author"`
	Recommendation    string     `json:"recommendation"`
	Duration          int        `json:"duration"`
	PublishedAt       *time.Time `json:"published_at"`
	EpisodeURL        string     `json:"episode_url"`
	PayType           string     `json:"pay_type"`
	IsPrivateMedia    bool       `json:"is_private_media"`
}

// Preview 读取并解析清单，返回服务端绑定的预览。只读取清单本身，
// 不抓取整档节目、不下载音频、不调用模型。
func (s *Service) Preview(ctx context.Context, rawURL string) (*PreviewResult, error) {
	collectionURL, err := s.sourceURL(rawURL)
	if err != nil {
		return nil, err
	}
	body, err := s.fetch(ctx, collectionURL.Raw)
	if err != nil {
		return nil, err
	}
	draft, err := s.parse(string(body), collectionURL.ExternalID)
	if err != nil {
		return nil, err
	}
	draft.SourceURL = collectionURL.Raw

	token, err := s.previews.put(draft)
	if err != nil {
		return nil, err
	}
	return previewResultFromDraft(draft, token), nil
}

func previewResultFromDraft(draft *Draft, previewID string) *PreviewResult {
	items := make([]PreviewItemBrief, 0, len(draft.Items))
	for index, item := range draft.Items {
		items = append(items, PreviewItemBrief{
			Position:          index,
			ExternalEpisodeID: item.ExternalEpisodeID,
			EpisodeTitle:      item.EpisodeTitle,
			PodcastTitle:      item.PodcastTitle,
			PodcastAuthor:     item.PodcastAuthor,
			Recommendation:    item.Recommendation,
			Duration:          item.Duration,
			PublishedAt:       item.PublishedAt,
			EpisodeURL:        item.EpisodeURL,
			PayType:           item.PayType,
			IsPrivateMedia:    item.IsPrivateMedia,
		})
	}
	return &PreviewResult{
		PreviewID:   previewID,
		Platform:    draft.Platform,
		ExternalID:  draft.ExternalID,
		Title:       draft.Title,
		Description: draft.Description,
		Author:      draft.Author,
		SourceURL:   draft.SourceURL,
		TotalKnown:  draft.TotalKnown,
		ReadCount:   len(draft.Items),
		Items:       items,
	}
}

// ImportResult 描述确认导入的结果；重复导入返回已有清单身份，不新建副本。
type ImportResult struct {
	Duplicate    bool `json:"duplicate"`
	CollectionID uint `json:"collection_id"`
}

// ConfirmImport 保存用户实际预览过的那份清单版本。清单与条目在同一事务中
// 原子写入；重复的源平台+清单 ID 复用已有清单，不覆盖个人选择。
// 导入只写发现资料，不写个人节目、单集、订阅或行动队列。
func (s *Service) ConfirmImport(previewID string) (*ImportResult, error) {
	draft, err := s.previews.take(previewID)
	if err != nil {
		return nil, err
	}

	var result *ImportResult
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var existing models.EpisodeCollection
		err := tx.Where("source_platform = ? AND external_id = ?", draft.Platform, draft.ExternalID).
			First(&existing).Error
		if err == nil {
			// 重复导入：打开已有清单，不创建副本，也不隐式刷新覆盖。
			result = &ImportResult{Duplicate: true, CollectionID: existing.ID}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		now := s.now().UTC()
		collection := models.EpisodeCollection{
			SourcePlatform:  draft.Platform,
			ExternalID:      draft.ExternalID,
			Title:           draft.Title,
			Description:     draft.Description,
			Author:          draft.Author,
			SourceURL:       draft.SourceURL,
			TotalKnown:      draft.TotalKnown,
			Revision:        1,
			LastRefreshedAt: &now,
		}
		if err := tx.Create(&collection).Error; err != nil {
			return err
		}
		for index, item := range draft.Items {
			record := models.EpisodeCollectionItem{
				CollectionID:      collection.ID,
				Position:          index,
				ExternalEpisodeID: item.ExternalEpisodeID,
				ExternalPodcastID: item.ExternalPodcastID,
				PodcastTitle:      item.PodcastTitle,
				PodcastAuthor:     item.PodcastAuthor,
				PodcastCoverURL:   item.PodcastCoverURL,
				EpisodeTitle:      item.EpisodeTitle,
				Recommendation:    item.Recommendation,
				Shownotes:         item.Shownotes,
				Duration:          item.Duration,
				PublishedAt:       item.PublishedAt,
				ImageURL:          item.ImageURL,
				EpisodeURL:        item.EpisodeURL,
				PayType:           item.PayType,
				IsPrivateMedia:    item.IsPrivateMedia,
			}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
		}
		result = &ImportResult{Duplicate: false, CollectionID: collection.ID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CollectionSummary 清单列表条目：主题、来源、作者与真实收录计数。
type CollectionSummary struct {
	ID              uint       `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	Author          string     `json:"author"`
	Platform        string     `json:"platform"`
	ExternalID      string     `json:"external_id"`
	SourceURL       string     `json:"source_url"`
	TotalKnown      bool       `json:"total_known"`
	ItemCount       int64      `json:"item_count"`
	AdoptedCount    int64      `json:"adopted_count"`
	CreatedAt       time.Time  `json:"created_at"`
	LastRefreshedAt *time.Time `json:"last_refreshed_at"`
}

// ListCollections 列出已保存清单；search 非空时按清单标题过滤。
func (s *Service) ListCollections(search string) ([]CollectionSummary, error) {
	var collections []models.EpisodeCollection
	query := s.db.Order("created_at DESC, id DESC")
	if trimmed := strings.TrimSpace(search); trimmed != "" {
		query = query.Where("title LIKE ?", "%"+escapeLike(trimmed)+"%")
	}
	if err := query.Find(&collections).Error; err != nil {
		return nil, err
	}

	summaries := make([]CollectionSummary, 0, len(collections))
	for _, collection := range collections {
		itemCount, adoptedCount, err := s.collectionCounts(s.db, collection.ID)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, CollectionSummary{
			ID:              collection.ID,
			Title:           collection.Title,
			Description:     collection.Description,
			Author:          collection.Author,
			Platform:        collection.SourcePlatform,
			ExternalID:      collection.ExternalID,
			SourceURL:       collection.SourceURL,
			TotalKnown:      collection.TotalKnown,
			ItemCount:       itemCount,
			AdoptedCount:    adoptedCount,
			CreatedAt:       collection.CreatedAt,
			LastRefreshedAt: collection.LastRefreshedAt,
		})
	}
	return summaries, nil
}

// CollectionItemDetail 清单详情条目，含真实收录状态。
type CollectionItemDetail struct {
	ID                  uint       `json:"id"`
	Position            int        `json:"position"`
	ExternalEpisodeID   string     `json:"external_episode_id"`
	ExternalPodcastID   string     `json:"external_podcast_id"`
	PodcastTitle        string     `json:"podcast_title"`
	PodcastAuthor       string     `json:"podcast_author"`
	PodcastCoverURL     string     `json:"podcast_cover_url"`
	EpisodeTitle        string     `json:"episode_title"`
	Recommendation      string     `json:"recommendation"`
	Shownotes           string     `json:"shownotes"`
	Duration            int        `json:"duration"`
	PublishedAt         *time.Time `json:"published_at"`
	ImageURL            string     `json:"image_url"`
	EpisodeURL          string     `json:"episode_url"`
	PayType             string     `json:"pay_type"`
	IsPrivateMedia      bool       `json:"is_private_media"`
	AdoptedEpisodeID    *uint      `json:"adopted_episode_id"`
	AdoptedEpisodeTitle string     `json:"adopted_episode_title"`
	AdoptedEpisodeQueue *string    `json:"adopted_episode_queue"`
}

// CollectionDetail 清单详情：原始顺序、推荐语、Show Notes 与真实收录状态。
type CollectionDetail struct {
	ID              uint                   `json:"id"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	Author          string                 `json:"author"`
	Platform        string                 `json:"platform"`
	ExternalID      string                 `json:"external_id"`
	SourceURL       string                 `json:"source_url"`
	TotalKnown      bool                   `json:"total_known"`
	Revision        int                    `json:"revision"`
	ItemCount       int64                  `json:"item_count"`
	AdoptedCount    int64                  `json:"adopted_count"`
	CreatedAt       time.Time              `json:"created_at"`
	LastRefreshedAt *time.Time             `json:"last_refreshed_at"`
	Items           []CollectionItemDetail `json:"items"`
}

// GetCollection 按 ID 读取清单详情；地址直达与刷新不写库、不触发加工。
func (s *Service) GetCollection(id uint) (*CollectionDetail, error) {
	var collection models.EpisodeCollection
	if err := s.db.First(&collection, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCollectionNotFound
		}
		return nil, err
	}

	var items []models.EpisodeCollectionItem
	if err := s.db.Where("collection_id = ?", collection.ID).
		Order("position ASC, id ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}

	itemCount, adoptedCount, err := s.collectionCounts(s.db, collection.ID)
	if err != nil {
		return nil, err
	}

	detail := &CollectionDetail{
		ID:              collection.ID,
		Title:           collection.Title,
		Description:     collection.Description,
		Author:          collection.Author,
		Platform:        collection.SourcePlatform,
		ExternalID:      collection.ExternalID,
		SourceURL:       collection.SourceURL,
		TotalKnown:      collection.TotalKnown,
		Revision:        collection.Revision,
		ItemCount:       itemCount,
		AdoptedCount:    adoptedCount,
		CreatedAt:       collection.CreatedAt,
		LastRefreshedAt: collection.LastRefreshedAt,
		Items:           make([]CollectionItemDetail, 0, len(items)),
	}
	episodeIDs := make([]uint, 0, len(items))
	for _, item := range items {
		if item.EpisodeID != nil {
			episodeIDs = append(episodeIDs, *item.EpisodeID)
		}
	}
	// 已收录条目的真实状态来自既有本地单集与行动队列账本；这里只读展示。
	queueByEpisode, titleByEpisode, err := s.adoptedEpisodeFacts(s.db, episodeIDs)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		itemDetail := CollectionItemDetail{
			ID:                item.ID,
			Position:          item.Position,
			ExternalEpisodeID: item.ExternalEpisodeID,
			ExternalPodcastID: item.ExternalPodcastID,
			PodcastTitle:      item.PodcastTitle,
			PodcastAuthor:     item.PodcastAuthor,
			PodcastCoverURL:   item.PodcastCoverURL,
			EpisodeTitle:      item.EpisodeTitle,
			Recommendation:    item.Recommendation,
			Shownotes:         item.Shownotes,
			Duration:          item.Duration,
			PublishedAt:       item.PublishedAt,
			ImageURL:          item.ImageURL,
			EpisodeURL:        item.EpisodeURL,
			PayType:           item.PayType,
			IsPrivateMedia:    item.IsPrivateMedia,
			AdoptedEpisodeID:  item.EpisodeID,
		}
		if item.EpisodeID != nil {
			itemDetail.AdoptedEpisodeTitle = titleByEpisode[*item.EpisodeID]
			if queue, exists := queueByEpisode[*item.EpisodeID]; exists {
				itemDetail.AdoptedEpisodeQueue = &queue
			}
		}
		detail.Items = append(detail.Items, itemDetail)
	}
	return detail, nil
}

func (s *Service) collectionCounts(db *gorm.DB, collectionID uint) (int64, int64, error) {
	var itemCount, adoptedCount int64
	if err := db.Model(&models.EpisodeCollectionItem{}).
		Where("collection_id = ?", collectionID).
		Count(&itemCount).Error; err != nil {
		return 0, 0, err
	}
	if err := db.Model(&models.EpisodeCollectionItem{}).
		Where("collection_id = ? AND episode_id IS NOT NULL", collectionID).
		Count(&adoptedCount).Error; err != nil {
		return 0, 0, err
	}
	return itemCount, adoptedCount, nil
}

func (s *Service) adoptedEpisodeFacts(db *gorm.DB, episodeIDs []uint) (map[uint]string, map[uint]string, error) {
	queueByEpisode := make(map[uint]string, len(episodeIDs))
	titleByEpisode := make(map[uint]string, len(episodeIDs))
	if len(episodeIDs) == 0 {
		return queueByEpisode, titleByEpisode, nil
	}
	var decisions []models.EpisodeTriageDecision
	if err := db.Where("episode_id IN ?", episodeIDs).Find(&decisions).Error; err != nil {
		return nil, nil, err
	}
	for _, decision := range decisions {
		if decision.QueueState != nil {
			queueByEpisode[decision.EpisodeID] = *decision.QueueState
		}
	}
	var episodes []models.Episode
	if err := db.Select("id", "title").Where("id IN ?", episodeIDs).Find(&episodes).Error; err != nil {
		return nil, nil, err
	}
	for _, episode := range episodes {
		titleByEpisode[episode.ID] = episode.Title
	}
	return queueByEpisode, titleByEpisode, nil
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}
