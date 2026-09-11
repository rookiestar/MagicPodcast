package personidentity

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"magicpodcast/internal/contentsearch"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"
	"path/filepath"
	"testing"
	"time"
)

func TestSchema28NineLegacyCandidatesUpgradeWithoutClearingFacts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// Build the actual published schema from its registered migrations, stopping
	// before the repair. No pre-existing candidate rows are deleted to upgrade.
	require.NoError(t, db.AutoMigrate(&database.SchemaMigration{}))
	status, err := database.InspectSchema(db)
	require.NoError(t, err)
	for _, migration := range status.Pending {
		if migration.Version >= 29 {
			break
		}
		require.NoError(t, migration.Apply(db))
		require.NoError(t, db.Create(&database.SchemaMigration{Version: migration.Version, Name: migration.Name, AppliedAt: time.Now().UTC()}).Error)
	}
	require.False(t, db.Migrator().HasTable(&models.PersonPreparation{}))
	pod := models.Podcast{XYZID: "legacy-nine", Title: "商业访谈", Author: "张小珺", FeedURL: "https://example.test/legacy-nine"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "legacy-nine", Title: "和曾鸣聊产业史观", ShowNotes: "主持张小珺，嘉宾曾鸣。"}
	require.NoError(t, db.Create(&ep).Error)
	namesBefore := []string{"战略学", "这个观点", "因为纯粹我自己", "敢用的", "觉得绝对要有希望的", "觉得", "比较乐观的", "小俊", "曾鸣"}
	var hostID, guestID uint
	for i, name := range namesBefore {
		p := models.Person{StableKey: fmt.Sprintf("legacy-%d", i), DisplayName: name}
		require.NoError(t, db.Create(&p).Error)
		require.NoError(t, db.Create(&models.EpisodeAppearance{EpisodeID: ep.ID, PersonID: p.ID, SourceVersion: "v1", Status: StatusConfirmed, Role: RoleUnknown, EvidenceKind: "legacy-rule"}).Error)
		if name == "小俊" {
			hostID = p.ID
		}
		if name == "曾鸣" {
			guestID = p.ID
		}
	}
	text := "我是小俊。欢迎曾鸣。"
	require.NoError(t, db.Create(&models.SpeechAttribution{EpisodeID: ep.ID, PersonID: &hostID, SourceKind: SourceTranscript, SourceVersion: "v1", FragmentOrder: 1, SpeakerLabel: "Speaker 1", Text: text, Status: StatusConfirmed, EvidenceKind: "legacy-rule"}).Error)
	require.NoError(t, db.Create(&models.PersonUserConfirmation{EpisodeID: ep.ID, Kind: models.PersonConfirmationKindName, PersonID: &guestID, DisplayName: "曾鸣"}).Error)
	preview, err := PreviewRebuild(context.Background(), db, nil)
	require.NoError(t, err)
	require.Len(t, preview, 1)
	require.Len(t, preview[0].CandidateNames, 9)
	require.EqualValues(t, 1, preview[0].NameConfirmations)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	require.NoError(t, database.ApplyMigrations(db))
	require.NoError(t, database.RequireSchemaReady(db))
	var count int64
	require.NoError(t, db.Model(&models.EpisodeAppearance{}).Count(&count).Error)
	require.EqualValues(t, 9, count)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{{DisplayName: "张小珺", SourceNames: []string{"小俊"}, Role: RoleHost, EvidenceKind: "verified_runtime", SpeechOrders: []int{1}}, {DisplayName: "曾鸣", Role: RoleGuest, EvidenceKind: "verified_runtime"}}}, search)
	require.NoError(t, err)
	before, err := service.ListEpisodePeople(context.Background(), ep.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"曾鸣"}, names(before.People))
	require.Zero(t, before.People[0].ConfirmedSpeech)
	src := EpisodeSources{EpisodeID: ep.ID, SourceVersion: "v1", Segments: []Segment{{Order: 1, SpeakerLabel: "Speaker 1", Text: text}}}
	for i := 0; i < 2; i++ {
		after, err := service.Prepare(context.Background(), src)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"张小珺", "曾鸣"}, names(after.People))
		require.Equal(t, hostID, mustPersonByName(t, after, "张小珺").ID)
		require.NotContains(t, mustPersonByName(t, after, "张小珺").Aliases, "小俊")
		require.Equal(t, guestID, mustPersonByName(t, after, "曾鸣").ID)
		require.Equal(t, text, after.Attributions[0].Text)
	}
	require.NoError(t, db.Model(&models.PersonUserConfirmation{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, database.ApplyMigrations(db))
}
