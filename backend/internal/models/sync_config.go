package models

import "time"

// SyncConfig 同步配置模型
type SyncConfig struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	ConfigKey   string    `gorm:"size:100;uniqueIndex;not null" json:"config_key"` // 配置键
	ConfigValue string    `gorm:"type:text;not null" json:"config_value"`          // 配置值
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (SyncConfig) TableName() string {
	return "sync_configs"
}
