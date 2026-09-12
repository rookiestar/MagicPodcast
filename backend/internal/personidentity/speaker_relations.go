package personidentity

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Relation states describe evidence, never user approval. The immutable proposal
// is stored with the existing draft; Choice and edits are the user's decision.
type SpeakerRelation struct {
	Version    int                 `json:"version"`
	State      string              `json:"state"`
	Reason     string              `json:"reason"`
	Candidates []RelationCandidate `json:"candidates"`
}
type RelationCandidate struct {
	ID              string             `json:"id"`
	DisplayName     string             `json:"display_name"`
	Role            string             `json:"role"`
	Status          string             `json:"status"`
	IdentityNote    string             `json:"identity_note"`
	SourceNames     []string           `json:"source_names"`
	Aliases         []string           `json:"aliases"`
	EvidenceLocator string             `json:"evidence_locator"`
	Level           string             `json:"level"`
	Reason          string             `json:"reason"`
	Evidence        []identityEvidence `json:"evidence"`
	CounterEvidence []identityEvidence `json:"counter_evidence"`
}
type proposedRelation struct {
	Speaker    string `json:"speaker_label"`
	Reason     string `json:"reason"`
	Candidates []struct {
		PersonIndex     int                `json:"person_index"`
		Level           string             `json:"level"`
		Basis           string             `json:"basis"`
		Reason          string             `json:"reason"`
		Evidence        []identityEvidence `json:"evidence"`
		CounterEvidence []identityEvidence `json:"counter_evidence"`
	} `json:"candidates"`
}

func decodeSpeakerSuggestions(raw json.RawMessage, src EpisodeSources) (Suggestions, error) {
	var response struct {
		People   []identityProposal `json:"people"`
		Speakers []proposedRelation `json:"speakers"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return Suggestions{}, err
	}
	// Legacy decoder remains the identity evidence validator, not a second speaker
	// classifier. Direct anchors are supplied from the located relation evidence.
	segments := map[int]Segment{}
	groups := map[string][]int{}
	order := []string{}
	for _, seg := range src.Segments {
		segments[seg.Order] = seg
		if strings.TrimSpace(seg.SpeakerLabel) == "" {
			continue
		}
		if _, ok := groups[seg.SpeakerLabel]; !ok {
			order = append(order, seg.SpeakerLabel)
		}
		groups[seg.SpeakerLabel] = append(groups[seg.SpeakerLabel], seg.Order)
	}
	validEvidence := func(e identityEvidence) bool {
		var text string
		if e.Source == SourceTranscript {
			seg, ok := segments[e.Fragment]
			if !ok {
				return false
			}
			text = seg.Text
		} else {
			if e.Fragment != 0 {
				return false
			}
			switch e.Source {
			case SourceShowNotes:
				text = src.ShowNotes
			case "podcast_title":
				text = src.PodcastTitle
			case "podcast_author":
				text = src.PodcastAuthor
			case "podcast_description":
				text = src.PodcastDescription
			case "episode_title":
				text = src.EpisodeTitle
			default:
				return false
			}
		}
		q := normalizeIdentityEvidence(e.Quote)
		return q != "" && strings.Contains(normalizeIdentityEvidence(text), q)
	}
	people := map[int]RelationCandidate{}
	for i, p := range response.People {
		// Reconstruct only direct, source-located anchors for existing identity/name
		// validation and local identity stability. Contextual proposals cannot invent one.
		for _, r := range response.Speakers {
			for _, c := range r.Candidates {
				if c.PersonIndex != i || c.Level != "direct" {
					continue
				}
				for _, e := range c.Evidence {
					if e.Source == SourceTranscript && segments[e.Fragment].SpeakerLabel == r.Speaker && validEvidence(e) {
						b := struct {
							SpeakerLabel string           `json:"speaker_label"`
							Evidence     identityEvidence `json:"evidence"`
							Basis        string           `json:"basis"`
						}{r.Speaker, e, c.Basis}
						p.SpeechBindings = append(p.SpeechBindings, b)
						break
					}
				}
			}
		}
		single, _ := json.Marshal(map[string]any{"people": []identityProposal{p}})
		validated, err := decodeIdentitySuggestions(single, src)
		if err != nil {
			return Suggestions{}, err
		}
		if len(validated) == 0 {
			continue
		}
		v := validated[0]
		people[i] = RelationCandidate{ID: fmt.Sprintf("person:%d", i), DisplayName: v.DisplayName, Role: v.Role, Status: v.Status, IdentityNote: v.IdentityNote, SourceNames: v.SourceNames, Aliases: v.Aliases, EvidenceLocator: v.EvidenceLocator}
	}
	bySpeaker := map[string][]proposedRelation{}
	for _, r := range response.Speakers {
		if _, ok := groups[r.Speaker]; ok {
			bySpeaker[r.Speaker] = append(bySpeaker[r.Speaker], r)
		}
	}
	matches := make([]ReviewMatch, 0, len(order))
	used := map[int]bool{}
	for _, speaker := range order {
		rel := &SpeakerRelation{Version: 1, State: "insufficient", Reason: "尚无足够证据", Candidates: []RelationCandidate{}}
		rs := bySpeaker[speaker]
		if len(rs) == 0 {
			rel.State = "not_assessed"
			rel.Reason = "本次未给出判断"
		}
		seen := map[int]bool{}
		invalid := false
		for _, r := range rs {
			if len(r.Candidates) == 0 && strings.TrimSpace(r.Reason) != "" {
				rel.Reason = r.Reason
			}
			for _, c := range r.Candidates {
				p, ok := people[c.PersonIndex]
				if !ok {
					invalid = true
					continue
				}
				if seen[c.PersonIndex] {
					invalid = true
					continue
				}
				seen[c.PersonIndex] = true
				used[c.PersonIndex] = true
				p.Level = c.Level
				p.Reason = c.Reason
				p.Evidence = c.Evidence
				p.CounterEvidence = c.CounterEvidence
				hasAnchor := false
				bad := false
				for _, e := range append(append([]identityEvidence{}, c.Evidence...), c.CounterEvidence...) {
					if !validEvidence(e) {
						bad = true
					}
				}
				for _, e := range c.Evidence {
					if e.Source == SourceTranscript && segments[e.Fragment].SpeakerLabel == speaker {
						hasAnchor = true
					}
				}
				if !hasAnchor || len(c.Evidence) == 0 || strings.TrimSpace(c.Reason) == "" || (c.Level != "direct" && c.Level != "inferred") {
					bad = true
				}
				if c.Level == "inferred" && len(c.Evidence) < 2 {
					bad = true
				}
				if c.Level == "direct" && c.Basis != "self_introduction" && c.Basis != "first_person_identity" && c.Basis != "addressed_response" {
					bad = true
				}
				if c.Level == "direct" {
					anchoredName, namedInvitation := false, false
					names := append([]string{p.DisplayName}, p.SourceNames...)
					for _, e := range c.Evidence {
						if e.Source != SourceTranscript {
							continue
						}
						for _, name := range names {
							if name == "" || !strings.Contains(normalizeIdentityEvidence(e.Quote), normalizeIdentityEvidence(name)) {
								continue
							}
							if segments[e.Fragment].SpeakerLabel == speaker {
								anchoredName = true
							} else {
								namedInvitation = true
							}
						}
					}
					if (c.Basis == "addressed_response" && !namedInvitation) || (c.Basis != "addressed_response" && !anchoredName) {
						bad = true
					}
				}
				if bad {
					p.Level = "invalid"
					invalid = true
				}
				// Identity uncertainty and material counter-evidence must remain explicit.
				if !bad && (p.Status != StatusConfirmed || len(c.CounterEvidence) > 0) {
					p.Level = "inferred"
				}
				rel.Candidates = append(rel.Candidates, p)
			}
		}
		switch {
		case invalid:
			rel.State = "invalid_evidence"
			rel.Reason = "部分证据未通过核对，请查看依据或手动确认"
		case len(rel.Candidates) > 1:
			rel.State = "conflict"
			rel.Reason = "存在多个候选，请选择人物"
		case len(rel.Candidates) == 1:
			rel.State = rel.Candidates[0].Level
			rel.Reason = rel.Candidates[0].Reason
		}
		orders := groups[speaker]
		sort.Ints(orders)
		m := ReviewMatch{Key: "speaker:" + speaker, DisplayName: speaker, Role: RoleUnknown, SpeakerLabel: speaker, Orders: orders, Relation: rel, Uncertain: true}
		if len(rel.Candidates) == 1 && rel.State != "invalid_evidence" {
			c := rel.Candidates[0]
			m.Choice = c.ID
			m.DisplayName = c.DisplayName
			m.Role = c.Role
			assignRelationPerson(&m, c)
			m.Selected = rel.State == "direct"
			m.Uncertain = rel.State != "direct"
		}
		matches = append(matches, m)
	}
	// Unbound identities stay in the candidate pool; the UI presents them in the
	// group selector, not as duplicate zero-fragment cards.
	for i := range response.People {
		if p, ok := people[i]; ok && !used[i] {
			matches = append(matches, ReviewMatch{Key: "unbound:" + p.ID, DisplayName: p.DisplayName, Role: p.Role, Orders: []int{}, Relation: &SpeakerRelation{Version: 1, State: "unbound", Candidates: []RelationCandidate{p}}, Uncertain: true})
		}
	}
	return Suggestions{Matches: matches}, nil
}

func assignRelationPerson(m *ReviewMatch, p RelationCandidate) {
	m.OriginalName = p.DisplayName
	m.IdentityNote = p.IdentityNote
	m.Aliases = p.Aliases
	m.SourceNames = p.SourceNames
	m.SuggestedStatus = p.Status
	m.EvidenceLocator = p.EvidenceLocator
}

// Validate the immutable suggestion before deriving applied identity fields. An
// older client that drops the relation cannot silently accept a new proposal.
func resolveSpeakerChoices(matches, stored []ReviewMatch) error {
	originals := map[string]ReviewMatch{}
	pool := map[string]RelationCandidate{}
	for _, m := range stored {
		originals[m.Key] = m
		if m.Relation != nil {
			for _, p := range m.Relation.Candidates {
				pool[p.ID] = p
			}
		}
	}
	for i := range matches {
		m := &matches[i]
		old, ok := originals[m.Key]
		if !ok {
			return ErrInvalidCorrection
		}
		if !reflect.DeepEqual(m.Relation, old.Relation) {
			return ErrInvalidCorrection
		}
		if m.Relation == nil {
			continue
		}
		if m.Relation.Version != 1 || m.SpeakerLabel != old.SpeakerLabel {
			return ErrInvalidCorrection
		}
		if m.Choice == "" {
			if m.Selected {
				return ErrInvalidCorrection
			}
			m.PersonID = 0
			continue
		}
		if m.Choice == "manual" {
			if isGenericLabel(m.DisplayName) {
				return ErrInvalidCorrection
			}
			m.OriginalName = ""
			m.EvidenceLocator = ""
			m.SourceNames = nil
			m.Aliases = nil
			m.IdentityNote = ""
			m.SuggestedStatus = StatusPending
		} else {
			p, ok := pool[m.Choice]
			if !ok {
				return ErrInvalidCorrection
			}
			assignRelationPerson(m, p)
		}
		// The client cannot supply an arbitrary global person identity.
		m.PersonID = 0
		if old.Choice == m.Choice && old.DisplayName == m.DisplayName {
			m.PersonID = old.PersonID
		}
	}
	return nil
}
