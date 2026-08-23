package services

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	episodelabel "magicpodcast/internal/episode"

	"gorm.io/gorm"
)

const (
	CompletionHistoryDefaultLimit = 50
	CompletionHistoryMaxLimit     = 50

	CompletionHistoryStatusDismissed  = "dismissed"
	CompletionHistoryStatusUnassigned = "unassigned"
)

var ErrInvalidCompletionHistoryCursor = errors.New("invalid completion history cursor")

type CompletionHistoryOptions struct {
	Query  string
	Cursor string
	Limit  int
}

type CompletionHistoryItem struct {
	EpisodeID       uint      `json:"episode_id"`
	PodcastID       uint      `json:"podcast_id"`
	PodcastTitle    string    `json:"podcast_title"`
	PodcastCoverURL string    `json:"podcast_cover_url"`
	EpisodeTitle    string    `json:"episode_title"`
	EpisodeNo       string    `json:"episode_no"`
	ImageURL        string    `json:"image_url"`
	CompletedAt     time.Time `json:"completed_at"`
	CurrentStatus   string    `json:"current_status"`
}

type CompletionHistorySnapshot struct {
	Items       []CompletionHistoryItem `json:"items"`
	TotalCount  int64                   `json:"total_count"`
	MatchCount  int64                   `json:"match_count"`
	HasMore     bool                    `json:"has_more"`
	NextCursor  string                  `json:"next_cursor,omitempty"`
	SearchQuery string                  `json:"search_query"`
}

type completionHistoryCursor struct {
	CompletedAt time.Time `json:"completed_at"`
	EpisodeID   uint      `json:"episode_id"`
	Query       string    `json:"query"`
}

type completionHistoryRow struct {
	EpisodeID       uint
	PodcastID       uint
	PodcastTitle    string
	PodcastCoverURL string
	EpisodeTitle    string
	EpisodeNo       string
	ImageURL        string
	CompletedAt     time.Time
	QueueState      *string
	DismissedAt     *time.Time
}

func (s *ConsumptionService) ListCompletionHistory(
	options CompletionHistoryOptions,
) (CompletionHistorySnapshot, error) {
	limit := options.Limit
	if limit <= 0 {
		limit = CompletionHistoryDefaultLimit
	}
	if limit > CompletionHistoryMaxLimit {
		limit = CompletionHistoryMaxLimit
	}
	query := normalizeCompletionHistoryQuery(options.Query)

	var cursor *completionHistoryCursor
	if options.Cursor != "" {
		decoded, err := decodeCompletionHistoryCursor(options.Cursor)
		if err != nil || decoded.Query != query {
			return CompletionHistorySnapshot{}, ErrInvalidCompletionHistoryCursor
		}
		cursor = &decoded
	}

	var snapshot CompletionHistorySnapshot
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := completionHistoryBaseQuery(tx, "").
			Count(&snapshot.TotalCount).Error; err != nil {
			return fmt.Errorf("count completion history: %w", err)
		}
		if err := completionHistoryBaseQuery(tx, query).
			Count(&snapshot.MatchCount).Error; err != nil {
			return fmt.Errorf("count matching completion history: %w", err)
		}

		pageQuery := completionHistoryBaseQuery(tx, query).
			Select(`
				episode_completions.episode_id,
				episode_completions.completed_at,
				episodes.podcast_id,
				episodes.title AS episode_title,
				episodes.episode_no,
				episodes.image_url,
				podcasts.title AS podcast_title,
				CASE
					WHEN podcasts.custom_cover_url <> '' THEN podcasts.custom_cover_url
					ELSE podcasts.cover_url
				END AS podcast_cover_url,
				episode_triage_decisions.queue_state,
				episode_triage_decisions.dismissed_at
			`)
		if cursor != nil {
			pageQuery = pageQuery.Where(
				`episode_completions.completed_at < ?
				 OR (
					episode_completions.completed_at = ?
					AND episode_completions.episode_id < ?
				 )`,
				cursor.CompletedAt,
				cursor.CompletedAt,
				cursor.EpisodeID,
			)
		}

		var rows []completionHistoryRow
		if err := pageQuery.
			Order("episode_completions.completed_at DESC").
			Order("episode_completions.episode_id DESC").
			Limit(limit + 1).
			Scan(&rows).Error; err != nil {
			return fmt.Errorf("list completion history: %w", err)
		}

		snapshot.HasMore = len(rows) > limit
		if snapshot.HasMore {
			rows = rows[:limit]
		}
		snapshot.Items = make([]CompletionHistoryItem, 0, len(rows))
		for _, row := range rows {
			snapshot.Items = append(snapshot.Items, CompletionHistoryItem{
				EpisodeID:       row.EpisodeID,
				PodcastID:       row.PodcastID,
				PodcastTitle:    row.PodcastTitle,
				PodcastCoverURL: row.PodcastCoverURL,
				EpisodeTitle:    row.EpisodeTitle,
				EpisodeNo:       episodelabel.Normalize(row.EpisodeTitle, row.EpisodeNo),
				ImageURL:        row.ImageURL,
				CompletedAt:     row.CompletedAt,
				CurrentStatus:   completionHistoryStatus(row.QueueState, row.DismissedAt),
			})
		}
		if snapshot.HasMore && len(rows) > 0 {
			last := rows[len(rows)-1]
			encoded, err := encodeCompletionHistoryCursor(completionHistoryCursor{
				CompletedAt: last.CompletedAt,
				EpisodeID:   last.EpisodeID,
				Query:       query,
			})
			if err != nil {
				return err
			}
			snapshot.NextCursor = encoded
		}
		snapshot.SearchQuery = query
		return nil
	})
	if err != nil {
		return CompletionHistorySnapshot{}, err
	}
	return snapshot, nil
}

func completionHistoryBaseQuery(tx *gorm.DB, query string) *gorm.DB {
	result := tx.
		Table("episode_completions").
		Joins(
			"JOIN episodes ON episodes.id = episode_completions.episode_id AND episodes.deleted_at IS NULL",
		).
		Joins(
			"JOIN podcasts ON podcasts.id = episodes.podcast_id AND podcasts.deleted_at IS NULL",
		).
		Joins(
			"LEFT JOIN episode_triage_decisions ON episode_triage_decisions.episode_id = episode_completions.episode_id",
		)
	if query == "" {
		return result
	}
	pattern := "%" + escapeCompletionHistoryLike(query) + "%"
	return result.Where(
		`(
			LOWER(episodes.title) LIKE ? ESCAPE '\'
			OR LOWER(podcasts.title) LIKE ? ESCAPE '\'
		)`,
		pattern,
		pattern,
	)
}

func normalizeCompletionHistoryQuery(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func escapeCompletionHistoryLike(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return replacer.Replace(value)
}

func completionHistoryStatus(queueState *string, dismissedAt *time.Time) string {
	if queueState != nil && ValidQueueState(*queueState) {
		return *queueState
	}
	if dismissedAt != nil {
		return CompletionHistoryStatusDismissed
	}
	return CompletionHistoryStatusUnassigned
}

func encodeCompletionHistoryCursor(cursor completionHistoryCursor) (string, error) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode completion history cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCompletionHistoryCursor(value string) (completionHistoryCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return completionHistoryCursor{}, ErrInvalidCompletionHistoryCursor
	}
	var cursor completionHistoryCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return completionHistoryCursor{}, ErrInvalidCompletionHistoryCursor
	}
	if cursor.EpisodeID == 0 || cursor.CompletedAt.IsZero() {
		return completionHistoryCursor{}, ErrInvalidCompletionHistoryCursor
	}
	cursor.Query = normalizeCompletionHistoryQuery(cursor.Query)
	return cursor, nil
}
