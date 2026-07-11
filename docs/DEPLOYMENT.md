# MagicPodcast 部署和运维

最后更新：2026-05-31

本文记录当前可用的本地运行、生产模式启动、健康检查和备份恢复入口。旧 Docker 配置已移入根目录 `archive/`，只作历史参考。

## 快速启动

生产模式：

```bash
./scripts/start.sh --prod
```

开发模式：

```bash
./scripts/start.sh --dev
```

重启：

```bash
./scripts/restart.sh --prod
./scripts/restart.sh --dev
```

停止：

```bash
./scripts/stop.sh
```

项目根目录也保留了软链接，可使用：

```bash
./start.sh --prod
./restart.sh --prod
./stop.sh
```

## 服务地址

| 服务 | 地址 |
| --- | --- |
| 前端 | `http://localhost:3000` |
| 后端 | `http://localhost:8080` |
| 后端健康检查 | `http://localhost:8080/health` |

## 健康检查

```bash
./scripts/health-check.sh
```

或使用根目录软链接：

```bash
./health.sh
```

健康检查会覆盖：

- 前端和后端端口。
- 后端 `/health`。
- 生产/开发模式状态。
- 数据库文件和备份状态。
- 常用脚本入口。

## 生产模式说明

`./scripts/start.sh --prod` 会：

1. 编译后端为 `backend/api`。
2. 默认设置后端为 `release` 模式。
3. 默认关闭数据库 SQL 调试日志。
4. 构建 Next.js 生产版本。
5. 使用后台会话启动前后端，并记录监听端口对应的进程。
6. 等待端口监听并执行健康检查。

日志位置：

```text
/tmp/magicpodcast-backend.log
/tmp/magicpodcast-frontend.log
```

PID 文件位置：

```text
/tmp/magicpodcast-backend.pid
/tmp/magicpodcast-frontend.pid
```

## 环境配置

后端配置：

```text
backend/configs/config.yaml
backend/.env
```

前端配置：

```text
frontend/.env.local
```

详细说明见 [ENV_SETUP.md](ENV_SETUP.md)。

## Cloudflare Tunnel

Cloudflare Tunnel 辅助脚本：

```bash
./scripts/cloudflare-tunnel.sh help
```

常用命令：

```bash
./scripts/cloudflare-tunnel.sh start
./scripts/cloudflare-tunnel.sh auth
```

默认隧道目标是 `http://localhost:3000`，后端健康检查地址是 `http://localhost:8080/health`。如需改目标地址，可设置 `MAGICPODCAST_TUNNEL_URL` 和 `MAGICPODCAST_HEALTH_URL`。

## 开机自启

当前保留 `scripts/production-startup.sh`，用于本机登录后启动生产服务。该脚本只负责健康判断，并委托当前标准入口 `scripts/start.sh --prod` 或 `scripts/restart.sh --prod` 执行真实启动，避免单独维护第二套启动流程。

可用环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `MAGICPODCAST_PROJECT_DIR` | 脚本所在项目根目录 | 项目路径 |
| `MAGICPODCAST_STARTUP_LOG` | `/tmp/magicpodcast-production.log` | 开机启动日志 |
| `MAGICPODCAST_STARTUP_DELAY` | `3` | 等待网络和系统服务就绪的秒数 |

配置 launchd 前应先确认本机路径、端口和环境变量都正确。

## 备份和恢复

创建备份：

```bash
./scripts/backup-db.sh
```

安装每日备份任务：

```bash
./scripts/install-backup-agent.sh
```

验证数据库：

```bash
./scripts/verify-db.sh backend/data/magicpodcast.db
```

恢复备份：

```bash
./scripts/restore-db.sh backend/data/backups/magicpodcast_YYYYMMDD_HHMMSS.db.gz
```

恢复会要求服务停止，除非显式使用 `--force`。

## 性能和发布前检查

```bash
(cd backend && go test ./...)
(cd backend && go vet ./...)
(cd frontend && npm run type-check)
(cd frontend && npm run lint)
(cd frontend && npm run test:run)
(cd frontend && npm run build)
```

页面和 API 巡检：

```bash
node scripts/performance-audit.mjs \
  --base-url http://localhost:3000 \
  --api-url http://localhost:8080 \
  --runs 3
```

详细性能检查见 [PERFORMANCE_TESTING_GUIDE.md](PERFORMANCE_TESTING_GUIDE.md)。

## 清理

清理当前可用命令见 [CLEAN_GUIDE.md](CLEAN_GUIDE.md)。遇到前端缓存异常时优先使用：

```bash
./scripts/restart.sh --clean --prod
```

## 历史 Docker 配置

根目录 `archive/` 下保留旧 Dockerfile、docker-compose 和 Nginx 配置。当前默认运维入口是脚本方式；是否继续维护 Docker 部署需要人工确认。
