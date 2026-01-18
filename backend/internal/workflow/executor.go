package workflow

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/models"
	syncsvc "magicpodcast/internal/sync"

	"gorm.io/gorm"
)

// Executor 工作流执行器
type Executor struct {
	db      *gorm.DB
	syncSvc *syncsvc.Service
}

// NewExecutor 创建执行器
func NewExecutor(db *gorm.DB, syncSvc *syncsvc.Service) *Executor {
	return &Executor{
		db:      db,
		syncSvc: syncSvc,
	}
}

// Execute 执行工作流
func (e *Executor) Execute(ctx context.Context, workflow *models.Workflow, triggeredBy string) (*models.Job, error) {
	log.Printf("🚀 开始执行工作流 [ID=%d, Name=%s]", workflow.ID, workflow.Name)

	// 1. 创建 Job 记录
	startTime := time.Now()
	job := &models.Job{
		WorkflowID: workflow.ID,
		Status:     models.JobStatusRunning,
		StartTime:  &startTime,
		TriggeredBy: triggeredBy,
	}

	if err := e.db.Create(job).Error; err != nil {
		return nil, fmt.Errorf("创建Job失败: %w", err)
	}

	log.Printf("✅ Job记录已创建 [ID=%d]", job.ID)

	// 2. 解析配置并获取目标播客
	podcasts, err := e.getTargetPodcasts(workflow)
	if err != nil {
		errMsg := fmt.Sprintf("获取目标播客失败: %v", err)
		log.Printf("❌ %s", errMsg)
		e.updateJobStatus(job, models.JobStatusFailed, errMsg)
		return job, err
	}

	log.Printf("📊 获取到 %d 个目标播客", len(podcasts))

	if len(podcasts) == 0 {
		log.Printf("⚠️  没有需要处理的播客")
		e.finalizeJob(job, []*models.JobExecution{})
		return job, nil
	}

	// 3. 并发执行同步
	results := e.executeSync(ctx, workflow, job, podcasts)

	// 4. 汇总结果并更新Job
	e.finalizeJob(job, results)

	log.Printf("✅ 工作流执行完成 [JobID=%d, 处理=%d, 成功=%d, 失败=%d]",
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
		log.Printf("📝 范围类型: 指定播客 (%d个)", len(podcastIDs))

	case models.ScopeTypeAllSubscribed:
		// 所有订阅
		if err := e.db.Where("is_subscribed = ?", true).Find(&podcasts).Error; err != nil {
			return nil, fmt.Errorf("查询订阅播客失败: %w", err)
		}
		log.Printf("📝 范围类型: 所有订阅 (%d个)", len(podcasts))

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
	log.Printf("📝 范围类型: 自定义源 (%d个URL)", len(urls))

	var podcasts []models.Podcast

	for _, feedURL := range urls {
		log.Printf("📡 处理自定义RSS源: %s", feedURL)

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
			log.Printf("❌ 创建或查找播客记录失败 [%s]: %v", feedURL, err)
			continue
		}

		log.Printf("✅ 成功获取播客记录: %s (ID=%d)", newPodcast.Title, newPodcast.ID)
		podcasts = append(podcasts, newPodcast)
	}

	if len(podcasts) == 0 {
		return nil, fmt.Errorf("未能从自定义源获取任何播客")
	}

	log.Printf("📊 自定义源处理完成，获取到 %d 个播客", len(podcasts))
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

	log.Printf("🔄 启动 %d 个并发worker处理 %d 个播客", concurrency, len(podcasts))

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
					log.Printf("⚠️  Worker %d: Context已取消", workerID)
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
		log.Printf("❌ 创建JobExecution失败 [PodcastID=%d]: %v", podcast.ID, err)
		// 即使创建失败，也返回execution对象（标记为失败），避免nil指针
		execution.Status = models.ExecutionStatusFailed
		execution.ErrorMessage = fmt.Sprintf("创建记录失败: %v", err)
		execution.ProcessingTime = int(time.Since(startTime).Milliseconds())
		return execution
	}

	log.Printf("📡 [%s] 开始同步: %s", workflow.Name, podcast.Title)

	// 构建同步配置
	syncConfig := syncsvc.DefaultEpisodeSyncConfig

	// 应用规则配置 - 支持三种时间范围模式
	if workflow.RulesConfig.TimeRangeMode == "since_last_update" || workflow.RulesConfig.TimeRange == -1 {
		// 模式1：自上次更新后 - 使用podcast的last_fetched_at作为基准
		syncConfig.Mode = syncsvc.SyncModeIncremental
		log.Printf("⏱️  时间范围: 自上次更新 (增量模式)")

	} else if workflow.RulesConfig.TimeRange > 0 {
		// 模式2：最近N天 - 指定天数范围
		syncConfig.Mode = syncsvc.SyncModeIncremental
		syncConfig.TimeRangeDays = &workflow.RulesConfig.TimeRange
		log.Printf("⏱️  时间范围: %d天 (增量模式)", workflow.RulesConfig.TimeRange)

	} else {
		// 模式3：全部历史数据
		syncConfig.Mode = syncsvc.SyncModeFull
		log.Printf("⏱️  时间范围: 全部历史数据")
	}

	// 执行同步
	result, err := e.syncSvc.SyncPodcastEpisodes(
		podcast.ID,
		&silentReporter{}, // 不输出详细日志
		syncConfig,
	)

	processingTime := int(time.Since(startTime).Milliseconds())

	// 更新执行记录
	if err != nil {
		execution.Status = models.ExecutionStatusFailed
		execution.ErrorMessage = err.Error()
		log.Printf("❌ [%s] 同步失败: %s - %v", workflow.Name, podcast.Title, err)
	} else {
		execution.Status = models.ExecutionStatusSuccess
		execution.EpisodesFound = result.Created + result.Updated
		execution.EpisodesCreated = result.Created
		log.Printf("✅ [%s] 同步完成: %s - 新增:%d, 更新:%d (耗时:%dms)",
			workflow.Name, podcast.Title,
			result.Created, result.Updated, processingTime)
	}

	execution.ProcessingTime = processingTime
	e.db.Save(execution)

	return execution
}

// finalizeJob 汇总结果并更新Job状态
func (e *Executor) finalizeJob(job *models.Job, executions []*models.JobExecution) {
	endTime := time.Now()
	job.EndTime = &endTime

	var successCount, failedCount, skippedCount int
	var totalEpisodes int

	for _, exec := range executions {
		switch exec.Status {
		case models.ExecutionStatusSuccess:
			successCount++
			totalEpisodes += exec.EpisodesCreated
		case models.ExecutionStatusFailed:
			failedCount++
		case models.ExecutionStatusSkipped:
			skippedCount++
		}
	}

	job.PodcastsProcessed = len(executions)
	job.EpisodesFound = totalEpisodes
	job.EpisodesCreated = totalEpisodes
	job.ErrorCount = failedCount

	duration := endTime.Sub(*job.StartTime).Milliseconds()
	jobDuration := int64(duration)

	// 确定最终状态
	if failedCount == 0 && successCount == 0 && skippedCount == 0 {
		job.Status = models.JobStatusCompleted
	} else if failedCount == 0 {
		job.Status = models.JobStatusCompleted
	} else if successCount == 0 {
		job.Status = models.JobStatusFailed
	} else {
		job.Status = models.JobStatusCompleted // 部分成功也算完成
	}

	if err := e.db.Save(job).Error; err != nil {
		log.Printf("❌ 更新Job状态失败 [JobID=%d]: %v", job.ID, err)
	}

	// 更新workflow的last_job_id
	e.db.Model(&models.Workflow{}).
		Where("id = ?", job.WorkflowID).
		Update("last_job_id", job.ID)

	log.Printf("📊 Job完成统计 [ID=%d] - 成功:%d, 失败:%d, 跳过:%d, 耗时:%dms",
		job.ID, successCount, failedCount, skippedCount, jobDuration)

	// ⭐ 异步生成执行报告（避免阻塞Job完成）
	go func() {
		reportGen := NewReportGenerator(e.db)
		report, err := reportGen.GenerateForJob(job)
		if err != nil {
			log.Printf("❌ 生成报告失败 [JobID=%d]: %v", job.ID, err)
		} else {
			log.Printf("✅ 报告已生成 [JobID=%d, ReportID=%d, Size=%d bytes]",
				job.ID, report.ID, report.FileSize)
		}
	}()
}

// updateJobStatus 更新Job状态和错误信息
func (e *Executor) updateJobStatus(job *models.Job, status models.JobStatus, errorMsg string) {
	job.Status = status
	job.ErrorCount = 1 // 简单处理，设置为1表示有错误

	endTime := time.Now()
	job.EndTime = &endTime

	if err := e.db.Save(job).Error; err != nil {
		log.Printf("❌ 更新Job状态失败: %v", err)
	}
}

// silentReporter 静默进度报告器（用于工作流执行，避免日志过多）
type silentReporter struct{}

func (r *silentReporter) Report(msg string)                                         {}
func (r *silentReporter) ReportSuccess(msg string)                                 {}
func (r *silentReporter) ReportError(msg string)                                   {}
func (r *silentReporter) ReportProgress(current, total int, msg string)           {}
func (r *silentReporter) ReportSkip(reason syncsvc.SkipReason, msg string)       {}
func (r *silentReporter) ReportSummary(summary *syncsvc.SyncSummary)             {}
func (r *silentReporter) Close()                                                   {}
