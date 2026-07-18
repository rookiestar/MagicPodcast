# PodcastIndex 数据集安全升级运行手册

本手册对应 GitHub Issue #16。它只处理 PodcastIndex 外部候选库，不修改 MagicPodcast 主业务库，不选择替代 Feed，也不改变工作流状态。默认下载直连；本次经明确批准可用本机 FlyingBird 代理完成受控下载和候选 URL 可访问性检查，代理不会注入生产服务。

## 命令入口

```bash
cd backend
go run ./cmd/podcastindex_dataset self-test
```

所有下载和验收都必须使用独立 staging 目录，并显式传入当前生产库路径，让工具拒绝在生产数据库目录内操作：

```bash
STAGING=/path/to/independent/podcastindex-staging-20260718
LIVE=/Users/rookiestar/VSCode/Projects/MagicPodcast/backend/data/podcastindex_feeds.db
DOWNLOAD_MANIFEST="$STAGING/download.manifest.json"
VALIDATE_MANIFEST="$STAGING/validate.manifest.json"

go run ./cmd/podcastindex_dataset download \
  --staging-dir "$STAGING" \
  --live-db "$LIVE" \
  --manifest "$DOWNLOAD_MANIFEST" \
  --proxy http://127.0.0.1:7892 \
  --timeout 60s

go run ./cmd/podcastindex_dataset validate \
  --staging-dir "$STAGING" \
  --live-db "$LIVE" \
  --download-manifest "$DOWNLOAD_MANIFEST" \
  --manifest "$VALIDATE_MANIFEST" \
  --archive "$STAGING/podcastindex_feeds.db.tgz" \
  --baseline-db "$LIVE" \
  --samples "$STAGING/failed-samples.json" \
  --proxy http://127.0.0.1:7892 \
  --check-accessibility=true
```

样本文件只能从主业务库只读导出，不能通过修复、回填或工作流命令生成：

```bash
go run ./cmd/podcastindex_dataset export-samples \
  --primary-db /path/to/magicpodcast.db \
  --output "$STAGING/failed-samples.json"
```

## Go 条件

`validate` 只有在以下证据齐全且人工确认后才会把 manifest 标记为 `GO`：

- 下载前后 HEAD 的 URL、状态、长度、类型、ETag/Last-Modified 一致；若使用代理，manifest 必须记录明确的代理端点，且下载前后使用同一端点；
- 压缩包和 SQLite 候选文件均可复算 SHA-256；
- gzip、tar 路径、文件类型、符号链接和可执行内容检查通过；
- staging 文件系统在压缩包、解压库和 `max(20 GiB, 15% 总容量)` 余量之后仍有空间；
- 候选库真实 Schema、`podcasts` 字段类型、`v_unique_podcasts` 和 URL/title/iTunes ID 查询通过；
- 146 个失败节目样本已输出身份方法、置信级别和现场可访问性；同名但没有稳定身份标识的记录只进入人审清单；
- 原子切换/回滚本地自测通过，质量差异和 Human Review Queue 结论已人工确认。

任何动态元数据缺失、对象在下载期间发生变化、上游无法访问或磁盘门槛不足，均保持 `NO-GO`。不能用未明确批准的代理、替代源或换 source IP 填补证据；本次已明确批准的 FlyingBird 只用于本次 staging 验证，不改变生产服务网络路径。

## 切换和回滚

切换前必须在维护窗口停止服务，并确认没有 SQLite 连接或 WAL/SHM 文件被打开：

```bash
go run ./cmd/podcastindex_dataset cutover \
  --candidate "$STAGING/candidate/podcastindex_feeds.db" \
  --live-db "$LIVE" \
  --manifest "$VALIDATE_MANIFEST" \
  --service-stopped \
  --confirm I_UNDERSTAND_PODCASTINDEX_CUTOVER
```

切换只在同一文件系统内重命名；旧库会保留为带时间戳的只读 rollback 副本。重启后必须用现有服务入口检查 `/health`、`/ready` 和代表性 PodcastIndex 查询。若任何一项失败，停止服务并执行：

```bash
go run ./cmd/podcastindex_dataset rollback \
  --live-db "$LIVE" \
  --backup-db "$BACKUP_DB" \
  --manifest "$VALIDATE_MANIFEST" \
  --service-stopped \
  --confirm I_UNDERSTAND_PODCASTINDEX_ROLLBACK
```

失败候选不会被删除，会以 `.failed-时间戳` 保留供复核。服务重启和生产现场回滚验证不由该工具伪造为已完成，必须把实际运行结果补回 manifest。
