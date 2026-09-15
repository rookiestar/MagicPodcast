package podcastindex

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestScanToleratesEmptyTextInNumericColumns 复现官方数据集的空文本数字列
// （理想屯、Orpheus微见等样本）：整行必须可读，空值保持未知而非默认为
// 有效业务事实（#420 AC14 最小兼容）。
func TestScanToleratesEmptyTextInNumericColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "podcastindex-empty-text.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE podcasts (
  id INTEGER PRIMARY KEY, url TEXT NOT NULL, title TEXT NOT NULL,
  lastUpdate INTEGER, link TEXT, lastHttpStatus INTEGER, dead INTEGER,
  itunesAuthor TEXT, itunesId INTEGER, imageUrl TEXT,
  newestItemPubdate INTEGER, language TEXT, oldestItemPubdate INTEGER,
  episodeCount INTEGER, popularityScore INTEGER, priority INTEGER,
  updateFrequency INTEGER, newestEnclosureUrl TEXT, podcastGuid TEXT,
  description TEXT, newestEnclosureDuration INTEGER)`); err != nil {
		t.Fatal(err)
	}
	// 官方样本签名：多个数字列存入空文本。
	if _, err := db.Exec(`INSERT INTO podcasts
 (id, url, title, lastUpdate, link, lastHttpStatus, dead, itunesAuthor, itunesId,
  imageUrl, newestItemPubdate, language, oldestItemPubdate, episodeCount,
  popularityScore, priority, updateFrequency, newestEnclosureUrl, podcastGuid,
  description, newestEnclosureDuration)
 VALUES (1, 'https://empty.example/feed.xml', '理想屯', '', 'https://example.com', '',
         '', '作者', '', 'https://example.com/image.jpg', '', 'zh', '', 7, '', 1, '',
         'https://example.com/episode.mp3', 'guid-empty', 'description', '')`); err != nil {
		t.Fatal(err)
	}
	query, err := NewQuery(path)
	if err != nil {
		t.Fatal(err)
	}
	defer query.Close()

	info, err := query.FindByFeedURL("https://empty.example/feed.xml")
	if err != nil || info == nil {
		t.Fatalf("FindByFeedURL() info=%+v err=%v", info, err)
	}
	if info.EpisodeCount != 7 {
		t.Fatalf("episodeCount = %d, want the readable numeric value", info.EpisodeCount)
	}
	if info.ITunesID != 0 || info.LastUpdate != 0 || info.NewestEnclosureDuration != 0 || info.PopularityScore != 0 || info.NewestItemPubdate != 0 {
		t.Fatalf("empty text must stay unknown, got %+v", info)
	}
	if info.Priority != 1 {
		t.Fatalf("priority = %d, want readable numeric value 1", info.Priority)
	}
	if _, err := query.FindByFeedURLContext(context.Background(), "https://empty.example/feed.xml"); err != nil {
		t.Fatalf("context lookup must tolerate empty text too: %v", err)
	}
}
