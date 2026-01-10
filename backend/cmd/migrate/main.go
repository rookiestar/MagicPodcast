package main

import (
	"fmt"
	"log"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
)

func main() {
	if err := config.Load(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db := database.GetDB()
	fmt.Println("Running manual migration to remove xyz_id...")

	// 重建表（排除xyz_id列）
	recreateTableSQL := `
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
			enclosure_length INTEGER DEFAULT 0,
			updated_date DATETIME,
			guid TEXT(255),
			fetched_at DATETIME,
			my_rate INTEGER DEFAULT 0,
			notes TEXT,
			FOREIGN KEY (podcast_id) REFERENCES podcasts(id) ON DELETE CASCADE
		);
	`
	if err := db.Exec(recreateTableSQL).Error; err != nil {
		log.Fatalf("Failed to create new table: %v", err)
	}

	copyDataSQL := `
		INSERT INTO episodes_new 
		SELECT id, created_at, updated_at, deleted_at, podcast_id, episode_no,
		       title, medium_url, show_notes, published_date, duration, link,
		       content, image_url, enclosure_type, enclosure_length, updated_date,
		       guid, fetched_at, my_rate, notes
		FROM episodes;
	`
	if err := db.Exec(copyDataSQL).Error; err != nil {
		log.Fatalf("Failed to copy data: %v", err)
	}

	if err := db.Exec("DROP TABLE episodes").Error; err != nil {
		log.Fatalf("Failed to drop old table: %v", err)
	}

	if err := db.Exec("ALTER TABLE episodes_new RENAME TO episodes").Error; err != nil {
		log.Fatalf("Failed to rename table: %v", err)
	}

	indexes := []string{
		"CREATE INDEX idx_episodes_podcast_id ON episodes(podcast_id)",
		"CREATE INDEX idx_episodes_deleted_at ON episodes(deleted_at)",
		"CREATE INDEX idx_episodes_published_date ON episodes(published_date DESC)",
		"CREATE INDEX idx_episodes_updated_date ON episodes(updated_date DESC)",
		"CREATE UNIQUE INDEX idx_episodes_guid ON episodes(guid)",
	}
	for _, idx := range indexes {
		if err := db.Exec(idx).Error; err != nil {
			log.Printf("Warning: Failed to create index: %v", err)
		}
	}

	fmt.Println("✅ Migration completed successfully!")
}
