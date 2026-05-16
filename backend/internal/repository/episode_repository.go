package repository

import (
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// EpisodeFilters 单集筛选条件
type EpisodeFilters struct {
	PodcastID *uint
	Search    string
	Page      int
	PageSize  int
}

// EpisodeRepository 单集数据访问接口
type EpisodeRepository interface {
	Repository

	// Create 创建单集
	Create(episode *models.Episode) error

	// BatchCreate 批量创建单集
	BatchCreate(episodes []*models.Episode) error

	// GetByID 根据ID获取单集
	GetByID(id uint) (*models.Episode, error)

	// List 获取单集列表
	List(filters EpisodeFilters) ([]*models.Episode, int64, error)

	// Update 更新单集
	Update(episode *models.Episode) error

	// Delete 删除单集
	Delete(id uint) error

	// GetByPodcastID 获取播客的所有单集
	GetByPodcastID(podcastID uint, page, pageSize int) ([]*models.Episode, int64, error)

	// GetByPodcastIDsWithFilters 根据播客ID和筛选条件获取单集
	GetByPodcastIDsWithFilters(podcastID uint, filters EpisodeFilters) ([]*models.Episode, int64, error)

	// Search 搜索单集
	Search(keyword string, page, pageSize int) ([]*models.Episode, int64, error)

	// BatchCreateWithTx 使用事务批量创建
	BatchCreateWithTx(tx *gorm.DB, episodes []*models.Episode) error

	// GetWithTags 获取单集及其标签
	GetWithTags(id uint) (*models.Episode, error)
}

// episodeRepository 单集数据访问实现
type episodeRepository struct {
	*BaseRepository
}

// NewEpisodeRepository 创建单集Repository
func NewEpisodeRepository(db *gorm.DB) EpisodeRepository {
	return &episodeRepository{
		BaseRepository: NewBaseRepository(db),
	}
}

// Create 创建单集
func (r *episodeRepository) Create(episode *models.Episode) error {
	return r.DB().Create(episode).Error
}

// BatchCreate 批量创建单集
func (r *episodeRepository) BatchCreate(episodes []*models.Episode) error {
	if len(episodes) == 0 {
		return nil
	}
	return r.DB().CreateInBatches(episodes, 100).Error
}

// GetByID 根据ID获取单集
func (r *episodeRepository) GetByID(id uint) (*models.Episode, error) {
	var episode models.Episode
	err := r.DB().First(&episode, id).Error
	if err != nil {
		return nil, err
	}
	return &episode, nil
}

// List 获取单集列表
func (r *episodeRepository) List(filters EpisodeFilters) ([]*models.Episode, int64, error) {
	var episodes []*models.Episode
	var total int64

	query := r.DB().Model(&models.Episode{})

	// 应用筛选条件
	if filters.PodcastID != nil {
		query = query.Where("podcast_id = ?", *filters.PodcastID)
	}

	if filters.Search != "" {
		searchTerm := "%" + filters.Search + "%"
		query = query.Where("title LIKE ? OR show_notes LIKE ?", searchTerm, searchTerm)
	}

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 应用排序和分页
	query = query.Order("published_date DESC, id DESC")
	offset := (filters.Page - 1) * filters.PageSize
	query = query.Offset(offset).Limit(filters.PageSize)

	if err := query.Find(&episodes).Error; err != nil {
		return nil, 0, err
	}

	return episodes, total, nil
}

// Update 更新单集
func (r *episodeRepository) Update(episode *models.Episode) error {
	return r.DB().Save(episode).Error
}

// Delete 删除单集
func (r *episodeRepository) Delete(id uint) error {
	return r.DB().Delete(&models.Episode{}, id).Error
}

// GetByPodcastID 获取播客的所有单集
func (r *episodeRepository) GetByPodcastID(podcastID uint, page, pageSize int) ([]*models.Episode, int64, error) {
	var episodes []*models.Episode
	var total int64

	query := r.DB().Model(&models.Episode{}).Where("podcast_id = ?", podcastID)

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页
	offset := (page - 1) * pageSize
	err := query.Order("published_date DESC, id DESC").Offset(offset).Limit(pageSize).Find(&episodes).Error

	return episodes, total, err
}

// GetByPodcastIDsWithFilters 根据播客ID和筛选条件获取单集
func (r *episodeRepository) GetByPodcastIDsWithFilters(podcastID uint, filters EpisodeFilters) ([]*models.Episode, int64, error) {
	var episodes []*models.Episode
	var total int64

	query := r.DB().Model(&models.Episode{}).Where("podcast_id = ?", podcastID)

	// 应用搜索筛选
	if filters.Search != "" {
		searchTerm := "%" + filters.Search + "%"
		query = query.Where("title LIKE ? OR show_notes LIKE ?", searchTerm, searchTerm)
	}

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 应用排序和分页
	query = query.Order("published_date DESC, id DESC")
	offset := (filters.Page - 1) * filters.PageSize
	query = query.Offset(offset).Limit(filters.PageSize)

	if err := query.Find(&episodes).Error; err != nil {
		return nil, 0, err
	}

	return episodes, total, nil
}

// Search 搜索单集
func (r *episodeRepository) Search(keyword string, page, pageSize int) ([]*models.Episode, int64, error) {
	var episodes []*models.Episode
	var total int64

	searchTerm := "%" + keyword + "%"
	query := r.DB().Model(&models.Episode{}).
		Where("title LIKE ? OR show_notes LIKE ?", searchTerm, searchTerm)

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页
	offset := (page - 1) * pageSize
	err := query.Order("published_date DESC, id DESC").Offset(offset).Limit(pageSize).Find(&episodes).Error

	return episodes, total, err
}

// BatchCreateWithTx 使用事务批量创建
func (r *episodeRepository) BatchCreateWithTx(tx *gorm.DB, episodes []*models.Episode) error {
	if len(episodes) == 0 {
		return nil
	}
	return tx.CreateInBatches(episodes, 100).Error
}

// GetWithTags 获取单集及其标签
func (r *episodeRepository) GetWithTags(id uint) (*models.Episode, error) {
	var episode models.Episode
	err := r.DB().Preload("Tags").First(&episode, id).Error
	if err != nil {
		return nil, err
	}
	return &episode, nil
}
