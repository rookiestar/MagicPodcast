package models

import "time"

// Tag 标签模型
type Tag struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Name      string    `gorm:"size:64;uniqueIndex;not null" json:"name"` // 标签名称
	Color     string    `gorm:"size:7" json:"color"`                        // 标签颜色（十六进制，如 #FF5733）
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 关联关系
	Podcasts []Podcast `gorm:"many2many:podcasts_tags;constraint:OnDelete:CASCADE" json:"podcasts,omitempty"`
	Episodes []Episode `gorm:"many2many:episodes_tags;constraint:OnDelete:CASCADE" json:"episodes,omitempty"`
}

// TableName 指定表名
func (Tag) TableName() string {
	return "tags"
}
