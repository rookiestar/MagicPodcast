package workflow

import (
	"strings"
	"testing"
	"time"

	"magicpodcast/internal/models"
)

func TestConvertToLLMReportDataCopiesPodcastAndEpisodeFields(t *testing.T) {
	updatedAt := time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC)
	publishedAt := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)
	data := []EpisodeReportData{
		{
			PodcastID:      7,
			PodcastTitle:   "播客",
			PodcastFeedURL: "https://example.com/feed.xml",
			Episodes: []EpisodeDetail{
				{
					Title:         "单集",
					ShowNotes:     "Shownotes",
					PublishedDate: publishedAt,
					UpdatedDate:   &updatedAt,
					EpisodeNo:     "42",
					Link:          "https://example.com/episode",
					XYZID:         "xyz",
					QRCode:        "base64",
					QRCodeError:   true,
				},
			},
		},
	}

	converted := ConvertToLLMReportData(data)

	if len(converted) != 1 {
		t.Fatalf("expected 1 podcast, got %d", len(converted))
	}
	if converted[0].PodcastID != data[0].PodcastID ||
		converted[0].PodcastTitle != data[0].PodcastTitle ||
		converted[0].PodcastFeedURL != data[0].PodcastFeedURL {
		t.Fatalf("podcast fields were not preserved: %#v", converted[0])
	}
	if len(converted[0].Episodes) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(converted[0].Episodes))
	}

	episode := converted[0].Episodes[0]
	source := data[0].Episodes[0]
	if episode.Title != source.Title ||
		episode.ShowNotes != source.ShowNotes ||
		!episode.PublishedDate.Equal(source.PublishedDate) ||
		episode.UpdatedDate != source.UpdatedDate ||
		episode.EpisodeNo != source.EpisodeNo ||
		episode.Link != source.Link ||
		episode.XYZID != source.XYZID ||
		episode.QRCode != source.QRCode ||
		episode.QRCodeError != source.QRCodeError {
		t.Fatalf("episode fields were not preserved: %#v", episode)
	}
}

func TestGenerateMarkdownIncludesFeedCoverage(t *testing.T) {
	rg := &ReportGenerator{}
	job := &models.Job{BaseModel: models.BaseModel{CreatedAt: time.Date(2026, 7, 25, 16, 0, 0, 0, time.UTC)}, TriggeredBy: "manual"}
	markdown := rg.generateMarkdown(job, nil, job.CreatedAt, job.CreatedAt, "manual", "覆盖报告", 0, "", FeedCoverageSummary{
		Total: 29, Attempted: 15, Successes: 7, Failures: 8, Unattempted: 14,
	})
	if !strings.Contains(markdown, "Feed覆盖**: 15/29 已尝试 | 7 成功 | 8 失败 | 14 未尝试") {
		t.Fatalf("coverage summary missing from report: %s", markdown)
	}
}
