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

type PodcastCategoryRelation struct {
	PodcastName string
	CategoryName string
	PodcastID uint
	Matched bool
}

func main() {
	fmt.Println("🏷️  开始导入标签及关联关系...")

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

	fmt.Printf("📊 从Excel读取到 %d 个节目-分类关系\n", len(rows)-1)

	// 5. 提取所有节目-分类关系
	var relations []PodcastCategoryRelation
	categoryMap := make(map[string]bool)

	for i := 1; i < len(rows); i++ {
		if len(rows[i]) <= categoryIndex {
			continue
		}

		podcastName := rows[i][0]
		categoryName := rows[i][categoryIndex]

		if categoryName == "" || categoryName == "-" {
			continue
		}

		relations = append(relations, PodcastCategoryRelation{
			PodcastName: podcastName,
			CategoryName: categoryName,
			Matched: false,
		})
		categoryMap[categoryName] = true
	}

	fmt.Printf("📋 提取到 %d 个唯一分类标签\n", len(categoryMap))

	// 6. 查询数据库中的所有播客
	var podcasts []models.Podcast
	db.Select("id, title").Find(&podcasts)
	fmt.Printf("📊 数据库中有 %d 个播客\n", len(podcasts))

	// 7. 构建播客名称到ID的映射
	podcastMap := make(map[string]uint)
	for _, p := range podcasts {
		podcastMap[p.Title] = p.ID
	}

	// 8. 第一阶段：匹配节目
	fmt.Println("\n🔍 第一阶段：匹配节目...")
	matchedCount := 0
	var unmatchedPodcasts []string

	for i := range relations {
		if podcastID, exists := podcastMap[relations[i].PodcastName]; exists {
			relations[i].PodcastID = podcastID
			relations[i].Matched = true
			matchedCount++
		} else {
			unmatchedPodcasts = append(unmatchedPodcasts, relations[i].PodcastName)
		}
	}

	fmt.Printf("  ✅ 成功匹配: %d 个节目\n", matchedCount)
	fmt.Printf("  ❌ 未匹配: %d 个节目\n", len(unmatchedPodcasts))

	// 9. 第二阶段：提取有效标签
	fmt.Println("\n📋 第二阶段：提取有效标签...")
	validTagsMap := make(map[string][]uint) // categoryName -> []podcastID

	for _, rel := range relations {
		if rel.Matched {
			validTagsMap[rel.CategoryName] = append(validTagsMap[rel.CategoryName], rel.PodcastID)
		}
	}

	// 转换为切片并排序
	var validCategories []string
	for cat := range validTagsMap {
		validCategories = append(validCategories, cat)
	}
	sort.Strings(validCategories)

	fmt.Printf("  ✅ 有效标签: %d 个\n", len(validCategories))

	// 显示Top 10标签
	fmt.Println("\n📊 标签预览（按节目数量排序，Top 10）:")
	type TagCount struct {
		Name string
		Count int
	}
	var tagCounts []TagCount
	for _, cat := range validCategories {
		tagCounts = append(tagCounts, TagCount{
			Name: cat,
			Count: len(validTagsMap[cat]),
		})
	}
	sort.Slice(tagCounts, func(i, j int) bool {
		return tagCounts[i].Count > tagCounts[j].Count
	})
	for i, tc := range tagCounts {
		if i >= 10 {
			break
		}
		fmt.Printf("  %d. %s: %d 个节目\n", i+1, tc.Name, tc.Count)
	}

	// 10. 第三阶段：开始事务并清空数据
	fmt.Println("\n🗑️  第三阶段：清空现有数据...")
	tx := db.Begin()
	if tx.Error != nil {
		log.Fatalf("❌ 开始事务失败: %v", tx.Error)
	}

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
	fmt.Println("  ✅ 已清空 tags 和 podcasts_tags")

	// 11. 第四阶段：批量插入标签
	fmt.Println("\n📥 第四阶段：批量插入有效标签...")
	tagIDMap := make(map[string]uint) // categoryName -> tagID

	for _, categoryName := range validCategories {
		tag := models.Tag{
			Name:  categoryName,
			Color: "",
		}

		if err := tx.Create(&tag).Error; err != nil {
			tx.Rollback()
			log.Fatalf("❌ 插入标签失败 [%s]: %v", categoryName, err)
		}

		tagIDMap[categoryName] = tag.ID
	}
	fmt.Printf("  ✅ 成功插入 %d 个标签\n", len(validCategories))

	// 12. 第五阶段：建立节目-标签关联
	fmt.Println("\n🔗 第五阶段：建立节目-标签关联...")
	relationCount := 0
	duplicateCount := 0

	// 去重：同一个节目-标签组合只插入一次
	relationSet := make(map[string]bool) // "podcastID:tagID" -> true

	for _, rel := range relations {
		if !rel.Matched {
			continue
		}

		tagID, exists := tagIDMap[rel.CategoryName]
		if !exists {
			continue
		}

		relationKey := fmt.Sprintf("%d:%d", rel.PodcastID, tagID)
		if relationSet[relationKey] {
			duplicateCount++
			continue
		}

		// 直接使用SQL插入，提高性能
		if err := tx.Exec("INSERT INTO podcasts_tags (podcast_id, tag_id) VALUES (?, ?)",
			rel.PodcastID, tagID).Error; err != nil {
			tx.Rollback()
			log.Fatalf("❌ 插入关联失败 [%s - %s]: %v", rel.PodcastName, rel.CategoryName, err)
		}

		relationSet[relationKey] = true
		relationCount++
	}

	fmt.Printf("  ✅ 成功建立: %d 条关联\n", relationCount)
	if duplicateCount > 0 {
		fmt.Printf("  ⚠️  跳过重复: %d 条\n", duplicateCount)
	}

	// 13. 提交事务
	if err := tx.Commit().Error; err != nil {
		log.Fatalf("❌ 提交事务失败: %v", err)
	}

	// 14. 生成最终报告
	fmt.Println("\n" + "========================================")
	fmt.Println("✅ 导入完成！")
	fmt.Println("========================================")
	fmt.Printf("  标签总数: %d 个\n", len(validCategories))
	fmt.Printf("  关联总数: %d 条\n", relationCount)
	fmt.Printf("  匹配率: %.1f%% (%d/%d)\n",
		float64(matchedCount)/float64(len(podcasts))*100,
		matchedCount, len(podcasts))
	fmt.Printf("  未匹配播客: %d 个\n", len(unmatchedPodcasts))
	fmt.Println("========================================")

	// 15. 显示未匹配的节目示例
	if len(unmatchedPodcasts) > 0 {
		fmt.Println("\n❌ 未匹配节目示例（前20个）:")
		maxShow := 20
		if len(unmatchedPodcasts) < maxShow {
			maxShow = len(unmatchedPodcasts)
		}
		for i := 0; i < maxShow; i++ {
			fmt.Printf("  %d. %s\n", i+1, unmatchedPodcasts[i])
		}
		if len(unmatchedPodcasts) > maxShow {
			fmt.Printf("  ... (共%d个)\n", len(unmatchedPodcasts))
		}
	}

	// 16. 显示数据库验证
	var finalTagCount int64
	var finalRelationCount int64
	db.Table("tags").Count(&finalTagCount)
	db.Table("podcasts_tags").Count(&finalRelationCount)

	fmt.Println("\n📊 数据库验证:")
	fmt.Printf("  tags表: %d 条记录\n", finalTagCount)
	fmt.Printf("  podcasts_tags表: %d 条记录\n", finalRelationCount)

	// 17. 显示标签示例
	fmt.Println("\n📋 标签示例（前5个）:")
	var sampleTags []models.Tag
	db.Order("id ASC").Limit(5).Find(&sampleTags)
	for _, tag := range sampleTags {
		var count int64
		db.Table("podcasts_tags").Where("tag_id = ?", tag.ID).Count(&count)
		fmt.Printf("  [%d] %s (%d个节目)\n", tag.ID, tag.Name, count)
	}
}
