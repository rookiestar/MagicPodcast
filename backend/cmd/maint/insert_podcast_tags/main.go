package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/xuri/excelize/v2"
)

func main() {
	// 打开数据库
	db, err := sql.Open("sqlite3", "data/magicpodcast.db")
	if err != nil {
		log.Fatal("无法打开数据库:", err)
	}
	defer db.Close()

	// 读取Excel文件
	filePath := "/Users/rookiestar/Downloads/热门节目+热门播客.xlsx"
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		log.Fatal("无法打开Excel文件:", err)
	}
	defer f.Close()

	// 读取Sheet2（节目名称和分类）
	rows, err := f.GetRows("Sheet2")
	if err != nil {
		log.Fatal("无法读取Sheet2:", err)
	}

	if len(rows) <= 1 {
		log.Fatal("Sheet2 没有数据")
	}

	fmt.Println("🔍 开始从Excel读取并插入节目标签关联...")
	fmt.Printf("📊 Excel总数: %d 个节目\n", len(rows)-1)

	// 构建分类名到ID的映射
	tagMap := make(map[string]uint)
	tagRows, err := db.Query("SELECT id, name FROM tags")
	if err != nil {
		log.Fatal("查询tags失败:", err)
	}
	defer tagRows.Close()

	for tagRows.Next() {
		var id uint
		var name string
		if err := tagRows.Scan(&id, &name); err != nil {
			log.Fatal("扫描tag失败:", err)
		}
		tagMap[name] = id
	}
	fmt.Printf("\n🏷️  数据库标签数: %d\n\n", len(tagMap))

	// 统计
	successCount := 0
	skipCount := 0
	errorCount := 0
	notFoundCount := 0
	duplicateCount := 0

	// 从第2行开始（跳过表头）
	for i := 1; i < len(rows); i++ {
		if len(rows[i]) < 2 {
			continue
		}

		podcastName := rows[i][0]
		categoryName := rows[i][1]

		// 跳过空分类
		if categoryName == "" || categoryName == "-" {
			continue
		}

		// 查找数据库中的节目
		var podcastID uint
		err := db.QueryRow("SELECT id FROM podcasts WHERE title = ?", podcastName).Scan(&podcastID)

		if err == sql.ErrNoRows {
			// 未找到节目
			notFoundCount++
			if notFoundCount <= 10 {
				fmt.Printf("  [未找到] %s\n", truncateStr(podcastName, 40))
			}
			if notFoundCount == 10 {
				fmt.Printf("  ... (共%d个)\n", notFoundCount)
			}
			continue
		}
		if err != nil {
			log.Printf("查询节目失败: %s, 错误: %v\n", podcastName, err)
			errorCount++
			continue
		}

		// 查找分类标签
		tagID, exists := tagMap[categoryName]
		if !exists {
			fmt.Printf("  [警告] 未找到标签: %s (节目: %s)\n", categoryName, podcastName)
			errorCount++
			continue
		}

		// 检查是否已存在
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM podcasts_tags WHERE podcast_id = ? AND tag_id = ?",
			podcastID, tagID).Scan(&count)

		if err != nil {
			log.Printf("查询失败 [%d] %s: %v\n", podcastID, podcastName, err)
			errorCount++
			continue
		}

		if count > 0 {
			// 已存在，跳过
			duplicateCount++
			if duplicateCount <= 5 {
				fmt.Printf("  [已存在] [%d] %s -> %s\n", podcastID, truncateStr(podcastName, 40), categoryName)
			}
			if duplicateCount == 5 {
				fmt.Printf("  ... (共%d个)\n", duplicateCount)
			}
			skipCount++
			continue
		}

		// 插入关联
		_, err = db.Exec("INSERT INTO podcasts_tags (podcast_id, tag_id) VALUES (?, ?)",
			podcastID, tagID)

		if err != nil {
			log.Printf("插入失败 [%d] %s -> %s: %v\n", podcastID, podcastName, categoryName, err)
			errorCount++
			continue
		}

		successCount++
		if successCount <= 20 || successCount%50 == 0 {
			fmt.Printf("  [%d] %s -> %s ✓\n", podcastID, truncateStr(podcastName, 40), categoryName)
		}
		if successCount == 20 {
			fmt.Printf("  ... (共%d个)\n", successCount)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("✅ 插入完成")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("  成功插入: %d 条\n", successCount)
	fmt.Printf("  跳过重复: %d 条\n", skipCount)
	fmt.Printf("  数据库未找到: %d 个\n", notFoundCount)
	fmt.Printf("  标签未找到: %d 个\n", errorCount)
	fmt.Printf("  失败: %d 条\n", errorCount)

	// 验证插入结果
	var totalTags int
	db.QueryRow("SELECT COUNT(*) FROM podcasts_tags").Scan(&totalTags)
	fmt.Printf("\n📊 pods_tags表总数: %d\n", totalTags)
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
