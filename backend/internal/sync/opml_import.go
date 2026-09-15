package sync

import (
	"context"
	"fmt"
	"magicpodcast/internal/logger"
	"net/url"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/models"
	"magicpodcast/internal/opml"
	"magicpodcast/internal/podcastindex"

	"github.com/mmcdole/gofeed"
)

// 导入确认决策。只有预览/导入结果中明确标记为需确认的条目（清单关联、
// 换址合并、已删除恢复）才会读取对应决策；其余条目不受决策影响。
const (
	ImportDecisionConfirm = "confirm"
)

// ImportEntryResult 是单个订阅条目的导入业务结果。Outcome 与抓取状态彼此
// 独立：pending（待同步）不是失败，也不是成功刷新（#398 R12）。
type ImportEntryResult struct {
	Title     string `json:"title"`
	FeedURL   string `json:"feed_url"`
	Outcome   string `json:"outcome"`
	Detail    string `json:"detail,omitempty"`
	PodcastID uint   `json:"podcast_id,omitempty"`
	// Created 表示本次导入实际新建了该节目记录（#417/#418）。新建空壳
	// （pending）同样成立；更新、确认合并与恢复旧记录不算新建。旧任务缺
	// 少该字段时，仅 outcome=new 仍可作为确切的新建证据。
	Created bool `json:"created,omitempty"`
}

// importEntryResult 是核心流程内部使用的条目结果别名。
type importEntryResult = ImportEntryResult

// 导入条目结果分类。
const (
	ImportOutcomeNew       = "new"       // 新增节目
	ImportOutcomeUpdated   = "updated"   // 已有节目资料有变化，已更新
	ImportOutcomeUnchanged = "unchanged" // 已有节目且资料无变化
	ImportOutcomeMerged    = "merged"    // 确认后关联/换址合并到已有记录
	ImportOutcomePending   = "pending"   // 已保留订阅但 RSS 暂不可访问，待同步
	ImportOutcomeConflict  = "conflict"  // 身份冲突/需确认，未写入
	ImportOutcomeDeleted   = "deleted"   // 命中已删除记录，未静默恢复
	ImportOutcomeSkipped   = "skipped"   // 按来源分类跳过（付费/地域等）
	ImportOutcomeFailed    = "failed"    // 写入前拒绝或保存失败
)

// ImportOPML 导入OPML文件（使用默认配置）
func (s *Service) ImportOPML(filePath string) (*SyncResult, error) {
	return s.ImportOPMLWithProgressAndConfig(filePath, NewLogProgressReporter(), DefaultImportConfig)
}

// ImportOPMLWithProgress 导入OPML文件（带进度报告，使用默认并发配置）
func (s *Service) ImportOPMLWithProgress(filePath string, reporter ProgressReporter) (*SyncResult, error) {
	return s.ImportOPMLWithProgressAndConfig(filePath, reporter, DefaultImportConfig)
}

// ImportOPMLWithDecisions 携带用户确认决策导入 OPML 文件，主要用于
// 对上次结果中「需确认」条目的补充处理。
func (s *Service) ImportOPMLWithDecisions(filePath string, decisions map[string]string) (*SyncResult, error) {
	outlines, err := s.opmlParser.ParseFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OPML: %w", err)
	}
	return s.ImportOPMLOutlines(outlines, NewLogProgressReporter(), DefaultImportConfig, decisions)
}

// ImportOPMLWithProgressAndConfig 导入OPML文件（带进度报告和自定义并发配置）
func (s *Service) ImportOPMLWithProgressAndConfig(filePath string, reporter ProgressReporter, config ImportConfig) (*SyncResult, error) {
	outlines, err := s.opmlParser.ParseFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OPML: %w", err)
	}
	return s.ImportOPMLOutlines(outlines, reporter, config, nil)
}

// ImportOPMLFromPodcastIndexOnly 从PodcastIndex导入并在线同步元数据
func (s *Service) ImportOPMLFromPodcastIndexOnly(filePath string, reporter ProgressReporter) (*SyncResult, error) {
	outlines, err := s.opmlParser.ParseFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OPML: %w", err)
	}
	return s.ImportOPMLOutlinesFromPodcastIndexOnly(outlines, reporter)
}

// ImportOPMLOutlinesFromPodcastIndexOnly 接受预解析的订阅条目。SSE 入口在
// 发送响应头前完成解析，解析失败可以与普通入口返回同一 JSON 错误契约；
// 业务规则与普通入口完全一致（#398 R10/R11）。
func (s *Service) ImportOPMLOutlinesFromPodcastIndexOnly(outlines []opml.Outline, reporter ProgressReporter) (*SyncResult, error) {
	return s.ImportOPMLOutlines(outlines, reporter, DefaultImportConfig, nil)
}

// ImportOPMLOutlines 是普通与流式导入共用的唯一业务入口。decisions 是
// 用户在预览/上次结果中对需确认条目的显式决定（xmlUrl → confirm）。
func (s *Service) ImportOPMLOutlines(outlines []opml.Outline, reporter ProgressReporter, config ImportConfig, decisions map[string]string) (*SyncResult, error) {
	logger.Infof("开始导入OPML: %d 条订阅 (并发度: %d)", len(outlines), config.Concurrency)
	reporter.Report(fmt.Sprintf("开始导入OPML文件，共 %d 条订阅", len(outlines)))
	reporter.ReportSuccess(fmt.Sprintf("解析到 %d 个RSS feed", len(outlines)))
	if len(outlines) == 0 {
		reporter.Report("文件中没有订阅条目（0 条），未新增节目")
	}

	// 文件内重复地址先归并，保证重复确认/重复上传不创建多条（#401）。
	outlines = dedupeImportOutlines(outlines)

	result := &SyncResult{
		TotalPodcasts: len(outlines),
		Errors:        []string{},
		Entries:       make([]ImportEntryResult, 0, len(outlines)),
	}

	taskChan := make(chan opml.Outline, len(outlines))
	resultChan := make(chan *importEntryResult, len(outlines))

	var mu sync.Mutex
	processedCount := 0
	var persistErr error
	startTime := time.Now()

	concurrency := config.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultImportConfig.Concurrency
	}
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for outline := range taskChan {
				resultChan <- s.processImportOutline(&outline, decisions, reporter)
			}
		}(i)
	}

	for _, outline := range outlines {
		taskChan <- outline
	}
	close(taskChan)

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for res := range resultChan {
		mu.Lock()
		processedCount++
		result.Entries = append(result.Entries, *res)
		if recorder, ok := reporter.(interface {
			RecordImportResult(ImportEntryResult, int) error
		}); ok {
			if err := recorder.RecordImportResult(*res, processedCount); err != nil && persistErr == nil {
				persistErr = err
			}
		}
		reporter.ReportProgress(processedCount, len(outlines), fmt.Sprintf("正在处理: %s", res.Title))
		switch res.Outcome {
		case ImportOutcomeNew, ImportOutcomeUpdated, ImportOutcomeMerged:
			result.SuccessPodcasts++
			if res.Outcome == ImportOutcomeMerged {
				result.MergedPodcasts++
			}
			reporter.ReportSuccess(fmt.Sprintf("成功导入: %s", res.Title))
		case ImportOutcomeUnchanged:
			result.UnchangedPodcasts++
			reporter.Report(fmt.Sprintf("%s - 已在库中且资料无变化", res.Title))
		case ImportOutcomePending:
			result.StubPodcasts++
			reporter.Report(fmt.Sprintf("%s - 已保留订阅，待同步", res.Title))
		case ImportOutcomeConflict:
			result.ConflictPodcasts++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", res.Title, res.Detail))
			reporter.ReportSkip(SkipReasonDuplicate, fmt.Sprintf("%s - %s", res.Title, res.Detail))
		case ImportOutcomeDeleted:
			result.ConflictPodcasts++
			reporter.ReportSkip(SkipReasonDuplicate, fmt.Sprintf("%s - %s", res.Title, res.Detail))
		case ImportOutcomeSkipped:
			result.SkippedPodcasts++
			reporter.ReportSkip(SkipReasonOther, fmt.Sprintf("%s - %s", res.Title, res.Detail))
		default:
			result.FailedPodcasts++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", res.Title, res.Detail))
			reporter.ReportError(fmt.Sprintf("%s - %s", res.Title, res.Detail))
		}
		mu.Unlock()
	}

	duration := time.Since(startTime)
	logger.Infof("导入汇总 成功=%d 待同步=%d 冲突=%d 失败=%d 耗时=%v",
		result.SuccessPodcasts, result.StubPodcasts, result.ConflictPodcasts, result.FailedPodcasts, duration)

	reporter.ReportSummary(&SyncSummary{
		Operation: "import", TotalPodcasts: result.TotalPodcasts,
		SuccessPodcasts: result.SuccessPodcasts, FailedPodcasts: result.FailedPodcasts,
		SkippedPodcasts: result.SkippedPodcasts, StubPodcasts: result.StubPodcasts,
		MergedPodcasts: result.MergedPodcasts, ConflictPodcasts: result.ConflictPodcasts,
		UnchangedPodcasts: result.UnchangedPodcasts, Duration: duration,
	})

	return result, persistErr
}

// dedupeImportOutlines 按 URL 归并文件内重复条目，保留首个并统计重复数。
func dedupeImportOutlines(outlines []opml.Outline) []opml.Outline {
	if len(outlines) <= 1 {
		return outlines
	}
	seen := make(map[string]struct{}, len(outlines))
	unique := make([]opml.Outline, 0, len(outlines))
	for _, outline := range outlines {
		if _, exists := seen[outline.XMLURL]; exists {
			continue
		}
		seen[outline.XMLURL] = struct{}{}
		unique = append(unique, outline)
	}
	return unique
}

// processImportOutline 处理单个订阅条目：身份核对 → 抓取/索引匹配 →
// 身份冲突核对 → 补充式保存。它从不取消关注、不删除资料。
func (s *Service) processImportOutline(outline *opml.Outline, decisions map[string]string, reporter ProgressReporter) *importEntryResult {
	title := outline.GetTitle()
	res := &importEntryResult{Title: title, FeedURL: outline.XMLURL}

	if err := validateImportFeedURL(outline.XMLURL); err != nil {
		res.Outcome = ImportOutcomeFailed
		res.Detail = err.Error()
		return res
	}

	// 写入前本地身份核对（不抓取）。
	resolved := s.resolveImportIdentity(outline.XMLURL)
	decision := decisions[outline.XMLURL]

	switch resolved.kind {
	case identityDeleted:
		// 已软删除的记录不静默复活；仅显式确认后恢复关注。
		if decision != ImportDecisionConfirm {
			res.Outcome = ImportOutcomeDeleted
			res.PodcastID = resolved.podcast.ID
			res.Detail = "本地存在已删除的同地址记录，已跳过（可在预览中确认后重新关注）"
			return res
		}
	case identityByXyzID:
		// 清单收录记录：绑定/改绑订阅地址需要用户确认。
		if decision != ImportDecisionConfirm {
			res.Outcome = ImportOutcomeConflict
			res.PodcastID = resolved.podcast.ID
			res.Detail = "命中清单已收录的节目（外部 ID 一致），需确认后才会转为关注并绑定该订阅地址"
			return res
		}
	}

	// 同步节目资料：索引优先，未命中在线抓取；失败保留待同步空壳。
	podcast, stub, err := s.syncPodcastFromPodcastIndexOnly(outline, outline.XMLURL, reporter)
	if err != nil {
		res.Outcome = ImportOutcomeFailed
		res.Detail = err.Error()
		return res
	}
	// Other workers may have committed the same stable identity during fetching.
	s.importWriteMu.Lock()
	defer s.importWriteMu.Unlock()
	resolved = s.resolveImportIdentity(outline.XMLURL)
	if (resolved.kind == identityDeleted || resolved.kind == identityByXyzID) && decision != ImportDecisionConfirm {
		res.Outcome = ImportOutcomeConflict
		res.PodcastID = resolved.podcastID()
		res.Detail = "本地身份已变化，请重新预览并确认"
		return res
	}
	if stub {
		// 待同步同样执行确认过的身份绑定（转关注但保留待同步状态）。
		if resolved.podcast != nil && decision == ImportDecisionConfirm {
			podcast = pendingImportPodcastFrom(outline, outline.XMLURL, resolved.podcast)
		}
		if _, err := s.saveImportPodcast(podcast, resolved, importCreationRecorder(reporter, outline.XMLURL)); err != nil {
			res.Outcome = ImportOutcomeFailed
			res.Detail = fmt.Sprintf("save failed - %v", err)
			return res
		}
		res.Outcome = ImportOutcomePending
		res.PodcastID = podcast.ID
		// 本次保存前本地无同地址记录：这是一条新建的待同步空壳，而不是
		// 已有节目刷新失败（#417/#418 新建事实）。
		res.Created = resolved.podcast == nil
		res.Detail = "RSS 暂不可访问，已保留订阅待同步"
		return res
	}

	// 抓取成功后核对稳定身份冲突：换地址的节目在证据不足时不得自动合并。
	if candidates := s.findIdentityConflictCandidates(podcast, resolved.podcastID()); len(candidates) > 0 {
		if len(candidates) > 1 || resolved.podcast != nil {
			res.Outcome = ImportOutcomeConflict
			res.Detail = fmt.Sprintf("稳定身份匹配到 %d 条本地记录（多候选），已跳过；请在库中核对后处理", len(candidates))
			return res
		}
		candidate := candidates[0]
		if decision != ImportDecisionConfirm {
			res.Outcome = ImportOutcomeConflict
			res.PodcastID = candidate.PodcastID
			res.Detail = fmt.Sprintf("稳定身份与已有节目「%s」一致（证据：%s）但地址不同，需确认后合并",
				candidate.Title, candidate.Evidence)
			if candidate.Deleted {
				res.Detail += "；该记录已删除，确认将恢复关注"
			}
			return res
		}
		// 确认合并：复用本地记录与其单集/标签/备注，更新订阅地址。
		candidateKind := identityByFeedURL
		if candidate.Deleted {
			candidateKind = identityDeleted
		}
		if _, err := s.saveImportPodcast(podcast, resolvedPodcastIdentity{
			kind:    candidateKind,
			podcast: s.reloadPodcast(candidate.PodcastID),
		}); err != nil {
			res.Outcome = ImportOutcomeFailed
			res.Detail = fmt.Sprintf("save failed - %v", err)
			return res
		}
		res.Outcome = ImportOutcomeMerged
		res.PodcastID = candidate.PodcastID
		res.Detail = fmt.Sprintf("已确认关联到已有节目「%s」（证据：%s）", candidate.Title, candidate.Evidence)
		return res
	}

	changed, err := s.saveImportPodcast(podcast, resolved, importCreationRecorder(reporter, outline.XMLURL))
	if err != nil {
		res.Outcome = ImportOutcomeFailed
		res.Detail = fmt.Sprintf("save failed - %v", err)
		return res
	}
	res.PodcastID = podcast.ID
	switch {
	case resolved.kind == identityDeleted:
		// 已确认恢复：报告为合并语义，提示用户这是显式恢复。
		res.Outcome = ImportOutcomeMerged
		res.Detail = "已确认恢复本地已删除的记录并重新关注"
	case resolved.kind == identityByXyzID:
		// 已确认关联清单收录节目：复用本地记录并绑定订阅地址。
		res.Outcome = ImportOutcomeMerged
		res.Detail = "已确认关联清单收录节目并绑定订阅地址，已收录单集保留原 ID"
	case resolved.kind == identityNone:
		res.Outcome = ImportOutcomeNew
		res.Created = true
	case changed:
		res.Outcome = ImportOutcomeUpdated
		res.Detail = "已更新已有节目资料"
	default:
		res.Outcome = ImportOutcomeUnchanged
		res.Detail = "已在库中且资料无变化"
	}
	if resolved.kind == identityDeleted {
		// 已确认恢复：报告为合并语义，提示用户这是显式恢复。
		res.Outcome = ImportOutcomeMerged
		res.Detail = "已确认恢复本地已删除的记录并重新关注"
	}
	return res
}

// resolvedPodcastID 返回已解析记录 ID（可能为 0）。
func (r resolvedPodcastIdentity) podcastID() uint {
	if r.podcast == nil {
		return 0
	}
	return r.podcast.ID
}

func (s *Service) reloadPodcast(id uint) *models.Podcast {
	if id == 0 {
		return nil
	}
	var podcast models.Podcast
	if err := s.db.Unscoped().First(&podcast, id).Error; err != nil {
		return nil
	}
	return &podcast
}

func pendingImportPodcastFrom(outline *opml.Outline, feedURL string, existing *models.Podcast) *models.Podcast {
	podcast := pendingImportPodcast(outline, feedURL)
	if existing != nil {
		podcast.ID = existing.ID
		podcast.XYZID = existing.XYZID
		podcast.Notes = existing.Notes
		podcast.MyRate = existing.MyRate
		podcast.CustomCoverURL = existing.CustomCoverURL
	}
	return podcast
}

// syncPodcastFromFeed 从RSS feed同步播客信息
func (s *Service) syncPodcastFromFeed(feedURL string) (*models.Podcast, error) {
	return s.syncPodcastFromFeedWithRetry(nil, feedURL, NewLogProgressReporter())
}

// syncPodcastFromFeedWithRetry 从RSS feed同步播客信息（带重试）。重试行为完全由
// feed.RetryPolicy 提供：单一可重试分类、Retry-After 与有界 full-jitter 退避；每次
// 重试都经 Fetcher/Coordinator，断路、按域并发、去重与 fallback 语义不被旁路。
func (s *Service) syncPodcastFromFeedWithRetry(outline *opml.Outline, feedURL string, reporter ProgressReporter) (*models.Podcast, error) {
	title := ""
	if outline != nil {
		title = outline.GetTitle()
	}

	logPrefix := ""
	if title != "" {
		logPrefix = fmt.Sprintf("[%s]", title)
	}

	logger.Infof("%s 🔍 开始同步 feed: %s", logPrefix, feedURL)

	// 尝试从PodcastIndex查询（不重试，因为它是本地数据库）
	if s.podcastIndexQuery != nil {
		logger.Infof("%s 📚 尝试从 PodcastIndex 查询...", logPrefix)

		var piInfo *podcastindex.PodcastInfo
		var err error

		// 策略: 使用 feed_url 匹配（带 http/https 转换）
		logger.Infof("%s   📌 尝试使用 feed_url 匹配: %s", logPrefix, feedURL)

		// 1. 先尝试原始URL
		piInfo, err = s.podcastIndexQuery.FindByFeedURL(feedURL)
		if err != nil {
			logger.Infof("%s   ⚠️  feed_url 查询出错: %v", logPrefix, err)
		}

		// 2. 如果原始URL未匹配，尝试 http/https 互换
		if piInfo == nil && err == nil {
			var altURL string
			if strings.HasPrefix(feedURL, "http://") {
				altURL = strings.Replace(feedURL, "http://", "https://", 1)
				logger.Infof("%s   🔄 尝试 https 转换: %s", logPrefix, altURL)
			} else if strings.HasPrefix(feedURL, "https://") {
				altURL = strings.Replace(feedURL, "https://", "http://", 1)
				logger.Infof("%s   🔄 尝试 http 转换: %s", logPrefix, altURL)
			}

			if altURL != "" {
				piInfo, err = s.podcastIndexQuery.FindByFeedURL(altURL)
				if err != nil {
					logger.Infof("%s   ⚠️  转换URL查询出错: %v", logPrefix, err)
				}
			}
		}

		// 3. 检查匹配结果
		if piInfo != nil {
			logger.Infof("%s   ✅ feed_url 匹配成功: %s (作者: %s)", logPrefix, piInfo.Title, piInfo.Author)
			reporter.Report(fmt.Sprintf("%s - 从本地数据库快速获取（feed_url匹配）", title))
			podcast := s.createEnhancedPodcastFromOPML(piInfo, outline)
			// Index metadata alone cannot establish that the subscription was refreshed.
			return s.updatePodcastMetadataOnline(podcast, reporter)
		} else {
			logger.Infof("%s   📭 feed_url 未找到，准备在线抓取", logPrefix)
		}
	} else {
		logger.Infof("%s ⚠️  PodcastIndex 未初始化，直接在线抓取", logPrefix)
	}

	feedData, err := s.fetchFeedWithRetry(feedURL, title, reporter)
	if err != nil {
		return nil, err
	}
	podcast := s.convertGofeedToModel(feedData, "rss", feedURL)
	// A successful feed with no episodes or artwork is still a valid import.
	podcast.FeedURLValid = true
	return podcast, nil
}

// fetchFeedWithRetry is the single import retry path. It is shared by
// PodcastIndex matches and direct RSS imports so both paths honor the same
// classification, Retry-After, admission, and bounded backoff policy.
func (s *Service) fetchFeedWithRetry(feedURL, title string, reporter ProgressReporter) (*gofeed.Feed, error) {
	policy := s.retryPolicy
	logPrefix := ""
	if title != "" {
		logPrefix = fmt.Sprintf("[%s]", title)
	}
	logger.Infof("%s 🌐 开始在线抓取 RSS feed (最多重试 %d 次)", logPrefix, policy.Budget)

	var lastErr error
	for attempt := 0; attempt <= policy.Budget; attempt++ {
		if attempt > 0 {
			delay, _ := policy.NextDelay(lastErr, attempt-1)
			category := feed.CategoryOf(lastErr)
			retryAfter := feed.RetryAfterOf(lastErr)
			if title != "" {
				reporter.Report(fmt.Sprintf("%s - 第 %d 次重试中...", title, attempt))
			}
			logger.Infof("%s ⏳ 等待 %.0f 秒后重试 (category=%s retry_after=%q)...", logPrefix, delay.Seconds(), category, retryAfter)
			policy.Sleep(delay)
		}

		logger.Infof("%s 📡 正在抓取 (第 %d 次尝试)...", logPrefix, attempt+1)
		var release func()
		if attempt > 0 {
			var admitted bool
			release, admitted = policy.AcquireRetry(context.Background(), feed.TargetDomain(feedURL))
			if !admitted {
				logger.Warnf("%s ⛔ 重试准入等待被取消，停止本次重试: domain=%s", logPrefix, feed.TargetDomain(feedURL))
				break
			}
		}
		feedData, err := s.feedFetcher.FetchFeed(feedURL)
		if release != nil {
			release()
		}
		if err == nil {
			logger.Infof("%s ✅ 抓取成功: %s", logPrefix, feedData.Title)
			if attempt > 0 && title != "" {
				reporter.ReportSuccess(fmt.Sprintf("%s - 重试成功", title))
			}
			return feedData, nil
		}

		lastErr = err
		logger.Infof("%s ❌ 抓取失败 (第 %d 次尝试): %v", logPrefix, attempt+1, err)
		if !policy.ShouldRetry(err) {
			logger.Infof("%s ⛔ 不可重试的错误，停止重试: %v", logPrefix, err)
			return nil, err
		}
	}

	if title != "" {
		reporter.ReportError(fmt.Sprintf("%s - 重试 %d 次后仍然失败", title, policy.Budget))
	}
	logger.Infof("%s 💥 达到最大重试次数，放弃", logPrefix)
	return nil, fmt.Errorf("failed after %d retries: %w", policy.Budget, lastErr)
}

// syncPodcastFromPodcastIndexOnly 先本地匹配，再在线抓取。
// 第三个返回值表示这是一条待同步空壳：调用方不得计入成功。
func (s *Service) syncPodcastFromPodcastIndexOnly(outline *opml.Outline, feedURL string, reporter ProgressReporter) (*models.Podcast, bool, error) {
	if err := validateImportFeedURL(feedURL); err != nil {
		return nil, false, err
	}
	title := ""
	if outline != nil {
		title = outline.GetTitle()
	}

	logPrefix := ""
	if title != "" {
		logPrefix = fmt.Sprintf("[%s]", title)
	}

	logger.Infof("%s 🔍 智能同步模式: %s", logPrefix, feedURL)

	// 步骤1: 尝试从PodcastIndex本地数据库匹配
	var piInfo *podcastindex.PodcastInfo
	var err error

	if s.podcastIndexQuery != nil {
		// 1. 先尝试原始URL
		piInfo, err = s.podcastIndexQuery.FindByFeedURL(feedURL)
		if err != nil {
			logger.Infof("%s   ⚠️  feed_url 查询出错: %v", logPrefix, err)
		}

		// 2. 如果原始URL未匹配，尝试 http/https 互换
		if piInfo == nil && err == nil {
			var altURL string
			if strings.HasPrefix(feedURL, "http://") {
				altURL = strings.Replace(feedURL, "http://", "https://", 1)
				logger.Infof("%s   🔄 尝试 https 转换: %s", logPrefix, altURL)
			} else if strings.HasPrefix(feedURL, "https://") {
				altURL = strings.Replace(feedURL, "https://", "http://", 1)
				logger.Infof("%s   🔄 尝试 http 转换: %s", logPrefix, altURL)
			}

			if altURL != "" {
				piInfo, err = s.podcastIndexQuery.FindByFeedURL(altURL)
				if err != nil {
					logger.Infof("%s   ⚠️  转换URL查询出错: %v", logPrefix, err)
				}
			}
		}
	}

	// 步骤2: 根据匹配结果采取不同策略
	if piInfo != nil {
		// 情况A: 本地数据库匹配成功 - 在线抓取4个字段
		logger.Infof("%s   ✅ 本地数据库匹配成功: %s (作者: %s)", logPrefix, piInfo.Title, piInfo.Author)
		reporter.Report(fmt.Sprintf("%s - 本地匹配成功，正在更新元数据...", title))

		// 创建基础播客对象（从本地数据库）
		podcast := s.createEnhancedPodcastFromOPML(piInfo, outline)

		// 在线抓取4个关键字段
		logger.Infof("%s   🌐 在线更新元数据字段", logPrefix)
		updatedPodcast, updateErr := s.updatePodcastMetadataOnline(podcast, reporter)
		if updateErr != nil {
			reporter.Report(fmt.Sprintf("%s - RSS暂不可用，保留订阅待同步", title))
			return pendingImportPodcast(outline, feedURL), true, nil
		}

		logger.Infof("%s   ✅ 同步完成: %s", logPrefix, updatedPodcast.Title)
		reporter.Report(fmt.Sprintf("%s - 同步完成", title))
		return updatedPodcast, false, nil
	}

	// 情况B: 本地数据库未匹配 - 在线抓取完整信息
	logger.Infof("%s   📭 本地数据库未找到，尝试在线抓取...", logPrefix)
	reporter.Report(fmt.Sprintf("%s - 未在本地数据库找到，正在在线抓取...", title))

	podcast, fetchErr := s.fetchPodcastOnline(outline, feedURL, reporter)
	if fetchErr != nil {
		reporter.Report(fmt.Sprintf("%s - RSS暂不可用，保留订阅待同步", title))
		return pendingImportPodcast(outline, feedURL), true, nil
	}

	logger.Infof("%s   ✅ 在线抓取成功: %s", logPrefix, podcast.Title)
	reporter.Report(fmt.Sprintf("%s - 在线抓取成功", title))
	return podcast, false, nil
}

// updatePodcastMetadataOnline 在线更新播客的4个关键字段
func (s *Service) updatePodcastMetadataOnline(podcast *models.Podcast, reporter ProgressReporter) (*models.Podcast, error) {
	logger.Infof("   🌐 抓取元数据: %s", podcast.FeedURL)

	// 在线抓取RSS feed
	gofeed, err := s.fetchFeedWithRetry(podcast.FeedURL, podcast.Title, reporter)
	if err != nil {
		return nil, fmt.Errorf("抓取feed失败: %w", err)
	}

	// 提取4个关键字段
	updated := s.convertGofeedToModel(gofeed, podcast.DataSource, podcast.FeedURL)
	if updated.Title != "" {
		podcast.Title = updated.Title
	}
	if updated.Description != "" {
		podcast.Description = updated.Description
	}
	if updated.Author != "" {
		podcast.Author = updated.Author
	}
	if updated.CoverURL != "" {
		podcast.CoverURL = updated.CoverURL
	}
	if updated.Link != "" {
		podcast.Link = updated.Link
	}
	if updated.ITunesID != "" {
		podcast.ITunesID = updated.ITunesID
	}
	if updated.PodcastGUID != "" {
		podcast.PodcastGUID = updated.PodcastGUID
	}

	// 只更新这4个字段，保留其他字段
	podcast.EpisodeCount = updated.EpisodeCount
	podcast.NewestEpisodeDate = updated.NewestEpisodeDate
	podcast.NewestEnclosureURL = updated.NewestEnclosureURL
	podcast.NewestEnclosureDuration = updated.NewestEnclosureDuration

	now := time.Now()
	podcast.LastFetchedAt = &now
	podcast.FeedURLValid = true
	podcast.FetchErrorCount = 0

	logger.Infof("   ✅ 元数据更新成功: episode_count=%d, newest_episode_date=%v",
		podcast.EpisodeCount, podcast.NewestEpisodeDate)

	return podcast, nil
}

// fetchPodcastOnline 在线抓取完整播客信息。与索引命中路径共用
// fetchFeedWithRetry，保证两个导入入口在同一条件下使用相同重试预算
// （#398 R10）。
func (s *Service) fetchPodcastOnline(outline *opml.Outline, feedURL string, reporter ProgressReporter) (*models.Podcast, error) {
	logger.Infof("   🌐 在线抓取完整播客信息: %s", feedURL)

	title := ""
	if outline != nil {
		title = outline.GetTitle()
	}
	gofeed, err := s.fetchFeedWithRetry(feedURL, title, reporter)
	if err != nil {
		// 分类错误类型
		classifiedErr := feed.ClassifyError(feedURL, err)
		return nil, classifiedErr
	}

	// 转换为播客模型
	podcast := s.convertGofeedToModel(gofeed, "rss", feedURL)

	// 设置额外字段
	if outline != nil {
		if podcast.Title == "" {
			podcast.Title = outline.GetTitle()
		}
		podcast.AddedDate = time.Now()
	}

	podcast.IsSubscribed = true

	now := time.Now()
	podcast.LastFetchedAt = &now
	podcast.FeedURLValid = true
	podcast.FetchErrorCount = 0

	logger.Infof("   ✅ 在线抓取完成: %s (episode_count=%d)", podcast.Title, podcast.EpisodeCount)

	return podcast, nil
}

// OPML is user-supplied input: malformed/non-HTTP addresses are not subscriptions.
// Network reachability is deliberately NOT a condition for retaining a subscription.
func validateImportFeedURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return fmt.Errorf("无效的RSS地址：需要完整的HTTP或HTTPS链接")
	}
	return nil
}

func pendingImportPodcast(outline *opml.Outline, feedURL string) *models.Podcast {
	return &models.Podcast{
		Title: outline.GetTitle(), Description: outline.GetDescription(), Link: outline.HTMLURL,
		FeedURL: feedURL, IsSubscribed: true, FeedURLValid: false, DataSource: "rss", Priority: 5,
	}
}
