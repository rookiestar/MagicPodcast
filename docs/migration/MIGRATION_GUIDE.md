# MagicPodcast 数据库迁移指南

最后更新：2026-08-25

本文只记录当前仍适用的数据库迁移入口和操作顺序。旧的项目搬家记录已移入 [../archive/reports/PROJECT_LOCATION_MIGRATION_2026-01-21.md](../archive/reports/PROJECT_LOCATION_MIGRATION_2026-01-21.md)。

## 当前版本化迁移

当前 schema 版本为 `22`（与源码 `backend/internal/database/migrate.go` 中 `CurrentSchemaVersion` 一致），版本记录保存在 `schema_migrations`。迁移注册表位于同一文件，每个版本包含名称、说明和事务内的执行函数。当前版本链为：

1. `1 baseline-current-model`：空数据库创建当前模型表和索引；已有且完整的数据库只记录 baseline。
2. `2 feed-access-observability`：记录 Feed HTTP 状态、错误类别、耗时、缓存和出口等观测字段。
3. `3 feed-snapshot-retrieved-at`：记录执行实际使用内容的取回时间。
4. `4 feed-circuit-state`：记录域名断路的打开、跳过和探测状态。
5. `5 feed-source-verification`：记录实际使用的 Feed 来源和 PodcastIndex 身份校验结果。
6. `6 scheduler-run-history`：创建调度运行历史表，供连续失败观测使用。
7. `7 feed-snapshots-last-good`：创建有界 `feed_snapshots` 表，持久化 Feed 快照与验证器，供重启后的 304 恢复和诊断；迁移历史名称保留 `last-good`，但按 #35 普通失败后的快照命中不计作本批成功。
8. `8 podcast-alternative-feeds`：按节目 + 当前主 Feed + 稳定身份缓存已验证替代 Feed（或不可用原因）；只服务当前批次，不永久改写主 Feed（#37）。生产 apply 需单独授权。
9. `9 job-feed-attempts`：按 Job 追加安全的 Feed 尝试元数据（来源、序号、HTTP 状态、错误类别、失败阶段、重试决定、身份验证），不含正文/凭据；JobExecution 仍只保存最终结果（#39）。
10. `10 job-compensation-links`：partial Job 与「仅重试失败 Feed」补偿 Job 的双向关联字段（#40）。生产 apply 需单独授权。
11. `11 job-execution-failure-phase`：JobExecution 终态投影增加 `feed_failure_phase`（dns/connect/tls/response_header/body_read），供尝试链展示（#39）。
12. `12 single-active-workflow-job`：为每个工作流的 pending/running/finalizing Job 增加部分唯一索引，避免并发重复执行（#38）。
13. `13 feed-user-agent-gates`：创建 `feed_user_agent_gates`，按域名和 User-Agent 单向指纹持久化明确 UA ACL 阻断及恢复元数据（#48）；生产 apply 需单独授权。
14. `14 feed-user-agent-recovery`：为 UA 阻断增加人工探测审批字段、审批审计表、不同 Feed 的渐进恢复记录，并把恢复状态写入 `job_executions` 与 `job_feed_attempts`（#49）；生产 apply 需单独授权。
15. `15 episode-triage-decisions`：为个人库单集建立唯一的发现判断记录，保存 pending、shortlisted 与 discarded 状态（#55）。
16. `16 homepage-workflow-reports`：为工作流与报告增加首页发布、报告类型、工作流名称和结构化单集字段（#89/#90）。
17. `17 episode-consumption-state`：把单集判断扩展为跨日 Inbox、Focus、Someday、Done、进行中与已读状态；历史 shortlisted 迁入 Inbox，discarded 保留为不感兴趣（#101/#102）。生产 apply 需单独授权。
18. `18 consumption-queue-order`：为四条消费队列增加独立位置和版本；按升级前的稳定可见顺序回填，供刷新、跨设备一致的精确排序使用（#157）。生产 apply 需单独授权。
19. `19 episode-completion-facts`：为每个单集建立唯一完成事实；只从当前 Done 且具有既有队列更新时间的记录回填，缺少时间则预检失败（#168/#169）。生产 apply 需单独授权。
20. `20 episode-processing-foundation`：新增独立的单集加工运行、可恢复检查点、不可变产物集与逐目标知识交付记录；数据库约束保证每集最多一个活动运行、一个当前产物集及终态不可回退（#179）。生产 apply 需单独授权。
21. `21 managed-episode-audio-assets`：新增受管音频准备状态，记录本地可用性而不保存源 URL 或暴露本地路径（#181）。生产 apply 需单独授权。
22. `22 focus-processing-schedule-history`：新增 Focus 定时加工的幂等触发与候选结果记录，不复用 Feed 工作流调度（#182）。生产 apply 需单独授权。

运行约束（非独立版本号）：

- 缺少部分必需表时拒绝继续，避免把不完整结构伪装成可用版本。
- 迁移和版本记录在同一事务中执行；失败会回滚事务，API 不会启动。
- 普通 API 启动只读检查版本，不自动 apply 迁移。

显式查看迁移计划：

```bash
./scripts/migrate-db.sh --dry-run
```

真实数据库应用迁移前，必须先创建并验证近期备份、停止使用该数据库的服务，然后显式确认：

```bash
./scripts/backup-db.sh
./scripts/verify-db.sh backend/data/magicpodcast.db
./scripts/stop.sh

export MAGICPODCAST_MIGRATION_CONFIRM=I_UNDERSTAND_THIS_WRITES_DATA
export MAGICPODCAST_MIGRATION_BACKUP=/absolute/path/to/verified-backup.db.gz
./scripts/migrate-db.sh --apply
./scripts/verify-db.sh backend/data/magicpodcast.db
```

`--apply` 缺少确认字符串、备份路径、配置文件或数据库文件，或发现 3000/8080 仍有服务监听时，会拒绝执行。普通 API 启动只读检查版本、必需表和 SQLite 运行参数，不再静默执行结构迁移：

```bash
./scripts/restart.sh --prod
```

如果迁移失败，先保持服务停止，使用迁移前备份恢复到临时库或生产库，再重新执行完整验证；不要让 API 使用半完成结构启动。

## 代码回退与数据库配对

数据库迁移是单向追加的，旧版本后端不会自动兼容更高的 schema。发布脚本会把 schema 版本写入发布元数据，并在回退前比较旧版本要求与当前数据库版本；缺少版本信息或两者不一致时，脚本会在停止服务前拒绝回退。

因此，涉及 schema 迁移的回退必须同时准备迁移前的已验证数据库备份。先将备份恢复到临时库并通过 `verify-db.sh`，确认它与旧版本产物配对后，再在明确的维护窗口中恢复数据库并执行代码回退。不要只执行 `./scripts/release.sh --rollback`，也不要让旧版本直接连接更高版本的线上数据库。

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

## 旧命令边界

以下入口会直接改写真数据，不能作为无人值守清理项自动执行：

| 入口 | 风险 | 当前处理 |
| --- | --- | --- |
| `backend/cmd/migrate` | 当前只接受 `--dry-run` / `--apply`，并强制要求确认字符串和已验证备份 | 真实库仍需按本文顺序执行 |
| `backend/cmd/maint/*` | 多数来自历史数据修复、导入或外部数据补全 | 已列入人审队列 |
| `backend/scripts/fix_newest_episode_date.sql` 和 `backend/scripts/init_tags.sql` | 可能修正或重建真实数据 | 已列入人审队列 |

这些入口的当前说明见 [../../backend/cmd/README.md](../../backend/cmd/README.md) 和 [../../backend/cmd/maint/README.md](../../backend/cmd/maint/README.md)。

## 推荐操作顺序

涉及真实数据库前，按下面顺序执行：

1. 在项目根目录执行 `./scripts/backup-db.sh`，并保存它输出的备份路径。
2. 用 `./scripts/verify-db.sh <备份路径>` 验证备份可恢复。
3. 如果会写同一个数据库，先执行 `./scripts/stop.sh` 停止服务。
4. 先运行 `./scripts/migrate-db.sh --dry-run`，再设置确认字符串和备份路径执行 `--apply`。
5. 执行 `./scripts/verify-db.sh backend/data/magicpodcast.db`，并检查 schema 版本和核心数据。
6. 执行 `./scripts/restart.sh --prod` 与 `./scripts/health-check.sh`。

如任一步骤失败，先停止继续写入，再从备份或临时库中复查问题。

## 当前专题文档

- [PODCASTINDEX_DEDUP.md](PODCASTINDEX_DEDUP.md)：当前 PodcastIndex 去重视图入口和验证方式。
