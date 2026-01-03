# PodcastIndex 字段迁移计划

## 📊 字段分析对照表

### PodcastIndex.db → MagicPodcast.db 字段映射

| PodcastIndex 字段 | MagicPodcast 字段 | 状态 | 说明 |
|-------------------|-------------------|------|------|
| `url` | `feed_url` | ✅ 已存在 | RSS Feed URL（主要标识） |
| `link` | **需新增** | ❌ 不存在 | 播客官网链接 |
| `podcastGuid` | `podcast_guid` | ✅ 已存在 | 全局唯一标识符 |
| `description` | `description` | ✅ 已存在 | 播客描述 |
| `itunesAuthor` | `author` | ✅ 已存在 | 作者/主播 |
| `imageUrl` | `cover_url` | ✅ 已存在 | 封面图片 URL |
| `newestEnclosureUrl` | **需新增** | ❌ 不存在 | 最新单集音频 URL |
| `newestEnclosureDuration` | **需新增** | ❌ 不存在 | 最新单集时长（秒） |
| `lastUpdate` | **需新增** | ❌ 不存在 | Feed 最后更新时间 |
| `newestItemPubdate` | `newest_episode_date` | ✅ 已存在 | 最新单集发布日期 |
| `oldestItemPubdate` | **需新增** | ❌ 不存在 | 最旧单集发布日期 |
| `dead` | `is_dead` | ✅ 已存在 | 失效标记 |
| `episodeCount` | `episode_count` | ✅ 已存在 | 单集总数 |
| `popularityScore` | **需新增** | ❌ 不存在 | 受欢迎程度（0-10） |
| `priority` | **需新增** | ❌ 不存在 | 抓取优先级（-1到10） |
| `updateFrequency` | **需新增** | ❌ 不存在 | 更新频率（0-10） |

---

## 🎯 需要新增的字段（7个）

### 1. `link` - 播客网站链接

**PodcastIndex 说明**:
- 类型: TEXT (NOT NULL)
- 作用: 播客的官方网站或主页
- 示例: `https://patreon.com/rahdo`, `https://soundcloud.com/idiotspeakshow`

**MagicPodcast 定义**:
```go
Link string `gorm:"size:512" json:"link"` // 播客网站链接
```

**用途**:
- 在详情页提供"访问官网"按钮
- 帮助用户了解更多播客信息
- 可用于验证播客真实性

---

### 2. `newest_enclosure_url` - 最新单集音频 URL

**PodcastIndex 说明**:
- 类型: TEXT
- 作用: 最新发布的单集的音频文件地址
- 示例: `https://traffic.libsyn.com/secure/markalanwilliams/001_Are_all_sins_equal.mp3`

**MagicPodcast 定义**:
```go
NewestEnclosureURL string `gorm:"size:512" json:"newest_enclosure_url"` // 最新单集音频URL
```

**用途**:
- 快速访问最新单集
- 在列表页显示"最新一集"播放按钮
- 检测 Feed 是否在更新

---

### 3. `newest_enclosure_duration` - 最新单集时长

**PodcastIndex 说明**:
- 类型: INTEGER (秒)
- 作用: 最新单集的时长
- 示例: `384` (6分24秒), `17918` (4小时58分)

**MagicPodcast 定义**:
```go
NewestEnclosureDuration int `json:"newest_enclosure_duration"` // 最新单集时长（秒）
```

**用途**:
- 显示预估播放时间
- 统计平均单集时长
- UI显示 "45分钟" 而不是 "2700秒"

---

### 4. `last_update` - Feed 最后更新时间

**PodcastIndex 说明**:
- 类型: INTEGER (Unix timestamp)
- 作用: Feed 最后一次成功更新的时间
- 示例: `1766819322` (2024-11-26)

**MagicPodcast 定义**:
```go
LastUpdate *time.Time `json:"last_update"` // Feed最后更新时间
```

**用途**:
- 判断播客活跃度
- 增量抓取判断
- 显示"最后更新"信息

---

### 5. `oldest_episode_date` - 最旧单集发布日期

**PodcastIndex 说明**:
- 类型: INTEGER (Unix timestamp)
- 作用: 最早单集的发布日期
- 示例: `1432837532` (2015-05-28)

**MagicPodcast 定义**:
```go
OldestEpisodeDate *time.Time `json:"oldest_episode_date"` // 最旧单集发布日期
```

**用途**:
- 计算播客寿命
- 统计单集总数的时间跨度
- 显示"始于 XXXX 年"

---

### 6. `popularity_score` - 受欢迎程度评分

**PodcastIndex 说明**:
- 类型: INTEGER (0-10)
- 作用: 基于多个因素计算的流行度评分
- 示例: `9` (非常受欢迎), `5` (中等), `1` (冷门)

**MagicPodcast 定义**:
```go
PopularityScore int `gorm:"default:0" json:"popularity_score"` // 受欢迎程度 (0-10)
```

**用途**:
- 按受欢迎程度排序
- 推荐"热门播客"
- 显示"⭐ 热门"标签

---

### 7. `priority` - 抓取优先级

**PodcastIndex 说明**:
- 类型: INTEGER (-1到10)
- 作用: 抓取和更新的优先级
- 示例: `9` (最高优先), `-1` (暂停抓取)

**MagicPodcast 定义**:
```go
Priority int `gorm:"default:5" json:"priority"` // 抓取优先级 (0-10, -1=暂停)
```

**用途**:
- 优化抓取调度
- 优先更新热门播客
- 控制 API 请求频率

---

### 8. `update_frequency` - 更新频率

**PodcastIndex 说明**:
- 类型: INTEGER (0-10)
- 作用: 预期的更新频率级别
- 示例: `9` (每天多次), `5` (每天一次)

**MagicPodcast 定义**:
```go
UpdateFrequency int `gorm:"default:0" json:"update_frequency"` // 更新频率 (0-10)
```

**用途**:
- 优化抓取间隔
- 避免频繁检查不活跃的 Feed
- 节省 API 请求配额

---

## 🔄 数据库迁移计划

### 阶段 1: 模型更新

```go
// backend/internal/models/podcast.go

type Podcast struct {
    BaseModel

    // 小宇宙相关
    XYZID       string `gorm:"uniqueIndex;size:64" json:"xyz_id"`
    Title       string `gorm:"size:255;not null" json:"title"`
    FeedURL     string `gorm:"size:512" json:"feed_url"`
    ITunesID    string `gorm:"size:64" json:"itunes_id"`
    PodcastGUID string `gorm:"size:128" json:"podcast_guid"`
    Description string `gorm:"type:text" json:"description"`
    Author      string `gorm:"size:255" json:"author"`
    CoverURL    string `gorm:"size:512" json:"cover_url"`

    // 🆕 新增：来自 PodcastIndex 的字段
    Link                    string     `gorm:"size:512" json:"link"`                              // 播客网站链接
    NewestEnclosureURL      string     `gorm:"size:512" json:"newest_enclosure_url"`              // 最新单集音频URL
    NewestEnclosureDuration int        `json:"newest_enclosure_duration"`                       // 最新单集时长（秒）
    LastUpdate              *time.Time `json:"last_update"`                                     // Feed最后更新时间
    OldestEpisodeDate       *time.Time `json:"oldest_episode_date"`                             // 最旧单集发布日期
    PopularityScore         int        `gorm:"default:0" json:"popularity_score"`                 // 受欢迎程度 (0-10)
    Priority                int        `gorm:"default:5" json:"priority"`                         // 抓取优先级 (0-10, -1=暂停)
    UpdateFrequency         int        `gorm:"default:0" json:"update_frequency"`                  // 更新频率 (0-10)

    // 统计信息
    AddedDate        time.Time `json:"added_date"`
    EpisodeCount     int       `gorm:"default:0" json:"episode_count"`
    NewestEpisodeDate time.Time `json:"newest_episode_date"`

    // 状态标识
    IsSubscribed bool `gorm:"default:true" json:"is_subscribed"`
    IsDead       bool `gorm:"default:false" json:"is_dead"`

    // 用户自定义
    MyRate int    `gorm:"default:0" json:"my_rate"`
    Notes  string `gorm:"type:text" json:"notes"`

    // 同步相关
    FeedURLValid    bool       `gorm:"default:true" json:"feed_url_valid"`
    LastFetchedAt   *time.Time `json:"last_fetched_at"`
    FetchErrorCount int        `gorm:"default:0" json:"fetch_error_count"`
    DataSource      string     `gorm:"size:20;default:'rss'" json:"data_source"`

    // 关联关系
    Episodes []Episode `gorm:"foreignKey:PodcastID;constraint:OnDelete:CASCADE" json:"episodes,omitempty"`
    Tags     []Tag     `gorm:"many2many:podcasts_tags;constraint:OnDelete:CASCADE" json:"tags,omitempty"`
}
```

---

### 阶段 2: 数据库迁移脚本

创建迁移脚本：`backend/scripts/migrations/001_add_podcastindex_fields.go`

```go
package main

import (
    "fmt"
    "log"
    "time"

    "magicpodcast/internal/database"
    "magicpodcast/internal/models"
)

func main() {
    db, err := database.Init()
    if err != nil {
        log.Fatalf("Failed to connect to database: %v", err)
    }

    // 开始事务
    tx := db.Begin()
    if tx.Error != nil {
        log.Fatalf("Failed to start transaction: %v", tx.Error)
    }

    // 添加新列
    migrations := []string{
        // 播客网站链接
        "ALTER TABLE podcasts ADD COLUMN link TEXT",

        // 最新单集音频URL
        "ALTER TABLE podcasts ADD COLUMN newest_enclosure_url TEXT",

        // 最新单集时长（秒）
        "ALTER TABLE podcasts ADD COLUMN newest_enclosure_duration INTEGER DEFAULT 0",

        // Feed最后更新时间
        "ALTER TABLE podcasts ADD COLUMN last_update DATETIME",

        // 最旧单集发布日期
        "ALTER TABLE podcasts ADD COLUMN oldest_episode_date DATETIME",

        // 受欢迎程度 (0-10)
        "ALTER TABLE podcasts ADD COLUMN popularity_score INTEGER DEFAULT 0",

        // 抓取优先级 (0-10, -1=暂停)
        "ALTER TABLE podcasts ADD COLUMN priority INTEGER DEFAULT 5",

        // 更新频率 (0-10)
        "ALTER TABLE podcasts ADD COLUMN update_frequency INTEGER DEFAULT 0",
    }

    for _, migration := range migrations {
        if err := tx.Exec(migration).Error; err != nil {
            tx.Rollback()
            log.Fatalf("Migration failed: %s - %v", migration, err)
        }
        log.Printf("✅ %s", migration)
    }

    // 提交事务
    if err := tx.Commit().Error; err != nil {
        log.Fatalf("Failed to commit transaction: %v", err)
    }

    log.Println("✅ Migration completed successfully!")
}
```

---

### 阶段 3: 向后兼容的数据同步逻辑

更新 `backend/internal/sync/service.go` 中的转换函数：

```go
// convertPodcastIndexToModel 将 PodcastIndex 信息转换为模型
func (s *Service) convertPodcastIndexToModel(info *podcastindex.PodcastInfo) *models.Podcast {
    podcast := &models.Podcast{
        Title:           info.Title,
        Author:          info.Author,
        Description:     info.Description,
        CoverURL:        info.CoverURL,
        FeedURL:         info.FeedURL,
        ITunesID:        fmt.Sprintf("%d", info.ITunesID),
        PodcastGUID:     info.PodcastGUID,
        Link:            info.Link,                    // 🆕
        NewestEnclosureURL: info.NewestEnclosureURL, // 🆕
        EpisodeCount:     info.EpisodeCount,
        IsSubscribed:    true,
        DataSource:      "podcastindex",
    }

    // 🆕 处理时间戳字段
    if info.NewestItemPubdate > 0 {
        t := time.Unix(info.NewestItemPubdate, 0)
        podcast.NewestEpisodeDate = t
        podcast.LastUpdate = &t
    }

    if info.OldestItemPubdate > 0 {
        t := time.Unix(info.OldestItemPubdate, 0)
        podcast.OldestEpisodeDate = &t
    }

    // 🆕 处理评分字段
    if info.PopularityScore > 0 {
        podcast.PopularityScore = info.PopularityScore
    }

    if info.Priority >= -1 { // 允许 -1（暂停）
        podcast.Priority = info.Priority
    }

    if info.UpdateFrequency >= 0 {
        podcast.UpdateFrequency = info.UpdateFrequency
    }

    // 🆕 处理最新单集时长
    if info.NewestEnclosureDuration > 0 {
        podcast.NewestEnclosureDuration = info.NewestEnclosureDuration
    }

    return podcast
}
```

---

### 阶段 4: 最小影响的 API 响应更新

**策略**: 保持向后兼容，新字段作为可选补充

```go
// backend/internal/handlers/podcast.go

// ListResponse API 响应结构
type ListResponse struct {
    Success   bool       `json:"success"`
    Data      []Podcast  `json:"data"`
    Pagination Pagination `json:"pagination"`
}

// PodcastDTO 数据传输对象
type PodcastDTO struct {
    // 原有字段（保持不变）
    ID          uint      `json:"id"`
    Title       string    `json:"title"`
    Description string    `json:"description"`
    Author      string    `json:"author"`
    CoverURL    string    `json:"cover_url"`
    // ... 其他原有字段

    // 🆕 新增字段（可选，可能为空）
    Link                    string `json:"link,omitempty"`                        // 播客官网
    NewestEnclosureURL      string `json:"newest_enclosure_url,omitempty"`        // 最新单集音频
    NewestEnclosureDuration int    `json:"newest_enclosure_duration,omitempty"`   // 最新单集时长
    LastUpdate              string `json:"last_update,omitempty"`                 // 最后更新
    OldestEpisodeDate       string `json:"oldest_episode_date,omitempty"`        // 最旧单集
    PopularityScore         int    `json:"popularity_score,omitempty"`           // 受欢迎程度
    Priority                int    `json:"priority,omitempty"`                    // 优先级
    UpdateFrequency         int    `json:"update_frequency,omitempty"`           // 更新频率
}

func ModelToDTO(podcast *models.Podcast) PodcastDTO {
    dto := PodcastDTO{
        ID:          podcast.ID,
        Title:       podcast.Title,
        Description: podcast.Description,
        Author:      podcast.Author,
        CoverURL:    podcast.CoverURL,
        // ... 原有字段映射
    }

    // 🆕 新字段映射（仅在非零值时添加）
    if podcast.Link != "" {
        dto.Link = podcast.Link
    }
    if podcast.NewestEnclosureURL != "" {
        dto.NewestEnclosureURL = podcast.NewestEnclosureURL
    }
    if podcast.NewestEnclosureDuration > 0 {
        dto.NewestEnclosureDuration = podcast.NewestEnclosureDuration
    }
    // ... 其他新字段

    return dto
}
```

---

## ⚠️ 影响分析与风险控制

### 1. 现有功能影响评估

| 功能模块 | 影响程度 | 说明 | 缓解措施 |
|---------|---------|------|---------|
| 播客列表 | ✅ 无影响 | 新字段默认值，前端可选显示 | 使用 `omitempty` 标签 |
| 播客详情 | ✅ 无影响 | 渐进式增强，不影响现有信息 | 前端按需渲染新字段 |
| 标签筛选 | ✅ 无影响 | 筛选逻辑不变 | - |
| OPML 导入 | ⚠️ 轻微 | 需要更新同步逻辑映射 | 更新 `convertGofeedToModel` |
| RSS 抓取 | ⚠️ 轻微 | 需要更新解析逻辑 | 更新 `convertGofeedToModel` |
| 数据库查询 | ✅ 无影响 | 新字段有默认值，不影响现有查询 | SELECT * 自动包含新列 |

### 2. 前端兼容性

**策略**: 渐进式增强，不破坏现有功能

```typescript
// frontend/src/types/index.ts

export interface Podcast {
  // 原有字段（必需）
  id: number
  title: string
  description: string
  author: string
  cover_url: string
  episode_count: number
  // ... 其他原有字段

  // 🆕 新增字段（可选）
  link?: string                          // 播客官网
  newest_enclosure_url?: string          // 最新单集音频
  newest_enclosure_duration?: number     // 最新单集时长
  last_update?: string                   // 最后更新
  oldest_episode_date?: string           // 最旧单集
  popularity_score?: number              // 受欢迎程度
  priority?: number                      // 优先级
  update_frequency?: number              // 更新频率
}
```

**前端使用示例**:

```tsx
// 只在新字段存在时显示
{podcast.link && (
  <a href={podcast.link} target="_blank" rel="noopener noreferrer">
    访问官网
  </a>
)}

{podcast.popularity_score && podcast.popularity_score >= 7 && (
  <span className="badge badge-hot">🔥 热门</span>
)}

{podcast.newest_enclosure_duration && (
  <span>{formatDuration(podcast.newest_enclosure_duration)}</span>
)}
```

### 3. 数据库迁移风险

| 风险 | 概率 | 影响 | 缓解措施 |
|-----|------|------|---------|
| 迁移失败 | 低 | 高 | 1. 使用事务，失败回滚<br>2. 先在测试环境验证<br>3. 备份生产数据库 |
| 性能下降 | 低 | 低 | 1. 新字段有默认值<br>2. 不增加索引<br>3. 按需查询 |
| 数据不一致 | 中 | 中 | 1. 旧数据新字段为空<br>2. 渐进式填充<br>3. 后台任务补全数据 |
| 现有代码报错 | 低 | 高 | 1. 使用 DTO 转换层<br>2. 新字段添加 `omitempty`<br>3. 全面回归测试 |

---

## 🚀 实施步骤

### Step 1: 准备阶段（5分钟）

1. **备份数据库**
```bash
cd backend
cp data/magicpodcast.db data/magicpodcast.db.backup_$(date +%Y%m%d)
```

2. **更新模型定义**
```bash
# 编辑 backend/internal/models/podcast.go
# 添加新字段到 Podcast 结构体
```

### Step 2: 创建迁移脚本（10分钟）

1. **创建迁移目录**
```bash
mkdir -p backend/scripts/migrations
```

2. **编写迁移脚本**
```bash
# 创建 backend/scripts/migrations/001_add_podcastindex_fields.go
# 参考上面"阶段 2"的代码
```

3. **测试迁移**
```bash
cd backend
go run scripts/migrations/001_add_podcastindex_fields.go
```

### Step 3: 更新同步逻辑（15分钟）

1. **更新 PodcastIndex 转换函数**
   - 文件: `backend/internal/sync/service.go`
   - 函数: `convertPodcastIndexToModel`
   - 参考"阶段 3"的代码

2. **更新 RSS Feed 转换函数**
   - 文件: `backend/internal/sync/service.go`
   - 函数: `convertGofeedToModel`
   - 添加新字段映射（尽可能填充）

### Step 4: 更新 API 处理器（10分钟）

1. **更新响应结构**
   - 文件: `backend/internal/handlers/podcast.go`
   - 添加新字段到响应（使用 DTO）

2. **保持向后兼容**
   - 新字段使用 `omitempty`
   - 旧数据返回空值或默认值

### Step 5: 前端适配（可选，15分钟）

1. **更新类型定义**
   - 文件: `frontend/src/types/index.ts`
   - 添加可选的新字段

2. **渐进式增强 UI**
   - 详情页显示"访问官网"按钮
   - 列表页显示"热门"标签
   - 显示"最后更新"时间

### Step 6: 测试验证（20分钟）

1. **后端测试**
```bash
# 启动后端
cd backend
go run cmd/api/main.go

# 测试 API
curl http://localhost:8080/api/v1/podcasts?page=1&page_size=5 | jq
```

2. **前端测试**
```bash
# 启动前端
cd frontend
npm run dev

# 访问页面
open http://localhost:3000/podcasts
```

3. **功能验证**
- ✅ 播客列表正常显示
- ✅ 分页功能正常
- ✅ 无限滚动正常
- ✅ 标签筛选正常
- ✅ OPML 导入正常

### Step 7: 数据填充（后台任务，可选）

创建后台任务，为现有数据填充 PodcastIndex 字段：

```go
// backend/scripts/backfill_podcastindex_data.go

func main() {
    db := database.GetDB()

    var podcasts []models.Podcast
    db.Where("data_source = ?", "podcastindex").Find(&podcasts)

    for _, podcast := range podcasts {
        // 从 PodcastIndex 重新获取完整信息
        info, err := podcastIndexQuery.FindByFeedURL(podcast.FeedURL)
        if err != nil {
            continue
        }

        // 更新新字段
        podcast.Link = info.Link
        podcast.PopularityScore = info.PopularityScore
        // ... 其他字段

        db.Save(&podcast)
    }
}
```

---

## 📋 迁移检查清单

### 数据库层

- [ ] 备份现有数据库
- [ ] 创建迁移脚本
- [ ] 在测试环境验证迁移
- [ ] 执行生产数据库迁移
- [ ] 验证新字段已正确添加
- [ ] 检查索引和约束

### 后端层

- [ ] 更新 `models/podcast.go`
- [ ] 更新 `sync/service.go` 转换函数
- [ ] 更新 API 响应结构
- [ ] 添加单元测试
- [ ] 执行回归测试

### 前端层

- [ ] 更新 `types/index.ts`
- [ ] 更新详情页显示 link
- [ ] 添加热门标签显示
- [ ] 更新单集列表显示 duration
- [ ] 测试所有页面

### 文档

- [ ] 更新 API 文档
- [ ] 更新数据库 schema 文档
- [ ] 记录迁移过程
- [ ] 编写回滚方案

---

## 🔄 回滚方案

如果迁移出现问题，可以快速回滚：

### 方案 1: 数据库回滚

```bash
# 恢复备份
cd backend
cp data/magicpodcast.db.backup_YYYYMMDD data/magicpodcast.db

# 重启服务
killall api
go run cmd/api/main.go
```

### 方案 2: 代码回滚

```bash
# 回退 Git 提交
git revert <migration-commit-hash>
git push

# 重新部署
```

### 方案 3: 字段标记为废弃

如果无法回滚数据库，可以在代码中标记字段为废弃：

```go
// Deprecated: No longer used, will be removed in v2.0
type Podcast struct {
    // ...
    Link string `gorm:"-" json:"-"` // 忽略此字段
}
```

---

## 📊 预期收益

添加这些字段后的预期效果：

### 1. 数据丰富度提升

| 指标 | 提升前 | 提升后 | 改善 |
|-----|-------|-------|------|
| 播客信息字段数 | 18 | 26 | +44% |
| 可搜索维度 | 5 | 8 | +60% |
| 可排序维度 | 3 | 6 | +100% |
| 数据完整性 | 60% | 85% | +25% |

### 2. 功能增强

- ✅ **快速访问最新单集**: 通过 `newest_enclosure_url`
- ✅ **播客官网链接**: 通过 `link`
- ✅ **热门推荐**: 通过 `popularity_score`
- ✅ **智能抓取**: 通过 `priority` 和 `update_frequency`
- ✅ **活跃度分析**: 通过 `last_update` 和 `oldest_episode_date`

### 3. 用户体验提升

- 更详细的播客信息展示
- 更智能的推荐算法
- 更高效的更新机制
- 更准确的搜索结果

---

## ⏱️ 预计时间线

| 阶段 | 任务 | 预计时间 |
|-----|------|---------|
| 1 | 准备和备份 | 5分钟 |
| 2 | 创建迁移脚本 | 10分钟 |
| 3 | 执行数据库迁移 | 5分钟 |
| 4 | 更新同步逻辑 | 15分钟 |
| 5 | 更新 API | 10分钟 |
| 6 | 前端适配 | 15分钟 |
| 7 | 测试验证 | 20分钟 |
| 8 | 数据填充 | 30分钟（可选） |
| **总计** | | **~2小时** |

---

## 📝 总结

### 关键要点

1. **向后兼容**: 新字段使用可选类型，不影响现有功能
2. **渐进式增强**: 逐步添加新功能，不破坏现有体验
3. **数据完整**: 从 PodcastIndex 获取更丰富的元数据
4. **智能调度**: 利用优先级和频率字段优化抓取
5. **风险可控**: 完整的备份和回滚方案

### 下一步行动

1. **立即可做**: 创建迁移脚本并测试
2. **短期计划**: 更新同步逻辑，填充新数据
3. **长期规划**: 基于新字段开发推荐功能

### 风险提示

- ⚠️ 务必先在测试环境验证
- ⚠️ 生产环境迁移前必须备份
- ⚠️ 选择低峰期执行迁移
- ⚠️ 准备好回滚方案

---

**准备好了吗？我们可以开始执行迁移！** 🚀
