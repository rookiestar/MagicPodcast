package dataprofile

import (
	"fmt"
	"sort"
	"time"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

const fixtureInlinePNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func seedCompleteFixture(db *gorm.DB, scenario string, anchor time.Time) error {
	reference := anchor
	podcasts := fixturePodcasts(reference)
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&podcasts).Error; err != nil {
		return fmt.Errorf("create fixture podcasts: %w", err)
	}

	episodes := fixtureEpisodes(reference)
	if scenario == FixtureScenarioCompletionHistory {
		episodes = append(episodes, fixtureCompletionHistoryEpisodes(reference)...)
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&episodes).Error; err != nil {
		return fmt.Errorf("create fixture episodes: %w", err)
	}

	tags := fixtureTags(reference)
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&tags).Error; err != nil {
		return fmt.Errorf("create fixture tags: %w", err)
	}
	if err := db.Exec(`
		INSERT INTO podcasts_tags (podcast_id, tag_id) VALUES
			(1001, 3001), (1002, 3002), (1003, 3003);
		INSERT INTO episodes_tags (episode_id, tag_id) VALUES
			(2001, 3001), (2002, 3001), (2010, 3002), (2015, 3003)
	`).Error; err != nil {
		return fmt.Errorf("create fixture tag relations: %w", err)
	}

	decisions := fixtureDecisions(scenario, anchor)
	if scenario == FixtureScenarioCompletionHistory {
		decisions = append(decisions, fixtureCompletionHistoryDecisions(anchor)...)
	}
	if len(decisions) > 0 {
		if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&decisions).Error; err != nil {
			return fmt.Errorf("create fixture consumption states: %w", err)
		}
	}
	completions := fixtureCompletions(decisions)
	if scenario == FixtureScenarioCompletionHistory {
		completions = append(
			completions,
			fixtureCompletionHistoryFacts(anchor)...,
		)
	}
	if len(completions) > 0 {
		if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&completions).Error; err != nil {
			return fmt.Errorf("create fixture completion facts: %w", err)
		}
	}

	if scenario == FixtureScenarioReportEmpty {
		return nil
	}
	return seedFixtureReports(db, scenario, reference)
}

func fixtureCompletions(
	decisions []models.EpisodeTriageDecision,
) []models.EpisodeCompletion {
	completions := make([]models.EpisodeCompletion, 0)
	for _, decision := range decisions {
		if decision.QueueState == nil ||
			*decision.QueueState != models.QueueStateDone ||
			decision.QueueUpdatedAt == nil {
			continue
		}
		completedAt := *decision.QueueUpdatedAt
		completions = append(completions, models.EpisodeCompletion{
			EpisodeID:   decision.EpisodeID,
			CompletedAt: completedAt,
			CreatedAt:   completedAt,
			UpdatedAt:   completedAt,
		})
	}
	return completions
}

func fixturePodcasts(anchor time.Time) []models.Podcast {
	return []models.Podcast{
		{
			BaseModel:         models.BaseModel{ID: 1001, CreatedAt: anchor, UpdatedAt: anchor},
			XYZID:             "fixture-podcast-1001",
			Title:             "Fixture：深度科技",
			FeedURL:           "https://fixture.invalid/feeds/1001.xml",
			Description:       "用于 Discovery、报告与跨队列一致性验收的确定性科技播客。",
			Author:            "MagicPodcast Fixture",
			CoverURL:          fixtureInlinePNG,
			AddedDate:         anchor.AddDate(0, -6, 0),
			EpisodeCount:      8,
			NewestEpisodeDate: anchor.Add(-time.Hour),
			IsSubscribed:      true,
			FeedURLValid:      true,
			DataSource:        "fixture",
		},
		{
			BaseModel:         models.BaseModel{ID: 1002, CreatedAt: anchor, UpdatedAt: anchor},
			XYZID:             "fixture-podcast-1002",
			Title:             "Fixture：产品笔记",
			FeedURL:           "https://fixture.invalid/feeds/1002.xml",
			Description:       "缺少封面，包含长标题与异常富文本，供双端边界验收。",
			Author:            "MagicPodcast Fixture",
			AddedDate:         anchor.AddDate(0, -4, 0),
			EpisodeCount:      7,
			NewestEpisodeDate: anchor.Add(-3 * time.Hour),
			IsSubscribed:      true,
			FeedURLValid:      true,
			DataSource:        "fixture",
		},
		{
			BaseModel:         models.BaseModel{ID: 1003, CreatedAt: anchor, UpdatedAt: anchor},
			XYZID:             "fixture-podcast-1003",
			Title:             "Fixture：投资观察",
			FeedURL:           "https://fixture.invalid/feeds/1003.xml",
			Description:       "用于多报告、历史报告与富文本图片验收。",
			Author:            "MagicPodcast Fixture",
			CoverURL:          fixtureInlinePNG,
			AddedDate:         anchor.AddDate(0, -2, 0),
			EpisodeCount:      5,
			NewestEpisodeDate: anchor.Add(-2 * time.Hour),
			IsSubscribed:      true,
			FeedURLValid:      true,
			DataSource:        "fixture",
		},
	}
}

func fixtureEpisodes(anchor time.Time) []models.Episode {
	type episodeSeed struct {
		id        uint
		podcastID uint
		number    string
		title     string
		fetched   time.Time
		showNotes string
		link      string
		image     string
		duration  int
	}
	seeds := []episodeSeed{
		{2001, 1001, "101", "离线开发也能保持真实后端契约", anchor.Add(-time.Hour),
			`<p>安全链接：<a href="https://example.invalid/fixture/2001?from=show-notes">查看资料</a>。</p>`,
			"https://example.invalid/episodes/2001", "", 2580},
		{2002, 1001, "102", "同一单集在 Discovery、报告与 Inbox 中保持同一身份", anchor.Add(-2 * time.Hour),
			`<p>已读且已收集的 canonical episode。</p>`,
			"https://example.invalid/episodes/2002", "", 3120},
		{2003, 1001, "103", "Focus：混合格式 Show Notes", anchor.Add(-3 * time.Hour),
			`<p># AI 原生组织转型<br><br><br>**对齐事实**<br><br><br>---<br><br><br>- 建立共同语言<br><br><br>[延伸阅读](https://example.invalid/show-notes)<br><br><br>![转型示意图](/brand/magicpodcast-tuning-mark.png)</p>`,
			"https://example.invalid/episodes/2003", "", 1800},
		{2004, 1001, "104", "Focus：正在消费", anchor.Add(-24 * time.Hour),
			"具有显式 in-progress 状态。", "https://example.invalid/episodes/2004", "", 3660},
		{2005, 1001, "105", "Focus：已读但仍待完成", anchor.Add(-48 * time.Hour),
			"读取不会自动完成或移出队列。", "https://example.invalid/episodes/2005", "", 2220},
		{2006, 1001, "106", "Focus：接近陈旧提醒边界", anchor.Add(-4 * 24 * time.Hour),
			"队列动作距基准约 6 天。", "https://example.invalid/episodes/2006", "", 2940},
		{2007, 1001, "107", "Focus：已出现陈旧提醒", anchor.Add(-6 * 24 * time.Hour),
			"队列动作超过 7 天。", "https://example.invalid/episodes/2007", "", 3300},
		{2008, 1001, "108", "Focus：接近 30 天复盘边界", anchor.Add(-7 * 24 * time.Hour),
			"长期承诺仍由用户手动处理。", "https://example.invalid/episodes/2008", "", 2700},
		{2009, 1002, "201", "Someday：超过 30 天需要复盘", anchor.Add(-8 * 24 * time.Hour),
			"不会自动移动、归档或删除。", "https://example.invalid/episodes/2009", "", 2100},
		{2010, 1002, "202", "Inbox：下一条可投入 Focus 的内容", anchor.Add(-12 * time.Hour),
			"用于默认完整消费旅程。", "https://example.invalid/episodes/2010", "", 2460},
		{2011, 1002, "203", "Someday：陈旧但仍由用户保留", anchor.Add(-5 * 24 * time.Hour),
			"站内提示不创建任务或通知。", "https://example.invalid/episodes/2011", "", 1980},
		{2012, 1002, "204", "Done：已手动完成", anchor.Add(-3 * 24 * time.Hour),
			"Done 只来自显式用户动作。", "https://example.invalid/episodes/2012", "", 1560},
		{2013, 1002, "205", "14 天窗口内侧的多日期候选", anchor.Add(-14*24*time.Hour + time.Hour),
			"应出现在最近更新。", "https://www.xiaoyuzhoufm.com/episode/6a734c29ab3a91c24a1067fa?utm_source=rss", "", 1740},
		{2014, 1002, "206", "14 天窗口外侧的对照候选", anchor.Add(-14*24*time.Hour - time.Hour),
			"不应出现在最近更新。", "https://www.xiaoyuzhoufm.com/episode/6a8cf80a1352af56ff3b7e2d?utm_source=rss", "", 1860},
		{2015, 1003, "301", "报告中的安全链接与允许图片", anchor.Add(-90 * time.Minute),
			"精选报告和最近更新复用该 episode。", "https://example.invalid/episodes/2015", fixtureInlinePNG, 2880},
		{2016, 1002, "207",
			"这是一条用于验证移动端换行、中文 English Mixed Content 与极长标题不会造成页面横向溢出的 Fixture 单集",
			anchor.Add(-3 * time.Hour),
			`<p onclick="alert('fixture')">异常富文本 <script>alert("x")</script><img src="http://127.0.0.1/private.png"></p>`,
			"javascript:alert('unsafe')", "", 0},
		{2017, 1003, "302", "Focus 第 7 项边界", anchor.Add(-9 * 24 * time.Hour),
			"只在 focus-7 与 focus-over-limit 场景进入 Focus。", "https://example.invalid/episodes/2017", "", 2640},
		{2018, 1003, "303", "Focus 超过软上限的并发结果", anchor.Add(-10 * 24 * time.Hour),
			"只在 focus-over-limit 场景进入 Focus，系统保留数据并提示。", "https://example.invalid/episodes/2018", "", 2760},
		{2019, 1003, "304", "已删除单集不应进入精选报告", anchor.Add(-2 * time.Hour),
			"负向资格数据。", "https://example.invalid/episodes/2019", "", 1200},
		{2020, 1003, "305", "往期报告仍可按需读取完整正文", anchor.Add(-11 * 24 * time.Hour),
			"用于历史报告详情。", "https://example.invalid/episodes/2020", "", 3060},
	}

	episodes := make([]models.Episode, 0, len(seeds))
	for _, seed := range seeds {
		fetched := seed.fetched
		deletedAt := gorm.DeletedAt{}
		if seed.id == 2019 {
			deletedAt = gorm.DeletedAt{Time: anchor.Add(-30 * time.Minute), Valid: true}
		}
		videoAvailability := ""
		switch seed.id {
		case 2013:
			videoAvailability = models.VideoAvailabilityAvailable
		case 2014:
			videoAvailability = models.VideoAvailabilityUnavailable
		}
		episodes = append(episodes, models.Episode{
			BaseModel: models.BaseModel{
				ID:        seed.id,
				CreatedAt: seed.fetched,
				UpdatedAt: seed.fetched,
				DeletedAt: deletedAt,
			},
			PodcastID:         seed.podcastID,
			EpisodeNo:         seed.number,
			Title:             seed.title,
			ShowNotes:         seed.showNotes,
			PublishedDate:     seed.fetched.Add(-2 * time.Hour),
			Duration:          seed.duration,
			Link:              seed.link,
			ImageURL:          seed.image,
			GUID:              fmt.Sprintf("fixture-episode-%d", seed.id),
			FetchedAt:         &fetched,
			VideoAvailability: videoAvailability,
		})
	}
	return episodes
}

func fixtureCompletionHistoryEpisodes(anchor time.Time) []models.Episode {
	episodes := make([]models.Episode, 0, 55)
	for index := 0; index < 55; index++ {
		id := uint(2101 + index)
		podcastID := uint(1001 + index%3)
		title := fmt.Sprintf("Fixture 完成历史第 %02d 条", index+1)
		switch index {
		case 0:
			title = "Fixture 历史：不感兴趣后仍可重新处理"
		case 1:
			title = "Fixture 历史：唯一检索针"
		}
		fetchedAt := anchor.Add(-time.Duration(9*24+index) * time.Hour)
		episodes = append(episodes, models.Episode{
			BaseModel: models.BaseModel{
				ID:        id,
				CreatedAt: fetchedAt,
				UpdatedAt: fetchedAt,
			},
			PodcastID:     podcastID,
			EpisodeNo:     fmt.Sprintf("H%02d", index+1),
			Title:         title,
			ShowNotes:     "用于完成历史搜索、分页、状态与重新处理验收。",
			PublishedDate: fetchedAt.Add(-2 * time.Hour),
			Duration:      1800 + index*15,
			Link:          fmt.Sprintf("https://example.invalid/episodes/%d", id),
			GUID:          fmt.Sprintf("fixture-history-episode-%d", id),
			FetchedAt:     &fetchedAt,
		})
	}
	return episodes
}

func fixtureCompletionHistoryDecisions(anchor time.Time) []models.EpisodeTriageDecision {
	dismissedAt := anchor.Add(-4 * time.Hour)
	return []models.EpisodeTriageDecision{
		{
			BaseModel: models.BaseModel{
				ID:        7901,
				CreatedAt: dismissedAt,
				UpdatedAt: dismissedAt,
			},
			EpisodeID:   2101,
			State:       models.TriageStateDiscarded,
			DecidedAt:   dismissedAt,
			DismissedAt: &dismissedAt,
		},
	}
}

func fixtureCompletionHistoryFacts(anchor time.Time) []models.EpisodeCompletion {
	completions := []models.EpisodeCompletion{
		fixtureCompletionFact(2002, anchor.Add(-8*24*time.Hour)),
		fixtureCompletionFact(2003, anchor.Add(-9*24*time.Hour)),
		fixtureCompletionFact(2009, anchor.Add(-10*24*time.Hour)),
	}
	for index := 0; index < 55; index++ {
		completions = append(
			completions,
			fixtureCompletionFact(
				uint(2101+index),
				anchor.Add(-time.Duration(11*24+index)*time.Hour),
			),
		)
	}
	return completions
}

func fixtureCompletionFact(
	episodeID uint,
	completedAt time.Time,
) models.EpisodeCompletion {
	return models.EpisodeCompletion{
		EpisodeID:   episodeID,
		CompletedAt: completedAt,
		CreatedAt:   completedAt,
		UpdatedAt:   completedAt,
	}
}

func fixtureTags(anchor time.Time) []models.Tag {
	return []models.Tag{
		{ID: 3001, Name: "Fixture 科技", Color: "#5B6B8C", CreatedAt: anchor, UpdatedAt: anchor},
		{ID: 3002, Name: "Fixture 产品", Color: "#8A6F55", CreatedAt: anchor, UpdatedAt: anchor},
		{ID: 3003, Name: "Fixture 投资", Color: "#A05A3B", CreatedAt: anchor, UpdatedAt: anchor},
	}
}

func fixtureDecisions(scenario string, anchor time.Time) []models.EpisodeTriageDecision {
	type decisionSeed struct {
		episodeID   uint
		queue       string
		actionAge   time.Duration
		readAge     time.Duration
		progressAge time.Duration
	}
	seeds := []decisionSeed{
		{2002, models.QueueStateInbox, 2 * time.Hour, time.Hour, 0},
		{2010, models.QueueStateInbox, 12 * time.Hour, 0, 0},
		{2009, models.QueueStateSomeday, 30 * 24 * time.Hour, 0, 0},
		{2011, models.QueueStateSomeday, 7 * 24 * time.Hour, 0, 0},
		{2012, models.QueueStateDone, 3 * 24 * time.Hour, 2 * 24 * time.Hour, 0},
	}
	if scenario != FixtureScenarioFocusZero {
		seeds = append(seeds,
			decisionSeed{2003, models.QueueStateFocus, time.Hour, 0, 0},
			decisionSeed{2004, models.QueueStateFocus, 2 * 24 * time.Hour, 0, 24 * time.Hour},
			decisionSeed{2005, models.QueueStateFocus, 5 * 24 * time.Hour, 4 * 24 * time.Hour, 0},
			decisionSeed{2006, models.QueueStateFocus, 6 * 24 * time.Hour, 0, 0},
			decisionSeed{2007, models.QueueStateFocus, 7 * 24 * time.Hour, 0, 0},
			decisionSeed{2008, models.QueueStateFocus, 29 * 24 * time.Hour, 0, 0},
		)
	}
	if scenario == FixtureScenarioFocusSeven || scenario == FixtureScenarioFocusOverLimit {
		seeds = append(seeds, decisionSeed{2017, models.QueueStateFocus, 7 * 24 * time.Hour, 0, 0})
	}
	if scenario == FixtureScenarioFocusOverLimit {
		seeds = append(seeds, decisionSeed{2018, models.QueueStateFocus, 30 * 24 * time.Hour, 0, 0})
	}

	decisions := make([]models.EpisodeTriageDecision, 0, len(seeds))
	for index, seed := range seeds {
		actionAt := anchor.Add(-seed.actionAge)
		queue := seed.queue
		var readAt *time.Time
		if seed.readAge > 0 {
			value := anchor.Add(-seed.readAge)
			readAt = &value
		}
		var progressAt *time.Time
		if seed.progressAge > 0 {
			value := anchor.Add(-seed.progressAge)
			progressAt = &value
		}
		decisions = append(decisions, models.EpisodeTriageDecision{
			BaseModel: models.BaseModel{
				ID:        uint(7001 + index),
				CreatedAt: actionAt,
				UpdatedAt: actionAt,
			},
			EpisodeID:      seed.episodeID,
			State:          models.TriageStateShortlisted,
			DecidedAt:      actionAt,
			QueueState:     &queue,
			QueueUpdatedAt: &actionAt,
			InProgressAt:   progressAt,
			ReadAt:         readAt,
		})
	}

	byQueue := make(map[string][]int)
	for index := range decisions {
		if decisions[index].QueueState != nil {
			byQueue[*decisions[index].QueueState] = append(byQueue[*decisions[index].QueueState], index)
		}
	}
	for _, indexes := range byQueue {
		sort.Slice(indexes, func(left, right int) bool {
			leftDecision := decisions[indexes[left]]
			rightDecision := decisions[indexes[right]]
			if !leftDecision.QueueUpdatedAt.Equal(*rightDecision.QueueUpdatedAt) {
				return leftDecision.QueueUpdatedAt.After(*rightDecision.QueueUpdatedAt)
			}
			return leftDecision.EpisodeID > rightDecision.EpisodeID
		})
		for position, index := range indexes {
			queuePosition := int64(position)
			decisions[index].QueuePosition = &queuePosition
		}
	}
	return decisions
}

func seedFixtureReports(db *gorm.DB, scenario string, anchor time.Time) error {
	workflows := []models.Workflow{
		{
			BaseModel: models.BaseModel{ID: 4001, CreatedAt: anchor, UpdatedAt: anchor},
			Name:      "Fixture 科技日报", Description: "确定性科技精选报告",
			Schedule: "0 8 * * *", ScopeType: models.ScopeTypeAllSubscribed,
			IsEnabled: true, PublishToHomepage: true, ReportType: string(models.HomepageReportTypeDaily),
		},
		{
			BaseModel: models.BaseModel{ID: 4002, CreatedAt: anchor, UpdatedAt: anchor},
			Name:      "Fixture 投资日报", Description: "确定性投资精选报告",
			Schedule: "30 7 * * *", ScopeType: models.ScopeTypeAllSubscribed,
			IsEnabled: true, PublishToHomepage: true, ReportType: string(models.HomepageReportTypeDaily),
		},
		{
			BaseModel: models.BaseModel{ID: 4003, CreatedAt: anchor, UpdatedAt: anchor},
			Name:      "Fixture 往期周报", Description: "历史报告按需加载",
			Schedule: "0 9 * * 1", ScopeType: models.ScopeTypeAllSubscribed,
			IsEnabled: true, PublishToHomepage: true, ReportType: string(models.HomepageReportTypeWeekly),
		},
		{
			BaseModel: models.BaseModel{ID: 4004, CreatedAt: anchor, UpdatedAt: anchor},
			Name:      "Fixture 无效报告", Description: "只含不存在或已删除单集",
			Schedule: "0 6 * * *", ScopeType: models.ScopeTypeAllSubscribed,
			IsEnabled: true, PublishToHomepage: true, ReportType: string(models.HomepageReportTypeDaily),
		},
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&workflows).Error; err != nil {
		return fmt.Errorf("create fixture workflows: %w", err)
	}

	completed := []time.Time{
		anchor,
		anchor,
		anchor.Add(-24 * time.Hour),
		anchor,
	}
	jobs := make([]models.Job, 0, 4)
	for index, completedAt := range completed {
		startedAt := completedAt.Add(-5 * time.Minute)
		jobs = append(jobs, models.Job{
			BaseModel: models.BaseModel{
				ID:        uint(5001 + index),
				CreatedAt: startedAt,
				UpdatedAt: completedAt,
			},
			WorkflowID:  uint(4001 + index),
			Status:      models.JobStatusCompleted,
			StartTime:   &startedAt,
			EndTime:     &completedAt,
			TriggeredBy: "fixture",
		})
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&jobs).Error; err != nil {
		return fmt.Errorf("create fixture jobs: %w", err)
	}

	reportContent := "# Fixture 科技日报\n\n" +
		"默认场景通过真实后端提供完整正文、结构化单集和安全链接。\n\n" +
		"[安全报告链接](https://example.invalid/reports/fixture?source=dataset)\n\n" +
		"![允许的内嵌图片](" + fixtureInlinePNG + ")\n\n" +
		"<script>alert('raw html must not execute')</script>\n\n" +
		"[危险链接](javascript:alert('blocked'))"
	reports := []models.Report{
		{
			BaseModel: models.BaseModel{ID: 6001, CreatedAt: completed[0], UpdatedAt: completed[0]},
			JobID:     5001, Title: "Fixture 科技日报", Content: reportContent,
			Summary:       "真实后端、同源 API 与 canonical episode 的完整验收。",
			EpisodesCount: 2, PodcastsCount: 1, MatchedCount: 2,
			TimeRangeStart: anchor.Add(-24 * time.Hour), TimeRangeEnd: anchor,
			TimeRangeMode: "daily", GeneratedAt: completed[0], Format: "markdown",
			PublishToHomepage: true, ReportType: string(models.HomepageReportTypeDaily),
			WorkflowName: "Fixture 科技日报",
			StructuredEpisodes: models.ReportEpisodeList{
				{
					EpisodeID: 2001, Order: 1, PodcastID: 1001,
					PodcastTitle: "Fixture：深度科技", PodcastCoverURL: fixtureInlinePNG,
					EpisodeTitle: "离线开发也能保持真实后端契约", EpisodeNo: "101",
					Duration: 2580, PublishedDate: anchor.Add(-3 * time.Hour).Format(time.RFC3339),
					Link:           "https://example.invalid/episodes/2001",
					Recommendation: "用于验证未读、未收集的报告单集能够进入 Inbox。",
					Context:        "安全链接与完整 Show Notes 上下文。",
				},
				{
					EpisodeID: 2002, Order: 2, PodcastID: 1001,
					PodcastTitle: "Fixture：深度科技", EpisodeTitle: "同一单集在 Discovery、报告与 Inbox 中保持同一身份",
					EpisodeNo: "102", Duration: 3120,
					Link:           "javascript:alert('blocked')",
					Recommendation: "用于验证跨来源队列状态一致且危险协议不可点击。",
					Context:        "已读且已收集。",
				},
			},
			LLMSummary: "跨来源身份与离线开发",
		},
		{
			BaseModel: models.BaseModel{ID: 6002, CreatedAt: completed[1], UpdatedAt: completed[1]},
			JobID:     5002, Title: "Fixture 投资日报",
			Content:       "# Fixture 投资日报\n\n长标题、缺图和允许图片的双端报告。",
			Summary:       "用于多报告切换与移动横滑。",
			EpisodesCount: 2, PodcastsCount: 2, MatchedCount: 2,
			TimeRangeStart: anchor.Add(-24 * time.Hour), TimeRangeEnd: anchor,
			TimeRangeMode: "daily", GeneratedAt: completed[1], Format: "markdown",
			PublishToHomepage: true, ReportType: string(models.HomepageReportTypeDaily),
			WorkflowName: "Fixture 投资日报",
			StructuredEpisodes: models.ReportEpisodeList{
				{
					EpisodeID: 2015, Order: 1, PodcastID: 1003,
					PodcastTitle: "Fixture：投资观察", EpisodeTitle: "报告中的安全链接与允许图片",
					EpisodeNo: "301", Duration: 2880, ImageURL: fixtureInlinePNG,
					Link:           "https://example.invalid/episodes/2015",
					Recommendation: "验证报告图像和链接在安全规则下正常渲染。",
				},
				{
					EpisodeID: 2016, Order: 2, PodcastID: 1002,
					PodcastTitle: "Fixture：产品笔记",
					EpisodeTitle: "这是一条用于验证移动端换行、中文 English Mixed Content 与极长标题不会造成页面横向溢出的 Fixture 单集",
					EpisodeNo:    "207", Recommendation: "验证缺图与长文本边界。",
					Context: "异常富文本只能以安全方式展示。",
				},
			},
			LLMSummary: "长内容、图片与移动端边界",
		},
		{
			BaseModel: models.BaseModel{ID: 6003, CreatedAt: completed[2], UpdatedAt: completed[2]},
			JobID:     5003, Title: "Fixture 往期周报",
			Content:       "# Fixture 往期周报\n\n完整正文只在选择往期报告后按需读取。",
			Summary:       "元数据先加载，正文按需读取。",
			EpisodesCount: 1, PodcastsCount: 1, MatchedCount: 1,
			TimeRangeStart: anchor.Add(-7 * 24 * time.Hour), TimeRangeEnd: anchor.Add(-24 * time.Hour),
			TimeRangeMode: "daily", GeneratedAt: completed[2], Format: "markdown",
			PublishToHomepage: true, ReportType: string(models.HomepageReportTypeWeekly),
			WorkflowName: "Fixture 往期周报",
			StructuredEpisodes: models.ReportEpisodeList{
				{
					EpisodeID: 2020, Order: 1, PodcastID: 1003,
					PodcastTitle: "Fixture：投资观察", EpisodeTitle: "往期报告仍可按需读取完整正文",
					EpisodeNo: "305", Recommendation: "验证往期详情按需加载。",
				},
			},
			LLMSummary: "往期正文按需读取",
		},
		{
			BaseModel: models.BaseModel{ID: 6004, CreatedAt: completed[3], UpdatedAt: completed[3]},
			JobID:     5004, Title: "Fixture 无效报告",
			Content:       "# Fixture 无效报告\n\n该报告不会进入 Discovery。",
			Summary:       "只含不存在或已删除单集。",
			EpisodesCount: 2, PodcastsCount: 1,
			TimeRangeStart: anchor.Add(-24 * time.Hour), TimeRangeEnd: anchor,
			TimeRangeMode: "daily", GeneratedAt: completed[3], Format: "markdown",
			PublishToHomepage: true, ReportType: string(models.HomepageReportTypeDaily),
			WorkflowName: "Fixture 无效报告",
			StructuredEpisodes: models.ReportEpisodeList{
				{EpisodeID: 999999, Order: 1, EpisodeTitle: "不存在的单集"},
				{EpisodeID: 2019, Order: 2, PodcastID: 1003, EpisodeTitle: "已删除单集"},
			},
		},
	}
	if scenario == FixtureScenarioReportSingle {
		reports = []models.Report{reports[0], reports[2], reports[3]}
	}
	if err := db.Session(&gorm.Session{SkipHooks: true}).Create(&reports).Error; err != nil {
		return fmt.Errorf("create fixture reports: %w", err)
	}
	return nil
}
