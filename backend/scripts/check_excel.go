package main

import (
	"fmt"
	"log"
	"sort"

	"github.com/xuri/excelize/v2"
)

func main() {
	filePath := "/Users/rookiestar/Downloads/热门节目+热门播客.xlsx"

	f, err := excelize.OpenFile(filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// 读取Sheet2
	rows, err := f.GetRows("Sheet2")
	if err != nil {
		log.Fatal(err)
	}

	if len(rows) == 0 {
		log.Fatal("Sheet2 没有数据")
	}

	fmt.Printf("📊 Excel总行数: %d\n\n", len(rows))

	// 显示表头
	fmt.Println("表头:")
	for i, col := range rows[0] {
		fmt.Printf("  [%d] %s\n", i, col)
	}

	// 查找分类列
	categoryIndex := -1
	for i, col := range rows[0] {
		if col == "分类" {
			categoryIndex = i
			break
		}
	}

	if categoryIndex == -1 {
		log.Fatal("未找到'分类'列")
	}

	// 提取所有分类并统计
	categoryMap := make(map[string]int)
	techPodcasts := []string{}

	for i := 1; i < len(rows); i++ {
		if len(rows[i]) <= categoryIndex {
			continue
		}

		podcastName := rows[i][0]
		category := rows[i][categoryIndex]

		if category != "" && category != "-" {
			categoryMap[category]++
			if category == "科技" || category == "科技新闻" {
				techPodcasts = append(techPodcasts, podcastName)
			}
		}
	}

	// 排序
	categories := make([]string, 0, len(categoryMap))
	for cat := range categoryMap {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	fmt.Printf("\n📋 所有分类（共%d个）:\n", len(categories))
	for _, cat := range categories {
		fmt.Printf("  %s: %d 个节目\n", cat, categoryMap[cat])
	}

	fmt.Printf("\n🔬 科技相关节目（共%d个）:\n", len(techPodcasts))
	for i, pod := range techPodcasts {
		if i >= 10 {
			fmt.Printf("  ... (共%d个)\n", len(techPodcasts))
			break
		}
		fmt.Printf("  %d. %s\n", i+1, pod)
	}
}
