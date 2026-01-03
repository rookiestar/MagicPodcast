package models

import (
	"time"
)

// Episode 播客单集模型
type Episode struct {
	BaseModel

	// 小宇宙相关
	XYZID string `gorm:"uniqueIndex;size:64" json:"xyz_id"` // 小宇宙单集 ID

	// 关联关系
	PodcastID uint `gorm:"not null;index" json:"podcast_id"` // 所属节目 ID
	Podcast   Podcast `gorm:"foreignKey:PodcastID" json:"podcast,omitempty"` // 所属节目

	// 单集信息
	EpisodeNo     string `gorm:"size:64" json:"episode_no"`     // 期号（支持"番外1"等字符串）
	Title         string `gorm:"size:512;not null" json:"title"` // 单集标题
	MediumURL     string `gorm:"size:512" json:"medium_url"`     // 音频文件 URL
	ShowNotes     string `gorm:"type:text" json:"show_notes"`    // 节目详情/show notes
	PublishedDate time.Time `json:"published_date"`              // 发布日期

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
