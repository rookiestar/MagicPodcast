package services

import (
	"errors"
	"fmt"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// 采纳身份解析失败分类：调用方据此给出可区分的用户提示，绝不自动猜测合并。
var (
	ErrAdoptionItemNotFound    = errors.New("collection item not found")
	ErrAdoptionEpisodeDeleted  = errors.New("episode is soft deleted")
	ErrAdoptionCrossPodcast    = errors.New("episode identity belongs to another podcast")
	ErrAdoptionAmbiguous       = errors.New("multiple candidate episodes match the identity")
	ErrAdoptionIdentityInvalid = errors.New("external identity conflicts with recorded mapping")
)

// CollectionAdoptionService 把清单条目原子收录为个人单集并入队。
// 身份解析严格：仅复用已验证外部身份或原始单集链接；不按标题/宽松日期猜合并。
type CollectionAdoptionService struct {
	db  *gorm.DB
	now func() time.Time
}

// NewCollectionAdoptionService 创建采纳服务。
func NewCollectionAdoptionService(db *gorm.DB) *CollectionAdoptionService {
	return &CollectionAdoptionService{
		db:  db,
		now: func() time.Time { return time.Now().UTC() },
	}
}

// AdoptResult 收录结果：返回收录后的真实状态，重复提交复用既有结果。
type AdoptResult struct {
	ItemID            uint       `json:"item_id"`
	EpisodeID         uint       `json:"episode_id"`
	QueueState        *string    `json:"queue_state"`
	DismissedAt       *time.Time `json:"dismissed_at,omitempty"`
	EpisodeCreated    bool       `json:"episode_created"`
	PodcastCreated    bool       `json:"podcast_created"`
	PodcastID         uint       `json:"podcast_id"`
	PodcastTitle      string     `json:"podcast_title"`
	PodcastSubscribed bool       `json:"podcast_subscribed"`
	// CollectionOnly 表示该集仍只是清单独有、未被普通同步识别的单集。
	CollectionOnly bool `json:"collection_only"`
	// AudioAvailable 表示快照中存在公开音频地址；私密/付费为 false。
	AudioAvailable bool `json:"audio_available"`
	// InboxWritten 说明本次请求把该集放入了 Inbox；false 表示保留既有状态。
	InboxWritten bool `json:"inbox_written"`
}

// externalGUID 命名空间外部键：仅作为暂存 GUID，后续 RSS 同步必须先查
// 外部映射再决定创建，不能按 GUID 前缀做去重。
func externalGUID(platform, externalEpisodeID string) string {
	return platform + ":episode:" + externalEpisodeID
}

// AdoptCollectionItem 收录一条清单条目：网络读取不发生；单集复用/创建、
// 外部映射、清单关联、来源摘要与 Inbox 写入在同一事务中完成。
func (s *CollectionAdoptionService) AdoptCollectionItem(collectionID, itemID uint) (*AdoptResult, error) {
	var collection models.EpisodeCollection
	if err := s.db.First(&collection, collectionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdoptionItemNotFound
		}
		return nil, err
	}
	var item models.EpisodeCollectionItem
	if err := s.db.Where("id = ? AND collection_id = ?", itemID, collectionID).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdoptionItemNotFound
		}
		return nil, err
	}

	var result *AdoptResult
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 事务内重读条目；并发/重复提交复用既有收录结果。
		fresh := models.EpisodeCollectionItem{}
		if err := tx.Where("id = ? AND collection_id = ?", itemID, collectionID).First(&fresh).Error; err != nil {
			return err
		}
		item = fresh

		episode, episodeCreated, podcastCreated, err := s.resolveOrReuseEpisode(tx, &item)
		if err != nil {
			return err
		}

		if err := s.recordExternalRef(tx, &item, episode.ID, episode.PodcastID); err != nil {
			return err
		}
		if err := tx.Model(&models.EpisodeCollectionItem{}).Where("id = ?", item.ID).
			Update("episode_id", episode.ID).Error; err != nil {
			return err
		}
		if err := s.recordAdoptionSummary(tx, &collection, &item, episode.ID); err != nil {
			return err
		}

		inboxWritten, err := s.enqueueInbox(tx, episode.ID)
		if err != nil {
			return err
		}

		// 回读验证节目关注状态与单集标识，不直接照搬写入值。
		var podcast models.Podcast
		if err := tx.Select("id", "title", "is_subscribed").First(&podcast, episode.PodcastID).Error; err != nil {
			return err
		}
		var episodeState models.Episode
		if err := tx.Select("id", "collection_only").First(&episodeState, episode.ID).Error; err != nil {
			return err
		}

		result = &AdoptResult{
			ItemID:            item.ID,
			EpisodeID:         episode.ID,
			EpisodeCreated:    episodeCreated,
			PodcastCreated:    podcastCreated,
			PodcastID:         podcast.ID,
			PodcastTitle:      podcast.Title,
			PodcastSubscribed: podcast.IsSubscribed,
			CollectionOnly:    episodeState.CollectionOnly,
			AudioAvailable:    episode.MediumURL != "",
			InboxWritten:      inboxWritten,
		}
		// 回读真实队列状态，避免把“已在 Focus/Done”误报成本次写入 Inbox。
		var decision models.EpisodeTriageDecision
		if err := tx.Where("episode_id = ?", episode.ID).First(&decision).Error; err == nil {
			result.QueueState = decision.QueueState
			result.DismissedAt = decision.DismissedAt
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 收录改变节目/单集汇总：失效播客列表与相关详情缓存，避免列表回读陈旧数据。
	cache.InvalidatePodcastList()
	cache.InvalidatePodcastDetail(result.PodcastID)
	return result, nil
}

// resolveOrReuseEpisode 依次复用已验证外部身份、原始单集链接；多候选或跨节目
// 冲突时拒绝自动合并。软删除单集不复活。无匹配时按需建立节目归属并创建新单集。
func (s *CollectionAdoptionService) resolveOrReuseEpisode(
	tx *gorm.DB,
	item *models.EpisodeCollectionItem,
) (*models.Episode, bool, bool, error) {
	candidates := make(map[uint]struct{})
	softDeleted := false

	var refs []models.EpisodeExternalRef
	if err := tx.Where("source_platform = ? AND external_episode_id = ?", models.SourcePlatformXiaoyuzhoufm, item.ExternalEpisodeID).
		Find(&refs).Error; err != nil {
		return nil, false, false, err
	}
	for _, ref := range refs {
		if ref.ExternalPodcastID != "" && item.ExternalPodcastID != "" && ref.ExternalPodcastID != item.ExternalPodcastID {
			return nil, false, false, fmt.Errorf("%w: recorded podcast %q != item podcast %q",
				ErrAdoptionIdentityInvalid, ref.ExternalPodcastID, item.ExternalPodcastID)
		}
		candidates[ref.EpisodeID] = struct{}{}
	}

	if item.EpisodeURL != "" {
		// Unscoped 包含软删除行：检测到被个人库删除的同链接单集时，
		// 采纳必须停止并提示，而不是绕开软删除账本另建一份。
		var byLink []models.Episode
		if err := tx.Unscoped().Where("link = ?", item.EpisodeURL).Find(&byLink).Error; err != nil {
			return nil, false, false, err
		}
		for _, episode := range byLink {
			if episode.DeletedAt.Valid {
				softDeleted = true
				continue
			}
			candidates[episode.ID] = struct{}{}
		}
	}

	if item.ExternalPodcastID == "" {
		return nil, false, false, ErrAdoptionIdentityInvalid
	}
	var byGUID []models.Episode
	if err := tx.Unscoped().Model(&models.Episode{}).Select("episodes.*").
		Joins("JOIN podcasts ON podcasts.id = episodes.podcast_id AND podcasts.deleted_at IS NULL").
		Where("podcasts.xyz_id = ? AND episodes.guid IN ?", item.ExternalPodcastID, []string{item.ExternalEpisodeID, externalGUID(models.SourcePlatformXiaoyuzhoufm, item.ExternalEpisodeID)}).
		Find(&byGUID).Error; err != nil {
		return nil, false, false, err
	}
	for _, existing := range byGUID {
		if existing.DeletedAt.Valid {
			softDeleted = true
		} else {
			candidates[existing.ID] = struct{}{}
		}
	}

	if len(candidates) > 1 {
		return nil, false, false, fmt.Errorf("%w: %d candidate episodes", ErrAdoptionAmbiguous, len(candidates))
	}
	if len(candidates) == 1 {
		for episodeID := range candidates {
			var episode models.Episode
			if err := tx.Unscoped().First(&episode, episodeID).Error; err != nil {
				return nil, false, false, err
			}
			if episode.DeletedAt.Valid {
				return nil, false, false, ErrAdoptionEpisodeDeleted
			}
			// 跨节目冲突：已有单集所属节目与清单条目指向的节目不一致时，
			// 停止该条采纳并明确提示，不扩展成全库合并工具。
			if item.ExternalPodcastID != "" {
				var podcast models.Podcast
				if err := tx.First(&podcast, episode.PodcastID).Error; err != nil {
					return nil, false, false, err
				}
				if podcast.XYZID != item.ExternalPodcastID {
					return nil, false, false, fmt.Errorf("%w: episode %d belongs to podcast %q (xyz %q), item podcast %q",
						ErrAdoptionCrossPodcast, episode.ID, podcast.Title, podcast.XYZID, item.ExternalPodcastID)
				}
			}
			// 复用既有单集：不改写任何字段，不刷新同步时间，不改变资格标识。
			return &episode, false, false, nil
		}
	}

	if softDeleted {
		return nil, false, false, ErrAdoptionEpisodeDeleted
	}
	if item.EpisodeID != nil {
		// 条目仍指向已不存在的本地单集属于异常状态（正常删除会置空关联）。
		var count int64
		if err := tx.Model(&models.Episode{}).Where("id = ?", *item.EpisodeID).Count(&count).Error; err != nil {
			return nil, false, false, err
		}
		if count == 0 {
			return nil, false, false, ErrAdoptionEpisodeDeleted
		}
	}

	podcast, podcastCreated, err := s.resolveOrCreatePodcast(tx, item)
	if err != nil {
		return nil, false, false, err
	}
	now := s.now().UTC()
	episode := &models.Episode{
		PodcastID:       podcast.ID,
		Title:           item.EpisodeTitle,
		ShowNotes:       item.Shownotes,
		Duration:        item.Duration,
		Link:            item.EpisodeURL,
		ImageURL:        item.ImageURL,
		MediumURL:       item.AudioURL,
		EnclosureType:   item.AudioMimeType,
		EnclosureLength: item.AudioSize,
		GUID:            externalGUID(models.SourcePlatformXiaoyuzhoufm, item.ExternalEpisodeID),
		CollectionOnly:  true,
	}
	if item.PublishedAt != nil {
		episode.PublishedDate = *item.PublishedAt
	}
	if err := tx.Create(episode).Error; err != nil {
		return nil, false, false, err
	}

	var count int64
	if err := tx.Model(&models.Episode{}).Where("podcast_id = ?", podcast.ID).Count(&count).Error; err != nil {
		return nil, false, false, err
	}
	updates := map[string]any{"episode_count": count}
	if podcastCreated {
		updates["added_date"] = now
	}
	if item.PublishedAt != nil && item.PublishedAt.After(podcast.NewestEpisodeDate) {
		updates["newest_episode_date"] = *item.PublishedAt
	}
	if err := tx.Model(&models.Podcast{}).Where("id = ?", podcast.ID).Updates(updates).Error; err != nil {
		return nil, false, false, err
	}

	return episode, true, podcastCreated, nil
}

// resolveOrCreatePodcast 复用 XYZID 匹配的已有节目（保留原关注状态）；
// 新节目以显式写入持久化未关注并回读验证，缺 Feed 以 NULL 表示，不伪造地址。
func (s *CollectionAdoptionService) resolveOrCreatePodcast(
	tx *gorm.DB,
	item *models.EpisodeCollectionItem,
) (*models.Podcast, bool, error) {
	var podcast models.Podcast
	err := tx.Where("xyz_id = ?", item.ExternalPodcastID).First(&podcast).Error
	if err == nil {
		return &podcast, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	now := s.now().UTC()
	// map 写入绕过 GORM default:true 的零值覆盖：is_subscribed 必须显式落库为
	// false；feed_url 以 NULL 表示“未知/无 RSS”，允许多节目无 Feed。
	values := map[string]any{
		"xyz_id":                 item.ExternalPodcastID,
		"title":                  item.PodcastTitle,
		"author":                 item.PodcastAuthor,
		"cover_url":              item.PodcastCoverURL,
		"description":            "",
		"feed_url":               nil,
		"feed_url_valid":         false,
		"is_subscribed":          false,
		"is_dead":                false,
		"data_source":            models.SourcePlatformXiaoyuzhoufm,
		"added_date":             now,
		"external_episode_count": item.PodcastEpisodeCount,
		"created_at":             now,
		"updated_at":             now,
	}
	if err := tx.Model(&models.Podcast{}).Create(values).Error; err != nil {
		return nil, false, err
	}
	created := models.Podcast{}
	if err := tx.Where("xyz_id = ?", item.ExternalPodcastID).First(&created).Error; err != nil {
		return nil, false, err
	}
	// 回读验证：未关注值必须真实持久化，缺 Feed 不得被默认值伪造。
	if created.IsSubscribed {
		return nil, false, fmt.Errorf("collection podcast %d persisted subscribed=true; refusing to adopt", created.ID)
	}
	if created.FeedURL != "" {
		return nil, false, fmt.Errorf("collection podcast %d fabricated feed url %q", created.ID, created.FeedURL)
	}
	return &created, true, nil
}

func (s *CollectionAdoptionService) recordExternalRef(tx *gorm.DB, item *models.EpisodeCollectionItem, episodeID, podcastID uint) error {
	var ref models.EpisodeExternalRef
	err := tx.Where("source_platform = ? AND external_episode_id = ?", models.SourcePlatformXiaoyuzhoufm, item.ExternalEpisodeID).
		First(&ref).Error
	if err == nil {
		if ref.EpisodeID != episodeID || ref.PodcastID != podcastID {
			return fmt.Errorf("%w: ref maps to episode %d/podcast %d", ErrAdoptionIdentityInvalid, ref.EpisodeID, ref.PodcastID)
		}
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&models.EpisodeExternalRef{
		SourcePlatform:    models.SourcePlatformXiaoyuzhoufm,
		ExternalEpisodeID: item.ExternalEpisodeID,
		ExternalPodcastID: item.ExternalPodcastID,
		EpisodeID:         episodeID,
		PodcastID:         podcastID,
	}).Error
}

func (s *CollectionAdoptionService) recordAdoptionSummary(
	tx *gorm.DB,
	collection *models.EpisodeCollection,
	item *models.EpisodeCollectionItem,
	episodeID uint,
) error {
	var existing models.EpisodeCollectionAdoption
	err := tx.Where(
		"episode_id = ? AND source_platform = ? AND collection_external_id = ?",
		episodeID, collection.SourcePlatform, collection.ExternalID,
	).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&models.EpisodeCollectionAdoption{
		EpisodeID:            episodeID,
		SourcePlatform:       collection.SourcePlatform,
		CollectionExternalID: collection.ExternalID,
		CollectionTitle:      collection.Title,
		CollectionURL:        collection.SourceURL,
		ExternalEpisodeID:    item.ExternalEpisodeID,
		AdoptedAt:            s.now().UTC(),
	}).Error
}

// enqueueInbox 只在没有任何已有行动状态时写入 Inbox；已有 Focus、Someday、
// Done、不感兴趣状态及并发其他页面的决定不被覆盖。
func (s *CollectionAdoptionService) enqueueInbox(tx *gorm.DB, episodeID uint) (bool, error) {
	consumption := &ConsumptionService{db: tx, now: s.now}
	current, err := consumption.ensureState(tx, episodeID)
	if err != nil {
		return false, err
	}
	if current.QueueState != nil || current.DismissedAt != nil {
		return false, nil
	}

	targetIDs, err := queueEpisodeIDs(tx, models.QueueStateInbox)
	if err != nil {
		return false, err
	}
	targetIDs = removeEpisodeID(targetIDs, episodeID)
	if err := consumption.moveDecisionToQueue(tx, current, models.QueueStateInbox); err != nil {
		return false, err
	}
	targetIDs = append([]uint{episodeID}, targetIDs...)
	if err := resequenceQueue(tx, models.QueueStateInbox, targetIDs); err != nil {
		return false, err
	}
	if err := consumption.bumpQueueRevisions(tx, []string{models.QueueStateInbox}); err != nil {
		return false, err
	}
	return true, nil
}

// ListAdoptionSources 返回单集的清单来源摘要（展示与返回入口用）。
func (s *CollectionAdoptionService) ListAdoptionSources(episodeID uint) ([]models.EpisodeCollectionAdoption, error) {
	var sources []models.EpisodeCollectionAdoption
	err := s.db.Where("episode_id = ?", episodeID).
		Order("adopted_at ASC, id ASC").
		Find(&sources).Error
	return sources, err
}
