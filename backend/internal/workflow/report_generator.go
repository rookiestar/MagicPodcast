package workflow

import (
	"fmt"
	"magicpodcast/internal/logger"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"magicpodcast/internal/llm"
	"magicpodcast/internal/models"
	"magicpodcast/internal/utils"

	"gorm.io/gorm"
)

// ReportGenerator 报告生成器
type ReportGenerator struct {
	db         *gorm.DB
	summarizer SummarizerInterface
}

// SummarizerInterface 摘要生成器接口（避免循环依赖）
type SummarizerInterface interface {
	GenerateForReport(data []llm.EpisodeReportData, workflowName string, userPrompt string, options llm.SummaryOptions) (*llm.SummaryResult, error)
}

// NewReportGenerator 创建报告生成器
func NewReportGenerator(db *gorm.DB, summarizer SummarizerInterface) *ReportGenerator {
	return &ReportGenerator{
		db:         db,
		summarizer: summarizer,
	}
}

// EpisodeReportData 报告数据结构
type EpisodeReportData struct {
	PodcastID       uint
	PodcastTitle    string
	PodcastFeedURL  string
	PodcastCoverURL string
	Episodes        []EpisodeDetail
}

// EpisodeDetail 单集详情
type EpisodeDetail struct {
	EpisodeID     uint // real library episode id; required for homepage interactivity (#90)
	Title         string
	ShowNotes     string
	PublishedDate time.Time
	UpdatedDate   *time.Time
	EpisodeNo     string
	Duration      int
	ImageURL      string
	Link          string
	XYZID         string // 小宇宙ID
	QRCode        string // 小宇宙二维码（base64编码）
	QRCodeError   bool   // 二维码生成失败标记
}

// FeedCoverageSummary makes network coverage explicit in the report. A
// successful parse with zero matching episodes is different from a Feed that
// never reached its first attempt.
type FeedCoverageSummary struct {
	Total       int
	Attempted   int
	Successes   int
	Failures    int
	Unattempted int
}

// ConvertToLLMReportData 转换为 LLM 包所需的数据格式。
func ConvertToLLMReportData(data []EpisodeReportData) []llm.EpisodeReportData {
	result := make([]llm.EpisodeReportData, len(data))

	for i, podcast := range data {
		episodes := make([]llm.EpisodeDetail, len(podcast.Episodes))
		for j, ep := range podcast.Episodes {
			episodes[j] = llm.EpisodeDetail{
				Title:         ep.Title,
				ShowNotes:     ep.ShowNotes,
				PublishedDate: ep.PublishedDate,
				UpdatedDate:   ep.UpdatedDate,
				EpisodeNo:     ep.EpisodeNo,
				Link:          ep.Link,
				XYZID:         ep.XYZID,
				QRCode:        ep.QRCode,
				QRCodeError:   ep.QRCodeError,
			}
		}

		result[i] = llm.EpisodeReportData{
			PodcastID:      podcast.PodcastID,
			PodcastTitle:   podcast.PodcastTitle,
			PodcastFeedURL: podcast.PodcastFeedURL,
			Episodes:       episodes,
		}
	}

	return result
}

// GenerateForJob 为Job生成报告
func (rg *ReportGenerator) GenerateForJob(job *models.Job) (*models.Report, error) {
	// 重新从数据库查询Job，确保操作的是最新记录
	var freshJob models.Job
	if err := rg.db.First(&freshJob, job.ID).Error; err != nil {
		return nil, fmt.Errorf("查询Job失败: %w", err)
	}
	job = &freshJob

	// 1. 获取工作流配置以确定时间范围
	var workflow models.Workflow
	if err := rg.db.First(&workflow, job.WorkflowID).Error; err != nil {
		return nil, fmt.Errorf("获取工作流配置失败: %w", err)
	}

	// 2. 计算扫描时间窗口
	var timeRangeMode utils.TimeRangeMode
	var timeRangeDays int
	var triggeredAt time.Time

	// 判断触发模式：根据job的triggered_by字段判断
	if job.TriggeredBy == "cron" {
		timeRangeMode = utils.TimeRangeModeDaily
		timeRangeDays = workflow.RulesConfig.TimeRange // 使用配置的时间范围
		// Cron模式：使用Job.StartTime作为实际触发时间
		if job.StartTime != nil {
			triggeredAt = *job.StartTime
		} else {
			triggeredAt = job.CreatedAt // Fallback to CreatedAt
			logger.Infof("⚠️  [JobID=%d] StartTime为nil，使用CreatedAt作为触发时间", job.ID)
		}
	} else {
		timeRangeMode = utils.TimeRangeModeManual
		timeRangeDays = workflow.RulesConfig.TimeRange
		triggeredAt = job.CreatedAt // Manual模式：使用CreatedAt
	}

	// 调用时间窗口计算工具
	timeRangeStart, timeRangeEnd, err := utils.GetTimeRangeWindow(
		timeRangeMode,
		timeRangeDays,
		triggeredAt,
	)
	if err != nil {
		return nil, fmt.Errorf("计算时间范围失败: %w", err)
	}

	// 3. 收集匹配的episodes（按时间窗口扫描）
	reportData, err := rg.collectMatchedEpisodes(job, timeRangeStart, timeRangeEnd)
	if err != nil {
		return nil, fmt.Errorf("收集报告数据失败: %w", err)
	}

	coverage := rg.feedCoverage(job)

	// 4. 生成Markdown内容（暂时传入0和空字符串，后续生成LLM摘要后更新）
	markdown := rg.generateMarkdown(job, reportData, timeRangeStart, timeRangeEnd, string(timeRangeMode), workflow.Name, 0, "", coverage)

	// 5. 生成LLM摘要（如果启用）
	var llmSummary string
	var llmError string
	var llmModelUsed string
	var llmTokensUsed int

	logger.Debugf("[ReportGenerator] LLMEnabled=%v, SummarizerNil=%v, MatchedEpisodes=%d",
		workflow.RulesConfig.LLMEnabled, rg.summarizer == nil, len(reportData))

	// 优化：如果没有匹配的单集，跳过LLM摘要生成，不设置任何LLM字段
	if len(reportData) == 0 {
		logger.Infof("⏭️  [JobID=%d] 没有匹配的单集，跳过LLM摘要生成（不显示AI相关内容）", job.ID)
		// 不设置任何LLM字段，让前端完全不显示AI相关信息
	} else if workflow.RulesConfig.LLMEnabled && rg.summarizer != nil {
		logger.Infof("🤖 开始生成LLM摘要 [JobID=%d]", job.ID)
		logger.Debugf("  - Summarizer type: %T", rg.summarizer)

		// 准备选项
		options := llm.SummaryOptions{
			Model:       workflow.RulesConfig.LLMModel,
			Temperature: workflow.RulesConfig.LLMTemperature,
			MaxTokens:   workflow.RulesConfig.LLMMaxTokens,
			MaxEpisodes: workflow.RulesConfig.LLMMaxEpisodes,
		}
		if options.Temperature == 0 {
			options.Temperature = 0.7 // 默认值
		}
		if options.MaxTokens == 0 {
			options.MaxTokens = 1000 // 默认值
		}
		if options.MaxEpisodes == 0 {
			options.MaxEpisodes = 20 // 默认值
		}

		// 转换数据格式
		llmReportData := ConvertToLLMReportData(reportData)
		logger.Debugf("  - Converted %d podcasts to LLM report format", len(llmReportData))

		// 调用摘要生成器（只传入user prompt，system prompt从config获取）
		logger.Debugf("  - Calling summarizer.GenerateForReport...")
		result, err := rg.summarizer.GenerateForReport(
			llmReportData,
			workflow.Name,
			workflow.RulesConfig.LLMUserPrompt, // 只传入user prompt
			options,
		)
		if err != nil {
			logger.Warnf("LLM摘要生成失败 [JobID=%d]: %v", job.ID, err)
			llmError = err.Error()
			// 不中断流程，继续生成基础报告
		} else {
			llmSummary = result.Summary
			llmModelUsed = result.ModelUsed
			llmTokensUsed = result.TokensUsed
			logger.Infof("✅ LLM摘要生成成功 [JobID=%d, Tokens=%d]", job.ID, llmTokensUsed)

			// 重新生成包含token统计的Markdown（在LLM摘要生成前先生成完整内容）
			markdown = rg.generateMarkdown(job, reportData, timeRangeStart, timeRangeEnd, string(timeRangeMode), workflow.Name, llmTokensUsed, llmModelUsed, coverage)
			// 将LLM摘要插入到Markdown开头
			markdown = rg.insertLLMSummary(markdown, llmSummary)
		}
	}

	// 6. 使用事务创建Report记录并更新Job（确保原子性）
	matchedCount := rg.countEpisodes(reportData)
	structured := rg.buildStructuredEpisodes(reportData)
	publishToHomepage := false
	reportType := ""
	// Homepage publish requires explicit workflow config + at least one real episode id.
	// Zero-episode successes remain successful jobs but never surface on the homepage (#90).
	if workflow.PublishToHomepage &&
		models.IsValidHomepageReportType(workflow.ReportType) &&
		len(structured) > 0 {
		publishToHomepage = true
		reportType = workflow.ReportType
	}
	report := &models.Report{
		JobID:          job.ID,
		Title:          fmt.Sprintf("%s - %s", workflow.Name, job.CreatedAt.Format("2006-01-02 15:04:05")),
		Content:        markdown,
		Summary:        rg.generateSummary(job, reportData),
		EpisodesCount:  matchedCount, // 历史字段，保留兼容
		PodcastsCount:  len(reportData),
		MatchedCount:   matchedCount, // 新增：实际匹配的单集数
		TimeRangeStart: timeRangeStart,
		TimeRangeEnd:   timeRangeEnd,
		TimeRangeMode:  string(timeRangeMode),
		GeneratedAt:    time.Now(),
		Format:         "markdown",
		FileSize:       len(markdown),
		// Homepage snapshot + structured episodes share this generation run (#90).
		PublishToHomepage:  publishToHomepage,
		ReportType:         reportType,
		WorkflowName:       workflow.Name,
		StructuredEpisodes: structured,
		// LLM相关字段
		LLMSummary:    llmSummary,
		LLMModelUsed:  llmModelUsed,
		LLMTokensUsed: llmTokensUsed,
		LLMError:      llmError,
	}

	// 使用事务确保Report创建和Job更新的原子性。Report.JobID is unique: if a
	// prior attempt already persisted a report, reuse it instead of failing
	// (idempotent finalize after process restart mid-report).
	err = rg.db.Transaction(func(tx *gorm.DB) error {
		var existing models.Report
		if err := tx.Where("job_id = ?", job.ID).First(&existing).Error; err == nil {
			*report = existing
			return nil
		}
		// 创建Report记录
		if err := tx.Create(report).Error; err != nil {
			return fmt.Errorf("保存报告失败: %w", err)
		}

		// 更新Job的episodes_matched为实际匹配的单集数
		if err := tx.Model(job).Update("episodes_matched", matchedCount).Error; err != nil {
			return fmt.Errorf("更新Job的episodes_matched失败: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	logger.Infof("✅ 已更新Job的episodes_matched [JobID=%d, Matched=%d]", job.ID, matchedCount)

	return report, nil
}

func (rg *ReportGenerator) feedCoverage(job *models.Job) FeedCoverageSummary {
	coverage := FeedCoverageSummary{}
	if job == nil || rg == nil || rg.db == nil {
		return coverage
	}
	coverage.Total = job.PodcastsProcessed
	if attempts, err := ListFeedAttempts(rg.db, job.ID); err == nil && len(attempts) > 0 {
		summary := BuildRootCauseSummary(attempts)
		coverage.Total = summary.TotalFeeds
		coverage.Attempted = summary.AttemptedFeeds
		coverage.Successes = summary.FinalSuccesses
		coverage.Failures = summary.FinalFailures
		coverage.Unattempted = summary.UnattemptedFeeds
		if job.PodcastsProcessed > coverage.Total {
			coverage.Total = job.PodcastsProcessed
			coverage.Unattempted = coverage.Total - coverage.Attempted
		}
		return coverage
	}

	var executions []models.JobExecution
	if err := rg.db.Where("job_id = ?", job.ID).Find(&executions).Error; err != nil {
		return coverage
	}
	coverage.Attempted = len(executions)
	for _, execution := range executions {
		switch execution.Status {
		case models.ExecutionStatusSuccess:
			coverage.Successes++
		case models.ExecutionStatusFailed:
			coverage.Failures++
		}
	}
	if coverage.Total < coverage.Attempted {
		coverage.Total = coverage.Attempted
	}
	coverage.Unattempted = coverage.Total - coverage.Attempted
	return coverage
}

// collectMatchedEpisodes 收集在时间窗口内匹配的episodes（优化N+1查询）
func (rg *ReportGenerator) collectMatchedEpisodes(job *models.Job, timeRangeStart, timeRangeEnd time.Time) ([]EpisodeReportData, error) {
	// 从JobExecution中获取成功处理的podcast IDs
	var executions []models.JobExecution
	if err := rg.db.Where("job_id = ? AND status = ?", job.ID, models.ExecutionStatusSuccess).Find(&executions).Error; err != nil {
		return nil, err
	}

	if len(executions) == 0 {
		return []EpisodeReportData{}, nil
	}

	// 收集所有 podcast IDs 和建立 execution 映射
	podcastIDs := make([]uint, 0, len(executions))
	executionMap := make(map[uint]models.JobExecution)
	for _, exec := range executions {
		if exec.PodcastID != nil {
			podcastIDs = append(podcastIDs, *exec.PodcastID)
			executionMap[*exec.PodcastID] = exec
		}
	}

	if len(podcastIDs) == 0 {
		return []EpisodeReportData{}, nil
	}

	// 批量查询所有相关的 episodes（优化 N+1 查询）
	var allEpisodes []models.Episode
	query := rg.db.Where("podcast_id IN ?", podcastIDs)
	query = query.Where(`
		COALESCE(updated_date, published_date) >= ? AND
		COALESCE(updated_date, published_date) <= ?
	`, timeRangeStart, timeRangeEnd)
	query = query.Order("podcast_id, COALESCE(updated_date, published_date) DESC")

	if err := query.Find(&allEpisodes).Error; err != nil {
		return nil, err
	}

	// 按 podcast_id 分组
	episodesByPodcast := make(map[uint][]models.Episode)
	for _, ep := range allEpisodes {
		episodesByPodcast[ep.PodcastID] = append(episodesByPodcast[ep.PodcastID], ep)
	}

	// Stable podcast order: by newest matched episode, then podcast id.
	sortedPodcastIDs := make([]uint, 0, len(executionMap))
	for podcastID := range executionMap {
		if episodes, ok := episodesByPodcast[podcastID]; ok && len(episodes) > 0 {
			sortedPodcastIDs = append(sortedPodcastIDs, podcastID)
		}
	}
	sort.SliceStable(sortedPodcastIDs, func(i, j int) bool {
		left := episodesByPodcast[sortedPodcastIDs[i]][0]
		right := episodesByPodcast[sortedPodcastIDs[j]][0]
		leftTime := left.PublishedDate
		if left.UpdatedDate != nil && left.UpdatedDate.After(leftTime) {
			leftTime = *left.UpdatedDate
		}
		rightTime := right.PublishedDate
		if right.UpdatedDate != nil && right.UpdatedDate.After(rightTime) {
			rightTime = *right.UpdatedDate
		}
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return sortedPodcastIDs[i] < sortedPodcastIDs[j]
	})

	// Podcast covers for structured homepage episodes (not inferred from Markdown).
	coverByPodcast := make(map[uint]string, len(sortedPodcastIDs))
	if len(sortedPodcastIDs) > 0 {
		var podcasts []models.Podcast
		if err := rg.db.Select("id", "cover_url", "custom_cover_url").
			Where("id IN ?", sortedPodcastIDs).
			Find(&podcasts).Error; err == nil {
			for _, podcast := range podcasts {
				cover := podcast.CustomCoverURL
				if cover == "" {
					cover = podcast.CoverURL
				}
				coverByPodcast[podcast.ID] = cover
			}
		}
	}

	// 构建 reportData
	var reportData []EpisodeReportData
	for _, podcastID := range sortedPodcastIDs {
		exec := executionMap[podcastID]
		episodes := episodesByPodcast[podcastID]

		// 转换为报告数据结构
		episodeDetails := make([]EpisodeDetail, len(episodes))
		for i, ep := range episodes {
			// 提取小宇宙episode ID（从link中）
			var xyzID string
			if ep.Link != "" {
				// 解析小宇宙链接：https://www.xiaoyuzhoufm.com/episode/{id}?utm_source=rss
				parts := strings.Split(ep.Link, "/")
				if len(parts) >= 5 && parts[3] == "episode" {
					xyzID = strings.Split(parts[4], "?")[0] // 去掉URL参数
				}
			}

			// 生成二维码
			var qrCode string
			var qrCodeError bool // 默认无错误
			if xyzID != "" {
				var err error
				qrCode, err = utils.GenerateQRCodeForEpisode(xyzID, 128)
				if err != nil {
					// 标记二维码生成失败，但不中断流程
					qrCodeError = true
					logger.Warnf("⚠️  生成二维码失败 [EpisodeID=%d, XYZID=%s]: %v", ep.ID, xyzID, err)
				}
			}

			episodeDetails[i] = EpisodeDetail{
				EpisodeID:     ep.ID,
				Title:         ep.Title,
				ShowNotes:     utils.HTMLToMarkdown(ep.ShowNotes),
				PublishedDate: ep.PublishedDate,
				UpdatedDate:   ep.UpdatedDate,
				EpisodeNo:     ep.EpisodeNo,
				Duration:      ep.Duration,
				ImageURL:      ep.ImageURL,
				Link:          ep.Link,
				XYZID:         xyzID,
				QRCode:        qrCode,
				QRCodeError:   qrCodeError, // 记录二维码生成是否失败
			}
		}

		reportData = append(reportData, EpisodeReportData{
			PodcastID:       podcastID,
			PodcastTitle:    exec.PodcastTitle,
			PodcastFeedURL:  exec.PodcastFeedURL,
			PodcastCoverURL: coverByPodcast[podcastID],
			Episodes:        episodeDetails,
		})
	}

	return reportData, nil
}

// buildStructuredEpisodes produces ordered report episodes with real episode IDs.
// Only items with EpisodeID > 0 are included; Markdown is never parsed for identity.
// Show Notes become Context only — never Recommendation (#93).
func (rg *ReportGenerator) buildStructuredEpisodes(data []EpisodeReportData) models.ReportEpisodeList {
	if len(data) == 0 {
		return nil
	}
	out := make(models.ReportEpisodeList, 0)
	order := 1
	for _, podcast := range data {
		for _, ep := range podcast.Episodes {
			if ep.EpisodeID == 0 {
				continue
			}
			imageURL := ep.ImageURL
			published := ""
			if !ep.PublishedDate.IsZero() {
				published = ep.PublishedDate.UTC().Format(time.RFC3339)
			}
			out = append(out, models.ReportEpisode{
				EpisodeID:       ep.EpisodeID,
				Order:           order,
				PodcastID:       podcast.PodcastID,
				PodcastTitle:    podcast.PodcastTitle,
				PodcastCoverURL: podcast.PodcastCoverURL,
				EpisodeTitle:    ep.Title,
				EpisodeNo:       ep.EpisodeNo,
				Duration:        ep.Duration,
				PublishedDate:   published,
				ImageURL:        imageURL,
				Link:            sanitizeHomepageEpisodeLink(ep.Link),
				// No per-episode report rationale is produced today; leave empty so UI degrades explicitly.
				Recommendation: "",
				Context:        contextForReportEpisode(ep.ShowNotes),
			})
			order++
		}
	}
	return out
}

const maxReportEpisodeContextRunes = 280

func contextForReportEpisode(showNotes string) string {
	text := strings.TrimSpace(showNotes)
	if text == "" {
		return ""
	}
	// Compact program context; not a report recommendation (#93).
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= maxReportEpisodeContextRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxReportEpisodeContextRunes]) + "…"
}

// sanitizeHomepageEpisodeLink keeps only safe http(s) episode links for homepage actions.
func sanitizeHomepageEpisodeLink(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "vbscript:") {
		return ""
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return raw
	}
	// Relative paths and other schemes are not homepage-safe clickable links.
	return ""
}

// generateMarkdown 生成Markdown内容
func (rg *ReportGenerator) generateMarkdown(job *models.Job, data []EpisodeReportData, timeRangeStart, timeRangeEnd time.Time, timeRangeMode string, workflowName string, llmTokensUsed int, llmModelUsed string, coverage FeedCoverageSummary) string {
	var builder strings.Builder

	// 报告标题（简化时间格式：只到分钟）
	builder.WriteString(fmt.Sprintf("# %s - %s\n\n", workflowName, job.CreatedAt.Format("2006-01-02 15:04")))

	// 紧凑元数据卡片（引用块）
	totalEpisodes := rg.countEpisodes(data)
	timeWindowDesc := rg.formatTimeWindow(timeRangeStart, timeRangeEnd)

	// 基础元数据行
	metaLine := fmt.Sprintf(
		"> **🕐 执行**: %s | **⏱️ 窗口**: %s (%s) | **📊 统计**: %d节目/%d单集",
		job.CreatedAt.Format("2006-01-02 15:04:05"),
		timeWindowDesc,
		job.TriggeredBy, // 显示触发模式
		len(data),
		totalEpisodes,
	)

	// 如果使用了 LLM，追加 token 统计
	if llmTokensUsed > 0 && llmModelUsed != "" {
		tokenStr := formatTokenCount(llmTokensUsed)
		metaLine += fmt.Sprintf(" | **🤖 AI**: %s (%s)", tokenStr, llmModelUsed)
	}

	metaLine += "\n\n"
	builder.WriteString(metaLine)
	if coverage.Total > 0 {
		builder.WriteString(fmt.Sprintf("> **📡 Feed覆盖**: %d/%d 已尝试 | %d 成功 | %d 失败 | %d 未尝试\n\n",
			coverage.Attempted, coverage.Total, coverage.Successes, coverage.Failures, coverage.Unattempted))
	}

	builder.WriteString("---\n\n")

	// 按播客分组
	if len(data) > 0 {
		builder.WriteString("## 📝 单集详情\n\n")

		for _, podcast := range data {
			// 节目名称
			builder.WriteString(fmt.Sprintf("### 节目名称：%s\n\n", podcast.PodcastTitle))
			builder.WriteString(fmt.Sprintf("**RSS源**: %s\n\n", podcast.PodcastFeedURL))
			builder.WriteString(fmt.Sprintf("**单集数量**: %d\n\n", len(podcast.Episodes)))

			for _, ep := range podcast.Episodes {
				// 单集标题（不使用序号）
				builder.WriteString(fmt.Sprintf("#### 单集名称：%s\n\n", ep.Title))

				// 元数据行
				if ep.EpisodeNo != "" {
					builder.WriteString(fmt.Sprintf("**期号**: %s | ", ep.EpisodeNo))
				}
				builder.WriteString(fmt.Sprintf("**发布时间**: %s", ep.PublishedDate.Format("2006-01-02 15:04")))

				if ep.UpdatedDate != nil {
					builder.WriteString(fmt.Sprintf(" | **更新时间**: %s", ep.UpdatedDate.Format("2006-01-02 15:04")))
				}
				builder.WriteString("\n\n")

				// 小宇宙二维码或错误提示
				if ep.QRCodeError {
					builder.WriteString("**⚠️ 二维码生成失败**\n\n")
				} else if ep.QRCode != "" {
					builder.WriteString(fmt.Sprintf("![二维码](%s)\n\n", ep.QRCode))
				}

				if ep.Link != "" {
					builder.WriteString(fmt.Sprintf("**链接**: [%s](%s)\n\n", ep.Link, ep.Link))
				}

				// Show Notes
				showNotes := strings.TrimSpace(ep.ShowNotes)
				if showNotes != "" {
					builder.WriteString("**节目详情**:\n\n")
					builder.WriteString(showNotes)
					builder.WriteString("\n\n")
				}

				builder.WriteString("---\n\n")
			}
		}
	} else {
		builder.WriteString("## 📝 单集详情\n\n")
		builder.WriteString("本次执行在指定时间窗口内未匹配到单集内容。\n")
	}

	return builder.String()
}

// generateSummary 生成摘要
func (rg *ReportGenerator) generateSummary(job *models.Job, data []EpisodeReportData) string {
	totalEpisodes := rg.countEpisodes(data)
	qrCodeErrors := rg.countQRCodeErrors(data)

	summary := fmt.Sprintf("处理了 %d 个节目，共 %d 个单集", len(data), totalEpisodes)
	if qrCodeErrors > 0 {
		summary += fmt.Sprintf("（%d 个单集二维码生成失败）", qrCodeErrors)
	}
	return summary
}

// countEpisodes 统计总episode数
func (rg *ReportGenerator) countEpisodes(data []EpisodeReportData) int {
	count := 0
	for _, podcast := range data {
		count += len(podcast.Episodes)
	}
	return count
}

// countQRCodeErrors 统计二维码生成失败的数量
func (rg *ReportGenerator) countQRCodeErrors(data []EpisodeReportData) int {
	count := 0
	for _, podcast := range data {
		for _, ep := range podcast.Episodes {
			if ep.QRCodeError {
				count++
			}
		}
	}
	return count
}

// formatTime 格式化时间指针
func formatTime(t *time.Time) string {
	if t == nil {
		return "N/A"
	}
	return t.Format("2006-01-02 15:04:05")
}

// formatTimeWindow 格式化时间窗口为人类可读的描述
func (rg *ReportGenerator) formatTimeWindow(start, end time.Time) string {
	duration := end.Sub(start)

	days := int(duration.Hours() / 24)
	hours := int(duration.Hours()) % 24

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%d天%d小时", days, hours)
		}
		return fmt.Sprintf("%d天", days)
	}

	hours = int(duration.Hours())
	if hours > 0 {
		return fmt.Sprintf("%d小时", hours)
	}

	minutes := int(duration.Minutes())
	return fmt.Sprintf("%d分钟", minutes)
}

// formatTokenCount 格式化token数量（人类可读）
func formatTokenCount(tokens int) string {
	if tokens == 0 {
		return "0"
	}

	if tokens < 1000 {
		return fmt.Sprintf("%d", tokens)
	} else if tokens < 1000000 {
		return fmt.Sprintf("%.1fK", float64(tokens)/1000)
	} else {
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000)
	}
}

// insertLLMSummary 将LLM摘要插入到标题之后、元数据卡片之前
func (rg *ReportGenerator) insertLLMSummary(markdown, llmSummary string) string {
	// markdown格式：
	// # 标题 - 时间\n\n
	// > 元数据卡片\n\n
	// ---\n\n
	// ## 单集详情\n\n
	//
	// 我们需要在标题和元数据卡片之间插入AI摘要

	lines := strings.Split(markdown, "\n")
	var result strings.Builder
	inserted := false

	for i, line := range lines {
		// 第一行是标题（# 标题 - 时间）
		if i == 0 {
			result.WriteString(line)
			result.WriteString("\n\n")
			// 在标题之后插入AI摘要
			result.WriteString("## 🤖 AI智能摘要\n\n")
			result.WriteString(llmSummary)
			result.WriteString("\n\n---\n\n")
			inserted = true
		} else {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	if !inserted {
		// 如果没有找到标题，fallback到原来的逻辑（追加到开头）
		var builder strings.Builder
		builder.WriteString("## 🤖 AI智能摘要\n\n")
		builder.WriteString(llmSummary)
		builder.WriteString("\n\n---\n\n")
		builder.WriteString(markdown)
		return builder.String()
	}

	return result.String()
}
