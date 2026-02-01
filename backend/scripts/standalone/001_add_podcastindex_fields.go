package main

import (
	"log"
	"time"

	"magicpodcast/internal/config"
	"magicpodcast/internal/database"
)

func main() {
	log.Println("🚀 开始数据库迁移：添加 PodcastIndex 字段")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 加载配置
	if _, err := config.Load("configs/config.yaml"); err != nil {
		log.Fatalf("❌ 加载配置失败: %v", err)
	}
	log.Println("✅ 配置加载成功")

	// 获取数据库连接
	db := database.GetDB()
	log.Println("✅ 数据库连接成功")

	// 获取当前时间
	startTime := time.Now()

	// 开始事务
	log.Println("\n📝 开始事务...")
	tx := db.Begin()
	if tx.Error != nil {
		log.Fatalf("❌ 启动事务失败: %v", tx.Error)
	}

	// 定义迁移 SQL
	migrations := []struct {
		Name string
		SQL  string
	}{
		{
			Name: "添加 link 字段（播客官网链接）",
			SQL:  "ALTER TABLE podcasts ADD COLUMN link TEXT",
		},
		{
			Name: "添加 newest_enclosure_url 字段（最新单集音频URL）",
			SQL:  "ALTER TABLE podcasts ADD COLUMN newest_enclosure_url TEXT",
		},
		{
			Name: "添加 newest_enclosure_duration 字段（最新单集时长）",
			SQL:  "ALTER TABLE podcasts ADD COLUMN newest_enclosure_duration INTEGER DEFAULT 0",
		},
		{
			Name: "添加 last_update 字段（Feed最后更新时间）",
			SQL:  "ALTER TABLE podcasts ADD COLUMN last_update DATETIME",
		},
		{
			Name: "添加 oldest_episode_date 字段（最旧单集发布日期）",
			SQL:  "ALTER TABLE podcasts ADD COLUMN oldest_episode_date DATETIME",
		},
		{
			Name: "添加 popularity_score 字段（受欢迎程度 0-10）",
			SQL:  "ALTER TABLE podcasts ADD COLUMN popularity_score INTEGER DEFAULT 0",
		},
		{
			Name: "添加 priority 字段（抓取优先级 -1到10）",
			SQL:  "ALTER TABLE podcasts ADD COLUMN priority INTEGER DEFAULT 5",
		},
		{
			Name: "添加 update_frequency 字段（更新频率 0-10）",
			SQL:  "ALTER TABLE podcasts ADD COLUMN update_frequency INTEGER DEFAULT 0",
		},
	}

	// 执行迁移
	successCount := 0
	for i, migration := range migrations {
		log.Printf("[%d/%d] %s", i+1, len(migrations), migration.Name)
		if err := tx.Exec(migration.SQL).Error; err != nil {
			tx.Rollback()
			log.Fatalf("❌ 迁移失败: %s\n   错误: %v", migration.Name, err)
		}
		log.Printf("   ✅ 成功\n")
		successCount++
	}

	// 提交事务
	log.Println("\n💾 提交事务...")
	if err := tx.Commit().Error; err != nil {
		log.Fatalf("❌ 提交事务失败: %v", err)
	}

	// 验证迁移
	log.Println("\n🔍 验证迁移结果...")
	var columnCount int
	db.Raw("SELECT COUNT(*) FROM pragma_table_info('podcasts') WHERE name IN ('link', 'newest_enclosure_url', 'newest_enclosure_duration', 'last_update', 'oldest_episode_date', 'popularity_score', 'priority', 'update_frequency')").Scan(&columnCount)

	if columnCount == 8 {
		log.Printf("✅ 验证成功：所有 8 个新字段已添加\n")
	} else {
		log.Printf("⚠️  警告：只找到 %d/8 个新字段\n", columnCount)
	}

	duration := time.Since(startTime)

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("🎉 迁移完成！")
	log.Printf("   成功: %d/%d", successCount, len(migrations))
	log.Printf("   耗时: %v", duration)
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}
