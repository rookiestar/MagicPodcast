package models

import (
	"time"
)

// EpisodeCollection 播客单集清单：外部来源的发现资料，独立于个人播客库。
// 同一源平台 + 外部清单 ID 唯一；导入不写入个人节目、单集或行动队列。
type EpisodeCollection struct {
	BaseModel

	SourcePlatform  string     `gorm:"size:32;not null;uniqueIndex:idx_episode_collections_source_external,priority:1" json:"source_platform"` // 来源平台，如 xiaoyuzhoufm
	ExternalID      string     `gorm:"size:64;not null;uniqueIndex:idx_episode_collections_source_external,priority:2" json:"external_id"`     // 源清单 ID
	Title           string     `gorm:"size:255;not null" json:"title"`                                                                         // 清单标题
	Description     string     `gorm:"type:text" json:"description"`                                                                           // 清单简介（源站原文）
	Author          string     `gorm:"size:255" json:"author"`                                                                                 // 源清单作者昵称，不能由内容推测
	SourceURL       string     `gorm:"size:512;not null" json:"source_url"`                                                                    // 源清单地址
	TotalKnown      bool       `gorm:"not null;default:false" json:"total_known"`                                                              // 能否核实源站总数；false 时只表示已读取条数
	Revision        int        `gorm:"not null;default:1" json:"revision"`                                                                     // 当前内容修订号，刷新确认时递增
	LastRefreshedAt *time.Time `json:"last_refreshed_at"`                                                                                      // 最近一次确认导入/刷新成功的时间

	Items []EpisodeCollectionItem `gorm:"foreignKey:CollectionID;constraint:OnDelete:CASCADE" json:"items,omitempty"`
}

// EpisodeCollectionItem 清单条目：某一集在某份清单中的位置、推荐语与外部身份快照。
// 未采纳条目只是发现元数据，可空关联本地单集；队列、备注和产物只存在于既有 Episode。
type EpisodeCollectionItem struct {
	BaseModel

	CollectionID      uint       `gorm:"not null;uniqueIndex:idx_episode_collection_items_collection_eid,priority:1" json:"collection_id"`
	Position          int        `gorm:"not null" json:"position"`                                                                                       // 原始顺序，从 0 开始
	ExternalEpisodeID string     `gorm:"size:64;not null;uniqueIndex:idx_episode_collection_items_collection_eid,priority:2" json:"external_episode_id"` // 源平台单集 ID
	ExternalPodcastID string     `gorm:"size:64;index" json:"external_podcast_id"`                                                                       // 源平台节目 ID
	PodcastTitle      string     `gorm:"size:255;not null" json:"podcast_title"`
	PodcastAuthor     string     `gorm:"size:255" json:"podcast_author"`
	PodcastCoverURL   string     `gorm:"size:512" json:"podcast_cover_url"`
	EpisodeTitle      string     `gorm:"size:512;not null" json:"episode_title"`
	Recommendation    string     `gorm:"type:text" json:"recommendation"` // 清单推荐语，缺失时不编造
	Shownotes         string     `gorm:"type:text" json:"shownotes"`
	Duration          int        `gorm:"not null;default:0" json:"duration"` // 秒
	PublishedAt       *time.Time `json:"published_at"`
	ImageURL          string     `gorm:"size:512" json:"image_url"`
	EpisodeURL        string     `gorm:"size:512" json:"episode_url"`                          // 源平台单集链接
	PayType           string     `gorm:"size:32;not null;default:''" json:"pay_type"`          // 源站付费标记快照，如 FREE
	IsPrivateMedia    bool       `gorm:"not null;default:false" json:"is_private_media"`       // 源站私密音频标记快照
	EpisodeID         *uint      `gorm:"index;constraint:OnDelete:SET NULL" json:"episode_id"` // 已采纳的本地单集；删除个人单集时置空

	Collection EpisodeCollection `gorm:"foreignKey:CollectionID" json:"-"`
}

// TableName 指定表名
func (EpisodeCollection) TableName() string {
	return "episode_collections"
}

// EpisodeCollectionsCreateSQL 建表语句，供版本化迁移使用，保持与模型标签一致。
const EpisodeCollectionsCreateSQL = `CREATE TABLE IF NOT EXISTS episode_collections (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	source_platform TEXT NOT NULL,
	external_id TEXT NOT NULL,
	title TEXT NOT NULL,
	description TEXT,
	author TEXT,
	source_url TEXT NOT NULL,
	total_known NUMERIC NOT NULL DEFAULT false,
	revision INTEGER NOT NULL DEFAULT 1,
	last_refreshed_at DATETIME
)`

// EpisodeCollectionsSourceExternalUniqueIndexSQL 清单按源平台与外部 ID 唯一。
const EpisodeCollectionsSourceExternalUniqueIndexSQL = `CREATE UNIQUE INDEX IF NOT EXISTS idx_episode_collections_source_external ON episode_collections(source_platform, external_id)`

// EpisodeCollectionItemsCreateSQL 建表语句，供版本化迁移使用，保持与模型标签一致。
const EpisodeCollectionItemsCreateSQL = `CREATE TABLE IF NOT EXISTS episode_collection_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	collection_id INTEGER NOT NULL,
	position INTEGER NOT NULL,
	external_episode_id TEXT NOT NULL,
	external_podcast_id TEXT,
	podcast_title TEXT NOT NULL,
	podcast_author TEXT,
	podcast_cover_url TEXT,
	episode_title TEXT NOT NULL,
	recommendation TEXT,
	shownotes TEXT,
	duration NUMERIC NOT NULL DEFAULT 0,
	published_at DATETIME,
	image_url TEXT,
	episode_url TEXT,
	pay_type TEXT NOT NULL DEFAULT '',
	is_private_media NUMERIC NOT NULL DEFAULT false,
	episode_id INTEGER,
	CONSTRAINT fk_episode_collection_items_collection FOREIGN KEY (collection_id) REFERENCES episode_collections(id) ON DELETE CASCADE,
	CONSTRAINT fk_episode_collection_items_episode FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE SET NULL
)`

// EpisodeCollectionItemsCollectionEidUniqueIndexSQL 清单内按外部单集身份去重。
const EpisodeCollectionItemsCollectionEidUniqueIndexSQL = `CREATE UNIQUE INDEX IF NOT EXISTS idx_episode_collection_items_collection_eid ON episode_collection_items(collection_id, external_episode_id)`

// EpisodeCollectionItemsEpisodeIndexSQL 按本地单集反查清单条目。
const EpisodeCollectionItemsEpisodeIndexSQL = `CREATE INDEX IF NOT EXISTS idx_episode_collection_items_episode ON episode_collection_items(episode_id)`

// EpisodeCollectionItemsPodcastIndexSQL 按源平台节目 ID 汇总条目。
const EpisodeCollectionItemsPodcastIndexSQL = `CREATE INDEX IF NOT EXISTS idx_episode_collection_items_external_podcast_id ON episode_collection_items(external_podcast_id)`

// EpisodeCollectionsDeletedAtIndexSQL 与 GORM 软删除索引保持一致。
const EpisodeCollectionsDeletedAtIndexSQL = `CREATE INDEX IF NOT EXISTS idx_episode_collections_deleted_at ON episode_collections(deleted_at)`

// EpisodeCollectionItemsDeletedAtIndexSQL 与 GORM 软删除索引保持一致。
const EpisodeCollectionItemsDeletedAtIndexSQL = `CREATE INDEX IF NOT EXISTS idx_episode_collection_items_deleted_at ON episode_collection_items(deleted_at)`

// TableName 指定表名
func (EpisodeCollectionItem) TableName() string {
	return "episode_collection_items"
}
