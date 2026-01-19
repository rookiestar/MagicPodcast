package workflow

import (
	"fmt"
	"log"
	"strings"
	"time"

	"magicpodcast/internal/models"
	"magicpodcast/internal/utils"

	"gorm.io/gorm"
)

// ReportGenerator 报告生成器
type ReportGenerator struct {
	db *gorm.DB
}

// NewReportGenerator 创建报告生成器
func NewReportGenerator(db *gorm.DB) *ReportGenerator {
	return &ReportGenerator{db: db}
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
	triggeredAt := job.CreatedAt
	var timeRangeMode utils.TimeRangeMode
	var timeRangeDays int

	// 判断触发模式：根据job的triggered_by字段判断
	if job.TriggeredBy == "cron" {
		timeRangeMode = utils.TimeRangeModeDaily
		timeRangeDays = 0 // daily模式不使用days参数
	} else {
		timeRangeMode = utils.TimeRangeModeManual
		timeRangeDays = workflow.RulesConfig.TimeRange
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

	// 5. 创建Report记录
	matchedCount := rg.countEpisodes(reportData)
	report := &models.Report{
		JobID:          job.ID,
		Title:          fmt.Sprintf("工作流执行报告 - %s", job.CreatedAt.Format("2006-01-02 15:04:05")),
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
	}

	if err := rg.db.Create(report).Error; err != nil {
		return nil, fmt.Errorf("保存报告失败: %w", err)
	}

	// 6. 更新Job的episodes_matched为实际匹配的单集数
	if err := rg.db.Model(job).Update("episodes_matched", matchedCount).Error; err != nil {
		log.Printf("❌ 更新Job的episodes_matched失败 [JobID=%d]: %v", job.ID, err)
	} else {
		log.Printf("✅ 已更新Job的episodes_matched [JobID=%d, Matched=%d]", job.ID, matchedCount)
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
					fmt.Printf("⚠️  生成二维码失败 [EpisodeID=%d, XYZID=%s]: %v\n", ep.ID, xyzID, err)
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

	// 报告标题
	builder.WriteString(fmt.Sprintf("# %s - %s\n\n", workflowName, job.CreatedAt.Format("2006-01-02 15:04:05")))

	// 时间范围信息
	builder.WriteString("## ⏱️ 时间范围\n\n")
	modeDesc := "手动触发"
	if timeRangeMode == "daily" {
		modeDesc = "自动定时"
	}
	builder.WriteString(fmt.Sprintf("- **触发模式**: %s\n", modeDesc))
	builder.WriteString(fmt.Sprintf("- **时间窗口**: %s 至 %s\n\n",
		timeRangeStart.Format("2006-01-02 15:04:05"),
		timeRangeEnd.Format("2006-01-02 15:04:05")))

	// 按播客分组
	if len(data) > 0 {
		builder.WriteString("## 📝 单集简介\n\n")

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
		builder.WriteString("## 📝 单集简介\n\n")
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
