package models

import (
	"time"
)

// Podcast 播客节目模型
type Podcast struct {
	BaseModel

	// 小宇宙相关
	XYZID       string `gorm:"uniqueIndex;size:64" json:"xyz_id"`        // 小宇宙节目 ID
	Title       string `gorm:"size:255;not null" json:"title"`            // 节目标题
	FeedURL     string `gorm:"size:512" json:"feed_url"`                  // RSS 订阅源 URL
	ITunesID    string `gorm:"size:64" json:"itunes_id"`                  // iTunes ID
	PodcastGUID string `gorm:"size:128" json:"podcast_guid"`              // Podcast GUID
	Description string `gorm:"type:text" json:"description"`              // 节目描述
	Author      string `gorm:"size:255" json:"author"`                    // 作者/主播
	CoverURL    string `gorm:"size:512" json:"cover_url"`                 // 封面图片 URL

	// 统计信息
	AddedDate        time.Time `json:"added_date"`            // 添加日期
	EpisodeCount     int       `gorm:"default:0" json:"episode_count"`     // 单集总数
	NewestEpisodeDate time.Time `json:"newest_episode_date"`  // 最新单集发布日期

	// 状态标识
	IsSubscribed bool `gorm:"default:true" json:"is_subscribed"` // 是否已订阅
	IsDead       bool `gorm:"default:false" json:"is_dead"`      // RSS 源是否失效

	// 用户自定义
	MyRate int    `gorm:"default:0" json:"my_rate"` // 个人评分 (0-5)
	Notes  string `gorm:"type:text" json:"notes"`   // 个人备注

	// 同步相关
	FeedURLValid    bool       `gorm:"default:true" json:"feed_url_valid"`     // RSS feed是否有效
	LastFetchedAt   *time.Time `json:"last_fetched_at"`                        // 最后抓取时间
	FetchErrorCount int        `gorm:"default:0" json:"fetch_error_count"`     // 抓取失败次数
	DataSource      string     `gorm:"size:20;default:'rss'" json:"data_source"` // 数据来源: podcastindex, rss, scraped

	// 关联关系
	Episodes []Episode `gorm:"foreignKey:PodcastID;constraint:OnDelete:CASCADE" json:"episodes,omitempty"`
	Tags     []Tag     `gorm:"many2many:podcasts_tags;constraint:OnDelete:CASCADE" json:"tags,omitempty"`
}

// TableName 指定表名
func (Podcast) TableName() string {
	return "podcasts"
}
