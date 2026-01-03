# PodcastIndex.db podcasts 表字段详解

## 📊 表结构概览

PodcastIndex.org 是一个开源的播客索引数据库，包含超过 480 万个播客的信息。`podcasts` 表是其核心数据表，存储了播客的完整元数据。

**数据库文件**: `podcastindex_feeds.db` (4.5GB)
**表名**: `podcasts`
**总记录数**: ~4,800,000+ 条
**最后更新**: 2024年12月

---

## 🔑 字段详解

### 核心标识字段

#### 1. `id` - 主键
- **类型**: INTEGER (PRIMARY KEY)
- **作用**: 唯一标识符，自增ID
- **说明**: PodcastIndex 内部使用的数字ID
- **示例**: `1`, `2`, `4`

#### 2. `url` - RSS Feed URL (唯一索引)
- **类型**: TEXT (UNIQUE, NOT NULL)
- **作用**: 播客的 RSS Feed 地址，是播客的唯一标识
- **说明**:
  - 通过此 URL 订阅和更新播客内容
  - 是表中的唯一键，确保同一个 Feed 不会被重复索引
  - 可以是 RSS、Atom 或 JSON Feed 格式
- **示例**:
  ```
  https://markalanwilliams.libsyn.com/rss
  https://anchor.fm/s/19ccb320/podcast/rss
  https://feeds.soundcloud.com/users/soundcloud:users:150466516/sounds.rss
  ```

#### 3. `originalUrl` - 原始 URL
- **类型**: TEXT (NOT NULL)
- **作用**: 记录最初发现的 Feed URL
- **说明**:
  - 用于追踪 Feed 的重定向历史
  - 如果某个 Feed URL 改变了，这里保留最初的地址
  - 帮助识别和管理 Feed 变更
- **示例**:
  ```
  https://markalanwilliams.libsyn.com/rss
  https://anchor.fm/s/19ccb320/podcast/rss
  ```

#### 4. `link` - 播客网站链接
- **类型**: TEXT (NOT NULL)
- **作用**: 播客的官方网站或主页
- **说明**:
  - 通常是播客的主页、博客或展示页面
  - 用于用户访问和了解播客的详细信息
- **示例**:
  ```
  http://markalanwilliams.libsyn.com/webpage
  https://patreon.com/rahdo
  https://soundcloud.com/idiotspeakshow
  ```

#### 5. `podcastGuid` - 播客 GUID
- **类型**: TEXT
- **作用**: 播客的全局唯一标识符 (UUID)
- **说明**:
  - 用于跨平台识别同一个播客
  - 通常由播客托管平台生成
  - 比播客 ID 更可靠，因为即使播客更换平台，GUID 仍可保持不变
- **示例**:
  ```
  78e9f51b-0ac3-5711-a921-1f2c7b5dbb35
  655b4e1a-4deb-56a2-9256-cad10f099410
  7fba92fb-42eb-56da-b562-cef9d2cf2546
  ```

---

## 📝 基础信息字段

#### 6. `title` - 播客标题
- **类型**: TEXT (NOT NULL)
- **作用**: 播客的名称
- **说明**:
  - 最主要的识别字段之一
  - 通常与播客托管平台或 iTunes 上的标题一致
  - 可以包含 Unicode 字符（支持中文、日文等）
- **示例**:
  ```
  Christianity Questions and Answers
  Rahdo Talks Through
  IdiotSpeakShow
  ```

#### 7. `description` - 播客描述
- **类型**: TEXT
- **作用**: 播客的详细描述或简介
- **说明**:
  - 通常来自 RSS Feed 中的 `<description>` 标签
  - 可以包含 HTML 格式
  - 用于向用户介绍播客的内容主题
- **示例**:
  ```
  Dr. Mark Alan Williams and friends answer questions about the Christian faith:
  questions about the God, Jesus, the Bible, eternity, belief, religion
  the reasonableness of faith and others.

  A podcast all about boardgames, hosted by Richard "Rahdo" Ham
  ```

#### 8. `itunesAuthor` - 作者/主播
- **类型**: TEXT (NOT NULL)
- **作用**: 播客的作者或主播名称
- **说明**:
  - 来自 iTunes 扩展命名空间
  - 通常是播客的主播、制作团队或机构名称
  - 用于展示和搜索
- **示例**:
  ```
  Dr. Mark Alan Williams
  Richard Ham
  IdiotSpeakShow
  ```

#### 9. `itunesOwnerName` - 所有者名称
- **类型**: TEXT (NOT NULL)
- **作用**: 播客所有者的名称
- **说明**:
  - 来自 iTunes Podcast 连接信息
  - 通常是个人或公司名称
  - 用于版权和联系信息
- **示例**:
  ```
  Mark Williams
  Richard Ham
  IdiotSpeakShow
  ```

#### 10. `host` - 托管平台
- **类型**: TEXT
- **作用**: Feed 的托管域名
- **说明**:
  - 从 Feed URL 中提取的域名
  - 用于统计播客托管平台的分布
  - 帮助识别流行的托管服务
- **示例**:
  ```
  libsyn.com
  anchor.fm
  soundcloud.com
  buzzsprout.com
  simplecast.com
  ```

---

## 🎨 多媒体字段

#### 11. `imageUrl` - 封面图片 URL
- **类型**: TEXT
- **作用**: 播客封面图片的地址
- **说明**:
  - 通常用于播客列表和详情页展示
  - 优先级高于 iTunes 图片
  - 可能是 JPG、PNG 等格式
- **示例**:
  ```
  https://static.libsyn.com/p/assets/e/7/5/d/e75de19145e2153b/thumb.jpg
  https://d3t3ozftmdmh3i.cloudfront.net/staging/podcast_uploaded_nologo/4228456/81316933823cb437.jpeg
  https://i1.sndcdn.com/avatars-000143391344-h7vuir-original.png
  ```

#### 12. `newestEnclosureUrl` - 最新单集音频 URL
- **类型**: TEXT
- **作用**: 最新发布的单集的音频文件地址
- **说明**:
  - 直接指向音频文件（MP3、M4A 等）
  - 用于播放最新单集
  - 可用于检测 Feed 是否在更新
- **示例**:
  ```
  https://traffic.libsyn.com/secure/markalanwilliams/001_Are_all_sins_equal.mp3
  https://traffic.megaphone.fm/APO8753353300.mp3
  https://feeds.soundcloud.com/stream/206778721-idiotspeakshow-idiotspeak-episode-3-game-of-spoilers.mp3
  ```

#### 13. `newestEnclosureDuration` - 最新单集时长
- **类型**: INTEGER
- **作用**: 最新单集的时长（秒）
- **说明**:
  - 用于预估播放时间
  - 可用于统计平均单集时长
  - 如果为 0 或 NULL 表示无法获取
- **示例**:
  ```
  384 (6分24秒)
  17918 (4小时58分38秒)
  2722 (45分22秒)
  ```

#### 14. `contentType` - 内容类型
- **类型**: TEXT (NOT NULL)
- **作用**: Feed 的 MIME 类型
- **说明**:
  - 表示 Feed 的格式和编码
  - 帮助解析器正确处理 Feed
  - 大多数是 `application/rss+xml` 或 `application/atom+xml`
- **示例**:
  ```
  application/rss+xml; charset=utf-8
  application/xml
  application/atom+xml
  ```

---

## 📅 时间戳字段

#### 15. `lastUpdate` - 最后更新时间
- **类型**: INTEGER (Unix timestamp)
- **作用**: Feed 最后一次成功更新的时间
- **说明**:
  - 表示播客的活跃程度
  - 用于判断播客是否仍在更新
  - 可用于增量抓取
- **示例**:
  ```
  1766819322 (2024-11-26)
  1766858366 (2024-11-27)
  1766821229 (2024-11-26)
  ```

#### 16. `newestItemPubdate` - 最新单集发布时间
- **类型**: INTEGER (Unix timestamp)
- **作用**: 最新单集的发布日期
- **说明**:
  - 表示播客的最新内容
  - 用于排序和推荐
  - 可以计算播客的更新频率
- **示例**:
  ```
  1610226815 (2021-01-09)
  1716150169 (2024-05-20)
  1432935194 (2015-05-29)
  ```

#### 17. `oldestItemPubdate` - 最旧单集发布时间
- **类型**: INTEGER (Unix timestamp)
- **作用**: 最早单集的发布日期
- **说明**:
  - 表示播客的开始时间
  - 可以计算播客的寿命和单集总数
  - 用于历史数据分析
- **示例**:
  ```
  1432837532 (2015-05-28)
  1433041200 (2015-05-31)
  1432337604 (2015-05-23)
  ```

#### 18. `createdOn` - 添加到索引的时间
- **类型**: INTEGER (Unix timestamp)
- **作用**: 该播客首次被 PodcastIndex 索引的时间
- **说明**:
  - 表示播客被发现的时间
  - 可用于数据分析和增长追踪
- **示例**:
  ```
  1596752487 (2020-08-07)
  ```

---

## 🔍 状态与质量字段

#### 19. `lastHttpStatus` - 最后 HTTP 状态码
- **类型**: INTEGER
- **作用**: 最后一次请求 Feed 的 HTTP 状态码
- **说明**:
  - 用于判断 Feed 的可用性
  - 常见值：
    - `200`: 成功
    - `301`: 永久重定向
    - `302`: 临时重定向
    - `404`: 未找到
    - `410`: 已删除
    - `500`: 服务器错误
- **示例**:
  ```
  200 (成功)
  404 (Feed 不存在)
  ```

#### 20. `dead` - 失效标记
- **类型**: INTEGER (0 或 1)
- **作用**: 标记 Feed 是否失效
- **说明**:
  - `0`: 正常
  - `1`: Feed 已失效（多次无法访问或返回错误）
  - 通常根据 `lastHttpStatus` 和连续失败次数判断
- **示例**:
  ```
  0 (正常)
  1 (失效)
  ```

#### 21. `explicit` - 内容分级标记
- **类型**: INTEGER (0 或 1)
- **作用**: 标记播客是否包含成人内容
- **说明**:
  - `0`: 无成人内容（适合所有年龄）
  - `1`: 包含成人内容（需要家长指导）
  - 来自 iTunes Podcast 规范
- **示例**:
  ```
  0 (适合所有年龄)
  1 (成人内容)
  ```

#### 22. `chash` - 内容哈希
- **类型**: TEXT (NOT NULL)
- **作用**: Feed 内容的 SHA1 哈希值
- **说明**:
  - 用于检测 Feed 是否发生变化
  - 避免重复处理相同的内容
  - 40 位十六进制字符串
- **示例**:
  ```
  cfc6e8dbaacbcba9f0b341c89df9a703
  f9fce8424a9918be9182c2d67fc51c16
  3414348449422fc0970d059cc680d0ab
  ```

---

## 📊 统计与分类字段

#### 23. `episodeCount` - 单集总数
- **类型**: INTEGER
- **作用**: 播客的单集总数
- **说明**:
  - 表示播客的规模
  - 用于搜索和排序
  - 可能为 0（如果未成功解析单集）
- **示例**:
  ```
  90 (90个单集)
  464 (464个单集)
    2 (2个单集)
  ```

#### 24. `popularityScore` - 受欢迎程度评分
- **类型**: INTEGER
- **作用**: 播客的流行度评分（0-10）
- **说明**:
  - 基于多个因素计算：
    - 订阅数量
    - 下载量
    - 更新频率
    - 社交媒体提及
  - 用于推荐和排序
- **示例**:
  ```
  9 (非常受欢迎)
  5 (中等)
  1 (不太受欢迎)
  ```

#### 25. `priority` - 优先级标记
- **类型**: INTEGER (0-10)
- **作用**: 抓取和更新的优先级
- **说明**:
  - 高优先级的播客会更频繁地被检查更新
  - 通常根据 `popularityScore` 计算
  - 热门播客（如 NPR、BBC）会有更高优先级
- **示例**:
  ```
  9 (最高优先级)
  5 (中等优先级)
  1 (低优先级)
  -1 (特殊标记，暂停抓取)
  ```

#### 26. `updateFrequency` - 更新频率
- **类型**: INTEGER (0-10)
- **作用**: 预期的更新频率级别
- **说明**:
  - 根据历史更新模式计算
  - 用于优化抓取调度
  - 值越大表示更新越频繁
- **示例**:
  ```
  9 (每天多次)
  5 (每天一次)
  3 (每周一次)
  1 (每月一次)
  ```

#### 27. `language` - 语言代码
- **类型**: TEXT
- **作用**: 播客的主要语言
- **说明**:
  - ISO 639-1 语言代码（2个字母）
  - 用于语言筛选和推荐
- **示例**:
  ```
  en (英语)
  zh (中文)
  es (西班牙语)
  ja (日语)
  de (德语)
  ```

---

## 🏷️ 分类字段

#### 28-37. `category1` 到 `category10` - 分类标签
- **类型**: TEXT
- **作用**: 播客的分类（最多10个）
- **说明**:
  - 来自 iTunes Podcast 分类
  - 一个播客可以有多个分类
  - 用于分类浏览和推荐
  - 常见分类：
    - `news`: 新闻
    - `sports`: 体育
    - `technology`: 科技
    - `business`: 商业
    - `comedy`: 喜剧
    - `education`: 教育
    - `games`: 游戏
    - `leisure`: 休闲
    - `religion`: 宗教
    - `spirituality`: 灵性
    - `christianity`: 基督教
- **示例**:
  ```
  religion, spirituality, christianity
  leisure, games
  (空)
  ```

---

## 🔧 技术字段

#### 38. `itunesId` - iTunes ID
- **类型**: INTEGER
- **作用**: Apple Podcasts 的播客 ID
- **说明**:
  - 用于链接到 iTunes Store
  - 可以通过此 ID 获取额外的 iTunes 元数据
  - 部分播客可能没有此 ID
- **示例**:
  ```
  1000000618
  1000016089
  1000035657
  ```

#### 39. `itunesType` - iTunes 类型
- **类型**: TEXT
- **作用**: 播客的类型（ episodic 或 serial ）
- **说明**:
  - `episodic`: 每集独立，可以按任何顺序收听
  - `serial`: 有连续剧情，建议按顺序收听
  - 来自 iTunes Podcast 规范
- **示例**:
  ```
  episodic ( episodic)
  serial (连续剧)
  ```

#### 40. `generator` - Feed 生成器
- **类型**: TEXT
- **作用**: 生成 RSS Feed 的软件或平台
- **说明**:
  - 用于统计托管平台分布
  - 帮助诊断 Feed 问题
  - 常见值：
    - `Libsyn RSSgen 1.0`
    - `Anchor Podcasts`
    - `Buzzsprout`
    - `Simplecast`
    - `WordPress`
- **示例**:
  ```
  Libsyn RSSgen 1.0
  Anchor Podcasts
  (空)
  ```

---

## 📈 字段应用示例

### 示例 1: 查找高质量的中文播客

```sql
SELECT
    id,
    title,
    itunesAuthor,
    description,
    episodeCount,
    popularityScore,
    language
FROM podcasts
WHERE language = 'zh'
  AND dead = 0
  AND episodeCount > 50
  AND popularityScore >= 7
ORDER BY popularityScore DESC, episodeCount DESC
LIMIT 20;
```

**查询结果**:
```
找到 20 个高质量中文播客，按受欢迎程度和单集数量排序
```

### 示例 2: 查找需要更新的播客

```sql
SELECT
    title,
    url,
    lastUpdate,
    newestItemPubdate,
    (strftime('%s', 'now') - newestItemPubdate) / 86400 as daysSinceLastEpisode
FROM podcasts
WHERE dead = 0
  AND popularityScore >= 5
  AND newestItemPubdate < strftime('%s', 'now', '-7 days')
ORDER BY daysSinceLastEpisode DESC
LIMIT 50;
```

**查询结果**:
```
找到 50 个 7 天未更新的高质量播客，优先抓取这些
```

### 示例 3: 统计播客托管平台分布

```sql
SELECT
    host,
    COUNT(*) as count,
    AVG(popularityScore) as avgPopularity,
    AVG(episodeCount) as avgEpisodes
FROM podcasts
WHERE dead = 0
GROUP BY host
ORDER BY count DESC
LIMIT 10;
```

**查询结果**:
```
libsyn.com       | 450,000 | 6.2 | 120
anchor.fm        | 380,000 | 5.8 | 85
buzzsprout.com   | 280,000 | 7.1 | 150
simplecast.com   | 180,000 | 6.9 | 200
soundcloud.com   | 120,000 | 4.5 | 50
```

### 示例 4: 查找失效的 Feed

```sql
SELECT
    title,
    url,
    lastHttpStatus,
    lastUpdate,
    episodeCount
FROM podcasts
WHERE dead = 1
  AND episodeCount > 10
ORDER BY episodeCount DESC
LIMIT 100;
```

**查询结果**:
```
找到 100 个失效但内容丰富的播客，可能需要手动修复
```

---

## 🔗 字段关联关系

```
┌─────────────────────────────────────────────────────────┐
│                    podcasts 表                          │
├─────────────────────────────────────────────────────────┤
│  核心标识                                               │
│  ├─ id (主键)                                          │
│  ├─ url (唯一索引) ←─ 用于查询和更新                    │
│  ├─ originalUrl ←─ 追踪 URL 变更                       │
│  ├─ link ←─ 播客官网                                  │
│  └─ podcastGuid ←─ 跨平台标识                           │
│                                                         │
│  基础信息                                               │
│  ├─ title ←─ 主要显示字段                              │
│  ├─ description ←─ 详细说明                            │
│  ├─ itunesAuthor ←─ 主播名称                           │
│  ├─ itunesOwnerName ←─ 所有者                         │
│  └─ host ←─ 托管平台                                   │
│                                                         │
│  多媒体                                                 │
│  ├─ imageUrl ←─ 封面图片                              │
│  ├─ newestEnclosureUrl ←─ 最新音频                     │
│  ├─ newestEnclosureDuration ←─ 时长（秒）             │
│  └─ contentType ←─ Feed MIME 类型                     │
│                                                         │
│  时间戳                                                 │
│  ├─ lastUpdate ←─ Feed 更新时间                       │
│  ├─ newestItemPubdate ←─ 最新单集                     │
│  ├─ oldestItemPubdate ←─ 最旧单集                     │
│  └─ createdOn ←─ 首次索引时间                         │
│                                                         │
│  状态与质量                                             │
│  ├─ lastHttpStatus ←─ HTTP 状态码                     │
│  ├─ dead ←─ 失效标记 (0/1)                           │
│  ├─ explicit ←─ 成人内容标记 (0/1)                     │
│  └─ chash ←─ 内容哈希（40字符）                        │
│                                                         │
│  统计与分类                                             │
│  ├─ episodeCount ←─ 单集总数                          │
│  ├─ popularityScore ←─ 受欢迎程度 (0-10)              │
│  ├─ priority ←─ 抓取优先级 (0-10)                     │
│  ├─ updateFrequency ←─ 更新频率 (0-10)                │
│  ├─ language ←─ 语言代码 (2字母)                      │
│  └─ category1-10 ←─ iTunes 分类                        │
│                                                         │
│  技术信息                                               │
│  ├─ itunesId ←─ Apple ID                             │
│  ├─ itunesType ←─ episodic/serial                      │
│  └─ generator ←─ Feed 生成器                          │
└─────────────────────────────────────────────────────────┘
```

---

## 💡 使用建议

### 1. 数据质量判断

```python
def is_high_quality_podcast(podcast):
    """判断播客是否高质量"""
    return (
        podcast['dead'] == 0 and                      # Feed 有效
        podcast['episodeCount'] >= 50 and             # 有足够内容
        podcast['popularityScore'] >= 5 and           # 有一定知名度
        podcast['lastHttpStatus'] == 200 and          # HTTP 正常
        (time.time() - podcast['newestItemPubdate']) < 365 * 24 * 3600  # 一年内有更新
    )
```

### 2. 智能抓取调度

```python
def calculate_fetch_priority(podcast):
    """计算抓取优先级"""
    base_score = podcast['popularityScore']

    # 更新频率修正
    if podcast['updateFrequency'] >= 7:
        base_score += 2

    # 最后更新时间修正（超过7天未更新）
    days_since_update = (time.time() - podcast['newestItemPubdate']) / 86400
    if days_since_update > 7:
        base_score += min(3, int(days_since_update / 7))

    # Feed 质量修正
    if podcast['lastHttpStatus'] != 200:
        base_score -= 3

    if podcast['dead'] == 1:
        return -1  # 不抓取

    return min(10, max(0, base_score))
```

### 3. 推荐算法

```python
def recommend_podcasts(user_preferences, limit=20):
    """基于用户偏好推荐播客"""
    query = """
        SELECT
            id, title, description, itunesAuthor,
            episodeCount, popularityScore, language,
            category1, category2, category3
        FROM podcasts
        WHERE dead = 0
          AND episodeCount >= 20
          AND popularityScore >= 4
    """

    # 添加语言过滤
    if user_preferences.get('language'):
        query += f" AND language = '{user_preferences['language']}'"

    # 添加分类过滤
    if user_preferences.get('categories'):
        categories = "', '".join(user_preferences['categories'])
        query += f" AND category1 IN ('{categories}')"

    # 排序和限制
    query += f"""
        ORDER BY
            popularityScore DESC,
            episodeCount DESC,
            newestItemPubdate DESC
        LIMIT {limit}
    """

    return execute_query(query)
```

---

## 📚 相关资源

- **PodcastIndex.org**: https://podcastindex.org/
- **GitHub Repository**: https://github.com/Podcastindex-org/database
- **Podcast Namespace Spec**: https://github.com/Podcastindex-org/podcast-namespace
- **iTunes Podcast Spec**: https://help.apple.com/itc/podcasts_connect/

## 🔗 与本项目的映射关系

```
PodcastIndex.db                     MagicPodcast.db
───────────────────────────────────────────────────────────
url (UNIQUE)           ──────────────► feed_url
title                  ──────────────► title
description            ──────────────► description
itunesAuthor           ──────────────► author
imageUrl               ──────────────► cover_url
episodeCount           ──────────────► episode_count
newestItemPubdate      ──────────────► newest_episode_date
language               ──────────────► (新增字段)
category1-10           ──────────────► (通过 tags 关联)
popularityScore        ──────────────► (新增字段)
itunesId               ──────────────► itunes_id
chash                  ──────────────► podcast_guid
```

**关键映射逻辑**:
1. 使用 `url` 作为唯一标识进行匹配
2. 优先从 PodcastIndex 获取数据（快速、离线）
3. 如果未找到，再通过 `url` 在线抓取 RSS Feed
4. 保留用户的自定义数据（notes, my_rate）
5. 记录 `data_source = "podcastindex"` 或 `"rss"`
