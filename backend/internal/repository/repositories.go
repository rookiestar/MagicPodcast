package repository

import (
	"magicpodcast/internal/database"

	"gorm.io/gorm"
)

// Repositories Repository容器
type Repositories struct {
	Podcast  PodcastRepository
	Episode  EpisodeRepository
	Tag      TagRepository
	Workflow WorkflowRepository
	db       *gorm.DB
}

// NewRepositories 创建Repository容器
func NewRepositories() (*Repositories, error) {
	db := database.GetDB()

	return &Repositories{
		Podcast:  NewPodcastRepository(db),
		Episode:  NewEpisodeRepository(db),
		Tag:      NewTagRepository(db),
		Workflow: NewWorkflowRepository(db),
		db:       db,
	}, nil
}

// NewRepositoriesWithDB 使用指定数据库连接创建Repository容器
func NewRepositoriesWithDB(db *gorm.DB) *Repositories {
	return &Repositories{
		Podcast:  NewPodcastRepository(db),
		Episode:  NewEpisodeRepository(db),
		Tag:      NewTagRepository(db),
		Workflow: NewWorkflowRepository(db),
		db:       db,
	}
}

// Transaction 执行事务
func (r *Repositories) Transaction(fn func(*Repositories) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		txRepos := NewRepositoriesWithDB(tx)
		return fn(txRepos)
	})
}

// DB 获取数据库连接
func (r *Repositories) DB() *gorm.DB {
	return r.db
}
