# 后端命令入口

最后更新：2026-08-30

本目录保存后端服务和维护命令。除 `api` 外，多数命令会直接读取或改写 SQLite 数据库，执行前需要先确认数据库路径和备份状态。

## 当前常用入口

| 目录 | 用途 | 备注 |
| --- | --- | --- |
| `api` | 启动后端 API 服务 | 推荐通过根目录 `scripts/start.sh`、`scripts/restart.sh` 启动 |
| `benchmark` | 对本地 API 做性能压测 | 用于建立和复查性能基线 |
| `add_indexes` | 为主数据库应用性能索引和搜索 FTS 表 | 默认数据库为 `data/magicpodcast.db`，也可传入数据库路径 |
| `podcastindex_dataset` | PodcastIndex 外部 SQLite 数据集的 staging、验收、原子切换和回滚 | 只操作外部候选库；下载、校验和切换均需显式路径 |
| `data-profile` | 本地 Fixture/Snapshot 状态、校验和受控后端切换 | 推荐使用根目录 `scripts/data-profile.sh` |

## 需要人工确认后再运行

| 目录 | 风险 | 建议 |
| --- | --- | --- |
| `migrate` | 在已验证备份副本执行影子迁移，或消费通过的 Migration Report 应用版本化迁移 | `--preflight` 生成 Migration Report；`--apply` 由根脚本在共享维护窗口内调用，必须提供确认字符串、原备份和未漂移报告 |
| `maint/init_db` | 只初始化完全空白的 SQLite | 发现任何既有业务 schema 即拒绝；已有库只能走生产迁移 Runner |
| `maint/*` | 多数是历史数据修复、导入、检查或外部数据处理脚本，部分带本机文件路径，部分会删除或覆盖数据 | 已单独列入人审清单；保留源码，后续按真实维护需求决定保留、合并、归档或删除 |
| `snapshot-export` | 从生产 SQLite 创建一致、脱敏的只读传输包 | 生产读取和 Mac mini 操作需单独明确授权 |

## 执行原则

1. 先备份并验证真实数据库，再运行任何写入型命令。
2. 优先使用支持 dry-run 的命令查看影响范围。
3. 对生产或长期使用的数据执行维护命令前，先停止正在写入同一数据库的服务。
4. 普通 API 启动不得承担结构迁移；迁移失败时禁止启动服务。
5. 不确定用途的命令不直接删除，也不自动执行，统一放入 `docs/HUMAN_REVIEW_QUEUE.md`。

## PodcastIndex 数据集安全流程

从仓库根目录执行 `go run ./backend/cmd/podcastindex_dataset ...`，或从 `backend/` 目录执行 `go run ./cmd/podcastindex_dataset ...`。完整顺序是：

1. `download` 将官方固定地址写入独立 staging，记录 HEAD 指纹和压缩包 SHA-256；默认直连，若经批准可通过 `--proxy` 显式指定验证器代理；
2. `validate` 在 staging 候选库上检查 gzip/tar、真实 SQLite Schema、`v_unique_podcasts`、URL/title/iTunes ID 查询和失败样本；
3. 只有 manifest 为 GO 且已确认服务停止时，才允许 `cutover`；
4. 服务重启、health/ready 和代表性查询由维护窗口执行；异常时用 `rollback` 恢复时间戳回滚副本。

该命令不会改写 MagicPodcast 主业务库，不会自动挑选替代 Feed，也不会让生产服务继承代理配置；代理只在命令显式传入 `--proxy` 时用于本次 staging/验证。
