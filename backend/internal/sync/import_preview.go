package sync

import (
	"magicpodcast/internal/opml"
)

// ImportEntryKind 是导入预览的条目分类。预览只读、不抓取网络：跨地址的
// 稳定身份冲突需要抓取 Feed 后才能发现，会在导入结果里以 conflict 上报，
// 不在这里伪造结论。
type ImportEntryKind string

const (
	// ImportEntryNew 无本地候选，导入时新建。
	ImportEntryNew ImportEntryKind = "new"
	// ImportEntryExisting 精确 feed_url 命中本地已有节目，导入时更新资料。
	ImportEntryExisting ImportEntryKind = "existing"
	// ImportEntryCollection 命中清单收录创建的无 RSS 记录（xyz_id 一致），
	// 需确认后转关注并绑定订阅地址。
	ImportEntryCollection ImportEntryKind = "collection"
	// ImportEntryDeleted 命中已软删除记录；不静默复活，除非确认。
	ImportEntryDeleted ImportEntryKind = "deleted"
	// ImportEntryInvalid 非法地址，写入前拒绝。
	ImportEntryInvalid ImportEntryKind = "invalid"
	// ImportEntryDuplicate 文件内重复地址，已归并为同一任务条目。
	ImportEntryDuplicate ImportEntryKind = "duplicate"
)

// ImportPreviewEntry 是单个订阅条目的预览结论与证据。
type ImportPreviewEntry struct {
	XMLURL          string          `json:"xml_url"`
	Title           string          `json:"title"`
	Kind            ImportEntryKind `json:"kind"`
	PodcastID       uint            `json:"podcast_id,omitempty"`
	PodcastTitle    string          `json:"podcast_title,omitempty"`
	CurrentFeed     string          `json:"current_feed,omitempty"`
	Subscribed      bool            `json:"subscribed,omitempty"`
	Evidence        string          `json:"evidence,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	DuplicateInFile int             `json:"duplicate_in_file,omitempty"`
}

// ImportPreview 是一次 OPML 预览的完整差异核对结果。
type ImportPreview struct {
	Total                int                  `json:"total"`
	Entries              []ImportPreviewEntry `json:"entries"`
	NewCount             int                  `json:"new_count"`
	ExistingCount        int                  `json:"existing_count"`
	CollectionCount      int                  `json:"collection_count"`
	DeletedCount         int                  `json:"deleted_count"`
	InvalidCount         int                  `json:"invalid_count"`
	DuplicateMergedCount int                  `json:"duplicate_merged_count"`
}

// PreviewImportOPML 对已解析的订阅条目做写入前的补充式差异核对：
// 只分类，不抓取、不写库、不生成取消关注动作。
func (s *Service) PreviewImportOPML(outlines []opml.Outline) (*ImportPreview, error) {
	preview := &ImportPreview{Total: len(outlines), Entries: make([]ImportPreviewEntry, 0)}
	seenURL := make(map[string]int)

	for _, outline := range outlines {
		entry := ImportPreviewEntry{
			XMLURL: outline.XMLURL,
			Title:  outline.GetTitle(),
		}

		// 文件内重复地址：保留首个条目，归并计数。
		if firstIdx, exists := seenURL[outline.XMLURL]; exists {
			preview.Entries[firstIdx].DuplicateInFile++
			preview.DuplicateMergedCount++
			continue
		}

		if err := validateImportFeedURL(outline.XMLURL); err != nil {
			entry.Kind = ImportEntryInvalid
			entry.Reason = "需要完整的 HTTP 或 HTTPS 链接"
			preview.Entries = append(preview.Entries, entry)
			seenURL[outline.XMLURL] = len(preview.Entries) - 1
			preview.InvalidCount++
			continue
		}

		resolved := s.resolveImportIdentity(outline.XMLURL)
		switch resolved.kind {
		case identityByFeedURL:
			entry.Kind = ImportEntryExisting
			entry.PodcastID = resolved.podcast.ID
			entry.PodcastTitle = resolved.podcast.Title
			entry.CurrentFeed = resolved.podcast.FeedURL
			entry.Subscribed = resolved.podcast.IsSubscribed
			entry.Evidence = "feed_url"
			preview.ExistingCount++
		case identityByXyzID:
			entry.Kind = ImportEntryCollection
			entry.PodcastID = resolved.podcast.ID
			entry.PodcastTitle = resolved.podcast.Title
			entry.CurrentFeed = resolved.podcast.FeedURL
			entry.Subscribed = resolved.podcast.IsSubscribed
			entry.Evidence = "xyz_id"
			if resolved.podcast.FeedURL == "" {
				entry.Reason = "清单已收录同名来源节目（外部 ID 一致），确认后转为关注并绑定订阅地址"
			} else {
				entry.Reason = "清单收录记录已绑定其他订阅地址，确认后改为本次地址"
			}
			preview.CollectionCount++
		case identityDeleted:
			entry.Kind = ImportEntryDeleted
			entry.PodcastID = resolved.podcast.ID
			entry.PodcastTitle = resolved.podcast.Title
			entry.Reason = "本地已有同地址的已删除记录；不会自动恢复，需确认后重新关注"
			preview.DeletedCount++
		default:
			entry.Kind = ImportEntryNew
			preview.NewCount++
		}
		preview.Entries = append(preview.Entries, entry)
		seenURL[outline.XMLURL] = len(preview.Entries) - 1
	}

	return preview, nil
}
