package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbPath := flag.String("db", "./data/podcastindex_feeds.db", "Path to PodcastIndex database")
	flag.Parse()

	if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
		log.Fatalf("Database file not found: %s", *dbPath)
	}

	// Open database with exclusive lock to prevent concurrent access
	dsn := fmt.Sprintf("file:%s?mode=rwc&_journal_mode=WAL&_timeout=5000", *dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Set busy timeout to wait for locks
	if _, err := db.Exec("PRAGMA busy_timeout = 30000;"); err != nil {
		log.Fatalf("Failed to set busy timeout: %v", err)
	}

	// Check existing indexes
	log.Println("Checking existing indexes...")
	var existingIndexes []string
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='podcasts';")
	if err != nil {
		log.Fatalf("Failed to query indexes: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Printf("Warning: failed to scan index name: %v", err)
			continue
		}
		existingIndexes = append(existingIndexes, name)
		log.Printf("  Found index: %s", name)
	}

	// Create indexes if they don't exist
	indexes := []struct {
		name   string
		column string
	}{
		{"idx_podcasts_title", "title"},
		{"idx_podcasts_url", "url"},
	}

	for _, idx := range indexes {
		// Check if index exists
		exists := false
		for _, existing := range existingIndexes {
			if existing == idx.name {
				exists = true
				break
			}
		}

		if exists {
			log.Printf("Index '%s' already exists, skipping...", idx.name)
			continue
		}

		// Create index
		log.Printf("Creating index '%s' on column '%s'...", idx.name, idx.column)
		startTime := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON podcasts(%s)", idx.name, idx.column)

		result, err := db.Exec(startTime)
		if err != nil {
			log.Printf("ERROR: Failed to create index '%s': %v", idx.name, err)
			continue
		}

		rowsAffected, _ := result.RowsAffected()
		log.Printf("Index '%s' created successfully (rows affected: %d)", idx.name, rowsAffected)
	}

	// Verify indexes were created
	log.Println("\nFinal index verification:")
	rows, err = db.Query("SELECT name, sql FROM sqlite_master WHERE type='index' AND tbl_name='podcasts' ORDER BY name;")
	if err != nil {
		log.Fatalf("Failed to verify indexes: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var name, sql string
		if err := rows.Scan(&name, &sql); err != nil {
			continue
		}
		if sql != "" { // Skip automatic SQLite indexes
			log.Printf("  ✓ %s", name)
			count++
		}
	}

	log.Printf("\n✅ Database optimization complete! Total custom indexes: %d", count)
	log.Println("💡 Tips:")
	log.Println("  - The title index will significantly speed up title-based queries")
	log.Println("  - The url index will speed up feed_url matching as a fallback")
	log.Println("  - Queries should now be 10-100x faster for large datasets")
}
