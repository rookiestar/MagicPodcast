package services

import (
	apperrors "magicpodcast/internal/errors"
	"magicpodcast/internal/models"
	"magicpodcast/internal/repository"

	"gorm.io/gorm"
)

// TagRelationService 标签关联服务
type TagRelationService struct {
	repos *repository.Repositories
}

// NewTagRelationService 创建标签关联服务
func NewTagRelationService(repos *repository.Repositories) *TagRelationService {
	return &TagRelationService{
		repos: repos,
	}
}

// TargetType 目标类型
type TargetType string

const (
	TargetTypePodcast TargetType = "podcast"
	TargetTypeEpisode TargetType = "episode"
)

// AddTagResult 添加标签结果
type AddTagResult struct {
	Message  string
	TargetID uint
	TagID    uint
	TagName  string
}

// AddTag 为目标添加标签
func (s *TagRelationService) AddTag(targetType TargetType, targetID, tagID uint) (*AddTagResult, error) {
	// 验证标签存在
	tag, err := s.repos.Tag.GetByID(tagID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("tag", tagID)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	// 根据目标类型处理
	switch targetType {
	case TargetTypePodcast:
		return s.addTagToPodcast(targetID, tagID, tag)
	case TargetTypeEpisode:
		return s.addTagToEpisode(targetID, tagID, tag)
	default:
		return nil, apperrors.BadRequestError("unsupported target type: " + string(targetType))
	}
}

// addTagToPodcast 为播客添加标签
func (s *TagRelationService) addTagToPodcast(podcastID, tagID uint, tag *models.Tag) (*AddTagResult, error) {
	// 检查播客是否存在
	_, err := s.repos.Podcast.GetByID(podcastID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("podcast", podcastID)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch podcast")
	}

	// 添加关联（Repository会检查是否已存在）
	if err := s.repos.Tag.AddTagToPodcast(podcastID, tagID); err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to add tag to podcast")
	}

	return &AddTagResult{
		Message:  "标签已添加",
		TargetID: podcastID,
		TagID:    tagID,
		TagName:  tag.Name,
	}, nil
}

// addTagToEpisode 为单集添加标签
func (s *TagRelationService) addTagToEpisode(episodeID, tagID uint, tag *models.Tag) (*AddTagResult, error) {
	// 检查单集是否存在
	_, err := s.repos.Episode.GetByID(episodeID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.NotFoundError("episode", episodeID)
		}
		return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch episode")
	}

	// 添加关联（Repository会检查是否已存在）
	if err := s.repos.Tag.AddTagToEpisode(episodeID, tagID); err != nil {
		return nil, apperrors.InternalErrorWithErr(err, "Failed to add tag to episode")
	}

	return &AddTagResult{
		Message:  "标签已添加",
		TargetID: episodeID,
		TagID:    tagID,
		TagName:  tag.Name,
	}, nil
}

// RemoveTag 移除目标的标签
func (s *TagRelationService) RemoveTag(targetType TargetType, targetID, tagID uint) error {
	// 验证标签存在
	_, err := s.repos.Tag.GetByID(tagID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("tag", tagID)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch tag")
	}

	// 根据目标类型处理
	switch targetType {
	case TargetTypePodcast:
		return s.removeTagFromPodcast(targetID, tagID)
	case TargetTypeEpisode:
		return s.removeTagFromEpisode(targetID, tagID)
	default:
		return apperrors.BadRequestError("unsupported target type: " + string(targetType))
	}
}

// removeTagFromPodcast 移除播客标签
func (s *TagRelationService) removeTagFromPodcast(podcastID, tagID uint) error {
	// 检查播客是否存在
	_, err := s.repos.Podcast.GetByID(podcastID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("podcast", podcastID)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch podcast")
	}

	if err := s.repos.Tag.RemoveTagFromPodcast(podcastID, tagID); err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to remove tag from podcast")
	}
	return nil
}

// removeTagFromEpisode 移除单集标签
func (s *TagRelationService) removeTagFromEpisode(episodeID, tagID uint) error {
	// 检查单集是否存在
	_, err := s.repos.Episode.GetByID(episodeID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.NotFoundError("episode", episodeID)
		}
		return apperrors.InternalErrorWithErr(err, "Failed to fetch episode")
	}

	if err := s.repos.Tag.RemoveTagFromEpisode(episodeID, tagID); err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to remove tag from episode")
	}
	return nil
}

// RemoveTagFromPodcast 直接移除播客标签的公开方法
func (s *TagRelationService) RemoveTagFromPodcast(podcastID, tagID uint) error {
	if err := s.repos.Tag.RemoveTagFromPodcast(podcastID, tagID); err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to remove tag from podcast")
	}
	return nil
}

// RemoveTagFromEpisode 直接移除单集标签的公开方法
func (s *TagRelationService) RemoveTagFromEpisode(episodeID, tagID uint) error {
	if err := s.repos.Tag.RemoveTagFromEpisode(episodeID, tagID); err != nil {
		return apperrors.InternalErrorWithErr(err, "Failed to remove tag from episode")
	}
	return nil
}

// TagWithCount 带计数的标签
type TagWithCount struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Count int    `json:"podcast_count"` // 保持字段名兼容性
}

// GetTags 获取目标的所有标签
func (s *TagRelationService) GetTags(targetType TargetType, targetID uint) ([]TagWithCount, error) {
	var tags []models.Tag

	// 根据目标类型加载标签
	switch targetType {
	case TargetTypePodcast:
		podcast, err := s.repos.Podcast.GetWithTags(targetID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, apperrors.NotFoundError("podcast", targetID)
			}
			return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch podcast tags")
		}
		tags = podcast.Tags

	case TargetTypeEpisode:
		// 使用 EpisodeRepository 的 GetWithTags 方法
		episode, err := s.repos.Episode.GetWithTags(targetID)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, apperrors.NotFoundError("episode", targetID)
			}
			return nil, apperrors.InternalErrorWithErr(err, "Failed to fetch episode tags")
		}
		tags = episode.Tags

	default:
		return nil, apperrors.BadRequestError("unsupported target type: " + string(targetType))
	}

	// 转换为带计数的格式
	tagsWithCount := make([]TagWithCount, len(tags))

	// 批量获取所有标签的播客数量，避免N+1查询
	tagIDs := make([]uint, len(tags))
	for i, tag := range tags {
		tagIDs[i] = tag.ID
	}

	countMap, err := s.repos.Tag.GetPodcastCountsBatch(tagIDs)
	if err != nil {
		// 如果批量查询失败，使用0作为计数
		for i, tag := range tags {
			tagsWithCount[i] = TagWithCount{
				ID:    tag.ID,
				Name:  tag.Name,
				Color: tag.Color,
				Count: 0,
			}
		}
		return tagsWithCount, nil
	}

	// 填充结果
	for i, tag := range tags {
		tagsWithCount[i] = TagWithCount{
			ID:    tag.ID,
			Name:  tag.Name,
			Color: tag.Color,
			Count: int(countMap[tag.ID]),
		}
	}

	return tagsWithCount, nil
}
