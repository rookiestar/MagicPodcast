package personidentity

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"magicpodcast/internal/models"
)

func TestEvidenceKeepsCanonicalNameAndRejectsWrongSpeakerAnchor(t *testing.T) {
	sources := EpisodeSources{PodcastAuthor: "林小筠", ShowNotes: "本集由林小筠主持，嘉宾陈明。",
		Segments: []Segment{{Order: 1, SpeakerLabel: "Speaker 1", Text: "我是小云。欢迎陈明。"}, {Order: 2, SpeakerLabel: "Speaker 2", Text: "谢谢邀请。"}}}
	raw := []byte(`{"people":[{"name":"林小筠","kind":"participant","status":"confirmed","role":"host","presence_basis":"self_introduction","name_evidence":{"source":"podcast_author","fragment":0,"quote":"林小筠"},"presence_evidence":{"source":"transcript","fragment":1,"quote":"我是小云。"},"role_evidence":{"source":"show_notes","fragment":0,"quote":"本集由林小筠主持"},"source_names":[{"name":"小云","evidence":{"source":"transcript","fragment":1,"quote":"我是小云。"}}],"speech_bindings":[{"speaker_label":"Speaker 1","basis":"self_introduction","evidence":{"source":"transcript","fragment":1,"quote":"我是小云。"},"orders":[1,2,999]}]}]}`)
	var payload struct {
		People []identityProposal `json:"people"`
	}
	require.NoError(t, json.Unmarshal(raw, &payload))
	items, err := decodeIdentitySuggestions(raw, sources)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "林小筠", items[0].DisplayName)
	require.Contains(t, items[0].SourceNames, "小云")
	require.NotContains(t, items[0].Aliases, "小云")
	require.Equal(t, []int{1}, items[0].SpeechOrders)
	// Stable labels are expanded only after the model explicitly reviewed
	// their scope; a listed-fragments decision must never expand implicitly.
	sources.Segments = append(sources.Segments, Segment{Order: 3, SpeakerLabel: "Speaker 1", Text: "接下来我们聊市场。"})
	payload.People[0].SpeechBindings[0].Scope = "stable_speaker"
	stable, err := json.Marshal(payload)
	require.NoError(t, err)
	items, err = decodeIdentitySuggestions(stable, sources)
	require.NoError(t, err)
	require.Equal(t, []int{1, 3}, items[0].SpeechOrders)
	payload.People[0].SpeechBindings[0].ExcludedOrders = []int{3}
	exceptions, err := json.Marshal(payload)
	require.NoError(t, err)
	items, err = decodeIdentitySuggestions(exceptions, sources)
	require.NoError(t, err)
	require.Equal(t, []int{1}, items[0].SpeechOrders)

	payload.People[0].SpeechBindings[0].SpeakerLabel = "Speaker 2"
	wrongAnchor, err := json.Marshal(payload)
	require.NoError(t, err)
	items, err = decodeIdentitySuggestions(wrongAnchor, sources)
	require.NoError(t, err)
	require.Empty(t, items[0].SpeechOrders, "host introduction cannot anchor a guest's speaker label")

	payload.People[0].PresenceEvidence = payload.People[0].NameEvidence
	metadataOnly, err := json.Marshal(payload)
	require.NoError(t, err)
	items, err = decodeIdentitySuggestions(metadataOnly, sources)
	require.NoError(t, err)
	require.Empty(t, items, "podcast author alone is not episode participation")
}

func TestPrepareWithoutRuntimeDoesNotInferPeople(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "no-identity-runtime", Title: "访谈", FeedURL: "https://example.test/no-runtime"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "no-identity-runtime", Title: "访谈"}
	require.NoError(t, db.Create(&ep).Error)
	service, err := NewService(db, nil)
	require.NoError(t, err)
	_, err = service.Prepare(context.Background(), EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", ShowNotes: "嘉宾是战略学家陈明。"})
	require.ErrorIs(t, err, ErrIdentityUnavailable)
	listed, err := service.ListEpisodePeople(context.Background(), ep.ID)
	require.NoError(t, err)
	require.Empty(t, listed.People)
}

func TestLocatedInvitationAndResponseConfirmParticipant(t *testing.T) {
	sources := EpisodeSources{ShowNotes: "本集嘉宾陈明", Segments: []Segment{
		{Order: 1, SpeakerLabel: "主持", Text: "陈明，欢迎参加访谈。"},
		{Order: 2, SpeakerLabel: "嘉宾", Text: "谢谢你邀请我。"},
	}}
	raw := []byte(`{"people":[{"name":"陈明","kind":"participant","status":"confirmed","role":"guest","presence_basis":"introduced_participant","name_evidence":{"source":"show_notes","fragment":0,"quote":"本集嘉宾陈明"},"presence_evidence":{"source":"transcript","fragment":2,"quote":"谢谢你邀请我。"},"role_evidence":{"source":"show_notes","fragment":0,"quote":"本集嘉宾陈明"},"speech_bindings":[{"speaker_label":"嘉宾","basis":"addressed_response","evidence":{"source":"transcript","fragment":2,"quote":"谢谢你邀请我。"},"orders":[2]}]}]}`)
	items, err := decodeIdentitySuggestions(raw, sources)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, StatusConfirmed, items[0].Status)
	require.Equal(t, []int{2}, items[0].SpeechOrders)
	sources.Segments[0].Text = "另一位嘉宾，欢迎。"
	items, err = decodeIdentitySuggestions(raw, sources)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, StatusPending, items[0].Status)
	require.Empty(t, items[0].SpeechOrders)
}

func TestVerifiedShortNameSupportsFullNameParticipation(t *testing.T) {
	sources := EpisodeSources{ShowNotes: "嘉宾 Ada Chen，访谈中称 Ada。"}
	raw := []byte(`{"people":[{"name":"Ada Chen","aliases":["Ada"],"kind":"participant","status":"confirmed","role":"guest","presence_basis":"introduced_participant","name_evidence":{"source":"show_notes","fragment":0,"quote":"嘉宾 Ada Chen"},"presence_evidence":{"source":"show_notes","fragment":0,"quote":"访谈中称 Ada"},"role_evidence":{"source":"show_notes","fragment":0,"quote":"嘉宾 Ada Chen"}}]}`)
	items, err := decodeIdentitySuggestions(raw, sources)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, StatusConfirmed, items[0].Status)
	require.Equal(t, "Ada Chen", items[0].DisplayName)
}

func TestPrepareDoesNotPromoteUnreviewedPhrases(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "identity-phrases", Title: "商业访谈", FeedURL: "https://example.test/identity-phrases"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "identity-phrases", Title: "战略访谈", ShowNotes: "今天的嘉宾是战略学家陈明教授。"}
	require.NoError(t, db.Create(&ep).Error)
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "陈明", Role: RoleGuest, EvidenceKind: "verified_runtime", EvidenceLocator: "show_notes:0:今天的嘉宾是战略学家陈明教授。"}}})
	require.NoError(t, err)
	segments := []Segment{}
	for i, text := range []string{"我是这个观点。", "我是因为纯粹我自己。", "我是敢用的。", "我是觉得绝对要有希望的。", "我是觉得。", "我是比较乐观的。"} {
		segments = append(segments, Segment{Order: i + 1, SpeakerLabel: "Speaker 2", Text: text})
	}
	got, err := service.Prepare(context.Background(), EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", ShowNotes: ep.ShowNotes, Segments: segments})
	require.NoError(t, err)
	require.Equal(t, []string{"陈明"}, names(got.People))
}

func TestPrepareKeepsSemanticHostRole(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "identity-role", Title: "访谈", FeedURL: "https://example.test/identity-role"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "identity-role", Title: "对谈"}
	require.NoError(t, db.Create(&ep).Error)
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "小林", Role: RoleHost, IdentityNote: "访谈主持人", EvidenceKind: "verified_runtime", SpeechOrders: []int{1}}}})
	require.NoError(t, err)
	got, err := service.Prepare(context.Background(), EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, SpeakerLabel: "Speaker 1", Text: "我是小林。今天由我来主持访谈。"}}})
	require.NoError(t, err)
	require.Equal(t, RoleHost, mustPersonByName(t, got, "小林").Role)
}

func TestPrepareSuppliesPublicPodcastContext(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "identity-metadata", Title: "林小筠的访谈", Author: "林小筠", Description: "商业研究访谈", FeedURL: "https://example.test/identity-metadata"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "identity-metadata", Title: "新一期对话", Notes: "private-do-not-identify", PublishedDate: time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC)}
	require.NoError(t, db.Create(&ep).Error)
	var received string
	service, err := NewService(db, captureSourcesSuggester{capture: func(src EpisodeSources) {
		received = fmt.Sprintf("%+v", src)
	}})
	require.NoError(t, err)
	_, err = service.Prepare(context.Background(), EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, SpeakerLabel: "Speaker 1", Text: "我是小云。"}}})
	require.NoError(t, err)
	for _, value := range []string{pod.Title, pod.Author, pod.Description, ep.Title, ep.PublishedDate.Format(time.RFC3339)} {
		require.Contains(t, received, value)
	}
	require.NotContains(t, received, ep.Notes)
}

type captureSourcesSuggester struct{ capture func(EpisodeSources) }

func (s captureSourcesSuggester) Suggest(_ context.Context, src EpisodeSources) ([]SuggestedCandidate, error) {
	s.capture(src)
	return nil, nil
}

// Controlled decisions for existing synthetic persistence tests only.
// This adapter is not Runtime quality evidence and is never wired in production.
type testSourceSuggester struct{}

func (testSourceSuggester) Suggest(_ context.Context, src EpisodeSources) ([]SuggestedCandidate, error) {
	candidates, fragments := extractFromSources(src)
	out := []SuggestedCandidate{}
	for _, c := range candidates {
		item := SuggestedCandidate{DisplayName: c.DisplayName, Aliases: c.Aliases, IdentityNote: c.IdentityNote, Role: c.Role, Status: c.Status, EvidenceKind: "verified_runtime", EvidenceLocator: c.EvidenceLocator, MentionedOnly: c.MentionedOnly}
		// This adapter supplies explicit synthetic fixture decisions, not production
		// semantic inference. The fixture identities are pre-labelled.
		if c.IdentityNote != "" {
			proof, _ := json.Marshal(identityProposal{NameType: "canonical", IdentityAnchor: identityAnchor{Kind: "distinctive_affiliation", Key: c.IdentityNote}})
			item.EvidenceLocator = string(proof)
		}
		for _, f := range fragments {
			if f.PersonName == c.DisplayName && f.Status == StatusConfirmed {
				item.SpeechOrders = append(item.SpeechOrders, f.Order)
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func TestCrossEpisodeAnchorMustBeCanonicalAndSourceBacked(t *testing.T) {
	sources := EpisodeSources{ShowNotes: "本集嘉宾陈明，星河科技产品负责人。", Segments: []Segment{{Order: 1, Text: "我是陈明。", SpeakerLabel: "Speaker 1"}}}
	for _, tc := range []struct {
		label, nameType, quote, key string
		valid                       bool
	}{
		{"valid", "canonical", "本集嘉宾陈明，星河科技产品负责人。", "星河科技产品负责人", true},
		{"fabricated source", "canonical", "陈明是月球公司创始人", "月球公司创始人", false},
		{"missing relation", "canonical", "星河科技产品负责人", "星河科技产品负责人", false},
		{"invented key", "canonical", "本集嘉宾陈明，星河科技产品负责人。", "另一公司负责人", false},
		{"local callname", "episode_callname", "本集嘉宾陈明，星河科技产品负责人。", "星河科技产品负责人", false},
	} {
		t.Run(tc.label, func(t *testing.T) {
			proposal := identityProposal{Name: "陈明", NameType: tc.nameType, Kind: "participant", Status: StatusConfirmed, Role: RoleGuest, PresenceBasis: "self_introduction",
				NameEvidence:     identityEvidence{Source: SourceShowNotes, Quote: sources.ShowNotes},
				PresenceEvidence: identityEvidence{Source: SourceTranscript, Fragment: 1, Quote: "我是陈明。"},
				RoleEvidence:     identityEvidence{Source: SourceShowNotes, Quote: sources.ShowNotes},
				IdentityAnchor:   identityAnchor{Kind: "distinctive_affiliation", Key: tc.key, Evidence: identityEvidence{Source: SourceShowNotes, Quote: tc.quote}},
			}
			raw, err := json.Marshal(map[string]any{"people": []identityProposal{proposal}})
			require.NoError(t, err)
			items, err := decodeIdentitySuggestions(raw, sources)
			require.NoError(t, err)
			require.Len(t, items, 1, "invalid cross-episode evidence does not discard valid episode participation")
			var checked identityProposal
			require.NoError(t, json.Unmarshal([]byte(items[0].EvidenceLocator), &checked))
			require.Equal(t, tc.valid, checked.IdentityAnchor.Key != "")
		})
	}
}

func TestShortFormWithinLocatedIntroductionSupportsLaterAddressedResponse(t *testing.T) {
	sources := EpisodeSources{ShowNotes: "嘉宾陈明教授。", Segments: []Segment{{Order: 1, SpeakerLabel: "A", Text: "今天嘉宾陈明教授。陈教授研究管理学。"}, {Order: 2, SpeakerLabel: "A", Text: "陈教授，您的研究方法是什么？"}, {Order: 3, SpeakerLabel: "B", Text: "我结合理论与实践。"}}}
	raw := []byte(`{"people":[{"name":"陈明","role":"guest","status":"confirmed","kind":"participant","presence_basis":"introduced_participant","name_evidence":{"source":"show_notes","quote":"嘉宾陈明教授。","fragment":0},"presence_evidence":{"source":"transcript","fragment":3,"quote":"我结合理论与实践。"},"role_evidence":{"source":"show_notes","quote":"嘉宾陈明教授。","fragment":0},"source_names":[{"name":"陈教授","evidence":{"source":"transcript","fragment":1,"quote":"今天嘉宾陈明教授。"}}],"speech_bindings":[{"speaker_label":"B","basis":"addressed_response","scope":"stable_speaker","orders":[],"excluded_orders":[],"evidence":{"source":"transcript","fragment":3,"quote":"我结合理论与实践。"}}]}]}`)
	items, err := decodeIdentitySuggestions(raw, sources)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, StatusConfirmed, items[0].Status)
	require.Equal(t, []int{3}, items[0].SpeechOrders)
	var proof identityProposal
	require.NoError(t, json.Unmarshal([]byte(items[0].EvidenceLocator), &proof))
	require.Equal(t, sources.Segments[0].Text, proof.SourceNames[0].Evidence.Quote)
	sources.Segments[0].Text = "今天嘉宾陈明教授。"
	items, err = decodeIdentitySuggestions(raw, sources)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, StatusPending, items[0].Status, "an absent short form cannot be invented")
}

func TestFirstPersonIdentityReferenceBindsOnlyNamedSourceAndKeepsExcludedFragments(t *testing.T) {
	sources := EpisodeSources{ShowNotes: "本集林言主持，与Ada Chen对谈。", Segments: []Segment{
		{Order: 1, SpeakerLabel: "A", Text: "我把自己的资料整理成林言专用资料库，分享给我的团队。"},
		{Order: 2, SpeakerLabel: "A", Text: "我的团队可以继续使用这些资料。"},
		{Order: 3, SpeakerLabel: "A", Text: "我想把它公开。好的。之后再改进。"},
	}}
	raw := []byte(`{"people":[{"name":"林言","kind":"participant","status":"confirmed","role":"host","presence_basis":"introduced_participant","name_evidence":{"source":"show_notes","fragment":0,"quote":"本集林言主持"},"presence_evidence":{"source":"show_notes","fragment":0,"quote":"本集林言主持"},"role_evidence":{"source":"show_notes","fragment":0,"quote":"本集林言主持"},"speech_bindings":[{"speaker_label":"A","basis":"first_person_identity","evidence":{"source":"transcript","fragment":1,"quote":"我把自己的资料整理成林言专用资料库，分享给我的团队。"},"scope":"stable_speaker","orders":[],"excluded_orders":[3]}]}]}`)
	items, err := decodeIdentitySuggestions(raw, sources)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, []int{1, 2}, items[0].SpeechOrders)
	var payload struct {
		People []identityProposal `json:"people"`
	}
	require.NoError(t, json.Unmarshal(raw, &payload))
	// A first-person remark without the person's name is not an identity anchor.
	payload.People[0].SpeechBindings[0].Evidence = identityEvidence{Source: SourceTranscript, Fragment: 2, Quote: sources.Segments[1].Text}
	unnamed, err := json.Marshal(payload)
	require.NoError(t, err)
	items, err = decodeIdentitySuggestions(unnamed, sources)
	require.NoError(t, err)
	require.Empty(t, items[0].SpeechOrders)
}

func TestNamedResponseSurvivesOverSpecificFirstPersonBasis(t *testing.T) {
	sources := EpisodeSources{ShowNotes: "本集嘉宾陈明", Segments: []Segment{{Order: 1, SpeakerLabel: "A", Text: "陈明，你为什么进入这个行业？"}, {Order: 2, SpeakerLabel: "B", Text: "我当时在这家公司的研究部门工作。"}}}
	raw := []byte(`{"people":[{"name":"陈明","kind":"participant","status":"confirmed","role":"guest","presence_basis":"introduced_participant","name_evidence":{"source":"show_notes","fragment":0,"quote":"本集嘉宾陈明"},"presence_evidence":{"source":"show_notes","fragment":0,"quote":"本集嘉宾陈明"},"role_evidence":{"source":"show_notes","fragment":0,"quote":"本集嘉宾陈明"},"speech_bindings":[{"speaker_label":"B","basis":"first_person_identity","evidence":{"source":"transcript","fragment":2,"quote":"我当时在这家公司的研究部门工作。"},"scope":"stable_speaker","orders":[],"excluded_orders":[]}]}]}`)
	got, err := decodeIdentitySuggestions(raw, sources)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []int{2}, got[0].SpeechOrders)
	var proof identityProposal
	require.NoError(t, json.Unmarshal([]byte(got[0].EvidenceLocator), &proof))
	require.Equal(t, "addressed_response", proof.SpeechBindings[0].Basis)
	sources.Segments[0].Text = "我们进入下一个话题。"
	got, err = decodeIdentitySuggestions(raw, sources)
	require.NoError(t, err)
	require.Empty(t, got[0].SpeechOrders)
}
