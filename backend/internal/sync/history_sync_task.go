package sync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/cache"
	"magicpodcast/internal/feed"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// 历史同步任务的执行边界。单批上限由 EpisodeSyncConfig.MaxEpisodesPerPodcast
// 决定（默认 1000），续批直到来源可获取条目全部处理完成；maxHistorySyncPasses
// 只是防御异常来源（例如每次抓取都改写条目导致永远产生写入）的硬上限。
const (
	historySyncWorkers       = 3
	historySyncQueueSize     = 256
	maxHistorySyncPasses     = 40
	maxHistoryTaskAttempts   = 3
	historySyncSweepInterval = time.Minute
	historySyncRetryFloor    = 30 * time.Second
	historySyncRetryCap      = 15 * time.Minute
	historyTaskErrorLimit    = 1000
)

// historySyncWatermarkKey 是历史同步功能的水位线 KV 键：自动补偿只登记
// 水位线之后入库的节目，避免发版后的首次周期运行把旧库节目全部卷入回填。
const historySyncWatermarkKey = "history_sync_watermark"

// historyCoverageGuard 判断节目当前是否仍被任一启用工作流覆盖。由路由装配
// 注入（覆盖解析的唯一实现在 workflow 包，sync 包不能反向依赖）；为 nil 时
// 仅手动任务可执行。
var (
	historyGuardMu sync.RWMutex
	historyGuard   func(db *gorm.DB, podcastID uint) bool
)

// SetHistoryCoverageGuard 注入启用覆盖判定（router 装配时调用一次）。
func SetHistoryCoverageGuard(guard func(db *gorm.DB, podcastID uint) bool) {
	historyGuardMu.Lock()
	defer historyGuardMu.Unlock()
	historyGuard = guard
}

func coverageAllowsHistorySync(db *gorm.DB, podcastID uint, trigger string) bool {
	if trigger == models.HistorySyncTriggerManual {
		return true
	}
	historyGuardMu.RLock()
	guard := historyGuard
	historyGuardMu.RUnlock()
	if guard == nil {
		return false
	}
	return guard(db, podcastID)
}

// ---------------------------------------------------------------------------
// 任务登记（纯数据库操作，可在事务内调用；不依赖运行中的管理器）
// ---------------------------------------------------------------------------

// EnsureHistoryTasks 为节目补齐缺失的历史同步任务（幂等）：已有活动态任务
// 或任一终态记录的节目不重复登记。watermarkFilter 时跳过水位线前入库的旧
// 节目——旧库完整性默认未知，只允许显式入口或授权回填触碰（#462）。
// 返回本次新创建的任务。
func EnsureHistoryTasks(db *gorm.DB, podcastIDs []uint, trigger string, watermarkFilter bool) ([]models.PodcastHistorySyncTask, error) {
	uniqueIDs := dedupePodcastIDs(podcastIDs)
	if len(uniqueIDs) == 0 {
		return nil, nil
	}

	if watermarkFilter {
		watermark, err := HistorySyncWatermark(db)
		if err != nil {
			return nil, err
		}
		var eligible []uint
		if err := db.Model(&models.Podcast{}).Where("id IN ?", uniqueIDs).
			Where("created_at >= ?", watermark).Pluck("id", &eligible).Error; err != nil {
			return nil, fmt.Errorf("filter history sync candidates by watermark: %w", err)
		}
		uniqueIDs = eligible
		if len(uniqueIDs) == 0 {
			return nil, nil
		}
	}

	var created []models.PodcastHistorySyncTask
	err := db.Transaction(func(tx *gorm.DB) error {
		var existing []models.PodcastHistorySyncTask
		if err := tx.Where("podcast_id IN ?", uniqueIDs).Find(&existing).Error; err != nil {
			return err
		}
		hasTask := make(map[uint]bool, len(existing))
		for _, task := range existing {
			hasTask[task.PodcastID] = true
		}
		for _, id := range uniqueIDs {
			if hasTask[id] {
				continue
			}
			task := models.PodcastHistorySyncTask{
				PodcastID: id,
				Trigger:   trigger,
				Status:    models.HistorySyncStatusPending,
			}
			if err := tx.Create(&task).Error; err != nil {
				if isUniqueConstraintErr(err) {
					// 并发登记撞上部分唯一索引：任务已存在，视为幂等成功。
					continue
				}
				return err
			}
			created = append(created, task)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// StartManualHistoryTask 手动同步入口（启动与重试共用）：活动态任务直接复用
// （重复点击/刷新不重复建任务）；最新终态为 failed 的任务原样复位重试，任务
// 标识保持稳定；其余情况创建新任务（已完成节目再次手动检查来源）。
func StartManualHistoryTask(db *gorm.DB, podcastID uint) (*models.PodcastHistorySyncTask, bool, error) {
	var task models.PodcastHistorySyncTask
	created := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var active models.PodcastHistorySyncTask
		err := tx.Where("podcast_id = ? AND status IN ?", podcastID, models.HistorySyncActiveStatuses()).
			Order("id DESC").First(&active).Error
		if err == nil {
			task = active
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var latest models.PodcastHistorySyncTask
		err = tx.Where("podcast_id = ?", podcastID).Order("id DESC").First(&latest).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			latest = models.PodcastHistorySyncTask{}
		} else if err != nil {
			return err
		}

		if latest.ID != 0 && latest.Status == models.HistorySyncStatusFailed {
			updates := map[string]interface{}{
				"status":        models.HistorySyncStatusQueued,
				"next_retry_at": nil,
				"finished_at":   nil,
				"error_message": "",
				"updated_at":    time.Now(),
			}
			if err := tx.Model(&models.PodcastHistorySyncTask{}).Where("id = ?", latest.ID).
				Updates(updates).Error; err != nil {
				return err
			}
			latest.Status = models.HistorySyncStatusQueued
			latest.NextRetryAt = nil
			latest.FinishedAt = nil
			latest.ErrorMessage = ""
			task = latest
			return nil
		}

		newTask := models.PodcastHistorySyncTask{
			PodcastID: podcastID,
			Trigger:   models.HistorySyncTriggerManual,
			Status:    models.HistorySyncStatusPending,
		}
		if err := tx.Create(&newTask).Error; err != nil {
			if isUniqueConstraintErr(err) {
				// 并发点击：另一个请求刚创建了活动任务，返回既有任务。
				var race models.PodcastHistorySyncTask
				if lookupErr := tx.Where("podcast_id = ? AND status IN ?", podcastID, models.HistorySyncActiveStatuses()).
					Order("id DESC").First(&race).Error; lookupErr == nil {
					task = race
					return nil
				}
			}
			return err
		}
		task = newTask
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return &task, created, nil
}

// LatestHistoryTaskForPodcast 返回节目最新一条历史同步任务；不存在时返回 nil。
func LatestHistoryTaskForPodcast(db *gorm.DB, podcastID uint) (*models.PodcastHistorySyncTask, error) {
	var task models.PodcastHistorySyncTask
	err := db.Where("podcast_id = ?", podcastID).Order("id DESC").First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// HistorySyncTasksByPodcast 批量返回每个节目的最新一条历史同步任务。
func HistorySyncTasksByPodcast(db *gorm.DB, podcastIDs []uint) (map[uint]*models.PodcastHistorySyncTask, error) {
	result := make(map[uint]*models.PodcastHistorySyncTask, len(podcastIDs))
	if len(podcastIDs) == 0 {
		return result, nil
	}
	var tasks []models.PodcastHistorySyncTask
	if err := db.Where("podcast_id IN ?", podcastIDs).Order("id ASC").Find(&tasks).Error; err != nil {
		return nil, err
	}
	for i := range tasks {
		result[tasks[i].PodcastID] = &tasks[i]
	}
	return result, nil
}

// HistorySyncWatermark 读取历史同步水位线；不存在时以当前时间创建。
func HistorySyncWatermark(db *gorm.DB) (time.Time, error) {
	var cfg models.SyncConfig
	err := db.Where("config_key = ?", historySyncWatermarkKey).First(&cfg).Error
	if err == nil {
		parsed, parseErr := time.Parse(time.RFC3339Nano, cfg.ConfigValue)
		if parseErr == nil {
			return parsed, nil
		}
		return time.Time{}, fmt.Errorf("parse history sync watermark %q: %w", cfg.ConfigValue, parseErr)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, err
	}
	now := time.Now()
	if err := db.Create(&models.SyncConfig{
		ConfigKey:   historySyncWatermarkKey,
		ConfigValue: now.Format(time.RFC3339Nano),
	}).Error; err != nil {
		// 并发初始化：唯一索引冲突时回读既有值。
		var existing models.SyncConfig
		if readErr := db.Where("config_key = ?", historySyncWatermarkKey).First(&existing).Error; readErr == nil {
			parsed, parseErr := time.Parse(time.RFC3339Nano, existing.ConfigValue)
			if parseErr == nil {
				return parsed, nil
			}
		}
		return time.Time{}, err
	}
	return now, nil
}

func dedupePodcastIDs(podcastIDs []uint) []uint {
	uniqueIDs := make([]uint, 0, len(podcastIDs))
	seen := make(map[uint]struct{}, len(podcastIDs))
	for _, id := range podcastIDs {
		if id == 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	return uniqueIDs
}

func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToUpper(err.Error()), "UNIQUE CONSTRAINT")
}

// ---------------------------------------------------------------------------
// 管理器：有界并发执行、重启恢复与有限自动重试
// ---------------------------------------------------------------------------

// HistorySyncManager 在进程内以有界并发执行节目历史同步任务。任务行是唯一
// 事实来源：管理器只负责领取、执行与收尾，重启后由恢复扫描继续。
type HistorySyncManager struct {
	db  *gorm.DB
	svc *Service

	policy feed.RetryPolicy

	queue  chan uint
	stopCh chan struct{}
	wg     sync.WaitGroup

	stopOnce  sync.Once
	startOnce sync.Once

	// sleep 可注入；为 nil 时等待真实退避时长并响应停止/取消。
	sleep func(d time.Duration)
}

var (
	historyManagerMu   sync.RWMutex
	historyManagerInst *HistorySyncManager
)

// newHistorySyncManager 构造管理器实例（不启动）。测试可注入 sleep 后再 start。
func newHistorySyncManager(db *gorm.DB, svc *Service) *HistorySyncManager {
	return &HistorySyncManager{
		db:     db,
		svc:    svc,
		policy: feed.SharedRetryPolicy(),
		queue:  make(chan uint, historySyncQueueSize),
		stopCh: make(chan struct{}),
	}
}

// StartHistorySyncManager 启动管理器：恢复中断任务并开始消费队列。重复调用
// 返回既有实例。
func StartHistorySyncManager(db *gorm.DB, svc *Service) *HistorySyncManager {
	historyManagerMu.Lock()
	if historyManagerInst != nil {
		defer historyManagerMu.Unlock()
		return historyManagerInst
	}
	m := newHistorySyncManager(db, svc)
	historyManagerInst = m
	historyManagerMu.Unlock()

	m.start()
	return m
}

// start 启动恢复扫描、执行 worker 与周期补偿扫描（幂等）。
func (m *HistorySyncManager) start() {
	m.startOnce.Do(func() {
		m.resumeInterrupted()
		for i := 0; i < historySyncWorkers; i++ {
			m.wg.Add(1)
			go m.workerLoop()
		}
		m.wg.Add(1)
		go m.sweepLoop()
	})
}

// Stop 停止管理器：运行中的执行被取消且不写终态，任务行保持 running，由
// 下一次启动恢复。
func (m *HistorySyncManager) Stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() { close(m.stopCh) })
	m.wg.Wait()
}

// StopHistorySyncManager 停止并清除当前实例（测试与进程收尾用）。
func StopHistorySyncManager() {
	historyManagerMu.Lock()
	m := historyManagerInst
	historyManagerInst = nil
	historyManagerMu.Unlock()
	m.Stop()
}

// NotifyHistoryTasksEnqueued 把新登记的节目推入执行队列（无管理器时静默，
// 由恢复/补偿扫描兜底）。
func NotifyHistoryTasksEnqueued(podcastIDs []uint) {
	historyManagerMu.RLock()
	m := historyManagerInst
	historyManagerMu.RUnlock()
	if m == nil {
		return
	}
	for _, id := range podcastIDs {
		m.enqueue(id)
	}
}

func (m *HistorySyncManager) enqueue(podcastID uint) {
	if podcastID == 0 {
		return
	}
	select {
	case m.queue <- podcastID:
	case <-m.stopCh:
	default:
		// 队列满：任务行仍在，恢复扫描会重新入队。
	}
}

func (m *HistorySyncManager) workerLoop() {
	defer m.wg.Done()
	for {
		select {
		case <-m.stopCh:
			return
		case podcastID := <-m.queue:
			m.processPodcast(podcastID)
		}
	}
}

func (m *HistorySyncManager) sweepLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(historySyncSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.sweepRetryDue()
		}
	}
}

// resumeInterrupted 启动恢复：上个进程遗留的 running 行回退为 queued，并把
// 全部活动任务入队（有界：一次恢复至多 1000 条，其余随周期扫描继续）。
func (m *HistorySyncManager) resumeInterrupted() {
	if _, err := HistorySyncWatermark(m.db); err != nil {
		logger.Warnf("初始化历史同步水位线失败: %v", err)
	}
	if err := m.db.Model(&models.PodcastHistorySyncTask{}).
		Where("status = ?", models.HistorySyncStatusRunning).
		Updates(map[string]interface{}{"status": models.HistorySyncStatusQueued, "updated_at": time.Now()}).Error; err != nil {
		logger.Warnf("恢复中断的历史同步任务失败: %v", err)
	}
	m.enqueueActiveTasks(1000)
}

// sweepRetryDue 周期扫描：把到达重试时间且仍有自动重试预算的失败任务重新
// 入队；同时兜底入队尚未领取的活动任务。
func (m *HistorySyncManager) sweepRetryDue() {
	now := time.Now()
	var due []models.PodcastHistorySyncTask
	if err := m.db.
		Where("status = ? AND next_retry_at IS NOT NULL AND next_retry_at <= ? AND attempts < ?",
			models.HistorySyncStatusFailed, now, maxHistoryTaskAttempts).
		Limit(50).Find(&due).Error; err != nil {
		logger.Warnf("扫描待重试历史同步任务失败: %v", err)
		return
	}
	for _, task := range due {
		if !coverageAllowsHistorySync(m.db, task.PodcastID, task.Trigger) {
			continue
		}
		if err := m.db.Model(&models.PodcastHistorySyncTask{}).Where("id = ?", task.ID).
			Updates(map[string]interface{}{
				"status":        models.HistorySyncStatusQueued,
				"next_retry_at": nil,
				"finished_at":   nil,
				"updated_at":    now,
			}).Error; err != nil {
			continue
		}
		m.enqueue(task.PodcastID)
	}
	m.enqueueActiveTasks(200)
}

func (m *HistorySyncManager) enqueueActiveTasks(limit int) {
	var tasks []models.PodcastHistorySyncTask
	if err := m.db.Where("status IN ?", []string{
		models.HistorySyncStatusPending, models.HistorySyncStatusQueued,
	}).Order("id ASC").Limit(limit).Find(&tasks).Error; err != nil {
		logger.Warnf("加载活动历史同步任务失败: %v", err)
		return
	}
	for _, task := range tasks {
		m.enqueue(task.PodcastID)
	}
}

// claimHistoryTask 领取任务：数据库层原子流转 pending/queued → running，
// 竞争失败者直接放弃。工作流触发的任务在领取时重新核对启用覆盖，停用
// 工作流不再自动调度（保留任务行，启用后继续）。
func (m *HistorySyncManager) claimHistoryTask(task *models.PodcastHistorySyncTask) bool {
	res := m.db.Model(&models.PodcastHistorySyncTask{}).
		Where("id = ? AND status IN ?", task.ID, []string{
			models.HistorySyncStatusPending, models.HistorySyncStatusQueued,
		}).
		Updates(map[string]interface{}{
			"status":      models.HistorySyncStatusRunning,
			"attempts":    gorm.Expr("attempts + 1"),
			"started_at":  time.Now(),
			"finished_at": nil,
			"updated_at":  time.Now(),
		})
	if res.Error != nil || res.RowsAffected == 0 {
		return false
	}
	if !coverageAllowsHistorySync(m.db, task.PodcastID, task.Trigger) {
		// 回退为待同步并归还本次领取：停用覆盖下的任务保持等待。
		m.db.Model(&models.PodcastHistorySyncTask{}).Where("id = ?", task.ID).
			Updates(map[string]interface{}{
				"status":     models.HistorySyncStatusPending,
				"attempts":   gorm.Expr("attempts - 1"),
				"started_at": nil,
				"updated_at": time.Now(),
			})
		return false
	}
	task.Status = models.HistorySyncStatusRunning
	task.Attempts++
	return true
}

func (m *HistorySyncManager) processPodcast(podcastID uint) {
	var task models.PodcastHistorySyncTask
	err := m.db.Where("podcast_id = ? AND status IN ?", podcastID, models.HistorySyncActiveStatuses()).
		Order("id DESC").First(&task).Error
	if err != nil {
		return
	}
	if !m.claimHistoryTask(&task) {
		return
	}
	// 领取即失效列表/详情/单集缓存：卡片与详情的同步状态转为「同步中」，
	// 已入库内容在同步过程中即可查看。
	cache.InvalidatePodcastDetail(podcastID)
	cache.InvalidatePodcastList()
	cache.InvalidateEpisodeList(podcastID)

	// ctx 只绑定管理器生命周期：进程停止时取消执行且不写终态，任务行由
	// 下一次启动恢复；页面关闭不影响后台同步。
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		select {
		case <-m.stopCh:
		case <-done:
		}
	}()
	defer close(done)
	m.executeTask(ctx, &task)
}

type historyTaskProgress struct {
	mu         sync.Mutex
	db         *gorm.DB
	taskID     uint
	processed  int
	totalKnown *int
	lastFlush  time.Time
}

func (p *historyTaskProgress) persist(force bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if !force && now.Sub(p.lastFlush) < time.Second {
		return
	}
	p.lastFlush = now
	updates := map[string]interface{}{
		"processed_count": p.processed,
		"updated_at":      now,
	}
	if p.totalKnown != nil {
		updates["total_known"] = *p.totalKnown
	}
	p.db.Model(&models.PodcastHistorySyncTask{}).Where("id = ?", p.taskID).Updates(updates)
}

// historyTaskReporter 把单集同步窗口进度持久化到任务行（节流 1 秒）。
// current 是本次执行已按序核对的前缀长度（跨续批单调递增），total 是来源
// 返回的条目总数；总量未知时前端只展示已处理数量。
type historyTaskReporter struct {
	progress *historyTaskProgress
}

func (r *historyTaskReporter) Report(message string)        {}
func (r *historyTaskReporter) ReportSuccess(message string) {}
func (r *historyTaskReporter) ReportError(message string)   {}

func (r *historyTaskReporter) ReportSkip(reason SkipReason, message string) {}

func (r *historyTaskReporter) ReportProgress(current, total int, message string) {
	r.progress.mu.Lock()
	if current > r.progress.processed {
		r.progress.processed = current
	}
	if total > 0 {
		t := total
		r.progress.totalKnown = &t
	}
	r.progress.mu.Unlock()
	r.progress.persist(false)
}

func (r *historyTaskReporter) ReportSummary(summary *SyncSummary) {}
func (r *historyTaskReporter) Close()                             {}

// executeTask 执行一次任务：续批循环直到来源范围全部处理完成（Incomplete
// 为假）或达到续批硬上限；抓取失败按既有重试策略在执行内有限退避，预算
// 耗尽后落为 failed 并由自动重试扫描按任务级预算继续。
func (m *HistorySyncManager) executeTask(ctx context.Context, task *models.PodcastHistorySyncTask) {
	progress := &historyTaskProgress{db: m.db, taskID: task.ID}
	reporter := &historyTaskReporter{progress: progress}

	config := DefaultEpisodeSyncConfig
	config.Mode = SyncModeFull
	config.UpdateExisting = true

	var (
		frontier   int
		created    int
		updated    int
		failed     int
		lastErr    error
		lastResult *EpisodeSyncResult
		passesUsed int
	)

	for pass := 0; pass < maxHistorySyncPasses; pass++ {
		if ctx.Err() != nil {
			break
		}
		passesUsed = pass + 1
		result, err := m.svc.SyncPodcastEpisodesWithContext(ctx, task.PodcastID, reporter, config)
		if result != nil {
			if frontierSum := result.Created + result.Updated + result.Skipped; frontierSum > frontier {
				frontier = frontierSum
			}
			created += result.Created
			updated += result.Updated
			failed += result.Errors
		}
		progress.mu.Lock()
		progress.processed = frontier
		progress.mu.Unlock()
		progress.persist(true)

		if err != nil {
			lastErr = err
			// 执行内有限退避：复用既有重试策略（Retry-After 优先），仅对
			// 可重试错误继续，非可重试错误（403/404/解析失败）立即停止。
			if m.policy.ShouldRetry(err) && pass+1 < maxHistorySyncPasses {
				if delay, ok := m.policy.NextDelay(err, pass); ok {
					if !m.wait(ctx, delay) {
						return // 进程停止：不写终态，保持 running 由启动恢复
					}
					continue
				}
			}
			break
		}
		lastResult = result
		if result != nil && !result.Incomplete {
			break
		}
		if pass+1 >= maxHistorySyncPasses {
			break
		}
	}

	// 来源范围是否全部处理完成以最后一次执行的结果为准：中途 pass 的
	// 截断不能否定后续 pass 的完整覆盖。
	incomplete := lastErr == nil && (lastResult == nil || lastResult.Incomplete)

	// 进程停止导致的取消不写终态：任务行保持 running，由下一次启动恢复，
	// 已写入数据保留，未处理条目后续补齐。
	if errors.Is(lastErr, context.Canceled) && m.stopped() {
		logger.Infof("历史同步因进程停止暂停 [podcast=%d task=%d]，已核对 %d 条保留",
			task.PodcastID, task.ID, frontier)
		return
	}

	m.finalizeTask(task, taskOutcome{
		frontier:   frontier,
		created:    created,
		updated:    updated,
		failed:     failed,
		err:        lastErr,
		incomplete: incomplete,
		passes:     passesUsed,
	})
}

func (m *HistorySyncManager) stopped() bool {
	select {
	case <-m.stopCh:
		return true
	default:
		return false
	}
}

// wait 等待退避时长；进程停止或执行取消时提前返回 false。
func (m *HistorySyncManager) wait(ctx context.Context, d time.Duration) bool {
	if m.sleep != nil {
		m.sleep(d)
		return ctx.Err() == nil
	}
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-m.stopCh:
		return false
	case <-timer.C:
		return true
	}
}

type taskOutcome struct {
	frontier   int
	created    int
	updated    int
	failed     int
	err        error
	incomplete bool
	passes     int
}

// finalizeTask 收尾：只有来源范围全部成功处理才写完成标记；截断、写入失败
// 或取消都不得提前声称历史完整（#462）。
func (m *HistorySyncManager) finalizeTask(task *models.PodcastHistorySyncTask, outcome taskOutcome) {
	now := time.Now()
	updates := map[string]interface{}{
		"processed_count": outcome.frontier,
		"created_count":   outcome.created,
		"updated_count":   outcome.updated,
		"failed_count":    outcome.failed,
		"finished_at":     now,
		"updated_at":      now,
	}

	var (
		status     string
		errMessage string
		sourceNote string
	)

	switch {
	case outcome.err != nil:
		status = models.HistorySyncStatusFailed
		errMessage = truncateHistoryError(outcome.err)
		sourceNote = "同步失败，已写入部分已保留"
	case outcome.incomplete:
		status = models.HistorySyncStatusPartial
		sourceNote = "本次来源范围未全部处理完成，已写入部分已保留"
	default:
		status = models.HistorySyncStatusCompleted
		sourceNote = m.completedSourceNote(task.PodcastID, outcome.frontier)
	}

	// 自动重试预算：失败（可重试错误）或部分完成且尝试未耗尽时安排退避
	// 重试；否则清除重试时间，仅保留手动重试入口。
	retryableFailure := outcome.err != nil && m.policy.ShouldRetry(outcome.err)
	if (retryableFailure || (outcome.err == nil && outcome.incomplete)) && task.Attempts < maxHistoryTaskAttempts {
		updates["next_retry_at"] = m.nextRetryTime(task.Attempts, outcome.err)
	} else {
		updates["next_retry_at"] = nil
	}

	updates["status"] = status
	updates["error_message"] = errMessage
	updates["source_note"] = sourceNote

	if err := m.db.Model(&models.PodcastHistorySyncTask{}).Where("id = ?", task.ID).
		Updates(updates).Error; err != nil {
		logger.Warnf("写历史同步任务终态失败 [task=%d]: %v", task.ID, err)
	}
	// 已提交结果失效全部相关缓存：集数以本地实际入库为准（#462）。
	cache.InvalidatePodcastDetail(task.PodcastID)
	cache.InvalidatePodcastList()
	cache.InvalidateEpisodeList(task.PodcastID)

	if status == models.HistorySyncStatusCompleted {
		logger.Infof("历史同步完成 [podcast=%d task=%d 续批=%d 新增=%d 更新=%d 已核对=%d]",
			task.PodcastID, task.ID, outcome.passes, outcome.created, outcome.updated, outcome.frontier)
	} else {
		logger.Warnf("历史同步未完成 [podcast=%d task=%d status=%s err=%v]",
			task.PodcastID, task.ID, status, outcome.err)
	}
}

// nextRetryTime 计算任务级自动重试时间：下限 30 秒、指数退避、上限 15 分钟；
// 上游明确给出 Retry-After 时优先尊重。
func (m *HistorySyncManager) nextRetryTime(attempts int, err error) time.Time {
	if err != nil {
		if delay, ok := m.policy.NextDelay(err, attempts); ok {
			if delay < historySyncRetryFloor {
				delay = historySyncRetryFloor
			}
			if delay > historySyncRetryCap {
				delay = historySyncRetryCap
			}
			return time.Now().Add(delay)
		}
	}
	backoff := historySyncRetryFloor << attempts
	if backoff > historySyncRetryCap || backoff <= 0 {
		backoff = historySyncRetryCap
	}
	return time.Now().Add(backoff)
}

// completedSourceNote 生成完成状态的来源覆盖说明：合法空源、源站总量差异
// 与常规完成分别如实表达。
func (m *HistorySyncManager) completedSourceNote(podcastID uint, frontier int) string {
	if frontier == 0 {
		return "源当前未提供可获取单集"
	}
	var podcast models.Podcast
	if err := m.db.Select("id", "external_episode_count", "episode_count").
		First(&podcast, podcastID).Error; err == nil {
		if podcast.ExternalEpisodeCount > 0 && podcast.ExternalEpisodeCount > podcast.EpisodeCount {
			return fmt.Sprintf("已入库来源当前可获取的全部单集；源站报告共 %d 集，来源历史完整性未知",
				podcast.ExternalEpisodeCount)
		}
	}
	return "已入库来源当前可获取的全部单集"
}

func truncateHistoryError(err error) string {
	msg := err.Error()
	if len(msg) > historyTaskErrorLimit {
		msg = msg[:historyTaskErrorLimit]
	}
	return msg
}
