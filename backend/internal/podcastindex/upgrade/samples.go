package upgrade

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func ExportFailedSamples(path, feedPrefix string) ([]FailedSample, error) {
	return ExportFailedSamplesSince(path, feedPrefix, DefaultFailureSince)
}

func ExportFailedSamplesSince(path, feedPrefix, since string) ([]FailedSample, error) {
	if feedPrefix == "" {
		feedPrefix = "https://feed.xyzfm.space/"
	}
	if since == "" {
		since = DefaultFailureSince
	}
	db, err := OpenSQLite(path, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		WITH ranked_failures AS (
			SELECT podcast_id, podcast_title, podcast_feed_url,
			       ROW_NUMBER() OVER (PARTITION BY podcast_id ORDER BY created_at DESC, id DESC) AS row_num
			FROM job_executions
			WHERE deleted_at IS NULL
			  AND status = 'failed'
			  AND created_at >= ?
			  AND podcast_feed_url LIKE ?
		)
		SELECT f.podcast_id,
		       COALESCE(p.title, f.podcast_title),
		       p.author,
		       f.podcast_feed_url,
		       p.i_tunes_id,
		       p.podcast_guid
		FROM ranked_failures f
		LEFT JOIN podcasts p ON p.id = f.podcast_id
		WHERE f.row_num = 1
		ORDER BY f.podcast_id`, since, feedPrefix+"%")
	if err != nil {
		return nil, fmt.Errorf("export failed PodcastIndex samples: %w", err)
	}
	defer rows.Close()
	var samples []FailedSample
	for rows.Next() {
		var sample FailedSample
		var author, feedURL, itunesID, guid sql.NullString
		if err := rows.Scan(&sample.ID, &sample.Title, &author, &feedURL, &itunesID, &guid); err != nil {
			return nil, fmt.Errorf("scan failed sample: %w", err)
		}
		sample.Author = author.String
		sample.FeedURL = feedURL.String
		sample.ITunesID = itunesID.String
		sample.PodcastGUID = guid.String
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read failed samples: %w", err)
	}
	return samples, nil
}

func WriteSamples(path string, samples []FailedSample) error {
	return WriteJSONAtomic(path, samples, 0o600)
}

func ReadSamples(path string) ([]FailedSample, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open sample file: %w", err)
	}
	defer file.Close()
	var samples []FailedSample
	if err := json.NewDecoder(file).Decode(&samples); err != nil {
		return nil, fmt.Errorf("decode sample file: %w", err)
	}
	return samples, nil
}

type CompareOptions struct {
	FeedPrefix           string
	CheckAccessibility   bool
	AccessibilityClient  *http.Client
	AccessibilityTimeout time.Duration
	MaxConcurrency       int
}

func CompareFailedSamples(ctx context.Context, candidatePath string, samples []FailedSample, options CompareOptions) (SampleComparison, error) {
	if options.FeedPrefix == "" {
		options.FeedPrefix = "https://feed.xyzfm.space/"
	}
	if options.MaxConcurrency <= 0 {
		options.MaxConcurrency = 4
	}
	if options.AccessibilityTimeout <= 0 {
		options.AccessibilityTimeout = 10 * time.Second
	}
	comparison := SampleComparison{
		ExpectedSamples:      len(samples),
		ActualSamples:        len(samples),
		AccessibilityChecked: options.CheckAccessibility,
	}
	db, err := OpenSQLite(candidatePath, true)
	if err != nil {
		comparison.Error = err.Error()
		return comparison, err
	}
	defer db.Close()

	bulkMatches, err := findSampleMatchesBulk(db, samples, options.FeedPrefix)
	if err != nil {
		comparison.Error = err.Error()
		return comparison, err
	}
	for _, sample := range samples {
		match, ok := bulkMatches[sample.ID]
		if !ok {
			match = CandidateMatch{
				SampleID:       sample.ID,
				SampleTitle:    sample.Title,
				CurrentFeedURL: sample.FeedURL,
			}
		}
		comparison.Matches = append(comparison.Matches, match)
		if match.CandidateID == 0 {
			comparison.NoMatch++
			continue
		}
		comparison.Matched++
		if match.IdentityConfirmed {
			comparison.IdentityConfirmed++
		}
		if match.TitleOnly {
			comparison.TitleOnly++
		}
	}
	if !options.CheckAccessibility {
		return comparison, nil
	}
	checkAccessibility(ctx, &comparison, options)
	return comparison, nil
}

func findSampleMatchesBulk(db *sql.DB, samples []FailedSample, feedPrefix string) (map[int64]CandidateMatch, error) {
	if len(samples) == 0 {
		return map[int64]CandidateMatch{}, nil
	}
	valueParts := make([]string, 0, len(samples))
	args := make([]any, 0, len(samples)*6+1)
	for _, sample := range samples {
		valueParts = append(valueParts, "(?, ?, ?, ?, ?, ?)")
		args = append(args, sample.ID, sample.Title, sample.Author, sample.FeedURL, normalizeITunesID(sample.ITunesID), strings.ToLower(strings.TrimSpace(sample.PodcastGUID)))
	}
	args = append(args, feedPrefix+"%")
	var matchBranches []string
	var methodCases []string
	hasITunesID, hasGUID, hasAuthor := false, false, false
	for _, sample := range samples {
		hasITunesID = hasITunesID || normalizeITunesID(sample.ITunesID) != ""
		hasGUID = hasGUID || strings.TrimSpace(sample.PodcastGUID) != ""
		hasAuthor = hasAuthor || strings.TrimSpace(sample.Author) != ""
	}
	if hasITunesID {
		matchBranches = append(matchBranches, "(s.sample_itunes_id <> '' AND CAST(p.itunesId AS INTEGER) = CAST(s.sample_itunes_id AS INTEGER))")
		methodCases = append(methodCases,
			"WHEN s.sample_itunes_id <> '' AND CAST(p.itunesId AS INTEGER) = CAST(s.sample_itunes_id AS INTEGER) THEN 'itunes_id'")
	}
	if hasGUID {
		matchBranches = append(matchBranches, "(s.sample_guid <> '' AND lower(trim(p.podcastGuid)) = s.sample_guid)")
		methodCases = append(methodCases,
			"WHEN s.sample_guid <> '' AND lower(trim(p.podcastGuid)) = s.sample_guid THEN 'podcast_guid'")
	}
	if hasAuthor {
		matchBranches = append(matchBranches, "(s.sample_author <> '' AND p.title = s.sample_title AND p.itunesAuthor = s.sample_author)")
		methodCases = append(methodCases,
			"WHEN s.sample_author <> '' AND p.title = s.sample_title AND p.itunesAuthor = s.sample_author THEN 'title_author'")
	}
	matchBranches = append(matchBranches, "p.title = s.sample_title")
	methodCases = append(methodCases, "WHEN 1 = 1 THEN 'title_only'")
	var rankCases []string
	if hasITunesID {
		rankCases = append(rankCases,
			"WHEN s.sample_itunes_id <> '' AND CAST(p.itunesId AS INTEGER) = CAST(s.sample_itunes_id AS INTEGER) THEN 1")
	}
	if hasGUID {
		rankCases = append(rankCases,
			"WHEN s.sample_guid <> '' AND lower(trim(p.podcastGuid)) = s.sample_guid THEN 2")
	}
	if hasAuthor {
		rankCases = append(rankCases,
			"WHEN s.sample_author <> '' AND p.title = s.sample_title AND p.itunesAuthor = s.sample_author THEN 3")
	}
	rankCases = append(rankCases, "WHEN 1 = 1 THEN 4")
	query := `WITH sample_inputs(sample_id, sample_title, sample_author, sample_feed_url, sample_itunes_id, sample_guid) AS (VALUES ` + strings.Join(valueParts, ",") + `),
matched AS (
	SELECT s.sample_id,
	       s.sample_title,
	       s.sample_feed_url,
	       p.id,
	       p.title,
	       p.itunesAuthor,
	       p.url,
	       CAST(p.itunesId AS TEXT) AS itunes_id_text,
	       p.podcastGuid,
	       p.dead,
	       p.lastHttpStatus,
	       p.newestItemPubdate,
	       p.episodeCount,
	       p.popularityScore,
       CASE ` + strings.Join(methodCases, "\n         ") + `
       END AS identity_method,
       CASE ` + strings.Join(rankCases, "\n         ") + `
       END AS identity_rank
FROM sample_inputs s
JOIN podcasts p
  ON p.url NOT LIKE ?
 AND (` + strings.Join(matchBranches, "\n   OR ") + `)
), ranked AS (
	SELECT matched.*,
	       ROW_NUMBER() OVER (
	         PARTITION BY sample_id
	         ORDER BY identity_rank,
	                  dead ASC,
	                  CASE WHEN lastHttpStatus = 200 THEN 0 ELSE 1 END,
	                  newestItemPubdate DESC,
	                  episodeCount DESC,
	                  popularityScore DESC,
	                  id ASC
	       ) AS row_num
	FROM matched
)
	SELECT sample_id, sample_title, sample_feed_url, id, title, itunesAuthor, url,
	       itunes_id_text, podcastGuid, dead, lastHttpStatus,
       newestItemPubdate, episodeCount, popularityScore, identity_method
FROM ranked
WHERE row_num = 1`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("bulk sample candidate lookup: %w", err)
	}
	defer rows.Close()
	matches := make(map[int64]CandidateMatch, len(samples))
	for rows.Next() {
		var sampleID, candidateID int64
		var sampleTitle, sampleFeedURL string
		var candidateTitle, candidateAuthor, candidateURL, candidateITunesID, candidateGUID sql.NullString
		var dead, status, newest, episodes, popularity int64
		var method string
		if err := rows.Scan(&sampleID, &sampleTitle, &sampleFeedURL, &candidateID, &candidateTitle, &candidateAuthor, &candidateURL, &candidateITunesID, &candidateGUID, &dead, &status, &newest, &episodes, &popularity, &method); err != nil {
			return nil, fmt.Errorf("scan bulk sample match: %w", err)
		}
		matches[sampleID] = CandidateMatch{
			SampleID:          sampleID,
			SampleTitle:       sampleTitle,
			CurrentFeedURL:    sampleFeedURL,
			CandidateID:       candidateID,
			CandidateTitle:    candidateTitle.String,
			CandidateAuthor:   candidateAuthor.String,
			CandidateURL:      candidateURL.String,
			CandidateITunesID: candidateITunesID.String,
			CandidateGUID:     candidateGUID.String,
			IdentityMethod:    method,
			Confidence:        confidenceForMethod(method),
			IdentityConfirmed: method == "itunes_id" || method == "podcast_guid",
			TitleOnly:         method == "title_only",
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read bulk sample matches: %w", err)
	}
	return matches, nil
}

func confidenceForMethod(method string) string {
	switch method {
	case "itunes_id", "podcast_guid":
		return "high"
	case "title_author":
		return "supporting"
	case "title_only":
		return "low"
	default:
		return ""
	}
}

func checkAccessibility(ctx context.Context, comparison *SampleComparison, options CompareOptions) {
	client := options.AccessibilityClient
	if client == nil {
		client = NewDirectHTTPClient(options.AccessibilityTimeout)
	}
	uniqueURLs := make(map[string]struct{})
	for index := range comparison.Matches {
		if comparison.Matches[index].CandidateID != 0 && comparison.Matches[index].CandidateURL != "" {
			uniqueURLs[comparison.Matches[index].CandidateURL] = struct{}{}
		}
	}
	results := make(map[string]Accessibility, len(uniqueURLs))
	var mu sync.Mutex
	jobs := make(chan string)
	var wg sync.WaitGroup
	workers := options.MaxConcurrency
	if workers > len(uniqueURLs) {
		workers = len(uniqueURLs)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for url := range jobs {
				result := checkURL(ctx, client, url)
				mu.Lock()
				results[url] = result
				mu.Unlock()
			}
		}()
	}
	for url := range uniqueURLs {
		jobs <- url
	}
	close(jobs)
	wg.Wait()

	for index := range comparison.Matches {
		url := comparison.Matches[index].CandidateURL
		if result, ok := results[url]; ok {
			comparison.Matches[index].Accessible = &result
			if result.OK {
				comparison.AccessibleAny++
				if comparison.Matches[index].IdentityConfirmed {
					comparison.AccessibleIdentityConfirmed++
				}
			}
		}
	}
}

func checkURL(ctx context.Context, client *http.Client, sourceURL string) Accessibility {
	result := Accessibility{URL: sourceURL, CheckedAt: time.Now()}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	request.Header.Set("User-Agent", "MagicPodcast-PodcastIndex-Validator/1")
	request.Header.Set("Range", "bytes=0-1023")
	response, err := client.Do(request)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer response.Body.Close()
	result.StatusCode = response.StatusCode
	if response.Request != nil {
		result.FinalURL = response.Request.URL.String()
	}
	result.OK = response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
	if !result.OK {
		result.Error = fmt.Sprintf("HTTP %d", response.StatusCode)
	}
	return result
}

func normalizeITunesID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if numeric, err := strconv.ParseInt(value, 10, 64); err == nil && numeric > 0 {
		return strconv.FormatInt(numeric, 10)
	}
	return ""
}

func NormalizeSamples(samples []FailedSample) []FailedSample {
	copySamples := append([]FailedSample(nil), samples...)
	sort.Slice(copySamples, func(i, j int) bool { return copySamples[i].ID < copySamples[j].ID })
	return copySamples
}
