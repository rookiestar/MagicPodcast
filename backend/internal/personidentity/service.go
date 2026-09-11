package personidentity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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
	if s.suggester == nil {
		return EpisodePeople{}, ErrIdentityUnavailable
	}
	var revision uint
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		revision, err = reservePreparation(tx, sources.EpisodeID)
		return err
	}); err != nil {
		return EpisodePeople{}, err
	}
	metadata, err := readPreparationMetadata(s.db.WithContext(ctx), sources.EpisodeID)
	if err != nil {
		return EpisodePeople{}, err
	}
	sources.PodcastTitle = metadata.PodcastTitle
	sources.PodcastAuthor = metadata.PodcastAuthor
	sources.PodcastDescription = metadata.PodcastDescription
	sources.EpisodeTitle = metadata.EpisodeTitle
	sources.EpisodePublishedDate = ""
	if !metadata.PublishedAt.IsZero() {
		sources.EpisodePublishedDate = metadata.PublishedAt.Format(time.RFC3339)
	}
	sources.ShowNotes = metadata.ShowNotes

	var candidates []extractedCandidate
	fragments := make([]extractedFragment, 0, len(sources.Segments))
	for _, segment := range sources.Segments {
		fragments = append(fragments, extractedFragment{Segment: segment, Status: StatusPending, EvidenceKind: "unmatched_label", EvidenceLocator: fragmentLocator(segment)})
	}
	if s.suggester != nil {
		suggested, err := s.suggester.Suggest(ctx, sources)
		if err != nil {
			return EpisodePeople{}, err
		}
		assignments := map[int][]string{}
		for index, item := range suggested {
			converted := mergeSuggestedCandidates(nil, []SuggestedCandidate{item})
			if len(converted) == 0 {
				continue
			}
			candidate := converted[0]
			candidate.key = fmt.Sprintf("candidate:%d", index)
			candidates = append(candidates, candidate)
			if item.EvidenceKind == "verified_runtime" && item.Status != StatusPending && !item.MentionedOnly {
				for _, order := range item.SpeechOrders {
					assignments[order] = append(assignments[order], candidate.key)
				}
			}
		}
		for i := range fragments {
			if keys := assignments[fragments[i].Order]; len(keys) == 1 {
				fragments[i].PersonKey = keys[0]
				fragments[i].Status = StatusConfirmed
				fragments[i].EvidenceKind = "verified_runtime"
			}
		}
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		confirmations, err := loadConfirmations(tx, sources.EpisodeID)
		if err != nil {
			return err
		}
		var previousPreparation models.PersonPreparation
		if err := tx.First(&previousPreparation, "episode_id = ?", sources.EpisodeID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		retainRoles := previousPreparation.AlgorithmVersion == models.CurrentIdentityAlgorithm &&
			previousPreparation.SourceVersion == sources.SourceVersion && previousPreparation.MetadataDigest == metadata.digest()
		peopleByKey := map[string]models.Person{}
		claimedPeople := map[uint]bool{}
		for _, candidate := range candidates {
			if candidate.MentionedOnly {
				continue
			}
			person, err := upsertPerson(tx, sources.EpisodeID, candidate, claimedPeople)
			if err != nil {
				return err
			}
			peopleByKey[candidate.key] = person
			claimedPeople[person.ID] = true
			if err := upsertAppearance(tx, sources.EpisodeID, sources, person, candidate, confirmations, retainRoles); err != nil {
				return err
			}
		}
		retained := make([]uint, 0, len(peopleByKey)+len(confirmations))
		overrides, err := loadAppearanceOverrides(tx, sources.EpisodeID)
		if err != nil {
			return err
		}
		for id := range overrides {
			retained = append(retained, id)
			present := false
			for _, person := range peopleByKey {
				if person.ID == id {
					present = true
					break
				}
			}
			if !present && confirmationForPerson(confirmations, id) == nil {
				// A preserved role or exclusion is not a fresh identity confirmation.
				if err := tx.Model(&models.EpisodeAppearance{}).Where("episode_id = ? AND person_id = ?", sources.EpisodeID, id).Updates(map[string]any{"status": StatusPending, "status_reason": "本次未确认出场身份", "source_version": sources.SourceVersion}).Error; err != nil {
					return err
				}
			}
		}

		for _, person := range peopleByKey {
			retained = append(retained, person.ID)
		}
		for _, confirmation := range confirmations {
			if confirmation.Kind == models.PersonConfirmationKindName && confirmation.PersonID != nil {
				retained = append(retained, *confirmation.PersonID)
			}
			if confirmation.Kind == models.PersonConfirmationKindAttribution && confirmation.AssignedPersonID != nil {
				for _, fragment := range fragments {
					if confirmationForFragmentOrder([]models.PersonUserConfirmation{confirmation}, sources.SourceKind, sources.SourceVersion, fragment.Order, fragment.Text, fragment.SpeakerLabel) != nil {
						retained = append(retained, *confirmation.AssignedPersonID)
					}
				}
			}
		}
		retired := tx.Where("episode_id = ? AND source_version = ?", sources.EpisodeID, sources.SourceVersion)
		if len(retained) > 0 {
			retired = retired.Where("person_id NOT IN ?", retained)
		}
		if err := retired.Delete(&models.EpisodeAppearance{}).Error; err != nil {
			return err
		}
		if err := replaceAttributions(tx, sources, fragments, peopleByKey, confirmations); err != nil {
			return err
		}
		if err := publishPreparation(tx, sources.EpisodeID, sources.SourceVersion, metadata, revision); err != nil {
			return err
		}
		return s.reindexSearchTransaction(ctx, tx, sources.EpisodeID)
	})
	if err != nil {
		return EpisodePeople{}, err
	}
	return s.ListEpisodePeople(ctx, sources.EpisodeID)
}

func (s *Service) ListEpisodePeople(ctx context.Context, episodeID uint) (EpisodePeople, error) {
	var result EpisodePeople
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reader := *s
		reader.db = tx
		var err error
		result, err = reader.listEpisodePeople(ctx, episodeID)
		return err
	})
	return result, err
}

func (s *Service) listEpisodePeople(ctx context.Context, episodeID uint) (EpisodePeople, error) {
	if err := s.requireEpisode(ctx, episodeID); err != nil {
		return EpisodePeople{}, err
	}
	var episode models.Episode
	if err := s.db.WithContext(ctx).Select("id", "published_date").First(&episode, episodeID).Error; err != nil {
		return EpisodePeople{}, fmt.Errorf("load episode: %w", err)
	}
	currentVersion := currentAttributionVersion(s.db.WithContext(ctx), episodeID, SourceTranscript)
	var state models.PersonPreparation
	if err := s.db.WithContext(ctx).First(&state, "episode_id = ?", episodeID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return EpisodePeople{}, err
	}
	prepared, prepErr := s.preparationCurrent(ctx, episodeID, currentVersion)
	if prepErr != nil {
		return EpisodePeople{}, prepErr
	}
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
	overrides, err := loadAppearanceOverrides(s.db.WithContext(ctx), episodeID)
	if err != nil {
		return EpisodePeople{}, err
	}
	excludedPeople := make([]PersonView, 0)
	people := make([]PersonView, 0, len(appearances))
	for _, appearance := range appearances {
		if !prepared && confirmationForPerson(confirmations, appearance.PersonID) == nil && !overrides[appearance.PersonID].Excluded {
			continue
		}
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
		override := overrides[appearance.PersonID]
		if !prepared {
			view.Role = RoleUnknown
		}
		if override.Role != nil {
			view.Role = *override.Role
			view.RoleUserConfirmed = true
		}
		if override.Excluded {
			excludedPeople = append(excludedPeople, view)
			continue
		}
		for _, attribution := range attributions {
			if attribution.PersonID != nil && *attribution.PersonID == person.ID {
				status := attribution.Status
				if !prepared && confirmationForFragment(confirmations, attribution) == nil {
					status = StatusPending
				}
				if status == StatusConfirmed {
					view.ConfirmedSpeech++
				}
				if status == StatusPending {
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
		if !prepared && !view.UserConfirmed {
			view.PersonID = nil
			view.Status = StatusPending
		}
		if view.PersonID != nil && overrides[*view.PersonID].Excluded {
			view.PersonID = nil
			view.Status = StatusRejected
		}
		if view.PersonID != nil {
			person, err := s.loadPerson(ctx, *attribution.PersonID)
			if err == nil {
				view.DisplayName = person.DisplayName
			}
		}
		attributionViews = append(attributionViews, view)
	}
	indexReady := prepared && len(attributions) > 0
	if s.search != nil {
		var indexed int64
		if err := s.db.WithContext(ctx).Model(&models.ContentSearchCoverage{}).Where("episode_id = ? AND source_kind = ? AND source_version = ? AND complete = ?", episodeID, SourceTranscript, currentVersion, true).Count(&indexed).Error; err != nil {
			return EpisodePeople{}, err
		}
		indexReady = indexReady && indexed > 0
	}
	preparationState := "required"
	if currentVersion == "" {
		preparationState = "no_transcript"
	} else if prepared {
		preparationState = "ready"
	} else if len(appearances) > 0 || state.Revision > 0 {
		preparationState = "outdated"
	}
	return EpisodePeople{
		PreparationState:    preparationState,
		preparationRevision: state.Revision,
		publishedRevision:   state.PublishedRevision,
		EpisodeID:           episodeID,
		SourceVersion:       currentVersion,
		IndexReady:          indexReady,
		PublishedAt:         episode.PublishedDate,
		People:              people,
		ExcludedPeople:      excludedPeople,
		Attributions:        attributionViews,
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
		if strings.TrimSpace(correction.DisplayName) != person.DisplayName ||
			(correction.IdentityNote != "" && strings.TrimSpace(correction.IdentityNote) != person.IdentityNote) || correction.Aliases != nil {
			isolated, err := isolateEpisodePerson(tx, episodeID, person)
			if err != nil {
				return err
			}
			person = isolated
			appearance.PersonID = person.ID
		}
		sourceName := person.DisplayName
		var prior models.PersonUserConfirmation
		if err := tx.Where("episode_id = ? AND person_id = ? AND kind = ?", episodeID, person.ID, models.PersonConfirmationKindName).First(&prior).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if prior.SourceText != "" {
			sourceName = prior.SourceText
		}
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
			for _, alias := range uniqueStrings(correction.Aliases) {
				if err := tx.Create(&models.PersonAlias{
					PersonID:  person.ID,
					Alias:     alias,
					CreatedAt: nowUTC(),
				}).Error; err != nil {
					return err
				}
			}
		}
		appearance.Status = StatusConfirmed
		appearance.StatusReason = "user confirmed name"
		appearance.UpdatedAt = nowUTC()
		if err := tx.Save(&appearance).Error; err != nil {
			return err
		}
		if err := upsertConfirmation(tx, models.PersonUserConfirmation{
			EpisodeID:    episodeID,
			Kind:         models.PersonConfirmationKindName,
			SourceText:   sourceName,
			PersonID:     &person.ID,
			DisplayName:  person.DisplayName,
			IdentityNote: person.IdentityNote,
		}); err != nil {
			return err
		}
		return s.reindexSearchTransaction(ctx, tx, episodeID)
	})
	if err != nil {
		return EpisodePeople{}, err
	}
	return s.ListEpisodePeople(ctx, episodeID)
}

// A user correction is episode-scoped. Shared identities must be separated
// before a conflicting name/description/alias change, so other episodes and
// their manual facts retain the original person. Unshared IDs remain stable.
func isolateEpisodePerson(tx *gorm.DB, episodeID uint, original models.Person) (models.Person, error) {
	var otherAppearances int64
	if err := tx.Model(&models.EpisodeAppearance{}).Where("person_id = ? AND episode_id <> ?", original.ID, episodeID).Count(&otherAppearances).Error; err != nil {
		return models.Person{}, err
	}
	if otherAppearances == 0 {
		return original, nil
	}
	copy := original
	copy.ID = 0
	copy.StableKey = newStableKey()
	copy.CreatedAt = nowUTC()
	copy.UpdatedAt = copy.CreatedAt
	if err := tx.Create(&copy).Error; err != nil {
		return models.Person{}, err
	}
	if err := ensureAliases(tx, copy.ID, personAliases(tx, original.ID)); err != nil {
		return models.Person{}, err
	}
	for _, update := range []struct {
		model  any
		column string
	}{
		{&models.SpeechAttribution{}, "person_id"},
		{&models.PersonUserConfirmation{}, "person_id"},
		{&models.PersonUserConfirmation{}, "assigned_person_id"},
		{&models.PersonAppearanceOverride{}, "person_id"},
	} {
		if err := tx.Model(update.model).Where("episode_id = ? AND "+update.column+" = ?", episodeID, original.ID).Update(update.column, copy.ID).Error; err != nil {
			return models.Person{}, err
		}
	}
	return copy, nil
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
		if err := upsertConfirmation(tx, models.PersonUserConfirmation{
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
		}); err != nil {
			return err
		}
		return s.reindexSearchTransaction(ctx, tx, episodeID)
	})
	if err != nil {
		return EpisodePeople{}, err
	}
	return s.ListEpisodePeople(ctx, episodeID)
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

// Identity facts and their search copies share SQLite, so publish them in one
// transaction rather than acknowledging facts whose index failed to update.
func (s *Service) reindexSearchTransaction(ctx context.Context, tx *gorm.DB, episodeID uint) error {
	if s.search == nil {
		return nil
	}
	binder, ok := s.search.(interface {
		WithTransaction(*gorm.DB) contentsearch.Module
	})
	if !ok {
		return fmt.Errorf("identity indexing requires a transactional content search module")
	}
	writer := *s
	writer.db = tx
	writer.search = binder.WithTransaction(tx)
	listed, err := writer.listEpisodePeople(ctx, episodeID)
	if err != nil {
		return err
	}
	return writer.reindexSearch(ctx, episodeID, listed)
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
		AttributionVersion: &contentsearch.AttributionVersion{Revision: listed.preparationRevision, PublishedRevision: listed.publishedRevision},
		EpisodeID:          episodeID,
		PublishedAt:        episode.PublishedDate,
		ShowNotes:          episode.ShowNotes,
		SourceKind:         SourceTranscript,
		SourceVersion:      listed.SourceVersion,
		Fragments:          fragments,
		Complete:           true,
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
	listed, err := s.ListEpisodePeople(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	knownPeople := map[uint]bool{}
	for _, person := range listed.People {
		knownPeople[person.ID] = person.Status == StatusConfirmed
	}
	effectiveAttributions := map[uint]AttributionView{}
	for _, attribution := range listed.Attributions {
		effectiveAttributions[attribution.ID] = attribution
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
		if row.SourceKind == SourceTranscript && current[row.SourceKind] == row.SourceVersion {
			effective, ok := effectiveAttributions[row.ID]
			if row.Status == StatusConfirmed && (!ok || effective.Status != StatusConfirmed || row.PersonID == nil || !knownPeople[*row.PersonID]) {
				row.Status = StatusPending
			}
		}
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

func upsertPerson(tx *gorm.DB, episodeID uint, candidate extractedCandidate, claimed map[uint]bool) (models.Person, error) {
	// A source misspelling may be shared by two distinct canonical participants.
	// Never reuse the identity already assigned to another proposal in this run.
	if person, ok := findExistingPerson(tx, episodeID, candidate); ok && !claimed[person.ID] {
		changed := false
		if person.DisplayName != candidate.DisplayName && candidate.Status == StatusConfirmed {
			sourceMatched := false
			for _, spelling := range candidate.SourceNames {
				if spelling == person.DisplayName {
					sourceMatched = true
				}
			}
			var manual int64
			if err := tx.Model(&models.PersonUserConfirmation{}).Where("episode_id = ? AND kind = ? AND person_id = ?", episodeID, models.PersonConfirmationKindName, person.ID).Count(&manual).Error; err != nil {
				return models.Person{}, err
			}
			if sourceMatched && manual == 0 {
				originalID := person.ID
				isolated, err := isolateEpisodePerson(tx, episodeID, person)
				if err != nil {
					return models.Person{}, err
				}
				person = isolated
				if person.ID != originalID {
					if err := tx.Model(&models.EpisodeAppearance{}).Where("episode_id = ? AND person_id = ?", episodeID, originalID).Update("person_id", person.ID).Error; err != nil {
						return models.Person{}, err
					}
				}
				person.DisplayName = candidate.DisplayName
				changed = true
			}
		}
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
	if len(candidate.SourceNames) > 0 && candidate.Status == StatusConfirmed {
		var localSpellings []models.Person
		if err := tx.Table("people").Select("people.*").Joins("JOIN episode_appearances a ON a.person_id = people.id").Where("a.episode_id = ? AND people.display_name IN ?", episodeID, candidate.SourceNames).Find(&localSpellings).Error; err == nil {
			people = append(people, localSpellings...)
		}
	}
	// A corrected source spelling remains an episode-local lookup, never an
	// implicit global alias. Explicit user aliases still use person_aliases.
	var locallyCorrected []models.Person
	lookupNames := append(append([]string{}, names...), candidate.SourceNames...)
	if err := tx.Table("people").Select("people.*").Joins("JOIN person_user_confirmations c ON c.person_id = people.id").Where("c.episode_id = ? AND c.kind = ? AND c.source_text IN ?", episodeID, models.PersonConfirmationKindName, lookupNames).Find(&locallyCorrected).Error; err == nil {
		people = append(people, locallyCorrected...)
	}
	seen := map[uint]models.Person{}
	for _, person := range people {
		seen[person.ID] = person
	}
	compatible := make([]models.Person, 0)
	local := make([]models.Person, 0)
	for _, person := range seen {
		if identityConflicts(person.IdentityNote, candidate.IdentityNote) {
			continue
		}
		var appearances int64
		if err := tx.Model(&models.EpisodeAppearance{}).Where("episode_id = ? AND person_id = ?", episodeID, person.ID).Count(&appearances).Error; err != nil {
			continue
		}
		if appearances > 0 {
			local = append(local, person)
		} else if matchingIdentityAnchor(tx, person, candidate, 0) {
			compatible = append(compatible, person)
		}
	}
	// Prefer the exact existing canonical name over another person's matching
	// transcript spelling, keeping a repeated preparation stable after separation.
	exact := make([]models.Person, 0)
	for _, person := range local {
		if person.DisplayName == candidate.DisplayName {
			exact = append(exact, person)
		}
	}
	if len(exact) == 1 {
		return exact[0], true
	}
	if len(local) == 1 {
		return local[0], true
	}
	if len(local) > 1 {
		anchored := make([]models.Person, 0)
		for _, person := range local {
			if matchingIdentityAnchor(tx, person, candidate, episodeID) {
				anchored = append(anchored, person)
			}
		}
		if len(anchored) == 1 {
			return anchored[0], true
		}
		return models.Person{}, false
	}
	if len(compatible) == 1 {
		return compatible[0], true
	}

	return models.Person{}, false
}

// Descriptions and transcript spellings are not cross-episode identity keys.
// Match only canonical names with an independently evidenced distinctive anchor
// on both currently prepared appearances. Ambiguous multiple matches stay split.
func matchingIdentityAnchor(tx *gorm.DB, person models.Person, candidate extractedCandidate, localEpisodeID uint) bool {
	if person.DisplayName != candidate.DisplayName || candidate.Status != StatusConfirmed {
		return false
	}
	var incoming identityProposal
	if json.Unmarshal([]byte(candidate.EvidenceLocator), &incoming) != nil || incoming.NameType != "canonical" || incoming.IdentityAnchor.Key == "" {
		return false
	}
	var appearances []models.EpisodeAppearance
	query := tx.Where("episode_appearances.person_id = ? AND episode_appearances.status = ?", person.ID, StatusConfirmed)
	if localEpisodeID != 0 {
		// Match an independently re-verified local identity across preparation
		// upgrades so its manual exclusion survives. This does not reuse speech
		// confirmations or provide cross-episode identity evidence.
		query = query.Where("episode_appearances.episode_id = ?", localEpisodeID)
	} else {
		query = query.Joins("JOIN person_preparations pp ON pp.episode_id = episode_appearances.episode_id AND pp.source_version = episode_appearances.source_version AND pp.algorithm_version = ?", models.CurrentIdentityAlgorithm).
			Where("NOT EXISTS (SELECT 1 FROM person_appearance_overrides po WHERE po.episode_id = episode_appearances.episode_id AND po.person_id = episode_appearances.person_id AND po.excluded = 1)")
	}
	if err := query.Find(&appearances).Error; err != nil {
		return false
	}
	for _, appearance := range appearances {
		if localEpisodeID == 0 && currentAttributionVersion(tx, appearance.EpisodeID, SourceTranscript) != appearance.SourceVersion {
			continue
		}
		var previous identityProposal
		if json.Unmarshal([]byte(appearance.EvidenceLocator), &previous) != nil {
			continue
		}
		if previous.NameType == "canonical" && previous.IdentityAnchor.Key == incoming.IdentityAnchor.Key && previous.IdentityAnchor.Kind == incoming.IdentityAnchor.Kind {
			return true
		}
	}
	return false
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
	sources EpisodeSources,
	person models.Person,
	candidate extractedCandidate,
	confirmations []models.PersonUserConfirmation,
	retainRoles bool,
) error {
	now := nowUTC()
	sourceVersion := sources.SourceVersion
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
	if retainRoles {
		var previous models.EpisodeAppearance
		err := tx.Where("episode_id = ? AND person_id = ? AND source_version = ?", episodeID, person.ID, sourceVersion).First(&previous).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			if err := retainCurrentRoleEvidence(&appearance, previous, sources); err != nil {
				return err
			}
		}
	}
	if confirmation := confirmationForPerson(confirmations, person.ID); confirmation != nil {
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

// Only unchanged, current preparations reach here. Missing evidence cannot
// displace a supported role; contradictory automatic roles remain unresolved
// until sources change or an independent user override supplies the decision.
func retainCurrentRoleEvidence(current *models.EpisodeAppearance, previous models.EpisodeAppearance, sources EpisodeSources) error {
	var oldProof, newProof identityProposal
	if json.Unmarshal([]byte(previous.EvidenceLocator), &oldProof) != nil || json.Unmarshal([]byte(current.EvidenceLocator), &newProof) != nil {
		return nil
	}
	if len(oldProof.RoleConflicts) > 0 {
		for _, conflict := range oldProof.RoleConflicts {
			if !roleEvidenceStillPresent(conflict.Evidence, sources) {
				return nil
			}
		}
		newProof.RoleConflicts = oldProof.RoleConflicts
	} else if (previous.Role == RoleHost || previous.Role == RoleGuest) && roleEvidenceStillPresent(oldProof.RoleEvidence, sources) {
		if current.Role == RoleUnknown || current.Role == "" {
			current.Role = previous.Role
			newProof.Role = previous.Role
			newProof.RoleEvidence = oldProof.RoleEvidence
		} else if current.Role != previous.Role && newProof.RoleEvidence.Quote != "" {
			newProof.RoleConflicts = []roleConflict{{Role: previous.Role, Evidence: oldProof.RoleEvidence}, {Role: current.Role, Evidence: newProof.RoleEvidence}}
		} else {
			return nil
		}
	} else {
		return nil
	}
	if len(newProof.RoleConflicts) > 0 {
		current.Role = RoleUnknown
		newProof.Role = RoleUnknown
		newProof.RoleEvidence = identityEvidence{}
		current.StatusReason = "本集自动角色依据存在分歧，请核对角色"
	}
	raw, err := json.Marshal(newProof)
	if err != nil {
		return err
	}
	current.EvidenceLocator = string(raw)
	return nil
}

func roleEvidenceStillPresent(evidence identityEvidence, sources EpisodeSources) bool {
	quote := normalizeIdentityEvidence(evidence.Quote)
	if quote == "" {
		return false
	}
	switch evidence.Source {
	case SourceShowNotes:
		return evidence.Fragment == 0 && strings.Contains(normalizeIdentityEvidence(sources.ShowNotes), quote)
	case SourceTranscript:
		for _, fragment := range sources.Segments {
			if fragment.Order == evidence.Fragment {
				return strings.Contains(normalizeIdentityEvidence(fragment.Text), quote)
			}
		}
	}
	return false
}

func replaceAttributions(
	tx *gorm.DB,
	sources EpisodeSources,
	fragments []extractedFragment,
	peopleByKey map[string]models.Person,
	confirmations []models.PersonUserConfirmation,
) error {
	// Replace only the current input's derived rows; confirmation history remains
	// independent and is reapplied only when the full source identity matches.
	orders := make([]int, 0, len(fragments))
	for _, fragment := range fragments {
		orders = append(orders, fragment.Order)
	}
	stale := tx.Where("episode_id = ? AND source_kind = ? AND source_version = ?", sources.EpisodeID, sources.SourceKind, sources.SourceVersion)
	if len(orders) > 0 {
		stale = stale.Where("fragment_order NOT IN ?", orders)
	}
	if err := stale.Delete(&models.SpeechAttribution{}).Error; err != nil {
		return err
	}

	now := nowUTC()
	for _, fragment := range fragments {
		var personID *uint
		var appearanceID *uint
		status := fragment.Status
		if fragment.PersonKey != "" {
			if person, ok := peopleByKey[fragment.PersonKey]; ok {
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
			status = confirmation.Status
			if confirmation.AssignedPersonID != nil {
				var appearance models.EpisodeAppearance
				if err := tx.Where("episode_id = ? AND person_id = ?", sources.EpisodeID, *confirmation.AssignedPersonID).
					First(&appearance).Error; err == nil {
					appearanceID = &appearance.ID
				}
			} else {
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
	if err := tx.Clauses(clause.OnConflict{
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
	}).Create(&row).Error; err != nil {
		return err
	}
	_, err := reservePreparation(tx, row.EpisodeID)
	return err
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
	if sourceKind == SourceTranscript && strings.HasPrefix(version, "artifact-") {
		return "unavailable"
	}
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
			if item.EvidenceKind == "verified_runtime" && item.Status != StatusPending {
				status = StatusConfirmed
			}
			candidates = append(candidates, extractedCandidate{
				SourceNames:     item.SourceNames,
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
