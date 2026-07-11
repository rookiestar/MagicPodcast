package models

import (
	"time"
)

// Episode 播客单集模型
type Episode struct {
	BaseModel

	// 关联关系
	PodcastID uint    `gorm:"not null;index" json:"podcast_id"`              // 所属节目 ID
	Podcast   Podcast `gorm:"foreignKey:PodcastID" json:"podcast,omitempty"` // 所属节目

	// 单集信息
	EpisodeNo     string    `gorm:"size:64" json:"episode_no"`      // 期号（支持"番外1"等字符串）
	Title         string    `gorm:"size:512;not null" json:"title"` // 单集标题
	MediumURL     string    `gorm:"size:512" json:"medium_url"`     // 音频文件 URL
	ShowNotes     string    `gorm:"type:text" json:"show_notes"`    // 节目详情/show notes
	PublishedDate time.Time `json:"published_date"`                 // 发布日期

	// RSS 与播放元数据
	Duration        int        `gorm:"default:0" json:"duration"`         // 音频时长（秒）
	Link            string     `gorm:"size:512" json:"link"`              // 单集网页链接
	Content         string     `gorm:"type:text" json:"content"`          // 完整内容（区别于description）
	ImageURL        string     `gorm:"size:512" json:"image_url"`         // 单集封面图URL
	EnclosureType   string     `gorm:"size:100" json:"enclosure_type"`    // 音频MIME类型
	EnclosureLength int64      `gorm:"default:0" json:"enclosure_length"` // 音频文件大小（字节）
	UpdatedDate     *time.Time `json:"updated_date"`                      // 更新时间（区别于发布时间）

	// 同步相关
	GUID      string     `gorm:"size:255;uniqueIndex" json:"guid"` // RSS item GUID，用于去重（唯一索引）
	FetchedAt *time.Time `json:"fetched_at"`                       // 抓取时间

	// 用户自定义
	MyRate int    `gorm:"default:0" json:"my_rate"` // 个人评分 (0-5)
	Notes  string `gorm:"type:text" json:"notes"`   // 个人备注

	// 关联关系
	Tags []Tag `gorm:"many2many:episodes_tags;constraint:OnDelete:CASCADE" json:"tags,omitempty"`
}

// TableName 指定表名
func (Episode) TableName() string {
	return "episodes"
}
