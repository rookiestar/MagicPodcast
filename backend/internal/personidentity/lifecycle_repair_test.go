package personidentity

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"magicpodcast/internal/contentsearch"
	"magicpodcast/internal/models"
)

func TestLegacyAutomaticIdentityIsNotReliableBeforeRepreparation(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "legacy-identity", Title: "访谈", FeedURL: "https://example.test/legacy"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "legacy-identity", Title: "访谈"}
	require.NoError(t, db.Create(&ep).Error)
	person := models.Person{StableKey: "legacy-person", DisplayName: "旧自动人物"}
	require.NoError(t, db.Create(&person).Error)
	appearance := models.EpisodeAppearance{EpisodeID: ep.ID, PersonID: person.ID, SourceVersion: "v1", Role: RoleGuest, Status: StatusConfirmed, EvidenceKind: "show_notes"}
	require.NoError(t, db.Create(&appearance).Error)
	fragment := models.SpeechAttribution{EpisodeID: ep.ID, PersonID: &person.ID, SourceKind: SourceTranscript, SourceVersion: "v1", FragmentOrder: 1, SpeakerLabel: "Speaker 1", Text: "我是比较谨慎的，投资前应该验证需求。", Status: StatusConfirmed, EvidenceKind: "speaker_label"}
	require.NoError(t, db.Create(&fragment).Error)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	require.NoError(t, search.ReplaceEpisode(context.Background(), contentsearch.EpisodeDocument{EpisodeID: ep.ID, SourceKind: SourceTranscript, SourceVersion: "v1", Complete: true, Fragments: []contentsearch.FragmentInput{{Order: 1, Text: fragment.Text, PersonID: &person.ID, AttributionStatus: StatusConfirmed}}}))
	service, err := NewService(db, nil, search)
	require.NoError(t, err)
	listed, err := service.ListEpisodePeople(context.Background(), ep.ID)
	require.NoError(t, err)
	require.Empty(t, listed.People, "unreviewed legacy automatic candidates are not selectable people")
	require.False(t, listed.IndexReady)
	reliable, err := service.ReliableSpeech(context.Background(), ep.ID, person.ID)
	require.NoError(t, err)
	require.Empty(t, reliable)
	q := contentsearch.Request{Query: "投资", Scope: contentsearch.Scope{EpisodeIDs: []uint{ep.ID}}, Filter: contentsearch.Filter{PersonID: &person.ID}}
	hits, err := search.Search(context.Background(), q)
	require.NoError(t, err)
	require.Empty(t, hits.Hits)
	q.Filter.PersonID = nil
	general, err := search.Search(context.Background(), q)
	require.NoError(t, err)
	require.NotEmpty(t, general.Hits, "current original text remains generally searchable")
	confirmed, err := service.CorrectName(context.Background(), ep.ID, NameCorrection{PersonID: person.ID, DisplayName: "林言"})
	require.NoError(t, err)
	require.Len(t, confirmed.People, 1)
	require.Zero(t, confirmed.People[0].ConfirmedSpeech, "confirming a name must not confirm old automatic speech")
	_, err = service.CorrectAttribution(context.Background(), ep.ID, AttributionCorrection{SourceVersion: "v1", FragmentOrder: 1, AssignedPersonID: &person.ID, Status: StatusConfirmed})
	require.NoError(t, err)
	q.Filter.PersonID = &person.ID
	manual, err := search.Search(context.Background(), q)
	require.NoError(t, err)
	require.NotEmpty(t, manual.Hits, "explicit matching human confirmations remain usable")
	reliable, err = service.ReliableSpeech(context.Background(), ep.ID, person.ID)
	require.NoError(t, err)
	require.Len(t, reliable, 1)
}

func TestSameSourceReprepareReplacesAutomaticPeopleAndPreservesConfirmation(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "replace-candidates", Title: "访谈", FeedURL: "https://example.test/replace"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "replace-candidates", Title: "访谈", ShowNotes: "主持林言，嘉宾陈明。"}
	require.NoError(t, db.Create(&ep).Error)
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{
		{DisplayName: "战略学", Role: RoleGuest, EvidenceKind: "verified_runtime"},
		{DisplayName: "林言", Role: RoleHost, EvidenceKind: "verified_runtime"},
	}})
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", ShowNotes: ep.ShowNotes, Segments: []Segment{{Order: 1, SpeakerLabel: "Speaker 1", Text: "欢迎来参加访谈。"}}}
	before, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	host := mustPersonByName(t, before, "林言")
	_, err = service.CorrectName(context.Background(), ep.ID, NameCorrection{PersonID: host.ID, DisplayName: "林老师"})
	require.NoError(t, err)
	service.suggester = stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "陈明", Role: RoleGuest, EvidenceKind: "verified_runtime"}}}
	after, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"战略学", "林老师", "陈明"}, names(after.People))
	require.Equal(t, host.ID, mustPersonByName(t, after, "林老师").ID)
	again, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.ElementsMatch(t, names(after.People), names(again.People))
	require.Equal(t, mustPersonByName(t, after, "陈明").ID, mustPersonByName(t, again, "陈明").ID)
	var count int64
	require.NoError(t, db.Model(&models.PersonUserConfirmation{}).Where("episode_id = ?", ep.ID).Count(&count).Error)
	require.EqualValues(t, 3, count, "existing user-approved identities are preserved")
}

func TestMetadataInvalidationPreservesOriginalSearch(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "metadata-invalidation", Title: "访谈", Author: "林言", FeedURL: "https://example.test/metadata"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "metadata-invalidation", Title: "投资访谈", ShowNotes: "本集主持林言。"}
	require.NoError(t, db.Create(&ep).Error)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "林言", Role: RoleHost, EvidenceKind: "verified_runtime", SpeechOrders: []int{1}}}}, search)
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, SpeakerLabel: "Speaker 1", Text: "投资前应该验证需求。"}}}
	before, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	id := before.People[0].ID
	q := contentsearch.Request{Query: "投资", Scope: contentsearch.Scope{EpisodeIDs: []uint{ep.ID}}, Filter: contentsearch.Filter{PersonID: &id}}
	initial, err := search.Search(context.Background(), q)
	require.NoError(t, err)
	require.NotEmpty(t, initial.Hits)
	require.NoError(t, db.Model(&pod).Update("author", "制作团队").Error)
	after, err := service.ListEpisodePeople(context.Background(), ep.ID)
	require.NoError(t, err)
	require.Len(t, after.People, 1, "explicit human attribution survives metadata-only changes")
	require.False(t, after.IndexReady)
	personal, err := search.Search(context.Background(), q)
	require.NoError(t, err)
	require.NotEmpty(t, personal.Hits)
	require.False(t, personal.Coverage.Complete)
	q.Filter.PersonID = nil
	general, err := search.Search(context.Background(), q)
	require.NoError(t, err)
	require.NotEmpty(t, general.Hits)
	for _, hit := range general.Hits {
		require.Equal(t, &id, hit.PersonID)
	}
}

type decisionSuggester func(context.Context, EpisodeSources) ([]SuggestedCandidate, error)

func (f decisionSuggester) suggestCandidates(ctx context.Context, src EpisodeSources) ([]SuggestedCandidate, error) {
	return f(ctx, src)
}

func TestSourceChangeDuringRuntimeDoesNotPublishOldDecision(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "source-during-runtime", Title: "访谈", Author: "林言", FeedURL: "https://example.test/source-change"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "source-during-runtime", Title: "投资访谈"}
	require.NoError(t, db.Create(&ep).Error)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	changeSource := false
	service, err := NewService(db, decisionSuggester(func(context.Context, EpisodeSources) ([]SuggestedCandidate, error) {
		name := "林言"
		if changeSource {
			if err := db.Model(&pod).Update("author", "制作团队").Error; err != nil {
				return nil, err
			}
			name = "不应发布的旧决定"
		}
		return []SuggestedCandidate{{DisplayName: name, Role: RoleHost, EvidenceKind: "verified_runtime", SpeechOrders: []int{1}}}, nil
	}), search)
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, SpeakerLabel: "Speaker 1", Text: "投资前应该验证需求。"}}}
	initial, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	changeSource = true
	_, err = prepareReviewed(service, context.Background(), src)
	require.ErrorIs(t, err, ErrSourcesChanged)
	var count int64
	require.NoError(t, db.Model(&models.Person{}).Where("display_name = ?", "不应发布的旧决定").Count(&count).Error)
	require.Zero(t, count)
	after, err := service.ListEpisodePeople(context.Background(), ep.ID)
	require.NoError(t, err)
	require.Len(t, after.People, 1)
	hits, err := search.Search(context.Background(), contentsearch.Request{Query: "投资", Scope: contentsearch.Scope{EpisodeIDs: []uint{ep.ID}}, Filter: contentsearch.Filter{PersonID: &initial.People[0].ID}})
	require.NoError(t, err)
	require.NotEmpty(t, hits.Hits, "prior user confirmation remains valid")
}

func TestPreparationDecisionCannotOverwriteNewerFacts(t *testing.T) {
	for _, action := range []string{"newer preparation", "manual correction", "runtime failure", "cancellation"} {
		t.Run(action, func(t *testing.T) {
			db := openPersonIdentityDB(t)
			pod := models.Podcast{XYZID: "revision", Title: "访谈", FeedURL: "https://example.test/revision"}
			require.NoError(t, db.Create(&pod).Error)
			ep := models.Episode{PodcastID: pod.ID, GUID: "revision", Title: "投资访谈"}
			require.NoError(t, db.Create(&ep).Error)
			candidate := func(name string) stubSuggester {
				return stubSuggester{candidates: []SuggestedCandidate{{DisplayName: name, Role: RoleHost, EvidenceKind: "verified_runtime", SpeechOrders: []int{1}}}}
			}
			service, err := NewService(db, candidate("林言"))
			require.NoError(t, err)
			src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, SpeakerLabel: "Speaker 1", Text: "投资前应该验证需求。"}}}
			initial, err := prepareReviewed(service, context.Background(), src)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			expected := "林言"
			service.suggester = decisionSuggester(func(context.Context, EpisodeSources) ([]SuggestedCandidate, error) {
				current, err := service.ListEpisodePeople(context.Background(), ep.ID)
				require.NoError(t, err)
				require.Equal(t, names(initial.People), names(current.People), "in-flight request preserves published facts")
				switch action {
				case "newer preparation":
					newer, err := NewService(db, candidate("陈明"))
					require.NoError(t, err)
					_, err = prepareReviewed(newer, context.Background(), src)
					require.NoError(t, err)
					expected = "陈明"
				case "manual correction":
					_, err := service.CorrectName(context.Background(), ep.ID, NameCorrection{PersonID: initial.People[0].ID, DisplayName: "林老师"})
					require.NoError(t, err)
					expected = "林老师"
				case "runtime failure":
					return nil, ErrIdentityUnavailable
				case "cancellation":
					cancel()
				}
				return candidate("迟到的旧决定").candidates, nil
			})
			_, err = prepareReviewed(service, ctx, src)
			switch action {
			case "runtime failure":
				require.ErrorIs(t, err, ErrIdentityUnavailable)
			case "cancellation":
				require.ErrorIs(t, err, context.Canceled)
			default:
				require.ErrorIs(t, err, ErrSourcesChanged)
			}
			after, err := service.ListEpisodePeople(context.Background(), ep.ID)
			require.NoError(t, err)
			require.Equal(t, expected, after.Attributions[0].DisplayName)
			var count int64
			require.NoError(t, db.Model(&models.Person{}).Where("display_name = ?", "迟到的旧决定").Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestLateIndexCannotReplaceNewerPublishedOrManualFacts(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "index-revision", Title: "访谈", FeedURL: "https://example.test/index-revision"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "index-revision", Title: "投资访谈"}
	require.NoError(t, db.Create(&ep).Error)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	candidate := func(name string) stubSuggester {
		return stubSuggester{candidates: []SuggestedCandidate{{DisplayName: name, Role: RoleHost, EvidenceKind: "verified_runtime", SpeechOrders: []int{1}}}}
	}
	service, err := NewService(db, candidate("林言"), search)
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, Text: "投资前验证需求。", SpeakerLabel: "Speaker 1"}}}
	_, err = prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	var during EpisodePeople
	service.suggester = decisionSuggester(func(ctx context.Context, _ EpisodeSources) ([]SuggestedCandidate, error) {
		var err error
		during, err = service.ListEpisodePeople(ctx, ep.ID)
		require.NoError(t, err)
		return candidate("陈明").candidates, nil
	})
	after, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Greater(t, after.preparationRevision, during.preparationRevision, "explicit application advances the revision")
	require.NotEqual(t, during.publishedRevision, after.publishedRevision)
	require.ErrorIs(t, service.reindexSearch(context.Background(), ep.ID, during), contentsearch.ErrStaleDocument)
	personID := *after.Attributions[0].PersonID
	q := contentsearch.Request{Query: "投资", Scope: contentsearch.Scope{EpisodeIDs: []uint{ep.ID}}, Filter: contentsearch.Filter{PersonID: &personID}}
	hits, err := search.Search(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, hits.Hits, 1)
	_, err = service.CorrectAttribution(context.Background(), ep.ID, AttributionCorrection{SourceVersion: "v1", FragmentOrder: 1, Status: StatusRejected})
	require.NoError(t, err)
	require.ErrorIs(t, service.reindexSearch(context.Background(), ep.ID, after), contentsearch.ErrStaleDocument)
	hits, err = search.Search(context.Background(), q)
	require.NoError(t, err)
	require.Empty(t, hits.Hits)
	q.Filter.PersonID = nil
	hits, err = search.Search(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, hits.Hits, 1, "rejected identity does not remove original searchable text")
}

func TestNewVersionReviewPreservesOldFragmentsAndConfirmationHistory(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "replace-fragments", Title: "访谈", FeedURL: "https://example.test/replace-fragments"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "replace-fragments", Title: "访谈"}
	require.NoError(t, db.Create(&ep).Error)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "林言", Role: RoleHost, EvidenceKind: "verified_runtime", SpeechOrders: []int{1, 2}}}}, search)
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, Text: "保留的投资观点。"}, {Order: 2, Text: "删除的航天观点。"}}}
	first, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	_, err = service.CorrectAttribution(context.Background(), ep.ID, AttributionCorrection{SourceVersion: "v1", FragmentOrder: 2, AssignedPersonID: &first.People[0].ID, Status: StatusConfirmed})
	require.NoError(t, err)
	src.Segments = src.Segments[:1]
	src.SourceVersion = "v2"
	after, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Len(t, after.Attributions, 1)
	var count int64
	require.NoError(t, db.Model(&models.PersonUserConfirmation{}).Where("episode_id = ?", ep.ID).Count(&count).Error)
	require.GreaterOrEqual(t, count, int64(3))
	result, err := search.Search(context.Background(), contentsearch.Request{Query: "航天", Scope: contentsearch.Scope{EpisodeIDs: []uint{ep.ID}}})
	require.NoError(t, err)
	require.Empty(t, result.Hits)
}

func TestNameConfirmationDoesNotFreezeAutomaticRole(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "name-not-role", Title: "访谈", FeedURL: "https://example.test/name-not-role"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "name-not-role", Title: "访谈"}
	require.NoError(t, db.Create(&ep).Error)
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "林言", Role: RoleUnknown, EvidenceKind: "verified_runtime"}}})
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, Text: "欢迎来到节目。"}}}
	initial, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	_, err = service.CorrectName(context.Background(), ep.ID, NameCorrection{PersonID: initial.People[0].ID, DisplayName: "林言"})
	require.NoError(t, err)
	var confirmation models.PersonUserConfirmation
	require.NoError(t, db.Where("episode_id = ?", ep.ID).First(&confirmation).Error)
	require.Empty(t, confirmation.Role, "name-only correction does not write a role decision")
	// Published older versions copied the automatic role into name confirmations.
	// Such rows still confirm the name, but never constitute a manual role choice.
	require.NoError(t, db.Model(&confirmation).Update("role", RoleUnknown).Error)
	service.suggester = stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "林言", Role: RoleHost, EvidenceKind: "verified_runtime"}}}
	after, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, initial.People[0].ID, after.People[0].ID)
	require.Equal(t, RoleHost, after.People[0].Role)
	require.Zero(t, after.People[0].ConfirmedSpeech)
}

func TestAppearanceCorrectionsPersistIndependentlyAndExclusionIsReversible(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "appearance-override", Title: "访谈", FeedURL: "https://example.test/appearance-override"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "appearance-override", Title: "访谈"}
	require.NoError(t, db.Create(&ep).Error)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "林言", Role: RoleUnknown, EvidenceKind: "verified_runtime", SpeechOrders: []int{1}}}}, search)
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, Text: "投资需要验证需求。"}}}
	initial, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	id := initial.People[0].ID
	role := RoleGuest
	corrected, err := service.CorrectAppearance(context.Background(), ep.ID, AppearanceCorrection{PersonID: id, Role: &role})
	require.NoError(t, err)
	require.Equal(t, RoleGuest, corrected.People[0].Role)
	require.True(t, corrected.People[0].RoleUserConfirmed)
	var confirmations int64
	require.NoError(t, db.Model(&models.PersonUserConfirmation{}).Count(&confirmations).Error)
	require.EqualValues(t, 2, confirmations, "a role correction adds neither name nor speech confirmation")
	excluded := true
	hidden, err := service.CorrectAppearance(context.Background(), ep.ID, AppearanceCorrection{PersonID: id, Excluded: &excluded})
	require.NoError(t, err)
	require.Empty(t, hidden.People)
	require.Len(t, hidden.ExcludedPeople, 1)
	q := contentsearch.Request{Query: "投资", Scope: contentsearch.Scope{EpisodeIDs: []uint{ep.ID}}, Filter: contentsearch.Filter{PersonID: &id}}
	hits, err := search.Search(context.Background(), q)
	require.NoError(t, err)
	require.Empty(t, hits.Hits)
	general, err := search.Search(context.Background(), contentsearch.Request{Query: "投资", Scope: q.Scope})
	require.NoError(t, err)
	require.Len(t, general.Hits, 1, "excluding a person must preserve searchable original text")
	require.Nil(t, general.Hits[0].PersonID)
	speech, err := service.ReliableSpeech(context.Background(), ep.ID, id)
	require.NoError(t, err)
	require.Empty(t, speech)
	rebuilt, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Empty(t, rebuilt.People)
	require.Len(t, rebuilt.ExcludedPeople, 1)
	require.Equal(t, id, rebuilt.ExcludedPeople[0].ID)
	excluded = false
	restored, err := service.CorrectAppearance(context.Background(), ep.ID, AppearanceCorrection{PersonID: id, Excluded: &excluded})
	require.NoError(t, err)
	require.Empty(t, restored.ExcludedPeople)
	require.Len(t, restored.People, 1)
	require.Equal(t, RoleGuest, restored.People[0].Role)
	hits, err = search.Search(context.Background(), q)
	require.NoError(t, err)
	require.Len(t, hits.Hits, 1)
	service.suggester = stubSuggester{}
	missing, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Len(t, missing.People, 1)
	require.Equal(t, RoleGuest, missing.People[0].Role, "the manual role remains saved")
	require.Equal(t, StatusConfirmed, missing.People[0].Status, "an empty new suggestion cannot erase approved identity")
	require.Equal(t, 1, missing.People[0].ConfirmedSpeech)

}

func TestIndexFailureRollsBackIdentityPublicationAndCorrections(t *testing.T) {
	for _, operation := range []string{"prepare", "name", "attribution", "appearance"} {
		t.Run(operation, func(t *testing.T) {
			db := openPersonIdentityDB(t)
			pod := models.Podcast{XYZID: "atomic-index", Title: "访谈", FeedURL: "https://example.test/atomic-index"}
			require.NoError(t, db.Create(&pod).Error)
			ep := models.Episode{PodcastID: pod.ID, GUID: "atomic-index", Title: "访谈"}
			require.NoError(t, db.Create(&ep).Error)
			search, err := contentsearch.NewService(db)
			require.NoError(t, err)
			service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "林言", Role: RoleHost, EvidenceKind: "verified_runtime", SpeechOrders: []int{1}}}}, search)
			require.NoError(t, err)
			src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, Text: "投资前验证需求。"}}}
			initial, err := prepareReviewed(service, context.Background(), src)
			require.NoError(t, err)
			id := initial.People[0].ID
			// Inject a real SQLite index-write failure, including the UPSERT path.
			require.NoError(t, db.Exec(`CREATE TRIGGER fail_identity_index BEFORE INSERT ON content_search_fragments BEGIN SELECT RAISE(ABORT, 'injected index failure'); END`).Error)
			switch operation {
			case "prepare":
				service.suggester = stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "陈明", Role: RoleGuest, EvidenceKind: "verified_runtime", SpeechOrders: []int{1}}}}
				_, err = prepareReviewed(service, context.Background(), src)
			case "name":
				_, err = service.CorrectName(context.Background(), ep.ID, NameCorrection{PersonID: id, DisplayName: "林老师"})
			case "attribution":
				_, err = service.CorrectAttribution(context.Background(), ep.ID, AttributionCorrection{SourceVersion: "v1", FragmentOrder: 1, Status: StatusRejected})
			case "appearance":
				excluded := true
				_, err = service.CorrectAppearance(context.Background(), ep.ID, AppearanceCorrection{PersonID: id, Excluded: &excluded})
			}
			require.ErrorContains(t, err, "injected index failure")
			after, err := service.ListEpisodePeople(context.Background(), ep.ID)
			require.NoError(t, err)
			require.Equal(t, initial.People, after.People)
			require.Equal(t, initial.Attributions, after.Attributions)
			require.Empty(t, after.ExcludedPeople)
			require.True(t, after.IndexReady)
			var count int64
			require.NoError(t, db.Model(&models.PersonUserConfirmation{}).Count(&count).Error)
			require.EqualValues(t, 2, count)
			require.NoError(t, db.Model(&models.PersonAppearanceOverride{}).Count(&count).Error)
			require.Zero(t, count)
			require.NoError(t, db.Model(&models.Person{}).Where("display_name = ?", "陈明").Count(&count).Error)
			require.Zero(t, count)
			hits, err := search.Search(context.Background(), contentsearch.Request{Query: "投资", Scope: contentsearch.Scope{EpisodeIDs: []uint{ep.ID}}, Filter: contentsearch.Filter{PersonID: &id}})
			require.NoError(t, err)
			require.Len(t, hits.Hits, 1)
			require.NoError(t, db.Exec("DROP TRIGGER fail_identity_index").Error)
			_, err = prepareReviewed(service, context.Background(), src)
			require.NoError(t, err, "the failed operation remains retryable")
		})
	}
}

func TestCrossEpisodeIdentityRequiresDistinctiveEvidence(t *testing.T) {
	for _, tc := range []struct {
		label, name, note, firstKey, secondKey, nameType string
		same                                             bool
	}{
		{"generic callname", "王老师", "嘉宾，资料未提供全名", "", "", "episode_callname", false},
		{"same career", "陈明", "播客主播、访谈者", "", "", "canonical", false},
		{"canonical affiliation", "陈明", "产品负责人", "星河科技产品负责人", "星河科技产品负责人", "canonical", true},
		{"different affiliation", "陈明", "产品负责人", "星河科技产品负责人", "星空科技产品负责人", "canonical", false},
		{"callname with affiliation", "王老师", "产品负责人", "星河科技产品负责人", "星河科技产品负责人", "episode_callname", false},
	} {
		t.Run(tc.label, func(t *testing.T) {
			db := openPersonIdentityDB(t)
			pod := models.Podcast{XYZID: "cross-identity", Title: "访谈", FeedURL: "https://example.test/cross-identity"}
			require.NoError(t, db.Create(&pod).Error)
			var ids []uint
			for i, key := range []string{tc.firstKey, tc.secondKey} {
				ep := models.Episode{PodcastID: pod.ID, GUID: fmt.Sprintf("cross-%d", i), Title: "访谈"}
				require.NoError(t, db.Create(&ep).Error)
				proof, err := json.Marshal(identityProposal{NameType: tc.nameType, IdentityAnchor: identityAnchor{Key: key, Kind: "distinctive_affiliation"}})
				require.NoError(t, err)
				service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{{DisplayName: tc.name, IdentityNote: tc.note, Role: RoleGuest, EvidenceKind: "verified_runtime", EvidenceLocator: string(proof)}}})
				require.NoError(t, err)
				src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, Text: "欢迎。"}}}
				listed, err := prepareReviewed(service, context.Background(), src)
				require.NoError(t, err)
				require.Len(t, listed.People, 1)
				again, err := prepareReviewed(service, context.Background(), src)
				require.NoError(t, err)
				require.Equal(t, listed.People[0].ID, again.People[0].ID, "same-episode rebuilding remains idempotent")
				ids = append(ids, listed.People[0].ID)
			}
			require.Equal(t, tc.same, ids[0] == ids[1])
		})
	}
}

func TestConflictingNameCorrectionDoesNotRenameOtherEpisodes(t *testing.T) {
	lib := seedBaselineLibrary(t, nil)
	targetID := lib.episodeIDs["ep-product-host"]
	otherID := lib.episodeIDs["ep-startup-guest"]
	before, err := lib.service.ListEpisodePeople(context.Background(), targetID)
	require.NoError(t, err)
	otherBefore, err := lib.service.ListEpisodePeople(context.Background(), otherID)
	require.NoError(t, err)
	original := mustPersonByName(t, before, "王芳")
	require.Equal(t, original.ID, mustPersonByName(t, otherBefore, "王芳").ID)
	role := RoleGuest
	_, err = lib.service.CorrectAppearance(context.Background(), targetID, AppearanceCorrection{PersonID: original.ID, Role: &role})
	require.NoError(t, err)
	first := before.Attributions[0]
	_, err = lib.service.CorrectAttribution(context.Background(), targetID, AttributionCorrection{SourceVersion: before.SourceVersion, FragmentOrder: first.FragmentOrder, AssignedPersonID: &original.ID, Status: StatusConfirmed})
	require.NoError(t, err)
	corrected, err := lib.service.CorrectName(context.Background(), targetID, NameCorrection{PersonID: original.ID, DisplayName: "王芳芳", IdentityNote: "本集实际出场的另一位产品负责人"})
	require.NoError(t, err)
	renamed := mustPersonByName(t, corrected, "王芳芳")
	require.NotEqual(t, original.ID, renamed.ID, "conflicting shared identity is separated only for this episode")
	require.NotContains(t, renamed.Aliases, "王芳", "a name correction is not a global alias assertion")
	require.Equal(t, RoleGuest, renamed.Role)
	require.True(t, renamed.RoleUserConfirmed)
	require.Equal(t, renamed.ID, *attributionByOrder(t, corrected, first.FragmentOrder).PersonID)
	require.True(t, attributionByOrder(t, corrected, first.FragmentOrder).UserConfirmed)
	otherAfter, err := lib.service.ListEpisodePeople(context.Background(), otherID)
	require.NoError(t, err)
	require.Equal(t, otherBefore.People, otherAfter.People)
	require.Equal(t, otherBefore.Attributions, otherAfter.Attributions)
	again, err := lib.service.CorrectName(context.Background(), targetID, NameCorrection{PersonID: renamed.ID, DisplayName: "王芳芳"})
	require.NoError(t, err)
	require.Equal(t, renamed.ID, mustPersonByName(t, again, "王芳芳").ID)
	src := EpisodeSources{EpisodeID: targetID, SourceVersion: before.SourceVersion}
	for _, fragment := range before.Attributions {
		src.Segments = append(src.Segments, Segment{Order: fragment.FragmentOrder, SpeakerLabel: fragment.SpeakerLabel, StartMS: fragment.StartMS, Text: fragment.Text})
	}
	rebuilt, err := prepareReviewed(lib.service, context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, renamed.ID, mustPersonByName(t, rebuilt, "王芳芳").ID, "rebuilding must not re-merge the corrected episode into the old global identity")
	require.NotContains(t, names(rebuilt.People), "王芳")
	var confirmations []models.PersonUserConfirmation
	require.NoError(t, lib.db.Where("episode_id = ?", targetID).Find(&confirmations).Error)
	for _, confirmation := range confirmations {
		if confirmation.Kind == models.PersonConfirmationKindName {
			require.Equal(t, renamed.ID, *confirmation.PersonID)
		}
		if confirmation.Kind == models.PersonConfirmationKindAttribution {
			require.Equal(t, renamed.ID, *confirmation.AssignedPersonID)
		}
	}
}

func TestHomophonousSourceNameDoesNotMergeDistinctCanonicalParticipants(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "homophones", Title: "访谈", FeedURL: "https://example.test/homophones"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "homophones", Title: "同音不同人", ShowNotes: "嘉宾李景，明河科技产品经理；嘉宾李璟，东山大学历史教授。"}
	require.NoError(t, db.Create(&ep).Error)
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{
		{DisplayName: "李景", SourceNames: []string{"李景"}, IdentityNote: "明河科技产品经理", Role: RoleGuest, Status: StatusConfirmed, EvidenceKind: "verified_runtime", SpeechOrders: []int{1}},
		{DisplayName: "李璟", SourceNames: []string{"李景"}, IdentityNote: "东山大学历史教授", Role: RoleGuest, Status: StatusConfirmed, EvidenceKind: "verified_runtime", SpeechOrders: []int{2}},
	}})
	require.NoError(t, err)
	sources := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "fixture-homophones", Segments: []Segment{{Order: 1, SpeakerLabel: "A", Text: "我是李景，在明河科技负责产品。"}, {Order: 2, SpeakerLabel: "B", Text: "我是李景，在东山大学教历史。"}}}
	first, err := prepareReviewed(service, context.Background(), sources)
	require.NoError(t, err)
	require.Len(t, first.People, 2)
	product := mustPersonByName(t, first, "李景")
	professor := mustPersonByName(t, first, "李璟")
	require.NotEqual(t, product.ID, professor.ID)
	require.Equal(t, product.ID, *first.Attributions[0].PersonID)
	require.Equal(t, professor.ID, *first.Attributions[1].PersonID)
	again, err := prepareReviewed(service, context.Background(), sources)
	require.NoError(t, err)
	require.Len(t, again.People, 2)
	require.Equal(t, product.ID, mustPersonByName(t, again, "李景").ID)
	require.Equal(t, professor.ID, mustPersonByName(t, again, "李璟").ID)
}

func TestRoleSuggestionsDoNotOverwriteApprovedRole(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "role-retention", Title: "访谈", FeedURL: "https://example.test/role-retention"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "role-retention", Title: "对话", ShowNotes: "本集主播林言，本集访谈参与者林言。"}
	require.NoError(t, db.Create(&ep).Error)
	candidate := func(role, quote string) SuggestedCandidate {
		proof, _ := json.Marshal(identityProposal{Name: "林言", Role: role, RoleEvidence: identityEvidence{Source: SourceShowNotes, Quote: quote}})
		return SuggestedCandidate{DisplayName: "林言", Role: role, Status: StatusConfirmed, EvidenceKind: "verified_runtime", EvidenceLocator: string(proof)}
	}
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{candidate(RoleHost, "本集主播林言")}})
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "fixture-role-retention", Segments: []Segment{{Order: 1, SpeakerLabel: "A", Text: "欢迎来到节目。"}}}
	first, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	service.suggester = stubSuggester{candidates: []SuggestedCandidate{candidate(RoleUnknown, "")}}
	again, err := service.Prepare(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, RoleHost, again.People[0].Role)
	var proof identityProposal
	require.NoError(t, json.Unmarshal([]byte(again.People[0].EvidenceLocator), &proof))
	require.Equal(t, "本集主播林言", proof.RoleEvidence.Quote)
	service.suggester = stubSuggester{candidates: []SuggestedCandidate{candidate(RoleGuest, "本集访谈参与者林言")}}
	conflict, err := service.Prepare(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, RoleHost, conflict.People[0].Role)
	require.Equal(t, StatusConfirmed, conflict.People[0].Status)
	require.Equal(t, RoleGuest, conflict.Draft.Matches[0].Role, "new role awaits user review")
	service.suggester = stubSuggester{candidates: []SuggestedCandidate{candidate(RoleHost, "本集主播林言")}}
	conflict, err = service.Prepare(context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, RoleHost, conflict.People[0].Role)
	role := RoleHost
	fixed, err := service.CorrectAppearance(context.Background(), ep.ID, AppearanceCorrection{PersonID: first.People[0].ID, Role: &role})
	require.NoError(t, err)
	require.Equal(t, RoleHost, fixed.People[0].Role)
	require.True(t, fixed.People[0].RoleUserConfirmed)
}

func TestAutomaticRoleRetentionDoesNotCrossChangedMetadata(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "role-source", Title: "访谈", FeedURL: "https://example.test/role-source"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "role-source", Title: "对话", ShowNotes: "本集主播林言"}
	require.NoError(t, db.Create(&ep).Error)
	proof, _ := json.Marshal(identityProposal{Name: "林言", Role: RoleHost, RoleEvidence: identityEvidence{Source: SourceShowNotes, Quote: "本集主播林言"}})
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "林言", Role: RoleHost, Status: StatusConfirmed, EvidenceKind: "verified_runtime", EvidenceLocator: string(proof)}}})
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "fixture-role-source", Segments: []Segment{{Order: 1, SpeakerLabel: "A", Text: "欢迎来到节目。"}}}
	_, err = prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.NoError(t, db.Model(&ep).Update("show_notes", "本集参与者林言，角色未确认。").Error)
	unknown, _ := json.Marshal(identityProposal{Name: "林言", Role: RoleUnknown})
	service.suggester = stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "林言", Role: RoleUnknown, Status: StatusConfirmed, EvidenceKind: "verified_runtime", EvidenceLocator: string(unknown)}}}
	got, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, RoleUnknown, got.People[0].Role)
}

func TestAutomaticRoleRetentionRequiresItsQuoteInCurrentTranscript(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "role-text", Title: "访谈", FeedURL: "https://example.test/role-text"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "role-text", Title: "对话", ShowNotes: "本集参与者林言"}
	require.NoError(t, db.Create(&ep).Error)
	proof, _ := json.Marshal(identityProposal{Name: "林言", Role: RoleHost, RoleEvidence: identityEvidence{Source: SourceTranscript, Fragment: 1, Quote: "我是主持林言。"}})
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "林言", Role: RoleHost, Status: StatusConfirmed, EvidenceKind: "verified_runtime", EvidenceLocator: string(proof)}}})
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "fixture-role-text", Segments: []Segment{{Order: 1, SpeakerLabel: "A", Text: "我是主持林言。"}}}
	_, err = prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	unknown, _ := json.Marshal(identityProposal{Name: "林言", Role: RoleUnknown})
	service.suggester = stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "林言", Role: RoleUnknown, Status: StatusConfirmed, EvidenceKind: "verified_runtime", EvidenceLocator: string(unknown)}}}
	src.Segments[0].Text = "本期我的角色没有明确说明。"
	got, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, RoleUnknown, got.People[0].Role)
}

func TestSameEpisodeNamesKeepDistinctSourceBackedIdentities(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "same-name-episode", Title: "访谈", FeedURL: "https://example.test/same-name-episode"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "same-name-episode", Title: "两位同名嘉宾", ShowNotes: "本集嘉宾王芳，甲公司产品负责人；另一位嘉宾王芳，乙大学历史教授。"}
	require.NoError(t, db.Create(&ep).Error)
	makeCandidate := func(note string, order int) SuggestedCandidate {
		proof, _ := json.Marshal(identityProposal{Name: "王芳", NameType: "canonical", IdentityAnchor: identityAnchor{Key: note, Kind: "distinctive_affiliation", Evidence: identityEvidence{Source: SourceShowNotes, Quote: "王芳，" + note}}})
		return SuggestedCandidate{DisplayName: "王芳", IdentityNote: note, Role: RoleGuest, Status: StatusConfirmed, EvidenceKind: "verified_runtime", EvidenceLocator: string(proof), SpeechOrders: []int{order}}
	}
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{makeCandidate("甲公司产品负责人", 1), makeCandidate("乙大学历史教授", 2)}})
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "fixture-same-name-episode", Segments: []Segment{{Order: 1, SpeakerLabel: "A", Text: "我是王芳，在甲公司负责产品。"}, {Order: 2, SpeakerLabel: "B", Text: "我也叫王芳，在乙大学研究历史。"}}}
	first, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Len(t, first.People, 2)
	require.NotEqual(t, first.People[0].ID, first.People[1].ID)
	require.NotNil(t, first.Attributions[0].PersonID)
	require.NotNil(t, first.Attributions[1].PersonID)
	require.NotEqual(t, *first.Attributions[0].PersonID, *first.Attributions[1].PersonID)
	again, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Len(t, again.People, 2)
	require.Equal(t, *first.Attributions[0].PersonID, *again.Attributions[0].PersonID)
	require.Equal(t, *first.Attributions[1].PersonID, *again.Attributions[1].PersonID)
	id := *first.Attributions[0].PersonID
	excluded := true
	_, err = service.CorrectAppearance(context.Background(), ep.ID, AppearanceCorrection{PersonID: id, Excluded: &excluded})
	require.NoError(t, err)
	rebuilt, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Len(t, rebuilt.People, 1, "the excluded namesake must not return under a new ID")
	require.Equal(t, *first.Attributions[1].PersonID, rebuilt.People[0].ID)
	require.Len(t, rebuilt.ExcludedPeople, 1)
	require.Equal(t, id, rebuilt.ExcludedPeople[0].ID)
	// Re-identification after an algorithm upgrade must retain the same exclusion.
	require.NoError(t, db.Model(&models.PersonPreparation{}).Where("episode_id = ?", ep.ID).Update("algorithm_version", "previous-algorithm").Error)
	rebuilt, err = prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Len(t, rebuilt.People, 1)
	require.Len(t, rebuilt.ExcludedPeople, 1)
	require.Equal(t, id, rebuilt.ExcludedPeople[0].ID)
	src.SourceVersion = "fixture-same-name-episode-v2"
	rebuilt, err = prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Len(t, rebuilt.People, 1)
	require.Len(t, rebuilt.ExcludedPeople, 1)
	require.Equal(t, id, rebuilt.ExcludedPeople[0].ID)
	excluded = false
	restored, err := service.CorrectAppearance(context.Background(), ep.ID, AppearanceCorrection{PersonID: id, Excluded: &excluded})
	require.NoError(t, err)
	require.Len(t, restored.People, 2)
	require.Empty(t, restored.ExcludedPeople)
	require.Equal(t, id, *restored.Attributions[0].PersonID)
}

func TestSameEpisodeUnanchoredNamesakesKeepLocalIDs(t *testing.T) {
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "same-callname-episode", Title: "访谈", FeedURL: "https://example.test/same-callname-episode"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "same-callname-episode", Title: "两位同名称呼"}
	require.NoError(t, db.Create(&ep).Error)
	makeCandidate := func(speaker string, order int) SuggestedCandidate {
		proof, err := json.Marshal(map[string]any{
			"name":      "王老师",
			"name_type": "episode_callname",
			"speech_bindings": []any{map[string]any{
				"speaker_label":   speaker,
				"evidence":        map[string]any{"source": SourceTranscript, "fragment": order, "quote": "我是王老师。"},
				"basis":           "self_introduction",
				"orders":          []int{order},
				"scope":           "listed_fragments",
				"excluded_orders": []int{},
			}},
		})
		require.NoError(t, err)
		return SuggestedCandidate{DisplayName: "王老师", Role: RoleGuest, Status: StatusConfirmed, EvidenceKind: "verified_runtime", EvidenceLocator: string(proof), SpeechOrders: []int{order}}
	}
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{makeCandidate("A", 1), makeCandidate("B", 2)}})
	require.NoError(t, err)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "fixture-same-callname", Segments: []Segment{{Order: 1, SpeakerLabel: "A", Text: "我是王老师。"}, {Order: 2, SpeakerLabel: "B", Text: "我是王老师。"}}}
	first, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Len(t, first.People, 2)
	firstIDs := []uint{*first.Attributions[0].PersonID, *first.Attributions[1].PersonID}
	again, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Equal(t, firstIDs[0], *again.Attributions[0].PersonID)
	require.Equal(t, firstIDs[1], *again.Attributions[1].PersonID)
	excluded := true
	_, err = service.CorrectAppearance(context.Background(), ep.ID, AppearanceCorrection{PersonID: firstIDs[0], Excluded: &excluded})
	require.NoError(t, err)
	rebuilt, err := prepareReviewed(service, context.Background(), src)
	require.NoError(t, err)
	require.Len(t, rebuilt.People, 1)
	require.Equal(t, firstIDs[1], rebuilt.People[0].ID)
	require.Len(t, rebuilt.ExcludedPeople, 1)
	require.Equal(t, firstIDs[0], rebuilt.ExcludedPeople[0].ID)
}

func (f decisionSuggester) Suggest(ctx context.Context, src EpisodeSources) (Suggestions, error) {
	c, err := f.suggestCandidates(ctx, src)
	return Suggestions{Candidates: c}, err
}
