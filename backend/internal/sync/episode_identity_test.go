package sync

import (
	"testing"
	"time"

	"magicpodcast/internal/models"

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

func TestEpisodeIdentityKeyMatchesChineseSpacingVariants(t *testing.T) {
	published := time.Date(2026, 8, 21, 16, 38, 26, 0, time.UTC)
	tight := &models.Episode{
		PodcastID:     7,
		Title:         "第229期 永远的钟鼓楼",
		PublishedDate: published,
	}
	spaced := &models.Episode{
		PodcastID:     7,
		Title:         "第 229 期 永远的钟鼓楼",
		PublishedDate: published,
	}
	other := &models.Episode{
		PodcastID:     7,
		Title:         "第230期 机器人运动会",
		PublishedDate: published,
	}

	tightKey, tightOK := episodeIdentityKey(tight)
	spacedKey, spacedOK := episodeIdentityKey(spaced)
	if !tightOK || !spacedOK {
		t.Fatalf("episodeIdentityKey ok = %v/%v, want both true", tightOK, spacedOK)
	}
	if tightKey != spacedKey {
		t.Fatalf("identity keys differ: %q vs %q", tightKey, spacedKey)
	}
	otherKey, _ := episodeIdentityKey(other)
	if tightKey == otherKey {
		t.Fatalf("different episode numbers must not share an identity key: %q", tightKey)
	}
}

func TestEpisodeNoFromItemRecognizesChineseMarker(t *testing.T) {
	item := &gofeed.Item{Title: "第 229 期 永远的钟鼓楼"}

	if got := episodeNoFromItem(item); got != "229" {
		t.Fatalf("episodeNoFromItem() = %q, want 229 from the chinese marker", got)
	}
}
