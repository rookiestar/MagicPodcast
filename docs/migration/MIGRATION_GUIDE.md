# MagicPodcast 数据库迁移指南

最后更新：2026-08-30

本文只记录当前仍适用的数据库迁移入口和操作顺序。旧的项目搬家记录已移入 [../archive/reports/PROJECT_LOCATION_MIGRATION_2026-01-21.md](../archive/reports/PROJECT_LOCATION_MIGRATION_2026-01-21.md)。

## 当前版本化迁移

当前 schema 版本为 `25`（与源码 `backend/internal/database/migrate.go` 中 `CurrentSchemaVersion` 一致），版本记录保存在 `schema_migrations`。迁移注册表位于同一文件，每个版本包含名称、说明和事务内的执行函数。当前版本链为：

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
23. `23 episode-video-availability`：为单集增加 `video_availability` 三态列（空/`unknown` / `unavailable` / `available`），供播客详情「看视频」使用；不存签名 HLS（#199）。生产 apply 需单独授权。
24. `24 episode-video-availability-check`：规范 `video_availability` 仅可保存空、`unknown`、`unavailable` 或 `available`；历史未知值归一为空。生产 apply 需单独授权。
25. `25 native-minutes-artifact-integrity`：为不可变产物集新增受管音频、妙记纪要与结构化时间轴摘要；旧 notes 摘要保留且不回填历史产物（#206）。生产 apply 需单独授权。

运行约束（非独立版本号）：

- 缺少部分必需表时拒绝继续，避免把不完整结构伪装成可用版本。
- 迁移和版本记录在同一事务中执行；失败会回滚事务，API 不会启动。
- 普通 API 启动只读检查版本，不自动 apply 迁移。
- 已承载数据的业务表不得使用会递归跟随模型关联的 `AutoMigrate`。修改已有表使用明确的 additive DDL；确需重建时，迁移必须显式声明重建对象、外键影响和数据保持合同。

schema 24→25 的迁移回归使用 `backend/internal/database/testdata/schema24_fixture.sql`。该脱敏 Fixture 的 schema 固定来自提交 `d3e5b81bf193cd1448fe83ed193576f66a5a206a` 的版本化迁移注册表，不由当前模型临时生成；它覆盖 13 条行动队列/个人信号以及 Episode 的直接、间接持久依赖。安全 additive 迁移必须逐行保持这些数据，未声明的 Episode 重建会因级联数据减少而在事务提交前失败。

生产迁移 preflight 必须引用已验证备份。它把备份恢复到临时 SQLite，使用目标代码和同一迁移注册表真实执行全部待迁移版本，并生成不含用户内容或绝对路径的 Migration Report：

```bash
export MAGICPODCAST_MIGRATION_BACKUP=/absolute/path/to/verified-backup.db.gz
./scripts/migrate-db.sh --preflight
```

`--dry-run` 保留为 `--preflight` 的兼容别名，不再只是打印计划。执行 checkout 必须干净且 `HEAD` 等于目标 40 位 commit；未提交源码不能冒充该 commit。报告默认写入被 Git 忽略的 `backend/data/migration-reports/latest.json`，并绑定报告版本、目标 commit、源/目标 schema、备份 SHA-256、源数据库指纹和 pending migration 清单。它同时记录实际 DDL、权威 schema diff、全表行数差异、外键依赖图、受保护数据摘要和每条允许变化合同；未知或未声明 DDL、声明但未发生的 schema 变化、删除、身份丢失或字段改写都会返回非零状态，报告不可用于 apply。

preflight 只读取源数据库，不停止生产服务、不改变 data profile，也不写源数据库。源数据库与备份逻辑指纹不一致时失败关闭，必须重新创建近期备份。

真实数据库应用迁移前，必须先创建并验证近期备份、完成通过的 preflight，并用目标 commit 执行 `release.sh --prepare` 生成未安装的干净 release stage。数据库 apply 与该 stage 的生产切换仍需分别授权；`migrate-db.sh --apply` 只在两项授权都具备时进入一个共享维护窗口，停止服务并确认 3000/8080 已释放：

```bash
./scripts/backup-db.sh
./scripts/verify-db.sh backend/data/magicpodcast.db
export MAGICPODCAST_MIGRATION_BACKUP=/absolute/path/to/verified-backup.db.gz
export MAGICPODCAST_MIGRATION_REPORT=backend/data/migration-reports/latest.json
./scripts/migrate-db.sh --preflight

./scripts/release.sh --prepare
export MAGICPODCAST_MIGRATION_RELEASE_STAGE=/absolute/path/from/prepared_stage

export MAGICPODCAST_MIGRATION_CONFIRM=I_UNDERSTAND_THIS_WRITES_DATA
export MAGICPODCAST_MIGRATION_RELEASE_CONFIRM=I_UNDERSTAND_THIS_SWITCHES_RELEASE
./scripts/migrate-db.sh --apply
./scripts/verify-db.sh backend/data/magicpodcast.db
```

`--apply` 只消费仍有效且通过的 Migration Report。目标 commit、干净 checkout、备份 SHA/元信息、源 schema、源数据库指纹、pending migrations 或目标 release 产物任一漂移都会拒绝写库。允许变化按每行旧值、新值、操作和条件验证；迁移、版本记录、preflight 重放对账和数据合同在同一事务内执行，失败会完整回滚。提交后再核对 SQLite integrity、foreign key、目标 schema、preflight 差异和行动/加工投影。

迁移与 deploy、rollback、recovery 共用 `/tmp/magicpodcast-production-deploy.lock`。窗口记录 owner、operation、开始时间、heartbeat 和 PID 启动时间；停服后先进入不可自动回收的 `critical` 状态，owner 即使被强杀也只能由显式 `recovery` 原地接管，期间锁目录始终存在，Supervisor 不会自动拉起未验收数据库。事务提交后，脚本通过 release 模块切换预先验证的目标 stage；该 stage 必须由干净 worktree 构建，manifest commit 等于目标 commit，后端哈希和前端 BUILD_ID 均匹配。随后通过 `/ready` 精确验证 release、frontend build、schema、`production` data profile，再读取行动队列和加工投影，之后才释放窗口。

事务、报告回读、备份重验、连接安全恢复、启动或启动后验收任一失败时，命令明确报告数据库是否已提交，服务保持停止，锁转为 `recovery_required`。恢复操作者检查 Migration Report 和配对备份后，通过 `restore-db.sh` 以同一 `recovery` 窗口接管；不得删除锁来绕过恢复判断。

普通 API 启动只读检查版本、必需表和 SQLite 运行参数，不再静默执行结构迁移；普通 deploy 也不会触发 migration apply：

```bash
./scripts/restart.sh --prod
```

如果迁移失败，先保持服务停止并读取 Migration Report 的 `database_committed`、失败代码、计划和备份摘要。未提交可修正后重新 preflight；已提交但验收失败必须先判断是继续验证还是按配对备份恢复。不要让 API 使用未验收结构启动。

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
| `backend/cmd/migrate` | `--preflight`（`--dry-run` 兼容别名）在备份副本生成 Migration Report；`--apply` 写真实库 | 真实库仍需按本文顺序执行，并单独授权 apply |
| `backend/cmd/maint/*` | 多数来自历史数据修复、导入或外部数据补全 | 已列入人审队列 |
| `backend/scripts/fix_newest_episode_date.sql` 和 `backend/scripts/init_tags.sql` | 可能修正或重建真实数据 | 已列入人审队列 |

这些入口的当前说明见 [../../backend/cmd/README.md](../../backend/cmd/README.md) 和 [../../backend/cmd/maint/README.md](../../backend/cmd/maint/README.md)。

## 推荐操作顺序

涉及真实数据库前，按下面顺序执行：

1. 在项目根目录执行 `./scripts/backup-db.sh`，并保存它输出的备份路径。
2. 用 `./scripts/verify-db.sh <备份路径>` 验证备份可恢复。
3. 用已验证备份运行 `./scripts/migrate-db.sh --preflight`，确认 Migration Report 通过且输入未漂移。
4. 取得真实数据库写授权后设置确认字符串，用同一备份和报告执行 `--apply`；脚本会在共享维护窗口内停止并重启既有生产产物。
5. 复核报告的提交状态、schema、受保护数据和启动后投影，再执行 `./scripts/verify-db.sh backend/data/magicpodcast.db` 与 `./scripts/health-check.sh`。

如任一步骤失败，先停止继续写入，再从备份或临时库中复查问题。

## 当前专题文档

- [PODCASTINDEX_DEDUP.md](PODCASTINDEX_DEDUP.md)：当前 PodcastIndex 去重视图入口和验证方式。
- [PRODUCTION_MIGRATION_SAFETY_DRILL.md](PRODUCTION_MIGRATION_SAFETY_DRILL.md)：#219 的 `ready-for-human` 生产迁移门禁演练、授权停点与证据模板。
