package main

import "testing"

func TestBuildEpisodeRepairUsesSharedOriginalLinkResolver(t *testing.T) {
	candidate := episodeCandidate{ID: 1, PodcastID: 2, EmptyLink: true}

	tests := []struct {
		name    string
		feedURL string
		item    *feedItem
		want    string
	}{
		{
			name:    "standard link",
			feedURL: "https://example.com/feed.xml",
			item:    &feedItem{Link: "https://example.com/episode/1"},
			want:    "https://example.com/episode/1",
		},
		{
			name:    "malformed standard link",
			feedURL: "https://example.com/feed.xml",
			item:    &feedItem{Link: "https://"},
		},
		{
			name:    "verified Libsyn content page",
			feedURL: "https://investlikethebest.libsyn.com/rss",
			item: &feedItem{
				Content: `<p>episode page <a href="https://colossus.com/episode/ai-market-jitters/">here</a></p>`,
			},
			want: "https://colossus.com/episode/ai-market-jitters/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repair := buildEpisodeRepair(candidate, tt.item, "guid", tt.feedURL, true)
			got := ""
			for _, field := range repair.Fields {
				if field.Field == "link" {
					got = field.Value
				}
			}
			if got != tt.want {
				t.Fatalf("link repair = %q, want %q", got, tt.want)
			}
		})
	}
}
