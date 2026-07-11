package repository

import (
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// TagRepository 标签数据访问接口
type TagRepository interface {
	Repository

	// Create 创建标签
	Create(tag *models.Tag) error

	// GetByID 根据ID获取标签
	GetByID(id uint) (*models.Tag, error)

	// List 获取标签列表
	List(page, pageSize int) ([]*models.Tag, int64, error)

	// Update 更新标签
	Update(tag *models.Tag) error

	// Delete 删除标签
	Delete(id uint) error

	// GetByName 根据名称获取标签
	GetByName(name string) (*models.Tag, error)

	// Search 搜索标签
	Search(keyword string, page, pageSize int) ([]*models.Tag, int64, error)

	// GetByIDs 批量获取标签
	GetByIDs(ids []uint) ([]*models.Tag, error)

	// GetPodcastsByTagID 获取使用该标签的播客
	GetPodcastsByTagID(tagID uint, page, pageSize int) ([]*models.Podcast, int64, error)

	// GetPodcastCountsBatch 批量获取多个标签的播客数量
	GetPodcastCountsBatch(tagIDs []uint) (map[uint]int64, error)

	// GetEpisodesByTagID 获取使用该标签的单集
	GetEpisodesByTagID(tagID uint, page, pageSize int) ([]*models.Episode, int64, error)

	// AddTagToPodcast 为播客添加标签
	AddTagToPodcast(podcastID, tagID uint) error

	// RemoveTagFromPodcast 移除播客标签
	RemoveTagFromPodcast(podcastID, tagID uint) error

	// AddTagToEpisode 为单集添加标签
	AddTagToEpisode(episodeID, tagID uint) error

	// RemoveTagFromEpisode 移除单集标签
	RemoveTagFromEpisode(episodeID, tagID uint) error

	// GetPodcastTags 获取播客的所有标签
	GetPodcastTags(podcastID uint) ([]*models.Tag, error)

	// GetEpisodeTags 获取单集的所有标签
	GetEpisodeTags(episodeID uint) ([]*models.Tag, error)
}

// tagRepository 标签数据访问实现
type tagRepository struct {
	*BaseRepository
}

// NewTagRepository 创建标签Repository
func NewTagRepository(db *gorm.DB) TagRepository {
	return &tagRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// Create 创建标签
func (r *tagRepository) Create(tag *models.Tag) error {
	return r.DB().Create(tag).Error
}

// GetByID 根据ID获取标签
func (r *tagRepository) GetByID(id uint) (*models.Tag, error) {
	var tag models.Tag
	err := r.DB().First(&tag, id).Error
	if err != nil {
		return nil, err
	}
	return &tag, nil
}

// List 获取标签列表
func (r *tagRepository) List(page, pageSize int) ([]*models.Tag, int64, error) {
	var tags []*models.Tag
	var total int64

	query := r.DB().Model(&models.Tag{})

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页
	offset := (page - 1) * pageSize
	err := query.Order("id ASC").Offset(offset).Limit(pageSize).Find(&tags).Error

	return tags, total, err
}

// Update 更新标签
func (r *tagRepository) Update(tag *models.Tag) error {
	return r.DB().Save(tag).Error
}

// Delete 删除标签
func (r *tagRepository) Delete(id uint) error {
	return r.DB().Transaction(func(tx *gorm.DB) error {
		// 删除播客标签关联
		if err := tx.Table("podcasts_tags").Where("tag_id = ?", id).Delete(nil).Error; err != nil {
			return err
		}

		// 删除单集标签关联
		if err := tx.Table("episodes_tags").Where("tag_id = ?", id).Delete(nil).Error; err != nil {
			return err
		}

		// 删除标签
		return tx.Delete(&models.Tag{}, id).Error
	})
}

// GetByName 根据名称获取标签
func (r *tagRepository) GetByName(name string) (*models.Tag, error) {
	var tag models.Tag
	err := r.DB().Where("name = ?", name).First(&tag).Error
	if err != nil {
		return nil, err
	}
	return &tag, nil
}

// Search 搜索标签
func (r *tagRepository) Search(keyword string, page, pageSize int) ([]*models.Tag, int64, error) {
	var tags []*models.Tag
	var total int64

	searchTerm := "%" + keyword + "%"
	query := r.DB().Model(&models.Tag{}).
		Where("name LIKE ?", searchTerm)

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页
	offset := (page - 1) * pageSize
	err := query.Order("id ASC").Offset(offset).Limit(pageSize).Find(&tags).Error

	return tags, total, err
}

// GetByIDs 批量获取标签
func (r *tagRepository) GetByIDs(ids []uint) ([]*models.Tag, error) {
	var tags []*models.Tag
	err := r.DB().Where("id IN ?", ids).Find(&tags).Error
	return tags, err
}

// GetPodcastsByTagID 获取使用该标签的播客
func (r *tagRepository) GetPodcastsByTagID(tagID uint, page, pageSize int) ([]*models.Podcast, int64, error) {
	var podcasts []*models.Podcast
	var total int64

	subQuery := r.DB().Table("podcasts_tags").Select("podcast_id").Where("tag_id = ?", tagID)

	query := r.DB().Model(&models.Podcast{}).Where("id IN (?)", subQuery)

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页
	offset := (page - 1) * pageSize
	err := query.Order("title ASC").Offset(offset).Limit(pageSize).Find(&podcasts).Error

	return podcasts, total, err
}

// GetEpisodesByTagID 获取使用该标签的单集
func (r *tagRepository) GetEpisodesByTagID(tagID uint, page, pageSize int) ([]*models.Episode, int64, error) {
	var episodes []*models.Episode
	var total int64

	subQuery := r.DB().Table("episodes_tags").Select("episode_id").Where("tag_id = ?", tagID)

	query := r.DB().Model(&models.Episode{}).Where("id IN (?)", subQuery)

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页
	offset := (page - 1) * pageSize
	err := query.Order("published_date DESC").Offset(offset).Limit(pageSize).Find(&episodes).Error

	return episodes, total, err
}

// AddTagToPodcast 为播客添加标签
func (r *tagRepository) AddTagToPodcast(podcastID, tagID uint) error {
	// 检查是否已存在
	var count int64
	if err := r.DB().Table("podcasts_tags").
		Where("podcast_id = ? AND tag_id = ?", podcastID, tagID).
		Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return nil // 已存在，无需重复添加
	}

	// 创建关联
	return r.DB().Exec(
		"INSERT INTO podcasts_tags (podcast_id, tag_id) VALUES (?, ?)",
		podcastID, tagID,
	).Error
}

// RemoveTagFromPodcast 移除播客标签
func (r *tagRepository) RemoveTagFromPodcast(podcastID, tagID uint) error {
	return r.DB().Table("podcasts_tags").
		Where("podcast_id = ? AND tag_id = ?", podcastID, tagID).
		Delete(nil).Error
}

// AddTagToEpisode 为单集添加标签
func (r *tagRepository) AddTagToEpisode(episodeID, tagID uint) error {
	// 检查是否已存在
	var count int64
	if err := r.DB().Table("episodes_tags").
		Where("episode_id = ? AND tag_id = ?", episodeID, tagID).
		Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return nil // 已存在，无需重复添加
	}

	// 创建关联
	return r.DB().Exec(
		"INSERT INTO episodes_tags (episode_id, tag_id) VALUES (?, ?)",
		episodeID, tagID,
	).Error
}

// RemoveTagFromEpisode 移除单集标签
func (r *tagRepository) RemoveTagFromEpisode(episodeID, tagID uint) error {
	return r.DB().Table("episodes_tags").
		Where("episode_id = ? AND tag_id = ?", episodeID, tagID).
		Delete(nil).Error
}

// GetPodcastTags 获取播客的所有标签
func (r *tagRepository) GetPodcastTags(podcastID uint) ([]*models.Tag, error) {
	var tags []*models.Tag

	err := r.DB().Joins("JOIN podcasts_tags ON podcasts_tags.tag_id = tags.id").
		Where("podcasts_tags.podcast_id = ?", podcastID).
		Find(&tags).Error

	return tags, err
}

// GetEpisodeTags 获取单集的所有标签
func (r *tagRepository) GetEpisodeTags(episodeID uint) ([]*models.Tag, error) {
	var tags []*models.Tag

	err := r.DB().Joins("JOIN episodes_tags ON episodes_tags.tag_id = tags.id").
		Where("episodes_tags.episode_id = ?", episodeID).
		Find(&tags).Error

	return tags, err
}

// GetPodcastCountsBatch 批量获取多个标签的播客数量
func (r *tagRepository) GetPodcastCountsBatch(tagIDs []uint) (map[uint]int64, error) {
	if len(tagIDs) == 0 {
		return make(map[uint]int64), nil
	}

	var results []struct {
		TagID uint
		Count int64
	}

	err := r.DB().Table("podcasts_tags").
		Select("tag_id, COUNT(*) as count").
		Where("tag_id IN ?", tagIDs).
		Group("tag_id").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	// 转换为 map
	countMap := make(map[uint]int64, len(tagIDs))
	for _, result := range results {
		countMap[result.TagID] = result.Count
	}

	// 确保所有标签都在 map 中（即使没有播客）
	for _, tagID := range tagIDs {
		if _, exists := countMap[tagID]; !exists {
			countMap[tagID] = 0
		}
	}

	return countMap, nil
}
