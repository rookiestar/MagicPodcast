package main

import (
	"fmt"
	"log"
	"sort"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
	"magicpodcast/internal/models"

	"github.com/xuri/excelize/v2"
)

func main() {
	fmt.Println("🏷️  开始导入标签数据...")

	// 0. 加载配置
	if _, err := config.Load("configs/config.yaml"); err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}

	// 1. 连接数据库
	db := database.GetDB()

	// 2. 读取Excel文件
	excelPath := "/Users/rookiestar/Downloads/热门节目+热门播客.xlsx"
	f, err := excelize.OpenFile(excelPath)
	if err != nil {
		log.Fatalf("❌ 无法打开Excel文件: %v", err)
	}
	defer f.Close()

	// 3. 读取Sheet2
	rows, err := f.GetRows("Sheet2")
	if err != nil {
		log.Fatalf("❌ 无法读取Sheet2: %v", err)
	}

	if len(rows) == 0 {
		log.Fatal("❌ Sheet2 没有数据")
	}

	// 4. 查找分类列的索引
	header := rows[0]
	categoryIndex := -1
	for i, col := range header {
		if col == "分类" {
			categoryIndex = i
			break
		}
	}

	if categoryIndex == -1 {
		log.Fatal("❌ 未找到'分类'列")
	}

	// 5. 提取所有分类并去重
	categoriesMap := make(map[string]bool)
	for i := 1; i < len(rows); i++ {
		if len(rows[i]) > categoryIndex {
			category := rows[i][categoryIndex]
			if category != "" && category != "-" {
				categoriesMap[category] = true
			}
		}
	}

	// 6. 排序
	categories := make([]string, 0, len(categoriesMap))
	for cat := range categoriesMap {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	fmt.Printf("📊 从Excel中提取到 %d 个分类\n", len(categories))

	// 7. 开始事务
	tx := db.Begin()
	if tx.Error != nil {
		log.Fatalf("❌ 开始事务失败: %v", tx.Error)
	}

	// 8. 清空现有数据
	fmt.Println("🗑️  清空现有标签数据...")
	if err := tx.Exec("DELETE FROM podcasts_tags").Error; err != nil {
		tx.Rollback()
		log.Fatalf("❌ 清空podcasts_tags失败: %v", err)
	}
	if err := tx.Exec("DELETE FROM tags").Error; err != nil {
		tx.Rollback()
		log.Fatalf("❌ 清空tags失败: %v", err)
	}
	if err := tx.Exec("DELETE FROM sqlite_sequence WHERE name='tags'").Error; err != nil {
		tx.Rollback()
		log.Fatalf("❌ 重置ID序列失败: %v", err)
	}

	// 9. 批量插入标签
	fmt.Println("📥 批量插入标签...")
	successCount := 0
	skipCount := 0

	for _, categoryName := range categories {
		// 检查是否已存在（理论上不应该存在，因为已经清空了）
		var existingTag models.Tag
		err := tx.Where("name = ?", categoryName).First(&existingTag).Error

		if err == nil {
			// 已存在，跳过
			skipCount++
			fmt.Printf("  ⚠️  跳过已存在的标签: %s\n", categoryName)
			continue
		}

		// 使用GORM创建标签，自动填充时间戳
		tag := models.Tag{
			Name:  categoryName,
			Color: "", // 可以在这里设置默认颜色
		}

		if err := tx.Create(&tag).Error; err != nil {
			tx.Rollback()
			log.Fatalf("❌ 插入标签失败 [%s]: %v", categoryName, err)
		}

		successCount++
		if successCount <= 10 || successCount%20 == 0 {
			fmt.Printf("  ✅ [%d] %s\n", tag.ID, categoryName)
		}
		if successCount == 10 {
			fmt.Println("  ...")
		}
	}

	// 10. 提交事务
	if err := tx.Commit().Error; err != nil {
		log.Fatalf("❌ 提交事务失败: %v", err)
	}

	// 11. 验证结果
	var finalCount int64
	db.Table("tags").Count(&finalCount)

	fmt.Println("\n" + "========================================")
	fmt.Println("✅ 标签导入完成！")
	fmt.Println("========================================")
	fmt.Printf("  成功插入: %d 个\n", successCount)
	fmt.Printf("  跳过: %d 个\n", skipCount)
	fmt.Printf("  数据库总数: %d 个\n", finalCount)
	fmt.Println("========================================")

	// 12. 显示前几个标签示例
	fmt.Println("\n📋 标签示例（前5个）:")
	var sampleTags []models.Tag
	db.Order("id ASC").Limit(5).Find(&sampleTags)
	for _, tag := range sampleTags {
		fmt.Printf("  [%d] %s (创建于: %s)\n", tag.ID, tag.Name, tag.CreatedAt.Format("2006-01-02 15:04:05"))
	}
}
