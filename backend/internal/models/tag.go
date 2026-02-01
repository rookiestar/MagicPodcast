package models

import (
	"time"

	"gorm.io/gorm"
)

// Tag 标签模型
type Tag struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Name      string    `gorm:"size:64;uniqueIndex;not null" json:"name"` // 标签名称
	Color     string    `gorm:"size:7" json:"color"`                      // 标签颜色（十六进制，如 #FF5733）
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// 关联关系
	Podcasts []Podcast `gorm:"many2many:podcasts_tags;constraint:OnDelete:CASCADE" json:"podcasts,omitempty"`
	Episodes []Episode `gorm:"many2many:episodes_tags;constraint:OnDelete:CASCADE" json:"episodes,omitempty"`
}

// Don't implement soft delete interface
func (Tag) BeforeCreate(tx *gorm.DB) error {
	return nil
}

// TableName 指定表名
func (Tag) TableName() string {
	return "tags"
}
