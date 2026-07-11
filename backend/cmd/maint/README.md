# 维护命令说明

最后更新：2026-05-31

这些命令主要来自历史数据清理、标签导入、PodcastIndex 数据补全和临时排查。它们不是产品运行的必需入口，很多命令会写数据库或依赖本机文件，因此本轮只做梳理，不自动删除。

## 只读或偏检查类

| 命令 | 作用 | 注意 |
| --- | --- | --- |
| `check_empty_podcasts` | 检查播客数据是否缺少必要字段 | 读取主数据库 |
| `check_excel` | 检查历史 Excel 标签文件 | 依赖 `/Users/rookiestar/Downloads/热门节目+热门播客.xlsx` |
| `db_stats` | 输出数据库统计信息 | 读取主数据库 |
| `match_podcast_tags` | 根据 Excel 生成标签匹配报告 | 依赖本机 Excel 文件，并会生成 `match_report.txt` |
| `read_excel` | 从历史 Excel 输出标签 SQL | 依赖本机 Excel 文件，只输出 SQL |
| `test_itunesid_fix` | 验证 iTunes ID 修复逻辑 | 偏临时测试工具 |

## 会写数据库或外部文件

| 命令 | 作用 | 风险 |
| --- | --- | --- |
| `add_indexes` | 为 PodcastIndex 本地库加索引 | 默认目标是 `data/podcastindex_feeds.db`，不是主业务库 |
| `backfill_podcastindex_data` | 回填 PodcastIndex 相关字段 | 会更新主数据库 |
| `delete_invalid_podcasts` | 删除无效播客 | 会永久删除数据 |
| `fetch_xiaoyuzhou_cover` | 抓取并写入小宇宙封面 | 会更新主数据库，依赖网络结果 |
| `fix_title_128` / `fix_title_393` / `fix_title_484` / `fix_title_487` | 修复指定播客标题 | 面向特定历史数据 |
| `import_tags` / `import_tags_with_relations` | 从 Excel 重建标签和播客标签关系 | 会清空并重建标签相关表 |
| `init_db` | 初始化数据库和默认标签 | 会写主数据库 |
| `insert_podcast_tags` | 从 Excel 写入播客标签关系 | 依赖本机 Excel 文件，会写主数据库 |
| `insert_test_data` | 插入测试播客和单集 | 会写主数据库 |
| `repair_dirty_data` | 修复脏数据并可生成报告 | 默认 dry-run；加 `--apply` 后会写数据库 |
| `update_episode_covers` | 用播客封面补齐单集封面 | 会更新主数据库 |
| `update_podcast_metadata` | 更新播客元数据 | 会更新主数据库，依赖外部数据 |

## 人审建议

1. 保留真正会定期使用的维护命令，并补充参数化入口。
2. 将只服务于一次性历史修复的命令迁入归档或删除。
3. 删除或替换硬编码的本机文件路径。
4. 对所有写入命令补齐备份、dry-run、影响范围报告和确认提示。
