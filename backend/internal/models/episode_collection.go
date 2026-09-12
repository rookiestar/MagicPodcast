package models

import (
	"time"
)

// SourcePlatformXiaoyuzhoufm 当前唯一支持的清单来源平台标识。
const SourcePlatformXiaoyuzhoufm = "xiaoyuzhoufm"

// EpisodeExternalRef 已收录单集的外部身份到本地单集的映射。
// source + external_episode_id 唯一；校验所属节目一致，不承担去重以外的职责。
type EpisodeExternalRef struct {
	BaseModel

	SourcePlatform    string `gorm:"size:32;not null;uniqueIndex:idx_episode_external_refs_identity,priority:1" json:"source_platform"`
	ExternalEpisodeID string `gorm:"size:64;not null;uniqueIndex:idx_episode_external_refs_identity,priority:2" json:"external_episode_id"`
	ExternalPodcastID string `gorm:"size:64;not null;default:''" json:"external_podcast_id"`
	EpisodeID         uint   `gorm:"not null;index" json:"episode_id"`
	PodcastID         uint   `gorm:"not null" json:"podcast_id"`
}

// EpisodeCollectionAdoption 单集被采纳的清单来源摘要（每集每清单一条）。
// 删除清单不删除该摘要；删除个人单集按既有删除规则级联清理。
type EpisodeCollectionAdoption struct {
	BaseModel

	EpisodeID            uint      `gorm:"not null;uniqueIndex:idx_episode_collection_adoptions_episode_collection,priority:1" json:"episode_id"`
	SourcePlatform       string    `gorm:"size:32;not null;uniqueIndex:idx_episode_collection_adoptions_episode_collection,priority:2" json:"source_platform"`
	CollectionExternalID string    `gorm:"size:64;not null;uniqueIndex:idx_episode_collection_adoptions_episode_collection,priority:3" json:"collection_external_id"`
	CollectionTitle      string    `gorm:"size:255;not null" json:"collection_title"`
	CollectionURL        string    `gorm:"size:512;not null" json:"collection_url"`
	ExternalEpisodeID    string    `gorm:"size:64;not null;default:''" json:"external_episode_id"`
	AdoptedAt            time.Time `gorm:"not null" json:"adopted_at"`
}

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

	CollectionID        uint       `gorm:"not null;uniqueIndex:idx_episode_collection_items_collection_eid,priority:1" json:"collection_id"`
	Position            int        `gorm:"not null" json:"position"`                                                                                       // 原始顺序，从 0 开始
	ExternalEpisodeID   string     `gorm:"size:64;not null;uniqueIndex:idx_episode_collection_items_collection_eid,priority:2" json:"external_episode_id"` // 源平台单集 ID
	ExternalPodcastID   string     `gorm:"size:64;index" json:"external_podcast_id"`                                                                       // 源平台节目 ID
	PodcastTitle        string     `gorm:"size:255;not null" json:"podcast_title"`
	PodcastAuthor       string     `gorm:"size:255" json:"podcast_author"`
	PodcastCoverURL     string     `gorm:"size:512" json:"podcast_cover_url"`
	PodcastEpisodeCount int        `gorm:"not null;default:0" json:"podcast_episode_count"` // 源站总集数快照，0=未知
	EpisodeTitle        string     `gorm:"size:512;not null" json:"episode_title"`
	Recommendation      string     `gorm:"type:text" json:"recommendation"` // 清单推荐语，缺失时不编造
	Shownotes           string     `gorm:"type:text" json:"shownotes"`
	Duration            int        `gorm:"not null;default:0" json:"duration"` // 秒
	PublishedAt         *time.Time `json:"published_at"`
	ImageURL            string     `gorm:"size:512" json:"image_url"`
	EpisodeURL          string     `gorm:"size:512" json:"episode_url"`                          // 源平台单集链接
	PayType             string     `gorm:"size:32;not null;default:''" json:"pay_type"`          // 源站付费标记快照，如 FREE
	IsPrivateMedia      bool       `gorm:"not null;default:false" json:"is_private_media"`       // 源站私密音频标记快照
	AudioURL            string     `gorm:"size:512" json:"audio_url"`                            // 源站公开音频地址快照；私密/付费留空
	AudioMimeType       string     `gorm:"size:100" json:"audio_mime_type"`                      // 音频 MIME 快照，未知留空
	AudioSize           int64      `gorm:"not null;default:0" json:"audio_size"`                 // 音频字节快照，未知为 0
	EpisodeID           *uint      `gorm:"index;constraint:OnDelete:SET NULL" json:"episode_id"` // 已采纳的本地单集；删除个人单集时置空

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
	podcast_episode_count NUMERIC NOT NULL DEFAULT 0,
	episode_title TEXT NOT NULL,
	recommendation TEXT,
	shownotes TEXT,
	duration NUMERIC NOT NULL DEFAULT 0,
	published_at DATETIME,
	image_url TEXT,
	episode_url TEXT,
	pay_type TEXT NOT NULL DEFAULT '',
	is_private_media NUMERIC NOT NULL DEFAULT false,
	audio_url TEXT,
	audio_mime_type TEXT,
	audio_size NUMERIC NOT NULL DEFAULT 0,
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

// EpisodeExternalRefsCreateSQL 已收录单集的外部身份映射：一个外部身份只能
// 对应一个本地单集；队列、备注和产物仍只引用本地 episode_id。
const EpisodeExternalRefsCreateSQL = `CREATE TABLE IF NOT EXISTS episode_external_refs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	source_platform TEXT NOT NULL,
	external_episode_id TEXT NOT NULL,
	external_podcast_id TEXT NOT NULL DEFAULT '',
	episode_id INTEGER NOT NULL,
	podcast_id INTEGER NOT NULL,
	CONSTRAINT fk_episode_external_refs_episode FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE,
	CONSTRAINT fk_episode_external_refs_podcast FOREIGN KEY (podcast_id) REFERENCES podcasts(id)
)`

// EpisodeExternalRefsUniqueIndexSQL 外部身份唯一映射。
const EpisodeExternalRefsUniqueIndexSQL = `CREATE UNIQUE INDEX IF NOT EXISTS idx_episode_external_refs_identity ON episode_external_refs(source_platform, external_episode_id)`

// EpisodeExternalRefsEpisodeIndexSQL 按本地单集反查外部身份。
const EpisodeExternalRefsEpisodeIndexSQL = `CREATE INDEX IF NOT EXISTS idx_episode_external_refs_episode ON episode_external_refs(episode_id)`

// EpisodeExternalRefsDeletedAtIndexSQL 与 GORM 软删除索引保持一致。
const EpisodeExternalRefsDeletedAtIndexSQL = `CREATE INDEX IF NOT EXISTS idx_episode_external_refs_deleted_at ON episode_external_refs(deleted_at)`

// EpisodeCollectionAdoptionsCreateSQL 精简采纳来源摘要：每集每清单一条，
// 删除清单不删除该摘要；删除个人单集按既有删除规则级联清理。
const EpisodeCollectionAdoptionsCreateSQL = `CREATE TABLE IF NOT EXISTS episode_collection_adoptions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	episode_id INTEGER NOT NULL,
	source_platform TEXT NOT NULL,
	collection_external_id TEXT NOT NULL,
	collection_title TEXT NOT NULL,
	collection_url TEXT NOT NULL,
	external_episode_id TEXT NOT NULL DEFAULT '',
	adopted_at DATETIME NOT NULL,
	CONSTRAINT fk_episode_collection_adoptions_episode FOREIGN KEY (episode_id) REFERENCES episodes(id) ON DELETE CASCADE
)`

// EpisodeCollectionAdoptionsUniqueIndexSQL 每集每清单一条采纳来源。
const EpisodeCollectionAdoptionsUniqueIndexSQL = `CREATE UNIQUE INDEX IF NOT EXISTS idx_episode_collection_adoptions_episode_collection ON episode_collection_adoptions(episode_id, source_platform, collection_external_id)`

// EpisodeCollectionAdoptionsDeletedAtIndexSQL 与 GORM 软删除索引保持一致。
const EpisodeCollectionAdoptionsDeletedAtIndexSQL = `CREATE INDEX IF NOT EXISTS idx_episode_collection_adoptions_deleted_at ON episode_collection_adoptions(deleted_at)`

// TableName 指定表名
func (EpisodeExternalRef) TableName() string {
	return "episode_external_refs"
}

// TableName 指定表名
func (EpisodeCollectionAdoption) TableName() string {
	return "episode_collection_adoptions"
}

// TableName 指定表名
func (EpisodeCollectionItem) TableName() string {
	return "episode_collection_items"
}
