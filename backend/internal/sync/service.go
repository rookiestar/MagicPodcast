package sync

import (
	"log"
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
	InitialDelay:  1 * time.Second,
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
	TotalPodcasts   int     `json:"total_podcasts"`
	SuccessPodcasts int     `json:"success_podcasts"`
	FailedPodcasts  int     `json:"failed_podcasts"`
	NewEpisodes     int     `json:"new_episodes"`
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
	Mode                 EpisodeSyncMode // 同步模式
	MaxEpisodesPerPodcast int             // 单次同步最大episode数（防止单个podcast过大）
	UpdateExisting       bool            // 是否更新已存在的episode
	DeleteMissing        bool            // 是否删除feed中不存在的episode（谨慎使用）
}

// DefaultEpisodeSyncConfig 默认Episode同步配置
var DefaultEpisodeSyncConfig = EpisodeSyncConfig{
	Mode:                 SyncModeSmart,
	MaxEpisodesPerPodcast: 1000, // 默认最多同步1000个episode
	UpdateExisting:       true,  // 更新已存在的episode
	DeleteMissing:        false, // 不自动删除（安全考虑）
}

// EpisodeSyncResult Episode同步结果
type EpisodeSyncResult struct {
	PodcastID    uint   `json:"podcast_id"`
	PodcastTitle string `json:"podcast_title"`
	Created      int    `json:"created"`   // 新增数量
	Updated      int    `json:"updated"`   // 更新数量
	Skipped      int    `json:"skipped"`   // 跳过数量
	Deleted      int    `json:"deleted"`   // 删除数量
	Errors       int    `json:"errors"`    // 错误数量
}

// NewService 创建同步服务
func NewService(db *gorm.DB, podcastIndexPath string) (*Service, error) {
	// 初始化PodcastIndex查询器
	podcastIndexQuery, err := podcastindex.NewQuery(podcastIndexPath)
	if err != nil {
		log.Printf("Warning: Failed to initialize PodcastIndex query: %v", err)
		// 不返回错误，继续创建服务（PodcastIndex是可选的）
		podcastIndexQuery = nil
	}

	return &Service{
		db:                db,
		opmlParser:        opml.NewParser(),
		feedFetcher:       feed.NewFetcher(30 * time.Second),
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
