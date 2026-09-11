package personidentity

import (
	"encoding/json"
	"sort"
	"strings"
)

type identityEvidence struct {
	Source   string `json:"source"`
	Fragment int    `json:"fragment"`
	Quote    string `json:"quote"`
}

type identityAnchor struct {
	Key      string           `json:"key"`
	Kind     string           `json:"kind"`
	Evidence identityEvidence `json:"evidence"`
}

type roleConflict struct {
	Role     string           `json:"role"`
	Evidence identityEvidence `json:"evidence"`
}

type identityProposal struct {
	RoleConflicts    []roleConflict   `json:"role_conflicts,omitempty"`
	NameType         string           `json:"name_type"`
	IdentityAnchor   identityAnchor   `json:"identity_anchor"`
	Name             string           `json:"name"`
	Aliases          []string         `json:"aliases"`
	Identity         string           `json:"identity"`
	Role             string           `json:"role"`
	Status           string           `json:"status"`
	Kind             string           `json:"kind"`
	PresenceBasis    string           `json:"presence_basis"`
	NameEvidence     identityEvidence `json:"name_evidence"`
	PresenceEvidence identityEvidence `json:"presence_evidence"`
	RoleEvidence     identityEvidence `json:"role_evidence"`
	SourceNames      []struct {
		Name     string           `json:"name"`
		Evidence identityEvidence `json:"evidence"`
	} `json:"source_names"`
	SpeechBindings []struct {
		SpeakerLabel   string           `json:"speaker_label"`
		Evidence       identityEvidence `json:"evidence"`
		Basis          string           `json:"basis"`
		Orders         []int            `json:"orders"`
		Scope          string           `json:"scope"`
		ExcludedOrders []int            `json:"excluded_orders"`
	} `json:"speech_bindings"`
}

func decodeIdentitySuggestions(raw json.RawMessage, sources EpisodeSources) ([]SuggestedCandidate, error) {
	var result struct {
		People []identityProposal `json:"people"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	segments := map[int]Segment{}
	for _, seg := range sources.Segments {
		segments[seg.Order] = seg
	}
	evidenceValid := func(e identityEvidence) bool {
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
				text = sources.ShowNotes
			case "podcast_title":
				text = sources.PodcastTitle
			case "podcast_author":
				text = sources.PodcastAuthor
			case "podcast_description":
				text = sources.PodcastDescription
			case "episode_title":
				text = sources.EpisodeTitle
			default:
				return false
			}
		}
		quote := normalizeIdentityEvidence(e.Quote)
		return quote != "" && strings.Contains(normalizeIdentityEvidence(text), quote)
	}
	var out []SuggestedCandidate
	for _, p := range result.People {
		p.Name = strings.TrimSpace(p.Name)
		if p.Kind != "participant" || p.Name == "" || isGenericLabel(p.Name) || !evidenceValid(p.NameEvidence) || !strings.Contains(normalizeIdentityEvidence(p.NameEvidence.Quote), normalizeIdentityEvidence(p.Name)) {
			continue
		}
		// Metadata supplies spelling, never proof that its author attended.
		presenceSource := p.PresenceEvidence.Source
		if (presenceSource != SourceTranscript && presenceSource != SourceShowNotes) || !evidenceValid(p.PresenceEvidence) {
			continue
		}
		switch p.Status {
		case StatusConfirmed, StatusPending:
		default:
			continue
		}
		switch p.PresenceBasis {
		case "self_introduction", "first_person_identity", "introduced_participant", "interview_role":
		case "uncertain":
			p.Status = StatusPending
		default:
			continue
		}
		aliases := []string{}
		for _, alias := range p.Aliases {
			if strings.TrimSpace(alias) != "" && strings.Contains(normalizeIdentityEvidence(p.NameEvidence.Quote), normalizeIdentityEvidence(alias)) {
				aliases = append(aliases, alias)
			}
		}
		localNames := append([]string{p.Name}, aliases...)
		for i := range p.SourceNames {
			n := &p.SourceNames[i]
			if n.Evidence.Source != SourceTranscript || !evidenceValid(n.Evidence) || strings.TrimSpace(n.Name) == "" {
				continue
			}
			// A model may quote only the formal name from an introduction whose same
			// located fragment also uses the short form. Expand to that actual fragment
			// so the local spelling remains explicitly visible in the stored evidence.
			if !strings.Contains(normalizeIdentityEvidence(n.Evidence.Quote), normalizeIdentityEvidence(n.Name)) {
				fragment := segments[n.Evidence.Fragment]
				if !strings.Contains(normalizeIdentityEvidence(fragment.Text), normalizeIdentityEvidence(n.Name)) {
					continue
				}
				n.Evidence.Quote = fragment.Text
			}
			localNames = append(localNames, n.Name)
		}
		// A biographical reply can be mislabeled as a first-person name claim.
		// Preserve it only when the existing located invitation/response contract
		// independently applies; unnamed autobiography alone remains insufficient.
		for i := range p.SpeechBindings {
			binding := &p.SpeechBindings[i]
			if binding.Basis != "first_person_identity" || binding.Evidence.Source != SourceTranscript || !evidenceValid(binding.Evidence) {
				continue
			}
			anchor := segments[binding.Evidence.Fragment]
			previous, exists := segments[binding.Evidence.Fragment-1]
			if !exists || binding.SpeakerLabel == "" || anchor.SpeakerLabel != binding.SpeakerLabel || previous.SpeakerLabel == anchor.SpeakerLabel {
				continue
			}
			ownsName, namedInvitation := false, false
			for _, name := range localNames {
				ownsName = ownsName || strings.Contains(normalizeIdentityEvidence(binding.Evidence.Quote), normalizeIdentityEvidence(name))
				namedInvitation = namedInvitation || strings.Contains(normalizeIdentityEvidence(previous.Text), normalizeIdentityEvidence(name))
			}
			if !ownsName && namedInvitation {
				binding.Basis = "addressed_response"
				if p.PresenceBasis == "first_person_identity" && presenceSource == SourceTranscript && p.PresenceEvidence.Fragment == binding.Evidence.Fragment {
					p.PresenceBasis = "interview_role"
				}
			}
		}
		namedPresence := false
		for _, name := range localNames {
			if strings.Contains(normalizeIdentityEvidence(p.PresenceEvidence.Quote), normalizeIdentityEvidence(name)) {
				namedPresence = true
			}
		}
		if !namedPresence && p.PresenceBasis == "self_introduction" && presenceSource == SourceTranscript {
			for _, binding := range p.SpeechBindings {
				if binding.Basis != "self_introduction" || binding.Evidence.Source != SourceTranscript ||
					binding.Evidence.Fragment != p.PresenceEvidence.Fragment || !evidenceValid(binding.Evidence) ||
					binding.SpeakerLabel != segments[p.PresenceEvidence.Fragment].SpeakerLabel {
					continue
				}
				for _, name := range localNames {
					if strings.Contains(normalizeIdentityEvidence(binding.Evidence.Quote), normalizeIdentityEvidence(name)) {
						namedPresence = true
					}
				}
			}
		}
		// A named invitation followed by an anchored response is also located
		// participation evidence; the response need not repeat the person's name.
		if !namedPresence && (p.PresenceBasis == "interview_role" || p.PresenceBasis == "introduced_participant") && presenceSource == SourceTranscript {
			previous, hasPrevious := segments[p.PresenceEvidence.Fragment-1]
			responding := segments[p.PresenceEvidence.Fragment]
			if hasPrevious && previous.SpeakerLabel != responding.SpeakerLabel {
				for _, binding := range p.SpeechBindings {
					if binding.Basis != "addressed_response" || binding.Evidence.Source != SourceTranscript ||
						binding.Evidence.Fragment != p.PresenceEvidence.Fragment || !evidenceValid(binding.Evidence) ||
						binding.SpeakerLabel != responding.SpeakerLabel {
						continue
					}
					for _, name := range localNames {
						if strings.Contains(normalizeIdentityEvidence(previous.Text), normalizeIdentityEvidence(name)) {
							namedPresence = true
						}
					}
				}
			}
		}
		if !namedPresence {
			p.Status = StatusPending
		}
		switch p.Role {
		case RoleHost, RoleGuest:
			if (p.RoleEvidence.Source != SourceTranscript && p.RoleEvidence.Source != SourceShowNotes) || !evidenceValid(p.RoleEvidence) {
				p.Role = RoleUnknown
			}
		default:
			p.Role = RoleUnknown
		}

		orders := []int{}
		seen := map[int]bool{}
		if p.Status == StatusConfirmed {
			for _, binding := range p.SpeechBindings {
				if binding.Basis != "self_introduction" && binding.Basis != "addressed_response" && binding.Basis != "first_person_identity" {
					continue
				}
				if binding.Evidence.Source != SourceTranscript || !evidenceValid(binding.Evidence) {
					continue
				}
				anchor := segments[binding.Evidence.Fragment]
				if strings.TrimSpace(binding.SpeakerLabel) == "" || anchor.SpeakerLabel != binding.SpeakerLabel {
					continue
				}
				if binding.Basis == "first_person_identity" {
					named := false
					for _, name := range localNames {
						if strings.Contains(normalizeIdentityEvidence(binding.Evidence.Quote), normalizeIdentityEvidence(name)) {
							named = true
						}
					}
					if !named {
						continue
					}
				}
				bindingOrders := binding.Orders
				excluded := map[int]bool{}
				for _, order := range binding.ExcludedOrders {
					excluded[order] = true
				}
				if binding.Scope == "stable_speaker" {
					bindingOrders = make([]int, 0)
					for order, seg := range segments {
						if seg.SpeakerLabel == binding.SpeakerLabel {
							bindingOrders = append(bindingOrders, order)
						}
					}
					sort.Ints(bindingOrders)
				}
				for _, order := range bindingOrders {
					seg, ok := segments[order]
					if ok && seg.SpeakerLabel == binding.SpeakerLabel && !seen[order] && !excluded[order] {
						orders = append(orders, order)
						seen[order] = true
					}
				}
			}
		}
		anchor := p.IdentityAnchor
		if p.NameType != "canonical" || p.Status != StatusConfirmed ||
			(anchor.Kind != "distinctive_affiliation" && anchor.Kind != "public_profile") ||
			strings.TrimSpace(anchor.Key) == "" || !evidenceValid(anchor.Evidence) ||
			!strings.Contains(normalizeIdentityEvidence(anchor.Evidence.Quote), normalizeIdentityEvidence(anchor.Key)) ||
			!strings.Contains(normalizeIdentityEvidence(anchor.Evidence.Quote), normalizeIdentityEvidence(p.Name)) {
			p.IdentityAnchor = identityAnchor{}
		}
		proof, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		out = append(out, SuggestedCandidate{DisplayName: p.Name, Aliases: uniqueStrings(aliases), SourceNames: uniqueStrings(localNames), Status: p.Status, IdentityNote: p.Identity, Role: p.Role, EvidenceKind: "verified_runtime", EvidenceLocator: string(proof), SpeechOrders: orders})
	}
	return out, nil
}
