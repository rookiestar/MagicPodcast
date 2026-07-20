# 小宇宙替代 Feed 候选核验

日期：2026-07-20
状态：候选研究与接入边界；未修改生产数据库、主 Feed URL 或生产网络

## 结论

本轮从 Job 700 涉及的 9 个节目中，确认 2 个可进入“已验证替代源”候选的 Feed：

| MagicPodcast 节目 | 生产 ID | 候选 Feed | 稳定身份 | 内容证据 | 结论 |
| --- | ---: | --- | --- | --- | --- |
| TIANYU2FM — 对谈未知领域 | 85 | `http://www.ximalaya.com/album/40320716.xml` | Apple `collectionId=1525369049`；PodcastIndex 同一 iTunes ID | 标题、作者一致；Feed 返回 167 集，和主库已保存的 E149、E147 等节目内容对应 | 可进入候选 |
| 科学星球 | 195 | `http://www.ximalaya.com/album/21469108.xml` | Apple `collectionId=1612954022`；PodcastIndex 同一 iTunes ID | 标题一致；候选前 10 集中至少 8 个主题/标题与主库已保存单集对应，作者为同一节目作者的目录写法 | 可进入候选 |

其余 7 个节目目前没有在 PodcastIndex 中发现第二个非小宇宙 Feed，Apple 目录结果也仍指向原小宇宙 Feed。因此它们继续保持：主 Feed → 域名断路 → last-good；不因同名搜索结果自动切换。

## 生产样本

生产主库：

`/Users/rookiestar/VSCode/Projects/MagicPodcast/backend/data/magicpodcast.db`

Job 700 的 9 个节目及当前身份字段如下：

| ID | 节目 | 作者 | 主 Feed |
| ---: | --- | --- | --- |
| 85 | TIANYU2FM — 对谈未知领域 | TIANYU2FM | `https://feed.xyzfm.space/m3rxnd7cqa86` |
| 89 | 日知录 | 日谈公园 | `https://feed.xyzfm.space/6fqtp9h4u8vm` |
| 195 | 科学星球 | 孙彬_BIMBOX | `https://feed.xyzfm.space/6an4ykjwf3vh` |
| 203 | IQ老友说 | IQVIA艾昆纬 | `https://feed.xyzfm.space/v7yueq3enqha` |
| 211 | 别想好 | Wealden | `https://feed.xyzfm.space/jb6guvbebm7f` |
| 214 | ATGC doctors' chat | 雅娴，莲燕，王祎 | `https://feed.xyzfm.space/dryajkkmhkny` |
| 363 | 自然选择NaturalSelection | 自然选择播客 | `https://feed.xyzfm.space/98t4c67awp76` |
| 409 | 果壳时间 | 果壳 | `https://feed.xyzfm.space/6bxx4bm3u78e` |
| 472 | 追问周发现｜AI+脑科学新知 | 开始追问 | `https://feed.xyzfm.space/dna4l3t37jvk` |

这 9 条记录当前的 `i_tunes_id` 和 `podcast_guid` 均为空。不能把候选目录的身份直接写入主库而不留证据，因此候选激活需要单独的、可回滚的数据变更。

## 候选一：TIANYU2FM

### 身份与目录证据

- Apple Search API 精确搜索返回唯一结果：
  `https://itunes.apple.com/search?term=TIANYU2FM%20%E5%AF%B9%E8%B0%88%E6%9C%AA%E7%9F%A5%E9%A2%86%E5%9F%9F&entity=podcast&country=cn&limit=20`
- Apple 返回节目名 `TIANYU2FM — 对谈未知领域`、作者 `TIANYU2FM`、`collectionId=1525369049`，且 `feedUrl` 是生产主 Feed。
- PodcastIndex `podcasts` 原表中有一条非小宇宙候选：
  `http://www.ximalaya.com/album/40320716.xml`，iTunes ID 同为 `1525369049`，`dead=0`，`lastHttpStatus=200`，167 集。
- 同一标题下另有 `https://feeds.fireside.fm/tianyu2/rss`。它可访问且内容相同，但没有 iTunes ID，Podcast GUID 也与 Ximalaya 记录不同；在主库身份尚未补齐前不自动在两者之间选择。补齐 iTunes ID 后，运行时只会按稳定身份命中 Ximalaya 条目，不会把 Fireside 条目当作同身份候选。

### Feed 实测

2026-07-20 从生产机直连、使用 `MagicPodcast/1.0`、单次 GET：

| URL | HTTP | 类型 | 大小 | 解析结果 |
| --- | ---: | --- | ---: | --- |
| `http://www.ximalaya.com/album/40320716.xml` | 200 | `application/xml` | 220,784 bytes | 标题/作者匹配，167 集，最新为 E154 |
| `https://feeds.fireside.fm/tianyu2/rss` | 200 | `application/xml` | 221,168 bytes | 标题/作者匹配，167 集，最新为 E154 |

主库已有 163 集，最新为 E150。Ximalaya Feed 的 E149、E147 与主库标题一致，E150、E148、E146 的期号、主题和嘉宾也对应；Feed 平台不同导致单集 GUID 不同，不能只用 GUID 做匹配。

## 候选二：科学星球

### 身份与目录证据

- Apple Search API 精确搜索返回唯一结果：
  `https://itunes.apple.com/search?term=%E7%A7%91%E5%AD%A6%E6%98%9F%E7%90%83%20%E5%AD%99%E5%BD%AC&entity=podcast&country=cn&limit=20`
- Apple 返回节目名 `科学星球`、作者 `BOX孙彬`、`collectionId=1612954022`，`feedUrl` 为 `http://www.ximalaya.com/album/21469108.xml`。
- PodcastIndex 原表中存在同一 URL、同一 iTunes ID 的记录，`dead=0`，`lastHttpStatus=200`，94 集。
- 生产库作者字段为 `孙彬_BIMBOX`，与候选目录的 `BOX孙彬` 是目录显示差异；标题完全一致，不能单凭作者字符串判为冲突。

### Feed 实测

2026-07-20 从生产机直连、使用 `MagicPodcast/1.0`、单次 GET：

| URL | HTTP | 类型 | 大小 | 解析结果 |
| --- | ---: | --- | ---: | --- |
| `http://www.ximalaya.com/album/21469108.xml` | 200 | `application/xml` | 40,786 bytes | 标题匹配，作者 `BOX孙彬`，94 集，最新为 106 期 |

主库已有 101 集，最新为 114 期。候选前 10 集中，“两代诺奖女神”“巴西真实核灾难”“春节历法”“黄金”“月亮/爱因斯坦与玻尔”“数学怪兽”“无线通信”“刨根问底”等内容与主库已保存单集对应；候选平台的期号与主库期号存在重编号，故运行时保留标题/作者与单集证据双重校验。

## 其余 7 个节目

PodcastIndex 按生产主 Feed、生产库已知身份和精确标题检查后，没有发现可安全切换的第二个非小宇宙 Feed。Apple Search API 的唯一结果仍为小宇宙主 Feed：

| 节目 ID | Apple collectionId | Apple Feed URL |
| ---: | ---: | --- |
| 89 | 1527514372 | `https://feed.xyzfm.space/6fqtp9h4u8vm` |
| 203 | 1641869549 | `https://feed.xyzfm.space/v7yueq3enqha` |
| 211 | 1625636114 | `https://feed.xyzfm.space/jb6guvbebm7f` |
| 214 | 1629435270 | `https://feed.xyzfm.space/dryajkkmhkny` |
| 363 | 1734924080 | `https://feed.xyzfm.space/98t4c67awp76` |
| 409 | 1745853892 | `https://feed.xyzfm.space/6bxx4bm3u78e` |
| 472 | 1803031246 | `https://feed.xyzfm.space/dna4l3t37jvk` |

这些结果能补充主 Feed 的稳定身份，但没有产生非小宇宙替代源；它们仍必须依赖断路和 last-good 保护。

## 接入边界

1. 不修改 9 个节目的主 `feed_url`；替代源只在主 Feed 失败或被断路时尝试。
2. 只把有稳定 iTunes ID/GUID 且有标题、作者或单集证据的候选交给现有 PodcastIndex fallback。
3. 仍按主 Feed → 已验证替代 Feed → last-good 的顺序执行。替代 Feed 失败后直接进入 last-good，不增加重试、不换出口。
4. 断路作用于 `feed.xyzfm.space`，不会因为候选来自其他域名而被绕过；替代请求仍通过同一个 FeedFetcher 和现有观测链路。
5. 候选身份激活应先备份主库，只补 85 和 195 的稳定 ID，保留更新前值；7 个没有候选的节目不写入猜测值。
6. 如果候选 Feed 后续出现解析失败、身份冲突或内容证据不足，立即停止选择该候选，恢复 last-good；不扩大候选池。

## 限制与未证明事项

- 本轮验证的是单次低频可达性，不证明候选 Feed 永久可用。
- Apple Search API 是目录身份来源，不是对 Feed 长期可用性的承诺。
- Ximalaya 与 Fireside 的单集 GUID 由各平台生成，不能要求跨平台相等。
- 其余 7 个节目没有找到安全替代源，不应为了覆盖率采用标题相似或搜索排名切换。
- 未部署代理、固定出口、住宅代理、共享代理池或浏览器伪装；未增加生产重试。

## 研究来源

1. 生产主库只读查询：`/Users/rookiestar/VSCode/Projects/MagicPodcast/backend/data/magicpodcast.db`。
2. PodcastIndex 官方批量 SQLite 数据库：`/Users/rookiestar/VSCode/Projects/MagicPodcast/backend/data/podcastindex_feeds.db`；候选筛选读取 `podcasts` 原表，保留重复身份以避免去重视图隐藏冲突。
3. Apple Podcasts Search API：`https://itunes.apple.com/search`，`entity=podcast`、`country=cn`、`limit=20`，逐节目精确查询。
4. 候选 Feed 本身：`http://www.ximalaya.com/album/40320716.xml`、`https://feeds.fireside.fm/tianyu2/rss`、`http://www.ximalaya.com/album/21469108.xml`。
5. 运行时安全边界与回退顺序：`backend/internal/sync/alternative_feed.go`、`backend/internal/sync/episode_sync.go`、`backend/internal/feed/coordinator.go`。
