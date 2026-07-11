# MagicPodcast 数据库索引指南

最后更新：2026-05-31

本文记录当前索引入口和验证方式。旧的“手动创建 `backend/scripts/migrations/add_performance_indexes.sql`”说明已过时；该脚本现在已经存在于仓库中。

## 索引入口

| 入口 | 用途 | 说明 |
| --- | --- | --- |
| `backend/internal/database/migrate.go` | 应用启动时创建基础索引 | `cmd/api` 启动后会执行 `database.CreateIndexes` |
| `backend/scripts/migrations/add_performance_indexes.sql` | 通用性能索引 | 覆盖播客列表、单集列表、工作流、标签关联、任务和报告查询 |
| `backend/scripts/migrations/add_search_fts.sql` | 搜索 FTS 索引 | 覆盖英文和数字类搜索快路径 |
| `backend/cmd/add_indexes` | 手动应用性能索引和搜索索引 | 会按顺序执行上面两个 SQL 文件 |

## 自动创建的基础索引

后端启动时会创建这些关键索引：

- 播客：`xyz_id`、订阅状态与最新单集日期、最近更新、添加日期、单集数量、标题排序。
- 单集：`podcast_id`、播客内发布日期排序、稳定分页排序。
- 工作流：启用状态、更新时间、下次执行时间。
- 任务：工作流关联。
- 任务执行：任务关联、状态。

这部分随服务启动自动执行，不需要单独手工运行。

## 手动补齐性能索引

如需对现有数据库补齐所有性能索引和搜索索引：

```bash
cd backend
go run ./cmd/add_indexes
```

指定数据库路径：

```bash
cd backend
go run ./cmd/add_indexes ./data/magicpodcast.db
```

执行前回到项目根目录先备份：

```bash
./scripts/backup-db.sh
```

## 验证索引

查看当前索引：

```bash
sqlite3 backend/data/magicpodcast.db \
  "SELECT name, tbl_name FROM sqlite_master WHERE type = 'index' ORDER BY tbl_name, name;"
```

查看某条查询是否使用索引：

```bash
sqlite3 backend/data/magicpodcast.db \
  "EXPLAIN QUERY PLAN SELECT * FROM podcasts ORDER BY COALESCE(newest_episode_date, created_at) DESC, id DESC LIMIT 20;"
```

## 性能复测

索引相关改动后，至少运行：

```bash
(cd backend && go test ./...)
node scripts/performance-audit.mjs \
  --base-url http://localhost:3000 \
  --api-url http://localhost:8080 \
  --runs 3
```

搜索索引相关改动还应运行：

```bash
(cd backend && go test ./internal/services)
(cd backend && go test -race ./internal/services)
```

## 注意事项

- 索引能提升读取性能，但会增加少量写入成本。
- 不要在未备份的真实数据库上做不可逆试验。
- `backend/cmd/maint/add_indexes` 面向 PodcastIndex 外部数据库，不是 MagicPodcast 主库索引入口。
