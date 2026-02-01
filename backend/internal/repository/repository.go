package repository

import (
	"gorm.io/gorm"
)

// PaginationResult 分页结果
type PaginationResult struct {
	Page       int
	PageSize   int
	Total      int64
	TotalPages int
}

// Repository 基础Repository接口
type Repository interface {
	// DB 获取数据库连接
	DB() *gorm.DB

	// WithTx 使用事务
	WithTx(tx *gorm.DB) Repository

	// Begin 开启事务
	Begin() *gorm.DB

	// Transaction 执行事务
	Tx(fn func(tx *gorm.DB) error) error
}

// BaseRepository 基础Repository实现
type BaseRepository struct {
	db *gorm.DB
}

// NewBaseRepository 创建基础Repository
func NewBaseRepository(db *gorm.DB) *BaseRepository {
	return &BaseRepository{db: db}
}

// DB 返回数据库连接
func (r *BaseRepository) DB() *gorm.DB {
	return r.db
}

// WithTx 返回使用事务的Repository
func (r *BaseRepository) WithTx(tx *gorm.DB) Repository {
	return &BaseRepository{db: tx}
}

// Begin 开启事务
func (r *BaseRepository) Begin() *gorm.DB {
	return r.db.Begin()
}

// Tx 执行事务
func (r *BaseRepository) Tx(fn func(tx *gorm.DB) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})
}

// BuildPagination 构建分页信息
func BuildPagination(total int64, page, pageSize int) PaginationResult {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	return PaginationResult{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	}
}
