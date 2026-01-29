package workflow

import (
	"fmt"
	"magicpodcast/internal/logger"
	"strings"
	"time"

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
	PodcastID      uint
	PodcastTitle   string
	PodcastFeedURL string
	Episodes       []EpisodeDetail
}

// EpisodeDetail 单集详情
type EpisodeDetail struct {
	Title         string
	ShowNotes     string
	PublishedDate time.Time
	UpdatedDate   *time.Time
	EpisodeNo     string
	Link          string
	XYZID         string // 小宇宙ID
	QRCode        string // 小宇宙二维码（base64编码）
	QRCodeError   bool   // 二维码生成失败标记
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

	// 4. 生成Markdown内容
	markdown := rg.generateMarkdown(job, reportData, timeRangeStart, timeRangeEnd, string(timeRangeMode), workflow.Name)

	// 5. 生成LLM摘要（如果启用）
	var llmSummary string
	var llmError string
	var llmModelUsed string
	var llmTokensUsed int

	logger.Infof("[ReportGenerator] LLMEnabled=%v, SummarizerNil=%v, MatchedEpisodes=%d",
		workflow.RulesConfig.LLMEnabled, rg.summarizer == nil, len(reportData))

	// 优化：如果没有匹配的单集，跳过LLM摘要生成，不设置任何LLM字段
	if len(reportData) == 0 {
		logger.Infof("⏭️  [JobID=%d] 没有匹配的单集，跳过LLM摘要生成（不显示AI相关内容）", job.ID)
		// 不设置任何LLM字段，让前端完全不显示AI相关信息
	} else if workflow.RulesConfig.LLMEnabled && rg.summarizer != nil {
		logger.Infof("🤖 开始生成LLM摘要 [JobID=%d]", job.ID)
		logger.Infof("  - Summarizer type: %T", rg.summarizer)

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
		llmReportData := rg.convertToLLMReportData(reportData)
		logger.Infof("  - Converted %d podcasts to LLM report format", len(llmReportData))

		// 调用摘要生成器（只传入user prompt，system prompt从config获取）
		logger.Infof("  - Calling summarizer.GenerateForReport...")
		result, err := rg.summarizer.GenerateForReport(
			llmReportData,
			workflow.Name,
			workflow.RulesConfig.LLMUserPrompt, // 只传入user prompt
			options,
		)
		if err != nil {
			logger.Infof("⚠️  LLM摘要生成失败 [JobID=%d]: %v", job.ID, err)
			llmError = err.Error()
			// 不中断流程，继续生成基础报告
		} else {
			llmSummary = result.Summary
			llmModelUsed = result.ModelUsed
			llmTokensUsed = result.TokensUsed
			logger.Infof("✅ LLM摘要生成成功 [JobID=%d, Tokens=%d]", job.ID, llmTokensUsed)

			// 将LLM摘要插入到Markdown开头
			markdown = rg.insertLLMSummary(markdown, llmSummary)
		}
	}

	// 6. 创建Report记录
	matchedCount := rg.countEpisodes(reportData)
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
		// LLM相关字段
		LLMSummary:    llmSummary,
		LLMModelUsed:  llmModelUsed,
		LLMTokensUsed: llmTokensUsed,
		LLMError:      llmError,
	}

	if err := rg.db.Create(report).Error; err != nil {
		return nil, fmt.Errorf("保存报告失败: %w", err)
	}

	// 6. 更新Job的episodes_matched为实际匹配的单集数
	if err := rg.db.Model(job).Update("episodes_matched", matchedCount).Error; err != nil {
		logger.Infof("❌ 更新Job的episodes_matched失败 [JobID=%d]: %v", job.ID, err)
	} else {
		logger.Infof("✅ 已更新Job的episodes_matched [JobID=%d, Matched=%d]", job.ID, matchedCount)
	}

	return report, nil
}

// collectMatchedEpisodes 收集在时间窗口内匹配的episodes
func (rg *ReportGenerator) collectMatchedEpisodes(job *models.Job, timeRangeStart, timeRangeEnd time.Time) ([]EpisodeReportData, error) {
	// 从JobExecution中获取成功处理的podcast IDs
	var executions []models.JobExecution
	if err := rg.db.Where("job_id = ? AND status = ?", job.ID, models.ExecutionStatusSuccess).Find(&executions).Error; err != nil {
		return nil, err
	}

	var reportData []EpisodeReportData

	for _, exec := range executions {
		if exec.PodcastID == nil {
			continue
		}

		// 根据时间窗口查询匹配的episodes
		var episodes []models.Episode
		query := rg.db.Where("podcast_id = ?", *exec.PodcastID)

		// 应用时间窗口过滤：查询updated_date或published_date在时间窗口内的episodes
		// 使用COALESCE优先使用updated_date，如果为空则使用published_date
		// 使用 >= 和 <= 包含边界时间点
		query = query.Where(`
			COALESCE(updated_date, published_date) >= ? AND
			COALESCE(updated_date, published_date) <= ?
		`, timeRangeStart, timeRangeEnd)

		// 按updated_date降序排序（优先使用updated_date）
		query = query.Order("COALESCE(updated_date, published_date) DESC")

		if err := query.Find(&episodes).Error; err != nil {
			continue
		}

		if len(episodes) == 0 {
			continue
		}

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
				Title:         ep.Title,
				ShowNotes:     ep.ShowNotes,
				PublishedDate: ep.PublishedDate,
				UpdatedDate:   ep.UpdatedDate,
				EpisodeNo:     ep.EpisodeNo,
				Link:          ep.Link,
				XYZID:         xyzID,
				QRCode:        qrCode,
				QRCodeError:   qrCodeError, // 记录二维码生成是否失败
			}
		}

		reportData = append(reportData, EpisodeReportData{
			PodcastID:      *exec.PodcastID,
			PodcastTitle:   exec.PodcastTitle,
			PodcastFeedURL: exec.PodcastFeedURL,
			Episodes:       episodeDetails,
		})
	}

	return reportData, nil
}

// generateMarkdown 生成Markdown内容
func (rg *ReportGenerator) generateMarkdown(job *models.Job, data []EpisodeReportData, timeRangeStart, timeRangeEnd time.Time, timeRangeMode string, workflowName string) string {
	var builder strings.Builder

	// 报告标题（简化时间格式：只到分钟）
	builder.WriteString(fmt.Sprintf("# %s - %s\n\n", workflowName, job.CreatedAt.Format("2006-01-02 15:04")))

	// 紧凑元数据卡片（引用块）
	totalEpisodes := rg.countEpisodes(data)
	timeWindowDesc := rg.formatTimeWindow(timeRangeStart, timeRangeEnd)

	builder.WriteString(fmt.Sprintf(
		"> **🕐 执行**: %s | **⏱️ 窗口**: %s (%s) | **📊 统计**: %d节目/%d单集\n\n",
		job.CreatedAt.Format("2006-01-02 15:04:05"),
		timeWindowDesc,
		job.TriggeredBy, // 显示触发模式
		len(data),
		totalEpisodes,
	))

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

// convertToLLMReportData 转换为LLM包所需的数据格式
func (rg *ReportGenerator) convertToLLMReportData(data []EpisodeReportData) []llm.EpisodeReportData {
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
