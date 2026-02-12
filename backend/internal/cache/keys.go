package cache

import (
	"fmt"
	"strings"
)

// KeyBuilder 缓存键构建器
type KeyBuilder struct{}

// NewKeyBuilder 创建缓存键构建器
func NewKeyBuilder() *KeyBuilder {
	return &KeyBuilder{}
}

// PodcastList 构建播客列表缓存键
func (k *KeyBuilder) PodcastList(page, pageSize int, sortBy string, tagIDs []uint, search string) string {
	var sb strings.Builder
	sb.WriteString("podcasts:list:")
	sb.WriteString(fmt.Sprintf("p%d:s%d:", page, pageSize))
	sb.WriteString(sortBy)
	sb.WriteString(":")

	if len(tagIDs) > 0 {
		sb.WriteString("t")
		for _, id := range tagIDs {
			sb.WriteString(fmt.Sprintf("%d,", id))
		}
		sb.WriteString(":")
	}

	if search != "" {
		sb.WriteString("q:")
		sb.WriteString(search)
	}

	return sb.String()
}

// PodcastDetail 构建播客详情缓存键
func (k *KeyBuilder) PodcastDetail(id uint) string {
	return fmt.Sprintf("podcasts:detail:%d", id)
}

// TagList 构建标签列表缓存键
func (k *KeyBuilder) TagList() string {
	return "tags:list"
}

// TagDetail 构建标签详情缓存键
func (k *KeyBuilder) TagDetail(id uint) string {
	return fmt.Sprintf("tags:detail:%d", id)
}

// Search 构建搜索缓存键
func (k *KeyBuilder) Search(query string, searchType string, page, pageSize int) string {
	return fmt.Sprintf("search:%s:p%d:s%d:q:%s", searchType, page, pageSize, query)
}

// WorkflowList 构建工作流列表缓存键
func (k *KeyBuilder) WorkflowList() string {
	return "workflows:list"
}

// WorkflowDetail 构建工作流详情缓存键
func (k *KeyBuilder) WorkflowDetail(id uint) string {
	return fmt.Sprintf("workflows:detail:%d", id)
}

// EpisodeList 构建单集列表缓存键
func (k *KeyBuilder) EpisodeList(podcastID uint, page, pageSize int) string {
	return fmt.Sprintf("episodes:list:podcast:%d:p%d:s%d", podcastID, page, pageSize)
}

// InvalidatePodcast 使播客相关缓存失效
func (k *KeyBuilder) InvalidatePodcast() []string {
	return []string{
		"podcasts:list",
	}
}

// InvalidateTag 使标签相关缓存失效
func (k *KeyBuilder) InvalidateTag() []string {
	return []string{
		"tags:list",
		"tags:detail",
	}
}

// InvalidateWorkflow 使工作流相关缓存失效
func (k *KeyBuilder) InvalidateWorkflow() []string {
	return []string{
		"workflows:list",
		"workflows:detail",
	}
}
