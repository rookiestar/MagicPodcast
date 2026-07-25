package workflow

import (
	"context"
	"errors"
	"fmt"
	"magicpodcast/internal/feed"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/utils"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/models"
	"magicpodcast/internal/notifier"
	syncsvc "magicpodcast/internal/sync"

	"gorm.io/gorm"
)

// Executor 工作流执行器
type Executor struct {
	db         *gorm.DB
	syncSvc    *syncsvc.Service
	notifier   *notifier.EmailNotifier
	summarizer SummarizerInterface

	// batchDuration is the hard networking window for one job (#35/#36).
	// Zero means feed.DefaultBatchDuration (15 minutes).
	batchDuration time.Duration
	// workerConcurrency bounds parallel first-pass/retry workers (default 5).
	// Tests may set 1 to avoid SQLite in-memory lock races.
	workerConcurrency int
	// now / sleep are injectable so batch scheduling tests never wait wall-clock minutes.
	now   func() time.Time
	sleep func(time.Duration)
}

// NewExecutor 创建执行器
func NewExecutor(db *gorm.DB, syncSvc *syncsvc.Service, emailNotifier *notifier.EmailNotifier, summarizer SummarizerInterface) *Executor {
	return &Executor{
		db:         db,
		syncSvc:    syncSvc,
		notifier:   emailNotifier,
		summarizer: summarizer,
	}
}

func (e *Executor) batchWindow() time.Duration {
	if e != nil && e.batchDuration > 0 {
		return e.batchDuration
	}
	return feed.DefaultBatchDuration
}

func (e *Executor) clockNow() time.Time {
	if e != nil && e.now != nil {
		return e.now()
	}
	return time.Now()
}

func (e *Executor) clockSleep(d time.Duration) {
	if d <= 0 {
		return
	}
	if e != nil && e.sleep != nil {
		e.sleep(d)
		return
	}
	time.Sleep(d)
}

// UseInstantBatchClock advances the batch clock without wall-clock waits. Tests
// that drive Execute() must call this (or inject now/sleep) so classified
// retries at minutes 3/8/13 do not block the suite for real minutes.
func (e *Executor) UseInstantBatchClock() {
	if e == nil {
		return
	}
	var mu sync.Mutex
	now := time.Now()
	e.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	e.sleep = func(d time.Duration) {
		mu.Lock()
		now = now.Add(d)
		mu.Unlock()
	}
	if e.batchDuration <= 0 {
		e.batchDuration = feed.DefaultBatchDuration
	}
}

// ExecuteCompensation runs a new 15-minute batch that only retries podcasts
// whose final result on the source Job was failed. It uses the current main
// Feed URLs from the DB, links the new Job bidirectionally, and never
// overwrites the original Job, its successes, or its report (#40).
func (e *Executor) ExecuteCompensation(ctx context.Context, sourceJobID uint) (*models.Job, error) {
	var source models.Job
	if err := e.db.First(&source, sourceJobID).Error; err != nil {
		return nil, fmt.Errorf("load source job: %w", err)
	}
	if err := ValidateCompensationSource(&source); err != nil {
		return nil, err
	}
	failedIDs, err := FailedPodcastIDsFromJob(e.db, source.ID)
	if err != nil {
		return nil, err
	}
	if len(failedIDs) == 0 {
		return nil, fmt.Errorf("%w: no failed podcasts", ErrCompensationNotAllowed)
	}

	var workflow models.Workflow
	if err := e.db.First(&workflow, source.WorkflowID).Error; err != nil {
		return nil, fmt.Errorf("load workflow: %w", err)
	}

	// Re-verify alternatives against current main Feeds for failed podcasts.
	if e.syncSvc != nil {
		for _, id := range failedIDs {
			e.syncSvc.InvalidateAlternativeCache(id)
		}
	}

	job, err := ClaimActiveJob(e.db, workflow.ID, "compensation")
	if err != nil {
		if errors.Is(err, ErrWorkflowJobActive) {
			return nil, fmt.Errorf("%w: workflow_id=%d", ErrWorkflowJobActive, workflow.ID)
		}
		return nil, fmt.Errorf("创建补偿 Job 失败: %w", err)
	}
	job.CompensationOfJobID = &source.ID
	_ = e.db.Model(job).Update("compensation_of_job_id", source.ID).Error
	_ = e.db.Model(&models.Job{}).Where("id = ?", source.ID).
		Update("compensated_by_job_id", job.ID).Error

	// Load current main Feeds for the failed set only.
	var podcasts []models.Podcast
	if err := e.db.Where("id IN ?", failedIDs).Find(&podcasts).Error; err != nil {
		e.updateJobStatus(job, models.JobStatusFailed, err.Error())
		return job, err
	}
	if len(podcasts) == 0 {
		e.finalizeJob(job, []*models.JobExecution{})
		return job, nil
	}

	results := e.executeSync(ctx, &workflow, job, podcasts)
	e.finalizeJob(job, results)

	// Reload links for the response.
	_ = e.db.First(job, job.ID).Error
	return job, nil
}

// Execute 执行工作流
func (e *Executor) Execute(ctx context.Context, workflow *models.Workflow, triggeredBy string) (*models.Job, error) {
	logger.Infof("🚀 开始执行工作流 [ID=%d, Name=%s]", workflow.ID, workflow.Name)

	// 1. DB-level single-active-job claim (manual / cron / compensation share it).
	job, err := ClaimActiveJob(e.db, workflow.ID, triggeredBy)
	if err != nil {
		if errors.Is(err, ErrWorkflowJobActive) {
			return nil, fmt.Errorf("%w: workflow_id=%d", ErrWorkflowJobActive, workflow.ID)
		}
		return nil, fmt.Errorf("创建Job失败: %w", err)
	}

	logger.Infof("✅ Job记录已创建 [ID=%d]", job.ID)

	// 2. 解析配置并获取目标播客
	podcasts, err := e.getTargetPodcasts(workflow)
	if err != nil {
		errMsg := fmt.Sprintf("获取目标播客失败: %v", err)
		logger.Infof("❌ %s", errMsg)
		e.updateJobStatus(job, models.JobStatusFailed, errMsg)
		return job, err
	}

	logger.Infof("📊 获取到 %d 个目标播客", len(podcasts))

	if len(podcasts) == 0 {
		logger.Infof("⚠️  没有需要处理的播客")
		e.finalizeJob(job, []*models.JobExecution{})
		return job, nil
	}

	// 3. 并发执行同步
	results := e.executeSync(ctx, workflow, job, podcasts)

	// 4. 汇总结果并更新Job（含 finalizing → 报告 → 终态，持锁到报告持久化）
	e.finalizeJob(job, results)

	logger.Infof("✅ 工作流执行完成 [JobID=%d, 处理=%d, 成功=%d, 失败=%d]",
		job.ID, job.PodcastsProcessed,
		job.PodcastsProcessed-job.ErrorCount, job.ErrorCount)

	return job, nil
}

// getTargetPodcasts 根据配置获取目标播客列表
func (e *Executor) getTargetPodcasts(workflow *models.Workflow) ([]models.Podcast, error) {
	var podcasts []models.Podcast

	switch workflow.ScopeType {
	case models.ScopeTypeSpecificPodcasts:
		// 指定播客
		podcastIDs := workflow.ScopeConfig.PodcastIDs
		if len(podcastIDs) == 0 {
			return nil, fmt.Errorf("未指定任何播客ID")
		}

		// 转换为uint类型
		uintIDs := make([]uint, len(podcastIDs))
		for i, id := range podcastIDs {
			uintIDs[i] = uint(id)
		}

		if err := e.db.Where("id IN ?", uintIDs).Find(&podcasts).Error; err != nil {
			return nil, fmt.Errorf("查询指定播客失败: %w", err)
		}
		logger.Infof("📝 范围类型: 指定播客 (%d个)", len(podcastIDs))

	case models.ScopeTypeAllSubscribed:
		// 所有订阅
		if err := e.db.Where("is_subscribed = ?", true).Find(&podcasts).Error; err != nil {
			return nil, fmt.Errorf("查询订阅播客失败: %w", err)
		}
		logger.Infof("📝 范围类型: 所有订阅 (%d个)", len(podcasts))

	case models.ScopeTypeCustomSources:
		// 自定义源 - 需要先解析URL
		customURLs := workflow.ScopeConfig.CustomURLs
		if len(customURLs) == 0 {
			return nil, fmt.Errorf("未提供任何自定义RSS源")
		}
		return e.fetchCustomPodcasts(customURLs)

	default:
		return nil, fmt.Errorf("不支持的范围类型: %s", workflow.ScopeType)
	}

	return podcasts, nil
}

// fetchCustomPodcasts 从自定义URL获取播客信息
func (e *Executor) fetchCustomPodcasts(urls []string) ([]models.Podcast, error) {
	logger.Infof("📝 范围类型: 自定义源 (%d个URL)", len(urls))

	var podcasts []models.Podcast

	for _, feedURL := range urls {
		logger.Infof("📡 处理自定义RSS源: %s", feedURL)

		// 使用 FirstOrCreate 避免竞态条件
		// 如果多个并发worker同时检测到同一个URL不存在，FirstOrCreate会自动处理唯一约束冲突
		// 生成唯一的XYZ ID，避免空字符串导致的唯一约束冲突
		newPodcast := models.Podcast{
			XYZID:        "custom-" + fmt.Sprintf("%d", time.Now().UnixNano()) + "-" + feedURL,
			FeedURL:      feedURL,
			Title:        "自定义源-" + feedURL[strings.LastIndex(feedURL, "/")+1:],
			IsSubscribed: false, // 自定义源默认不订阅
		}

		// FirstOrCreate 会在冲突时返回已存在的记录
		// 注意：由于XYZID也是唯一的，冲突时我们只基于feed_url判断
		if err := e.db.Where("feed_url = ?", feedURL).
			FirstOrCreate(&newPodcast).Error; err != nil {
			logger.Infof("❌ 创建或查找播客记录失败 [%s]: %v", feedURL, err)
			continue
		}

		logger.Infof("✅ 成功获取播客记录: %s (ID=%d)", newPodcast.Title, newPodcast.ID)
		podcasts = append(podcasts, newPodcast)
	}

	if len(podcasts) == 0 {
		return nil, fmt.Errorf("未能从自定义源获取任何播客")
	}

	logger.Infof("📊 自定义源处理完成，获取到 %d 个播客", len(podcasts))
	return podcasts, nil
}

// batchFeedState tracks one podcast through the first-pass + classified retry
// window of a single workflow job.
type batchFeedState struct {
	podcast             models.Podcast
	execution           *models.JobExecution
	attempt             int
	accessDeniedRetries int
	transientRetries    int
	// lastRetryDecision is the DecideBatchRetry.Reason applied to the most
	// recent failed attempt (empty when success or not yet decided).
	lastRetryDecision string
	done              bool
}

// executeSync runs a bounded 15-minute batch (#35/#36):
//  1. First-pass primary attempt for every target Feed (fairness before retries)
//  2. Classified retries (403 ≈ min 3/8/13; network/5xx bounded; 429/503 Retry-After)
//  3. Stop networking at the batch deadline and return final per-feed outcomes
func (e *Executor) executeSync(
	ctx context.Context,
	workflow *models.Workflow,
	job *models.Job,
	podcasts []models.Podcast,
) []*models.JobExecution {
	if ctx == nil {
		ctx = context.Background()
	}
	batchStart := e.clockNow()
	deadline := batchStart.Add(e.batchWindow())
	// Bind a real context deadline only when using the wall clock. Tests inject
	// e.now / e.sleep and drive the cutoff via clockNow() comparisons instead.
	batchCtx := ctx
	cancel := func() {}
	if e.now == nil {
		batchCtx, cancel = context.WithDeadline(ctx, deadline)
	}
	defer cancel()

	states := make([]*batchFeedState, 0, len(podcasts))
	for _, podcast := range podcasts {
		states = append(states, &batchFeedState{podcast: podcast})
	}

	logger.Infof("🔄 15 分钟批次开始 [JobID=%d, Feeds=%d, Window=%s]",
		job.ID, len(states), e.batchWindow())

	// --- First pass: every target Feed gets one primary attempt before any retry ---
	e.runBatchPass(batchCtx, workflow, job, states, true /* firstPass */)

	// --- Classified retry rounds until deadline or no remaining work ---
	for {
		now := e.clockNow()
		if !now.Before(deadline) || batchCtx.Err() != nil {
			logger.Infof("⏱️  批次截止，停止网络重试 [JobID=%d]", job.ID)
			e.finalizeOpenRetryDecisions(job.ID, states, "batch_deadline")
			break
		}

		due, minWait := e.collectDueRetries(job.ID, states, batchStart, deadline, now)
		if len(due) == 0 {
			if minWait < 0 {
				break // nothing left to retry
			}
			remaining := deadline.Sub(e.clockNow())
			if remaining <= 0 {
				e.finalizeOpenRetryDecisions(job.ID, states, "batch_deadline")
				break
			}
			if minWait > remaining {
				logger.Infof("⏱️  下一次重试已超过批次截止 [JobID=%d]", job.ID)
				e.finalizeOpenRetryDecisions(job.ID, states, "access_denied_past_deadline")
				break
			}
			e.clockSleep(minWait)
			continue
		}

		e.runBatchPass(batchCtx, workflow, job, due, false /* firstPass */)
	}

	// Mark any still-open failed executions as final; leave successes as-is.
	results := make([]*models.JobExecution, 0, len(states))
	for _, state := range states {
		if state.execution == nil {
			// First-pass never ran (deadline before admission).
			state.execution = e.recordSkippedBatchDeadline(job.ID, state.podcast)
		}
		results = append(results, state.execution)
	}
	return results
}

func (e *Executor) decideRetry(state *batchFeedState, batchStart, deadline, now time.Time) feed.BatchRetryDecision {
	category := feed.ErrorCategory(state.execution.FeedErrorCategory)
	if category == "" || category == "not_observed" {
		category = feed.ErrorCategoryUnknown
	}
	remaining := deadline.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	return feed.DecideBatchRetry(feed.BatchRetryInput{
		Category:            category,
		Attempt:             state.attempt,
		AccessDeniedRetries: state.accessDeniedRetries,
		TransientRetries:    state.transientRetries,
		BatchElapsed:        now.Sub(batchStart),
		BatchRemaining:      remaining,
		RetryAfter:          state.execution.FeedRetryAfter,
		Now:                 now,
	})
}

// collectDueRetries stamps DecideBatchRetry.Reason onto the latest attempt for
// each failed feed and returns those due for an immediate network retry plus
// the minimum wait among scheduled retries (minWait < 0 means none waiting).
func (e *Executor) collectDueRetries(
	jobID uint,
	states []*batchFeedState,
	batchStart, deadline, now time.Time,
) (due []*batchFeedState, minWait time.Duration) {
	minWait = -1
	for _, state := range states {
		if state.done || state.execution == nil {
			continue
		}
		if state.execution.Status != models.ExecutionStatusFailed {
			if state.execution.Status == models.ExecutionStatusSuccess {
				e.patchLastAttemptRetryDecision(jobID, state.podcast.ID, "not_needed")
				state.lastRetryDecision = "not_needed"
			}
			state.done = true
			continue
		}
		decision := e.decideRetry(state, batchStart, deadline, now)
		e.patchLastAttemptRetryDecision(jobID, state.podcast.ID, decision.Reason)
		state.lastRetryDecision = decision.Reason
		if !decision.Retry {
			state.done = true
			continue
		}
		if decision.Wait <= 0 {
			due = append(due, state)
			continue
		}
		if minWait < 0 || decision.Wait < minWait {
			minWait = decision.Wait
		}
	}
	return due, minWait
}

func (e *Executor) finalizeOpenRetryDecisions(jobID uint, states []*batchFeedState, reason string) {
	for _, state := range states {
		if state.done || state.execution == nil {
			continue
		}
		if state.execution.Status != models.ExecutionStatusFailed {
			continue
		}
		if state.lastRetryDecision == "" || isRetryableDecision(state.lastRetryDecision) {
			e.patchLastAttemptRetryDecision(jobID, state.podcast.ID, reason)
			state.lastRetryDecision = reason
		}
		state.done = true
	}
}

func isRetryableDecision(reason string) bool {
	switch reason {
	case "access_denied_scheduled", "access_denied_slot", "retry_after_or_backoff", "transient_backoff":
		return true
	default:
		return false
	}
}

// patchLastAttemptRetryDecision writes the batch policy Reason onto the latest
// attempt row for this job+podcast (the attempt that just finished).
func (e *Executor) patchLastAttemptRetryDecision(jobID, podcastID uint, reason string) {
	if e == nil || e.db == nil || reason == "" {
		return
	}
	if !e.db.Migrator().HasTable(&models.JobFeedAttempt{}) {
		return
	}
	var last models.JobFeedAttempt
	err := e.db.Where("job_id = ? AND podcast_id = ?", jobID, podcastID).
		Order("attempt_no DESC, id DESC").
		First(&last).Error
	if err != nil {
		return
	}
	_ = e.db.Model(&last).Update("retry_decision", reason).Error
}

// runBatchPass executes the given states concurrently (bounded workers).
// firstPass=true means every state is attempted once; otherwise only the
// provided due set is retried and attempt counters advance by classification.
func (e *Executor) runBatchPass(
	ctx context.Context,
	workflow *models.Workflow,
	job *models.Job,
	states []*batchFeedState,
	firstPass bool,
) {
	if len(states) == 0 {
		return
	}
	// Check deadline before starting network work.
	select {
	case <-ctx.Done():
		return
	default:
	}

	concurrency := 5
	if e != nil && e.workerConcurrency > 0 {
		concurrency = e.workerConcurrency
	}
	if len(states) < concurrency {
		concurrency = len(states)
	}
	if concurrency < 1 {
		concurrency = 1
	}

	type workItem struct {
		state *batchFeedState
	}
	taskChan := make(chan workItem, len(states))
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range taskChan {
				select {
				case <-ctx.Done():
					return
				default:
				}
				state := item.state
				// On retry, count the classified retry before the attempt so
				// DecideBatchRetry budgets stay accurate even if the process dies.
				if !firstPass && state.execution != nil {
					cat := feed.ErrorCategory(state.execution.FeedErrorCategory)
					switch cat {
					case feed.ErrorCategoryAccessDenied:
						state.accessDeniedRetries++
					case feed.ErrorCategoryRateLimited, feed.ErrorCategoryServiceUnavailable,
						feed.ErrorCategoryTimeout, feed.ErrorCategoryNetwork, feed.ErrorCategoryHTTP:
						state.transientRetries++
					}
				}
				state.attempt++
				// Clear prior decision so the new attempt is stamped fresh after.
				state.lastRetryDecision = ""
				state.execution = e.syncPodcast(ctx, workflow, job.ID, state.podcast)
				if state.execution != nil && state.execution.Status == models.ExecutionStatusSuccess {
					e.recordFeedAttempt(job.ID, &state.podcast, state.execution, state.attempt, "not_needed")
					state.lastRetryDecision = "not_needed"
					state.done = true
				} else if state.execution != nil {
					// Retry decision is patched by collectDueRetries after this pass.
					e.recordFeedAttempt(job.ID, &state.podcast, state.execution, state.attempt, "")
				}
			}
		}()
	}
	for _, state := range states {
		if state.done {
			continue
		}
		taskChan <- workItem{state: state}
	}
	close(taskChan)
	wg.Wait()
}

func (e *Executor) recordSkippedBatchDeadline(jobID uint, podcast models.Podcast) *models.JobExecution {
	execution := &models.JobExecution{
		JobID:            jobID,
		PodcastID:        &podcast.ID,
		PodcastTitle:     podcast.Title,
		PodcastFeedURL:   podcast.FeedURL,
		Status:           models.ExecutionStatusFailed,
		ErrorMessage:     "batch deadline reached before first-pass attempt",
		FeedErrorCategory: string(feed.ErrorCategoryUnknown),
		FeedTargetDomain: feed.TargetDomain(podcast.FeedURL),
	}
	if err := e.db.Create(execution).Error; err != nil {
		logger.Infof("❌ 创建批次截止 JobExecution 失败 [PodcastID=%d]: %v", podcast.ID, err)
	}
	return execution
}

// syncPodcast 同步单个播客
func (e *Executor) syncPodcast(
	ctx context.Context,
	workflow *models.Workflow,
	jobID uint,
	podcast models.Podcast,
) *models.JobExecution {

	startTime := time.Now()

	// Prefer updating an existing execution row for this job+podcast (retry path)
	// so JobExecution remains the final-result projection; attempt history is #39.
	var execution models.JobExecution
	err := e.db.Where("job_id = ? AND podcast_id = ?", jobID, podcast.ID).
		Order("id DESC").
		First(&execution).Error
	if err != nil {
		execution = models.JobExecution{
			JobID:          jobID,
			PodcastID:      &podcast.ID,
			PodcastTitle:   podcast.Title,
			PodcastFeedURL: podcast.FeedURL,
			Status:         models.ExecutionStatusRunning,
		}
		if createErr := e.db.Create(&execution).Error; createErr != nil {
			logger.Infof("❌ 创建JobExecution失败 [PodcastID=%d]: %v", podcast.ID, createErr)
			execution.Status = models.ExecutionStatusFailed
			execution.ErrorMessage = fmt.Sprintf("创建记录失败: %v", createErr)
			execution.ProcessingTime = int(time.Since(startTime).Milliseconds())
			return &execution
		}
	} else {
		execution.Status = models.ExecutionStatusRunning
		execution.ErrorMessage = ""
		_ = e.db.Save(&execution).Error
	}

	logger.Infof("📡 [%s] 开始同步: %s", workflow.Name, podcast.Title)

	// 构建同步配置
	syncConfig := syncsvc.DefaultEpisodeSyncConfig

	// 应用规则配置 - 支持三种时间范围模式
	if workflow.RulesConfig.TimeRangeMode == "since_last_update" || workflow.RulesConfig.TimeRange == -1 {
		// 模式1：自上次更新后 - 使用podcast的last_fetched_at作为基准
		syncConfig.Mode = syncsvc.SyncModeIncremental
		logger.Infof("⏱️  时间范围: 自上次更新 (增量模式)")

	} else if workflow.RulesConfig.TimeRange > 0 {
		// 模式2：最近N天 - 指定天数范围
		syncConfig.Mode = syncsvc.SyncModeIncremental
		syncConfig.TimeRangeDays = &workflow.RulesConfig.TimeRange
		logger.Infof("⏱️  时间范围: %d天 (增量模式)", workflow.RulesConfig.TimeRange)

	} else {
		// 模式3：全部历史数据
		syncConfig.Mode = syncsvc.SyncModeFull
		logger.Infof("⏱️  时间范围: 全部历史数据")
	}

	// 执行同步
	result, err := e.syncSvc.SyncPodcastEpisodesWithContext(
		ctx,
		podcast.ID,
		&silentReporter{}, // 不输出详细日志
		syncConfig,
	)
	if result != nil {
		applyFeedAccessOutcome(&execution, result.FeedAccess)
	}

	processingTime := int(time.Since(startTime).Milliseconds())

	// 更新执行记录
	if err != nil {
		execution.Status = models.ExecutionStatusFailed
		execution.ErrorMessage = err.Error()
		logger.Infof("❌ [%s] 同步失败: %s - %v", workflow.Name, podcast.Title, err)
	} else {
		execution.Status = models.ExecutionStatusSuccess
		execution.EpisodesFound = result.Created + result.Updated
		execution.EpisodesCreated = result.Created
		execution.EpisodesMatched = result.Created + result.Updated // 先设置为同步结果

		// 查询时间窗口内实际匹配的episodes数量
		// 获取Job的时间窗口
		var job models.Job
		if err := e.db.First(&job, execution.JobID).Error; err == nil {
			// 使用统一的时间窗口计算函数
			days := workflow.RulesConfig.TimeRange
			if days <= 0 {
				days = 1 // 默认1天
			}

			// 确定触发时间
			var triggerTime time.Time
			if job.StartTime != nil {
				triggerTime = *job.StartTime
			} else {
				triggerTime = startTime // 如果 StartTime 为空，使用当前时间
			}

			// 确定模式并计算时间窗口
			var mode utils.TimeRangeMode
			if job.TriggeredBy == "cron" || job.TriggeredBy == "cron-catchup" {
				mode = utils.TimeRangeModeDaily
			} else {
				mode = utils.TimeRangeModeManual
			}

			timeRangeStart, timeRangeEnd, err := utils.GetTimeRangeWindow(mode, days, triggerTime)
			if err != nil {
				logger.Infof("⚠️  [Executor] 时间窗口计算失败: %v", err)
			} else {
				logger.Infof("🔍 [Executor] 时间窗口计算 [JobID=%d, PodcastID=%d]", job.ID, podcast.ID)
				logger.Infof("   - TriggeredBy: %s, Mode: %s", job.TriggeredBy, mode)
				logger.Infof("   - TimeRangeDays: %d", days)
				logger.Infof("   - TimeWindow: %s → %s", timeRangeStart.Format("2006-01-02 15:04:05"), timeRangeEnd.Format("2006-01-02 15:04:05"))

				// 查询该podcast在时间窗口内的episodes数量
				var matchedCount int64
				e.db.Model(&models.Episode{}).
					Where("podcast_id = ?", podcast.ID).
					Where("COALESCE(updated_date, published_date) >= ? AND COALESCE(updated_date, published_date) <= ?", timeRangeStart, timeRangeEnd).
					Count(&matchedCount)

				logger.Infof("   - EpisodesMatched: %d", matchedCount)
				execution.EpisodesMatched = int(matchedCount)
			}
		}

		logger.Infof("✅ [%s] 同步完成: %s - 新增:%d, 更新:%d, 匹配:%d (耗时:%dms)",
			workflow.Name, podcast.Title,
			result.Created, result.Updated, execution.EpisodesMatched, processingTime)
	}

	execution.ProcessingTime = processingTime
	e.db.Save(&execution)

	// Attempt history is recorded by runBatchPass so retry decisions from
	// DecideBatchRetry can be attached on the real path (#39).
	return &execution
}

// recordFeedAttempt persists one safe attempt. attemptNo<=0 means "next".
// FailurePhase is copied from the JobExecution final projection (set from
// AccessOutcome via applyFeedAccessOutcome). retryDecision is the batch-policy
// Reason for this attempt (may be empty until collectDueRetries patches it).
func (e *Executor) recordFeedAttempt(jobID uint, podcast *models.Podcast, execution *models.JobExecution, attemptNo int, retryDecision string) {
	if e == nil || e.db == nil || podcast == nil || execution == nil {
		return
	}
	if attemptNo <= 0 {
		var count int64
		_ = e.db.Model(&models.JobFeedAttempt{}).
			Where("job_id = ? AND podcast_id = ?", jobID, podcast.ID).
			Count(&count).Error
		attemptNo = int(count) + 1
	}
	attempt := &models.JobFeedAttempt{
		JobID:                jobID,
		PodcastID:            &podcast.ID,
		AttemptNo:            attemptNo,
		SourceType:           execution.FeedSourceType,
		AttemptedAt:          time.Now(),
		HTTPStatus:           execution.FeedHTTPStatus,
		ErrorCategory:        execution.FeedErrorCategory,
		FailurePhase:         execution.FeedFailurePhase,
		RetryDecision:        retryDecision,
		IdentityVerification: execution.FeedIdentityVerification,
		TargetDomain:         execution.FeedTargetDomain,
		SourceURL:            execution.FeedSourceURL,
		IsFinalResult:        true, // updated when later attempts overwrite final flags
	}
	// Demote previous final flags for this podcast.
	if e.db.Migrator().HasTable(&models.JobFeedAttempt{}) {
		_ = e.db.Model(&models.JobFeedAttempt{}).
			Where("job_id = ? AND podcast_id = ? AND is_final_result = ?", jobID, podcast.ID, true).
			Update("is_final_result", false).Error
	}
	_ = PersistFeedAttempt(e.db, attempt)
}

func applyFeedAccessOutcome(execution *models.JobExecution, outcome *feed.AccessOutcome) {
	if execution == nil || outcome == nil {
		return
	}
	execution.FeedHTTPStatus = outcome.HTTPStatus
	execution.FeedErrorCategory = string(outcome.ErrorCategory)
	execution.FeedTargetDomain = outcome.TargetDomain
	execution.FeedResponseTimeMs = outcome.ResponseTimeMs
	execution.FeedRetryAfter = outcome.RetryAfter
	execution.FeedETag = outcome.ETag
	execution.FeedLastModified = outcome.LastModified
	execution.FeedCacheControl = outcome.CacheControl
	execution.FeedExpires = outcome.Expires
	execution.FeedAge = outcome.Age
	execution.FeedResponseBytes = outcome.ResponseBytes
	execution.FeedSourceType = string(outcome.SourceType)
	execution.FeedSourceURL = outcome.SourceURL
	execution.FeedIdentityVerification = outcome.IdentityVerification
	execution.FeedCacheStatus = string(outcome.CacheStatus)
	execution.FeedFreshness = string(outcome.Freshness)
	execution.FeedEgressID = outcome.EgressID
	execution.FeedSnapshotRetrievedAt = outcome.RetrievedAt
	execution.FeedCircuitState = string(outcome.CircuitState)
	// Copy connection-stage diagnosis from the live fetch onto the final
	// projection and attempt history (#39).
	if outcome.FailurePhase != "" && outcome.FailurePhase != feed.FailurePhaseNotObserved {
		execution.FeedFailurePhase = string(outcome.FailurePhase)
	} else if outcome.FailurePhase == feed.FailurePhaseNotObserved {
		execution.FeedFailurePhase = string(feed.FailurePhaseNotObserved)
	}
}

// finalizeJob aggregates execution outcomes, holds finalizing until one report
// is persisted (or generation fails with an explicit terminal status), then
// releases the active-job slot by writing a terminal status (#38).
func (e *Executor) finalizeJob(job *models.Job, executions []*models.JobExecution) {
	var successCount, failedCount, skippedCount int
	var totalEpisodes int
	var totalMatched int

	for _, exec := range executions {
		switch exec.Status {
		case models.ExecutionStatusSuccess:
			successCount++
			totalEpisodes += exec.EpisodesCreated
			totalMatched += exec.EpisodesMatched
		case models.ExecutionStatusFailed:
			failedCount++
		case models.ExecutionStatusSkipped:
			skippedCount++
		}
	}

	terminal := finalJobStatus(successCount, failedCount, len(executions))

	job.PodcastsProcessed = len(executions)
	job.EpisodesFound = totalEpisodes
	job.EpisodesCreated = totalEpisodes
	job.EpisodesMatched = totalMatched
	job.ErrorCount = failedCount
	if err := e.db.Save(job).Error; err != nil {
		logger.Infof("❌ 更新Job中间统计失败 [JobID=%d]: %v", job.ID, err)
	}

	// Hold the single-active-job slot through report persistence.
	if err := MarkJobFinalizing(e.db, job); err != nil {
		logger.Infof("❌ 标记 finalizing 失败 [JobID=%d]: %v", job.ID, err)
	}

	logger.Infof("📊 Job 进入 finalizing，生成报告 [ID=%d] - 成功:%d, 失败:%d, 跳过:%d",
		job.ID, successCount, failedCount, skippedCount)

	e.db.Model(&models.Workflow{}).
		Where("id = ?", job.WorkflowID).
		Update("last_job_id", job.ID)

	reportGen := NewReportGenerator(e.db, e.summarizer)
	// Idempotent: if a report already exists (retry after crash during terminal
	// update), do not create a second one.
	if exists, err := HasReportForJob(e.db, job.ID); err == nil && exists {
		logger.Infof("ℹ️  报告已存在，跳过重复生成 [JobID=%d]", job.ID)
	} else {
		report, err := reportGen.GenerateForJob(job)
		if err != nil {
			logger.Infof("❌ 生成报告失败 [JobID=%d]: %v", job.ID, err)
			// Even without a report row, release the lock with the terminal status.
		} else {
			logger.Infof("✅ 报告已生成 [JobID=%d, ReportID=%d, Size=%d bytes]",
				job.ID, report.ID, report.FileSize)
			// completed/partial both get exactly one report; LLM failures keep
			// the base report with LLMError warning (handled inside GenerateForJob).
			if e.notifier != nil && terminal == models.JobStatusCompleted && job.TriggeredBy == "cron" {
				if err := e.notifier.SendReport(report.Title, report.Content); err != nil {
					logger.Infof("❌ 发送邮件通知失败 [JobID=%d]: %v", job.ID, err)
				} else {
					logger.Infof("✅ 邮件通知已发送 [JobID=%d, TriggeredBy=%s]", job.ID, job.TriggeredBy)
				}
			}
		}
	}

	endTime := time.Now()
	job.EndTime = &endTime
	job.Status = terminal
	if err := e.db.Save(job).Error; err != nil {
		logger.Infof("❌ 更新Job终态失败 [JobID=%d]: %v", job.ID, err)
	} else if job.StartTime != nil {
		logger.Infof("✅ Job已完成 [JobID=%d, Status=%s, 总耗时=%dms]",
			job.ID, job.Status, endTime.Sub(*job.StartTime).Milliseconds())
	}
}

func finalJobStatus(successCount, failedCount, executionCount int) models.JobStatus {
	_ = executionCount
	switch feed.BatchTerminalStatus(successCount, failedCount) {
	case "partial":
		return models.JobStatusPartial
	case "failed":
		return models.JobStatusFailed
	default:
		return models.JobStatusCompleted
	}
}

// updateJobStatus 更新Job状态和错误信息
func (e *Executor) updateJobStatus(job *models.Job, status models.JobStatus, errorMsg string) {
	job.Status = status
	job.ErrorCount = 1 // 简单处理，设置为1表示有错误

	endTime := time.Now()
	job.EndTime = &endTime

	if err := e.db.Save(job).Error; err != nil {
		logger.Infof("❌ 更新Job状态失败: %v", err)
	}
}

// silentReporter 静默进度报告器（用于工作流执行，避免日志过多）
type silentReporter struct{}

func (r *silentReporter) Report(msg string)                                {}
func (r *silentReporter) ReportSuccess(msg string)                         {}
func (r *silentReporter) ReportError(msg string)                           {}
func (r *silentReporter) ReportProgress(current, total int, msg string)    {}
func (r *silentReporter) ReportSkip(reason syncsvc.SkipReason, msg string) {}
func (r *silentReporter) ReportSummary(summary *syncsvc.SyncSummary)       {}
func (r *silentReporter) Close()                                           {}
