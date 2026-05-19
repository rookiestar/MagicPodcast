package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// 数据库路径
	dbPath := "data/magicpodcast.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	// 打开数据库
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer db.Close()

	fmt.Println("📊 MagicPodcast 数据库索引优化")
	fmt.Println("==============================")
	fmt.Printf("数据库: %s\n\n", dbPath)

	sqlFiles := []string{
		"scripts/migrations/add_performance_indexes.sql",
		"scripts/migrations/add_search_fts.sql",
	}

	for _, path := range sqlFiles {
		if err := runSQLFile(db, path); err != nil {
			log.Fatalf("执行SQL失败 [%s]: %v", path, err)
		}
	}

	fmt.Println("✅ 索引创建完成！")
	fmt.Println()

	// 显示索引统计
	showIndexStats(db)
}

func runSQLFile(db *sql.DB, path string) error {
	sqlFile, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开SQL文件失败: %w", err)
	}
	defer sqlFile.Close()

	content, err := io.ReadAll(sqlFile)
	if err != nil {
		return fmt.Errorf("读取SQL文件失败: %w", err)
	}

	fmt.Printf("开始执行 %s...\n", path)
	if _, err := db.Exec(string(content)); err != nil {
		return err
	}
	return nil
}

func showIndexStats(db *sql.DB) {
	fmt.Println("📈 索引统计")
	fmt.Println("-----------------")

	// 统计每个表的索引数量
	rows, err := db.Query(`
		SELECT tbl_name, COUNT(*) as index_count
		FROM sqlite_master
		WHERE type = 'index'
		  AND tbl_name NOT LIKE 'sqlite_%'
		GROUP BY tbl_name
		ORDER BY tbl_name
	`)
	if err != nil {
		log.Printf("查询索引统计失败: %v", err)
		return
	}
	defer rows.Close()

	totalIndexes := 0
	for rows.Next() {
		var tableName string
		var count int
		if err := rows.Scan(&tableName, &count); err != nil {
			log.Printf("扫描行失败: %v", err)
			continue
		}
		fmt.Printf("  %s: %d 个索引\n", tableName, count)
		totalIndexes += count
	}

	fmt.Printf("\n总计: %d 个索引\n", totalIndexes)
}
