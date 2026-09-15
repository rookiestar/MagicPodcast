package handlers

import (
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"
	"magicpodcast/internal/sync"
	"magicpodcast/internal/workflow"

	"github.com/gin-gonic/gin"
)

// importNewPodcastItem 是导入批次新建节目的响应条目：资料状态取节目当前
// 记录（重试成功后待同步转为就绪），工作流归属来自共享覆盖查询。
type importNewPodcastItem struct {
	ID           uint                   `json:"id"`
	Title        string                 `json:"title"`
	FeedURL      string                 `json:"feed_url"`
	Ready        bool                   `json:"ready"`
	IsSubscribed bool                   `json:"is_subscribed"`
	Workflows    []workflow.WorkflowRef `json:"workflows"`
}

// GetImportTaskNewPodcasts 返回一次导入任务及其重试链实际新建的节目清单，
// 供导入结果页批量补入已有工作流（#417/#418）。只读：不开放运行中任务之外
// 的判断由调用方界面控制，本端点始终返回当前已持久化的创建事实。
// GET /api/v1/sync/import/tasks/:id/new-podcasts
func (h *SyncHandler) GetImportTaskNewPodcasts(c *gin.Context) {
	taskID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	if h.db == nil {
		middleware.NotFoundResponse(c, "IMPORT_TASK_NOT_FOUND", "导入任务不存在")
		return
	}
	if _, _, err := sync.GetImportTask(h.db, taskID); err != nil {
		middleware.NotFoundResponse(c, "IMPORT_TASK_NOT_FOUND", "导入任务不存在")
		return
	}

	ids, err := sync.CollectImportBatchCreatedPodcastIDs(h.db, taskID)
	if err != nil {
		middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "读取导入新建节目失败")
		return
	}

	podcasts := make([]importNewPodcastItem, 0, len(ids))
	if len(ids) > 0 {
		var records []models.Podcast
		if err := h.db.Where("id IN ?", ids).Find(&records).Error; err != nil {
			middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "读取导入新建节目失败")
			return
		}
		byID := make(map[uint]models.Podcast, len(records))
		for _, record := range records {
			byID[record.ID] = record
		}
		// 已删除（失去资格）的节目不进入清单；其余保持创建顺序。
		existingIDs := make([]uint, 0, len(ids))
		for _, id := range ids {
			if _, ok := byID[id]; ok {
				existingIDs = append(existingIDs, id)
			}
		}
		coverage, err := workflow.CoverageForPodcasts(h.db, existingIDs)
		if err != nil {
			middleware.InternalErrorResponseWithCode(c, "DATABASE_ERROR", "读取工作流归属失败")
			return
		}
		for _, id := range existingIDs {
			record := byID[id]
			podcasts = append(podcasts, importNewPodcastItem{
				ID:           record.ID,
				Title:        record.Title,
				FeedURL:      record.FeedURL,
				Ready:        record.FeedURLValid,
				IsSubscribed: record.IsSubscribed,
				Workflows:    coverage[id],
			})
		}
	}

	c.JSON(200, gin.H{
		"success":  true,
		"task_id":  taskID,
		"total":    len(podcasts),
		"podcasts": podcasts,
	})
}
