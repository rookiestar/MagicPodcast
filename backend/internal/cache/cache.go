package cache

import (
	"fmt"
	"sync"
	"time"
)

// CacheItem 缓存项
type CacheItem struct {
	Data      interface{}
	ExpiresAt time.Time
}

// Cache 内存缓存服务
type Cache struct {
	items sync.Map
	ttl   time.Duration
}

// globalCache 全局缓存实例
var globalCache *Cache
var once sync.Once

// GetCache 获取全局缓存实例
func GetCache() *Cache {
	once.Do(func() {
		globalCache = &Cache{
			ttl: 5 * time.Minute, // 默认5分钟TTL
		}
		// 启动后台清理协程
		go globalCache.startCleanup()
	})
	return globalCache
}

// NewCache 创建新的缓存实例
func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		ttl: ttl,
	}
	go c.startCleanup()
	return c
}

// Set 设置缓存
func (c *Cache) Set(key string, value interface{}) {
	c.SetWithTTL(key, value, c.ttl)
}

// SetWithTTL 设置缓存（自定义TTL）
func (c *Cache) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	item := CacheItem{
		Data:      value,
		ExpiresAt: time.Now().Add(ttl),
	}
	c.items.Store(key, item)
}

// Get 获取缓存
func (c *Cache) Get(key string) (interface{}, bool) {
	value, ok := c.items.Load(key)
	if !ok {
		return nil, false
	}

	item := value.(CacheItem)
	// 检查是否过期
	if time.Now().After(item.ExpiresAt) {
		c.items.Delete(key)
		return nil, false
	}

	return item.Data, true
}

// Delete 删除缓存
func (c *Cache) Delete(key string) {
	c.items.Delete(key)
}

// DeleteByPrefix 删除指定前缀的所有缓存
func (c *Cache) DeleteByPrefix(prefix string) {
	c.items.Range(func(key, value interface{}) bool {
		if k, ok := key.(string); ok {
			if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
				c.items.Delete(key)
			}
		}
		return true
	})
}

// Clear 清空所有缓存
func (c *Cache) Clear() {
	c.items.Range(func(key, value interface{}) bool {
		c.items.Delete(key)
		return true
	})
}

// startCleanup 启动后台清理协程
func (c *Cache) startCleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup 清理过期缓存
func (c *Cache) cleanup() {
	now := time.Now()
	c.items.Range(func(key, value interface{}) bool {
		if item, ok := value.(CacheItem); ok {
			if now.After(item.ExpiresAt) {
				c.items.Delete(key)
			}
		}
		return true
	})
}

// GetOrSet 获取缓存，如果不存在则执行fn并缓存结果
func (c *Cache) GetOrSet(key string, fn func() (interface{}, error)) (interface{}, error) {
	// 先尝试从缓存获取
	if data, ok := c.Get(key); ok {
		return data, nil
	}

	// 执行函数获取数据
	data, err := fn()
	if err != nil {
		return nil, err
	}

	// 缓存结果
	c.Set(key, data)
	return data, nil
}

// Stats 缓存统计
type Stats struct {
	TotalItems int
	HitCount   int64
	MissCount  int64
}

// hitCount 和 missCount 用于统计
var hitCount int64
var missCount int64
var statsMutex sync.Mutex

// RecordHit 记录缓存命中
func RecordHit() {
	statsMutex.Lock()
	defer statsMutex.Unlock()
	hitCount++
}

// RecordMiss 记录缓存未命中
func RecordMiss() {
	statsMutex.Lock()
	defer statsMutex.Unlock()
	missCount++
}

// GetStats 获取缓存统计
func GetStats() Stats {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	var totalItems int
	GetCache().items.Range(func(_, _ interface{}) bool {
		totalItems++
		return true
	})

	return Stats{
		TotalItems: totalItems,
		HitCount:   hitCount,
		MissCount:  missCount,
	}
}

// ========== 业务失效函数 ==========

// InvalidatePodcastList 使播客列表缓存失效
// 当播客被创建、更新、删除、订阅状态改变时调用
func InvalidatePodcastList() {
	GetCache().DeleteByPrefix("podcasts:list:")
}

// InvalidatePodcastDetail 使指定播客详情缓存失效
// 当播客信息更新时调用
func InvalidatePodcastDetail(id uint) {
	cache := GetCache()
	cache.Delete(fmt.Sprintf("podcasts:detail:%d", id))
	// 同时使列表缓存失效
	InvalidatePodcastList()
}

// InvalidateTagList 使标签列表缓存失效
// 当标签被创建、更新、删除时调用
func InvalidateTagList() {
	cache := GetCache()
	cache.Delete("tags:list")
	// 播客列表可能包含标签信息，也需要失效
	InvalidatePodcastList()
}

// InvalidateTagDetail 使指定标签详情缓存失效
func InvalidateTagDetail(id uint) {
	cache := GetCache()
	cache.Delete(fmt.Sprintf("tags:detail:%d", id))
	InvalidateTagList()
}

// InvalidateWorkflowList 使工作流列表缓存失效
// 当工作流被创建、更新、删除、启用/禁用时调用
func InvalidateWorkflowList() {
	GetCache().DeleteByPrefix("workflows:list")
}

// InvalidateWorkflowDetail 使指定工作流详情缓存失效
func InvalidateWorkflowDetail(id uint) {
	cache := GetCache()
	cache.Delete(fmt.Sprintf("workflows:detail:%d", id))
	InvalidateWorkflowList()
}

// InvalidateEpisodeList 使单集列表缓存失效
// 当单集被添加或更新时调用
func InvalidateEpisodeList(podcastID uint) {
	cache := GetCache()
	cache.DeleteByPrefix(fmt.Sprintf("episodes:list:podcast:%d:", podcastID))
	// 播客详情可能包含单集数量等信息
	InvalidatePodcastDetail(podcastID)
}

// InvalidateSearch 使搜索缓存失效
// 当任何可搜索内容变更时调用
func InvalidateSearch() {
	GetCache().DeleteByPrefix("search:")
}
