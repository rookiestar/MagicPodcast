package handlers

import (
	"strconv"

	"magicpodcast/internal/logger"
	"magicpodcast/internal/middleware"
	"magicpodcast/internal/models"
	"magicpodcast/internal/opml"
	"magicpodcast/internal/sync"

	"github.com/gin-gonic/gin"
)

// retryableOutcomes 是允许手动重试的条目结果：仅失败/待同步。成功、
// 未变化与冲突条目不盲目再执行（冲突需要显式确认决策）。
var retryableOutcomes = map[string]struct{}{
	sync.ImportOutcomePending: {},
	sync.ImportOutcomeFailed:  {},
}

// RetryImportTask 仅重试一次导入任务中的失败/待同步条目，引用原任务身份
// 映射；其他节目不重复执行。POST /api/v1/sync/import/tasks/:id/retry
func (h *SyncHandler) RetryImportTask(c *gin.Context) {
	if !middleware.RequireConfirmationText(
		c,
		middleware.ConfirmationTextFromHeaderOrForm(c),
		"RETRY IMPORT",
		"仅重试上次导入中失败或待同步的条目并写入播客订阅数据",
	) {
		return
	}

	taskID, ok := ParseUintParam(c, "id")
	if !ok {
		return
	}
	decisions, err := parseImportDecisions(c.PostForm("decisions"))
	if err != nil {
		middleware.BadRequestResponse(c, "INVALID_DECISIONS", err.Error())
		return
	}

	task, entries, err := sync.GetImportTask(h.db, taskID)
	if err != nil {
		middleware.NotFoundResponse(c, "IMPORT_TASK_NOT_FOUND", "导入任务不存在")
		return
	}

	if task.Status == models.ImportTaskStatusRunning {
		middleware.BadRequestResponse(c, "IMPORT_TASK_RUNNING", "原任务仍在执行，请等待结果")
		return
	}
	outlines := make([]opml.Outline, 0, len(entries))
	for _, entry := range entries {
		_, retryable := retryableOutcomes[entry.Outcome]
		// 抓取后才发现的稳定身份冲突：预览时无法附带确认，重试请求携带
		// 显式 confirm 决策的冲突条目参与重试，保证换址重导可收敛。
		confirmedConflict := entry.Outcome == sync.ImportOutcomeConflict &&
			decisions[entry.FeedURL] == sync.ImportDecisionConfirm
		if retryable || confirmedConflict || (task.Status == models.ImportTaskStatusInterrupted && entry.Outcome == "unprocessed") {
			outlines = append(outlines, opml.Outline{
				Title:  entry.Title,
				XMLURL: entry.FeedURL,
				Type:   "rss",
			})
		}
	}
	if len(outlines) == 0 {
		middleware.BadRequestResponse(c, "NOTHING_TO_RETRY", "该任务中没有失败或待同步的条目，无需重试")
		return
	}

	logger.Infof("重试导入任务 #%d：共 %d 条失败/待同步条目", taskID, len(outlines))
	reporter := sync.NewLogProgressReporter()
	retryTask, wrapped := h.startChildImportTask(
		task.ID,
		"重试任务#"+strconv.Itoa(int(task.ID))+"("+strconv.Itoa(len(outlines))+"条)",
		len(outlines), reporter)
	_ = retryTask
	result, runErr := h.runImport(outlines, wrapped, decisions, retryTask)
	if runErr != nil {
		middleware.InternalErrorResponseWithCode(c, "IMPORT_ERROR", "重试失败: "+runErr.Error())
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"task_id": func() interface{} {
			if retryTask != nil {
				return retryTask.ID
			}
			return nil
		}(),
		"parent_task_id":     task.ID,
		"message":            opmlResultMessage(result.TotalPodcasts, result.SuccessPodcasts, result.StubPodcasts, result.FailedPodcasts),
		"total_podcasts":     result.TotalPodcasts,
		"success_count":      result.SuccessPodcasts,
		"failed_count":       result.FailedPodcasts,
		"stub_podcasts":      result.StubPodcasts,
		"merged_podcasts":    result.MergedPodcasts,
		"conflict_podcasts":  result.ConflictPodcasts,
		"unchanged_podcasts": result.UnchangedPodcasts,
		"entries":            result.Entries,
		"errors":             result.Errors,
	})
}
