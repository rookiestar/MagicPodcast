package sync

import (
	"fmt"
	"magicpodcast/internal/logger"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/opml"
	"magicpodcast/internal/podcastindex"
	"magicpodcast/internal/scraper"

	"gorm.io/gorm"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries    int           // 最大重试次数
	InitialDelay  time.Duration // 初始延迟
	MaxDelay      time.Duration // 最大延迟
	BackoffFactor float64       // 退避因子
}

// DefaultRetryConfig 默认重试配置
var DefaultRetryConfig = RetryConfig{
	MaxRetries:    3,
	InitialDelay:  2 * time.Second, // 从2秒开始：2s -> 4s -> 8s
	MaxDelay:      8 * time.Second,
	BackoffFactor: 2.0,
}

// ImportConfig 导入配置
type ImportConfig struct {
	Concurrency int // 并发数（默认 10）
}

// DefaultImportConfig 默认导入配置
var DefaultImportConfig = ImportConfig{
	Concurrency: 10,
}

// Service 同步服务
type Service struct {
	db                *gorm.DB
	opmlParser        *opml.Parser
	feedFetcher       *feed.Fetcher
	podcastIndexQuery *podcastindex.Query
	scraper           *scraper.Scraper
}

// SyncResult 同步结果
type SyncResult struct {
	TotalPodcasts   int      `json:"total_podcasts"`
	SuccessPodcasts int      `json:"success_podcasts"`
	FailedPodcasts  int      `json:"failed_podcasts"`
	NewEpisodes     int      `json:"new_episodes"`
	Errors          []string `json:"errors,omitempty"`
}

// EpisodeSyncMode Episode同步模式
type EpisodeSyncMode string

const (
	SyncModeIncremental EpisodeSyncMode = "incremental" // 增量同步
	SyncModeFull        EpisodeSyncMode = "full"        // 全量同步
	SyncModeSmart       EpisodeSyncMode = "smart"       // 智能模式
)

// EpisodeSyncConfig Episode同步配置
type EpisodeSyncConfig struct {
	Mode                  EpisodeSyncMode // 同步模式
	MaxEpisodesPerPodcast int             // 单次同步最大episode数（防止单个podcast过大）
	UpdateExisting        bool            // 是否更新已存在的episode
	DeleteMissing         bool            // 是否删除feed中不存在的episode（谨慎使用）
	TimeRangeDays         *int            // 时间范围（天数），仅在增量模式下使用
}

// DefaultEpisodeSyncConfig 默认Episode同步配置
var DefaultEpisodeSyncConfig = EpisodeSyncConfig{
	Mode:                  SyncModeSmart,
	MaxEpisodesPerPodcast: 1000,  // 默认最多同步1000个episode
	UpdateExisting:        true,  // 更新已存在的episode
	DeleteMissing:         false, // 不自动删除（安全考虑）
}

// ParseEpisodeSyncMode 解析同步模式字符串，无效值返回默认的 Smart 模式
func ParseEpisodeSyncMode(mode string) EpisodeSyncMode {
	switch mode {
	case "incremental":
		return SyncModeIncremental
	case "full":
		return SyncModeFull
	case "smart":
		return SyncModeSmart
	default:
		return SyncModeSmart
	}
}

// EpisodeSyncResult Episode同步结果
type EpisodeSyncResult struct {
	PodcastID    uint                `json:"podcast_id"`
	PodcastTitle string              `json:"podcast_title"`
	Created      int                 `json:"created"` // 新增数量
	Updated      int                 `json:"updated"` // 更新数量
	Skipped      int                 `json:"skipped"` // 跳过数量
	Deleted      int                 `json:"deleted"` // 删除数量
	Errors       int                 `json:"errors"`  // 错误数量
	FeedAccess   *feed.AccessOutcome `json:"feed_access,omitempty"`
}

// NewService 创建同步服务
func NewService(db *gorm.DB, podcastIndexPath string) (*Service, error) {
	return NewServiceWithFeedCoordinator(db, podcastIndexPath, feed.SharedCoordinator())
}

// NewServiceWithFeedCoordinator keeps the workflow seam testable while the
// normal constructor uses the process-wide Feed coordination boundary.
func NewServiceWithFeedCoordinator(db *gorm.DB, podcastIndexPath string, coordinator *feed.Coordinator) (*Service, error) {
	// 初始化PodcastIndex查询器
	podcastIndexQuery, err := podcastindex.NewQuery(podcastIndexPath)
	if err != nil {
		logger.Infof("Warning: Failed to initialize PodcastIndex query: %v", err)
		// 不返回错误，继续创建服务（PodcastIndex是可选的）
		podcastIndexQuery = nil
	}

	return &Service{
		db:                db,
		opmlParser:        opml.NewParser(),
		feedFetcher:       feed.NewFetcherWithCoordinator(30*time.Second, coordinator),
		podcastIndexQuery: podcastIndexQuery,
		scraper:           scraper.NewScraper(),
	}, nil
}

// Close 关闭服务，释放资源
func (s *Service) Close() error {
	if s.podcastIndexQuery != nil {
		return s.podcastIndexQuery.Close()
	}
	return nil
}

// SyncAllPodcasts 同步所有订阅的播客（非SSE版本，用于REST API）
func (s *Service) SyncAllPodcasts() (*SyncResult, error) {
	// 创建一个简单的进度报告器来收集结果
	result := &SyncResult{}

	reporter := &progressReporter{
		onProgress: func(msg string) {
			logger.Info(msg)
		},
		onError: func(msg string) {
			result.Errors = append(result.Errors, msg)
		},
	}

	// 调用SSE版本的同步方法
	err := s.SyncPodcastsMetadataSSE(reporter)
	if err != nil {
		return result, err
	}

	// 统计结果
	result.TotalPodcasts = reporter.totalPodcasts
	result.SuccessPodcasts = reporter.successPodcasts
	result.FailedPodcasts = reporter.failedPodcasts
	result.NewEpisodes = reporter.newEpisodes

	return result, nil
}

// progressReporter 简单的进度报告器实现（用于REST API）
type progressReporter struct {
	totalPodcasts    int
	successPodcasts  int
	failedPodcasts   int
	skippedPodcasts  int
	noUpdatePodcasts int
	newEpisodes      int
	updatedEpisodes  int
	totalEpisodes    int
	onProgress       func(msg string)
	onError          func(msg string)
}

func (r *progressReporter) Report(msg string) {
	if r.onProgress != nil {
		r.onProgress(msg)
	}
}

func (r *progressReporter) ReportSuccess(msg string) {
	if r.onProgress != nil {
		r.onProgress(msg)
	}
}

func (r *progressReporter) ReportError(msg string) {
	if r.onError != nil {
		r.onError(msg)
	}
}

func (r *progressReporter) ReportProgress(current, total int, message string) {
	r.totalPodcasts = total
	if r.onProgress != nil {
		r.onProgress(fmt.Sprintf("[%d/%d] %s", current, total, message))
	}
}

func (r *progressReporter) ReportSkip(reason SkipReason, message string) {
	r.skippedPodcasts++
	if reason == SkipReasonNoUpdate {
		r.noUpdatePodcasts++
	}
}

func (r *progressReporter) ReportSummary(summary *SyncSummary) {
	r.totalPodcasts = summary.TotalPodcasts
	r.successPodcasts = summary.SuccessPodcasts
	r.failedPodcasts = summary.FailedPodcasts
	r.skippedPodcasts = summary.SkippedPodcasts
	r.noUpdatePodcasts = summary.NoUpdatePodcasts
	r.totalEpisodes = summary.TotalEpisodes
	r.newEpisodes = summary.NewEpisodes
	r.updatedEpisodes = summary.UpdatedEpisodes
}

func (r *progressReporter) Close() {
	// Nothing to close
}
