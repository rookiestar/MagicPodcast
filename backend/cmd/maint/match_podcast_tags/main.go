package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/xuri/excelize/v2"
)

type PodcastTag struct {
	PodcastID   uint
	PodcastName string
	TagName     string
	TagID       uint
}

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

	if len(rows) == 0 {
		log.Fatal("Sheet2 没有数据")
	}

	fmt.Println("🔍 开始匹配节目和分类标签...")
	fmt.Printf("📊 Excel总数: %d 个节目\n", len(rows)-1)

	// 显示前3行示例
	fmt.Println("\n📄 Excel示例数据:")
	for i := 1; i <= 3 && i < len(rows); i++ {
		if len(rows[i]) >= 2 {
			fmt.Printf("  [%d] %s -> %s\n", i, rows[i][0], rows[i][1])
		}
	}

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
	fmt.Printf("\n🏷️  数据库标签数: %d\n", len(tagMap))

	// 匹配结果
	var matched []PodcastTag
	var unmatched []string
	var tagNotFound []string

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
			unmatched = append(unmatched, podcastName)
			continue
		}
		if err != nil {
			log.Printf("查询节目失败: %s, 错误: %v\n", podcastName, err)
			continue
		}

		// 查找分类标签
		tagID, exists := tagMap[categoryName]
		if !exists {
			tagNotFound = append(tagNotFound, categoryName)
			continue
		}

		// 记录匹配结果
		matched = append(matched, PodcastTag{
			PodcastID:   podcastID,
			PodcastName: podcastName,
			TagName:     categoryName,
			TagID:       tagID,
		})
	}

	// 生成报告
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("📋 匹配报告")
	fmt.Println(strings.Repeat("=", 70))

	fmt.Printf("\n✅ 成功匹配: %d 个节目\n", len(matched))
	fmt.Printf("❌ 数据库中未找到的节目: %d 个\n", len(unmatched))
	fmt.Printf("⚠️  标签表中未找到的分类: %d 个\n", len(tagNotFound))

	// 保存详细报告到文件
	reportFile, err := os.Create("match_report.txt")
	if err != nil {
		log.Fatal("创建报告文件失败:", err)
	}
	defer reportFile.Close()

	fmt.Fprintln(reportFile, "节目分类标签匹配报告")
	fmt.Fprintln(reportFile, strings.Repeat("=", 70))
	fmt.Fprintf(reportFile, "\n生成时间: %s\n", "2026-01-17")
	fmt.Fprintf(reportFile, "Excel文件: 热门节目+热门播客.xlsx (Sheet2)\n")
	fmt.Fprintf(reportFile, "数据库: magicpodcast.db\n")

	fmt.Fprintln(reportFile, strings.Repeat("=", 70))
	fmt.Fprintf(reportFile, "\n📊 统计摘要:\n\n")
	fmt.Fprintf(reportFile, "  ✅ 成功匹配: %d 个节目\n", len(matched))
	fmt.Fprintf(reportFile, "  ❌ 数据库中未找到的节目: %d 个\n", len(unmatched))
	fmt.Fprintf(reportFile, "  ⚠️  标签表中未找到的分类: %d 个\n\n", len(tagNotFound))

	if len(matched) > 0 {
		fmt.Fprintln(reportFile, strings.Repeat("=", 70))
		fmt.Fprint(reportFile, "\n✅ 成功匹配的节目详情:\n")
		fmt.Fprintln(reportFile, strings.Repeat("-", 70))
		fmt.Fprintf(reportFile, "%-8s %-40s %-20s\n", "节目ID", "节目名称", "分类标签")
		fmt.Fprintln(reportFile, strings.Repeat("-", 70))

		for _, item := range matched {
			fmt.Fprintf(reportFile, "%-8d %-40s %-20s\n", item.PodcastID, truncate(item.PodcastName, 40), item.TagName)
		}
	}

	if len(unmatched) > 0 {
		fmt.Fprintln(reportFile, strings.Repeat("=", 70))
		fmt.Fprintf(reportFile, "\n❌ 数据库中未找到的节目 (%d个):\n\n", len(unmatched))
		fmt.Fprintln(reportFile, strings.Repeat("-", 70))
		for i, name := range unmatched {
			fmt.Fprintf(reportFile, "%3d. %s\n", i+1, name)
			if i >= 99 { // 只显示前100个
				fmt.Fprintf(reportFile, "\n... (共%d个)\n", len(unmatched))
				break
			}
		}
	}

	if len(tagNotFound) > 0 {
		fmt.Fprintln(reportFile, strings.Repeat("=", 70))
		fmt.Fprintf(reportFile, "\n⚠️  标签表中未找到的分类 (%d个):\n\n", len(tagNotFound))
		fmt.Fprintln(reportFile, strings.Repeat("-", 70))
		for i, name := range tagNotFound {
			fmt.Fprintf(reportFile, "%3d. %s\n", i+1, name)
		}
	}

	fmt.Fprintln(reportFile, "\n"+strings.Repeat("=", 70))
	fmt.Fprintln(reportFile, "\n⚠️  重要提示:")
	fmt.Fprintln(reportFile, "  1. 此报告仅供审核，尚未执行数据库插入操作")
	fmt.Fprintln(reportFile, "  2. 请检查上述匹配结果是否正确")
	fmt.Fprintln(reportFile, "  3. 确认无误后，运行以下命令执行插入:")
	fmt.Fprintln(reportFile, "     go run scripts/apply_podcast_tags.go")
	fmt.Fprintln(reportFile, "  4. 或者直接手动执行SQL:")
	fmt.Fprintln(reportFile, "     sqlite3 backend/data/magicpodcast.db < apply_tags.sql")

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("✅ 报告已生成到: match_report.txt")
	fmt.Println("📝 请查看报告并审核匹配结果")
	fmt.Println(strings.Repeat("=", 70))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
