package sync

import (
	"errors"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"magicpodcast/internal/models"
	"magicpodcast/internal/opml"
	"strings"
	"testing"
)

type reviewReporter struct {
	ProgressReporter
	onSuccess func(string)
	onSummary func()
}

func (r *reviewReporter) ReportSuccess(message string) {
	if r.onSuccess != nil {
		r.onSuccess(message)
	}
}
func (r *reviewReporter) ReportSummary(_ *SyncSummary) {
	if r.onSummary != nil {
		r.onSummary()
	}
}

func TestImportReviewPersistsScopeProgressBeforeSummary(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.ImportTask{}))
	server := newCursorFeedServer(t, identityFeedXML(""))
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	checkedProgress, checkedSummary := false, false
	reporter := &reviewReporter{ProgressReporter: NewLogProgressReporter()}
	reporter.onSuccess = func(message string) {
		if !strings.HasPrefix(message, "成功导入:") {
			return
		}
		task, entries, err := GetLatestImportTask(db)
		require.NoError(t, err)
		require.Equal(t, "running", task.Status)
		require.Len(t, entries, 2)
		require.Equal(t, 1, task.Processed)
		require.Equal(t, ImportOutcomeNew, entries[0].Outcome)
		require.Equal(t, "unprocessed", entries[1].Outcome)
		checkedProgress = true
	}
	reporter.onSummary = func() {
		task, entries, err := GetLatestImportTask(db)
		require.NoError(t, err)
		require.Equal(t, "completed", task.Status)
		require.Len(t, entries, 2)
		require.Equal(t, 2, task.Processed)
		checkedSummary = true
	}
	_, _, err = service.RunImportTask(db, "review.opml", []opml.Outline{
		{Title: "first", XMLURL: server.URL}, {Title: "invalid", XMLURL: "not-a-url"},
	}, reporter, ImportConfig{Concurrency: 1}, nil)
	require.NoError(t, err)
	require.True(t, checkedProgress)
	require.True(t, checkedSummary)
}

func TestImportReviewPreservesMissingMetadataAndPreferences(t *testing.T) {
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	existing := &models.Podcast{XYZID: "review", Title: "Title", FeedURL: "https://example.com/rss", Description: "known", Author: "author", Link: "https://example.com", CoverURL: "cover", ITunesID: "123", Priority: -1, UpdateFrequency: 7, IsSubscribed: true, FeedURLValid: true}
	require.NoError(t, db.Create(existing).Error)
	incoming := &models.Podcast{Title: "Title", FeedURL: existing.FeedURL, IsSubscribed: true, FeedURLValid: true}
	_, err = service.saveImportPodcast(incoming, resolvedPodcastIdentity{kind: identityByFeedURL, podcast: existing})
	require.NoError(t, err)
	var saved models.Podcast
	require.NoError(t, db.First(&saved, existing.ID).Error)
	require.Equal(t, "known", saved.Description)
	require.Equal(t, "author", saved.Author)
	require.Equal(t, "cover", saved.CoverURL)
	require.Equal(t, existing.Link, saved.Link)
	require.Equal(t, "123", saved.ITunesID)
	require.Equal(t, -1, saved.Priority)
	require.Equal(t, 7, saved.UpdateFrequency)
}

func TestImportReviewOnlineMetadataOverridesIndex(t *testing.T) {
	server := newCursorFeedServer(t, `<rss version="2.0"><channel><title>Live</title><description>Live description</description><link>https://live.example</link></channel></rss>`)
	db := setupTestDB(t)
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	updated, err := service.updatePodcastMetadataOnline(&models.Podcast{Title: "Old index", Description: "Old", FeedURL: server.URL, ITunesID: "123"}, NewLogProgressReporter())
	require.NoError(t, err)
	require.Equal(t, "Live", updated.Title)
	require.Equal(t, "Live description", updated.Description)
	require.Equal(t, "https://live.example", updated.Link)
	require.Equal(t, "123", updated.ITunesID)
}

func TestImportReviewConcurrentAddressesDoNotDuplicateIdentity(t *testing.T) {
	db := setupTestDB(t)
	server := newCursorFeedServer(t, identityFeedXML("123456"))
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	result, err := service.ImportOPMLOutlines([]opml.Outline{{Title: "one", XMLURL: server.URL + "/one"}, {Title: "two", XMLURL: server.URL + "/two"}}, NewLogProgressReporter(), ImportConfig{Concurrency: 2}, nil)
	require.NoError(t, err)
	require.Equal(t, 1, result.SuccessPodcasts)
	require.Equal(t, 1, result.ConflictPodcasts)
	var count int64
	require.NoError(t, db.Model(&models.Podcast{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestImportReviewDeletedStableIdentityNeedsConfirmation(t *testing.T) {
	db := setupTestDB(t)
	server := newCursorFeedServer(t, identityFeedXML("123456"))
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	podcast := models.Podcast{XYZID: "deleted-review", Title: "Deleted", FeedURL: "https://old.example/rss", ITunesID: "123456"}
	require.NoError(t, db.Create(&podcast).Error)
	require.NoError(t, db.Delete(&podcast).Error)
	result, err := service.ImportOPMLOutlines([]opml.Outline{{Title: "one", XMLURL: server.URL}}, NewLogProgressReporter(), DefaultImportConfig, nil)
	require.NoError(t, err)
	require.Equal(t, 1, result.ConflictPodcasts)
	require.Contains(t, result.Entries[0].Detail, "已删除")
	var count int64
	require.NoError(t, db.Model(&models.Podcast{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestImportReviewFinalPersistenceFailureDoesNotPublishSuccess(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.ImportTask{}))
	service, err := NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("fail-final", func(tx *gorm.DB) {
		if updates, ok := tx.Statement.Dest.(map[string]interface{}); ok && updates["status"] == models.ImportTaskStatusCompleted {
			tx.AddError(errors.New("disk unavailable"))
		}
	}))
	summary := false
	reporter := &reviewReporter{ProgressReporter: NewLogProgressReporter(), onSummary: func() { summary = true }}
	task, _, err := service.RunImportTask(db, "failure.opml", []opml.Outline{{Title: "invalid", XMLURL: "invalid"}}, reporter, DefaultImportConfig, nil)
	require.ErrorContains(t, err, "disk unavailable")
	require.False(t, summary)
	saved, entries, err := GetImportTask(db, task.ID)
	require.NoError(t, err)
	require.Equal(t, models.ImportTaskStatusInterrupted, saved.Status)
	require.Len(t, entries, 1)
	require.Equal(t, ImportOutcomeFailed, entries[0].Outcome)
	require.Equal(t, 1, saved.Processed)
}
