package handlers

import (
	"errors"
	"net/http"
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
	retryTask, wrapped, err := h.startChildImportTask(
		task.ID,
		"重试任务#"+strconv.Itoa(int(task.ID))+"("+strconv.Itoa(len(outlines))+"条)",
		len(outlines), reporter)
	if err != nil {
		if errors.Is(err, sync.ErrImportTaskAlreadyRunning) {
			middleware.ConflictResponse(c, "IMPORT_TASK_RETRY_RUNNING", "该任务已有重试在后台执行，请等待当前任务完成")
			return
		}
		middleware.InternalErrorResponseWithCode(c, "IMPORT_TASK_ERROR", "重试任务未能创建")
		return
	}
	taskResponse := *retryTask

	// Create the durable child first, then let the import continue after the
	// HTTP response. The browser can immediately poll this task instead of
	// waiting for every slow RSS attempt to finish in one request.
	h.runImportInBackground(outlines, wrapped, decisions, retryTask)

	c.JSON(http.StatusAccepted, gin.H{
		"success":            true,
		"task_id":            retryTask.ID,
		"parent_task_id":     task.ID,
		"status":             models.ImportTaskStatusRunning,
		"message":            "重试任务 #" + strconv.Itoa(int(retryTask.ID)) + " 已创建，正在后台执行",
		"total_podcasts":     len(outlines),
		"success_count":      0,
		"failed_count":       0,
		"stub_podcasts":      0,
		"merged_podcasts":    0,
		"conflict_podcasts":  0,
		"unchanged_podcasts": 0,
		"entries":            []sync.ImportEntryResult{},
		"errors":             []string{},
		"task":               taskResponse,
	})
}
