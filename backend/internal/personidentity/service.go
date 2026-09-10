package personidentity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"magicpodcast/internal/contentsearch"
	"magicpodcast/internal/models"
	"magicpodcast/internal/processing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db        *gorm.DB
	reader    processing.ArtifactReader
	suggester CandidateSuggester
	search    contentsearch.Module
}

func NewService(db *gorm.DB, suggester CandidateSuggester, searchers ...contentsearch.Module) (*Service, error) {
	if db == nil {
		return nil, fmt.Errorf("person identity database is required")
	}
	service := &Service{db: db, suggester: suggester}
	if len(searchers) > 0 {
		service.search = searchers[0]
	}
	return service, nil
}

func (s *Service) Prepare(ctx context.Context, sources EpisodeSources) (EpisodePeople, error) {
	if sources.EpisodeID == 0 {
		return EpisodePeople{}, ErrEpisodeNotFound
	}
	if strings.TrimSpace(sources.SourceKind) == "" {
		sources.SourceKind = SourceTranscript
	}
	if strings.TrimSpace(sources.SourceVersion) == "" {
		return EpisodePeople{}, ErrTranscriptRequired
	}
	if err := s.requireEpisode(ctx, sources.EpisodeID); err != nil {
		return EpisodePeople{}, err
	}
	candidates, fragments := extractFromSources(sources)
	if s.suggester != nil {
		suggested, err := s.suggester.Suggest(ctx, sources)
		if err != nil {
			return EpisodePeople{}, err
		}
		candidates = mergeSuggestedCandidates(candidates, suggested)
		fragments = bindFragments(sources.Segments, candidates)
		assignments := map[int][]string{}
		for _, item := range suggested {
			if item.EvidenceKind == "verified_runtime" {
				for _, order := range item.SpeechOrders {
					assignments[order] = append(assignments[order], item.DisplayName)
				}
			}
		}
		for i := range fragments {
			if names := assignments[fragments[i].Order]; len(names) == 1 {
				fragments[i].PersonName = names[0]
				fragments[i].Status = StatusConfirmed
				fragments[i].EvidenceKind = "verified_runtime"
			}
		}
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		confirmations, err := loadConfirmations(tx, sources.EpisodeID)
		if err != nil {
			return err
		}
		peopleByName := map[string]models.Person{}
		for _, candidate := range candidates {
			if candidate.MentionedOnly {
				continue
			}
			person, err := upsertPerson(tx, sources.EpisodeID, candidate)
			if err != nil {
				return err
			}
			peopleByName[candidate.DisplayName] = person
			for _, alias := range candidate.Aliases {
				peopleByName[alias] = person
			}
			if err := upsertAppearance(tx, sources.EpisodeID, sources.SourceVersion, person, candidate, confirmations); err != nil {
				return err
			}
		}
		if err := replaceAttributions(tx, sources, fragments, peopleByName, confirmations); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return EpisodePeople{}, err
	}
	listed, err := s.ListEpisodePeople(ctx, sources.EpisodeID)
	if err != nil {
		return EpisodePeople{}, err
	}
	if err := s.reindexSearch(ctx, sources.EpisodeID, listed); err != nil {
		return EpisodePeople{}, err
	}
	listed.IndexReady = len(listed.Attributions) > 0
	return listed, nil
}

func (s *Service) ListEpisodePeople(ctx context.Context, episodeID uint) (EpisodePeople, error) {
	if err := s.requireEpisode(ctx, episodeID); err != nil {
		return EpisodePeople{}, err
	}
	var episode models.Episode
	if err := s.db.WithContext(ctx).Select("id", "published_date").First(&episode, episodeID).Error; err != nil {
		return EpisodePeople{}, fmt.Errorf("load episode: %w", err)
	}
	currentVersion := currentAttributionVersion(s.db.WithContext(ctx), episodeID, SourceTranscript)
	var appearances []models.EpisodeAppearance
	if err := s.db.WithContext(ctx).
		Where("episode_id = ? AND source_version = ?", episodeID, currentVersion).
		Order("id ASC").
		Find(&appearances).Error; err != nil {
		return EpisodePeople{}, fmt.Errorf("list episode appearances: %w", err)
	}
	var attributions []models.SpeechAttribution
	query := s.db.WithContext(ctx).Where("episode_id = ?", episodeID)
	if currentVersion != "" {
		query = query.Where("source_kind = ? AND source_version = ?", SourceTranscript, currentVersion)
	}
	if err := query.Order("fragment_order ASC").Find(&attributions).Error; err != nil {
		return EpisodePeople{}, fmt.Errorf("list speech attributions: %w", err)
	}
	confirmations, err := loadConfirmations(s.db.WithContext(ctx), episodeID)
	if err != nil {
		return EpisodePeople{}, err
	}
	people := make([]PersonView, 0, len(appearances))
	for _, appearance := range appearances {
		person, err := s.loadPerson(ctx, appearance.PersonID)
		if err != nil {
			return EpisodePeople{}, err
		}
		view := PersonView{
			ID:              person.ID,
			StableKey:       person.StableKey,
			DisplayName:     person.DisplayName,
			Aliases:         personAliases(s.db.WithContext(ctx), person.ID),
			IdentityNote:    person.IdentityNote,
			Role:            appearance.Role,
			Status:          appearance.Status,
			StatusReason:    appearance.StatusReason,
			EvidenceKind:    appearance.EvidenceKind,
			EvidenceLocator: appearance.EvidenceLocator,
		}
		for _, attribution := range attributions {
			if attribution.PersonID != nil && *attribution.PersonID == person.ID {
				if attribution.Status == StatusConfirmed {
					view.ConfirmedSpeech++
				}
				if attribution.Status == StatusPending {
					view.PendingSpeech++
				}
			}
		}
		people = append(people, view)
	}
	attributionViews := make([]AttributionView, 0, len(attributions))
	for _, attribution := range attributions {
		view := AttributionView{
			ID:              attribution.ID,
			PersonID:        attribution.PersonID,
			SourceKind:      attribution.SourceKind,
			SourceVersion:   attribution.SourceVersion,
			FragmentOrder:   attribution.FragmentOrder,
			SpeakerLabel:    attribution.SpeakerLabel,
			StartMS:         attribution.StartMS,
			Text:            attribution.Text,
			Status:          attribution.Status,
			EvidenceKind:    attribution.EvidenceKind,
			EvidenceLocator: attribution.EvidenceLocator,
			UserConfirmed:   confirmationForFragment(confirmations, attribution) != nil,
		}
		if attribution.PersonID != nil {
			person, err := s.loadPerson(ctx, *attribution.PersonID)
			if err == nil {
				view.DisplayName = person.DisplayName
			}
		}
		attributionViews = append(attributionViews, view)
	}
	indexReady := len(attributions) > 0
	if s.search != nil {
		var indexed int64
		if err := s.db.WithContext(ctx).Model(&models.ContentSearchCoverage{}).Where("episode_id = ? AND source_kind = ? AND source_version = ? AND complete = ?", episodeID, SourceTranscript, currentVersion, true).Count(&indexed).Error; err != nil {
			return EpisodePeople{}, err
		}
		indexReady = indexReady && indexed > 0
	}
	return EpisodePeople{
		EpisodeID:     episodeID,
		SourceVersion: currentVersion,
		IndexReady:    indexReady,
		PublishedAt:   episode.PublishedDate,
		People:        people,
		Attributions:  attributionViews,
	}, nil
}

func (s *Service) CorrectName(ctx context.Context, episodeID uint, correction NameCorrection) (EpisodePeople, error) {
	if correction.PersonID == 0 {
		return EpisodePeople{}, ErrPersonNotFound
	}
	if strings.TrimSpace(correction.DisplayName) == "" {
		return EpisodePeople{}, ErrInvalidCorrection
	}
	if err := s.requireEpisode(ctx, episodeID); err != nil {
		return EpisodePeople{}, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var appearance models.EpisodeAppearance
		if err := tx.Where("episode_id = ? AND person_id = ?", episodeID, correction.PersonID).
			First(&appearance).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotEpisodeParticipant
			}
			return err
		}
		var person models.Person
		if err := tx.First(&person, correction.PersonID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPersonNotFound
			}
			return err
		}
		oldName := person.DisplayName
		person.DisplayName = strings.TrimSpace(correction.DisplayName)
		if correction.IdentityNote != "" {
			person.IdentityNote = strings.TrimSpace(correction.IdentityNote)
		}
		person.UpdatedAt = nowUTC()
		if err := tx.Save(&person).Error; err != nil {
			return err
		}
		if correction.Aliases != nil {
			if err := tx.Where("person_id = ?", person.ID).Delete(&models.PersonAlias{}).Error; err != nil {
				return err
			}
			for _, alias := range uniqueStrings(append(correction.Aliases, oldName)) {
				if err := tx.Create(&models.PersonAlias{
					PersonID:  person.ID,
					Alias:     alias,
					CreatedAt: nowUTC(),
				}).Error; err != nil {
					return err
				}
			}
		}
		if correction.Aliases == nil && oldName != person.DisplayName {
			if err := ensureAliases(tx, person.ID, []string{oldName}); err != nil {
				return err
			}
		}
		appearance.Status = StatusConfirmed
		appearance.StatusReason = "user confirmed name"
		appearance.UpdatedAt = nowUTC()
		if err := tx.Save(&appearance).Error; err != nil {
			return err
		}
		return upsertConfirmation(tx, models.PersonUserConfirmation{
			EpisodeID:    episodeID,
			Kind:         models.PersonConfirmationKindName,
			PersonID:     &person.ID,
			DisplayName:  person.DisplayName,
			IdentityNote: person.IdentityNote,
			Role:         appearance.Role,
		})
	})
	if err != nil {
		return EpisodePeople{}, err
	}
	return s.ListEpisodePeople(ctx, episodeID)
}

func (s *Service) CorrectAttribution(ctx context.Context, episodeID uint, correction AttributionCorrection) (EpisodePeople, error) {
	if correction.FragmentOrder <= 0 {
		return EpisodePeople{}, ErrInvalidCorrection
	}
	if strings.TrimSpace(correction.SourceKind) == "" {
		correction.SourceKind = SourceTranscript
	}
	status := strings.TrimSpace(correction.Status)
	if status == "" {
		if correction.AssignedPersonID == nil {
			status = StatusPending
		} else {
			status = StatusConfirmed
		}
	}
	if status == StatusConfirmed && correction.AssignedPersonID == nil {
		return EpisodePeople{}, ErrInvalidCorrection
	}
	if status != StatusConfirmed && status != StatusPending && status != StatusRejected {
		return EpisodePeople{}, ErrInvalidCorrection
	}
	if err := s.requireEpisode(ctx, episodeID); err != nil {
		return EpisodePeople{}, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if correction.AssignedPersonID != nil {
			var appearance models.EpisodeAppearance
			if err := tx.Where("episode_id = ? AND person_id = ?", episodeID, *correction.AssignedPersonID).
				First(&appearance).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrNotEpisodeParticipant
				}
				return err
			}
		}
		version := currentAttributionVersion(tx, episodeID, correction.SourceKind)
		if correction.SourceVersion != "" && correction.SourceVersion != version {
			return ErrInvalidCorrection
		}
		var row models.SpeechAttribution
		query := tx.Where(
			"episode_id = ? AND source_kind = ? AND fragment_order = ?",
			episodeID,
			correction.SourceKind,
			correction.FragmentOrder,
		)
		if version != "" {
			query = query.Where("source_version = ?", version)
		}
		if err := query.First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalidCorrection
			}
			return err
		}
		row.PersonID = correction.AssignedPersonID
		row.Status = status
		row.EvidenceKind = "user_confirmation"
		row.UpdatedAt = nowUTC()
		if correction.AssignedPersonID != nil {
			var appearance models.EpisodeAppearance
			if err := tx.Where("episode_id = ? AND person_id = ?", episodeID, *correction.AssignedPersonID).
				First(&appearance).Error; err == nil {
				row.AppearanceID = &appearance.ID
			}
		} else {
			row.AppearanceID = nil
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return upsertConfirmation(tx, models.PersonUserConfirmation{
			EpisodeID:        episodeID,
			Kind:             models.PersonConfirmationKindAttribution,
			FragmentOrder:    correction.FragmentOrder,
			SourceKind:       correction.SourceKind,
			SpeakerLabel:     row.SpeakerLabel,
			AssignedPersonID: correction.AssignedPersonID,
			PersonID:         nil,
			SourceVersion:    row.SourceVersion,
			SourceText:       row.Text,
			Status:           status,
		})
	})
	if err != nil {
		return EpisodePeople{}, err
	}
	listed, err := s.ListEpisodePeople(ctx, episodeID)
	if err != nil {
		return EpisodePeople{}, err
	}
	if err := s.reindexSearch(ctx, episodeID, listed); err != nil {
		return EpisodePeople{}, err
	}
	listed.IndexReady = len(listed.Attributions) > 0
	return listed, nil
}

func (s *Service) IndexPublishedTranscript(
	ctx context.Context,
	episodeID uint,
	sourceVersion string,
	showNotes string,
	segments []processing.TranscriptSegment,
) error {
	_, err := s.Prepare(ctx, EpisodeSources{
		EpisodeID:     episodeID,
		ShowNotes:     showNotes,
		SourceKind:    SourceTranscript,
		SourceVersion: sourceVersion,
		Segments:      SegmentsFromTranscript(segments),
	})
	return err
}

func (s *Service) RemoveIndexedEpisode(ctx context.Context, episodeID uint) error {
	if s.search == nil || episodeID == 0 {
		return nil
	}
	return s.search.RemoveEpisode(ctx, episodeID)
}

func (s *Service) reindexSearch(ctx context.Context, episodeID uint, listed EpisodePeople) error {
	if s.search == nil || listed.SourceVersion == "" {
		return nil
	}
	var episode models.Episode
	if err := s.db.WithContext(ctx).
		Select("id", "published_date", "show_notes").
		First(&episode, episodeID).Error; err != nil {
		return fmt.Errorf("load episode for search index: %w", err)
	}
	fragments := make([]contentsearch.FragmentInput, 0, len(listed.Attributions))
	for _, attribution := range listed.Attributions {
		if attribution.SourceKind != SourceTranscript {
			continue
		}
		fragments = append(fragments, contentsearch.FragmentInput{
			Order:             attribution.FragmentOrder,
			StartMS:           attribution.StartMS,
			Text:              attribution.Text,
			PersonID:          attribution.PersonID,
			AttributionStatus: attribution.Status,
		})
	}
	return s.search.ReplaceEpisode(ctx, contentsearch.EpisodeDocument{
		EpisodeID:     episodeID,
		PublishedAt:   episode.PublishedDate,
		ShowNotes:     episode.ShowNotes,
		SourceKind:    SourceTranscript,
		SourceVersion: listed.SourceVersion,
		Fragments:     fragments,
		Complete:      true,
	})
}

func (s *Service) ReliableSpeech(ctx context.Context, episodeID uint, personID uint) ([]AttributionFact, error) {
	facts, err := s.CurrentFacts(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	out := make([]AttributionFact, 0, len(facts))
	for _, fact := range facts {
		if fact.Current && fact.Status == StatusConfirmed && fact.PersonID != nil && *fact.PersonID == personID {
			out = append(out, fact)
		}
	}
	return out, nil
}

func (s *Service) CurrentFacts(ctx context.Context, episodeID uint) ([]AttributionFact, error) {
	if err := s.requireEpisode(ctx, episodeID); err != nil {
		return nil, err
	}
	var rows []models.SpeechAttribution
	if err := s.db.WithContext(ctx).
		Where("episode_id = ?", episodeID).
		Order("source_version DESC, fragment_order ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list attribution facts: %w", err)
	}
	current := map[string]string{
		SourceTranscript: currentAttributionVersion(s.db.WithContext(ctx), episodeID, SourceTranscript),
		SourceShowNotes:  currentAttributionVersion(s.db.WithContext(ctx), episodeID, SourceShowNotes),
	}
	facts := make([]AttributionFact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, AttributionFact{
			EpisodeID:     row.EpisodeID,
			PersonID:      row.PersonID,
			SourceKind:    row.SourceKind,
			SourceVersion: row.SourceVersion,
			FragmentOrder: row.FragmentOrder,
			SpeakerLabel:  row.SpeakerLabel,
			StartMS:       row.StartMS,
			Text:          row.Text,
			Status:        row.Status,
			Current:       current[row.SourceKind] == row.SourceVersion,
		})
	}
	return facts, nil
}

func (s *Service) AccessibleEpisodeIDs(ctx context.Context) ([]uint, error) {
	var ids []uint
	if err := s.db.WithContext(ctx).Model(&models.Episode{}).Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list accessible episodes: %w", err)
	}
	return ids, nil
}

func (s *Service) requireEpisode(ctx context.Context, episodeID uint) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.Episode{}).Where("id = ?", episodeID).Count(&count).Error; err != nil {
		return fmt.Errorf("lookup episode: %w", err)
	}
	if count == 0 {
		return ErrEpisodeNotFound
	}
	return nil
}

func (s *Service) loadPerson(ctx context.Context, id uint) (models.Person, error) {
	var person models.Person
	if err := s.db.WithContext(ctx).First(&person, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Person{}, ErrPersonNotFound
		}
		return models.Person{}, err
	}
	return person, nil
}

func upsertPerson(tx *gorm.DB, episodeID uint, candidate extractedCandidate) (models.Person, error) {
	if person, ok := findExistingPerson(tx, episodeID, candidate); ok {
		changed := false
		if person.IdentityNote == "" && candidate.IdentityNote != "" {
			person.IdentityNote = candidate.IdentityNote
			changed = true
		}
		if changed {
			person.UpdatedAt = nowUTC()
			if err := tx.Save(&person).Error; err != nil {
				return models.Person{}, err
			}
		}
		if err := ensureAliases(tx, person.ID, candidate.Aliases); err != nil {
			return models.Person{}, err
		}
		return person, nil
	}
	person := models.Person{
		StableKey:    newStableKey(),
		DisplayName:  candidate.DisplayName,
		IdentityNote: candidate.IdentityNote,
		CreatedAt:    nowUTC(),
		UpdatedAt:    nowUTC(),
	}
	if err := tx.Create(&person).Error; err != nil {
		return models.Person{}, err
	}
	if err := ensureAliases(tx, person.ID, candidate.Aliases); err != nil {
		return models.Person{}, err
	}
	return person, nil
}

func findExistingPerson(tx *gorm.DB, episodeID uint, candidate extractedCandidate) (models.Person, bool) {
	names := uniqueStrings(append([]string{candidate.DisplayName}, candidate.Aliases...))
	var people []models.Person
	if err := tx.Where("display_name IN ?", names).Find(&people).Error; err != nil {
		return models.Person{}, false
	}
	var aliased []models.Person
	if err := tx.Raw(`
		SELECT people.* FROM people
		INNER JOIN person_aliases ON person_aliases.person_id = people.id
		WHERE person_aliases.alias IN ?
	`, names).Scan(&aliased).Error; err == nil {
		people = append(people, aliased...)
	}
	seen := map[uint]models.Person{}
	for _, person := range people {
		seen[person.ID] = person
	}
	compatible := make([]models.Person, 0)
	for _, person := range seen {
		if identityConflicts(person.IdentityNote, candidate.IdentityNote) {
			continue
		}
		var appearances int64
		if err := tx.Model(&models.EpisodeAppearance{}).Where("episode_id = ? AND person_id = ?", episodeID, person.ID).Count(&appearances).Error; err != nil {
			continue
		}
		if appearances > 0 || (strings.TrimSpace(person.IdentityNote) != "" && strings.TrimSpace(candidate.IdentityNote) != "" && identityCompatible(person.IdentityNote, candidate.IdentityNote)) {
			compatible = append(compatible, person)
		}
	}
	if len(compatible) == 1 {
		return compatible[0], true
	}

	return models.Person{}, false
}

func identityConflicts(existing, incoming string) bool {
	incoming = strings.TrimSpace(incoming)
	existing = strings.TrimSpace(existing)
	if incoming == "" || existing == "" {
		return false
	}
	if strings.Contains(incoming, "同名不同人") || strings.Contains(incoming, "不是") {
		for token := range identityTokens(existing) {
			if utf8Count(token) >= 2 && strings.Contains(incoming, token) &&
				(strings.Contains(incoming, "不是") || strings.Contains(incoming, "同名")) {
				if strings.Contains(existing, token) && !strings.Contains(incoming[:min(len(incoming), 20)], token) {
					return true
				}
			}
		}
		if strings.Contains(incoming, "不是") {
			after := incoming
			if idx := strings.Index(incoming, "不是"); idx >= 0 {
				after = incoming[idx+len("不是"):]
			}
			for token := range identityTokens(existing) {
				if utf8Count(token) >= 2 && strings.Contains(after, token) {
					return true
				}
			}
		}
	}
	return false
}

func utf8Count(value string) int {
	return len([]rune(value))
}

func upsertAppearance(
	tx *gorm.DB,
	episodeID uint,
	sourceVersion string,
	person models.Person,
	candidate extractedCandidate,
	confirmations []models.PersonUserConfirmation,
) error {
	now := nowUTC()
	appearance := models.EpisodeAppearance{
		SourceVersion:   sourceVersion,
		PersonID:        person.ID,
		EpisodeID:       episodeID,
		Role:            candidate.Role,
		Status:          candidate.Status,
		StatusReason:    candidate.StatusReason,
		EvidenceKind:    candidate.EvidenceKind,
		EvidenceLocator: candidate.EvidenceLocator,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if confirmation := confirmationForPerson(confirmations, person.ID); confirmation != nil {
		if confirmation.Role != "" {
			appearance.Role = confirmation.Role
		}
		appearance.Status = StatusConfirmed
		appearance.StatusReason = "user confirmed"
		appearance.EvidenceKind = "user_confirmation"
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "episode_id"}, {Name: "person_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"source_version", "role", "status", "status_reason", "evidence_kind", "evidence_locator", "updated_at",
		}),
	}).Create(&appearance).Error
}

func replaceAttributions(
	tx *gorm.DB,
	sources EpisodeSources,
	fragments []extractedFragment,
	peopleByName map[string]models.Person,
	confirmations []models.PersonUserConfirmation,
) error {
	now := nowUTC()
	for _, fragment := range fragments {
		var personID *uint
		var appearanceID *uint
		status := fragment.Status
		if fragment.PersonName != "" {
			if person, ok := peopleByName[fragment.PersonName]; ok {
				personID = &person.ID
				var appearance models.EpisodeAppearance
				if err := tx.Where("episode_id = ? AND person_id = ?", sources.EpisodeID, person.ID).
					First(&appearance).Error; err == nil {
					appearanceID = &appearance.ID
				}
			}
		}
		if confirmation := confirmationForFragmentOrder(confirmations, sources.SourceKind, sources.SourceVersion, fragment.Order, fragment.Text, fragment.SpeakerLabel); confirmation != nil {
			personID = confirmation.AssignedPersonID
			if confirmation.AssignedPersonID != nil {
				status = confirmation.Status
				var appearance models.EpisodeAppearance
				if err := tx.Where("episode_id = ? AND person_id = ?", sources.EpisodeID, *confirmation.AssignedPersonID).
					First(&appearance).Error; err == nil {
					appearanceID = &appearance.ID
				}
			} else {
				status = StatusPending
				appearanceID = nil
			}
		}
		row := models.SpeechAttribution{
			AppearanceID:    appearanceID,
			EpisodeID:       sources.EpisodeID,
			PersonID:        personID,
			SourceKind:      sources.SourceKind,
			SourceVersion:   sources.SourceVersion,
			FragmentOrder:   fragment.Order,
			SpeakerLabel:    fragment.SpeakerLabel,
			StartMS:         fragment.StartMS,
			Text:            fragment.Text,
			Status:          status,
			EvidenceKind:    fragment.EvidenceKind,
			EvidenceLocator: fragment.EvidenceLocator,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "episode_id"},
				{Name: "source_kind"},
				{Name: "source_version"},
				{Name: "fragment_order"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"appearance_id", "person_id", "speaker_label", "start_ms", "text",
				"status", "evidence_kind", "evidence_locator", "updated_at",
			}),
		}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureAliases(tx *gorm.DB, personID uint, aliases []string) error {
	for _, alias := range uniqueStrings(aliases) {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "person_id"}, {Name: "alias"}},
			DoNothing: true,
		}).Create(&models.PersonAlias{
			PersonID:  personID,
			Alias:     alias,
			CreatedAt: nowUTC(),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func loadConfirmations(tx *gorm.DB, episodeID uint) ([]models.PersonUserConfirmation, error) {
	var rows []models.PersonUserConfirmation
	if err := tx.Where("episode_id = ?", episodeID).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func upsertConfirmation(tx *gorm.DB, row models.PersonUserConfirmation) error {
	now := nowUTC()
	row.CreatedAt = now
	row.UpdatedAt = now
	if row.PersonID == nil {
		zero := uint(0)
		row.PersonID = &zero
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "episode_id"},
			{Name: "kind"},
			{Name: "source_kind"},
			{Name: "fragment_order"},
			{Name: "person_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"assigned_person_id", "display_name", "identity_note", "role", "speaker_label", "source_version", "source_text", "status", "updated_at",
		}),
	}).Create(&row).Error
}

func confirmationForPerson(rows []models.PersonUserConfirmation, personID uint) *models.PersonUserConfirmation {
	for i := range rows {
		if rows[i].Kind == models.PersonConfirmationKindName && rows[i].PersonID != nil && *rows[i].PersonID == personID {
			return &rows[i]
		}
	}
	return nil
}

func confirmationForFragment(rows []models.PersonUserConfirmation, attribution models.SpeechAttribution) *models.PersonUserConfirmation {
	return confirmationForFragmentOrder(rows, attribution.SourceKind, attribution.SourceVersion, attribution.FragmentOrder, attribution.Text, attribution.SpeakerLabel)
}

func confirmationForFragmentOrder(rows []models.PersonUserConfirmation, sourceKind, sourceVersion string, order int, text string, speaker string) *models.PersonUserConfirmation {
	for i := range rows {
		if rows[i].Kind == models.PersonConfirmationKindAttribution &&
			rows[i].SourceKind == sourceKind &&
			rows[i].SourceVersion == sourceVersion &&
			rows[i].FragmentOrder == order && rows[i].SourceText == text && rows[i].SpeakerLabel == speaker {
			return &rows[i]
		}
	}
	return nil
}

func personAliases(db *gorm.DB, personID uint) []string {
	var rows []models.PersonAlias
	if err := db.Where("person_id = ?", personID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Alias)
	}
	return out
}

func currentAttributionVersion(db *gorm.DB, episodeID uint, sourceKind string) string {
	var artifactCount int64
	if err := db.Model(&models.EpisodeArtifactSet{}).Where("episode_id = ?", episodeID).Count(&artifactCount).Error; err == nil && artifactCount > 0 && sourceKind == SourceTranscript {
		var artifact models.EpisodeArtifactSet
		if err := db.Where("episode_id = ? AND is_current = ?", episodeID, true).First(&artifact).Error; err == nil {
			return fmt.Sprintf("artifact-%d", artifact.ID)
		}
		return "unavailable"
	}
	var version string
	_ = db.Model(&models.SpeechAttribution{}).
		Select("source_version").
		Where("episode_id = ? AND source_kind = ?", episodeID, sourceKind).
		Order("id DESC").
		Limit(1).
		Scan(&version).Error
	return version
}

func newStableKey() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("person-%d", nowUTC().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

func mergeSuggestedCandidates(candidates []extractedCandidate, suggested []SuggestedCandidate) []extractedCandidate {
	for _, item := range suggested {
		if item.MentionedOnly || strings.TrimSpace(item.DisplayName) == "" {
			continue
		}
		found := false
		for i := range candidates {
			if namesEquivalent(candidates[i], item.DisplayName) {
				candidates[i].Aliases = uniqueStrings(append(candidates[i].Aliases, item.Aliases...))
				if item.EvidenceKind == "verified_runtime" {
					candidates[i].Status = StatusConfirmed
					candidates[i].EvidenceKind = item.EvidenceKind
					candidates[i].EvidenceLocator = item.EvidenceLocator
				}
				if candidates[i].IdentityNote == "" {
					candidates[i].IdentityNote = item.IdentityNote
				}
				found = true
				break
			}
		}
		if !found {
			status := StatusPending
			if item.EvidenceKind == "verified_runtime" {
				status = StatusConfirmed
			}
			candidates = append(candidates, extractedCandidate{
				DisplayName:     item.DisplayName,
				Aliases:         item.Aliases,
				IdentityNote:    item.IdentityNote,
				Role:            firstNonEmpty(item.Role, RoleUnknown),
				Status:          status,
				EvidenceKind:    firstNonEmpty(item.EvidenceKind, "runtime_candidate"),
				EvidenceLocator: item.EvidenceLocator,
			})
		}
	}
	return candidates
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var _ Module = (*Service)(nil)

// WithArtifactReader enables explicit preparation of existing successful transcripts.
func (s *Service) WithArtifactReader(reader processing.ArtifactReader) *Service {
	s.reader = reader
	return s
}

func (s *Service) PrepareCurrent(ctx context.Context, episodeID uint) (EpisodePeople, error) {
	if s.reader == nil {
		return EpisodePeople{}, ErrTranscriptRequired
	}
	if err := s.requireEpisode(ctx, episodeID); err != nil {
		return EpisodePeople{}, err
	}
	var artifact models.EpisodeArtifactSet
	if err := s.db.WithContext(ctx).Where("episode_id = ? AND is_current = ?", episodeID, true).First(&artifact).Error; err != nil {
		return EpisodePeople{}, ErrTranscriptRequired
	}
	content, err := s.reader.ReadText(ctx, artifact, "transcript")
	if err != nil {
		return EpisodePeople{}, err
	}
	if len(content.Segments) == 0 {
		return EpisodePeople{}, ErrTranscriptRequired
	}
	var episode models.Episode
	if err := s.db.WithContext(ctx).First(&episode, episodeID).Error; err != nil {
		return EpisodePeople{}, err
	}
	return s.Prepare(ctx, EpisodeSources{EpisodeID: episodeID, SourceVersion: fmt.Sprintf("artifact-%d", artifact.ID), SourceKind: SourceTranscript, ShowNotes: episode.ShowNotes, Segments: SegmentsFromTranscript(content.Segments)})
}
