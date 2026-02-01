package services

import (
	apperrors "magicpodcast/internal/errors"
	"magicpodcast/internal/models"
	"magicpodcast/internal/repository"

	"gorm.io/gorm"
)

// TagService 标签服务层
type TagService struct {
	repos *repository.Repositories
}

// NewTagService 创建标签服务
func NewTagService(repos *repository.Repositories) *TagService {
	return &TagService{
		repos: repos,
	}
}

// ========== 请求和响应DTO ==========

// CreateTagRequest 创建标签请求（匹配实际模型）
type CreateTagRequest struct {
	Name  string `json:"name" binding:"required"`
	Color string `json:"color" binding:"required"`
}

// UpdateTagRequest 更新标签请求
type UpdateTagRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
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
	_, err := s.repos.Tag.GetByName(req.Name)
	if err == nil {
		return nil, apperrors.ConflictError("tag", "name already exists")
	} else if err != gorm.ErrRecordNotFound {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to check tag name")
	}

	tag := &models.Tag{
		Name:  req.Name,
		Color: req.Color,
	}

	if err := s.repos.Tag.Create(tag); err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to create tag")
	}

	return s.toTagResponse(tag), nil
}

// GetTag 获取标签详情
func (s *TagService) GetTag(id uint) (*TagResponse, error) {
	tag, err := s.repos.Tag.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("tag", id)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	return s.toTagResponse(tag), nil
}

// UpdateTag 更新标签
func (s *TagService) UpdateTag(id uint, req *UpdateTagRequest) (*TagResponse, error) {
	// 获取现有标签
	tag, err := s.repos.Tag.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("tag", id)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	// 检查新名称是否与其他标签冲突
	if req.Name != nil && *req.Name != tag.Name {
		_, err := s.repos.Tag.GetByName(*req.Name)
		if err == nil {
			return nil, apperrors.ConflictError("tag", "name already exists")
		} else if err != gorm.ErrRecordNotFound {
			return nil, apperrors.InternalErrorWithErr(err, "Failed to check tag name")
		}
		tag.Name = *req.Name
	}

	if req.Color != nil {
		tag.Color = *req.Color
	}

	if err := s.repos.Tag.Update(tag); err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to update tag")
	}

	return s.toTagResponse(tag), nil
}

// DeleteTag 删除标签
func (s *TagService) DeleteTag(id uint) error {
	// 检查是否存在
	_, err := s.repos.Tag.GetByID(id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("tag", id)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	// Repository 的 Delete 方法应该自动处理关联删除(通过 GORM 的约束)
	if err := s.repos.Tag.Delete(id); err != nil {
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

	tags, total, err := s.repos.Tag.List(page, pageSize)
	if err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch tags")
	}

	// 转换为响应格式
	responses := make([]TagResponse, len(tags))
	for i, t := range tags {
		responses[i] = *s.toTagResponse(t)
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
	_, err := s.repos.Podcast.GetByID(podcastID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("podcast", podcastID)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch podcast")
	}

	_, err = s.repos.Tag.GetByID(tagID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("tag", tagID)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	// 使用 Repository 方法添加关联(已实现幂等性检查)
	if err := s.repos.Tag.AddTagToPodcast(podcastID, tagID); err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to add tag to podcast")
	}

	return nil
}

// RemoveTagFromPodcast 从播客移除标签
func (s *TagService) RemoveTagFromPodcast(podcastID, tagID uint) error {
	// 使用 Repository 方法移除关联
	if err := s.repos.Tag.RemoveTagFromPodcast(podcastID, tagID); err != nil {
		// 检查是否因为关联不存在而失败
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("podcast tag association", "")
		}
		return apperrors.InternalErrorWithErr(err, "Failed to remove tag from podcast")
	}

	return nil
}

// AddTagToEpisode 为单集添加标签
func (s *TagService) AddTagToEpisode(episodeID, tagID uint) error {
	// 验证单集和标签是否存在
	_, err := s.repos.Episode.GetByID(episodeID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("episode", episodeID)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch episode")
	}

	_, err = s.repos.Tag.GetByID(tagID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("tag", tagID)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	// 使用 Repository 方法添加关联
	if err := s.repos.Tag.AddTagToEpisode(episodeID, tagID); err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to add tag to episode")
	}

	return nil
}

// RemoveTagFromEpisode 从单集移除标签
func (s *TagService) RemoveTagFromEpisode(episodeID, tagID uint) error {
	// 使用 Repository 方法移除关联
	if err := s.repos.Tag.RemoveTagFromEpisode(episodeID, tagID); err != nil {
		// 检查是否因为关联不存在而失败
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("episode tag association", "")
		}
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
