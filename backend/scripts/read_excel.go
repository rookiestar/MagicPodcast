package main

import (
	"fmt"
	"log"
	"sort"
	"strings"

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

	// 查找分类列的索引
	header := rows[0]
	categoryIndex := -1
	for i, col := range header {
		if col == "分类" {
			categoryIndex = i
			break
		}
	}

	if categoryIndex == -1 {
		log.Fatal("未找到'分类'列")
	}

	// 提取所有分类并去重
	categoriesMap := make(map[string]bool)
	for i := 1; i < len(rows); i++ {
		if len(rows[i]) > categoryIndex {
			category := rows[i][categoryIndex]
			if category != "" {
				categoriesMap[category] = true
			}
		}
	}

	// 排序
	categories := make([]string, 0, len(categoriesMap))
	for cat := range categoriesMap {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	// 生成SQL语句
	fmt.Println("-- 从热门节目+热门播客.xlsx的Sheet2导入分类数据")
	fmt.Printf("-- 总计 %d 个分类\n", len(categories))
	fmt.Println()
	for _, cat := range categories {
		// 转义单引号
		escapedCat := strings.ReplaceAll(cat, "'", "''")
		fmt.Printf("INSERT INTO tags (name) VALUES ('%s');\n", escapedCat)
	}
}
