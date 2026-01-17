package migrations

import (
	"fmt"
	"log"

	"gorm.io/gorm"
)

// RemoveTagDescription 从tags表移除description字段
func RemoveTagDescription(db *gorm.DB) error {
	println("🔄 Removing description field from tags table...")

	// 由于SQLite对ALTER TABLE的限制，需要重建表
	log.Println("   📦 Recreating tags table without description column...")

	// 步骤1: 创建新表（不包含description）
	createTableSQL := `
		CREATE TABLE tags_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			name TEXT(64) NOT NULL UNIQUE,
			color TEXT(7)
		);
	`
	if err := db.Exec(createTableSQL).Error; err != nil {
		return fmt.Errorf("failed to create new table: %w", err)
	}
	log.Println("   ✅ Created tags_new table")

	// 步骤2: 复制数据（排除description列）
	copyDataSQL := `
		INSERT INTO tags_new (id, created_at, updated_at, deleted_at, name, color)
		SELECT id, created_at, updated_at, deleted_at, name, color
		FROM tags;
	`
	if err := db.Exec(copyDataSQL).Error; err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	// 获取复制的行数
	var count int64
	db.Table("tags_new").Count(&count)
	log.Printf("   ✅ Copied %d tags to new table", count)

	// 步骤3: 删除旧表
	if err := db.Exec("DROP TABLE tags").Error; err != nil {
		return fmt.Errorf("failed to drop old table: %w", err)
	}
	log.Println("   ✅ Dropped old tags table")

	// 步骤4: 重命名新表
	if err := db.Exec("ALTER TABLE tags_new RENAME TO tags").Error; err != nil {
		return fmt.Errorf("failed to rename table: %w", err)
	}
	log.Println("   ✅ Renamed tags_new to tags")

	// 步骤5: 重建索引
	log.Println("   🔨 Recreating indexes...")

	// name字段已在CREATE TABLE中声明为UNIQUE，无需额外创建索引
	// 只需要创建deleted_at索引
	if err := db.Exec("CREATE INDEX idx_tags_deleted_at ON tags(deleted_at)").Error; err != nil {
		return fmt.Errorf("failed to create deleted_at index: %w", err)
	}
	log.Println("   ✅ Created indexes")

	println("✅ Successfully removed description field from tags table")
	return nil
}
