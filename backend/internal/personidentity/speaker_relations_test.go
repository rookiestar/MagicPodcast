package personidentity

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func relationSource() EpisodeSources {
	return EpisodeSources{ShowNotes: "本集主持林言，嘉宾陈明。", Segments: []Segment{
		{Order: 1, SpeakerLabel: "A", Text: "今天请到了陈明。"},
		{Order: 2, SpeakerLabel: "intro", Text: "节目片头"},
		{Order: 3, SpeakerLabel: "B", Text: "谢谢邀请，我在做研究。"},
		{Order: 4, SpeakerLabel: "A", Text: "接下来请讲讲你做研究的经历。"},
		{Order: 5, SpeakerLabel: "B", Text: "是的。"},
	}}
}
func relationPayload() map[string]any {
	people := []any{}
	for _, name := range []string{"林言", "陈明"} {
		people = append(people, map[string]any{"name": name, "kind": "participant", "status": "confirmed", "role": "unknown", "presence_basis": "introduced_participant", "name_evidence": identityEvidence{Source: SourceShowNotes, Quote: "本集主持林言，嘉宾陈明。"}, "presence_evidence": identityEvidence{Source: SourceShowNotes, Quote: "本集主持林言，嘉宾陈明。"}})
	}
	return map[string]any{"people": people, "speakers": []any{
		map[string]any{"speaker_label": "A", "reason": "主持关系", "candidates": []any{map[string]any{"person_index": 0, "level": "inferred", "basis": "contextual", "reason": "名单列出本集主持，且开场与后续主持一致", "evidence": []identityEvidence{{Source: SourceShowNotes, Quote: "本集主持林言，嘉宾陈明。"}, {Source: SourceTranscript, Fragment: 1, Quote: "今天请到了陈明。"}, {Source: SourceTranscript, Fragment: 4, Quote: "接下来请讲讲你做研究的经历。"}}, "counter_evidence": []identityEvidence{}}}},
		map[string]any{"speaker_label": "B", "reason": "回应点名", "candidates": []any{map[string]any{"person_index": 1, "level": "direct", "basis": "addressed_response", "reason": "片头插入前被点名，随后回应邀请", "evidence": []identityEvidence{{Source: SourceTranscript, Fragment: 1, Quote: "今天请到了陈明。"}, {Source: SourceTranscript, Fragment: 3, Quote: "谢谢邀请，我在做研究。"}}, "counter_evidence": []identityEvidence{}}}},
		map[string]any{"speaker_label": "intro", "reason": "片头，无法确定姓名", "candidates": []any{}},
	}}
}

func splitSpeakerSource() EpisodeSources {
	return EpisodeSources{ShowNotes: "导游：曲凯，42章经创始人；六号珍藏：Albert，AI 创业者。", Segments: []Segment{
		{Order: 1, SpeakerLabel: "Speaker 2", Text: "今天我们继续聊这个问题。"},
		{Order: 2, SpeakerLabel: "Speaker 1", Text: "我觉得还有一个方向。"},
		{Order: 3, SpeakerLabel: "Speaker 2", Text: "那我们进入最后一个问题。"},
		{Order: 4, SpeakerLabel: "Speaker 3", Text: "你觉得这个能力是怎么形成的？"},
		{Order: 5, SpeakerLabel: "Speaker 1", Text: "我觉得主要还是长期训练。"},
		{Order: 6, SpeakerLabel: "Speaker 3", Text: "如果技术成熟了，你最想做什么产品？"},
	}}
}

func splitSpeakerPayload() map[string]any {
	person := func(name, role, quote string) map[string]any {
		return map[string]any{
			"name": name, "kind": "participant", "status": "confirmed", "role": role,
			"presence_basis":    "introduced_participant",
			"name_evidence":     identityEvidence{Source: SourceShowNotes, Quote: quote},
			"presence_evidence": identityEvidence{Source: SourceShowNotes, Quote: quote},
			"role_evidence":     identityEvidence{Source: SourceShowNotes, Quote: quote},
		}
	}
	return map[string]any{
		"people": []any{
			person("曲凯", RoleHost, "导游：曲凯，42章经创始人"),
			person("Albert", RoleGuest, "六号珍藏：Albert，AI 创业者"),
		},
		"speakers": []any{
			map[string]any{
				"speaker_label": "Speaker 3", "reason": "疑似已知主持人被妙记拆成另一组标签",
				"candidates": []any{map[string]any{
					"person_index": 0, "level": "inferred", "basis": "contextual",
					"reason": "该组在访谈后段连续追问，且没有新的参与者身份依据，可能仍是曲凯。",
					"evidence": []identityEvidence{
						{Source: SourceShowNotes, Quote: "导游：曲凯，42章经创始人"},
						{Source: SourceTranscript, Fragment: 4, Quote: "你觉得这个能力是怎么形成的？"},
						{Source: SourceTranscript, Fragment: 6, Quote: "如果技术成熟了，你最想做什么产品？"},
					},
					"counter_evidence": []identityEvidence{},
				}},
			},
		},
	}
}

func producerSource() EpisodeSources {
	return EpisodeSources{ShowNotes: "导游：曲凯。制作人：陈皮。", Segments: []Segment{
		{Order: 1, SpeakerLabel: "Speaker 2", Text: "好，今天先聊到这里。"},
		{Order: 2, SpeakerLabel: "Speaker 4", Text: "不好意思，我想再补一个问题。大家好，我是陈皮，在节目组做内容。"},
		{Order: 3, SpeakerLabel: "Speaker 3", Text: "这个问题很好。"},
	}}
}

func producerPayload() map[string]any {
	return map[string]any{
		"people": []any{
			map[string]any{"name": "陈皮", "kind": "participant", "status": "confirmed", "role": "unknown", "presence_basis": "self_introduction",
				"name_evidence":     identityEvidence{Source: SourceShowNotes, Quote: "制作人：陈皮。"},
				"presence_evidence": identityEvidence{Source: SourceTranscript, Fragment: 2, Quote: "大家好，我是陈皮，在节目组做内容。"},
				"source_names":      []any{map[string]any{"name": "陈皮", "evidence": identityEvidence{Source: SourceTranscript, Fragment: 2, Quote: "我是陈皮"}}}},
		},
		"speakers": []any{
			map[string]any{"speaker_label": "Speaker 4", "reason": "明确自我介绍为制作人", "candidates": []any{map[string]any{
				"person_index": 0, "level": "direct", "basis": "self_introduction",
				"reason":           "该 Speaker 在补问前明确自报姓名陈皮。",
				"evidence":         []identityEvidence{{Source: SourceTranscript, Fragment: 2, Quote: "不好意思，我想再补一个问题。大家好，我是陈皮，在节目组做内容。"}},
				"counter_evidence": []identityEvidence{},
			}}},
		},
	}
}

func decodeRelations(t *testing.T, p map[string]any, src EpisodeSources) Suggestions {
	t.Helper()
	b, err := json.Marshal(p)
	require.NoError(t, err)
	result, err := decodeSpeakerSuggestions(b, src)
	require.NoError(t, err)
	return result
}

func TestSpeakerRelationSuggestsKnownHostForASecondUnmatchedLabel(t *testing.T) {
	got := decodeRelations(t, splitSpeakerPayload(), splitSpeakerSource())
	var match ReviewMatch
	found := false
	for _, candidate := range got.Matches {
		if candidate.SpeakerLabel == "Speaker 3" {
			match = candidate
			found = true
			break
		}
	}
	require.True(t, found)
	require.Equal(t, "Speaker 3", match.SpeakerLabel)
	require.Equal(t, "inferred", match.Relation.State)
	require.Equal(t, "曲凯", match.DisplayName)
	require.Equal(t, "person:0", match.Choice)
	require.Equal(t, []int{4, 6}, match.Orders)
	require.False(t, match.Selected, "same-person suggestions remain user-confirmed")
	require.True(t, match.Uncertain)
}

func TestSpeakerRelationKeepsExplicitProducerSeparateFromKnownHost(t *testing.T) {
	got := decodeRelations(t, producerPayload(), producerSource())
	var match ReviewMatch
	found := false
	for _, candidate := range got.Matches {
		if candidate.SpeakerLabel == "Speaker 4" {
			match = candidate
			found = true
			break
		}
	}
	require.True(t, found)
	require.Equal(t, "Speaker 4", match.SpeakerLabel)
	require.Equal(t, "direct", match.Relation.State)
	require.Equal(t, "陈皮", match.DisplayName)
	require.Equal(t, "person:0", match.Choice)
	require.True(t, match.Selected)
	require.Equal(t, []int{2}, match.Orders)
}
func TestSpeakerRelationProposalsPreserveInferredGroupsAndUnknowns(t *testing.T) {
	got := decodeRelations(t, relationPayload(), relationSource())
	require.Len(t, got.Matches, 3)
	require.Equal(t, "inferred", got.Matches[0].Relation.State)
	require.Equal(t, "林言", got.Matches[0].DisplayName)
	require.Equal(t, []int{1, 4}, got.Matches[0].Orders)
	require.False(t, got.Matches[0].Selected)
	require.Equal(t, "insufficient", got.Matches[1].Relation.State)
	require.False(t, got.Matches[1].Selected)
	require.Equal(t, "direct", got.Matches[2].Relation.State)
	require.Equal(t, []int{3, 5}, got.Matches[2].Orders)
	require.True(t, got.Matches[2].Selected)
}
func TestSpeakerRelationConflictsInvalidEvidenceAndOmissionsRemainVisible(t *testing.T) {
	for _, kind := range []string{"conflict", "invalid", "omitted", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			p := relationPayload()
			speakers := p["speakers"].([]any)
			first := speakers[0].(map[string]any)
			c := first["candidates"].([]any)[0].(map[string]any)
			switch kind {
			case "conflict":
				b, _ := json.Marshal(c)
				var other map[string]any
				require.NoError(t, json.Unmarshal(b, &other))
				other["person_index"] = 1
				first["candidates"] = []any{c, other}
			case "invalid":
				c["evidence"] = []identityEvidence{{Source: SourceTranscript, Fragment: 1, Quote: "我叫林言"}}
			case "omitted":
				p["speakers"] = speakers[1:]
			case "duplicate":
				first["candidates"] = []any{c, c}
			}
			got := decodeRelations(t, p, relationSource())
			m := got.Matches[0]
			require.Equal(t, []int{1, 4}, m.Orders)
			require.False(t, m.Selected)
			expected := map[string]string{"conflict": "conflict", "invalid": "invalid_evidence", "omitted": "not_assessed", "duplicate": "invalid_evidence"}[kind]
			require.Equal(t, expected, m.Relation.State)
			require.Empty(t, m.Choice)
		})
	}
}

type relationSuggester struct{ result Suggestions }

func (r relationSuggester) Suggest(context.Context, EpisodeSources) (Suggestions, error) {
	return r.result, nil
}
func TestSpeakerRelationDraftApplyAndCorrectionUseOneAttributionSet(t *testing.T) {
	service, src := reviewFixture(t)
	src = publishReviewFixture(t, service, src)
	// Use fixture's published source identities for the persistence boundary.
	proposed := decodeRelations(t, relationPayload(), relationSource())
	proposed.Matches = proposed.Matches[:1]
	proposed.Matches[0].SpeakerLabel = src.Segments[0].SpeakerLabel
	proposed.Matches[0].Orders = []int{}
	for _, seg := range src.Segments {
		if seg.SpeakerLabel == proposed.Matches[0].SpeakerLabel {
			proposed.Matches[0].Orders = append(proposed.Matches[0].Orders, seg.Order)
		}
	}
	service.suggester = relationSuggester{proposed}
	draft, err := service.Prepare(context.Background(), src)
	require.NoError(t, err)
	require.Empty(t, draft.People)
	require.Empty(t, draft.Attributions)
	require.False(t, draft.Draft.Matches[0].Selected)
	request := draftRequest(draft)
	saved, err := service.Review(context.Background(), src.EpisodeID, request, false)
	require.NoError(t, err)
	require.Equal(t, draft.Draft.Matches[0].Relation, saved.Draft.Matches[0].Relation)
	require.Empty(t, saved.Attributions)
	stale := request
	request = draftRequest(saved)
	request.Matches[0].Selected = true
	applied, err := service.Review(context.Background(), src.EpisodeID, request, true)
	require.NoError(t, err)
	require.Len(t, applied.People, 1)
	facts, err := service.ReliableSpeech(context.Background(), src.EpisodeID, applied.People[0].ID)
	require.NoError(t, err)
	require.Len(t, facts, len(request.Matches[0].Orders))
	_, err = service.Review(context.Background(), src.EpisodeID, stale, true)
	require.ErrorIs(t, err, ErrSourcesChanged)
	cleared, err := service.ApplyManual(context.Background(), src.EpisodeID, ManualMatch{Revision: applied.Revision, SourceVersion: src.SourceVersion, FragmentOrder: request.Matches[0].Orders[0], Scope: "fragment", Clear: true})
	require.NoError(t, err)
	facts, err = service.ReliableSpeech(context.Background(), src.EpisodeID, applied.People[0].ID)
	require.NoError(t, err)
	require.Len(t, facts, len(request.Matches[0].Orders)-1)
	fresh, err := service.Prepare(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, cleared.Attributions, fresh.Attributions)
}
func TestSpeakerRelationRejectsTamperedOrDroppedProposal(t *testing.T) {
	src := relationSource()
	stored := decodeRelations(t, relationPayload(), src).Matches
	for _, kind := range []string{"drop", "tamper", "unknown_choice", "speaker", "person_id"} {
		t.Run(kind, func(t *testing.T) {
			b, _ := json.Marshal(stored)
			var m []ReviewMatch
			require.NoError(t, json.Unmarshal(b, &m))
			switch kind {
			case "drop":
				m[0].Relation = nil
			case "tamper":
				m[0].Relation.State = "direct"
			case "unknown_choice":
				m[0].Choice = "bad"
			case "speaker":
				m[0].SpeakerLabel = "B"
			case "person_id":
				m[0].PersonID = 999
			}
			err := resolveSpeakerChoices(m, stored)
			if kind == "person_id" {
				require.NoError(t, err)
				require.Zero(t, m[0].PersonID)
			} else {
				require.ErrorIs(t, err, ErrInvalidCorrection)
			}
		})
	}
}

func TestSpeakerRelationManualChoiceAndMultipleGroupsKeepIdentity(t *testing.T) {
	s, src := reviewFixture(t)
	got := decodeRelations(t, relationPayload(), relationSource())
	first := got.Matches[0]
	first.SpeakerLabel = "Speaker 1"
	first.Orders = []int{1, 3, 4}
	first.Selected = true
	second := first
	second.Key = "speaker:Speaker 2"
	second.SpeakerLabel = "Speaker 2"
	second.Orders = []int{2}
	s.suggester = relationSuggester{Suggestions{Matches: []ReviewMatch{first, second}}}
	draft, err := s.Prepare(context.Background(), src)
	require.NoError(t, err)
	req := draftRequest(draft)
	applied, err := s.Review(context.Background(), src.EpisodeID, req, true)
	require.NoError(t, err)
	require.Len(t, applied.People, 1)
	require.Equal(t, 4, applied.People[0].ConfirmedSpeech)
	req = draftRequest(applied)
	req.Matches[0].Choice = "manual"
	req.Matches[0].DisplayName = "另一位"
	req.Matches[0].Selected = true
	applied, err = s.Review(context.Background(), src.EpisodeID, req, true)
	require.NoError(t, err)
	require.Equal(t, "另一位", applied.Attributions[0].DisplayName)
	require.Equal(t, "林言", applied.Attributions[1].DisplayName)
	require.Empty(t, applied.Draft.Matches[0].EvidenceLocator)
	req = draftRequest(applied)
	req.Matches[0].Selected = true
	req.Matches[1].Choice = "manual"
	req.Matches[1].DisplayName = "另一位"
	req.Matches[1].Selected = true
	applied, err = s.Review(context.Background(), src.EpisodeID, req, true)
	require.NoError(t, err)
	require.NotEqual(t, applied.Attributions[0].PersonID, applied.Attributions[1].PersonID, "manual names alone do not merge distinct people")
}

func TestDirectRelationNeedsLocatedIdentityNotJustExistingWords(t *testing.T) {
	p := relationPayload()
	c := p["speakers"].([]any)[0].(map[string]any)["candidates"].([]any)[0].(map[string]any)
	c["level"] = "direct"
	c["basis"] = "first_person_identity"
	// All quotes exist, but the Speaker has not made a named first-person claim.
	got := decodeRelations(t, p, relationSource())
	require.Equal(t, "invalid_evidence", got.Matches[0].Relation.State)
	require.False(t, got.Matches[0].Selected)
}

func TestSpeakerRelationSeparateApplicationsKeepRenamedIdentity(t *testing.T) {
	s, src := reviewFixture(t)
	first := decodeRelations(t, relationPayload(), relationSource()).Matches[0]
	first.SpeakerLabel, first.Orders = "Speaker 1", []int{1, 3, 4}
	second := first
	second.Key, second.SpeakerLabel, second.Orders = "speaker:Speaker 2", "Speaker 2", []int{2}
	s.suggester = relationSuggester{Suggestions{Matches: []ReviewMatch{first, second}}}
	draft, err := s.Prepare(context.Background(), src)
	require.NoError(t, err)
	req := draftRequest(draft)
	for i := range req.Matches {
		req.Matches[i].DisplayName = "林老师"
	}
	req.Matches[0].Selected = true
	applied, err := s.Review(context.Background(), src.EpisodeID, req, true)
	require.NoError(t, err)
	personID := applied.Draft.Matches[0].PersonID
	req = draftRequest(applied)
	req.Matches[1].Selected = true
	applied, err = s.Review(context.Background(), src.EpisodeID, req, true)
	require.NoError(t, err)
	require.Equal(t, personID, applied.Draft.Matches[1].PersonID)
	facts, err := s.ReliableSpeech(context.Background(), src.EpisodeID, personID)
	require.NoError(t, err)
	require.Len(t, facts, 4)
}

func TestSpeakerRelationSeparateApplicationsKeepNamesakesDistinct(t *testing.T) {
	s, src := reviewFixture(t)
	first := decodeRelations(t, relationPayload(), relationSource()).Matches[0]
	first.SpeakerLabel, first.Orders = "Speaker 1", []int{1, 3, 4}
	second := first
	second.Key, second.SpeakerLabel, second.Orders = "speaker:Speaker 2", "Speaker 2", []int{2}
	other := first.Relation.Candidates[0]
	other.ID = "person:other"
	second.Relation = &SpeakerRelation{Version: 1, State: "inferred", Candidates: []RelationCandidate{other}}
	second.Choice = other.ID
	s.suggester = relationSuggester{Suggestions{Matches: []ReviewMatch{first, second}}}
	draft, err := s.Prepare(context.Background(), src)
	require.NoError(t, err)
	req := draftRequest(draft)
	req.Matches[0].Selected = true
	applied, err := s.Review(context.Background(), src.EpisodeID, req, true)
	require.NoError(t, err)
	firstID := applied.Draft.Matches[0].PersonID
	req = draftRequest(applied)
	req.Matches[1].Selected = true
	applied, err = s.Review(context.Background(), src.EpisodeID, req, true)
	require.NoError(t, err)
	require.NotEqual(t, firstID, applied.Draft.Matches[1].PersonID)
	facts, err := s.ReliableSpeech(context.Background(), src.EpisodeID, firstID)
	require.NoError(t, err)
	require.Len(t, facts, 3)
}
