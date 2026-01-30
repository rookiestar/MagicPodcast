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

	// 读取SQL脚本
	sqlFile, err := os.Open("scripts/migrations/add_performance_indexes.sql")
	if err != nil {
		log.Fatalf("打开SQL文件失败: %v", err)
	}
	defer sqlFile.Close()

	// 读取SQL内容
	content, err := io.ReadAll(sqlFile)
	if err != nil {
		log.Fatalf("读取SQL文件失败: %v", err)
	}

	sqlScript := string(content)

	// 执行SQL
	fmt.Println("开始创建索引...")
	_, err = db.Exec(sqlScript)
	if err != nil {
		log.Fatalf("执行SQL失败: %v", err)
	}

	fmt.Println("✅ 索引创建完成！")
	fmt.Println()

	// 显示索引统计
	showIndexStats(db)
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
