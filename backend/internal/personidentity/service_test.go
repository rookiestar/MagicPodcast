package personidentity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/contentsearch"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
	"magicpodcast/internal/processing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type seededLibrary struct {
	db          *gorm.DB
	service     *Service
	episodeIDs  map[string]uint
	personNames map[string]string
}

func openPersonIdentityDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf(
		"file:person_identity_%d?mode=memory&cache=shared&_foreign_keys=on",
		time.Now().UnixNano(),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.ApplyMigrations(db))
	require.NoError(t, database.RequireSchemaReady(db))
	return db
}

func seedBaselineLibrary(t *testing.T, suggester CandidateSuggester) seededLibrary {
	t.Helper()
	db := openPersonIdentityDB(t)
	if suggester == nil {
		suggester = testSourceSuggester{}
	}
	service, err := NewService(db, suggester)
	require.NoError(t, err)

	seeded, err := SeedBaseline(context.Background(), db, service)
	require.NoError(t, err)
	return seededLibrary{
		db:         db,
		service:    service,
		episodeIDs: seeded.EpisodeIDs,
	}
}

func TestPrepareKeepsSameNamePeopleDistinctAndSupportsNicknames(t *testing.T) {
	lib := seedBaselineLibrary(t, nil)
	tech, err := lib.service.ListEpisodePeople(context.Background(), lib.episodeIDs["ep-tech-overtime"])
	require.NoError(t, err)
	media, err := lib.service.ListEpisodePeople(context.Background(), lib.episodeIDs["ep-media-zhangsan"])
	require.NoError(t, err)

	techZhang := mustPersonByName(t, tech, "张三")
	mediaZhang := mustPersonByName(t, media, "张三")
	require.NotEqual(t, techZhang.ID, mediaZhang.ID)
	require.Contains(t, techZhang.IdentityNote, "架构")
	require.NotContains(t, names(tech.People), "王总")

	liming := mustPersonByName(t, tech, "李明")
	require.Contains(t, liming.Aliases, "小李")
	require.Equal(t, RoleGuest, liming.Role)
	require.Equal(t, StatusConfirmed, liming.Status)

	host := mustPersonByName(t, tech, "张三")
	require.Equal(t, RoleHost, host.Role)

	var aliasHits []models.PersonAlias
	require.NoError(t, lib.db.Where("alias = ?", "小李").Find(&aliasHits).Error)
	require.Len(t, aliasHits, 1)
	require.Equal(t, liming.ID, aliasHits[0].PersonID)
}

func TestPrepareKeepsCrossEpisodeRoleAndPendingFragments(t *testing.T) {
	lib := seedBaselineLibrary(t, nil)
	hostEp, err := lib.service.ListEpisodePeople(context.Background(), lib.episodeIDs["ep-product-host"])
	require.NoError(t, err)
	guestEp, err := lib.service.ListEpisodePeople(context.Background(), lib.episodeIDs["ep-startup-guest"])
	require.NoError(t, err)
	host := mustPersonByName(t, hostEp, "王芳")
	guest := mustPersonByName(t, guestEp, "王芳")
	require.Equal(t, host.ID, guest.ID)
	require.Equal(t, RoleHost, host.Role)
	require.Equal(t, RoleGuest, guest.Role)
	require.Contains(t, host.Aliases, "芳芳")

	anonymous, err := lib.service.ListEpisodePeople(context.Background(), lib.episodeIDs["ep-anonymous"])
	require.NoError(t, err)
	anon := mustPersonByName(t, anonymous, "匿名工程师")
	require.Equal(t, StatusPending, anon.Status)
	require.Zero(t, anon.ConfirmedSpeech)
	pendingFragment := attributionByOrder(t, anonymous, 1)
	require.Equal(t, StatusPending, pendingFragment.Status)
	require.Nil(t, pendingFragment.PersonID)

	mixed, err := lib.service.ListEpisodePeople(context.Background(), lib.episodeIDs["ep-mixed-label"])
	require.NoError(t, err)
	require.NotEmpty(t, mixed.People)
	for _, attribution := range mixed.Attributions {
		require.Equal(t, StatusPending, attribution.Status, attribution.Text)
		require.Nil(t, attribution.PersonID)
	}
}

func TestCorrectAttributionPersistsAndDoesNotRewriteTranscript(t *testing.T) {
	lib := seedBaselineLibrary(t, nil)
	episodeID := lib.episodeIDs["ep-mixed-label"]
	people, err := lib.service.ListEpisodePeople(context.Background(), episodeID)
	require.NoError(t, err)
	zhang := mustPersonByName(t, people, "张三")
	liming := mustPersonByName(t, people, "李明")

	transcriptPath := filepath.Join(t.TempDir(), "transcript.md")
	original := []byte("嘉宾: 我不赞成无限制加班。\n嘉宾: 阶段性冲刺可以接受。\n")
	require.NoError(t, os.WriteFile(transcriptPath, original, 0o600))

	_, err = lib.service.CorrectAttribution(context.Background(), episodeID, AttributionCorrection{
		SourceKind:       SourceTranscript,
		FragmentOrder:    1,
		AssignedPersonID: &zhang.ID,
		Status:           StatusConfirmed,
	})
	require.NoError(t, err)
	_, err = lib.service.CorrectAttribution(context.Background(), episodeID, AttributionCorrection{
		SourceKind:       SourceTranscript,
		FragmentOrder:    2,
		AssignedPersonID: &liming.ID,
		Status:           StatusConfirmed,
	})
	require.NoError(t, err)

	after, err := os.ReadFile(transcriptPath)
	require.NoError(t, err)
	require.Equal(t, original, after)

	updated, err := lib.service.ListEpisodePeople(context.Background(), episodeID)
	require.NoError(t, err)
	require.Equal(t, zhang.ID, *attributionByOrder(t, updated, 1).PersonID)
	require.Equal(t, liming.ID, *attributionByOrder(t, updated, 2).PersonID)
	require.True(t, attributionByOrder(t, updated, 1).UserConfirmed)

	reliable, err := lib.service.ReliableSpeech(context.Background(), episodeID, zhang.ID)
	require.NoError(t, err)
	require.Len(t, reliable, 1)
	require.Contains(t, reliable[0].Text, "不赞成无限制加班")

	_, err = lib.service.Prepare(context.Background(), EpisodeSources{
		EpisodeID:     episodeID,
		ShowNotes:     "主播张三，嘉宾李明。转写把两人混进同一说话人标签“嘉宾”。",
		SourceKind:    SourceTranscript,
		SourceVersion: "fixture-ep-mixed-label-v2",
		Segments: []Segment{
			{Order: 1, SpeakerLabel: "嘉宾", StartMS: 11000, Text: "我不赞成无限制加班。"},
			{Order: 2, SpeakerLabel: "嘉宾", StartMS: 24000, Text: "阶段性冲刺可以接受。"},
		},
	})
	require.NoError(t, err)
	reextracted, err := lib.service.ListEpisodePeople(context.Background(), episodeID)
	require.NoError(t, err)
	require.Nil(t, attributionByOrder(t, reextracted, 1).PersonID)
	require.Equal(t, StatusPending, attributionByOrder(t, reextracted, 1).Status)

	facts, err := lib.service.CurrentFacts(context.Background(), episodeID)
	require.NoError(t, err)
	var stale, current int
	for _, fact := range facts {
		if fact.SourceVersion == "fixture-ep-mixed-label" {
			require.False(t, fact.Current)
			stale++
		}
		if fact.SourceVersion == "fixture-ep-mixed-label-v2" {
			require.True(t, fact.Current)
			current++
		}
	}
	require.NotZero(t, stale)
	require.NotZero(t, current)
}

func TestPublishedTranscriptIndexesPeopleAndSearch(t *testing.T) {
	db := openPersonIdentityDB(t)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	service, err := NewService(db, testSourceSuggester{}, search)
	require.NoError(t, err)
	podcast := models.Podcast{
		XYZID: "publish-index", Title: "技术漫谈", FeedURL: "https://example.test/publish-index.xml",
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID, Title: "加班", ShowNotes: "主播张三讨论加班。",
		GUID: "publish-index-ep", PublishedDate: time.Date(2025, 3, 12, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&episode).Error)
	store, err := processing.NewDiskArtifactStore(t.TempDir())
	require.NoError(t, err)
	now := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
	run := models.EpisodeProcessingRun{
		EpisodeID: episode.ID, ProcessingKey: strings.Repeat("b", 64),
		AudioDigest: strings.Repeat("a", 64), PipelineVersion: processing.NativeMinutesPipelineVersion,
		TriggerSource: models.ProcessingTriggerManual, Status: models.ProcessingRunStatusCompleted,
		CurrentStep: processing.StepArtifactPublish, AttemptCount: 1, MaxAttempts: 3,
		RetryDeadlineAt: now.Add(24 * time.Hour), FinishedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(&run).Error)
	segments := []processing.TranscriptSegment{{
		Order: 1, Speaker: "张三", StartMS: 100, Text: "我不赞成无限制加班。",
	}}
	published, err := store.Publish(context.Background(), processing.ArtifactPublishRequest{
		RunID: run.ID, EpisodeID: episode.ID, AudioDigest: strings.Repeat("a", 64),
		PipelineVersion: processing.NativeMinutesPipelineVersion, NativeMinutes: true,
		MinutesSummary:       "# 纪要\n\n加班\n",
		Transcript:           "# 逐字稿\n\n张三 00:00:00.100\n我不赞成无限制加班。\n",
		TranscriptSegments:   segments,
		TranscriptionAdapter: "fake-minutes",
		TranscriptionVersion: "fake-minutes-v1",
		SkillVersions:        map[string]string{"minutes": "skill-v1"},
		GeneratedAt:          time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	artifact := models.EpisodeArtifactSet{
		RunID: run.ID, EpisodeID: episode.ID,
		PipelineVersion: processing.NativeMinutesPipelineVersion,
		RootPath:        published.RootPath, ManifestPath: published.ManifestPath,
		ManifestSHA256: published.ManifestSHA256, AudioSHA256: published.AudioSHA256,
		MinutesSummarySHA256:     published.MinutesSummarySHA256,
		TranscriptSHA256:         published.TranscriptSHA256,
		TranscriptTimelineSHA256: published.TranscriptTimelineSHA256,
		IsCurrent:                true, CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, db.Create(&artifact).Error)
	empty, err := service.ListEpisodePeople(context.Background(), episode.ID)
	require.NoError(t, err)
	require.Empty(t, empty.People)
	require.NoError(t, service.IndexPublishedTranscript(
		context.Background(),
		episode.ID,
		fmt.Sprintf("artifact-%d", artifact.ID),
		episode.ShowNotes,
		segments,
	))
	listed, err := service.ListEpisodePeople(context.Background(), episode.ID)
	require.NoError(t, err)
	require.Equal(t, "张三", listed.People[0].DisplayName)
	hits, err := search.Search(context.Background(), contentsearch.Request{
		Query:  "加班",
		Scope:  contentsearch.Scope{EpisodeIDs: []uint{episode.ID}},
		Filter: contentsearch.Filter{PersonID: &listed.People[0].ID},
		Limit:  8,
	})
	require.NoError(t, err)
	require.NotEmpty(t, hits.Hits)
	require.Equal(t, "2025-03-12", hits.Hits[0].PublishedAt)
	require.Contains(t, hits.Hits[0].Text, "不赞成无限制加班")

	t.Run("selected rebuild reports failures and retries without widening scope", func(t *testing.T) {
		service.WithArtifactReader(store)
		missing := models.Episode{PodcastID: podcast.ID, Title: "无转写", GUID: "missing-rebuild"}
		untouched := models.Episode{PodcastID: podcast.ID, Title: "未选中", GUID: "untouched-rebuild"}
		require.NoError(t, db.Create(&missing).Error)
		require.NoError(t, db.Create(&untouched).Error)
		preview, err := PreviewRebuild(context.Background(), db, []uint{episode.ID, missing.ID})
		require.NoError(t, err)
		require.Len(t, preview, 2)
		require.Equal(t, artifact.ID, preview[0].ArtifactID)
		require.False(t, preview[0].NeedsPreparation)
		var results []RebuildResult
		report := func(result RebuildResult) error { results = append(results, result); return nil }
		err = service.RebuildSelected(context.Background(), []uint{missing.ID, episode.ID}, report)
		require.ErrorContains(t, err, "1 episodes failed")
		require.Len(t, results, 2)
		for _, result := range results {
			require.Equal(t, result.EpisodeID == episode.ID, result.Success)
			if result.Success {
				require.Equal(t, []string{"张三"}, result.After)
			} else {
				require.NotEmpty(t, result.Error)
			}
		}
		results = nil
		require.NoError(t, service.RebuildSelected(context.Background(), []uint{episode.ID}, report))
		require.Len(t, results, 1)
		after, err := service.ListEpisodePeople(context.Background(), episode.ID)
		require.NoError(t, err)
		require.Equal(t, listed.People[0].ID, after.People[0].ID)
		var count int64
		require.NoError(t, db.Model(&models.PersonPreparation{}).Where("episode_id = ?", untouched.ID).Count(&count).Error)
		require.Zero(t, count)
		require.Error(t, service.RebuildSelected(context.Background(), nil, report))
		require.Error(t, service.RebuildSelected(context.Background(), []uint{episode.ID, episode.ID}, report))
	})
	t.Run("deleting the last artifact invalidates manual and automatic speech", func(t *testing.T) {
		id := listed.People[0].ID
		_, err := service.CorrectName(context.Background(), episode.ID, NameCorrection{PersonID: id, DisplayName: "张三"})
		require.NoError(t, err)
		_, err = service.CorrectAttribution(context.Background(), episode.ID, AttributionCorrection{SourceVersion: fmt.Sprintf("artifact-%d", artifact.ID), FragmentOrder: 1, AssignedPersonID: &id, Status: StatusConfirmed})
		require.NoError(t, err)
		service.suggester = decisionSuggester(func(context.Context, EpisodeSources) ([]SuggestedCandidate, error) {
			if err := db.Delete(&artifact).Error; err != nil {
				return nil, err
			}
			return []SuggestedCandidate{{DisplayName: "过期人物", Role: RoleHost, EvidenceKind: "verified_runtime", SpeechOrders: []int{1}}}, nil
		})
		_, err = service.PrepareCurrent(context.Background(), episode.ID)
		require.ErrorIs(t, err, ErrSourcesChanged)
		after, err := service.ListEpisodePeople(context.Background(), episode.ID)
		require.NoError(t, err)
		require.Empty(t, after.People)
		require.False(t, after.IndexReady)
		speech, err := service.ReliableSpeech(context.Background(), episode.ID, id)
		require.NoError(t, err)
		require.Empty(t, speech)
		result, err := search.Search(context.Background(), contentsearch.Request{Query: "加班", Scope: contentsearch.Scope{EpisodeIDs: []uint{episode.ID}}, Filter: contentsearch.Filter{SourceKind: SourceTranscript}})
		require.NoError(t, err)
		require.Empty(t, result.Hits)
		require.False(t, result.Coverage.Complete)
		var count int64
		require.NoError(t, db.Model(&models.PersonUserConfirmation{}).Where("episode_id = ?", episode.ID).Count(&count).Error)
		require.EqualValues(t, 2, count, "confirmation history is retained, not applied to a missing source")
	})

}

func TestCorrectAttributionUpdatesSearchWithoutAsk(t *testing.T) {
	db := openPersonIdentityDB(t)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	service, err := NewService(db, testSourceSuggester{}, search)
	require.NoError(t, err)
	podcast := models.Podcast{
		XYZID: "search-correct", Title: "技术漫谈", FeedURL: "https://example.test/search-correct.xml",
	}
	require.NoError(t, db.Create(&podcast).Error)
	episode := models.Episode{
		PodcastID: podcast.ID, Title: "混标签", ShowNotes: "主播张三，嘉宾李明。",
		GUID: "search-correct-ep", PublishedDate: time.Date(2025, 8, 11, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&episode).Error)
	_, err = service.Prepare(context.Background(), EpisodeSources{
		EpisodeID:     episode.ID,
		ShowNotes:     episode.ShowNotes,
		SourceKind:    SourceTranscript,
		SourceVersion: "v1",
		Segments: []Segment{
			{Order: 1, SpeakerLabel: "嘉宾", StartMS: 1000, Text: "我不赞成无限制加班。"},
			{Order: 2, SpeakerLabel: "嘉宾", StartMS: 2000, Text: "阶段性冲刺可以接受。"},
		},
	})
	require.NoError(t, err)
	people, err := service.ListEpisodePeople(context.Background(), episode.ID)
	require.NoError(t, err)
	zhang := mustPersonByName(t, people, "张三")
	pendingHits, err := search.Search(context.Background(), contentsearch.Request{
		Query:  "加班",
		Scope:  contentsearch.Scope{EpisodeIDs: []uint{episode.ID}},
		Filter: contentsearch.Filter{PersonID: &zhang.ID},
		Limit:  8,
	})
	require.NoError(t, err)
	require.False(t, containsHitText(pendingHits.Hits, "不赞成无限制加班"))

	_, err = service.CorrectAttribution(context.Background(), episode.ID, AttributionCorrection{
		SourceKind:       SourceTranscript,
		FragmentOrder:    1,
		AssignedPersonID: &zhang.ID,
		Status:           StatusConfirmed,
	})
	require.NoError(t, err)
	confirmedHits, err := search.Search(context.Background(), contentsearch.Request{
		Query:  "加班",
		Scope:  contentsearch.Scope{EpisodeIDs: []uint{episode.ID}},
		Filter: contentsearch.Filter{PersonID: &zhang.ID},
		Limit:  8,
	})
	require.NoError(t, err)
	require.True(t, containsHitText(confirmedHits.Hits, "不赞成无限制加班"))
	require.Equal(t, "confirmed", confirmedHits.Hits[0].AttributionStatus)
	require.Equal(t, "2025-08-11", confirmedHits.Hits[0].PublishedAt)
}

func TestPrepareUsesRuntimeSuggestionsWithoutOverwritingUserConfirmation(t *testing.T) {
	suggester := stubSuggester{
		candidates: []SuggestedCandidate{{
			DisplayName:     "李明",
			Aliases:         []string{"小李"},
			IdentityNote:    "独立工程顾问",
			Role:            RoleGuest,
			EvidenceKind:    "self_intro",
			EvidenceLocator: "runtime:seg-2",
		}},
	}
	lib := seedBaselineLibrary(t, suggester)
	tech, err := lib.service.ListEpisodePeople(context.Background(), lib.episodeIDs["ep-tech-overtime"])
	require.NoError(t, err)
	require.NotNil(t, mustPersonByName(t, tech, "李明"))
}

func containsHitText(hits []contentsearch.Hit, needle string) bool {
	for _, hit := range hits {
		if strings.Contains(hit.Text, needle) {
			return true
		}
	}
	return false
}

func names(people []PersonView) []string {
	out := make([]string, 0, len(people))
	for _, person := range people {
		out = append(out, person.DisplayName)
	}
	return out
}

func mustPersonByName(t *testing.T, episode EpisodePeople, name string) PersonView {
	t.Helper()
	for _, person := range episode.People {
		if person.DisplayName == name {
			return person
		}
		for _, alias := range person.Aliases {
			if alias == name {
				return person
			}
		}
	}
	t.Fatalf("person %q not found in %+v", name, names(episode.People))
	return PersonView{}
}

func attributionByOrder(t *testing.T, episode EpisodePeople, order int) AttributionView {
	t.Helper()
	for _, attribution := range episode.Attributions {
		if attribution.FragmentOrder == order {
			return attribution
		}
	}
	t.Fatalf("fragment %d not found", order)
	return AttributionView{}
}

type stubSuggester struct {
	candidates []SuggestedCandidate
}

func (s stubSuggester) Suggest(context.Context, EpisodeSources) ([]SuggestedCandidate, error) {
	return s.candidates, nil
}

func TestSameNameWithoutIdentityDoesNotMergeAcrossEpisodes(t *testing.T) {
	db := openPersonIdentityDB(t)
	service, err := NewService(db, testSourceSuggester{})
	require.NoError(t, err)
	pod := models.Podcast{Title: "同名核对", XYZID: "same-name", FeedURL: "https://example.test/same"}
	require.NoError(t, db.Create(&pod).Error)
	var ids []uint
	for _, guid := range []string{"one", "two"} {
		ep := models.Episode{PodcastID: pod.ID, Title: guid, GUID: guid}
		require.NoError(t, db.Create(&ep).Error)
		src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", ShowNotes: "嘉宾：张三", Segments: []Segment{{Order: 1, SpeakerLabel: "张三", Text: "我是张三。"}}}
		first, err := service.Prepare(context.Background(), src)
		require.NoError(t, err)
		require.Len(t, first.People, 1)
		again, err := service.Prepare(context.Background(), src)
		require.NoError(t, err)
		require.Len(t, again.People, 1)
		require.Equal(t, first.People[0].ID, again.People[0].ID)
		ids = append(ids, first.People[0].ID)
	}
	require.NotEqual(t, ids[0], ids[1])
}

func TestCorrectionReplacementAndVersionIsolation(t *testing.T) {
	lib := seedBaselineLibrary(t, nil)
	ctx := context.Background()
	ep := lib.episodeIDs["ep-tech-overtime"]
	before, err := lib.service.ListEpisodePeople(ctx, ep)
	require.NoError(t, err)
	a, b := before.People[0].ID, before.People[1].ID
	for _, id := range []uint{a, b} {
		_, err = lib.service.CorrectAttribution(ctx, ep, AttributionCorrection{FragmentOrder: 7, AssignedPersonID: &id, Status: StatusConfirmed})
		require.NoError(t, err)
	}
	sources := EpisodeSources{EpisodeID: ep, SourceVersion: before.SourceVersion, Segments: []Segment{}}
	for _, f := range before.Attributions {
		sources.Segments = append(sources.Segments, Segment{Order: f.FragmentOrder, SpeakerLabel: f.SpeakerLabel, StartMS: f.StartMS, Text: f.Text})
	}
	got, err := lib.service.Prepare(ctx, sources)
	require.NoError(t, err)
	require.Equal(t, b, *got.Attributions[6].PersonID)
	_, err = lib.service.CorrectAttribution(ctx, ep, AttributionCorrection{FragmentOrder: 7, AssignedPersonID: &b, Status: StatusPending})
	require.NoError(t, err)
	got, err = lib.service.Prepare(ctx, sources)
	require.NoError(t, err)
	require.Equal(t, StatusPending, got.Attributions[6].Status)
	_, err = lib.service.CorrectAttribution(ctx, ep, AttributionCorrection{FragmentOrder: 7, Status: StatusRejected})
	require.NoError(t, err)
	got, err = lib.service.Prepare(ctx, sources)
	require.NoError(t, err)
	require.Equal(t, StatusRejected, got.Attributions[6].Status)
	require.Nil(t, got.Attributions[6].PersonID)
	sources.SourceVersion = "v-new"
	got, err = lib.service.Prepare(ctx, sources)
	require.NoError(t, err)
	require.Equal(t, StatusPending, got.Attributions[6].Status)
	require.Nil(t, got.Attributions[6].PersonID)
}

func TestRuntimeSuggestionsRequireLocatedOriginalIdentity(t *testing.T) {
	sources := EpisodeSources{ShowNotes: "本集嘉宾：欧阳明月", Segments: []Segment{{Order: 1, SpeakerLabel: "Speaker 1", Text: "我是欧阳明月。"}}}
	raw := `{"people":[{"name":"欧阳明月","kind":"participant","status":"confirmed","role":"guest","presence_basis":"self_introduction","name_evidence":{"source":"show_notes","fragment":0,"quote":"本集嘉宾：欧阳明月"},"presence_evidence":{"source":"transcript","fragment":1,"quote":"我是欧阳明月。"},"role_evidence":{"source":"show_notes","fragment":0,"quote":"本集嘉宾：欧阳明月"},"speech_bindings":[{"speaker_label":"Speaker 1","basis":"self_introduction","evidence":{"source":"transcript","fragment":1,"quote":"我是欧阳明月。"},"orders":[1,99]}]},{"name":"李四","kind":"participant","status":"confirmed","name_evidence":{"source":"transcript","fragment":1,"quote":"我是李四。"}}]}`
	items, err := decodeIdentitySuggestions([]byte(raw), sources)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "欧阳明月", items[0].DisplayName)
	require.Equal(t, []int{1}, items[0].SpeechOrders)
}

func TestShowNotesChangeInvalidatesPeopleAndIndex(t *testing.T) {
	db := openPersonIdentityDB(t)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	service, err := NewService(db, testSourceSuggester{}, search)
	require.NoError(t, err)
	seed, err := SeedBaseline(context.Background(), db, service)
	require.NoError(t, err)
	id := seed.EpisodeIDs["ep-tech-overtime"]
	before, err := service.ListEpisodePeople(context.Background(), id)
	require.NoError(t, err)
	require.True(t, before.IndexReady)
	require.NoError(t, db.Model(&models.Episode{}).Where("id = ?", id).Update("show_notes", "嘉宾介绍已纠正，需要重新核对归属。").Error)
	after, err := service.ListEpisodePeople(context.Background(), id)
	require.NoError(t, err)
	require.False(t, after.IndexReady)
	for _, p := range after.People {
		require.Equal(t, StatusPending, p.Status)
	}
	result, err := search.Search(context.Background(), contentsearch.Request{Query: "加班", Scope: contentsearch.Scope{EpisodeIDs: []uint{id}}})
	require.NoError(t, err)
	require.NotEmpty(t, result.Hits, "valid original transcript remains generally searchable")
	for _, hit := range result.Hits {
		require.Nil(t, hit.PersonID)
	}
	require.False(t, result.Coverage.Complete)
	personal, err := search.Search(context.Background(), contentsearch.Request{Query: "加班", Scope: contentsearch.Scope{EpisodeIDs: []uint{id}}, Filter: contentsearch.Filter{PersonID: &before.People[0].ID}})
	require.NoError(t, err)
	require.Empty(t, personal.Hits)
	require.False(t, personal.Coverage.Complete)
}
