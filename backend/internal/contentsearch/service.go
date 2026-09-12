package contentsearch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"magicpodcast/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db *gorm.DB
}

// WithTransaction joins index publication to the caller's fact transaction.
// It retains the same indexing behavior and never commits the outer transaction.
func (s *Service) WithTransaction(tx *gorm.DB) Module { return &Service{db: tx} }

// Search consumes identity facts; old index copies are not identity authority.
// Text remains searchable when an attribution is revoked; only identity queries
// and returned person labels require agreement with current attribution facts.
// An explicit name confirmation plus matching fragment confirmation remains
// usable when the automatic preparation has not yet been upgraded.
const reliableAttributionSQL = `
content_search_fragments.attribution_status = 'confirmed'
AND (NOT EXISTS (SELECT 1 FROM speech_attributions a WHERE a.episode_id = content_search_fragments.episode_id)
 OR EXISTS (SELECT 1 FROM speech_attributions a
 WHERE a.episode_id = content_search_fragments.episode_id
 AND a.source_kind = content_search_fragments.source_kind
 AND a.source_version = content_search_fragments.source_version
 AND a.fragment_order = content_search_fragments.fragment_order
 AND a.person_id IS content_search_fragments.person_id
 AND a.status = content_search_fragments.attribution_status
 AND a.text = content_search_fragments.text))
AND NOT EXISTS (SELECT 1 FROM person_appearance_overrides po WHERE po.episode_id = content_search_fragments.episode_id AND po.person_id = content_search_fragments.person_id AND po.excluded = 1)
AND EXISTS (SELECT 1 FROM episode_appearances ap
 WHERE ap.episode_id = content_search_fragments.episode_id
 AND ap.person_id = content_search_fragments.person_id
 AND ap.source_version = content_search_fragments.source_version AND ap.status = 'confirmed')
AND (
 EXISTS (SELECT 1 FROM person_user_confirmations n
 WHERE n.episode_id = content_search_fragments.episode_id AND n.kind = 'person_name'
 AND n.person_id = content_search_fragments.person_id)
 AND EXISTS (SELECT 1 FROM person_user_confirmations c
 JOIN speech_attributions a ON a.episode_id = c.episode_id
 AND a.source_kind = c.source_kind AND a.source_version = c.source_version
 AND a.fragment_order = c.fragment_order
 WHERE c.episode_id = content_search_fragments.episode_id AND c.kind = 'fragment_attribution'
 AND c.assigned_person_id = content_search_fragments.person_id
 AND c.source_kind = content_search_fragments.source_kind
 AND c.source_version = content_search_fragments.source_version
 AND c.fragment_order = content_search_fragments.fragment_order
 AND c.source_text = content_search_fragments.text AND c.speaker_label = a.speaker_label
 AND c.status = 'confirmed')
 )`

func NewService(db *gorm.DB) (*Service, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: database is required", ErrSearchFailed)
	}
	return &Service{db: db}, nil
}

func (s *Service) Search(ctx context.Context, request Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(request.Query) == "" {
		return Result{}, fmt.Errorf("%w: query is required", ErrInvalidRequest)
	}
	scopeIDs := uniqueUint(request.Scope.EpisodeIDs)
	if request.Filter.EpisodeID != nil {
		if !containsUint(scopeIDs, *request.Filter.EpisodeID) {
			return Result{
				Hits: []Hit{},
				Coverage: Coverage{
					Complete: true,
					Reason:   CoverageMiss,
				},
			}, nil
		}
		scopeIDs = []uint{*request.Filter.EpisodeID}
	}
	if len(scopeIDs) == 0 {
		return Result{
			Hits: []Hit{},
			Coverage: Coverage{
				Complete: true,
				Reason:   CoverageMiss,
			},
		}, nil
	}
	limit := request.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	coverage, err := s.coverageFor(ctx, scopeIDs, request.Filter.SourceKind, request.Filter.PersonID != nil)
	if err != nil {
		return Result{}, err
	}

	query := s.db.WithContext(ctx).Model(&models.ContentSearchFragment{}).
		Where("current = ? AND episode_id IN ?", true, scopeIDs).
		Where("EXISTS (SELECT 1 FROM episodes e JOIN podcasts p ON p.id = e.podcast_id WHERE e.id = content_search_fragments.episode_id AND e.deleted_at IS NULL AND p.deleted_at IS NULL)").
		Where("source_kind != ? OR (content_search_fragments.source_version NOT LIKE 'artifact-%' AND NOT EXISTS (SELECT 1 FROM episode_artifact_sets a WHERE a.episode_id = content_search_fragments.episode_id)) OR EXISTS (SELECT 1 FROM episode_artifact_sets a WHERE a.episode_id = content_search_fragments.episode_id AND a.is_current = 1 AND 'artifact-' || a.id = content_search_fragments.source_version)", SourceTranscript).
		Where("source_kind != ? OR text = (SELECT show_notes FROM episodes e WHERE e.id = content_search_fragments.episode_id)", SourceShowNotes).
		Where("source_kind != ? OR NOT EXISTS (SELECT 1 FROM speech_attributions a WHERE a.episode_id = content_search_fragments.episode_id) OR EXISTS (SELECT 1 FROM speech_attributions a WHERE a.episode_id = content_search_fragments.episode_id AND a.source_kind = content_search_fragments.source_kind AND a.source_version = content_search_fragments.source_version AND a.fragment_order = content_search_fragments.fragment_order AND a.text = content_search_fragments.text)", SourceTranscript)
	if request.Filter.PersonID != nil {
		query = query.Where("person_id = ? AND attribution_status = ?", *request.Filter.PersonID, "confirmed")
		query = query.Where(reliableAttributionSQL)
	}
	if strings.TrimSpace(request.Filter.SourceKind) != "" {
		query = query.Where("source_kind = ?", request.Filter.SourceKind)
	}
	var rows []struct {
		models.ContentSearchFragment
		ReliableAttribution bool
	}
	if err := query.Select("content_search_fragments.*, (" + reliableAttributionSQL + ") AS reliable_attribution").Find(&rows).Error; err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}

	queryTokens := tokenize(request.Query)
	type ranked struct {
		row   models.ContentSearchFragment
		score int
	}
	rankedHits := make([]ranked, 0, len(rows))
	for _, candidate := range rows {
		row := candidate.ContentSearchFragment
		if !candidate.ReliableAttribution && row.PersonID != nil {
			row.PersonID = nil
			row.AttributionStatus = "pending"
		}
		score := overlapScore(queryTokens, strings.Fields(row.Tokens))
		if score <= 0 && !strings.Contains(row.Text, strings.TrimSpace(request.Query)) {
			continue
		}
		if score <= 0 {
			score = 1
		}
		rankedHits = append(rankedHits, ranked{row: row, score: score})
	}
	sort.SliceStable(rankedHits, func(i, j int) bool {
		if rankedHits[i].score != rankedHits[j].score {
			return rankedHits[i].score > rankedHits[j].score
		}
		if !rankedHits[i].row.PublishedAt.Equal(rankedHits[j].row.PublishedAt) {
			return rankedHits[i].row.PublishedAt.After(rankedHits[j].row.PublishedAt)
		}
		if rankedHits[i].row.EpisodeID != rankedHits[j].row.EpisodeID {
			return rankedHits[i].row.EpisodeID < rankedHits[j].row.EpisodeID
		}
		if rankedHits[i].row.SourceKind != rankedHits[j].row.SourceKind {
			return rankedHits[i].row.SourceKind < rankedHits[j].row.SourceKind
		}
		return rankedHits[i].row.FragmentOrder < rankedHits[j].row.FragmentOrder
	})

	truncated := len(rankedHits) > limit
	if truncated {
		rankedHits = rankedHits[:limit]
		coverage.Truncated = true
		if coverage.Reason == "" || coverage.Reason == CoverageComplete || coverage.Reason == CoverageMiss {
			coverage.Reason = CoverageTruncated
		}
	}
	type sourceLabel struct {
		ID           uint
		Title        string
		PodcastTitle string
	}
	labels := map[uint]sourceLabel{}
	hitIDs := []uint{}
	for _, item := range rankedHits {
		hitIDs = append(hitIDs, item.row.EpisodeID)
	}
	if len(hitIDs) > 0 {
		var rows []sourceLabel
		if err := s.db.WithContext(ctx).Table("episodes").Select("episodes.id, episodes.title, podcasts.title AS podcast_title").Joins("JOIN podcasts ON podcasts.id = episodes.podcast_id").Where("episodes.id IN ?", uniqueUint(hitIDs)).Scan(&rows).Error; err != nil {
			return Result{}, err
		}
		for _, row := range rows {
			labels[row.ID] = row
		}
	}
	hits := make([]Hit, 0, len(rankedHits))
	for _, item := range rankedHits {
		hits = append(hits, Hit{
			EpisodeID:         item.row.EpisodeID,
			EpisodeTitle:      labels[item.row.EpisodeID].Title,
			PodcastTitle:      labels[item.row.EpisodeID].PodcastTitle,
			SourceKind:        item.row.SourceKind,
			SourceVersion:     item.row.SourceVersion,
			FragmentOrder:     item.row.FragmentOrder,
			StartMS:           item.row.StartMS,
			Text:              item.row.Text,
			ContextBefore:     item.row.ContextBefore,
			ContextAfter:      item.row.ContextAfter,
			PersonID:          item.row.PersonID,
			AttributionStatus: item.row.AttributionStatus,
			PublishedAt:       item.row.PublishedAt.UTC().Format("2006-01-02"),
		})
	}
	if len(hits) == 0 && coverage.Complete && !coverage.Truncated {
		coverage.Reason = CoverageMiss
	}
	return Result{Hits: hits, Coverage: coverage}, nil
}

func (s *Service) ReplaceEpisode(ctx context.Context, document EpisodeDocument) error {
	if document.EpisodeID == 0 {
		return fmt.Errorf("%w: episode is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(document.SourceKind) == "" {
		document.SourceKind = SourceTranscript
	}
	if strings.TrimSpace(document.SourceVersion) == "" {
		return fmt.Errorf("%w: source version is required", ErrInvalidRequest)
	}
	now := time.Now().UTC()
	publishedAt := document.PublishedAt
	if publishedAt.IsZero() {
		publishedAt = now
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if expected := document.AttributionVersion; expected != nil {
			var state models.PersonPreparation
			err := tx.First(&state, "episode_id = ?", document.EpisodeID).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if state.Revision != expected.Revision || state.PublishedRevision != expected.PublishedRevision {
				return ErrStaleDocument
			}
		}

		if err := tx.Model(&models.ContentSearchFragment{}).
			Where("episode_id = ? AND source_kind = ?", document.EpisodeID, document.SourceKind).
			Updates(map[string]any{"current": false, "updated_at": now}).Error; err != nil {
			return err
		}
		for index, fragment := range document.Fragments {
			before, after := neighborContext(document.Fragments, index)
			row := models.ContentSearchFragment{
				EpisodeID:         document.EpisodeID,
				SourceKind:        document.SourceKind,
				SourceVersion:     document.SourceVersion,
				FragmentOrder:     fragment.Order,
				StartMS:           fragment.StartMS,
				Text:              fragment.Text,
				ContextBefore:     before,
				ContextAfter:      after,
				PersonID:          fragment.PersonID,
				AttributionStatus: fragment.AttributionStatus,
				Tokens:            tokenString(tokenize(fragment.Text)),
				PublishedAt:       publishedAt,
				Current:           true,
				CreatedAt:         now,
				UpdatedAt:         now,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "episode_id"}, {Name: "source_kind"}, {Name: "source_version"}, {Name: "fragment_order"},
				},
				DoUpdates: clause.AssignmentColumns([]string{
					"start_ms", "text", "context_before", "context_after", "person_id",
					"attribution_status", "tokens", "published_at", "current", "updated_at",
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		if document.SourceKind == SourceTranscript {
			showNotesDoc := EpisodeDocument{
				ShowNotes:        document.ShowNotes,
				EpisodeID:        document.EpisodeID,
				PublishedAt:      publishedAt,
				SourceKind:       SourceShowNotes,
				SourceVersion:    document.SourceVersion,
				Complete:         document.Complete,
				IncompleteReason: document.IncompleteReason,
				Fragments: []FragmentInput{{
					Order: 1,
					Text:  document.ShowNotes,
				}},
			}
			if err := replaceShowNotes(tx, showNotesDoc, now); err != nil {
				return err
			}
		}
		reason := document.IncompleteReason
		complete := document.Complete
		if !complete && reason == "" {
			reason = CoverageIndexNotReady
		}
		if complete {
			reason = CoverageComplete
		}
		coverage := models.ContentSearchCoverage{
			EpisodeID:     document.EpisodeID,
			SourceKind:    document.SourceKind,
			SourceVersion: document.SourceVersion,
			Complete:      complete,
			Reason:        reason,
			UpdatedAt:     now,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "episode_id"}, {Name: "source_kind"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"source_version", "complete", "reason", "updated_at",
			}),
		}).Create(&coverage).Error
	})
}

func replaceShowNotes(tx *gorm.DB, document EpisodeDocument, now time.Time) error {
	if err := tx.Model(&models.ContentSearchFragment{}).
		Where("episode_id = ? AND source_kind = ?", document.EpisodeID, SourceShowNotes).
		Updates(map[string]any{"current": false, "updated_at": now}).Error; err != nil {
		return err
	}
	row := models.ContentSearchFragment{
		EpisodeID:     document.EpisodeID,
		SourceKind:    SourceShowNotes,
		SourceVersion: document.SourceVersion,
		FragmentOrder: 1,
		Text:          document.ShowNotes,
		Tokens:        tokenString(tokenize(document.ShowNotes)),
		PublishedAt:   document.PublishedAt,
		Current:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "episode_id"}, {Name: "source_kind"}, {Name: "source_version"}, {Name: "fragment_order"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"text", "tokens", "published_at", "current", "updated_at",
		}),
	}).Create(&row).Error; err != nil {
		return err
	}
	coverage := models.ContentSearchCoverage{
		EpisodeID:     document.EpisodeID,
		SourceKind:    SourceShowNotes,
		SourceVersion: document.SourceVersion,
		Complete:      document.Complete,
		Reason:        document.IncompleteReason,
		UpdatedAt:     now,
	}
	if coverage.Complete {
		coverage.Reason = CoverageComplete
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "episode_id"}, {Name: "source_kind"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"source_version", "complete", "reason", "updated_at",
		}),
	}).Create(&coverage).Error
}

func (s *Service) RemoveEpisode(ctx context.Context, episodeID uint) error {
	if episodeID == 0 {
		return fmt.Errorf("%w: episode is required", ErrInvalidRequest)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("episode_id = ?", episodeID).Delete(&models.ContentSearchFragment{}).Error; err != nil {
			return err
		}
		return tx.Where("episode_id = ?", episodeID).Delete(&models.ContentSearchCoverage{}).Error
	})
}

func (s *Service) coverageFor(ctx context.Context, episodeIDs []uint, sourceKind string, personFiltered bool) (Coverage, error) {
	query := s.db.WithContext(ctx).Model(&models.ContentSearchCoverage{}).
		Where("episode_id IN ?", episodeIDs)
	sourceKinds := []string{sourceKind}
	if strings.TrimSpace(sourceKind) == "" {
		sourceKinds = []string{SourceTranscript, SourceShowNotes}
	}
	query = query.Where("source_kind IN ?", sourceKinds)
	var rows []models.ContentSearchCoverage
	if err := query.Find(&rows).Error; err != nil {
		return Coverage{}, fmt.Errorf("%w: %v", ErrSearchFailed, err)
	}
	byEpisode := map[string]models.ContentSearchCoverage{}
	for _, row := range rows {
		var active int64
		if err := s.db.WithContext(ctx).Model(&models.Episode{}).Where("id = ?", row.EpisodeID).Count(&active).Error; err != nil {
			return Coverage{}, err
		}
		if active == 0 {
			row.Complete = false
		}
		if row.SourceKind == SourceTranscript {
			var artifacts []models.EpisodeArtifactSet
			if err := s.db.WithContext(ctx).Select("id", "is_current").Where("episode_id = ?", row.EpisodeID).Find(&artifacts).Error; err != nil {
				return Coverage{}, err
			}
			if len(artifacts) > 0 || strings.HasPrefix(row.SourceVersion, "artifact-") {
				valid := false
				for _, a := range artifacts {
					if a.IsCurrent && fmt.Sprintf("artifact-%d", a.ID) == row.SourceVersion {
						valid = true
					}
				}
				row.Complete = row.Complete && valid
			}
		}
		if personFiltered {
			var prepared int64
			if err := s.db.WithContext(ctx).Model(&models.PersonPreparation{}).Where("episode_id = ? AND source_version = ? AND algorithm_version = ?", row.EpisodeID, row.SourceVersion, models.CurrentIdentityAlgorithm).Count(&prepared).Error; err != nil {
				return Coverage{}, err
			}
			row.Complete = row.Complete && prepared > 0
		}
		byEpisode[fmt.Sprintf("%d:%s", row.EpisodeID, row.SourceKind)] = row
	}
	missing := make([]uint, 0)
	complete := true
	reason := CoverageComplete
	for _, episodeID := range episodeIDs {
		for _, kind := range sourceKinds {
			row, ok := byEpisode[fmt.Sprintf("%d:%s", episodeID, kind)]
			if !ok || !row.Complete {
				complete = false
				missing = append(missing, episodeID)
				reason = CoverageIndexNotReady
				break
			}
		}
	}
	return Coverage{
		Complete:        complete && len(missing) == 0,
		Reason:          reason,
		MissingEpisodes: missing,
	}, nil
}

func neighborContext(fragments []FragmentInput, index int) (string, string) {
	before := ""
	after := ""
	if index > 0 {
		before = fragments[index-1].Text
	}
	if index+1 < len(fragments) {
		after = fragments[index+1].Text
	}
	return before, after
}

func uniqueUint(values []uint) []uint {
	seen := map[uint]struct{}{}
	out := make([]uint, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsUint(values []uint, target uint) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

var _ Module = (*Service)(nil)
