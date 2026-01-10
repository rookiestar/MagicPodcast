package migrations

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// RemoveXYZIDAndUniqueGUID 删除xyz_id字段，将guid改为唯一约束
func RemoveXYZIDAndUniqueGUID(db *gorm.DB) error {
	println("🔄 Removing xyz_id field and making guid unique...")

	// 步骤1: 删除xyz_id的索引
	log.Println("   Dropping xyz_id index...")
	if err := db.Exec("DROP INDEX IF EXISTS idx_episodes_xyz_id").Error; err != nil {
		log.Printf("   ⚠️  Failed to drop idx_episodes_xyz_id (may not exist): %v", err)
		// 继续执行，不中断
	}

	// 步骤2: 确保所有episodes都有非空的guid
	log.Println("   Ensuring all episodes have non-empty guid...")

	// 用medium_url填充空的guid
	result := db.Exec("UPDATE episodes SET guid = medium_url WHERE guid IS NULL OR guid = ''")
	if result.Error != nil {
		return fmt.Errorf("failed to fill guid with medium_url: %w", result.Error)
	}
	log.Printf("   ✅ Updated %d rows with medium_url", result.RowsAffected)

	// 如果仍然有空的（极少数情况），用link填充
	result = db.Exec("UPDATE episodes SET guid = link WHERE guid IS NULL OR guid = '' OR guid = ''")
	if result.Error != nil {
		return fmt.Errorf("failed to fill guid with link: %w", result.Error)
	}
	log.Printf("   ✅ Updated %d rows with link", result.RowsAffected)

	// 步骤3: 检查并删除重复的guid（保留最早创建的）
	log.Println("   Removing duplicate guids...")

	// SQLite不支持直接的DELETE with JOIN，使用子查询
	deleteDupSQL := `
		DELETE FROM episodes
		WHERE id NOT IN (
			SELECT MIN(id) FROM episodes GROUP BY guid
		)
		AND guid IS NOT NULL
		AND guid != ''
	`
	result = db.Exec(deleteDupSQL)
	if result.Error != nil {
		return fmt.Errorf("failed to remove duplicate guids: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		log.Printf("   ✅ Removed %d duplicate episodes", result.RowsAffected)
	} else {
		log.Println("   ℹ️  No duplicates found")
	}

	// 步骤4: 删除旧的guid索引
	log.Println("   Dropping old guid index...")
	if err := db.Exec("DROP INDEX IF EXISTS idx_episodes_guid").Error; err != nil {
		log.Printf("   ⚠️  Failed to drop idx_episodes_guid: %v", err)
	}

	// 步骤5: 删除xyz_id列
	log.Println("   Dropping xyz_id column...")
	if err := db.Exec("ALTER TABLE episodes DROP COLUMN xyz_id").Error; err != nil {
		// SQLite的DROP COLUMN限制：如果失败，可能需要重建表
		log.Printf("   ⚠️  Failed to drop xyz_id column (SQLite limitation): %v", err)
		// 尝试重建表的方式
		if err := recreateEpisodesTableWithoutXYZID(db); err != nil {
			return fmt.Errorf("failed to recreate table: %w", err)
		}
	} else {
		log.Println("   ✅ Dropped xyz_id column successfully")
	}

	// 步骤6: 创建唯一的guid索引
	log.Println("   Creating unique guid index...")
	if err := db.Exec("CREATE UNIQUE INDEX idx_episodes_guid ON episodes(guid)").Error; err != nil {
		return fmt.Errorf("failed to create unique guid index: %w", err)
	}
	log.Println("   ✅ Created unique guid index")

	println("✅ Successfully removed xyz_id and made guid unique")
	return nil
}

// recreateEpisodesTableWithoutXYZID 通过重建表的方式删除xyz_id列
// SQLite的ALTER TABLE DROP COLUMN有限制，需要重建表
func recreateEpisodesTableWithoutXYZID(db *gorm.DB) error {
	log.Println("   📦 Recreating episodes table without xyz_id column...")

	// 步骤1: 创建新表（不包含xyz_id）
	createTableSQL := `
		CREATE TABLE episodes_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			podcast_id INTEGER NOT NULL,
			episode_no TEXT(64),
			title TEXT(512) NOT NULL,
			medium_url TEXT(512),
			show_notes TEXT,
			published_date DATETIME,
			duration INTEGER DEFAULT 0,
			link TEXT(512),
			content TEXT,
			image_url TEXT(512),
			enclosure_type TEXT(100),
			enclosure_length BIGINT DEFAULT 0,
			updated_date DATETIME,
			guid TEXT(255),
			fetched_at DATETIME,
			my_rate INTEGER DEFAULT 0,
			notes TEXT,
			FOREIGN KEY (podcast_id) REFERENCES podcasts(id) ON DELETE CASCADE
		);
	`
	if err := db.Exec(createTableSQL).Error; err != nil {
		return fmt.Errorf("failed to create new table: %w", err)
	}

	// 步骤2: 复制数据（排除xyz_id列）
	copyDataSQL := `
		INSERT INTO episodes_new (
			id, created_at, updated_at, deleted_at, podcast_id, episode_no,
			title, medium_url, show_notes, published_date, duration, link,
			content, image_url, enclosure_type, enclosure_length, updated_date,
			guid, fetched_at, my_rate, notes
		)
		SELECT
			id, created_at, updated_at, deleted_at, podcast_id, episode_no,
			title, medium_url, show_notes, published_date, duration, link,
			content, image_url, enclosure_type, enclosure_length, updated_date,
			guid, fetched_at, my_rate, notes
		FROM episodes;
	`
	if err := db.Exec(copyDataSQL).Error; err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	// 步骤3: 删除旧表
	if err := db.Exec("DROP TABLE episodes").Error; err != nil {
		return fmt.Errorf("failed to drop old table: %w", err)
	}

	// 步骤4: 重命名新表
	if err := db.Exec("ALTER TABLE episodes_new RENAME TO episodes").Error; err != nil {
		return fmt.Errorf("failed to rename table: %w", err)
	}

	// 步骤5: 重建索引
	indexes := []string{
		"CREATE INDEX idx_episodes_podcast_id ON episodes(podcast_id)",
		"CREATE INDEX idx_episodes_deleted_at ON episodes(deleted_at)",
		"CREATE INDEX idx_episodes_published_date ON episodes(published_date DESC)",
		"CREATE INDEX idx_episodes_updated_date ON episodes(updated_date DESC)",
		"CREATE UNIQUE INDEX idx_episodes_guid ON episodes(guid)",
	}

	for _, idxSQL := range indexes {
		if err := db.Exec(idxSQL).Error; err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	log.Println("   ✅ Successfully recreated episodes table")
	return nil
}
