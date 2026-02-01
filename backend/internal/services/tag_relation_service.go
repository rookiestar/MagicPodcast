package services

import (
	"fmt"

	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// TagRelationService 标签关联服务
type TagRelationService struct {
	db *gorm.DB
}

// NewTagRelationService 创建标签关联服务
func NewTagRelationService() *TagRelationService {
	return &TagRelationService{
		db: database.GetDB(),
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
	db := s.db

	// 验证标签存在
	var tag models.Tag
	if err := db.First(&tag, tagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("标签不存在")
		}
		return nil, fmt.Errorf("查询标签失败: %w", err)
	}

	// 根据目标类型处理
	switch targetType {
	case TargetTypePodcast:
		return s.addTagToPodcast(targetID, tagID, &tag)
	case TargetTypeEpisode:
		return s.addTagToEpisode(targetID, tagID, &tag)
	default:
		return nil, fmt.Errorf("不支持的目标类型: %s", targetType)
	}
}

// addTagToPodcast 为播客添加标签
func (s *TagRelationService) addTagToPodcast(podcastID, tagID uint, tag *models.Tag) (*AddTagResult, error) {
	db := s.db

	// 检查播客是否存在
	var podcast models.Podcast
	if err := db.First(&podcast, podcastID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("播客不存在")
		}
		return nil, fmt.Errorf("查询播客失败: %w", err)
	}

	// 检查关联是否已存在
	var count int64
	db.Model(&podcast).Where("tag_id = ?", tagID).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("该播客已有此标签")
	}

	// 添加关联
	if err := db.Model(&podcast).Association("Tags").Append(tag); err != nil {
		return nil, fmt.Errorf("添加标签失败: %w", err)
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
	db := s.db

	// 检查单集是否存在
	var episode models.Episode
	if err := db.First(&episode, episodeID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("单集不存在")
		}
		return nil, fmt.Errorf("查询单集失败: %w", err)
	}

	// 检查关联是否已存在
	var count int64
	db.Model(&episode).Where("tag_id = ?", tagID).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("该单集已有此标签")
	}

	// 添加关联
	if err := db.Model(&episode).Association("Tags").Append(tag); err != nil {
		return nil, fmt.Errorf("添加标签失败: %w", err)
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
	db := s.db

	// 验证标签存在
	var tag models.Tag
	if err := db.First(&tag, tagID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("标签不存在")
		}
		return fmt.Errorf("查询标签失败: %w", err)
	}

	// 根据目标类型处理
	switch targetType {
	case TargetTypePodcast:
		return s.removeTagFromPodcast(targetID, &tag)
	case TargetTypeEpisode:
		return s.removeTagFromEpisode(targetID, &tag)
	default:
		return fmt.Errorf("不支持的目标类型: %s", targetType)
	}
}

// removeTagFromPodcast 移除播客标签
func (s *TagRelationService) removeTagFromPodcast(podcastID uint, tag *models.Tag) error {
	db := s.db

	// 检查播客是否存在
	var podcast models.Podcast
	if err := db.First(&podcast, podcastID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("播客不存在")
		}
		return fmt.Errorf("查询播客失败: %w", err)
	}

	// 移除关联
	if err := db.Model(&podcast).Association("Tags").Delete(tag); err != nil {
		return fmt.Errorf("移除标签失败: %w", err)
	}

	return nil
}

// removeTagFromEpisode 移除单集标签
func (s *TagRelationService) removeTagFromEpisode(episodeID uint, tag *models.Tag) error {
	db := s.db

	// 检查单集是否存在
	var episode models.Episode
	if err := db.First(&episode, episodeID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("单集不存在")
		}
		return fmt.Errorf("查询单集失败: %w", err)
	}

	// 移除关联
	if err := db.Model(&episode).Association("Tags").Delete(tag); err != nil {
		return fmt.Errorf("移除标签失败: %w", err)
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
	db := s.db

	var tags []models.Tag
	var err error

	// 根据目标类型加载标签
	switch targetType {
	case TargetTypePodcast:
		var podcast models.Podcast
		err = db.Preload("Tags").First(&podcast, targetID).Error
		if err == nil {
			tags = podcast.Tags
		}
	case TargetTypeEpisode:
		var episode models.Episode
		err = db.Preload("Tags").First(&episode, targetID).Error
		if err == nil {
			tags = episode.Tags
		}
	default:
		return nil, fmt.Errorf("不支持的目标类型: %s", targetType)
	}

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// 返回更具体的错误消息
			if targetType == TargetTypePodcast {
				return nil, fmt.Errorf("播客不存在")
			}
			return nil, fmt.Errorf("单集不存在")
		}
		return nil, fmt.Errorf("查询失败: %w", err)
	}

	// 转换为带计数的格式
	tagsWithCount := make([]TagWithCount, len(tags))
	for i, tag := range tags {
		// 查询每个标签的播客数量
		var count int64
		db.Table("podcasts_tags").Where("tag_id = ?", tag.ID).Count(&count)

		tagsWithCount[i] = TagWithCount{
			ID:    tag.ID,
			Name:  tag.Name,
			Color: tag.Color,
			Count: int(count),
		}
	}

	return tagsWithCount, nil
}
