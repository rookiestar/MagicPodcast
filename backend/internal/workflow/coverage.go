package workflow

import (
	"strings"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// WorkflowRef 是覆盖归属的最小工作流信息，供导入结果批量补入（#418）与
// 新建向导覆盖筛选（#419）共用。
type WorkflowRef struct {
	ID        uint                     `json:"id"`
	Name      string                   `json:"name"`
	ScopeType models.WorkflowScopeType `json:"scope_type"`
	IsEnabled bool                     `json:"is_enabled"`
}

// coverageIndex 按「被未删除的其他工作流纳入选源范围」口径（#417 决策 11）
// 建立工作流索引：停用但未删除的工作流计入，已删除的不计入。范围规则与
// executor.getTargetPodcasts 保持一致：指定节目按成员 ID；全部已订阅按
// is_subscribed；自定义源按与本地节目可明确对应的 Feed 地址（同执行器
// feed_url 精确匹配，含 http/https 互换），不做标题近似猜测。
type coverageIndex struct {
	specific      map[uint][]WorkflowRef
	allSubscribed []WorkflowRef
	custom        map[string][]WorkflowRef
}

func loadCoverageIndex(db *gorm.DB) (*coverageIndex, error) {
	var workflows []models.Workflow
	if err := db.Find(&workflows).Error; err != nil {
		return nil, err
	}
	index := &coverageIndex{
		specific: map[uint][]WorkflowRef{},
		custom:   map[string][]WorkflowRef{},
	}
	for i := range workflows {
		ref := WorkflowRef{
			ID:        workflows[i].ID,
			Name:      workflows[i].Name,
			ScopeType: workflows[i].ScopeType,
			IsEnabled: workflows[i].IsEnabled,
		}
		switch workflows[i].ScopeType {
		case models.ScopeTypeSpecificPodcasts:
			for _, id := range workflows[i].ScopeConfig.PodcastIDs {
				if id <= 0 {
					continue
				}
				key := uint(id)
				index.specific[key] = append(index.specific[key], ref)
			}
		case models.ScopeTypeAllSubscribed:
			index.allSubscribed = append(index.allSubscribed, ref)
		case models.ScopeTypeCustomSources:
			for _, rawURL := range workflows[i].ScopeConfig.CustomURLs {
				for _, key := range coverageFeedURLKeys(rawURL) {
					index.custom[key] = append(index.custom[key], ref)
				}
			}
		}
	}
	return index, nil
}

// attribute 返回一个节目被哪些工作流覆盖（同一工作流只出现一次）。
func (idx *coverageIndex) attribute(podcast models.Podcast) []WorkflowRef {
	refs := make([]WorkflowRef, 0)
	seen := map[uint]struct{}{}
	appendRef := func(list []WorkflowRef) {
		for _, ref := range list {
			if _, dup := seen[ref.ID]; dup {
				continue
			}
			seen[ref.ID] = struct{}{}
			refs = append(refs, ref)
		}
	}
	appendRef(idx.specific[podcast.ID])
	if podcast.IsSubscribed {
		appendRef(idx.allSubscribed)
	}
	for _, key := range coverageFeedURLKeys(podcast.FeedURL) {
		appendRef(idx.custom[key])
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

// coverageFeedURLKeys 生成 Feed 地址的精确匹配键：原始地址与 http/https
// 互换地址（与导入索引查询、执行器取数对同一地址的语义一致）。空地址与
// 非 http(s) 地址不参与匹配。
func coverageFeedURLKeys(rawURL string) []string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "https://"):
		return []string{trimmed, "http://" + trimmed[len("https://"):]}
	case strings.HasPrefix(lower, "http://"):
		return []string{trimmed, "https://" + trimmed[len("http://"):]}
	default:
		return nil
	}
}

// CoverageForPodcasts 返回每个节目 ID 被哪些未删除工作流覆盖（#417 共享
// 覆盖契约）。节目不存在或未被覆盖时键缺省。
func CoverageForPodcasts(db *gorm.DB, podcastIDs []uint) (map[uint][]WorkflowRef, error) {
	result := map[uint][]WorkflowRef{}
	if len(podcastIDs) == 0 {
		return result, nil
	}
	index, err := loadCoverageIndex(db)
	if err != nil {
		return nil, err
	}
	var podcasts []models.Podcast
	if err := db.Where("id IN ?", podcastIDs).Find(&podcasts).Error; err != nil {
		return nil, err
	}
	for i := range podcasts {
		if refs := index.attribute(podcasts[i]); len(refs) > 0 {
			result[podcasts[i].ID] = refs
		}
	}
	return result, nil
}

// CoveredPodcastIDs 返回被任一未删除工作流覆盖的节目 ID 集合，供节目列表
// 查询在分页前过滤（#419）。
func CoveredPodcastIDs(db *gorm.DB) (map[uint]struct{}, error) {
	index, err := loadCoverageIndex(db)
	if err != nil {
		return nil, err
	}
	var podcasts []models.Podcast
	if err := db.Select("id", "is_subscribed", "feed_url").Find(&podcasts).Error; err != nil {
		return nil, err
	}
	covered := make(map[uint]struct{}, len(podcasts))
	for i := range podcasts {
		if len(index.attribute(podcasts[i])) > 0 {
			covered[podcasts[i].ID] = struct{}{}
		}
	}
	return covered, nil
}
