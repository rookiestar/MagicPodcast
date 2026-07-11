package workflow

import (
	"testing"
	"time"
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
