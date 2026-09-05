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
