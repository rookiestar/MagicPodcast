# MagicPodcast 清理指南

最后更新：2026-05-31

本文记录当前仓库里真实可用的清理方式。旧的 `frontend/clean.sh`、`backend/clean.sh`、`clean-all.sh` 已下线，不再作为维护入口。

## 常用清理

### 重建前端生产缓存

```bash
./scripts/restart.sh --clean --prod
```

这会删除 `frontend/.next`，重新构建前端，并重启前后端服务。

### 停止服务后清理本地运行产物

```bash
./scripts/stop.sh
./scripts/clean-cache.sh --all
```

这些文件都是本地构建或运行产物，不进入版本库。

如只清理前端构建缓存：

```bash
./scripts/clean-cache.sh --frontend
```

如需先查看将删除哪些文件：

```bash
./scripts/clean-cache.sh --all --dry-run
```

更深度的缓存检查：

```bash
./scripts/clean-cache.sh --all --deep --dry-run
```

`--deep` 会额外清理前端依赖缓存；如果项目根目录没有 `package.json` 却存在误装的根目录 `node_modules/`，也会把它视为可清理产物。也可以只查看根目录残留：

```bash
./scripts/clean-cache.sh --workspace --deep --dry-run
```

在 `frontend/` 目录下仍可使用：

```bash
npm run clean
npm run clean:deep
```

### 检查本地服务和缓存状态

```bash
./scripts/health-check.sh
```

项目根目录也保留了 `health.sh` 软链接，可直接运行：

```bash
./health.sh
```

## 数据库相关清理

不要直接删除 `backend/data/` 下的真实数据库。涉及数据库前先备份：

```bash
./scripts/backup-db.sh
```

需要恢复备份时：

```bash
./scripts/restore-db.sh backend/data/backups/magicpodcast_YYYYMMDD_HHMMSS.db.gz
```

需要验证数据库文件时：

```bash
./scripts/verify-db.sh backend/data/magicpodcast.db
```

## 可删除与不可自动删除

| 类型 | 处理建议 |
| --- | --- |
| `frontend/.next` | 可删除，下一次构建会重新生成 |
| 根目录 `node_modules/` 且根目录没有 `package.json` | 可在 `--deep` 下删除，属于误装依赖产物 |
| 项目内 `.DS_Store` | 可删除，属于 macOS 本地文件；清理时会跳过依赖目录、构建目录和数据库目录 |
| 根目录 `api_server.log`、`api_server.pid` | 可删除，属于旧本地运行产物 |
| `backend/api` | 可删除，下一次启动会重新编译 |
| `backend/tmp`、`backend/bin` | 可删除，属于本地构建产物 |
| `/tmp/magicpodcast-*.log`、`/tmp/magicpodcast-*.pid` | 可在服务停止后删除 |
| `backend/data/` | 不自动删除，先备份并确认用途 |
| `backend/configs/config.yaml*`、`.env*` | 不自动删除，可能包含本机配置或敏感信息 |
| `logs/`、`backend/logs/` | 按需清理，先确认是否还需要排查问题 |

## 清理后验证

```bash
./scripts/restart.sh --prod
curl http://localhost:8080/health
curl -I http://localhost:3000
```

代码或文档改动后，再按需运行：

```bash
(cd backend && go test ./...)
(cd frontend && npm run type-check)
(cd frontend && npm run test:run)
(cd frontend && npm run build)
```
