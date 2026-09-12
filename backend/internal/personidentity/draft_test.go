package personidentity

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"magicpodcast/internal/contentsearch"
	"magicpodcast/internal/models"
	"magicpodcast/internal/processing"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func reviewFixture(t *testing.T) (*Service, EpisodeSources) {
	t.Helper()
	db := openPersonIdentityDB(t)
	pod := models.Podcast{XYZID: "review", Title: "访谈", FeedURL: "https://example.test/review"}
	require.NoError(t, db.Create(&pod).Error)
	ep := models.Episode{PodcastID: pod.ID, GUID: "review", Title: "人物核对"}
	require.NoError(t, db.Create(&ep).Error)
	search, err := contentsearch.NewService(db)
	require.NoError(t, err)
	service, err := NewService(db, stubSuggester{candidates: []SuggestedCandidate{
		{DisplayName: "小林", Role: RoleHost, Status: StatusConfirmed, EvidenceKind: "verified_runtime", SpeechOrders: []int{1, 3}},
		{DisplayName: "小周", Role: RoleGuest, Status: StatusConfirmed, EvidenceKind: "verified_runtime", SpeechOrders: []int{2}},
	}}, search)
	require.NoError(t, err)
	return service, EpisodeSources{EpisodeID: ep.ID, SourceKind: SourceTranscript, SourceVersion: "review-v1", Segments: []Segment{
		{Order: 1, SpeakerLabel: "Speaker 1", Text: "我不赞成无限制加班。"},
		{Order: 2, SpeakerLabel: "Speaker 2", Text: "应该保持休息。"},
		{Order: 3, SpeakerLabel: "Speaker 1", Text: "工作需要效率。"},
		{Order: 4, SpeakerLabel: "Speaker 1", Text: "这句无法确定。"},
	}}
}
func draftRequest(p EpisodePeople) ReviewRequest {
	return ReviewRequest{DraftID: p.Draft.ID, Revision: p.Revision, SourceVersion: p.Draft.SourceVersion, Matches: p.Draft.Matches}
}

func TestReviewDraftPersistsWithoutPublishingThenAppliesSelectedMatches(t *testing.T) {
	s, src := reviewFixture(t)
	ctx := context.Background()
	p, err := s.Prepare(ctx, src)
	require.NoError(t, err)
	require.Empty(t, p.People)
	require.Empty(t, p.Attributions)
	require.Len(t, p.Draft.Matches, 2)
	req := draftRequest(p)
	req.Matches[0].DisplayName = "林老师"
	req.Matches[1].Selected = false
	p, err = s.Review(ctx, src.EpisodeID, req, false)
	require.NoError(t, err)
	require.Empty(t, p.People)
	reopened, err := NewService(s.db, nil, s.search)
	require.NoError(t, err)
	p, err = reopened.ListEpisodePeople(ctx, src.EpisodeID)
	require.NoError(t, err)
	require.Equal(t, "林老师", p.Draft.Matches[0].DisplayName)
	req = draftRequest(p)
	p, err = reopened.Review(ctx, src.EpisodeID, req, true)
	require.NoError(t, err)
	require.Len(t, p.People, 1)
	require.Equal(t, "林老师", p.People[0].DisplayName)
	require.Equal(t, 2, p.People[0].ConfirmedSpeech)
	require.Equal(t, StatusPending, p.Attributions[3].Status)
	require.True(t, p.Attributions[0].UserConfirmed)
	hits, err := s.search.Search(ctx, contentsearch.Request{Query: "加班", Scope: contentsearch.Scope{EpisodeIDs: []uint{src.EpisodeID}}, Filter: contentsearch.Filter{PersonID: &p.People[0].ID}, Limit: 8})
	require.NoError(t, err)
	require.Len(t, hits.Hits, 1)
	retry, err := s.Review(ctx, src.EpisodeID, req, true)
	require.NoError(t, err)
	require.Equal(t, p.Revision, retry.Revision)
	req.Matches[0].DisplayName = "篡改重试"
	_, err = s.Review(ctx, src.EpisodeID, req, true)
	require.ErrorIs(t, err, ErrSourcesChanged)
	// Re-identification only produces a new draft. Applied facts retain manual spelling.
	p, err = s.Prepare(ctx, src)
	require.NoError(t, err)
	require.Equal(t, "林老师", p.People[0].DisplayName)
	history, err := s.ReviewHistory(ctx, src.EpisodeID)
	require.NoError(t, err)
	require.Len(t, history, 2)
}

func TestReviewRejectsStaleConcurrentConflictingAndChangedSources(t *testing.T) {
	s, src := reviewFixture(t)
	ctx := context.Background()
	p, err := s.Prepare(ctx, src)
	require.NoError(t, err)
	stale := draftRequest(p)
	p, err = s.Review(ctx, src.EpisodeID, stale, false)
	require.NoError(t, err)
	_, err = s.Review(ctx, src.EpisodeID, stale, true)
	require.ErrorIs(t, err, ErrSourcesChanged)
	req := draftRequest(p)
	req.Matches[1].SpeakerLabel = "Speaker 1"
	req.Matches[1].Orders = []int{1}
	_, err = s.Review(ctx, src.EpisodeID, req, true)
	require.ErrorIs(t, err, ErrInvalidCorrection)
	require.NoError(t, s.db.Model(&models.Episode{}).Where("id = ?", src.EpisodeID).Update("title", "已变更").Error)
	_, err = s.Review(ctx, src.EpisodeID, draftRequest(p), true)
	require.ErrorIs(t, err, ErrSourcesChanged)
	current, err := s.ListEpisodePeople(ctx, src.EpisodeID)
	require.NoError(t, err)
	require.Empty(t, current.People)
	require.True(t, current.Draft.Outdated)
}

// Existing lifecycle tests start from an explicitly approved fixture. The new
// review tests above exercise the unapproved boundary without this helper.
func prepareReviewed(s *Service, ctx context.Context, src EpisodeSources) (EpisodePeople, error) {
	p, err := s.Prepare(ctx, src)
	if err != nil || p.Draft == nil {
		return p, err
	}
	for i := range p.Draft.Matches {
		p.Draft.Matches[i].Selected = true
	}
	return s.Review(ctx, src.EpisodeID, draftRequest(p), true)
}

type reviewReader struct {
	segments []processing.TranscriptSegment
}

func (r reviewReader) ReadText(context.Context, models.EpisodeArtifactSet, string) (processing.ArtifactContent, error) {
	return processing.ArtifactContent{Segments: r.segments}, nil
}
func publishReviewFixture(t *testing.T, s *Service, src EpisodeSources) EpisodeSources {
	t.Helper()
	now := time.Now().UTC()
	run := models.EpisodeProcessingRun{EpisodeID: src.EpisodeID, AudioDigest: strings.Repeat("b", 64), PipelineVersion: processing.NativeMinutesPipelineVersion, TriggerSource: models.ProcessingTriggerManual, Status: models.ProcessingRunStatusCompleted, CurrentStep: processing.StepArtifactPublish, AttemptCount: 1, MaxAttempts: 3, RetryDeadlineAt: now.Add(time.Hour), FinishedAt: &now, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, s.db.Create(&run).Error)
	artifact := models.EpisodeArtifactSet{RunID: run.ID, EpisodeID: src.EpisodeID, PipelineVersion: processing.NativeMinutesPipelineVersion, IsCurrent: true, CreatedAt: now}
	require.NoError(t, s.db.Create(&artifact).Error)
	timeline := []processing.TranscriptSegment{}
	for _, seg := range src.Segments {
		timeline = append(timeline, processing.TranscriptSegment{Order: seg.Order, Speaker: seg.SpeakerLabel, StartMS: seg.StartMS, Text: seg.Text})
	}
	s.WithArtifactReader(reviewReader{timeline})
	src.SourceVersion = fmt.Sprintf("artifact-%d", artifact.ID)
	return src
}
func TestManualSpeakerEditsAreScopedAndWorkWithoutRecognition(t *testing.T) {
	s, src := reviewFixture(t)
	src = publishReviewFixture(t, s, src)
	ctx := context.Background()
	first, err := s.ApplyManual(ctx, src.EpisodeID, ManualMatch{SourceVersion: src.SourceVersion, FragmentOrder: 1, Scope: "speaker", DisplayName: "林老师"})
	require.NoError(t, err)
	require.Nil(t, first.Draft)
	require.Equal(t, 3, first.People[0].ConfirmedSpeech)
	originalID := first.People[0].ID
	one, err := s.ApplyManual(ctx, src.EpisodeID, ManualMatch{Revision: first.Revision, SourceVersion: src.SourceVersion, FragmentOrder: 3, Scope: "fragment", PersonID: originalID, DisplayName: "周老师"})
	require.NoError(t, err)
	require.Equal(t, "林老师", one.Attributions[0].DisplayName)
	require.Equal(t, "周老师", one.Attributions[2].DisplayName)
	require.Equal(t, "林老师", one.Attributions[3].DisplayName)
	require.NotEqual(t, one.Attributions[0].PersonID, one.Attributions[2].PersonID)
	require.Equal(t, src.Segments[2].Text, one.Attributions[2].Text)
	_, err = s.ApplyManual(ctx, src.EpisodeID, ManualMatch{Revision: first.Revision, SourceVersion: src.SourceVersion, FragmentOrder: 1, Scope: "speaker", DisplayName: "过期修改"})
	require.ErrorIs(t, err, ErrSourcesChanged)
	cleared, err := s.ApplyManual(ctx, src.EpisodeID, ManualMatch{Revision: one.Revision, SourceVersion: src.SourceVersion, FragmentOrder: 3, Scope: "fragment", Clear: true})
	require.NoError(t, err)
	require.Nil(t, cleared.Attributions[2].PersonID)
	require.Equal(t, "林老师", cleared.Attributions[0].DisplayName)
	speech, err := s.ReliableSpeech(ctx, src.EpisodeID, *one.Attributions[2].PersonID)
	require.NoError(t, err)
	require.Empty(t, speech)
}
func TestReviewSurvivesDatabaseCloseAndReopen(t *testing.T) {
	s, src := reviewFixture(t)
	ctx := context.Background()
	p, err := s.Prepare(ctx, src)
	require.NoError(t, err)
	req := draftRequest(p)
	req.Matches[0].DisplayName = "已保存的草稿名"
	_, err = s.Review(ctx, src.EpisodeID, req, false)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "review.sqlite")
	require.NoError(t, s.db.Exec("VACUUM INTO ?", path).Error)
	reopen := func() (*Service, func()) {
		db, err := gorm.Open(sqlite.Open(path+"?_foreign_keys=on"), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		search, err := contentsearch.NewService(db)
		require.NoError(t, err)
		reader, err := NewService(db, nil, search)
		require.NoError(t, err)
		return reader, func() { require.NoError(t, sqlDB.Close()) }
	}
	first, closeFirst := reopen()
	p, err = first.ListEpisodePeople(ctx, src.EpisodeID)
	require.NoError(t, err)
	require.Equal(t, "已保存的草稿名", p.Draft.Matches[0].DisplayName)
	require.Empty(t, p.People)
	p, err = first.Review(ctx, src.EpisodeID, draftRequest(p), true)
	require.NoError(t, err)
	closeFirst()
	second, closeSecond := reopen()
	defer closeSecond()
	restored, err := second.ListEpisodePeople(ctx, src.EpisodeID)
	require.NoError(t, err)
	require.Equal(t, p.People, restored.People)
	require.Equal(t, p.Attributions, restored.Attributions)
	require.NotNil(t, restored.Draft)
}

func TestReviewExplicitRenameDoesNotResolveBackThroughOldAliases(t *testing.T) {
	s, src := reviewFixture(t)
	ctx := context.Background()
	suggester := s.suggester.(stubSuggester)
	suggester.candidates[0].Aliases = []string{"小林", "林哥"}
	s.suggester = suggester
	first, err := prepareReviewed(s, ctx, src)
	require.NoError(t, err)
	originalID := *first.Attributions[0].PersonID
	next, err := s.Prepare(ctx, src)
	require.NoError(t, err)
	req := draftRequest(next)
	req.Matches[0].DisplayName = "林老师"
	applied, err := s.Review(ctx, src.EpisodeID, req, true)
	require.NoError(t, err)
	require.Equal(t, "林老师", applied.Attributions[0].DisplayName)
	require.NotEqual(t, originalID, *applied.Attributions[0].PersonID)
	var original models.Person
	require.NoError(t, s.db.First(&original, originalID).Error)
	require.Equal(t, "小林", original.DisplayName)
}

func TestConcurrentReviewCannotOverwriteAnotherApproval(t *testing.T) {
	s, src := reviewFixture(t)
	ctx := context.Background()
	p, err := s.Prepare(ctx, src)
	require.NoError(t, err)
	first := draftRequest(p)
	second := draftRequest(p)
	second.Matches = append([]ReviewMatch(nil), second.Matches...)
	first.Matches[0].DisplayName = "甲确认"
	second.Matches[0].DisplayName = "乙确认"
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, request := range []ReviewRequest{first, second} {
		go func(r ReviewRequest) { <-start; _, err := s.Review(ctx, src.EpisodeID, r, true); results <- err }(request)
	}
	close(start)
	success, stale := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, ErrSourcesChanged)
			stale++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, stale)
	current, err := s.ListEpisodePeople(ctx, src.EpisodeID)
	require.NoError(t, err)
	require.Contains(t, []string{"甲确认", "乙确认"}, current.Attributions[0].DisplayName)
	require.Len(t, current.People, 2)
}
