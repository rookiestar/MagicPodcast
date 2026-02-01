package services

import (
	apperrors "magicpodcast/internal/errors"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// PodcastService 播客服务层
type PodcastService struct {
	db *gorm.DB
}

// NewPodcastService 创建播客服务
func NewPodcastService(db *gorm.DB) *PodcastService {
	return &PodcastService{
		db: db,
	}
}

// ========== 请求和响应DTO ==========

// GetPodcastRequest 获取播客请求
type GetPodcastRequest struct {
	ID uint `uri:"id" binding:"required"`
}

// PodcastListRequest 播客列表请求
type PodcastListRequest struct {
	Page         int    `form:"page" binding:"min=1"`
	PageSize     int    `form:"page_size" binding:"min=1,max=100"`
	Search       string `form:"search"`
	IsSubscribed *bool  `form:"is_subscribed"`
	SortBy       string `form:"sort_by" binding:"omitempty,oneof=title created_at updated_at"`
	SortOrder    string `form:"sort_order" binding:"omitempty,oneof=asc desc"`
}

// PodcastResponse 播客响应
type PodcastResponse struct {
	ID             uint   `json:"id"`
	XYZID          string `json:"xyz_id"`
	Title          string `json:"title"`
	Author         string `json:"author"`
	Description    string `json:"description"`
	CoverURL       string `json:"cover_url"`
	FeedURL        string `json:"feed_url"`
	IsSubscribed   bool   `json:"is_subscribed"`
	EpisodeCount   int    `json:"episode_count"`
	Notes          string `json:"notes"`
	MyRate         int    `json:"my_rate"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// PodcastListResponse 播客列表响应
type PodcastListResponse struct {
	Podcasts []PodcastResponse `json:"podcasts"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// UpdatePodcastNotesRequest 更新播客备注请求
type UpdatePodcastNotesRequest struct {
	Notes string `json:"notes" binding:"required"`
}

// ========== CRUD 操作 ==========

// GetPodcast 获取播客详情
func (s *PodcastService) GetPodcast(id uint) (*PodcastResponse, error) {
	var podcast models.Podcast
	if err := s.db.First(&podcast, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("podcast", id)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch podcast")
	}

	return s.toPodcastResponse(&podcast), nil
}

// ListPodcasts 获取播客列表
func (s *PodcastService) ListPodcasts(req *PodcastListRequest) (*PodcastListResponse, error) {
	// 默认值
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}
	if req.SortBy == "" {
		req.SortBy = "created_at"
	}
	if req.SortOrder == "" {
		req.SortOrder = "desc"
	}

	var podcasts []models.Podcast
	var total int64

	query := s.db.Model(&models.Podcast{})

	// 搜索条件
	if req.Search != "" {
		searchTerm := "%" + req.Search + "%"
		query = query.Where("title LIKE ? OR author LIKE ?", searchTerm, searchTerm)
	}

	// 订阅状态筛选
	if req.IsSubscribed != nil {
		query = query.Where("is_subscribed = ?", *req.IsSubscribed)
	}

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to count podcasts")
	}

	// 排序和分页
	orderClause := req.SortBy + " " + req.SortOrder
	offset := (req.Page - 1) * req.PageSize

	if err := query.Order(orderClause).Offset(offset).Limit(req.PageSize).Find(&podcasts).Error; err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch podcasts")
	}

	// 转换为响应格式
	responses := make([]PodcastResponse, len(podcasts))
	for i, p := range podcasts {
		responses[i] = *s.toPodcastResponse(&p)
	}

	return &PodcastListResponse{
		Podcasts: responses,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

// BatchGetPodcasts 批量获取播客
func (s *PodcastService) BatchGetPodcasts(ids []uint) ([]PodcastResponse, error) {
	if len(ids) == 0 {
		return []PodcastResponse{}, nil
	}

	var podcasts []models.Podcast
	if err := s.db.Where("id IN ?", ids).Find(&podcasts).Error; err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch podcasts")
	}

	responses := make([]PodcastResponse, len(podcasts))
	for i, p := range podcasts {
		responses[i] = *s.toPodcastResponse(&p)
	}

	return responses, nil
}

// UpdatePodcastNotes 更新播客备注
func (s *PodcastService) UpdatePodcastNotes(id uint, notes string) error {
	var podcast models.Podcast
	if err := s.db.First(&podcast, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("podcast", id)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch podcast")
	}

	if err := s.db.Model(&podcast).Update("notes", notes).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to update podcast notes")
	}

	return nil
}

// GetPodcastTags 获取播客标签
func (s *PodcastService) GetPodcastTags(id uint) ([]models.Tag, error) {
	// 验证播客是否存在
	var podcast models.Podcast
	if err := s.db.First(&podcast, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("podcast", id)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch podcast")
	}

	var tags []models.Tag
	if err := s.db.Model(&podcast).Association("Tags").Find(&tags); err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch podcast tags")
	}

	return tags, nil
}

// GetPodcastEpisodes 获取播客单集列表
func (s *PodcastService) GetPodcastEpisodes(id uint, page, pageSize int) ([]models.Episode, int64, error) {
	// 验证播客是否存在
	var podcast models.Podcast
	if err := s.db.First(&podcast, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, 0, apperrors.NotFoundError("podcast", id)
		}
		return nil, 0, apperrors.InternalErrorWithErr(err, "Failed to fetch podcast")
	}

	var episodes []models.Episode
	var total int64

	query := s.db.Model(&models.Episode{}).Where("podcast_id = ?", id)

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperrors.InternalErrorWithErr(err, "Failed to count episodes")
	}

	// 分页查询
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize
	if err := query.Order("published_date DESC").Offset(offset).Limit(pageSize).Find(&episodes).Error; err != nil {
		return nil, 0, apperrors.InternalErrorWithErr(err, "Failed to fetch episodes")
	}

	return episodes, total, nil
}

// ========== 辅助方法 ==========

// toPodcastResponse 转换为响应格式
func (s *PodcastService) toPodcastResponse(podcast *models.Podcast) *PodcastResponse {
	return &PodcastResponse{
		ID:           podcast.ID,
		XYZID:        podcast.XYZID,
		Title:        podcast.Title,
		Author:       podcast.Author,
		Description:  podcast.Description,
		CoverURL:     podcast.CoverURL,
		FeedURL:      podcast.FeedURL,
		IsSubscribed: podcast.IsSubscribed,
		EpisodeCount: podcast.EpisodeCount,
		Notes:        podcast.Notes,
		MyRate:       podcast.MyRate,
		CreatedAt:    podcast.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:    podcast.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
