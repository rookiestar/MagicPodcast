package personidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"magicpodcast/internal/models"
)

type ReviewMatch struct {
	Relation        *SpeakerRelation `json:"relation,omitempty"`
	Choice          string           `json:"choice,omitempty"`
	RoleEdited      bool             `json:"role_edited"`
	SuggestedStatus string           `json:"suggested_status,omitempty"`
	SourceNames     []string         `json:"source_names,omitempty"`
	Applied         bool             `json:"applied"`
	OriginalName    string           `json:"original_name,omitempty"`
	IdentityNote    string           `json:"identity_note,omitempty"`
	Aliases         []string         `json:"aliases,omitempty"`
	Key             string           `json:"key"`
	PersonID        uint             `json:"person_id,omitempty"`
	DisplayName     string           `json:"display_name"`
	Role            string           `json:"role"`
	SpeakerLabel    string           `json:"speaker_label"`
	Orders          []int            `json:"orders"`
	Selected        bool             `json:"selected"`
	EvidenceLocator string           `json:"evidence_locator,omitempty"`
	Uncertain       bool             `json:"uncertain"`
}
type ReviewDraft struct {
	ID            uint          `json:"id"`
	Revision      uint          `json:"revision"`
	SourceVersion string        `json:"source_version"`
	Matches       []ReviewMatch `json:"matches"`
	Outdated      bool          `json:"outdated"`
}
type ReviewRequest struct {
	DraftID       uint          `json:"draft_id"`
	Revision      uint          `json:"revision"`
	SourceVersion string        `json:"source_version"`
	Matches       []ReviewMatch `json:"matches"`
}
type ManualMatch struct {
	Revision      uint   `json:"revision"`
	SourceVersion string `json:"source_version"`
	FragmentOrder int    `json:"fragment_order"`
	Scope         string `json:"scope"`
	PersonID      uint   `json:"person_id"`
	DisplayName   string `json:"display_name"`
	Clear         bool   `json:"clear"`
}

func (s *Service) latestDraft(ctx context.Context, episodeID uint) (*ReviewDraft, error) {
	var row models.PersonDraft
	err := s.db.WithContext(ctx).Where("episode_id = ?", episodeID).Order("revision DESC, id DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.draftView(ctx, row)
}
func (s *Service) draftView(ctx context.Context, row models.PersonDraft) (*ReviewDraft, error) {
	var matches []ReviewMatch
	if err := json.Unmarshal([]byte(row.Matches), &matches); err != nil {
		return nil, err
	}
	metadata, err := readPreparationMetadata(s.db.WithContext(ctx), row.EpisodeID)
	if err != nil {
		return nil, err
	}
	return &ReviewDraft{ID: row.ID, Revision: row.Revision, SourceVersion: row.SourceVersion, Matches: matches,
		Outdated: currentAttributionVersion(s.db.WithContext(ctx), row.EpisodeID, SourceTranscript) != row.SourceVersion || metadata.digest() != row.MetadataDigest}, nil
}
func (s *Service) ReviewHistory(ctx context.Context, episodeID uint) ([]ReviewDraft, error) {
	if err := s.requireEpisode(ctx, episodeID); err != nil {
		return nil, err
	}
	var rows []models.PersonDraft
	if err := s.db.WithContext(ctx).Where("episode_id = ?", episodeID).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ReviewDraft, 0, len(rows))
	for _, row := range rows {
		v, err := s.draftView(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}
func (s *Service) saveSuggestion(ctx context.Context, src EpisodeSources, metadata preparationMetadata, revision uint, candidates []extractedCandidate, fragments []extractedFragment, proposed []ReviewMatch) (EpisodePeople, error) {
	matches := make([]ReviewMatch, 0)
	for _, candidate := range candidates {
		bySpeaker := map[string][]int{}
		for _, f := range fragments {
			if f.PersonKey == candidate.key {
				bySpeaker[f.SpeakerLabel] = append(bySpeaker[f.SpeakerLabel], f.Order)
			}
		}
		// Keep uncertain identities reviewable without inventing speech attribution.
		if len(bySpeaker) == 0 {
			bySpeaker[""] = []int{}
		}
		for speaker, orders := range bySpeaker {
			matches = append(matches, ReviewMatch{Key: fmt.Sprintf("%s:%s", candidate.key, speaker), DisplayName: candidate.DisplayName, OriginalName: candidate.DisplayName, SuggestedStatus: candidate.Status, SourceNames: candidate.SourceNames, IdentityNote: candidate.IdentityNote, Aliases: candidate.Aliases, Role: candidate.Role,
				SpeakerLabel: speaker, Orders: orders, Selected: len(orders) > 0, Uncertain: len(orders) == 0 || candidate.Status != StatusConfirmed, EvidenceLocator: candidate.EvidenceLocator})
		}
	}
	if proposed != nil {
		matches = proposed
	}
	raw, err := json.Marshal(matches)
	if err != nil {
		return EpisodePeople{}, err
	}
	source, err := json.Marshal(src)
	if err != nil {
		return EpisodePeople{}, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validatePreparationSource(tx, src.EpisodeID, src.SourceVersion, metadata); err != nil {
			return err
		}
		var state models.PersonPreparation
		if err := tx.First(&state, "episode_id = ?", src.EpisodeID).Error; err != nil {
			return err
		}
		if state.Revision != revision {
			return ErrSourcesChanged
		}
		return tx.Create(&models.PersonDraft{EpisodeID: src.EpisodeID, Revision: revision, SourceVersion: src.SourceVersion, MetadataDigest: metadata.digest(), Sources: string(source), Matches: string(raw), UpdatedAt: nowUTC()}).Error
	})
	if err != nil {
		return EpisodePeople{}, err
	}
	return s.ListEpisodePeople(ctx, src.EpisodeID)
}

// Review modifies a persisted draft, or applies its selected matches in one fact/index transaction.
func (s *Service) Review(ctx context.Context, episodeID uint, request ReviewRequest, apply bool) (EpisodePeople, error) {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return EpisodePeople{}, err
	}
	var copied ReviewRequest
	if err := json.Unmarshal(requestBytes, &copied); err != nil {
		return EpisodePeople{}, err
	}

	request = copied
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row models.PersonDraft
		if err := tx.Where("id = ? AND episode_id = ?", request.DraftID, episodeID).First(&row).Error; err != nil {
			return ErrInvalidCorrection
		}
		var state models.PersonPreparation
		if err := tx.First(&state, "episode_id = ?", episodeID).Error; err != nil {
			return err
		}
		// A retry of the exact applied draft is harmless, provided no newer write occurred.
		if apply && row.LastAppliedRevision == request.Revision && row.Revision == state.Revision && row.AppliedRequest == string(requestBytes) {
			return nil
		}
		if request.Revision != state.Revision {
			return ErrSourcesChanged
		}
		metadata, err := readPreparationMetadata(tx, episodeID)
		if err != nil {
			return err
		}
		if row.MetadataDigest != metadata.digest() || request.SourceVersion != row.SourceVersion {
			return ErrSourcesChanged
		}
		if err := validatePreparationSource(tx, episodeID, row.SourceVersion, metadata); err != nil {
			return err
		}
		var src EpisodeSources
		if err := json.Unmarshal([]byte(row.Sources), &src); err != nil {
			return err
		}
		var stored []ReviewMatch
		if err := json.Unmarshal([]byte(row.Matches), &stored); err != nil {
			return err
		}
		if err := resolveSpeakerChoices(request.Matches, stored); err != nil {
			return err
		}
		if err := validateReview(request.Matches, stored, src); err != nil {
			return err
		}
		revision, err := reservePreparation(tx, episodeID)
		if err != nil {
			return err
		}
		if apply {
			if err := s.applyMatches(ctx, tx, src, request.Matches); err != nil {
				return err
			}
			var publishedState models.PersonPreparation
			if err := tx.First(&publishedState, "episode_id = ?", episodeID).Error; err != nil {
				return err
			}
			revision = publishedState.Revision
			if err := publishPreparation(tx, episodeID, src.SourceVersion, metadata, revision); err != nil {
				return err
			}
			row.LastAppliedRevision = request.Revision
			row.AppliedRequest = string(requestBytes)
			for i := range request.Matches {
				if request.Matches[i].Selected {
					request.Matches[i].Selected = false
					request.Matches[i].Applied = true
				}
			}
		}
		raw, err := json.Marshal(request.Matches)
		if err != nil {
			return err
		}
		row.Matches, row.Revision, row.UpdatedAt = string(raw), revision, nowUTC()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if apply {
			return s.reindexSearchTransaction(ctx, tx, episodeID)
		}
		return nil
	})
	if err != nil {
		return EpisodePeople{}, err
	}
	return s.ListEpisodePeople(ctx, episodeID)
}
func validateReview(matches, stored []ReviewMatch, src EpisodeSources) error {
	if len(matches) != len(stored) {
		return ErrInvalidCorrection
	}
	originals := map[string]ReviewMatch{}
	for _, item := range stored {
		originals[item.Key] = item
	}
	segments := map[int]Segment{}
	for _, seg := range src.Segments {
		segments[seg.Order] = seg
	}
	keys, selectedOrders := map[string]bool{}, map[int]bool{}
	for _, item := range matches {
		original, ok := originals[item.Key]
		if !ok || keys[item.Key] {
			return ErrInvalidCorrection
		}
		// New relation identity fields were derived from the immutable candidate
		// above. Historical drafts retain their original field equality contract.
		if item.Relation == nil && (item.EvidenceLocator != original.EvidenceLocator || item.OriginalName != original.OriginalName || item.IdentityNote != original.IdentityNote || item.SuggestedStatus != original.SuggestedStatus || strings.Join(item.SourceNames, "\n") != strings.Join(original.SourceNames, "\n") || strings.Join(item.Aliases, "\n") != strings.Join(original.Aliases, "\n")) {
			return ErrInvalidCorrection
		}
		keys[item.Key] = true
		if strings.TrimSpace(item.DisplayName) == "" || len([]rune(item.DisplayName)) > 200 || (item.Role != RoleHost && item.Role != RoleGuest && item.Role != RoleUnknown) {
			return ErrInvalidCorrection
		}
		seen := map[int]bool{}
		for _, order := range item.Orders {
			seg, ok := segments[order]
			if !ok || seg.SpeakerLabel != item.SpeakerLabel || seen[order] {
				return ErrInvalidCorrection
			}
			seen[order] = true
			if item.Selected {
				if selectedOrders[order] {
					return ErrInvalidCorrection
				}
				selectedOrders[order] = true
			}
		}
	}
	return nil
}
func (s *Service) applyMatches(ctx context.Context, tx *gorm.DB, src EpisodeSources, matches []ReviewMatch) error {
	// Initialise raw fragments without replacing previously applied matches.
	for _, seg := range src.Segments {
		row := models.SpeechAttribution{EpisodeID: src.EpisodeID, SourceKind: SourceTranscript, SourceVersion: src.SourceVersion, FragmentOrder: seg.Order, SpeakerLabel: seg.SpeakerLabel, StartMS: seg.StartMS, Text: seg.Text, Status: StatusPending, CreatedAt: nowUTC(), UpdatedAt: nowUTC()}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	claimed := map[uint]bool{}
	grouped := map[string]uint{}
	// A review can apply only part of a draft. Retain identities from earlier
	// applications so a later group shares its candidate's corrected identity,
	// while distinct candidates cannot reuse it through name-only lookup.
	for _, match := range matches {
		if match.Relation != nil && match.PersonID != 0 {
			claimed[match.PersonID] = true
			if match.Choice != "" && match.Choice != "manual" {
				grouped["relation:"+match.Choice+":"+strings.TrimSpace(match.DisplayName)] = match.PersonID
			}
		}
	}
	for matchIndex, match := range matches {
		if !match.Selected {
			continue
		}
		groupKey := ""
		if match.Relation != nil && match.Choice != "" && match.Choice != "manual" {
			groupKey = "relation:" + match.Choice + ":" + strings.TrimSpace(match.DisplayName)
		}
		parts := strings.SplitN(match.Key, ":", 3)
		if len(parts) == 3 && parts[0] == "candidate" {
			groupKey = parts[0] + ":" + parts[1] + ":" + strings.TrimSpace(match.DisplayName)
		}
		if match.PersonID == 0 && groupKey != "" {
			match.PersonID = grouped[groupKey]
		}
		person := models.Person{}
		var err error
		if match.PersonID != 0 {
			var appearance models.EpisodeAppearance
			if err = tx.Where("episode_id = ? AND person_id = ? AND source_version = ?", src.EpisodeID, match.PersonID, src.SourceVersion).First(&appearance).Error; err != nil {
				return ErrNotEpisodeParticipant
			}
			if err = tx.First(&person, match.PersonID).Error; err != nil {
				return err
			}

			if person.DisplayName != strings.TrimSpace(match.DisplayName) {
				// A scoped rename must not rename the other fragments sharing this person.
				person = models.Person{StableKey: newStableKey(), DisplayName: strings.TrimSpace(match.DisplayName), CreatedAt: nowUTC(), UpdatedAt: nowUTC()}
				if err = tx.Create(&person).Error; err != nil {
					return err
				}
			}
		} else {
			// Retain an evidence-backed identity anchor only when its proposed name is unchanged.
			evidence := match.EvidenceLocator
			if match.OriginalName != strings.TrimSpace(match.DisplayName) {
				evidence = ""
			}
			candidate := extractedCandidate{SourceNames: match.SourceNames, IdentityNote: match.IdentityNote, Aliases: match.Aliases, DisplayName: strings.TrimSpace(match.DisplayName), Role: match.Role, Status: StatusConfirmed, EvidenceKind: "user_confirmation", EvidenceLocator: evidence}
			if match.OriginalName == "" || strings.TrimSpace(match.DisplayName) != match.OriginalName {
				person = models.Person{StableKey: newStableKey(), DisplayName: candidate.DisplayName, CreatedAt: nowUTC(), UpdatedAt: nowUTC()}
				err = tx.Create(&person).Error
			} else {
				person, err = upsertPerson(tx, src.EpisodeID, candidate, claimed)
			}
			if err != nil {
				return err
			}
		}
		if groupKey != "" {
			grouped[groupKey] = person.ID
		}
		matches[matchIndex].PersonID = person.ID
		claimed[person.ID] = true
		appearance := models.EpisodeAppearance{EpisodeID: src.EpisodeID, PersonID: person.ID, SourceVersion: src.SourceVersion, Role: match.Role, Status: StatusConfirmed, EvidenceKind: "user_confirmation", EvidenceLocator: match.EvidenceLocator, CreatedAt: nowUTC(), UpdatedAt: nowUTC()}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "episode_id"}, {Name: "person_id"}}, DoUpdates: clause.AssignmentColumns([]string{"source_version", "role", "status", "evidence_kind", "evidence_locator", "updated_at"})}).Create(&appearance).Error; err != nil {
			return err
		}
		if err := tx.Where("episode_id = ? AND person_id = ?", src.EpisodeID, person.ID).First(&appearance).Error; err != nil {
			return err
		}
		if match.RoleEdited {
			if err := upsertAppearanceOverride(tx, src.EpisodeID, person.ID, &match.Role, nil); err != nil {
				return err
			}
		}
		if err := upsertConfirmation(tx, models.PersonUserConfirmation{EpisodeID: src.EpisodeID, Kind: models.PersonConfirmationKindName, PersonID: &person.ID, DisplayName: person.DisplayName, Role: match.Role, Status: StatusConfirmed, SourceVersion: src.SourceVersion}); err != nil {
			return err
		}
		for _, order := range match.Orders {
			var row models.SpeechAttribution
			if err := tx.Where("episode_id = ? AND source_version = ? AND source_kind = ? AND fragment_order = ?", src.EpisodeID, src.SourceVersion, SourceTranscript, order).First(&row).Error; err != nil {
				return err
			}
			row.PersonID, row.AppearanceID, row.Status, row.EvidenceKind = &person.ID, &appearance.ID, StatusConfirmed, "user_confirmation"
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
			if err := upsertConfirmation(tx, models.PersonUserConfirmation{EpisodeID: src.EpisodeID, Kind: models.PersonConfirmationKindAttribution, FragmentOrder: order, SourceKind: SourceTranscript, SpeakerLabel: row.SpeakerLabel, AssignedPersonID: &person.ID, SourceVersion: src.SourceVersion, SourceText: row.Text, Status: StatusConfirmed}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) ApplyManual(ctx context.Context, episodeID uint, request ManualMatch) (EpisodePeople, error) {
	src, err := s.currentSources(ctx, episodeID)
	if err != nil {
		return EpisodePeople{}, err
	}
	if request.SourceVersion != src.SourceVersion || (request.Scope != "speaker" && request.Scope != "fragment") {
		return EpisodePeople{}, ErrSourcesChanged
	}
	var anchor *Segment
	for i := range src.Segments {
		if src.Segments[i].Order == request.FragmentOrder {
			anchor = &src.Segments[i]
			break
		}
	}
	if anchor == nil || (!request.Clear && (strings.TrimSpace(request.DisplayName) == "" || len([]rune(request.DisplayName)) > 200)) {
		return EpisodePeople{}, ErrInvalidCorrection
	}
	orders := []int{}
	for _, seg := range src.Segments {
		if seg.Order == anchor.Order || (request.Scope == "speaker" && seg.SpeakerLabel == anchor.SpeakerLabel) {
			orders = append(orders, seg.Order)
		}
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state models.PersonPreparation
		if err := tx.First(&state, "episode_id = ?", episodeID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if state.Revision != request.Revision {
			return ErrSourcesChanged
		}
		metadata, err := readPreparationMetadata(tx, episodeID)
		if err != nil {
			return err
		}
		if err := validatePreparationSource(tx, episodeID, src.SourceVersion, metadata); err != nil {
			return err
		}
		revision, err := reservePreparation(tx, episodeID)
		if err != nil {
			return err
		}
		if request.Clear {
			for _, order := range orders {
				if err := tx.Model(&models.SpeechAttribution{}).Where("episode_id = ? AND source_version = ? AND fragment_order = ?", episodeID, src.SourceVersion, order).Updates(map[string]any{"person_id": nil, "appearance_id": nil, "status": StatusPending}).Error; err != nil {
					return err
				}
				if err := tx.Where("episode_id = ? AND source_version = ? AND fragment_order = ? AND kind = ?", episodeID, src.SourceVersion, order, models.PersonConfirmationKindAttribution).Delete(&models.PersonUserConfirmation{}).Error; err != nil {
					return err
				}
			}
		} else {
			role := RoleUnknown
			if request.PersonID != 0 {
				var appearance models.EpisodeAppearance
				if err := tx.Where("episode_id = ? AND person_id = ? AND source_version = ?", episodeID, request.PersonID, src.SourceVersion).First(&appearance).Error; err != nil {
					return ErrNotEpisodeParticipant
				}
				role = appearance.Role
				overrides, err := loadAppearanceOverrides(tx, episodeID)
				if err != nil {
					return err
				}
				if overrides[request.PersonID].Role != nil {
					role = *overrides[request.PersonID].Role
				}
			}
			if err := s.applyMatches(ctx, tx, src, []ReviewMatch{{Key: "manual", PersonID: request.PersonID, DisplayName: request.DisplayName, Role: role, SpeakerLabel: anchor.SpeakerLabel, Orders: orders, Selected: true}}); err != nil {
				return err
			}
		}
		var publishedState models.PersonPreparation
		if err := tx.First(&publishedState, "episode_id = ?", episodeID).Error; err != nil {
			return err
		}
		revision = publishedState.Revision
		if err := publishPreparation(tx, episodeID, src.SourceVersion, metadata, revision); err != nil {
			return err
		}
		return s.reindexSearchTransaction(ctx, tx, episodeID)
	})
	if err != nil {
		return EpisodePeople{}, err
	}
	return s.ListEpisodePeople(ctx, episodeID)
}
