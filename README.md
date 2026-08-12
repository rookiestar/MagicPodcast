# MagicPodcast

MagicPodcast 是一个个人播客管理与自动化处理工具，用来导入 OPML 订阅、整理播客标签和备注、同步单集信息，并通过工作流生成处理报告。

## 当前功能

- OPML 导入：支持小宇宙、Apple Podcasts、Overcast 等标准 OPML。
- 播客管理：节目列表、详情、封面、标签、备注和单集列表。
- 搜索：支持节目和单集搜索。
- 同步：支持订阅、播客元数据和单集同步。
- 工作流：支持创建、编辑、执行、查看报告和执行历史。
- 运维：提供启动、停止、健康检查、备份、恢复和性能巡检脚本。

## 技术栈

| 模块 | 技术 |
| --- | --- |
| 后端 | Go 1.24、Gin、GORM、SQLite |
| 前端 | Next.js 16.2.10、React 18、TypeScript、Tailwind CSS、SWR |
| 测试 | Go test、Vitest、Testing Library |
| 性能 | `scripts/performance-audit.mjs`、`backend/cmd/benchmark` |

## 项目结构

```text
MagicPodcast/
├── AGENTS.md       Agent 唯一权威治理合同
├── backend/        Go API、数据库、同步、工作流和维护工具
├── frontend/       Next.js 前端应用
├── scripts/        启停、健康检查、备份恢复和性能巡检脚本
├── docs/           当前文档、历史报告和性能基线
└── archive/        已归档的旧 Docker / Nginx 配置
```

编码代理请遵守 [AGENTS.md](AGENTS.md)。日常验证见 [docs/AGENT_VERIFICATION.md](docs/AGENT_VERIFICATION.md)；发布与回退见 [docs/RELEASE_CHECKLIST.md](docs/RELEASE_CHECKLIST.md)。

## 本地启动

推荐使用统一脚本启动当前本地服务：

```bash
./scripts/start.sh --prod
```

开发时也可以分别启动：

```bash
# 后端
cd backend
go run ./cmd/api

# 前端
cd frontend
npm install
npm run dev
```

常用地址：

- 前端：http://localhost:3000
- 后端健康检查：http://localhost:8080/health

## 常用检查

```bash
# 后端
cd backend
go test ./...

# 前端
cd frontend
npm run type-check
npm run test:run
npm run build

# 页面/API 性能巡检
cd ..
node scripts/performance-audit.mjs \
  --base-url http://localhost:3000 \
  --api-url http://localhost:8080 \
  --runs 3
```

最新性能基线见 [docs/performance/BASELINE_2026-05-31.md](docs/performance/BASELINE_2026-05-31.md)。

## 文档入口

- [文档中心](docs/README.md)
- [重构路线图](docs/REFACTORING_ROADMAP.md)
- [需要人工确认的事项](docs/HUMAN_REVIEW_QUEUE.md)
- [部署运维](docs/DEPLOYMENT.md)
- [环境配置](docs/ENV_SETUP.md)
- [数据 Profile](docs/DATA_PROFILES.md)
- [备份恢复](docs/BACKUP_RECOVERY.md)
- [前端测试](docs/FRONTEND_TESTING_SETUP.md)

## 数据与配置

- 示例后端配置：`backend/configs/config.example.yaml`
- 前端环境示例：`frontend/.env.example`
- 本地数据库默认位于 `backend/data/`
- 隔离网络开发使用 `./scripts/data-profile.sh use fixture`；详见 [数据 Profile](docs/DATA_PROFILES.md)
- 真实配置、数据库、日志和构建产物不进入版本库

## 许可

MIT License
