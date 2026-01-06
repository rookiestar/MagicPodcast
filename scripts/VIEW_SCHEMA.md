# v_unique_podcasts 视图Schema说明

## 视图结构

```sql
CREATE VIEW v_unique_podcasts AS
WITH ranked_podcasts AS (
    SELECT 
        ...,
        ROW_NUMBER() OVER (
            PARTITION BY title
            ORDER BY 筛选条件...
        ) AS rank_num
    FROM podcasts
)
SELECT ... FROM ranked_podcasts WHERE rank_num = 1;
```

## 字段列表

| 序号 | 字段名 | 类型 | 说明 | 来源 |
|------|--------|------|------|------|
| 1 | **id** | INTEGER | 主键ID | podcasts.id |
| 2 | **title** | TEXT | 播客标题 ⭐ | podcasts.title |
| 3 | **itunesAuthor** | TEXT | 作者/主播 | podcasts.itunesAuthor |
| 4 | **description** | TEXT | 播客描述 | podcasts.description |
| 5 | **imageUrl** | TEXT | 封面图片URL | podcasts.imageUrl |
| 6 | **url** | TEXT | RSS Feed URL | podcasts.url |
| 7 | **itunesId** | INTEGER | iTunes ID | podcasts.itunesId |
| 8 | **language** | TEXT | 语言代码 | podcasts.language |
| 9 | **link** | TEXT | 网站链接 | podcasts.link |
| 10 | **newestEnclosureUrl** | TEXT | 最新单集音频URL | podcasts.newestEnclosureUrl |
| 11 | **newestEnclosureDuration** | INTEGER | 最新单集时长（秒） | podcasts.newestEnclosureDuration |
| 12 | **lastUpdate** | INTEGER | Feed最后更新时间（Unix时间戳） | podcasts.lastUpdate |
| 13 | **newestItemPubdate** | INTEGER | 最新单集发布时间 ⭐ | podcasts.newestItemPubdate |
| 14 | **oldestItemPubdate** | INTEGER | 最旧单集发布时间 | podcasts.oldestItemPubdate |
| 15 | **popularityScore** | INTEGER | 受欢迎程度（0-10）⭐ | podcasts.popularityScore |
| 16 | **priority** | INTEGER | 抓取优先级 ⭐ | podcasts.priority |
| 17 | **updateFrequency** | INTEGER | 更新频率（0-10） | podcasts.updateFrequency |
| 18 | **episodeCount** | INTEGER | 单集总数 ⭐ | podcasts.episodeCount |
| 19 | **dead** | INTEGER | 是否失效（0=否, 1=是）⭐ | podcasts.dead |
| 20 | **lasthttpstatus** | INTEGER | 最后HTTP状态码 ⭐ | podcasts.lasthttpstatus |
| 21 | **explicit** | TEXT | 内容分级（yes/no/clean）⭐ | podcasts.explicit |

⭐ = **用于筛选的字段**

## 筛选规则（ORDER BY优先级）

视图使用 `ROW_NUMBER()` 窗口函数为每个title分配排名，然后只选择 `rank_num = 1` 的记录。

### 排序优先级（从高到低）

```sql
ORDER BY
    1. dead ASC                                    -- 未失效优先
    2. CASE WHEN lasthttpstatus = 200 THEN 0 ELSE 1 END  -- HTTP 200优先
    3. CASE WHEN explicit IS NOT NULL AND explicit != '' THEN 0 ELSE 1 END  -- 有分级信息优先
    4. CASE WHEN priority = -1 THEN 999 ELSE priority END ASC  -- 优先级数值小的优先
    5. newestItemPubdate DESC                       -- 最新发布时间优先
    6. episodeCount DESC                            -- 单集数多优先
    7. popularityScore DESC                         -- 受欢迎程度高优先
    8. id ASC                                       -- ID小优先（早期记录）
```

### 详细说明

#### 1. **dead** - 失效状态
- **值**: `0` (未失效) 或 `1` (已失效)
- **规则**: `0` 优先于 `1`
- **原因**: 未失效的feed更可能可用

#### 2. **lasthttpstatus** - HTTP状态码
- **值**: 200, 404, 500等
- **规则**: `200` 优先于其他状态码
- **原因**: 状态码200表示feed可正常访问

#### 3. **explicit** - 内容分级
- **值**: `yes`, `no`, `clean`, 或 NULL
- **规则**: 不为空优先于为空
- **原因**: 有分级信息的记录更完整

#### 4. **priority** - 优先级
- **值**: `-1` (暂停), `0` (正常), `1-10` (优先级递增)
- **规则**: 
  - `-1` 转换为 `999`，最低优先级（暂停的feed）
  - 数值越小越优先
- **原因**: 
  - 0 = 正常优先级
  - 1-10 = 高优先级
  - -1 = 暂停抓取

#### 5. **newestItemPubdate** - 最新单集发布时间
- **值**: Unix时间戳（如1704067200）
- **规则**: 数值越大越优先（越新越好）
- **原因**: 最新的feed说明仍在更新

#### 6. **episodeCount** - 单集总数
- **值**: 整数
- **规则**: 数值越大越优先
- **原因**: 单集数多说明feed更完整

#### 7. **popularityScore** - 受欢迎程度
- **值**: 0-10
- **规则**: 数值越大越优先
- **原因**: 更受欢迎的feed质量更高

#### 8. **id** - 主键ID
- **值**: 自增ID
- **规则**: 数值越小越优先
- **原因**: 最早的记录作为最后的选择标准

## 使用示例

### 基础查询
```sql
-- 查询所有唯一播客
SELECT * FROM v_unique_podcasts;

-- 查询总数
SELECT COUNT(*) FROM v_unique_podcasts;

-- 查询活跃的播客（未失效）
SELECT * FROM v_unique_podcasts WHERE dead = 0;
```

### 高级查询
```sql
-- 查询中文播客，受欢迎度>=7，单集数>50
SELECT 
    title,
    itunesAuthor,
    episodeCount,
    popularityScore,
    newestItemPubdate
FROM v_unique_podcasts
WHERE language LIKE 'zh%'
  AND popularityScore >= 7
  AND episodeCount > 50
ORDER BY popularityScore DESC, episodeCount DESC;

-- 统计各语言的播客数量
SELECT 
    language,
    COUNT(*) as count,
    AVG(popularityScore) as avg_popularity,
    AVG(episodeCount) as avg_episodes
FROM v_unique_podcasts
WHERE dead = 0
GROUP BY language
ORDER BY count DESC;

-- 查找最近更新的播客（最近30天）
SELECT 
    title,
    itunesAuthor,
    datetime(newestItemPubdate, 'unixepoch') as latest_pub,
    episodeCount
FROM v_unique_podcasts
WHERE newestItemPubdate > strftime('%s', 'now', '-30 days', 'unixepoch')
ORDER BY newestItemPubdate DESC;
```

## 视图特点

✅ **实时更新**：视图是虚拟表，每次查询都会重新计算  
✅ **不占用空间**：不存储数据，只存储查询逻辑  
✅ **自动去重**：每个title只返回一条最优记录  
✅ **灵活筛选**：可根据需要添加WHERE条件  

## 性能建议

对于大型数据库，建议在以下字段上创建索引：

```sql
CREATE INDEX idx_podcasts_title ON podcasts(title);
CREATE INDEX idx_podcasts_dead ON podcasts(dead);
CREATE INDEX idx_podcasts_lasthttpstatus ON podcasts(lasthttpstatus);
CREATE INDEX idx_podcasts_newestItemPubdate ON podcasts(newestItemPubdate);
```

## 与原始表的对比

| 特性 | 原始 podcasts 表 | v_unique_podcasts 视图 |
|------|-------------------|------------------------|
| 记录数 | 可能包含重复 | 每个title唯一 |
| 查询速度 | 快（物理表） | 稍慢（需计算） |
| 数据更新 | 直接更新 | 自动反映 |
| 去重逻辑 | 需手动编写 | 自动处理 |

## 注意事项

⚠️ **重要提示**：
1. 视图不会物理删除重复数据，只是隐藏
2. 每次查询都会重新计算排名，大数据量时可能较慢
3. 如果原始表更新，视图会自动反映最新数据
4. 建议在 `title`, `dead`, `lasthttpstatus`, `newestItemPubdate` 上创建索引以提升性能
