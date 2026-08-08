package sync

import (
	"testing"

	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
)

func TestEpisodeNoFromItemDoesNotUseUnreliableITunesPosition(t *testing.T) {
	item := &gofeed.Item{
		Title: "昆山杜克大学周忆粟：AI 来了，年轻人的梯子被抽掉了",
		ITunesExt: &ext.ITunesItemExtension{
			Episode: "1",
		},
	}

	if got := episodeNoFromItem(item); got != "" {
		t.Fatalf("episodeNoFromItem() = %q, want empty for an unlabelled title", got)
	}
}

func TestEpisodeNoFromItemPrefersValidatedTitleLabel(t *testing.T) {
	item := &gofeed.Item{
		Title: "E246 对话餐饮收尸人",
		ITunesExt: &ext.ITunesItemExtension{
			Episode: "1",
		},
	}

	if got := episodeNoFromItem(item); got != "246" {
		t.Fatalf("episodeNoFromItem() = %q, want 246 from the title", got)
	}
}
