package collection

import (
	"context"
	"errors"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// RefreshChangeSummary 刷新差异预览：新增、移出、重排与推荐语变化。
type RefreshChangeSummary struct {
	AddedCount            int `json:"added_count"`
	RemovedCount          int `json:"removed_count"`
	ReorderedCount        int `json:"reordered_count"`
	RecommendationChanged int `json:"recommendation_changed_count"`
	UnchangedCount        int `json:"unchanged_count"`
}

// HasChanges 报告是否存在任何实际变化；无变化时确认只记录检查时间。
func (s RefreshChangeSummary) HasChanges() bool {
	return s.AddedCount > 0 || s.RemovedCount > 0 || s.ReorderedCount > 0 || s.RecommendationChanged > 0
}

// RefreshPreviewResult 刷新预览：确认应用的是用户实际看过的这份差异。
type RefreshPreviewResult struct {
	PreviewID  string               `json:"preview_id"`
	Collection uint                 `json:"collection_id"`
	BaseRev    int                  `json:"base_revision"`
	SourceURL  string               `json:"source_url"`
	Title      string               `json:"title"`
	Author     string               `json:"author"`
	TotalKnown bool                 `json:"total_known"`
	ReadCount  int                  `json:"read_count"`
	Changes    RefreshChangeSummary `json:"changes"`
	Items      []PreviewItemBrief   `json:"items"`
	Removed    []RemovedItemBrief   `json:"removed"`
}

// RemovedItemBrief 源清单移出的条目摘要；已收录单集及其个人数据不受影响。
type RemovedItemBrief struct {
	Position          int    `json:"position"`
	ExternalEpisodeID string `json:"external_episode_id"`
	EpisodeTitle      string `json:"episode_title"`
	PodcastTitle      string `json:"podcast_title"`
	Adopted           bool   `json:"adopted"`
}

// RefreshPreview 重新读取源清单并生成差异预览。网络读取在事务外完成；
// 本地清单在确认前保持不变。
func (s *Service) RefreshPreview(ctx context.Context, collectionID uint) (*RefreshPreviewResult, error) {
	var collection models.EpisodeCollection
	if err := s.db.First(&collection, collectionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCollectionNotFound
		}
		return nil, err
	}

	body, err := s.fetch(ctx, collection.SourceURL)
	if err != nil {
		return nil, err
	}
	draft, err := s.parse(string(body), collection.ExternalID)
	if err != nil {
		return nil, err
	}
	draft.SourceURL = collection.SourceURL

	var stored []models.EpisodeCollectionItem
	if err := s.db.Where("collection_id = ?", collection.ID).Order("position ASC, id ASC").Find(&stored).Error; err != nil {
		return nil, err
	}
	storedByEID := make(map[string]models.EpisodeCollectionItem, len(stored))
	for _, item := range stored {
		storedByEID[item.ExternalEpisodeID] = item
	}
	newByEID := make(map[string]ItemDraft, len(draft.Items))
	for _, item := range draft.Items {
		newByEID[item.ExternalEpisodeID] = item
	}

	changes := RefreshChangeSummary{}
	added := make([]PreviewItemBrief, 0)
	kept := make([]PreviewItemBrief, 0, len(draft.Items))
	for index, item := range draft.Items {
		brief := PreviewItemBrief{
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
		}
		previous, exists := storedByEID[item.ExternalEpisodeID]
		if !exists {
			changes.AddedCount++
			added = append(added, brief)
			continue
		}
		if previous.Position != index {
			changes.ReorderedCount++
		}
		if previous.Recommendation != item.Recommendation {
			changes.RecommendationChanged++
		}
		if previous.Position == index && previous.Recommendation == item.Recommendation {
			changes.UnchangedCount++
		}
		kept = append(kept, brief)
	}
	removed := make([]RemovedItemBrief, 0)
	for _, item := range stored {
		if _, exists := newByEID[item.ExternalEpisodeID]; exists {
			continue
		}
		changes.RemovedCount++
		removed = append(removed, RemovedItemBrief{
			Position:          item.Position,
			ExternalEpisodeID: item.ExternalEpisodeID,
			EpisodeTitle:      item.EpisodeTitle,
			PodcastTitle:      item.PodcastTitle,
			Adopted:           item.EpisodeID != nil,
		})
	}

	token, err := s.refreshes.put(collection.ID, collection.Revision, draft)
	if err != nil {
		return nil, err
	}
	items := append(added, kept...)
	return &RefreshPreviewResult{
		PreviewID:  token,
		Collection: collection.ID,
		BaseRev:    collection.Revision,
		SourceURL:  collection.SourceURL,
		Title:      draft.Title,
		Author:     draft.Author,
		TotalKnown: draft.TotalKnown,
		ReadCount:  len(draft.Items),
		Changes:    changes,
		Items:      items,
		Removed:    removed,
	}, nil
}

// ApplyRefreshResult 确认刷新后的结果摘要。
type ApplyRefreshResult struct {
	Applied          bool `json:"applied"`
	NoChanges        bool `json:"no_changes"`
	Revision         int  `json:"revision"`
	ItemCount        int  `json:"item_count"`
	AdoptedKeptCount int  `json:"adopted_kept_count"`
	RemovedAdopted   int  `json:"removed_adopted_count"`
}

// ApplyRefresh 在同一事务中原子应用用户确认的那份刷新：保留已收录单集的
// 本地关联（被移出的条目只解除清单关联，个人单集与采纳摘要不受影响）；
// 新增条目保持未采纳。修订号校验防止多个页面相互覆盖。
func (s *Service) ApplyRefresh(collectionID uint, previewID string, expectedRevision int) (*ApplyRefreshResult, error) {
	draft, err := s.refreshes.take(previewID, collectionID, expectedRevision)
	if err != nil {
		return nil, err
	}

	var result *ApplyRefreshResult
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var collection models.EpisodeCollection
		if err := tx.First(&collection, collectionID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCollectionNotFound
			}
			return err
		}
		if collection.Revision != expectedRevision {
			return ErrRefreshConflict
		}

		var stored []models.EpisodeCollectionItem
		if err := tx.Where("collection_id = ?", collection.ID).Find(&stored).Error; err != nil {
			return err
		}
		adoptedByEID := make(map[string]uint, len(stored))
		for _, item := range stored {
			if item.EpisodeID != nil {
				adoptedByEID[item.ExternalEpisodeID] = *item.EpisodeID
			}
		}

		// 差异为空：只记录检查结果，不递增修订。
		changes := computeChanges(stored, draft)
		if !changes.HasChanges() {
			now := s.now().UTC()
			if err := tx.Model(&models.EpisodeCollection{}).Where("id = ?", collection.ID).
				Update("last_refreshed_at", now).Error; err != nil {
				return err
			}
			result = &ApplyRefreshResult{
				Applied: true, NoChanges: true,
				Revision: collection.Revision, ItemCount: len(stored),
				AdoptedKeptCount: len(adoptedByEID),
			}
			return nil
		}

		// 原子替换条目：已收录单集的关联随同 eid 保留；被移出条目的关联随
		// 条目删除解除，本地单集、队列与采纳摘要不受影响。
		if err := tx.Where("collection_id = ?", collection.ID).
			Unscoped().Delete(&models.EpisodeCollectionItem{}).Error; err != nil {
			return err
		}
		adoptedKept := 0
		for index, item := range draft.Items {
			record := models.EpisodeCollectionItem{
				CollectionID:        collection.ID,
				Position:            index,
				ExternalEpisodeID:   item.ExternalEpisodeID,
				ExternalPodcastID:   item.ExternalPodcastID,
				PodcastTitle:        item.PodcastTitle,
				PodcastAuthor:       item.PodcastAuthor,
				PodcastCoverURL:     item.PodcastCoverURL,
				PodcastEpisodeCount: item.PodcastEpisodeCount,
				EpisodeTitle:        item.EpisodeTitle,
				Recommendation:      item.Recommendation,
				Shownotes:           item.Shownotes,
				Duration:            item.Duration,
				PublishedAt:         item.PublishedAt,
				ImageURL:            item.ImageURL,
				EpisodeURL:          item.EpisodeURL,
				PayType:             item.PayType,
				IsPrivateMedia:      item.IsPrivateMedia,
				AudioURL:            item.AudioURL,
				AudioMimeType:       item.AudioMimeType,
				AudioSize:           item.AudioSize,
			}
			if episodeID, adopted := adoptedByEID[item.ExternalEpisodeID]; adopted {
				id := episodeID
				record.EpisodeID = &id
				adoptedKept++
			}
			if err := tx.Create(&record).Error; err != nil {
				return err
			}
		}

		now := s.now().UTC()
		if err := tx.Model(&models.EpisodeCollection{}).Where("id = ?", collection.ID).
			Updates(map[string]any{
				"title":             draft.Title,
				"description":       draft.Description,
				"author":            draft.Author,
				"total_known":       draft.TotalKnown,
				"revision":          collection.Revision + 1,
				"last_refreshed_at": now,
			}).Error; err != nil {
			return err
		}

		removedAdopted := 0
		for _, item := range stored {
			if _, exists := adoptedByEID[item.ExternalEpisodeID]; !exists {
				continue
			}
			if !draftContains(draft, item.ExternalEpisodeID) {
				removedAdopted++
			}
		}
		result = &ApplyRefreshResult{
			Applied: true, NoChanges: false,
			Revision: collection.Revision + 1, ItemCount: len(draft.Items),
			AdoptedKeptCount: adoptedKept, RemovedAdopted: removedAdopted,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func newByEID(draft *Draft, eid string) bool {
	for _, item := range draft.Items {
		if item.ExternalEpisodeID == eid {
			return true
		}
	}
	return false
}

func computeChanges(stored []models.EpisodeCollectionItem, draft *Draft) RefreshChangeSummary {
	storedByEID := make(map[string]models.EpisodeCollectionItem, len(stored))
	for _, item := range stored {
		storedByEID[item.ExternalEpisodeID] = item
	}
	changes := RefreshChangeSummary{}
	for index, item := range draft.Items {
		previous, exists := storedByEID[item.ExternalEpisodeID]
		if !exists {
			changes.AddedCount++
			continue
		}
		if previous.Position != index {
			changes.ReorderedCount++
		} else if previous.Recommendation == item.Recommendation {
			changes.UnchangedCount++
		}
		if previous.Recommendation != item.Recommendation {
			changes.RecommendationChanged++
		}
	}
	for _, item := range stored {
		if !draftContains(draft, item.ExternalEpisodeID) {
			changes.RemovedCount++
		}
	}
	return changes
}

func draftContains(draft *Draft, eid string) bool {
	for _, item := range draft.Items {
		if item.ExternalEpisodeID == eid {
			return true
		}
	}
	return false
}

// DeleteCollectionResult 删除清单的结果摘要。
type DeleteCollectionResult struct {
	Deleted         bool `json:"deleted"`
	ItemCount       int  `json:"item_count"`
	AdoptedDetached int  `json:"adopted_detached_count"`
}

// DeleteCollection 删除清单及其未采纳资料。已收录单集、队列、笔记与采纳
// 来源摘要全部保留；清单条目随清单一起移除，已收录条目只解除本地关联。
func (s *Service) DeleteCollection(collectionID uint) (*DeleteCollectionResult, error) {
	var result *DeleteCollectionResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var collection models.EpisodeCollection
		if err := tx.First(&collection, collectionID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCollectionNotFound
			}
			return err
		}

		var items []models.EpisodeCollectionItem
		if err := tx.Where("collection_id = ?", collection.ID).Find(&items).Error; err != nil {
			return err
		}
		adoptedDetached := 0
		for _, item := range items {
			if item.EpisodeID != nil {
				adoptedDetached++
			}
		}
		// 已收录单集不被删除：只随条目移除清单关联；episode 的行动队列、
		// 笔记和加工产物在各自账本中保持不变。
		if err := tx.Where("collection_id = ?", collection.ID).
			Unscoped().Delete(&models.EpisodeCollectionItem{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Delete(&models.EpisodeCollection{}, collection.ID).Error; err != nil {
			return err
		}
		// 采纳来源摘要（episode_collection_adoptions）独立于清单，保留。
		result = &DeleteCollectionResult{
			Deleted: true, ItemCount: len(items), AdoptedDetached: adoptedDetached,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
