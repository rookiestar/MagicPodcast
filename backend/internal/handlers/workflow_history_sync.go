package handlers

import (
	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"
	"magicpodcast/internal/workflow"

	"gorm.io/gorm"
)

// resolveWorkflowCoverage 解析工作流在给定启用状态下的覆盖节目集合；停用
// 状态覆盖为空集（历史同步的自动触发只认启用覆盖，#462）。
func resolveWorkflowCoverage(db *gorm.DB, wf *models.Workflow, enabled bool) map[uint]struct{} {
	if !enabled {
		return map[uint]struct{}{}
	}
	resolved, err := workflow.ResolvedCoveragePodcastIDs(db, wf)
	if err != nil {
		logger.Warnf("解析工作流覆盖失败 [ID=%d scope=%s]: %v", wf.ID, wf.ScopeType, err)
		return map[uint]struct{}{}
	}
	return resolved
}

// registerHistorySyncForCoverageChange 在工作流覆盖入口（新建/编辑/启用）变化
// 后登记历史同步任务：只对本次变化新增覆盖的节目登记；「指定节目」范围是
// 显式选择，不受水位线限制；全部订阅/自定义源范围受水位线过滤，避免发版后
// 把旧库节目全部卷入回填（#462）。登记后通知管理器入队（无管理器时由恢复
// /补偿扫描兜底）。
func registerHistorySyncForCoverageChange(db *gorm.DB, oldCoverage, newCoverage map[uint]struct{}, newScopeType models.WorkflowScopeType) {
	if len(newCoverage) == 0 {
		return
	}
	newlyEntered := make([]uint, 0, len(newCoverage))
	for id := range newCoverage {
		if _, was := oldCoverage[id]; !was {
			newlyEntered = append(newlyEntered, id)
		}
	}
	if len(newlyEntered) == 0 {
		return
	}

	watermarkFilter := newScopeType != models.ScopeTypeSpecificPodcasts
	created, err := syncsvc.EnsureHistoryTasks(db, newlyEntered, models.HistorySyncTriggerWorkflow, watermarkFilter)
	if err != nil {
		logger.Warnf("登记覆盖入口历史同步任务失败: %v", err)
		return
	}
	// 新覆盖的节目全部通知入队：既有待同步任务借此在启用后开始执行。
	syncsvc.NotifyHistoryTasksEnqueued(newlyEntered)
	if len(created) > 0 {
		logger.Infof("覆盖变化登记历史同步任务 %d 个（范围=%s 水位线过滤=%v）", len(created), newScopeType, watermarkFilter)
	}
}
