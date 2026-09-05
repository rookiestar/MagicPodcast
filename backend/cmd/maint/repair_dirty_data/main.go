package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"magicpodcast/internal/feed"
	"magicpodcast/internal/originallink"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mmcdole/gofeed"
)

const replacementChar = "\uFFFD"

type config struct {
	dbPath              string
	apply               bool
	includeMedia        bool
	sanitizeReplacement bool
	timeout             time.Duration
	concurrency         int
	podcastIDs          map[int64]bool
	reportPath          string
}

type podcastCandidate struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	FeedURL  string `json:"feed_url"`
	CoverURL string `json:"cover_url"`
}

type episodeCandidate struct {
	ID              int64  `json:"id"`
	PodcastID       int64  `json:"podcast_id"`
	GUID            string `json:"guid"`
	Title           string `json:"title"`
	MediumURL       string `json:"medium_url"`
	Link            string `json:"link"`
	DateKey         string `json:"date_key"`
	HasBadShowNotes bool   `json:"has_bad_show_notes"`
	HasBadContent   bool   `json:"has_bad_content"`
	EmptyShowNotes  bool   `json:"empty_show_notes"`
	EmptyLink       bool   `json:"empty_link"`
	EmptyImage      bool   `json:"empty_image"`
}

type fieldUpdate struct {
	Field string `json:"field"`
	Value string `json:"-"`
}

type episodeRepair struct {
	EpisodeID  int64         `json:"episode_id"`
	PodcastID  int64         `json:"podcast_id"`
	Title      string        `json:"title"`
	MatchBy    string        `json:"match_by"`
	Fields     []fieldUpdate `json:"fields"`
	Unresolved []string      `json:"unresolved,omitempty"`
}

type podcastResult struct {
	PodcastID       int64   `json:"podcast_id"`
	PodcastTitle    string  `json:"podcast_title"`
	Candidates      int     `json:"candidates"`
	Matched         int     `json:"matched"`
	Unmatched       int     `json:"unmatched"`
	Repairs         int     `json:"repairs"`
	Unresolved      int     `json:"unresolved"`
	FetchError      string  `json:"fetch_error,omitempty"`
	UnmatchedSample []int64 `json:"unmatched_sample,omitempty"`
}

type auditMetrics struct {
	EpisodesBadShowNotes      int64 `json:"episodes_bad_show_notes"`
	EpisodesBadContent        int64 `json:"episodes_bad_content"`
	EpisodesEmptyShowNotes    int64 `json:"episodes_empty_show_notes"`
	EpisodesEmptyLink         int64 `json:"episodes_empty_link"`
	EpisodesEmptyImage        int64 `json:"episodes_empty_image"`
	PodcastNewestDateMismatch int64 `json:"podcast_newest_date_mismatch"`
	PodcastZeroDateNoEpisodes int64 `json:"podcast_zero_date_no_episodes"`
	DuplicateEpisodeGUID      int64 `json:"duplicate_episode_guid_groups"`
	OrphanEpisodes            int64 `json:"orphan_episodes"`
}

type runReport struct {
	GeneratedAt       string            `json:"generated_at"`
	Apply             bool              `json:"apply"`
	IncludeMedia      bool              `json:"include_media"`
	Before            auditMetrics      `json:"before"`
	After             auditMetrics      `json:"after"`
	PodcastsScanned   int               `json:"podcasts_scanned"`
	PodcastsFailed    int               `json:"podcasts_failed"`
	EpisodesCandidate int               `json:"episodes_candidate"`
	EpisodesMatched   int               `json:"episodes_matched"`
	EpisodesUnmatched int               `json:"episodes_unmatched"`
	EpisodesRepaired  int               `json:"episodes_repaired"`
	FieldsPlanned     map[string]int    `json:"fields_planned"`
	FieldsApplied     map[string]int    `json:"fields_applied"`
	SanitizedFields   map[string]int    `json:"sanitized_fields"`
	ZeroDatesFixed    int               `json:"zero_dates_fixed"`
	PodcastResults    []podcastResult   `json:"podcast_results"`
	RepairSamples     []episodeRepair   `json:"repair_samples"`
	FetchErrors       map[string]string `json:"fetch_errors,omitempty"`
}

type feedItem struct {
	ID        string
	Title     string
	DateKeys  []string
	Link      string
	MediumURL string
	ShowNotes string
	Content   string
	ImageURL  string
}

type itemIndex struct {
	byID        map[string][]*feedItem
	byMediumURL map[string][]*feedItem
	byLink      map[string][]*feedItem
	byTitleDate map[string][]*feedItem
	byTitle     map[string][]*feedItem
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		exitf("参数错误: %v", err)
	}

	db, err := openDB(cfg.dbPath)
	if err != nil {
		exitf("打开数据库失败: %v", err)
	}
	defer db.Close()

	before, err := audit(db)
	if err != nil {
		exitf("体检数据库失败: %v", err)
	}
	printAudit("修复前", before)

	podcasts, episodesByPodcast, candidateCount, err := loadCandidates(db, cfg)
	if err != nil {
		exitf("读取候选数据失败: %v", err)
	}

	fmt.Printf("候选播客: %d, 候选单集: %d\n", len(podcasts), candidateCount)
	if !cfg.apply {
		fmt.Println("运行模式: dry-run（不会写数据库）")
	} else {
		fmt.Println("运行模式: apply（会写入可安全修复的字段）")
	}

	report := runReport{
		GeneratedAt:       time.Now().Format(time.RFC3339),
		Apply:             cfg.apply,
		IncludeMedia:      cfg.includeMedia,
		Before:            before,
		EpisodesCandidate: candidateCount,
		FieldsPlanned:     map[string]int{},
		FieldsApplied:     map[string]int{},
		SanitizedFields:   map[string]int{},
		FetchErrors:       map[string]string{},
	}

	repairs, podcastResults := planRepairs(cfg, podcasts, episodesByPodcast)
	report.PodcastResults = podcastResults
	report.PodcastsScanned = len(podcasts)

	for _, pr := range podcastResults {
		report.EpisodesMatched += pr.Matched
		report.EpisodesUnmatched += pr.Unmatched
		if pr.FetchError != "" {
			report.PodcastsFailed++
			report.FetchErrors[strconv.FormatInt(pr.PodcastID, 10)] = pr.FetchError
		}
	}

	for _, repair := range repairs {
		if len(repair.Fields) == 0 {
			continue
		}
		report.EpisodesRepaired++
		if len(report.RepairSamples) < 20 {
			report.RepairSamples = append(report.RepairSamples, repair)
		}
		for _, field := range repair.Fields {
			report.FieldsPlanned[field.Field]++
		}
	}

	if cfg.apply {
		applied, err := applyEpisodeRepairs(db, repairs)
		if err != nil {
			exitf("写入单集修复失败: %v", err)
		}
		report.FieldsApplied = applied

		if cfg.sanitizeReplacement {
			sanitized, err := sanitizeReplacementChars(db)
			if err != nil {
				exitf("移除替换字符失败: %v", err)
			}
			report.SanitizedFields = sanitized
		}

		fixed, err := fixZeroNewestDates(db)
		if err != nil {
			exitf("修复零值最新日期失败: %v", err)
		}
		report.ZeroDatesFixed = fixed
	} else {
		fixed, err := countZeroNewestDates(db)
		if err != nil {
			exitf("检查零值最新日期失败: %v", err)
		}
		report.ZeroDatesFixed = fixed
	}

	after, err := audit(db)
	if err != nil {
		exitf("复查数据库失败: %v", err)
	}
	report.After = after
	printSummary(report)
	printAudit("修复后", after)

	if cfg.reportPath != "" {
		if err := writeReport(cfg.reportPath, report); err != nil {
			exitf("写入报告失败: %v", err)
		}
		fmt.Printf("报告已写入: %s\n", cfg.reportPath)
	}
}

func parseFlags() (config, error) {
	var cfg config
	var timeout string
	var podcastIDs string

	flag.StringVar(&cfg.dbPath, "db", "data/magicpodcast.db", "SQLite database path")
	flag.BoolVar(&cfg.apply, "apply", false, "apply safe repairs to the database")
	flag.BoolVar(&cfg.includeMedia, "include-media", true, "also repair empty episode link/image fields when feed has a clean value")
	flag.BoolVar(&cfg.sanitizeReplacement, "sanitize-replacement", true, "remove U+FFFD replacement characters that cannot be restored from feed data")
	flag.StringVar(&timeout, "timeout", "20s", "per-feed fetch timeout")
	flag.IntVar(&cfg.concurrency, "concurrency", 4, "feed fetch concurrency")
	flag.StringVar(&podcastIDs, "podcast-id", "", "optional comma-separated podcast ids")
	flag.StringVar(&cfg.reportPath, "report", "", "optional JSON report path")
	flag.Parse()

	if cfg.concurrency < 1 {
		cfg.concurrency = 1
	}
	if cfg.concurrency > 12 {
		cfg.concurrency = 12
	}

	d, err := time.ParseDuration(timeout)
	if err != nil {
		return cfg, err
	}
	cfg.timeout = d

	if podcastIDs != "" {
		cfg.podcastIDs = map[int64]bool{}
		for _, part := range strings.Split(podcastIDs, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil {
				return cfg, fmt.Errorf("invalid podcast id %q: %w", part, err)
			}
			cfg.podcastIDs[id] = true
		}
	}

	if cfg.reportPath == "" {
		cfg.reportPath = filepath.Join("data", fmt.Sprintf("dirty_data_repair_%s.json", time.Now().Format("20060102_150405")))
	}

	return cfg, nil
}

func openDB(dbPath string) (*sql.DB, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_busy_timeout=10000&_journal_mode=WAL", dbPath))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec("PRAGMA busy_timeout=10000"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func audit(db *sql.DB) (auditMetrics, error) {
	var m auditMetrics
	queries := []struct {
		dest *int64
		sql  string
	}{
		{&m.EpisodesBadShowNotes, "select count(*) from episodes where deleted_at is null and instr(coalesce(show_notes,''), char(65533)) > 0"},
		{&m.EpisodesBadContent, "select count(*) from episodes where deleted_at is null and instr(coalesce(content,''), char(65533)) > 0"},
		{&m.EpisodesEmptyShowNotes, "select count(*) from episodes where deleted_at is null and trim(coalesce(show_notes,'')) = ''"},
		{&m.EpisodesEmptyLink, "select count(*) from episodes where deleted_at is null and trim(coalesce(link,'')) = ''"},
		{&m.EpisodesEmptyImage, "select count(*) from episodes where deleted_at is null and trim(coalesce(image_url,'')) = ''"},
		{&m.PodcastNewestDateMismatch, "with actual as (select podcast_id, max(published_date) newest from episodes where deleted_at is null group by podcast_id) select count(*) from podcasts p left join actual a on a.podcast_id=p.id where p.deleted_at is null and coalesce(substr(p.newest_episode_date,1,19),'') <> coalesce(substr(a.newest,1,19),'')"},
		{&m.PodcastZeroDateNoEpisodes, "select count(*) from podcasts p where p.deleted_at is null and coalesce(p.episode_count,0)=0 and coalesce(substr(p.newest_episode_date,1,10),'')='0001-01-01'"},
		{&m.DuplicateEpisodeGUID, "with dup as (select guid from episodes where deleted_at is null and trim(coalesce(guid,''))<>'' group by guid having count(*)>1) select count(*) from dup"},
		{&m.OrphanEpisodes, "select count(*) from episodes e left join podcasts p on p.id=e.podcast_id where e.deleted_at is null and p.id is null"},
	}

	for _, query := range queries {
		if err := db.QueryRow(query.sql).Scan(query.dest); err != nil {
			return m, err
		}
	}
	return m, nil
}

func loadCandidates(db *sql.DB, cfg config) ([]podcastCandidate, map[int64][]episodeCandidate, int, error) {
	condition := dirtyCondition(cfg.includeMedia)

	podcastSQL := fmt.Sprintf(`
select p.id, p.title, p.feed_url, coalesce(p.cover_url, '')
from podcasts p
where p.deleted_at is null
  and trim(coalesce(p.feed_url,'')) <> ''
  and exists (
    select 1
    from episodes e
    where e.deleted_at is null
      and e.podcast_id = p.id
      and (%s)
  )
order by p.id`, condition)

	rows, err := db.Query(podcastSQL)
	if err != nil {
		return nil, nil, 0, err
	}
	defer rows.Close()

	var podcasts []podcastCandidate
	for rows.Next() {
		var p podcastCandidate
		if err := rows.Scan(&p.ID, &p.Title, &p.FeedURL, &p.CoverURL); err != nil {
			return nil, nil, 0, err
		}
		if len(cfg.podcastIDs) > 0 && !cfg.podcastIDs[p.ID] {
			continue
		}
		podcasts = append(podcasts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, 0, err
	}

	podcastFilter := ""
	args := []any{}
	if len(cfg.podcastIDs) > 0 {
		ids := make([]string, 0, len(cfg.podcastIDs))
		for id := range cfg.podcastIDs {
			ids = append(ids, "?")
			args = append(args, id)
		}
		podcastFilter = " and e.podcast_id in (" + strings.Join(ids, ",") + ")"
	}

	episodeSQL := fmt.Sprintf(`
select e.id,
       e.podcast_id,
       coalesce(e.guid, ''),
       e.title,
       coalesce(e.medium_url, ''),
       coalesce(e.link, ''),
       coalesce(substr(e.published_date, 1, 10), ''),
       case when instr(coalesce(e.show_notes,''), char(65533)) > 0 then 1 else 0 end,
       case when instr(coalesce(e.content,''), char(65533)) > 0 then 1 else 0 end,
       case when trim(coalesce(e.show_notes,'')) = '' then 1 else 0 end,
       case when trim(coalesce(e.link,'')) = '' then 1 else 0 end,
       case when trim(coalesce(e.image_url,'')) = '' then 1 else 0 end
from episodes e
where e.deleted_at is null
  and (%s)
  %s
order by e.podcast_id, e.id`, condition, podcastFilter)

	rows, err = db.Query(episodeSQL, args...)
	if err != nil {
		return nil, nil, 0, err
	}
	defer rows.Close()

	episodesByPodcast := map[int64][]episodeCandidate{}
	total := 0
	for rows.Next() {
		var e episodeCandidate
		var badNotes, badContent, emptyNotes, emptyLink, emptyImage int
		if err := rows.Scan(
			&e.ID,
			&e.PodcastID,
			&e.GUID,
			&e.Title,
			&e.MediumURL,
			&e.Link,
			&e.DateKey,
			&badNotes,
			&badContent,
			&emptyNotes,
			&emptyLink,
			&emptyImage,
		); err != nil {
			return nil, nil, 0, err
		}
		e.HasBadShowNotes = badNotes == 1
		e.HasBadContent = badContent == 1
		e.EmptyShowNotes = emptyNotes == 1
		e.EmptyLink = emptyLink == 1
		e.EmptyImage = emptyImage == 1
		episodesByPodcast[e.PodcastID] = append(episodesByPodcast[e.PodcastID], e)
		total++
	}
	if err := rows.Err(); err != nil {
		return nil, nil, 0, err
	}

	return podcasts, episodesByPodcast, total, nil
}

func dirtyCondition(includeMedia bool) string {
	parts := []string{
		"instr(coalesce(e.show_notes,''), char(65533)) > 0",
		"instr(coalesce(e.content,''), char(65533)) > 0",
		"trim(coalesce(e.show_notes,'')) = ''",
	}
	if includeMedia {
		parts = append(parts,
			"trim(coalesce(e.link,'')) = ''",
			"trim(coalesce(e.image_url,'')) = ''",
		)
	}
	return strings.Join(parts, " or ")
}

func planRepairs(cfg config, podcasts []podcastCandidate, episodesByPodcast map[int64][]episodeCandidate) ([]episodeRepair, []podcastResult) {
	type job struct {
		podcast podcastCandidate
	}
	type outcome struct {
		repairs []episodeRepair
		result  podcastResult
	}

	jobs := make(chan job)
	outcomes := make(chan outcome, len(podcasts))

	var wg sync.WaitGroup
	for i := 0; i < cfg.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fetcher := feed.NewFetcher(cfg.timeout)
			for j := range jobs {
				repairs, result := planPodcastRepairs(cfg, fetcher, j.podcast, episodesByPodcast[j.podcast.ID])
				outcomes <- outcome{repairs: repairs, result: result}
			}
		}()
	}

	go func() {
		for _, p := range podcasts {
			jobs <- job{podcast: p}
		}
		close(jobs)
		wg.Wait()
		close(outcomes)
	}()

	var repairs []episodeRepair
	var results []podcastResult
	for outcome := range outcomes {
		repairs = append(repairs, outcome.repairs...)
		results = append(results, outcome.result)
	}

	sort.Slice(repairs, func(i, j int) bool {
		if repairs[i].PodcastID == repairs[j].PodcastID {
			return repairs[i].EpisodeID < repairs[j].EpisodeID
		}
		return repairs[i].PodcastID < repairs[j].PodcastID
	})
	sort.Slice(results, func(i, j int) bool {
		return results[i].PodcastID < results[j].PodcastID
	})

	return repairs, results
}

func planPodcastRepairs(cfg config, fetcher *feed.Fetcher, podcast podcastCandidate, candidates []episodeCandidate) ([]episodeRepair, podcastResult) {
	result := podcastResult{
		PodcastID:    podcast.ID,
		PodcastTitle: podcast.Title,
		Candidates:   len(candidates),
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	feedData, err := fetcher.FetchFeedWithContext(ctx, podcast.FeedURL)
	if err != nil {
		result.FetchError = err.Error()
		result.Unmatched = len(candidates)
		return nil, result
	}

	idx := buildItemIndex(podcast.ID, feedData)
	repairs := make([]episodeRepair, 0, len(candidates))

	for _, candidate := range candidates {
		item, matchBy := idx.match(candidate)
		if item == nil {
			result.Unmatched++
			if len(result.UnmatchedSample) < 10 {
				result.UnmatchedSample = append(result.UnmatchedSample, candidate.ID)
			}
			continue
		}

		result.Matched++
		repair := buildEpisodeRepair(candidate, item, matchBy, podcast.FeedURL, cfg.includeMedia)
		if len(repair.Fields) > 0 {
			result.Repairs++
			repairs = append(repairs, repair)
		}
		if len(repair.Unresolved) > 0 {
			result.Unresolved++
		}
	}

	return repairs, result
}

func buildItemIndex(podcastID int64, feedData *gofeed.Feed) itemIndex {
	idx := itemIndex{
		byID:        map[string][]*feedItem{},
		byMediumURL: map[string][]*feedItem{},
		byLink:      map[string][]*feedItem{},
		byTitleDate: map[string][]*feedItem{},
		byTitle:     map[string][]*feedItem{},
	}

	for _, item := range feedData.Items {
		fi := toFeedItem(podcastID, item)
		addIndex(idx.byID, fi.ID, fi)
		addIndex(idx.byMediumURL, fi.MediumURL, fi)
		addIndex(idx.byLink, fi.Link, fi)
		titleKey := normalizeKey(fi.Title)
		addIndex(idx.byTitle, titleKey, fi)
		for _, dateKey := range fi.DateKeys {
			addIndex(idx.byTitleDate, titleDateKey(titleKey, dateKey), fi)
		}
	}

	return idx
}

func toFeedItem(podcastID int64, item *gofeed.Item) *feedItem {
	fi := &feedItem{
		ID:        feedItemID(podcastID, item),
		Title:     item.Title,
		Link:      strings.TrimSpace(item.Link),
		ShowNotes: firstNonEmpty(item.Description, itunesSummary(item), item.Content),
		Content:   strings.TrimSpace(item.Content),
	}
	if len(item.Enclosures) > 0 {
		fi.MediumURL = strings.TrimSpace(item.Enclosures[0].URL)
	}
	if item.Image != nil {
		fi.ImageURL = strings.TrimSpace(item.Image.URL)
	}
	if fi.ImageURL == "" && item.ITunesExt != nil {
		fi.ImageURL = strings.TrimSpace(item.ITunesExt.Image)
	}
	if item.PublishedParsed != nil {
		fi.DateKeys = appendUnique(fi.DateKeys, item.PublishedParsed.Format("2006-01-02"))
		fi.DateKeys = appendUnique(fi.DateKeys, item.PublishedParsed.UTC().Format("2006-01-02"))
	}
	if item.UpdatedParsed != nil {
		fi.DateKeys = appendUnique(fi.DateKeys, item.UpdatedParsed.Format("2006-01-02"))
		fi.DateKeys = appendUnique(fi.DateKeys, item.UpdatedParsed.UTC().Format("2006-01-02"))
	}
	return fi
}

func feedItemID(podcastID int64, item *gofeed.Item) string {
	if strings.TrimSpace(item.GUID) != "" {
		return strings.TrimSpace(item.GUID)
	}
	if strings.TrimSpace(item.Link) != "" {
		return strings.TrimSpace(item.Link)
	}
	if len(item.Enclosures) > 0 && strings.TrimSpace(item.Enclosures[0].URL) != "" {
		return strings.TrimSpace(item.Enclosures[0].URL)
	}
	return generateHashID(fmt.Sprintf("%d-%s", podcastID, item.Title))
}

func (idx itemIndex) match(candidate episodeCandidate) (*feedItem, string) {
	if item := uniqueLookup(idx.byID, candidate.GUID); item != nil {
		return item, "guid"
	}
	if item := uniqueLookup(idx.byMediumURL, candidate.MediumURL); item != nil {
		return item, "medium_url"
	}
	if item := uniqueLookup(idx.byLink, candidate.Link); item != nil {
		return item, "link"
	}

	titleKey := normalizeKey(candidate.Title)
	if titleKey != "" && candidate.DateKey != "" {
		if item := uniqueLookup(idx.byTitleDate, titleDateKey(titleKey, candidate.DateKey)); item != nil {
			return item, "title_date"
		}
	}
	if item := uniqueLookup(idx.byTitle, titleKey); item != nil {
		return item, "title"
	}

	return nil, ""
}

func buildEpisodeRepair(candidate episodeCandidate, item *feedItem, matchBy, feedURL string, includeMedia bool) episodeRepair {
	repair := episodeRepair{
		EpisodeID: candidate.ID,
		PodcastID: candidate.PodcastID,
		Title:     candidate.Title,
		MatchBy:   matchBy,
	}

	if candidate.HasBadShowNotes || candidate.EmptyShowNotes {
		replacement := firstCleanText(item.ShowNotes, item.Content)
		if replacement != "" {
			repair.Fields = append(repair.Fields, fieldUpdate{Field: "show_notes", Value: replacement})
		} else if candidate.HasBadShowNotes {
			repair.Unresolved = append(repair.Unresolved, "show_notes_bad")
		} else {
			repair.Unresolved = append(repair.Unresolved, "show_notes_empty")
		}
	}

	if candidate.HasBadContent {
		replacement := firstCleanText(item.Content, item.ShowNotes)
		if replacement != "" {
			repair.Fields = append(repair.Fields, fieldUpdate{Field: "content", Value: replacement})
		} else {
			repair.Unresolved = append(repair.Unresolved, "content_bad")
		}
	}

	if includeMedia && candidate.EmptyLink {
		decision := originallink.Resolve(originallink.Input{
			Feed:         originallink.FeedIdentity{FeedURL: feedURL},
			RSSLink:      item.Link,
			ExistingLink: candidate.Link,
		})
		if decision.URL != "" {
			repair.Fields = append(repair.Fields, fieldUpdate{Field: "link", Value: decision.URL})
		}
	}

	if includeMedia && candidate.EmptyImage && validHTTPURL(item.ImageURL) {
		repair.Fields = append(repair.Fields, fieldUpdate{Field: "image_url", Value: item.ImageURL})
	}

	return repair
}

func sanitizeReplacementChars(db *sql.DB) (map[string]int, error) {
	sanitized := map[string]int{}
	for _, field := range []string{"show_notes", "content"} {
		result, err := db.Exec(
			fmt.Sprintf(
				"update episodes set %s = replace(%s, char(65533), ''), updated_at = ? where deleted_at is null and instr(coalesce(%s,''), char(65533)) > 0",
				field,
				field,
				field,
			),
			time.Now().Format(time.RFC3339Nano),
		)
		if err != nil {
			return nil, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if affected > 0 {
			sanitized[field] = int(affected)
		}
	}
	return sanitized, nil
}

func applyEpisodeRepairs(db *sql.DB, repairs []episodeRepair) (map[string]int, error) {
	applied := map[string]int{}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for _, repair := range repairs {
		if len(repair.Fields) == 0 {
			continue
		}

		setParts := make([]string, 0, len(repair.Fields)+2)
		args := make([]any, 0, len(repair.Fields)+2)
		seen := map[string]bool{}
		for _, field := range repair.Fields {
			if seen[field.Field] {
				continue
			}
			seen[field.Field] = true
			setParts = append(setParts, field.Field+" = ?")
			args = append(args, field.Value)
		}
		if len(setParts) == 0 {
			continue
		}

		now := time.Now().Format(time.RFC3339Nano)
		setParts = append(setParts, "updated_at = ?", "fetched_at = ?")
		args = append(args, now, now, repair.EpisodeID)

		stmt := fmt.Sprintf("update episodes set %s where id = ? and deleted_at is null", strings.Join(setParts, ", "))
		result, err := tx.Exec(stmt, args...)
		if err != nil {
			return nil, fmt.Errorf("episode %d: %w", repair.EpisodeID, err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if affected == 0 {
			return nil, fmt.Errorf("episode %d: no row updated", repair.EpisodeID)
		}
		for field := range seen {
			applied[field]++
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return applied, nil
}

func fixZeroNewestDates(db *sql.DB) (int, error) {
	result, err := db.Exec(`
update podcasts
set newest_episode_date = null,
    updated_at = ?
where deleted_at is null
  and coalesce(episode_count,0) = 0
  and coalesce(substr(newest_episode_date,1,10),'') = '0001-01-01'`, time.Now().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

func countZeroNewestDates(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(`
select count(*)
from podcasts
where deleted_at is null
  and coalesce(episode_count,0) = 0
  and coalesce(substr(newest_episode_date,1,10),'') = '0001-01-01'`).Scan(&count)
	return count, err
}

func printSummary(report runReport) {
	fmt.Println()
	fmt.Println("修复汇总")
	fmt.Printf("  扫描播客: %d\n", report.PodcastsScanned)
	fmt.Printf("  拉取失败播客: %d\n", report.PodcastsFailed)
	fmt.Printf("  候选单集: %d\n", report.EpisodesCandidate)
	fmt.Printf("  匹配成功单集: %d\n", report.EpisodesMatched)
	fmt.Printf("  匹配失败单集: %d\n", report.EpisodesUnmatched)
	fmt.Printf("  可修复单集: %d\n", report.EpisodesRepaired)
	fmt.Printf("  零值最新日期处理: %d\n", report.ZeroDatesFixed)
	fmt.Printf("  计划字段: %s\n", formatFieldCounts(report.FieldsPlanned))
	if report.Apply {
		fmt.Printf("  已写字段: %s\n", formatFieldCounts(report.FieldsApplied))
		fmt.Printf("  移除替换字符: %s\n", formatFieldCounts(report.SanitizedFields))
	}
}

func printAudit(label string, m auditMetrics) {
	fmt.Println()
	fmt.Println(label)
	fmt.Printf("  show_notes 含替换字符: %d\n", m.EpisodesBadShowNotes)
	fmt.Printf("  content 含替换字符: %d\n", m.EpisodesBadContent)
	fmt.Printf("  show_notes 为空: %d\n", m.EpisodesEmptyShowNotes)
	fmt.Printf("  link 为空: %d\n", m.EpisodesEmptyLink)
	fmt.Printf("  image_url 为空: %d\n", m.EpisodesEmptyImage)
	fmt.Printf("  播客最新日期不一致: %d\n", m.PodcastNewestDateMismatch)
	fmt.Printf("  零单集零值日期播客: %d\n", m.PodcastZeroDateNoEpisodes)
	fmt.Printf("  重复 GUID 分组: %d\n", m.DuplicateEpisodeGUID)
	fmt.Printf("  孤儿单集: %d\n", m.OrphanEpisodes)
}

func writeReport(path string, report runReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func formatFieldCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "无"
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}

func addIndex(index map[string][]*feedItem, key string, item *feedItem) {
	key = normalizeKey(key)
	if key == "" {
		return
	}
	index[key] = append(index[key], item)
}

func uniqueLookup(index map[string][]*feedItem, key string) *feedItem {
	key = normalizeKey(key)
	if key == "" {
		return nil
	}
	items := index[key]
	if len(items) != 1 {
		return nil
	}
	return items[0]
}

func titleDateKey(titleKey, dateKey string) string {
	if titleKey == "" || dateKey == "" {
		return ""
	}
	return titleKey + "\x00" + dateKey
}

func normalizeKey(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstCleanText(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || strings.Contains(value, replacementChar) {
			continue
		}
		return value
	}
	return ""
}

func itunesSummary(item *gofeed.Item) string {
	if item.ITunesExt == nil {
		return ""
	}
	return firstNonEmpty(item.ITunesExt.Summary, item.ITunesExt.Subtitle)
}

func validHTTPURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, replacementChar) {
		return false
	}
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func generateHashID(input string) string {
	hash := 5381
	for _, c := range input {
		hash = ((hash << 5) + hash) + int(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return fmt.Sprintf("gen_%d", hash)
}

func exitf(format string, args ...any) {
	err := fmt.Errorf(format, args...)
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "操作已取消")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
