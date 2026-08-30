# 生产迁移安全门禁演练

最后更新：2026-08-30

状态：`ready-for-human`。本文只准备 #219 的人工演练，不构成 Mac mini 访问、真实备份读取、服务停启、migration apply、部署或恢复授权。

## 进入条件

以下条件缺一即停止：

1. #216、#217、#218 的目标提交已合并到 `main`，远端 CI 成功；本地未提交实现不满足这一条件。
2. Mac mini checkout、目标 40 位 SHA、工作树、当前 release/schema、Supervisor 和备份任务已获准只读核对。
3. 已确认演练窗口、负责人、回退代码/schema 配对和恢复用备份。
4. 真实备份创建/读取、停启服务、apply、部署、恢复、Issue 评论/关闭分别取得对应授权；不得用一项授权替代另一项。

Issue 证据只记录 commit、schema、plan ID、备份文件名/SHA 前缀、计数、状态、耗时和跳过原因。不得粘贴绝对路径、标题、笔记、URL、环境变量或凭据。

## 1. 只读基线

取得 Mac mini 只读访问授权后，在终端本地设置生产目录变量；不要把变量值复制到 Issue：

```bash
cd "$PRODUCTION_DIR"
git status --short --untracked-files=all
git rev-parse HEAD
git branch --show-current
git log -1 --oneline
curl --fail --silent http://127.0.0.1:8080/health
lsof -nP -iTCP:3000 -sTCP:LISTEN
lsof -nP -iTCP:8080 -sTCP:LISTEN
sed -n '1,20p' logs/supervisor.status
./scripts/offsite-status.sh
```

通过标准：checkout 干净，HEAD 等于已通过 CI 的目标 SHA；`/health` 的 release/schema、`build_mode=release`、`data_profile=production` 配对；服务监听者和 Supervisor 状态可解释。任何偏差先停止，不执行清理、reset 或修复。

## 2. 近期备份与安全 preflight

以下步骤需要真实备份创建/读取授权，但不停止服务、不写生产库：

```bash
./scripts/backup-db.sh

backup_path="$(find backend/data/backups -maxdepth 1 -type f -name 'magicpodcast_*.db.gz' -print | sort | tail -n 1)"
./scripts/verify-db.sh "$backup_path"

export MAGICPODCAST_MIGRATION_BACKUP="$backup_path"
export MAGICPODCAST_MIGRATION_REPORT=backend/data/migration-reports/production-drill-safe.json
export MAGICPODCAST_TARGET_COMMIT="$(git rev-parse HEAD)"
./scripts/migrate-db.sh --preflight
```

preflight 因后台新写入导致 `source_backup_drift` 时，重新创建近期备份并重跑；不得修改报告或跳过指纹。安全报告必须为 `apply_eligible=true`，且受保护数据前后身份、内容摘要和计数一致。

### 破坏性事故重放

这一步只在同一备份生成的临时副本注入 Episode 父表重建；测试入口不具备生产写路径。需要真实备份读取授权：

```bash
export MAGICPODCAST_AUTHORIZED_DRILL_BACKUP="$backup_path"
export MAGICPODCAST_AUTHORIZED_DRILL_TARGET_COMMIT="$(git rev-parse HEAD)"
export MAGICPODCAST_AUTHORIZED_DRILL_REPORT=backend/data/migration-reports/production-drill-destructive.json

(cd backend && go test ./internal/database \
  -run '^TestAuthorizedBackupDestructiveMigrationDrill$' \
  -count=1 -v)
```

通过标准：命令以“破坏性迁移被门禁拒绝”结束；报告为不可 apply，包含未声明 Episode 重建或其外键后代数据减少；临时库仍为源 schema。测试不得输出用户内容或绝对路径。

## 3. 维护窗口演练（不 apply）

本节会真实停启服务，必须另行授权。它只验证共享窗口和 Supervisor 协调，不写数据库：

```bash
source scripts/production-maintenance.sh
production_maintenance_enter migration
trap 'production_maintenance_finish' EXIT

./scripts/stop.sh
! lsof -tiTCP:3000 -sTCP:LISTEN
! lsof -tiTCP:8080 -sTCP:LISTEN

production_maintenance_inspect
sed -n '1,20p' logs/supervisor.status

./scripts/start.sh --prod --no-build
curl --fail --silent http://127.0.0.1:8080/health
production_maintenance_finish
trap - EXIT
```

通过标准：operation 为 `migration`；owner、开始时间、heartbeat 存在；窗口内 Supervisor 不执行停启；3000/8080 在停服阶段保持释放；恢复后健康与进入前一致；窗口安全释放。不要在生产上伪造 owner 退出、PID 复用或 stale lock，这些由自动化测试证明。

## 4. apply 决策

- 没有真实 pending migration：**不执行 apply**。记录“无 pending，按设计跳过生产写”，只完成隔离 preflight 和维护窗口演练。
- 存在真实 pending migration：必须分别取得生产数据库写与目标 release 切换授权，先用目标 commit 生成未安装 stage，并确认代码/schema 配对回退后，才允许使用同一备份和通过报告：

```bash
./scripts/release.sh --prepare
export MAGICPODCAST_MIGRATION_RELEASE_STAGE=/absolute/path/from/prepared_stage
export MAGICPODCAST_MIGRATION_CONFIRM=I_UNDERSTAND_THIS_WRITES_DATA
export MAGICPODCAST_MIGRATION_RELEASE_CONFIRM=I_UNDERSTAND_THIS_SWITCHES_RELEASE
./scripts/migrate-db.sh --apply
```

该命令自行进入 migration 窗口、停服、重验报告/备份/commit/schema/源指纹/pending 与 stage 哈希、事务 apply，再通过 release 模块切换该 stage 并验收。不要同时运行其他 deploy、rollback 或 recovery。

## 5. 结束验收

取得相应访问和停启授权后核对：

```bash
git status --short --untracked-files=all
curl --fail --silent http://127.0.0.1:8080/health
./scripts/verify-db.sh backend/data/magicpodcast.db
sed -n '1,20p' logs/supervisor.status
./scripts/access-path-status.sh
```

必须确认 checkout 仍干净，数据库/备份/报告/日志/回退资产未进入 Git，`/ready` 的 release/frontend build/schema/Profile 精确配对，行动队列与加工投影可读，本机中继恢复。不得自动清理这些资产。

## 失败恢复判断

| 报告状态 | 数据库事实 | 服务/锁 | 下一步 |
| --- | --- | --- | --- |
| `rejected` | 未写入 | 未进入窗口或保持原状态 | 修正漂移，重新备份和 preflight |
| `rolled_back` | 未提交，源 schema 保持 | 服务停止，`recovery_required` | 查失败合同/DDL，修正后重新 preflight |
| `committed_verification_failed` | 已提交，禁止假定可回滚 | 服务停止，`recovery_required` | 先判断继续验证还是用配对备份恢复；恢复需单独授权 |
| `committed` / `already_applied` | 目标 schema 已提交 | 启动后验收通过才释放窗口 | 记录结果，不重复 apply |

恢复完成后只能通过 `restore-db.sh`（内部使用 `production_maintenance_enter recovery`）接管已知失败窗口；不要直接删除锁。

## RPO / RTO 与 Issue 证据模板

- RPO：维护窗口开始时间减去所用备份 `created_at`；记录实测值，不预设虚构目标。
- RTO：维护窗口开始到 release/schema/Profile、行动队列、加工投影和访问路径全部恢复的实测时长。

```text
目标 SHA：<40 位 SHA>
CI：<成功链接或检查名>
源/目标 schema：<N> → <N 或 N+1>
备份：<文件名> / SHA-256 <前 12 位> / verify=PASS
安全 preflight：plan=<plan ID 前 12 位> / apply_eligible=true
破坏性重放：blocked=true / violation=<非敏感代码> / source_schema_unchanged=true
维护窗口：operation=migration / supervisor_skipped=true / ports_released=true
生产 apply：SKIPPED(no pending) 或 AUTHORIZED+<committed 状态>
最终验收：health/profile/queue/processing/access=PASS
RPO：<实测>
RTO：<实测>
跳过项与原因：<逐项>
剩余人工边界：deploy / restore / Issue close 等
```
