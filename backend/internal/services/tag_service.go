package services

import (
	apperrors "magicpodcast/internal/errors"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// TagService 标签服务层
type TagService struct {
	db *gorm.DB
}

// NewTagService 创建标签服务
func NewTagService(db *gorm.DB) *TagService {
	return &TagService{
		db: db,
	}
}

// ========== 请求和响应DTO ==========

// CreateTagRequest 创建标签请求（匹配实际模型）
type CreateTagRequest struct {
	Name  string `json:"name" binding:"required"`
	Color string `json:"color" binding:"required"`
	// 注意：实际模型没有 Description 字段
}

// UpdateTagRequest 更新标签请求
type UpdateTagRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
	// 注意：实际模型没有 Description 字段
}

// TagResponse 标签响应（匹配实际模型）
type TagResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// TagListResponse 标签列表响应
type TagListResponse struct {
	Tags     []TagResponse `json:"tags"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

// ========== CRUD 操作 ==========

// CreateTag 创建标签
func (s *TagService) CreateTag(req *CreateTagRequest) (*TagResponse, error) {
	// 检查名称是否已存在
	var existingTag models.Tag
	if err := s.db.Where("name = ?", req.Name).First(&existingTag).Error; err == nil {
		return nil, apperrors.ConflictError("tag", "name already exists")
	}

	tag := &models.Tag{
		Name:  req.Name,
		Color: req.Color,
		// 注意：实际模型没有 Description 字段
	}

	if err := s.db.Create(tag).Error; err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to create tag")
	}

	return s.toTagResponse(tag), nil
}

// GetTag 获取标签详情
func (s *TagService) GetTag(id uint) (*TagResponse, error) {
	var tag models.Tag
	if err := s.db.First(&tag, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("tag", id)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	return s.toTagResponse(&tag), nil
}

// UpdateTag 更新标签
func (s *TagService) UpdateTag(id uint, req *UpdateTagRequest) (*TagResponse, error) {
	var tag models.Tag
	if err := s.db.First(&tag, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("tag", id)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	updates := make(map[string]interface{})

	if req.Name != nil {
		// 检查新名称是否与其他标签冲突
		var existingTag models.Tag
		if err := s.db.Where("name = ? AND id != ?", *req.Name, id).First(&existingTag).Error; err == nil {
			return nil, apperrors.ConflictError("tag", "name already exists")
		}
		updates["name"] = *req.Name
	}
	if req.Color != nil {
		updates["color"] = *req.Color
	}

	if err := s.db.Model(&tag).Updates(updates).Error; err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to update tag")
	}

	// 重新加载
	if err := s.db.First(&tag, id).Error; err != nil {
		return nil, apperrors.InternalError("Failed to reload tag")
	}

	return s.toTagResponse(&tag), nil
}

// DeleteTag 删除标签
func (s *TagService) DeleteTag(id uint) error {
	// 检查是否存在
	var tag models.Tag
	if err := s.db.First(&tag, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("tag", id)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	// 删除标签关联（podcasts_tags 和 episodes_tags）
	if err := s.db.Exec("DELETE FROM podcasts_tags WHERE tag_id = ?", id).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to delete podcast tag associations")
	}

	if err := s.db.Exec("DELETE FROM episodes_tags WHERE tag_id = ?", id).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to delete episode tag associations")
	}

	// 删除标签
	if err := s.db.Delete(&tag).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to delete tag")
	}

	return nil
}

// ListTags 获取标签列表
func (s *TagService) ListTags(page, pageSize int) (*TagListResponse, error) {
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 20
	}

	var tags []models.Tag
	var total int64

	query := s.db.Model(&models.Tag{})

	// 计算总数
	if err := query.Count(&total).Error; err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to count tags")
	}

	// 分页查询
	offset := (page - 1) * pageSize
	if err := query.Order("name ASC").Offset(offset).Limit(pageSize).Find(&tags).Error; err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch tags")
	}

	// 转换为响应格式
	responses := make([]TagResponse, len(tags))
	for i, t := range tags {
		responses[i] = *s.toTagResponse(&t)
	}

	return &TagListResponse{
		Tags:     responses,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// ========== 标签关联操作 ==========

// AddTagToPodcast 为播客添加标签
func (s *TagService) AddTagToPodcast(podcastID, tagID uint) error {
	// 验证播客和标签是否存在
	var podcast models.Podcast
	if err := s.db.First(&podcast, podcastID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("podcast", podcastID)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch podcast")
	}

	var tag models.Tag
	if err := s.db.First(&tag, tagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("tag", tagID)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	// 检查是否已关联
	var count int64
	if err := s.db.Table("podcasts_tags").
		Where("podcast_id = ? AND tag_id = ?", podcastID, tagID).
		Count(&count).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to check tag association")
	}

	if count > 0 {
		return apperrors.ConflictError("podcast tag", "already associated")
	}

	// 创建关联
	if err := s.db.Exec("INSERT INTO podcasts_tags (podcast_id, tag_id) VALUES (?, ?)", podcastID, tagID).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to add tag to podcast")
	}

	return nil
}

// RemoveTagFromPodcast 从播客移除标签
func (s *TagService) RemoveTagFromPodcast(podcastID, tagID uint) error {
	// 检查关联是否存在
	var count int64
	if err := s.db.Table("podcasts_tags").
		Where("podcast_id = ? AND tag_id = ?", podcastID, tagID).
		Count(&count).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to check tag association")
	}

	if count == 0 {
		return apperrors.NotFoundError("podcast tag association", "")
	}

	// 删除关联
	if err := s.db.Exec("DELETE FROM podcasts_tags WHERE podcast_id = ? AND tag_id = ?", podcastID, tagID).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to remove tag from podcast")
	}

	return nil
}

// AddTagToEpisode 为单集添加标签
func (s *TagService) AddTagToEpisode(episodeID, tagID uint) error {
	// 验证单集和标签是否存在
	var episode models.Episode
	if err := s.db.First(&episode, episodeID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("episode", episodeID)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch episode")
	}

	var tag models.Tag
	if err := s.db.First(&tag, tagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("tag", tagID)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	// 检查是否已关联
	var count int64
	if err := s.db.Table("episodes_tags").
		Where("episode_id = ? AND tag_id = ?", episodeID, tagID).
		Count(&count).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to check tag association")
	}

	if count > 0 {
		return apperrors.ConflictError("episode tag", "already associated")
	}

	// 创建关联
	if err := s.db.Exec("INSERT INTO episodes_tags (episode_id, tag_id) VALUES (?, ?)", episodeID, tagID).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to add tag to episode")
	}

	return nil
}

// RemoveTagFromEpisode 从单集移除标签
func (s *TagService) RemoveTagFromEpisode(episodeID, tagID uint) error {
	// 检查关联是否存在
	var count int64
	if err := s.db.Table("episodes_tags").
		Where("episode_id = ? AND tag_id = ?", episodeID, tagID).
		Count(&count).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to check tag association")
	}

	if count == 0 {
		return apperrors.NotFoundError("episode tag association", "")
	}

	// 删除关联
	if err := s.db.Exec("DELETE FROM episodes_tags WHERE episode_id = ? AND tag_id = ?", episodeID, tagID).Error; err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to remove tag from episode")
	}

	return nil
}

// ========== 辅助方法 ==========

// toTagResponse 转换为响应格式
func (s *TagService) toTagResponse(tag *models.Tag) *TagResponse {
	return &TagResponse{
		ID:        tag.ID,
		Name:      tag.Name,
		Color:     tag.Color,
		CreatedAt: tag.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt: tag.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
