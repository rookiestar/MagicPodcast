# MagicPodcast 数据备份与恢复

最后更新：2026-05-16

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

## 恢复

建议先停止本地服务：

```bash
./scripts/stop.sh
```

然后恢复：

```bash
./scripts/restore-db.sh backend/data/backups/magicpodcast_YYYYMMDD_HHMMSS.db.gz
```

脚本会先验证备份文件，再恢复数据库。若当前数据库存在，默认会先自动创建一份安全备份。恢复脚本同时支持 `.db` 和 `.db.gz`。

如果明确知道服务正在运行但仍要恢复，可以使用：

```bash
./scripts/restore-db.sh backend/data/backups/magicpodcast_YYYYMMDD_HHMMSS.db.gz --force
```

这个选项只建议在你清楚当前风险时使用。

## 建议节奏

- 日常自用：每天至少一次自动备份，默认保留最近 14 天。
- 批量导入、同步、清洗、重构前：手动备份一次。
- 长期服务化部署前：把备份目录同步到另一个磁盘或云端。
- 做大版本升级前：先备份，再跑 `verify-db.sh`，最后再迁移。
