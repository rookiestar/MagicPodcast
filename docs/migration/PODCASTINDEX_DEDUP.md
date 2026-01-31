# PodcastIndex 数据库去重指南

## 概述

PodcastIndex数据库中存在大量重复的播客记录（相同的title但不同的id），这些重复记录可能来自：
- 不同的feed URL（http vs https）
- 不同的抓取时间
- 不同的iTunes ID
- 不同的状态（dead/active）

## 创建的文件

### 1. `create_unique_podcasts_view.sql`
创建一个视图 `v_unique_podcasts`，为每个title自动选择最优的记录。

**使用方法：**
```bash
# 在数据库中创建视图
sqlite3 podcast-index.db < create_unique_podcasts_view.sql

# 或者在Python中
import sqlite3
conn = sqlite3.connect('podcast-index.db')
with open('create_unique_podcasts_view.sql', 'r') as f:
    conn.executescript(f.read())
```

**查询唯一播客：**
```sql
-- 查询所有唯一播客
SELECT * FROM v_unique_podcasts;

-- 查询特定播客
SELECT * FROM v_unique_podcasts WHERE title = '无聊斋';

-- 只查询活跃的播客（dead=0）
SELECT * FROM v_unique_podcasts WHERE dead = 0;

-- 统计唯一播客数量
SELECT COUNT(*) FROM v_unique_podcasts;
```

### 2. `analyze_duplicates.sql`
分析脚本，用于了解重复播客的情况。

**使用方法：**
```bash
# 运行分析脚本
sqlite3 podcast-index.db < analyze_duplicates.sql > analysis_results.txt

# 或在Python中
import sqlite3
conn = sqlite3.connect('podcast-index.db')
with open('analyze_duplicates.sql', 'r') as f:
    for row in conn.execute(f.read()):
        print(row)
```

## 筛选规则详解

视图使用以下优先级（从高到低）选择最佳记录：

1. **dead 状态**
   - `dead = 0`（未失效）优先
   - `dead = 1`（已失效）排除

2. **HTTP 状态码**
   - `lasthttpstatus = 200` 优先
   - 其他状态码次之

3. **内容分级**
   - `explicit` 不为空的优先
   - 有分级信息的记录更完整

4. **优先级**
   - `priority` 数值小的优先
   - `priority = -1` 表示暂停，最低优先级
   - `priority = 0` 正常
   - `priority = 1-10` 优先级递增

5. **最新发布时间**
   - `newestItemPubdate` 最新的优先
   - 表示该feed仍在更新

6. **单集数量**
   - `episodeCount` 最多的优先
   - 更完整的feed

7. **受欢迎程度**
   - `popularityScore` 最高的优先
   - 更受欢迎的播客

8. **记录ID**
   - `id` 最小的优先
   - 作为最终的同级排序

## 实际应用示例

### 1. 查询特定播客的所有版本
```sql
-- 查看所有版本
SELECT id, url, dead, lasthttpstatus, newestItemPubdate, episodeCount
FROM podcasts
WHERE title = '无聊斋'
ORDER BY newestItemPubdate DESC;

-- 使用视图只获取最佳版本
SELECT * FROM v_unique_podcasts WHERE title = '无聊斋';
```

### 2. 导出唯一播客到新表
```sql
-- 创建新表只包含唯一播客
CREATE TABLE podcasts_unique AS
SELECT * FROM v_unique_podcasts;

-- 或者创建带索引的表
CREATE TABLE podcasts_unique (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    itunesAuthor TEXT,
    description TEXT,
    imageUrl TEXT,
    url TEXT NOT NULL UNIQUE,
    itunesId INTEGER,
    language TEXT,
    link TEXT,
    newestEnclosureUrl TEXT,
    newestEnclosureDuration INTEGER,
    lastUpdate INTEGER,
    newestItemPubdate INTEGER,
    oldestItemPubdate INTEGER,
    popularityScore INTEGER,
    priority INTEGER DEFAULT 5,
    updateFrequency INTEGER DEFAULT 0,
    episodeCount INTEGER DEFAULT 0,
    dead INTEGER DEFAULT 0,
    lasthttpstatus INTEGER,
    explicit TEXT
);

-- 插入数据
INSERT INTO podcasts_unique
SELECT * FROM v_unique_podcasts;

-- 创建索引
CREATE INDEX idx_title ON podcasts_unique(title);
CREATE INDEX idx_url ON podcasts_unique(url);
CREATE INDEX idx_dead ON podcasts_unique(dead);
```

### 3. 在Python中使用
```python
import sqlite3
import pandas as pd

# 连接数据库
conn = sqlite3.connect('podcast-index.db')

# 读取唯一播客
df = pd.read_sql_query("SELECT * FROM v_unique_podcasts", conn)

# 导出到CSV
df.to_csv('unique_podcasts.csv', index=False, encoding='utf-8-sig')

# 查看统计信息
print(f"总播客数: {len(df)}")
print(f"活跃播客数: {len(df[df['dead'] == 0])}")
print(f"平均单集数: {df['episodeCount'].mean():.1f}")
print(f"平均受欢迎度: {df['popularityScore'].mean():.1f}")

# 查找特定播客
podcast = df[df['title'] == '无聊斋']
print(podcast)
```

## 维护建议

1. **定期运行分析**
   - 每周运行一次 `analyze_duplicates.sql`
   - 监控新产生的重复记录

2. **更新视图**
   - 视图是实时计算的，不需要更新
   - 每次查询都是最新数据

3. **性能优化**
   - 对于大型数据库，可以创建物化视图（定期刷新）
   - 在 `title`, `url`, `dead` 字段上创建索引

4. **备份数据**
   - 在执行任何去重操作前备份数据库
   - 保留原始的重复数据以供参考

## 注意事项

⚠️ **重要提示：**
- 视图只是虚拟表，不会修改原始数据
- 如需物理去重，应该创建新表并导入数据
- 建议先在测试环境验证筛选结果
- 某些字段可能为空（NULL），查询时需要注意
