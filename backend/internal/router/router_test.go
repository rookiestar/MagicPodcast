package router

import (
	"testing"

	"magicpodcast/internal/config"
)

func TestWorkflowPodcastIndexPathUsesConfiguredDatabase(t *testing.T) {
	cfg := &config.Config{
		PodcastIndex: config.PodcastIndexConfig{Path: "./data/podcastindex_feeds.db"},
	}

	if got := workflowPodcastIndexPath(cfg); got != "./data/podcastindex_feeds.db" {
		t.Fatalf("workflowPodcastIndexPath() = %q, want configured PodcastIndex path", got)
	}
}

func TestWorkflowPodcastIndexPathAllowsOptionalDatabase(t *testing.T) {
	if got := workflowPodcastIndexPath(&config.Config{}); got != "" {
		t.Fatalf("workflowPodcastIndexPath() = %q, want empty optional path", got)
	}
	if got := workflowPodcastIndexPath(nil); got != "" {
		t.Fatalf("workflowPodcastIndexPath(nil) = %q, want empty path", got)
	}
}
