package sync

import (
	"context"
	"fmt"
	"magicpodcast/internal/logger"
	"sync"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	"magicpodcast/internal/opml"
	"magicpodcast/internal/podcastindex"
	"magicpodcast/internal/scraper"

	"gorm.io/gorm"
)

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
	// retryPolicy is the unified outer-retry policy for every Feed fetch in the
	// sync workflow. It is derived from the startup-loaded FeedConfig and is the
	// single source of retryable classification, Retry-After handling, and
	// bounded full-jitter backoff. Tests override it via applyRetryPolicy to
	// inject a no-op sleeper and fixed randomness for determinism.
	retryPolicy feed.RetryPolicy

	// alternativePrewarm limits best-effort PodcastIndex verification work so a
	// successful bulk refresh never blocks on fallback preparation or opens an
	// unbounded goroutine/query fan-out.
	alternativePrewarmMu     sync.Mutex
	alternativePrewarmSem    chan struct{}
	alternativePrewarmWG     sync.WaitGroup
	alternativePrewarmClosed bool
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
	coordinator := feed.SharedCoordinator()
	attachPersistentLastGood(db, coordinator)
	return NewServiceWithFeedCoordinator(db, podcastIndexPath, coordinator)
}

// attachPersistentLastGood upgrades the shared coordinator's last-good store
// from in-process-only to a tiered (memory L1 + SQLite L2) store so a verified
// Feed snapshot survives a restart. It is best-effort: if the database handle is
// absent, the feed_snapshots table does not yet exist, or the startup-loaded
// FeedConfig explicitly disabled durable persistence, the coordinator keeps its
// in-process store and continues normally. Capacity bounds come from the
// startup-loaded feed.snapshot.bounds.
func attachPersistentLastGood(db *gorm.DB, coordinator *feed.Coordinator) {
	if db == nil || coordinator == nil {
		return
	}
	durable, bounds := feed.SharedSnapshotConfig()
	if !durable {
		return
	}
	if !db.Migrator().HasTable(feed.FeedSnapshotsTableName) {
		return
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB == nil {
		logger.Infof("Warning: feed last-good persistence unavailable (no db handle): %v", err)
		return
	}
	store, err := feed.NewSQLiteSnapshotStore(sqlDB, feed.LastGoodStoreConfigFromBounds(bounds))
	if err != nil {
		logger.Infof("Warning: feed last-good persistence unavailable: %v", err)
		return
	}
	coordinator.UsePersistentLastGood(store)
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

	feedFetcher := feed.NewFetcherWithCoordinator(30*time.Second, coordinator)
	if gateStore := persistentUserAgentGateStore(db); gateStore != nil {
		feedFetcher.SetUserAgentGateStore(gateStore)
	}

	return &Service{
		db:                    db,
		opmlParser:            opml.NewParser(),
		feedFetcher:           feedFetcher,
		podcastIndexQuery:     podcastIndexQuery,
		scraper:               scraper.NewScraper(),
		retryPolicy:           feed.SharedRetryPolicy(),
		alternativePrewarmSem: make(chan struct{}, 2),
	}, nil
}

func persistentUserAgentGateStore(db *gorm.DB) feed.UserAgentGateStore {
	if db == nil || !db.Migrator().HasTable(feed.FeedUserAgentGatesTableName) {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB == nil {
		logger.Warnf("persistent User-Agent gate unavailable: %v", err)
		return nil
	}
	store, err := feed.NewSQLiteUserAgentGateStore(sqlDB, feed.SharedUserAgentGateRecoveryConfig())
	if err != nil {
		logger.Warnf("persistent User-Agent gate unavailable: %v", err)
		return nil
	}
	return store
}

// applyRetryPolicy replaces the outer-retry policy. It exists for deterministic
// tests (no-op sleeper, fixed randomness, tight budget); production code never
// overrides the policy derived from the startup-loaded FeedConfig.
func (s *Service) applyRetryPolicy(policy feed.RetryPolicy) {
	s.retryPolicy = policy
}

// Close 关闭服务，释放资源
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	s.alternativePrewarmMu.Lock()
	s.alternativePrewarmClosed = true
	s.alternativePrewarmMu.Unlock()
	s.alternativePrewarmWG.Wait()
	if s.podcastIndexQuery != nil {
		return s.podcastIndexQuery.Close()
	}
	return nil
}

// scheduleAlternativePrewarm starts a bounded, best-effort verification after
// a successful import/refresh. The primary sync path does not wait for the
// optional PodcastIndex query or candidate Feed request; Close waits for work
// already admitted so the query handle is never closed underneath a worker.
func (s *Service) scheduleAlternativePrewarm(podcast *models.Podcast) {
	if s == nil || podcast == nil || podcast.ID == 0 || s.podcastIndexQuery == nil {
		return
	}

	s.alternativePrewarmMu.Lock()
	if s.alternativePrewarmClosed || s.alternativePrewarmSem == nil {
		s.alternativePrewarmMu.Unlock()
		return
	}
	select {
	case s.alternativePrewarmSem <- struct{}{}:
		copy := *podcast
		s.alternativePrewarmWG.Add(1)
		s.alternativePrewarmMu.Unlock()
		go func() {
			defer s.alternativePrewarmWG.Done()
			defer func() { <-s.alternativePrewarmSem }()
			ctx, cancel := context.WithTimeout(context.Background(), AlternativeLiveQueryTimeout)
			defer cancel()
			s.EnsureAlternativeVerified(ctx, &copy)
		}()
	default:
		// A full queue is an intentional bounded drop. The next healthy refresh
		// or an explicit maintenance call can retry without delaying the caller.
		s.alternativePrewarmMu.Unlock()
	}
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
