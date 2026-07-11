# MagicPodcast 数据库迁移指南

最后更新：2026-05-31

本文只记录当前仍适用的数据库迁移入口和操作顺序。旧的项目搬家记录已移入 [../archive/reports/PROJECT_LOCATION_MIGRATION_2026-01-21.md](../archive/reports/PROJECT_LOCATION_MIGRATION_2026-01-21.md)，历史 Go 迁移入口已删除，不再作为当前迁移依据。

## 当前自动迁移

后端 API 启动时会自动完成两类数据库准备：

1. 按当前模型结构执行自动迁移。
2. 创建当前列表、单集、工作流和任务执行所需的基础索引。

日常启动请使用项目根目录的脚本入口：

```bash
./scripts/restart.sh --prod
```

不要再使用旧文档里的后端直跑命令或本机路径脚本作为当前启动方式。

## 手动补齐索引

如需为现有数据库补齐性能索引和搜索 FTS 表，使用后端命令入口：

```bash
cd backend
go run ./cmd/add_indexes
```

指定数据库路径：

```bash
cd backend
go run ./cmd/add_indexes ./data/magicpodcast.db
```

索引细节和验证方式见 [../DATABASE_INDEX_GUIDE.md](../DATABASE_INDEX_GUIDE.md)。

## 高风险手工迁移

以下入口会直接改写真数据，不能作为无人值守清理项自动执行：

| 入口 | 风险 | 当前处理 |
| --- | --- | --- |
| `backend/cmd/migrate` | 会重建并替换 `episodes` 表 | 已列入人审队列 |
| `backend/cmd/maint/*` | 多数来自历史数据修复、导入或外部数据补全 | 已列入人审队列 |
| `backend/scripts/fix_newest_episode_date.sql` 和 `backend/scripts/init_tags.sql` | 可能修正或重建真实数据 | 已列入人审队列 |

这些入口的当前说明见 [../../backend/cmd/README.md](../../backend/cmd/README.md) 和 [../../backend/cmd/maint/README.md](../../backend/cmd/maint/README.md)。

## 推荐操作顺序

涉及真实数据库前，按下面顺序执行：

1. 在项目根目录执行 `./scripts/backup-db.sh`。
2. 如果会写同一个数据库，先执行 `./scripts/stop.sh` 停止服务。
3. 运行已经确认过的迁移或维护命令。
4. 执行 `./scripts/verify-db.sh backend/data/magicpodcast.db`。
5. 执行 `./scripts/restart.sh --prod`。
6. 执行 `./scripts/health-check.sh`。

如任一步骤失败，先停止继续写入，再从备份或临时库中复查问题。

## 当前专题文档

- [PODCASTINDEX_DEDUP.md](PODCASTINDEX_DEDUP.md)：当前 PodcastIndex 去重视图入口和验证方式。
