# MagicPodcast 数据备份与恢复

最后更新：2026-08-30

## 目标

MagicPodcast 的本地 SQLite 数据库已经承载了播客、单集、标签、工作流和报告数据。备份恢复流程的目标是：

1. 每次备份都先做一致性校验。
2. 每个备份都带校验文件，便于确认文件没有损坏。
3. 恢复前默认先给当前数据库再做一次安全备份。
4. 恢复完成后自动验证数据库结构和核心数据。
5. 每日自动备份，备份后压缩，并自动清理 14 天前的备份。

## 常用命令

从项目根目录执行：

```bash
./scripts/verify-db.sh
./scripts/backup-db.sh
./scripts/restore-db.sh backend/data/backups/magicpodcast_YYYYMMDD_HHMMSS.db.gz
./scripts/install-backup-agent.sh
```

也可以通过环境变量指定路径：

```bash
DB_PATH=/path/to/magicpodcast.db ./scripts/verify-db.sh
BACKUP_DIR=/path/to/backups RETENTION_DAYS=14 ./scripts/backup-db.sh
```

## 备份

运行：

```bash
./scripts/backup-db.sh
```

脚本会：

1. 使用 SQLite 的在线备份能力生成一致的数据库副本。
2. 验证备份是否可读、结构是否完整、外键是否一致。
3. 压缩为 `.db.gz`。
4. 生成 `.sha256` 校验文件。
5. 生成 `.meta` 元信息文件。
6. 默认清理 14 天前的备份。

`.meta` 只保存非敏感迁移基线：schema 版本、目标代码 commit、核心表计数和 Inbox/Focus/Someday/Done 聚合；不再记录数据库绝对路径。它可与 Migration Report 对账，但不能替代影子迁移和数据变化合同。

备份默认存放在：

```text
backend/data/backups/
```

## 每日自动备份

安装 macOS 定时任务：

```bash
./scripts/install-backup-agent.sh
```

默认每天 03:30 运行一次：

```text
~/Library/LaunchAgents/com.magicpodcast.backup.plist
```

日志位置：

```text
logs/backup-agent.out.log
logs/backup-agent.err.log
```

如果要调整时间，可以重新安装：

```bash
BACKUP_HOUR=4 BACKUP_MINUTE=15 ./scripts/install-backup-agent.sh
```

健康检查会显示定时任务是否已经加载：

```bash
./scripts/health-check.sh
```

## 加密异机副本

异机目标和密钥必须由所有者在 Mac mini 本机配置；脚本不会猜测云盘目录，也不会把密钥写入仓库、日志或备份元信息。当前推荐使用 `age` 的公钥加密：日常备份只读取公钥，私钥留在所有者控制的密码管理器或受限本机配置中。

配置完成后，使用以下环境变量启用：

```bash
export MAGICPODCAST_OFFSITE_DIR=/path/to/owner-controlled-offsite-folder
export MAGICPODCAST_AGE_RECIPIENT_FILE=/path/to/age-recipient.txt
export MAGICPODCAST_OFFSITE_KEEP=14
export MAGICPODCAST_OFFSITE_MAX_AGE_HOURS=26
```

手动加密最近一次本地备份并检查状态：

```bash
./scripts/offsite-backup.sh
./scripts/offsite-status.sh
```

要让每日 launchd 任务自动执行异机加密，可在 Mac mini 本机配置上述路径后重新安装备份任务：

```bash
MAGICPODCAST_OFFSITE_DIR=/path/to/owner-controlled-offsite-folder \
MAGICPODCAST_AGE_RECIPIENT_FILE=/path/to/age-recipient.txt \
MAGICPODCAST_OFFSITE_KEEP=14 \
MAGICPODCAST_OFFSITE_MAX_AGE_HOURS=26 \
./scripts/install-backup-agent.sh
```

安装器只把异机目录、公钥文件路径和保留/过期阈值写入 launchd 配置；私钥不会写入仓库、日志或 launchd 配置。

`offsite-status.sh` 只报告缺失、过期、校验失败或成功，不读取或打印密钥。异机目录必须与 `backend/data/backups/` 不同；保留策略只删除脚本生成的 `magicpodcast_*.db.gz.age` 文件及其配套校验/元信息文件。

## 临时恢复演练

恢复演练必须提供私钥路径，并且只解密到临时目录；脚本拒绝把结果写入生产数据库：

```bash
export MAGICPODCAST_AGE_IDENTITY_FILE=/path/to/owner-controlled-age-identity
./scripts/restore-drill.sh /path/to/magicpodcast_YYYYMMDD_HHMMSS.db.gz.age
```

演练会验证解密、压缩包、SQLite 完整性、必需表、外键和核心表可读性，完成后清理临时数据库，只留下不含真实内容的成功状态记录。

## 恢复

建议先停止本地服务：

```bash
./scripts/stop.sh
```

然后恢复：

```bash
./scripts/restore-db.sh backend/data/backups/magicpodcast_YYYYMMDD_HHMMSS.db.gz
```

脚本先取得与 deploy、rollback、migration 共用的 `recovery` 维护窗口，再验证并恢复备份；其他生产写操作正在运行时会直接拒绝。恢复不再提供绕过端口检查的 `--force`，8080/3000 任一仍监听都会拒绝替换数据库。数据库替换阶段进入不可自动回收的 `critical` 状态，异常中断后必须由下一次显式 recovery 接管。若当前数据库存在，默认会先自动创建一份安全备份。恢复脚本同时支持 `.db` 和 `.db.gz`。

数据库文件验证通过后仍保留 `recovery_required`，不会让 Supervisor 自动启动未知产物。先在与恢复后 schema 配对的干净代码 checkout 运行 `release.sh --prepare`，再由 release 模块原地接管、启动和验收：

```bash
MAGICPODCAST_RELEASE_MAINTENANCE_OPERATION=recovery \
MAGICPODCAST_RELEASE_SCHEMA_VERSION_OVERRIDE=<restored-schema> \
./scripts/release.sh --activate-prepared /absolute/path/to/paired-stage
```

只有目标 release 的 `/health` 与 `/ready`（含 schema、release、frontend build、production Profile）都验收通过后，release 模块才释放 recovery 窗口；再复核关键业务投影。

## 最新恢复演练

2026-05-31 已用最新备份 `backend/data/backups/magicpodcast_20260531_033001.db.gz` 恢复到临时数据库，并通过 `verify-db.sh` 验证。

演练结果：

- 备份可解压并恢复到临时库。
- SQLite 完整性检查通过。
- 必需表检查通过。
- 外键一致性检查通过。
- 临时库数据量：487 个播客、64451 个单集、51 个标签、18 个工作流。
- 演练没有覆盖当前运行数据库。

## 建议节奏

- 日常自用：每天至少一次自动备份，默认保留最近 14 天。
- 批量导入、同步、清洗、重构前：手动备份一次。
- 长期服务化部署前：把备份目录同步到另一个磁盘或云端。
- 做大版本升级前：先备份，再跑 `verify-db.sh`，最后再迁移。
