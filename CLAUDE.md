# MagicPodcast Agent Guide

最后更新：2026-05-31

本文件是给代码代理使用的当前维护指南。它不再重复记录长篇产品状态、历史计划或阶段报告；这些内容已经迁入 `docs/` 的当前入口和归档目录。

## 当前事实来源

优先按下面顺序查证当前状态：

1. [README.md](README.md)：项目总览、常用启动和检查入口。
2. [docs/README.md](docs/README.md)：文档中心，区分当前文档和历史归档。
3. [docs/REFACTORING_ROADMAP.md](docs/REFACTORING_ROADMAP.md)：当前重构进度和下一步边界。
4. [docs/HUMAN_REVIEW_QUEUE.md](docs/HUMAN_REVIEW_QUEUE.md)：不能无人值守处理、需要人审的事项。
5. [docs/performance/BASELINE_2026-05-31.md](docs/performance/BASELINE_2026-05-31.md)：最新可复跑性能基线。

`docs/archive/` 和根目录 `archive/` 只作历史追溯。引用归档内容前，必须用当前源码、测试、构建或运行结果重新验证。

## 项目边界

- 不改变现有产品功能和用户可见行为。
- 优先清理无用代码、旧脚本、过时文档和重复维护入口。
- 涉及真实数据库、数据删除、跨主版本升级、搜索排序变化、通知策略或部署方式变化时，先记录到 [docs/HUMAN_REVIEW_QUEUE.md](docs/HUMAN_REVIEW_QUEUE.md)。
- 不自动删除本地配置、数据库、日志、备份和可能含敏感信息的文件。

## 技术栈

- 后端：Go、Gin、GORM、SQLite、logrus、robfig/cron、gofeed。
- 前端：Next.js 14.2.35、React 18、TypeScript、Tailwind CSS、SWR、Vitest。
- 运行：本地生产入口默认使用后端 `8080`、前端 `3000`。
- 可选数据源：本地 PodcastIndex SQLite 数据库，说明见 [docs/migration/PODCASTINDEX_DEDUP.md](docs/migration/PODCASTINDEX_DEDUP.md)。

## 标准入口

启动生产服务：

```bash
./scripts/restart.sh --prod
```

健康检查：

```bash
./scripts/health-check.sh
```

清理构建和本地产物：

```bash
./scripts/clean-cache.sh --all --dry-run
./scripts/clean-cache.sh --all --deep --dry-run
```

数据库备份和验证：

```bash
./scripts/backup-db.sh
./scripts/verify-db.sh backend/data/magicpodcast.db
```

## 验证要求

后端改动至少运行：

```bash
(cd backend && go test ./...)
(cd backend && go vet ./...)
```

前端改动至少运行：

```bash
(cd frontend && npm run type-check)
(cd frontend && npm run test:run)
```

性能或启动相关改动运行：

```bash
./scripts/restart.sh --prod
./scripts/health-check.sh
node scripts/performance-audit.mjs \
  --base-url http://localhost:3000 \
  --api-url http://localhost:8080 \
  --runs 3 \
  --strict
```

文档改动后检查本地 Markdown 链接，并确认当前入口没有继续引用旧端口、旧命令或归档路径作为当前事实。

## 当前人审边界

已知需要人审的事项不要自动处理：

- Next 14 到 Next 16 的跨主版本升级。
- `backend/cmd/migrate`、`backend/cmd/maint/*` 和会写真实数据的 SQL。
- `archive/` 下旧 Docker 配置是否继续维护。
- 搜索排序、缓存策略或返回语义变化。
- 工作流连续失败通知渠道和打扰频率。

完整清单以 [docs/HUMAN_REVIEW_QUEUE.md](docs/HUMAN_REVIEW_QUEUE.md) 为准。

## 协作口径

最终汇报使用简洁中文，说明做了什么、验证了什么、还剩什么。不展开实现细节，不把未验证的内容说成已完成。
