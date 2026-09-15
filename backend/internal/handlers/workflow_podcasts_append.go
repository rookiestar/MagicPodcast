package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/database"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// workflowScopeMutationMu 在单 API 进程内串行化工作流成员的读改写。数据库
// 层已串行化写入者，但成员集合必须在持锁后读取，才能保证并发追加与重放
// 不丢成员、不重复写入（#417/#418 AC4）。
var workflowScopeMutationMu sync.Mutex

// AppendWorkflowPodcastsRequest 提交明确选中的节目 ID；不提交整份工作流
// 表单，名称、描述、规则、调度等既有配置一律不改写（#417 决策 6/7）。
type AppendWorkflowPodcastsRequest struct {
	PodcastIDs []int `json:"podcast_ids"`
}

// AppendPodcasts 把明确选中的节目追加到一个未删除的「指定节目」工作流：
// 事务内读取最新成员并做集合追加，重复请求与并发追加不产生重复成员。
// POST /api/v1/workflows/:id/podcasts/append
func (h *WorkflowHandler) AppendPodcasts(c *gin.Context) {
	workflowID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}

	var req AppendWorkflowPodcastsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BadRequestResponse(c, "INVALID_REQUEST_BODY", "请求体必须是包含 podcast_ids 的 JSON")
		return
	}
	requested := make([]int, 0, len(req.PodcastIDs))
	seen := make(map[int]struct{}, len(req.PodcastIDs))
	for _, id := range req.PodcastIDs {
		if id <= 0 {
			middleware.BadRequestResponse(c, "INVALID_PODCAST_IDS", "节目 ID 必须为正整数")
			return
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		requested = append(requested, id)
	}
	if len(requested) == 0 {
		middleware.BadRequestResponse(c, "PODCAST_IDS_REQUIRED", "请至少选择一个要追加的节目")
		return
	}

	workflowScopeMutationMu.Lock()
	defer workflowScopeMutationMu.Unlock()

	var (
		added        int
		alreadyCount int
		podcastCount int
		workflowName string
		scopeChanged bool
	)
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var wf models.Workflow
		if err := tx.First(&wf, workflowID).Error; err != nil {
			return err
		}
		workflowName = wf.Name
		if wf.ScopeType != models.ScopeTypeSpecificPodcasts {
			scopeChanged = true
			return nil
		}

		// 节目资格核对：任一节目不存在（含已删除）即整体拒绝，不产生
		// 未说明的部分追加（#417 决策 9）。
		var memberIDs []uint
		if err := tx.Model(&models.Podcast{}).Where("id IN ?", requested).
			Pluck("id", &memberIDs).Error; err != nil {
			return err
		}
		if len(memberIDs) != len(requested) {
			present := make(map[uint]struct{}, len(memberIDs))
			for _, id := range memberIDs {
				present[id] = struct{}{}
			}
			missing := make([]int, 0)
			for _, id := range requested {
				if _, ok := present[uint(id)]; !ok {
					missing = append(missing, id)
				}
			}
			return podcastEligibilityError{missing: missing}
		}

		existing := wf.ScopeConfig.PodcastIDs
		memberSet := make(map[int]struct{}, len(existing))
		for _, id := range existing {
			memberSet[id] = struct{}{}
		}
		appended := make([]int, 0, len(requested))
		for _, id := range requested {
			if _, ok := memberSet[id]; ok {
				continue
			}
			memberSet[id] = struct{}{}
			appended = append(appended, id)
		}
		alreadyCount = len(requested) - len(appended)
		if len(appended) == 0 {
			podcastCount = len(existing)
			return nil
		}

		merged := make([]int, 0, len(existing)+len(appended))
		merged = append(merged, existing...)
		merged = append(merged, appended...)
		updatedScope := wf.ScopeConfig
		updatedScope.PodcastIDs = merged
		if err := tx.Model(&models.Workflow{}).Where("id = ?", wf.ID).
			Updates(map[string]interface{}{
				"scope_config": updatedScope,
				"updated_at":   time.Now(),
			}).Error; err != nil {
			return err
		}
		added = len(appended)
		podcastCount = len(merged)
		return nil
	})

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			middleware.NotFoundResponse(c, "WORKFLOW_NOT_FOUND", "目标工作流不存在或已删除")
			return
		}
		var eligibilityErr podcastEligibilityError
		if errors.As(err, &eligibilityErr) {
			middleware.BadRequestResponse(c, "PODCAST_NOT_FOUND",
				fmt.Sprintf("节目不存在或已删除，无法追加: %v", eligibilityErr.missing))
			return
		}
		logger.Warnf("追加工作流成员失败 [ID=%d]: %v", workflowID, err)
		middleware.InternalErrorResponseWithCode(c, "INTERNAL_ERROR", "追加工作流成员失败")
		return
	}
	if scopeChanged {
		middleware.BadRequestResponse(c, "SCOPE_NOT_SUPPORTED", "目标工作流不是指定节目范围，无法追加成员")
		return
	}

	if added > 0 {
		cache.InvalidateWorkflowList()
		cache.InvalidateWorkflowDetail(workflowID)
		// 成员变化影响节目覆盖筛选结果（#419），需同步失效列表缓存。
		cache.InvalidatePodcastList()
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"workflow_id":    workflowID,
		"workflow_name":  workflowName,
		"added":          added,
		"already_member": alreadyCount,
		"podcast_count":  podcastCount,
	})
}

// podcastEligibilityError 标记资格核对失败并携带缺失的节目 ID。
type podcastEligibilityError struct {
	missing []int
}

func (e podcastEligibilityError) Error() string {
	return fmt.Sprintf("podcasts not found: %v", e.missing)
}
