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
func (k *KeyBuilder) PodcastList(page, pageSize int, sortBy string, tagIDs []uint, search string, view ...string) string {
	var sb strings.Builder
	sb.WriteString("podcasts:list:")
	sb.WriteString(fmt.Sprintf("p%d:s%d:", page, pageSize))
	sb.WriteString(sortBy)
	sb.WriteString(":")

	if len(view) > 0 && view[0] != "" {
		sb.WriteString("v:")
		sb.WriteString(view[0])
		sb.WriteString(":")
	}

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

// TagList 构建标签列表缓存键
func (k *KeyBuilder) TagList() string {
	return "tags:list"
}

// WorkflowList 构建工作流列表缓存键
func (k *KeyBuilder) WorkflowList() string {
	return "workflows:list"
}

// EpisodeList 构建单集列表缓存键
func (k *KeyBuilder) EpisodeList(podcastID uint, page, pageSize int, view ...string) string {
	key := fmt.Sprintf("episodes:list:podcast:%d:p%d:s%d", podcastID, page, pageSize)
	if len(view) > 0 && view[0] != "" {
		key += fmt.Sprintf(":v:%s", view[0])
	}
	return key
}
