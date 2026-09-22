package handlers

import (
	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"
	"magicpodcast/internal/workflow"

	"gorm.io/gorm"
)

// resolveWorkflowCoverage 解析工作流在给定启用状态下的覆盖节目集合；停用
// 状态覆盖为空集（历史同步的自动触发只认启用覆盖，#462）。
func resolveWorkflowCoverage(db *gorm.DB, wf *models.Workflow, enabled bool) (map[uint]struct{}, error) {
	if !enabled {
		return map[uint]struct{}{}, nil
	}
	return workflow.ResolvedCoveragePodcastIDs(db, wf)
}

// registerHistorySyncForCoverageChange 在工作流覆盖入口（新建/编辑/启用）变化
// 后登记历史同步任务：只对本次变化新增覆盖的节目登记；「指定节目」范围是
// 显式选择，不受水位线限制；全部订阅/自定义源范围受水位线过滤，避免发版后
// 把旧库节目全部卷入回填（#462）。登记后通知管理器入队（无管理器时由恢复
// /补偿扫描兜底）。
func registerHistorySyncForCoverageChange(db *gorm.DB, oldCoverage, newCoverage map[uint]struct{}, newScopeType models.WorkflowScopeType) ([]uint, error) {
	if len(newCoverage) == 0 {
		return nil, nil
	}
	newlyEntered := make([]uint, 0, len(newCoverage))
	for id := range newCoverage {
		if _, was := oldCoverage[id]; !was {
			newlyEntered = append(newlyEntered, id)
		}
	}
	if len(newlyEntered) == 0 {
		return nil, nil
	}

	watermarkFilter := newScopeType != models.ScopeTypeSpecificPodcasts
	_, err := syncsvc.EnsureHistoryTasks(db, newlyEntered, models.HistorySyncTriggerWorkflow, watermarkFilter)
	return newlyEntered, err
}

// saveWorkflowWithHistory atomically persists a scope change and its initial tasks.
// Notify only after commit: workers cannot observe an uncommitted membership.
func saveWorkflowWithHistory(db *gorm.DB, oldWorkflow, newWorkflow *models.Workflow, save func(*gorm.DB) error) error {
	var ids []uint
	err := db.Transaction(func(tx *gorm.DB) error {
		oldCoverage := map[uint]struct{}{}
		if oldWorkflow != nil {
			var err error
			oldCoverage, err = resolveWorkflowCoverage(tx, oldWorkflow, oldWorkflow.IsEnabled)
			if err != nil {
				return err
			}
		}
		if err := save(tx); err != nil {
			return err
		}
		newCoverage, err := resolveWorkflowCoverage(tx, newWorkflow, newWorkflow.IsEnabled)
		if err != nil {
			return err
		}
		ids, err = registerHistorySyncForCoverageChange(tx, oldCoverage, newCoverage, newWorkflow.ScopeType)
		return err
	})
	if err == nil {
		syncsvc.NotifyHistoryTasksEnqueued(ids)
	}
	return err
}
