package workflow

import (
	"fmt"
	"strings"
	"time"

	"magicpodcast/internal/models"

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
}

// GenerateForJob 为Job生成报告
func (rg *ReportGenerator) GenerateForJob(job *models.Job) (*models.Report, error) {
	// 1. 收集数据
	reportData, err := rg.collectJobEpisodes(job)
	if err != nil {
		return nil, fmt.Errorf("收集报告数据失败: %w", err)
	}

	// 2. 生成Markdown内容
	markdown := rg.generateMarkdown(job, reportData)

	// 3. 创建Report记录
	report := &models.Report{
		JobID:         job.ID,
		Title:         fmt.Sprintf("工作流执行报告 - %s", job.CreatedAt.Format("2006-01-02 15:04:05")),
		Content:       markdown,
		Summary:       rg.generateSummary(job, reportData),
		EpisodesCount: rg.countEpisodes(reportData),
		PodcastsCount: len(reportData),
		GeneratedAt:   time.Now(),
		Format:        "markdown",
		FileSize:      len(markdown),
	}

	if err := rg.db.Create(report).Error; err != nil {
		return nil, fmt.Errorf("保存报告失败: %w", err)
	}

	return report, nil
}

// collectJobEpisodes 收集Job相关的所有episodes
func (rg *ReportGenerator) collectJobEpisodes(job *models.Job) ([]EpisodeReportData, error) {
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

		// 根据时间范围查询episodes
		var episodes []models.Episode
		query := rg.db.Where("podcast_id = ?", *exec.PodcastID)

		// 应用时间过滤：查询在job执行期间新增或更新的episodes
		if job.StartTime != nil {
			query = query.Where("(created_at >= ? OR updated_date >= ?)", *job.StartTime, *job.StartTime)
		}

		// 按updated_date降序排序
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
			episodeDetails[i] = EpisodeDetail{
				Title:         ep.Title,
				ShowNotes:     ep.ShowNotes,
				PublishedDate: ep.PublishedDate,
				UpdatedDate:   ep.UpdatedDate,
				EpisodeNo:     ep.EpisodeNo,
				Link:          ep.Link,
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
func (rg *ReportGenerator) generateMarkdown(job *models.Job, data []EpisodeReportData) string {
	var builder strings.Builder

	// 标题
	builder.WriteString("# 工作流执行报告\n\n")
	builder.WriteString(fmt.Sprintf("**生成时间**: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	builder.WriteString(fmt.Sprintf("**执行时间**: %s - %s\n\n",
		formatTime(job.StartTime), formatTime(job.EndTime)))

	// 统计信息
	builder.WriteString("## 📊 执行统计\n\n")
	builder.WriteString(fmt.Sprintf("- **状态**: %s\n", job.Status))
	builder.WriteString(fmt.Sprintf("- **处理节目数**: %d\n", job.PodcastsProcessed))
	builder.WriteString(fmt.Sprintf("- **发现单集数**: %d\n", job.EpisodesFound))
	builder.WriteString(fmt.Sprintf("- **创建单集数**: %d\n", job.EpisodesCreated))
	if job.ErrorCount > 0 {
		builder.WriteString(fmt.Sprintf("- **错误数**: %d\n", job.ErrorCount))
	}
	builder.WriteString(fmt.Sprintf("- **触发方式**: %s\n\n", job.TriggeredBy))

	// 按播客分组
	if len(data) > 0 {
		builder.WriteString("## 📝 抓取详情\n\n")

		for _, podcast := range data {
			builder.WriteString(fmt.Sprintf("### %s\n\n", podcast.PodcastTitle))
			builder.WriteString(fmt.Sprintf("**RSS源**: %s\n\n", podcast.PodcastFeedURL))
			builder.WriteString(fmt.Sprintf("**单集数量**: %d\n\n", len(podcast.Episodes)))

			for i, ep := range podcast.Episodes {
				builder.WriteString(fmt.Sprintf("#### %d. %s\n\n", i+1, ep.Title))

				// 元数据行
				if ep.EpisodeNo != "" {
					builder.WriteString(fmt.Sprintf("**期号**: %s | ", ep.EpisodeNo))
				}
				builder.WriteString(fmt.Sprintf("**发布时间**: %s", ep.PublishedDate.Format("2006-01-02 15:04")))

				if ep.UpdatedDate != nil {
					builder.WriteString(fmt.Sprintf(" | **更新时间**: %s", ep.UpdatedDate.Format("2006-01-02 15:04")))
				}
				builder.WriteString("\n\n")

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
		builder.WriteString("本次执行未抓取到新的单集内容。\n")
	}

	return builder.String()
}

// generateSummary 生成摘要
func (rg *ReportGenerator) generateSummary(job *models.Job, data []EpisodeReportData) string {
	totalEpisodes := rg.countEpisodes(data)
	return fmt.Sprintf("处理了 %d 个节目，共 %d 个单集", len(data), totalEpisodes)
}

// countEpisodes 统计总episode数
func (rg *ReportGenerator) countEpisodes(data []EpisodeReportData) int {
	count := 0
	for _, podcast := range data {
		count += len(podcast.Episodes)
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
