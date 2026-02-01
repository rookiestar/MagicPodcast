package repository

import (
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// PodcastFilters 播客筛选条件
type PodcastFilters struct {
	TagID     *int
	TagIDs    []int
	Search    string
	SortBy    string
	Page      int
	PageSize  int
	IsSubscribed *bool
}

// PodcastRepository 播客数据访问接口
type PodcastRepository interface {
	Repository

	// Create 创建播客
	Create(podcast *models.Podcast) error

	// GetByID 根据ID获取播客
	GetByID(id uint) (*models.Podcast, error)

	// List 获取播客列表
	List(filters PodcastFilters) ([]*models.Podcast, int64, error)

	// Update 更新播客
	Update(podcast *models.Podcast) error

	// Delete 删除播客
	Delete(id uint) error

	// GetByIDs 批量获取播客
	GetByIDs(ids []uint) ([]*models.Podcast, error)

	// Search 搜索播客
	Search(keyword string, page, pageSize int) ([]*models.Podcast, int64, error)

	// GetWithTags 获取播客及其标签
	GetWithTags(id uint) (*models.Podcast, error)

	// UpdateNotes 更新播客备注
	UpdateNotes(id uint, notes string) error

	// UpdateLastFetchTime 更新最后抓取时间
	UpdateLastFetchTime(id uint) error

	// IncrementFetchErrorCount 增加抓取错误计数
	IncrementFetchErrorCount(id uint) error

	// ResetFetchErrorCount 重置抓取错误计数
	ResetFetchErrorCount(id uint) error
}

// podcastRepository 播客数据访问实现
type podcastRepository struct {
	*BaseRepository
}

// NewPodcastRepository 创建播客Repository
func NewPodcastRepository(db *gorm.DB) PodcastRepository {
	return &podcastRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// Create 创建播客
func (r *podcastRepository) Create(podcast *models.Podcast) error {
	return r.DB().Create(podcast).Error
}

// GetByID 根据ID获取播客
func (r *podcastRepository) GetByID(id uint) (*models.Podcast, error) {
	var podcast models.Podcast
	err := r.DB().First(&podcast, id).Error
	if err != nil {
		return nil, err
	}
	return &podcast, nil
}

// List 获取播客列表
func (r *podcastRepository) List(filters PodcastFilters) ([]*models.Podcast, int64, error) {
	var podcasts []*models.Podcast
	var total int64

	query := r.DB().Model(&models.Podcast{})

	// 应用筛选条件
	query = r.applyFilters(query, filters)

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 应用排序
	query = r.applySorting(query, filters.SortBy)

	// 应用分页
	offset := (filters.Page - 1) * filters.PageSize
	query = query.Offset(offset).Limit(filters.PageSize)

	// 预加载标签
	if err := query.Preload("Tags").Find(&podcasts).Error; err != nil {
		return nil, 0, err
	}

	return podcasts, total, nil
}

// applyFilters 应用筛选条件
func (r *podcastRepository) applyFilters(query *gorm.DB, filters PodcastFilters) *gorm.DB {
	// 标签筛选
	if filters.TagID != nil {
		query = query.Joins("JOIN podcasts_tags ON podcasts_tags.podcast_id = podcasts.id").
			Where("podcasts_tags.tag_id = ?", *filters.TagID)
	}

	if len(filters.TagIDs) > 0 {
		query = query.Joins("JOIN podcasts_tags ON podcasts_tags.podcast_id = podcasts.id").
			Where("podcasts_tags.tag_id IN ?", filters.TagIDs)
	}

	// 搜索筛选
	if filters.Search != "" {
		searchTerm := "%" + filters.Search + "%"
		query = query.Where("title LIKE ? OR author LIKE ? OR description LIKE ?",
			searchTerm, searchTerm, searchTerm)
	}

	// 订阅状态筛选
	if filters.IsSubscribed != nil {
		query = query.Where("is_subscribed = ?", *filters.IsSubscribed)
	}

	return query
}

// applySorting 应用排序
func (r *podcastRepository) applySorting(query *gorm.DB, sortBy string) *gorm.DB {
	switch sortBy {
	case "recent_update":
		return query.Order("newest_episode_date DESC")
	case "title":
		return query.Order("title ASC")
	case "episode_count":
		return query.Order("episode_count DESC")
	default:
		return query.Order("created_at DESC")
	}
}

// Update 更新播客
func (r *podcastRepository) Update(podcast *models.Podcast) error {
	return r.DB().Save(podcast).Error
}

// Delete 删除播客
func (r *podcastRepository) Delete(id uint) error {
	return r.DB().Delete(&models.Podcast{}, id).Error
}

// GetByIDs 批量获取播客
func (r *podcastRepository) GetByIDs(ids []uint) ([]*models.Podcast, error) {
	var podcasts []*models.Podcast
	err := r.DB().Where("id IN ?", ids).Find(&podcasts).Error
	return podcasts, err
}

// Search 搜索播客
func (r *podcastRepository) Search(keyword string, page, pageSize int) ([]*models.Podcast, int64, error) {
	var podcasts []*models.Podcast
	var total int64

	searchTerm := "%" + keyword + "%"
	query := r.DB().Model(&models.Podcast{}).
		Where("title LIKE ? OR author LIKE ? OR description LIKE ?",
			searchTerm, searchTerm, searchTerm)

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页
	offset := (page - 1) * pageSize
	err := query.Offset(offset).Limit(pageSize).Find(&podcasts).Error

	return podcasts, total, err
}

// GetWithTags 获取播客及其标签
func (r *podcastRepository) GetWithTags(id uint) (*models.Podcast, error) {
	var podcast models.Podcast
	err := r.DB().Preload("Tags").First(&podcast, id).Error
	if err != nil {
		return nil, err
	}
	return &podcast, nil
}

// UpdateNotes 更新播客备注
func (r *podcastRepository) UpdateNotes(id uint, notes string) error {
	return r.DB().Model(&models.Podcast{}).Where("id = ?", id).Update("notes", notes).Error
}

// UpdateLastFetchTime 更新最后抓取时间
func (r *podcastRepository) UpdateLastFetchTime(id uint) error {
	return r.DB().Model(&models.Podcast{}).Where("id = ?", id).
		Update("last_fetched_at", gorm.Expr("NOW()")).Error
}

// IncrementFetchErrorCount 增加抓取错误计数
func (r *podcastRepository) IncrementFetchErrorCount(id uint) error {
	return r.DB().Model(&models.Podcast{}).Where("id = ?", id).
		Update("fetch_error_count", gorm.Expr("fetch_error_count + 1")).Error
}

// ResetFetchErrorCount 重置抓取错误计数
func (r *podcastRepository) ResetFetchErrorCount(id uint) error {
	return r.DB().Model(&models.Podcast{}).Where("id = ?", id).
		Update("fetch_error_count", 0).Error
}
