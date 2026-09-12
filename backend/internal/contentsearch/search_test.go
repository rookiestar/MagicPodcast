package contentsearch

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
	"magicpodcast/internal/personaqa"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestSearchPublicEntryHonorsPersonFilterScopeAndParaphrase(t *testing.T) {
	db := openSearchDB(t)
	service, err := NewService(db)
	require.NoError(t, err)
	library := indexBaseline(t, db, service)

	zhang := library.people["person-zhangsan-tech"]
	personHits, err := service.Search(context.Background(), Request{
		Query:  "他对加班怎么看？",
		Scope:  Scope{EpisodeIDs: library.allEpisodeIDs},
		Filter: Filter{PersonID: &zhang},
		Limit:  8,
	})
	require.NoError(t, err)
	require.NotEmpty(t, personHits.Hits)
	require.True(t, containsText(personHits.Hits, "不赞成无限制加班"))
	for _, hit := range personHits.Hits {
		require.NotEmpty(t, hit.EpisodeTitle)
		require.NotEmpty(t, hit.PodcastTitle)
		require.NotNil(t, hit.PersonID)
		require.Equal(t, zhang, *hit.PersonID)
		require.Equal(t, "confirmed", hit.AttributionStatus)
		require.NotContains(t, hit.Text, "私有备注")
	}

	unfiltered, err := service.Search(context.Background(), Request{
		Query: "不加班就没有产出",
		Scope: Scope{EpisodeIDs: library.allEpisodeIDs},
		Limit: 8,
	})
	require.NoError(t, err)
	require.True(t, containsText(unfiltered.Hits, "不加班就没有产出"))

	filteredPending, err := service.Search(context.Background(), Request{
		Query:  "不加班就没有产出",
		Scope:  Scope{EpisodeIDs: library.allEpisodeIDs},
		Filter: Filter{PersonID: &zhang},
		Limit:  8,
	})
	require.NoError(t, err)
	require.False(t, containsText(filteredPending.Hits, "不加班就没有产出"))

	outOfScope := library.episodes["ep-media-zhangsan"]
	narrow, err := service.Search(context.Background(), Request{
		Query:  "加班",
		Scope:  Scope{EpisodeIDs: []uint{library.episodes["ep-tech-overtime"]}},
		Filter: Filter{EpisodeID: &outOfScope},
		Limit:  8,
	})
	require.NoError(t, err)
	require.Empty(t, narrow.Hits)

	first, err := service.Search(context.Background(), Request{
		Query: "加班",
		Scope: Scope{EpisodeIDs: library.allEpisodeIDs},
		Limit: 5,
	})
	require.NoError(t, err)
	second, err := service.Search(context.Background(), Request{
		Query: "加班",
		Scope: Scope{EpisodeIDs: library.allEpisodeIDs},
		Limit: 5,
	})
	require.NoError(t, err)
	require.Equal(t, hitKeys(first.Hits), hitKeys(second.Hits))
}

func TestSearchDistinguishesMissCoverageTruncationAndFailure(t *testing.T) {
	db := openSearchDB(t)
	service, err := NewService(db)
	require.NoError(t, err)
	library := indexBaseline(t, db, service)

	miss, err := service.Search(context.Background(), Request{
		Query: "航天推进器",
		Scope: Scope{EpisodeIDs: []uint{library.episodes["ep-no-topic"]}},
		Limit: 8,
	})
	require.NoError(t, err)
	require.Empty(t, miss.Hits)
	require.True(t, miss.Coverage.Complete)
	require.Equal(t, CoverageMiss, miss.Coverage.Reason)

	unindexed := library.episodes["ep-no-topic"] + 999
	gap, err := service.Search(context.Background(), Request{
		Query: "报道伦理",
		Scope: Scope{EpisodeIDs: []uint{unindexed}},
		Limit: 8,
	})
	require.NoError(t, err)
	require.False(t, gap.Coverage.Complete)
	require.Equal(t, CoverageIndexNotReady, gap.Coverage.Reason)
	require.Contains(t, gap.Coverage.MissingEpisodes, unindexed)

	truncated, err := service.Search(context.Background(), Request{
		Query: "加班",
		Scope: Scope{EpisodeIDs: library.allEpisodeIDs},
		Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, truncated.Hits, 1)
	require.True(t, truncated.Coverage.Truncated)

	_, err = service.Search(context.Background(), Request{Query: "加班"})
	require.NoError(t, err)
	closed, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, closed.Close())
	_, err = service.Search(context.Background(), Request{
		Query: "加班",
		Scope: Scope{EpisodeIDs: library.allEpisodeIDs},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSearchFailed)
}

func TestSearchExcludesStaleVersionsAfterReplaceDeleteAndCorrection(t *testing.T) {
	db := openSearchDB(t)
	service, err := NewService(db)
	require.NoError(t, err)
	library := indexBaseline(t, db, service)
	episodeID := library.episodes["ep-mixed-label"]
	zhang := library.people["person-zhangsan-tech"]

	// The indexing caller publishes authoritative attribution/source facts;
	// replacing an index alone must not manufacture a new identity decision.
	require.NoError(t, db.Model(&models.PersonPreparation{}).Where("episode_id = ?", episodeID).Update("source_version", "v2").Error)
	require.NoError(t, db.Model(&models.EpisodeAppearance{}).Where("episode_id = ? AND person_id = ?", episodeID, zhang).Update("source_version", "v2").Error)

	require.NoError(t, db.Create(&models.SpeechAttribution{EpisodeID: episodeID, SourceKind: SourceTranscript, SourceVersion: "v2", FragmentOrder: 1, Text: "我不赞成无限制加班。", PersonID: &zhang, Status: "confirmed"}).Error)
	require.NoError(t, db.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.PersonUserConfirmation{EpisodeID: episodeID, Kind: models.PersonConfirmationKindName, PersonID: &zhang}).Error)
	zero := uint(0)
	require.NoError(t, db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&models.PersonUserConfirmation{EpisodeID: episodeID, Kind: models.PersonConfirmationKindAttribution, PersonID: &zero, AssignedPersonID: &zhang, SourceKind: SourceTranscript, SourceVersion: "v2", FragmentOrder: 1, SourceText: "我不赞成无限制加班。", Status: "confirmed"}).Error)
	require.NoError(t, service.ReplaceEpisode(context.Background(), EpisodeDocument{
		EpisodeID:     episodeID,
		PublishedAt:   time.Date(2025, 8, 11, 0, 0, 0, 0, time.UTC),
		SourceKind:    SourceTranscript,
		SourceVersion: "v2",
		Complete:      true,
		Fragments: []FragmentInput{{
			Order:             1,
			Text:              "我不赞成无限制加班。",
			PersonID:          &zhang,
			AttributionStatus: "confirmed",
		}},
	}))
	result, err := service.Search(context.Background(), Request{
		Query:  "加班",
		Scope:  Scope{EpisodeIDs: []uint{episodeID}},
		Filter: Filter{PersonID: &zhang},
		Limit:  8,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Hits)
	for _, hit := range result.Hits {
		require.Equal(t, "v2", hit.SourceVersion)
	}

	require.NoError(t, service.RemoveEpisode(context.Background(), episodeID))
	removed, err := service.Search(context.Background(), Request{
		Query: "加班",
		Scope: Scope{EpisodeIDs: []uint{episodeID}},
		Limit: 8,
	})
	require.NoError(t, err)
	require.Empty(t, removed.Hits)
	require.False(t, removed.Coverage.Complete)
}

func TestContentSearchDoesNotImportCopilotOrWebSearch(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	forbidden := []string{
		"magicpodcast/internal/episodecopilot",
		"magicpodcast/internal/handlers",
		"codexruntime",
	}
	set := token.NewFileSet()
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(set, filepath.Join(dir, entry.Name()), nil, parser.ImportsOnly)
		require.NoError(t, err)
		for _, spec := range parsed.Imports {
			path := strings.Trim(spec.Path.Value, `"`)
			for _, item := range forbidden {
				require.NotContains(t, path, item, entry.Name())
			}
		}
	}
}

type indexedLibrary struct {
	episodes      map[string]uint
	people        map[string]uint
	allEpisodeIDs []uint
}

func openSearchDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:content_search_%d?mode=memory&cache=shared&_foreign_keys=on", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, database.ApplyMigrations(db))
	return db
}

func indexBaseline(t *testing.T, db *gorm.DB, service *Service) indexedLibrary {
	t.Helper()
	baseline, err := personaqa.Load()
	require.NoError(t, err)
	podcastIDs := map[string]uint{}
	for _, podcast := range baseline.Samples.Podcasts {
		row := models.Podcast{
			XYZID: podcast.ID, Title: podcast.Title, Author: podcast.Author,
			FeedURL: fmt.Sprintf("https://example.test/%s.xml", podcast.ID), PodcastGUID: podcast.ID,
		}
		require.NoError(t, db.Create(&row).Error)
		podcastIDs[podcast.ID] = row.ID
	}
	people := map[string]uint{}
	for _, person := range baseline.Samples.People {
		row := models.Person{
			StableKey: person.ID, DisplayName: person.DisplayName, IdentityNote: person.IdentityNote,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		require.NoError(t, db.Create(&row).Error)
		people[person.ID] = row.ID
	}
	episodes := map[string]uint{}
	all := make([]uint, 0)
	for _, episode := range baseline.Samples.Episodes {
		published, err := time.Parse("2006-01-02", episode.PublishedDate)
		require.NoError(t, err)
		row := models.Episode{
			PodcastID: podcastIDs[episode.PodcastID], Title: episode.Title, ShowNotes: episode.ShowNotes,
			Notes: episode.PrivateNotes, PublishedDate: published, GUID: episode.ID,
		}
		require.NoError(t, db.Create(&row).Error)
		episodes[episode.ID] = row.ID
		all = append(all, row.ID)
		require.NoError(t, db.Create(&models.PersonPreparation{EpisodeID: row.ID, Revision: 1, PublishedRevision: 1, SourceVersion: "fixture-" + episode.ID, AlgorithmVersion: models.CurrentIdentityAlgorithm, MetadataDigest: "declared-synthetic-fixture", UpdatedAt: time.Now().UTC()}).Error)
		for _, appearance := range episode.Appearances {
			require.NoError(t, db.Create(&models.EpisodeAppearance{EpisodeID: row.ID, PersonID: people[appearance.PersonID], SourceVersion: "fixture-" + episode.ID, Role: appearance.Role, Status: appearance.Status, EvidenceKind: "synthetic-fixture"}).Error)
		}
		fragments := make([]FragmentInput, 0, len(episode.TranscriptSegments))
		for _, segment := range episode.TranscriptSegments {
			item := FragmentInput{Order: segment.Order, StartMS: segment.StartMS, Text: segment.Text, AttributionStatus: segment.AttributionStatus}
			if segment.AttributionPersonID != "" {
				id := people[segment.AttributionPersonID]
				item.PersonID = &id
			}
			fragments = append(fragments, item)
			require.NoError(t, db.Create(&models.SpeechAttribution{EpisodeID: row.ID, SourceKind: SourceTranscript, SourceVersion: "fixture-" + episode.ID, FragmentOrder: segment.Order, SpeakerLabel: segment.SpeakerLabel, Text: segment.Text, Status: segment.AttributionStatus, PersonID: item.PersonID}).Error)
			if item.PersonID != nil && segment.AttributionStatus == "confirmed" {
				require.NoError(t, db.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.PersonUserConfirmation{EpisodeID: row.ID, Kind: models.PersonConfirmationKindName, PersonID: item.PersonID}).Error)
				zero := uint(0)
				require.NoError(t, db.Create(&models.PersonUserConfirmation{EpisodeID: row.ID, Kind: models.PersonConfirmationKindAttribution, PersonID: &zero, AssignedPersonID: item.PersonID, SourceKind: SourceTranscript, SourceVersion: "fixture-" + episode.ID, FragmentOrder: segment.Order, SpeakerLabel: segment.SpeakerLabel, SourceText: segment.Text, Status: "confirmed"}).Error)
			}
		}
		require.NoError(t, service.ReplaceEpisode(context.Background(), EpisodeDocument{
			EpisodeID:     row.ID,
			PublishedAt:   published,
			ShowNotes:     episode.ShowNotes,
			SourceKind:    SourceTranscript,
			SourceVersion: "fixture-" + episode.ID,
			Fragments:     fragments,
			Complete:      true,
		}))
	}
	return indexedLibrary{episodes: episodes, people: people, allEpisodeIDs: all}
}

func containsText(hits []Hit, needle string) bool {
	for _, hit := range hits {
		if strings.Contains(hit.Text, needle) {
			return true
		}
	}
	return false
}

func hitKeys(hits []Hit) []string {
	out := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, fmt.Sprintf("%d:%s:%d", hit.EpisodeID, hit.SourceVersion, hit.FragmentOrder))
	}
	return out
}
