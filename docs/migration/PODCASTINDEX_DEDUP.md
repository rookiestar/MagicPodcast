# PodcastIndex 去重视图指南

最后更新：2026-05-31

MagicPodcast 可选接入本地 PodcastIndex SQLite 数据库。导入 OPML 时，后端会优先通过 `v_unique_podcasts` 视图查找同一个 RSS 源的最佳记录，再回填播客标题、作者、封面、单集数等元数据。

## 当前入口

| 文件 | 用途 |
| --- | --- |
| [../../scripts/create_unique_podcasts_view.sql](../../scripts/create_unique_podcasts_view.sql) | 创建 `v_unique_podcasts` 去重视图 |
| [../../scripts/view_schema.sql](../../scripts/view_schema.sql) | 查看 PodcastIndex 原表、视图和索引 |
| [../../backend/internal/podcastindex/query.go](../../backend/internal/podcastindex/query.go) | 后端查询 `v_unique_podcasts` 的实现 |
| [../../backend/configs/config.example.yaml](../../backend/configs/config.example.yaml) | `podcast_index.path` 配置示例 |

旧的视图 Schema 长文已移入 [../archive/reports/UNIQUE_PODCASTS_VIEW_SCHEMA_2026-01-21.md](../archive/reports/UNIQUE_PODCASTS_VIEW_SCHEMA_2026-01-21.md)，只作历史参考。

## 配置路径

当前示例配置中的 PodcastIndex 路径为：

```yaml
podcast_index:
  path: "./data/podcastindex_feeds.db"
```

该数据库是可选依赖。如果路径为空或初始化失败，后端会继续使用在线抓取流程，不会阻止主服务启动。

## 创建视图

在后端目录执行：

```bash
cd backend
sqlite3 ./data/podcastindex_feeds.db < ../scripts/create_unique_podcasts_view.sql
```

查看视图和索引：

```bash
cd backend
sqlite3 ./data/podcastindex_feeds.db < ../scripts/view_schema.sql
```

执行前请确认目标是 PodcastIndex 外部数据库，不是 MagicPodcast 主业务库 `./data/magicpodcast.db`。

## 去重规则

`v_unique_podcasts` 会按播客标题分组，并为每个标题选出一条最优记录。当前排序优先级为：

1. `dead = 0` 的未失效记录优先。
2. `lastHttpStatus = 200` 的可访问记录优先。
3. `explicit` 不为空的记录优先。
4. `priority = -1` 视为最低优先级，其余值越小越优先。
5. `newestItemPubdate` 越新越优先。
6. `episodeCount` 越大越优先。
7. `popularityScore` 越高越优先。
8. `id` 越小越优先，作为最终稳定排序。

这些规则只影响 PodcastIndex 外部数据的候选记录选择，不会直接修改 MagicPodcast 主数据库。

## 后端查询字段

后端当前从视图读取这些字段：

| 字段 | 说明 |
| --- | --- |
| `id` | PodcastIndex 记录 ID |
| `title` | 播客标题 |
| `itunesAuthor` | 作者 |
| `description` | 描述 |
| `imageUrl` | 封面 |
| `url` | RSS Feed URL |
| `itunesId` | iTunes ID |
| `language` | 语言 |
| `link` | 官网 |
| `newestEnclosureUrl` | 最新单集音频 |
| `newestEnclosureDuration` | 最新单集时长 |
| `lastUpdate` | Feed 更新时间 |
| `newestItemPubdate` | 最新单集发布时间 |
| `oldestItemPubdate` | 最旧单集发布时间 |
| `popularityScore` | 受欢迎程度 |
| `priority` | 抓取优先级 |
| `updateFrequency` | 更新频率 |
| `episodeCount` | 单集数量 |
| `dead` | 是否失效 |
| `lastHttpStatus` | 最后 HTTP 状态码 |
| `explicit` | 内容分级 |

注意字段大小写应与 SQL 脚本保持一致，尤其是 `lastHttpStatus`。

## 验证查询

```sql
SELECT COUNT(*) FROM v_unique_podcasts;

SELECT id, title, url, dead, lastHttpStatus, newestItemPubdate, episodeCount
FROM v_unique_podcasts
WHERE title = '无聊斋';
```

如查询失败，先确认：

1. `podcast_index.path` 指向的是存在的 PodcastIndex 数据库。
2. 目标数据库中存在原始 `podcasts` 表。
3. 已对该数据库执行 [../../scripts/create_unique_podcasts_view.sql](../../scripts/create_unique_podcasts_view.sql)。
4. 字段名仍与脚本和后端查询一致。
