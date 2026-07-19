package workflow

import (
	"context"
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

// Execute 执行工作流
func (e *Executor) Execute(ctx context.Context, workflow *models.Workflow, triggeredBy string) (*models.Job, error) {
	logger.Infof("🚀 开始执行工作流 [ID=%d, Name=%s]", workflow.ID, workflow.Name)

	// 1. 创建 Job 记录
	startTime := time.Now()
	job := &models.Job{
		WorkflowID:  workflow.ID,
		Status:      models.JobStatusRunning,
		StartTime:   &startTime,
		TriggeredBy: triggeredBy,
	}

	if err := e.db.Create(job).Error; err != nil {
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

	// 4. 汇总结果并更新Job
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

// executeSync 并发执行播客同步
func (e *Executor) executeSync(
	ctx context.Context,
	workflow *models.Workflow,
	job *models.Job,
	podcasts []models.Podcast,
) []*models.JobExecution {

	concurrency := 5 // 最多5个并发
	taskChan := make(chan models.Podcast, len(podcasts))
	resultChan := make(chan *models.JobExecution, len(podcasts))

	logger.Infof("🔄 启动 %d 个并发worker处理 %d 个播客", concurrency, len(podcasts))

	// 启动worker
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for podcast := range taskChan {
				// 检查context是否已取消
				select {
				case <-ctx.Done():
					logger.Infof("⚠️  Worker %d: Context已取消", workerID)
					return
				default:
				}

				result := e.syncPodcast(ctx, workflow, job.ID, podcast)
				resultChan <- result
			}
		}(i)
	}

	// 分发任务
	for _, podcast := range podcasts {
		taskChan <- podcast
	}
	close(taskChan)

	// 等待完成
	wg.Wait()
	close(resultChan)

	// 收集结果
	var results []*models.JobExecution
	for result := range resultChan {
		results = append(results, result)
	}

	return results
}

// syncPodcast 同步单个播客
func (e *Executor) syncPodcast(
	ctx context.Context,
	workflow *models.Workflow,
	jobID uint,
	podcast models.Podcast,
) *models.JobExecution {

	startTime := time.Now()

	execution := &models.JobExecution{
		JobID:          jobID,
		PodcastID:      &podcast.ID,
		PodcastTitle:   podcast.Title,
		PodcastFeedURL: podcast.FeedURL,
		Status:         models.ExecutionStatusRunning,
	}

	// 保存初始记录
	if err := e.db.Create(execution).Error; err != nil {
		logger.Infof("❌ 创建JobExecution失败 [PodcastID=%d]: %v", podcast.ID, err)
		// 即使创建失败，也返回execution对象（标记为失败），避免nil指针
		execution.Status = models.ExecutionStatusFailed
		execution.ErrorMessage = fmt.Sprintf("创建记录失败: %v", err)
		execution.ProcessingTime = int(time.Since(startTime).Milliseconds())
		return execution
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
		applyFeedAccessOutcome(execution, result.FeedAccess)
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
	e.db.Save(execution)

	return execution
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
}

// finalizeJob 汇总结果并更新Job状态
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

	// ⭐ 先保存中间统计，但不设置EndTime和状态
	job.PodcastsProcessed = len(executions)
	job.EpisodesFound = totalEpisodes
	job.EpisodesCreated = totalEpisodes
	job.EpisodesMatched = totalMatched
	job.ErrorCount = failedCount
	if err := e.db.Save(job).Error; err != nil {
		logger.Infof("❌ 更新Job中间统计失败 [JobID=%d]: %v", job.ID, err)
	}

	logger.Infof("📊 Job执行完成，正在生成报告 [ID=%d] - 成功:%d, 失败:%d, 跳过:%d",
		job.ID, successCount, failedCount, skippedCount)

	// 更新workflow的last_job_id
	e.db.Model(&models.Workflow{}).
		Where("id = ?", job.WorkflowID).
		Update("last_job_id", job.ID)

	// ⭐ 异步生成执行报告，报告生成成功后再设置EndTime和最终状态
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Infof("❌ 报告生成panic已恢复 [JobID=%d]: %v", job.ID, r)
				endTime := time.Now()
				job.EndTime = &endTime
				job.Status = finalJobStatus(successCount, failedCount, len(executions))
				if saveErr := e.db.Save(job).Error; saveErr != nil {
					logger.Infof("❌ 更新Job状态失败 [JobID=%d]: %v", job.ID, saveErr)
				}
			}
		}()

		reportGen := NewReportGenerator(e.db, e.summarizer)
		report, err := reportGen.GenerateForJob(job)
		if err != nil {
			logger.Infof("❌ 生成报告失败 [JobID=%d]: %v", job.ID, err)

			// 报告生成失败，设置EndTime并决定最终状态
			endTime := time.Now()
			job.EndTime = &endTime
			job.Status = finalJobStatus(successCount, failedCount, len(executions))
			if saveErr := e.db.Save(job).Error; saveErr != nil {
				logger.Infof("❌ 更新Job状态失败 [JobID=%d]: %v", job.ID, saveErr)
			}
			return
		}

		logger.Infof("✅ 报告已生成 [JobID=%d, ReportID=%d, Size=%d bytes]",
			job.ID, report.ID, report.FileSize)

		// ⭐ 报告生成成功，设置EndTime（包括报告生成时间）并更新状态
		endTime := time.Now()
		job.EndTime = &endTime
		job.Status = finalJobStatus(successCount, failedCount, len(executions))

		if err := e.db.Save(job).Error; err != nil {
			logger.Infof("❌ 更新Job状态失败 [JobID=%d]: %v", job.ID, err)
		} else {
			duration := endTime.Sub(*job.StartTime).Milliseconds()
			logger.Infof("✅ Job已完成 [JobID=%d, 总耗时=%dms]", job.ID, duration)
		}

		// 发送邮件通知（仅cron触发的成功任务）
		if e.notifier != nil && job.Status == models.JobStatusCompleted && job.TriggeredBy == "cron" {
			if err := e.notifier.SendReport(report.Title, report.Content); err != nil {
				logger.Infof("❌ 发送邮件通知失败 [JobID=%d]: %v", job.ID, err)
			} else {
				logger.Infof("✅ 邮件通知已发送 [JobID=%d, TriggeredBy=%s]", job.ID, job.TriggeredBy)
			}
		}
	}()
}

func finalJobStatus(successCount, failedCount, executionCount int) models.JobStatus {
	if failedCount > 0 || (executionCount > 0 && successCount == 0) {
		return models.JobStatusFailed
	}
	return models.JobStatusCompleted
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
