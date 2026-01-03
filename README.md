# MagicPodcast

> 个人播库管理与自动化处理工具

## 📖 项目简介

MagicPodcast 是一个帮助个人用户管理小宇宙播客订阅、提升播客消费效率的 Web 应用。主要功能包括：

- 🎧 **我的订阅管理**: 同步小宇宙平台的订阅节目和单集数据
- 🏷️ **本地标签与备注**: 为节目/单集添加自定义标签和备注
- ⚙️ **自动化工作流**: 基于规则自动抓取播客信息并生成报告
- 📊 **数据可视化**: 展示播客消费统计和趋势

**适用场景**: 个人播客爱好者管理订阅、研究者分析播客数据

## 🎯 产品目标

解决小宇宙 APP 无法提供的站外数据管理功能，基于个人播客消费工作流提升信息筛选与吸收效率。

**MVP 范围**:
- ✅ 小宇宙订阅数据同步（手动 + 定时）
- ✅ 本地标签、备注管理
- ✅ 自定义工作流抓取播客信息
- ✅ 抓取报告与日志展示
- ❌ 暂不支持: 多租户、移动 APP、LLM 智能总结

## 🏗️ 技术架构

### 后端
- **语言**: Go 1.21+
- **框架**: Gin (github.com/gin-gonic/gin)
- **数据库**: SQLite + GORM
- **任务调度**: robfig/cron
- **配置管理**: Viper
- **日志**: logrus
- **外部集成**: [ultrazg/xyz](https://github.com/ultrazg/xyz) (小宇宙 API 代理)

### 前端
- **框架**: Next.js 14+ (App Router)
- **UI**: Tailwind CSS + shadcn/ui
- **语言**: TypeScript

### 数据库设计
核心表:
- `podcasts`: 节目信息
- `episodes`: 单集信息
- `tags`: 标签
- `podcasts_tags` / `episodes_tags`: 多对多关联
- `workflows`: 工作流
- `jobs`: 定时任务
- `reports`: 抓取报告

## 📂 项目结构

```
MagicPodcast/
├── backend/                     # Go 后端服务
│   ├── cmd/
│   │   └── api/
│   │       └── main.go         # 服务入口
│   ├── internal/
│   │   ├── config/             # 配置管理 (Viper)
│   │   ├── models/             # GORM 数据模型
│   │   ├── handlers/           # HTTP handlers (Gin)
│   │   ├── services/           # 业务逻辑层
│   │   │   ├── xyz_client.go   # 小宇宙客户端
│   │   │   ├── podcast.go
│   │   │   ├── sync.go
│   │   │   └── workflow.go
│   │   ├── middleware/         # Gin 中间件
│   │   └── database/           # 数据库初始化
│   ├── pkg/                    # 公共库
│   │   ├── logger/             # 日志工具 (logrus)
│   │   └── errors/             # 错误处理
│   ├── scripts/                # 独立脚本
│   │   ├── manual_sync.go      # 手动同步
│   │   └── init_db.go          # 数据库初始化
│   ├── configs/                # 配置文件
│   │   └── config.yaml
│   ├── go.mod
│   └── go.sum
├── frontend/                    # Next.js 前端应用
│   ├── src/
│   │   ├── app/                # Next.js 页面 (App Router)
│   │   ├── components/         # React 组件
│   │   └── lib/                # 工具库
│   └── package.json
├── docs/                       # 项目文档
├── data/                       # 本地数据存储 (开发用)
├── docker-compose.yml          # Docker 编排
├── Dockerfile.backend          # 后端 Docker 配置
├── Dockerfile.frontend         # 前端 Docker 配置
├── .gitignore
├── CLAUDE.md                   # 项目详细说明
└── README.md                   # 本文件
```

## 🚀 快速开始

### 环境要求

- Go 1.21+
- Node.js 18+
- SQLite 3
- Docker (可选，用于部署)

### 方式一：Docker 快速启动 (推荐)

```bash
# 克隆项目
git clone <repo-url>
cd MagicPodcast

# 启动所有服务（后端 + 前端 + 小宇宙 API）
docker-compose up -d

# 访问前端
open http://localhost:3000

# 查看后端日志
docker-compose logs -f backend
```

### 方式二：本地开发

#### 1. 启动小宇宙 API 服务

```bash
# 使用 Docker
docker run -d -p 8080:8080 --name xyz-api ghcr.io/ultrazg/xyz:latest

# 验证服务
curl http://localhost:8080/health
```

#### 2. 启动后端

```bash
cd backend

# 复制配置文件
cp configs/config.example.yaml configs/config.yaml

# 安装依赖
go mod download

# 初始化数据库
go run scripts/init_db.go

# 启动 API 服务
go run cmd/api/main.go
```

后端将运行在 http://localhost:8080

#### 3. 启动前端

```bash
cd frontend

# 安装依赖
npm install

# 配置环境变量
echo "NEXT_PUBLIC_API_URL=http://localhost:8080" > .env.local

# 启动开发服务器
npm run dev
```

访问 http://localhost:3000

## 📚 开发指南

### 开发模式

项目遵循 "Explore → Plan → Code → Commit" 工作流:

1. **Explore**: 调研和需求分析
2. **Plan**: 制定详细实现计划
3. **Code**: 按任务分步实现
4. **Commit**: 每个切片完成后提交代码

详细内容请查看 [CLAUDE.md](./CLAUDE.md)

### 编码规范

- **Go**: 遵守 [Effective Go](https://go.dev/doc/effective_go) 和 [Uber Go Style Guide](https://github.com/uber-go/guide)
  - 使用 `gofmt` 格式化代码
  - 包名使用小写单词
  - 导出的函数/类型必须添加注释
- **TypeScript**: 遵守 ESLint 配置
- **提交信息**: 使用约定式提交格式

### 测试

```bash
# 后端测试
cd backend
go test ./...

# 前端测试
cd frontend
npm test
```

### 常用命令

```bash
# 后端开发
cd backend
go mod tidy                    # 整理依赖
go run cmd/api/main.go         # 运行服务
go build -o bin/api cmd/api/main.go  # 编译二进制

# 前端开发
cd frontend
npm run dev                    # 开发服务器
npm run build                  # 生产构建
npm run lint                   # 代码检查
```

## 🗺️ 开发路线图

### ✅ 阶段 1: 项目骨架 + 数据库初始化
- [ ] 搭建基础架构
- [ ] 设计数据库模型
- [ ] 配置日志和测试环境

### 🔄 阶段 2: 小宇宙订阅同步 (MVP 核心)
- [ ] 集成小宇宙 API
- [ ] 实现登录态管理
- [ ] 手动同步"我的订阅"
- [ ] 增量更新策略

### 📝 阶段 3: 播客本地信息管理
- [ ] 标签系统 CRUD
- [ ] 备注功能
- [ ] 前端订阅列表页
- [ ] 前端节目详情页
- [ ] 标签视图页

### ⚙️ 阶段 4: 工作流与自动抓取
- [ ] 工作流定义与编辑
- [ ] 自定义节目源解析
- [ ] 定时任务调度
- [ ] 抓取报告生成

### 🚀 阶段 5: 数据初始化与优化
- [ ] PodcastIndex 数据集成
- [ ] 性能优化
- [ ] UI/UX 打磨
- [ ] 部署文档

## ⚠️ 免责声明

本项目仅供个人学习和研究使用。请遵守小宇宙平台的使用条款，合理控制请求频率，避免对服务器造成压力。本项目不支持任何商业用途或恶意爬取行为。

## 📄 许可证

MIT License

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📮 联系方式

如有问题或建议，请通过 [GitHub Issues](../../issues) 联系。

---

**Made with ❤️ for podcast lovers**
