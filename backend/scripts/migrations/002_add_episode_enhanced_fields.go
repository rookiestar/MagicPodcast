package migrations

import (
	"magicpodcast/internal/models"

	"gorm.io/gorm"
)

// AddEpisodeEnhancedFields 添加episode增强字段（第一阶段）
func AddEpisodeEnhancedFields(db *gorm.DB) error {
	println("🔄 Adding enhanced fields to episodes table (Phase 1)...")

	// 第一阶段：添加核心字段
	migrations := []string{
		// 音频时长（秒）
		"ALTER TABLE episodes ADD COLUMN duration INTEGER DEFAULT 0",

		// 单集网页链接
		"ALTER TABLE episodes ADD COLUMN link VARCHAR(512)",

		// 音频MIME类型（如audio/mpeg）
		"ALTER TABLE episodes ADD COLUMN enclosure_type VARCHAR(100)",

		// 完整内容（区别于description）
		"ALTER TABLE episodes ADD COLUMN content TEXT",

		// 单集封面图URL
		"ALTER TABLE episodes ADD COLUMN image_url VARCHAR(512)",

		// 音频文件大小（字节）
		"ALTER TABLE episodes ADD COLUMN enclosure_length BIGINT",

		// 更新时间（区别于发布时间）
		"ALTER TABLE episodes ADD COLUMN updated_date DATETIME",
	}

	for _, sql := range migrations {
		if err := db.Exec(sql).Error; err != nil {
			// 如果字段已存在，忽略错误
			if !containsError(err.Error(), "duplicate column") {
				return err
			}
		}
	}

	// 创建索引以优化查询性能
	indexes := []string{
		// 发布日期降序索引（用于获取最新episode）
		"CREATE INDEX IF NOT EXISTS idx_episodes_published_date ON episodes(published_date DESC)",

		// 更新日期索引
		"CREATE INDEX IF NOT EXISTS idx_episodes_updated_date ON episodes(updated_date DESC)",
	}

	for _, sql := range indexes {
		if err := db.Exec(sql).Error; err != nil {
			return err
		}
	}

	println("✅ Enhanced fields added to episodes table successfully")
	return nil
}

// containsError 检查错误信息是否包含特定字符串
func containsError(errStr, target string) bool {
	return len(errStr) > 0 && (contains(errStr, target) || contains(errStr, "SQLITE_ERROR"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
