package collection

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"magicpodcast/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 本文件是 #457 的性能采样入口：以等量级临时数据（92 份清单、1,045 个条目、
// 约 6.6 万有效单集）在同一环境对 ListCollections 串行采样，输出首轮、后续
// 中位数/P95/最大值、身份扫描查询条数与查询计划，并核对列表与详情一致。
// 仅在显式设置 MAGICPODCAST_COLLECTIONS_PERF=1 时运行，不进入常规 CI；
// 输出仅作为诊断证据，不作为自动断言门禁。

const (
	perfCollections         = 92
	perfItems               = 1045
	perfEpisodesPerPodcast  = 104
	perfSoftDeletedEpisodes = 40
)

func TestListCollectionsPerfScale(t *testing.T) {
	if strings.TrimSpace(os.Getenv("MAGICPODCAST_COLLECTIONS_PERF")) != "1" {
		t.Skip("性能采样需显式设置 MAGICPODCAST_COLLECTIONS_PERF=1")
	}

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "collections-perf.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Podcast{},
		&models.Episode{},
		&models.EpisodeTriageDecision{},
		&models.ConsumptionQueueOrder{},
		&models.EpisodeCollection{},
		&models.EpisodeCollectionItem{},
		&models.EpisodeExternalRef{},
		&models.EpisodeCollectionAdoption{},
	))

	seedStart := time.Now()
	expectedAdopted := seedPerfData(t, db)
	var liveEpisodes int64
	require.NoError(t, db.Model(&models.Episode{}).Count(&liveEpisodes).Error)
	t.Logf("种子数据构建耗时 %s（%d 份清单 / %d 条目 / %d 有效单集 / 预期收录条目 %d）",
		time.Since(seedStart).Round(time.Millisecond), perfCollections, perfItems, liveEpisodes, expectedAdopted)

	service := NewService(db)
	counter := newSQLCounter(db, "`episodes`")
	logQueryPlan(t, db)

	summaries, first := runList(t, service)
	t.Logf("首轮耗时 %s，命中 %q 的 SQL %d 条（进程内首个请求；数据库文件为本进程新建，不代表操作系统磁盘冷缓存）",
		first.Round(time.Millisecond), counter.fragment, counter.reset())

	subsequent := make([]time.Duration, 0, 12)
	lastScans := int64(0)
	for i := 0; i < 12; i++ {
		_, duration := runList(t, service)
		subsequent = append(subsequent, duration)
		lastScans = counter.reset()
	}
	t.Logf("后续单次请求命中 %q 的 SQL：%d 条", counter.fragment, lastScans)
	logDurations(t, "后续", subsequent)

	var totalItems, totalAdopted int64
	for _, summary := range summaries {
		totalItems += summary.ItemCount
		totalAdopted += summary.AdoptedCount
	}
	require.Equal(t, int64(perfItems), totalItems, "列表条目总数应等于种子条目数")
	require.Equal(t, int64(expectedAdopted), totalAdopted, "列表收录总数应等于种子预期")

	// 对照：逐份清单详情读取；列表与详情计数必须一致。
	detailStart := time.Now()
	for _, summary := range summaries {
		detail, err := service.GetCollection(summary.ID)
		require.NoError(t, err)
		require.Equal(t, summary.ItemCount, detail.ItemCount, "清单 %d 条目数列表与详情不一致", summary.ID)
		require.Equal(t, summary.AdoptedCount, detail.AdoptedCount, "清单 %d 收录数列表与详情不一致", summary.ID)
	}
	t.Logf("92 份清单详情对照总耗时 %s", time.Since(detailStart).Round(time.Millisecond))
}

func runList(t *testing.T, service *Service) ([]CollectionSummary, time.Duration) {
	t.Helper()
	start := time.Now()
	summaries, err := service.ListCollections("")
	duration := time.Since(start)
	require.NoError(t, err)
	return summaries, duration
}

func logDurations(t *testing.T, label string, durations []time.Duration) {
	t.Helper()
	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mean := time.Duration(0)
	for _, duration := range durations {
		mean += duration
	}
	mean /= time.Duration(len(durations))
	p95Index := (len(sorted)*95 + 99) / 100
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	t.Logf("%s %d 次：中位数 %s P95 %s 最大 %s 均值 %s（样本 %v）",
		label, len(durations),
		sorted[len(sorted)/2].Round(time.Millisecond),
		sorted[p95Index].Round(time.Millisecond),
		sorted[len(sorted)-1].Round(time.Millisecond),
		mean.Round(time.Millisecond), durations)
}

// sqlCounter 统计包含目标片段的已执行 SQL 条数，仅用于诊断证据。
type sqlCounter struct {
	count    int64
	fragment string
}

func newSQLCounter(db *gorm.DB, fragment string) *sqlCounter {
	counter := &sqlCounter{fragment: fragment}
	db.Logger = &countingLogger{Interface: logger.Default.LogMode(logger.Silent), counter: counter}
	return counter
}

func (c *sqlCounter) load() int64 { return atomic.LoadInt64(&c.count) }

func (c *sqlCounter) reset() int64 { return atomic.SwapInt64(&c.count, 0) }

type countingLogger struct {
	logger.Interface
	counter *sqlCounter
}

func (l *countingLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	if strings.Contains(sql, l.counter.fragment) {
		atomic.AddInt64(&l.counter.count, 1)
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, 0 }, err)
}

// logQueryPlan 打印身份匹配查询在当前数据形态下的查询计划（诊断证据）。
func logQueryPlan(t *testing.T, db *gorm.DB) {
	t.Helper()
	query := `SELECT episodes.id, episodes.link, episodes.guid, podcasts.xyz_id
		FROM "episodes" JOIN podcasts ON podcasts.id = episodes.podcast_id AND podcasts.deleted_at IS NULL
		WHERE episodes.id IN (1,2,3) OR episodes.link IN ('https://example.com/e1') OR episodes.guid IN ('guid-1')`
	rows, err := db.Raw("EXPLAIN QUERY PLAN " + query).Rows()
	require.NoError(t, err)
	defer rows.Close()
	values := make([]any, 4)
	for rows.Next() {
		pointers := make([]any, len(values))
		for i := range values {
			pointers[i] = &values[i]
		}
		require.NoError(t, rows.Scan(pointers...))
		parts := make([]string, 0, len(values))
		for _, value := range values {
			switch text := value.(type) {
			case string:
				if text != "" {
					parts = append(parts, text)
				}
			case []byte:
				if len(text) > 0 {
					parts = append(parts, string(text))
				}
			}
		}
		t.Logf("查询计划: %s", strings.Join(parts, " | "))
	}
}

// seedPerfData 构建等量级临时数据，返回预期已收录条目数。
func seedPerfData(t *testing.T, db *gorm.DB) int {
	t.Helper()

	// 节目：639 个有效节目（末位补齐到 66,404 有效单集）+ 2 个软删除节目。
	livePodcasts := (66404 + perfEpisodesPerPodcast - 1) / perfEpisodesPerPodcast // 639
	podcasts := make([]models.Podcast, 0, livePodcasts+2)
	for i := 0; i < livePodcasts+2; i++ {
		podcasts = append(podcasts, models.Podcast{
			Title:        fmt.Sprintf("节目 %04d", i),
			FeedURL:      fmt.Sprintf("https://feed.example.com/pod%04d.xml", i),
			XYZID:        fmt.Sprintf("xyz%04d", i),
			IsSubscribed: true,
		})
	}
	require.NoError(t, db.CreateInBatches(&podcasts, 200).Error)

	// 单集：有效单集 66,404；另有 40 条软删除单集（在有效节目内），
	// 以及 2 个软删除节目各 104 条单集（节目软删除，join 后不可见）。
	type identity struct {
		podcastIndex int
		episodeIndex int
		deleted      bool
	}
	liveCount := 66404
	totalEpisodes := liveCount + perfSoftDeletedEpisodes + 2*perfEpisodesPerPodcast
	identities := make([]identity, totalEpisodes)
	for i := 0; i < totalEpisodes; i++ {
		switch {
		case i < liveCount:
			identities[i] = identity{podcastIndex: i / perfEpisodesPerPodcast, episodeIndex: i}
		case i < liveCount+perfSoftDeletedEpisodes:
			identities[i] = identity{podcastIndex: i % livePodcasts, episodeIndex: i, deleted: true}
		default:
			identities[i] = identity{podcastIndex: livePodcasts + (i-liveCount-perfSoftDeletedEpisodes)/perfEpisodesPerPodcast, episodeIndex: i, deleted: true}
		}
	}
	deletedAt := gorm.DeletedAt{Time: time.Now(), Valid: true}
	// 生产行含 KB 级 ShowNotes 正文；种子按同量级填充，避免行重远低于生产。
	shownotes := strings.Repeat("本期节目聊到技术、社会与个人选择的交织，感谢收听与订阅。", 60)
	episodes := make([]models.Episode, 0, totalEpisodes)
	for _, id := range identities {
		episode := models.Episode{
			PodcastID: podcasts[id.podcastIndex].ID,
			Title:     fmt.Sprintf("单集 %06d", id.episodeIndex),
			ShowNotes: shownotes,
			GUID:      fmt.Sprintf("guid-%06d", id.episodeIndex),
			Link:      fmt.Sprintf("https://www.xiaoyuzhoufm.com/episode/e%06d", id.episodeIndex),
		}
		if id.deleted {
			episode.DeletedAt = deletedAt
		}
		episodes = append(episodes, episode)
	}
	require.NoError(t, db.CreateInBatches(&episodes, 200).Error)

	// 清单与条目：1045 个条目分给 92 份清单（每份 11 条 + 前 33 份各多 1 条）。
	// 条目身份规则：编号 n%4 决定收录方式（0=条目引用、1=外部身份映射、
	// 2=链接匹配、3=GUID+节目约束或未收录），与生产混合形态近似。
	now := time.Now().UTC()
	adopted := 0
	collections := make([]models.EpisodeCollection, perfCollections)
	for c := 0; c < perfCollections; c++ {
		collections[c] = models.EpisodeCollection{
			SourcePlatform:  PlatformXiaoyuzhoufm,
			ExternalID:      fmt.Sprintf("collection-%03d", c),
			Title:           fmt.Sprintf("清单 %03d", c),
			Description:     "等量级性能种子清单",
			Author:          "种子作者",
			SourceURL:       fmt.Sprintf("https://www.xiaoyuzhoufm.com/collection/episode/c%03d", c),
			TotalKnown:      true,
			Revision:        1,
			LastRefreshedAt: &now,
		}
	}
	require.NoError(t, db.CreateInBatches(&collections, 50).Error)

	items := make([]models.EpisodeCollectionItem, 0, perfItems)
	refs := make([]models.EpisodeExternalRef, 0, perfItems)
	for n := 0; n < perfItems; n++ {
		collectionIndex := n % perfCollections
		position := n / perfCollections
		// 条目身份指向真实宿主节目（与生产一致）；未收录条目的单集身份不命中任何本地单集。
		sourceShow := n % livePodcasts
		item := models.EpisodeCollectionItem{
			CollectionID:      collections[collectionIndex].ID,
			Position:          position,
			ExternalEpisodeID: fmt.Sprintf("item-%05d", n),
			ExternalPodcastID: podcasts[sourceShow].XYZID,
			PodcastTitle:      podcasts[sourceShow].Title,
			PodcastAuthor:     "种子作者",
			PodcastCoverURL:   fmt.Sprintf("https://img.example.com/pc%05d.jpg", n),
			EpisodeTitle:      fmt.Sprintf("条目 %05d", n),
			Recommendation:    "等量级性能种子条目",
			Duration:          3600,
			ImageURL:          fmt.Sprintf("https://img.example.com/e%05d.jpg", n),
			EpisodeURL:        fmt.Sprintf("https://www.xiaoyuzhoufm.com/episode/item%05d", n),
			PayType:           "FREE",
		}
		switch n % 4 {
		case 0:
			// 条目引用：直接关联种子单集。
			episode := episodes[n*3]
			require.False(t, episode.DeletedAt.Valid, "种子引用单集不应软删除")
			episodeID := episode.ID
			item.EpisodeID = &episodeID
			item.ExternalPodcastID = podcasts[episode.PodcastID-1].XYZID
			item.PodcastTitle = podcasts[episode.PodcastID-1].Title
			adopted++
		case 1:
			// 外部身份映射：episode_external_refs 指向种子单集。
			episode := episodes[n*3+1]
			item.ExternalPodcastID = podcasts[episode.PodcastID-1].XYZID
			item.PodcastTitle = podcasts[episode.PodcastID-1].Title
			refs = append(refs, models.EpisodeExternalRef{
				SourcePlatform:    PlatformXiaoyuzhoufm,
				ExternalEpisodeID: item.ExternalEpisodeID,
				ExternalPodcastID: podcasts[episode.PodcastID-1].XYZID,
				EpisodeID:         episode.ID,
				PodcastID:         episode.PodcastID,
			})
			adopted++
		case 2:
			// 链接匹配：条目链接等于种子单集链接。
			episode := episodes[n*3+2]
			item.EpisodeURL = episode.Link
			item.ExternalPodcastID = podcasts[episode.PodcastID-1].XYZID
			item.PodcastTitle = podcasts[episode.PodcastID-1].Title
			adopted++
		default:
			if n%8 == 7 {
				// GUID + 节目约束匹配。
				episode := episodes[n*3+3]
				podcast := podcasts[episode.PodcastID-1]
				item.ExternalEpisodeID = episode.GUID
				item.ExternalPodcastID = podcast.XYZID
				item.PodcastTitle = podcast.Title
				item.EpisodeURL = fmt.Sprintf("https://www.xiaoyuzhoufm.com/episode/guiditem%05d", n)
				adopted++
			}
		}
		items = append(items, item)
	}
	require.NoError(t, db.CreateInBatches(&items, 200).Error)
	require.NoError(t, db.CreateInBatches(&refs, 200).Error)
	return adopted
}
