# MagicPodcast

> 个人播库管理与自动化处理工具

## 📖 项目简介

MagicPodcast 是一个帮助个人用户管理小宇宙播客订阅、提升播客消费效率的 Web 应用。主要功能包括：

- 🎧 **OPML 导入**: 支持从小宇宙等播客平台导出的 OPML 文件快速导入订阅
- 📚 **PodcastIndex 集成**: 自动从 PodcastIndex 数据库获取丰富的播客元数据（热度、单集数、最新单集等）
- 🏷️ **本地标签与备注**: 为节目/单集添加自定义标签和备注
- ⚙️ **自动化工作流**: 基于规则自动抓取播客信息并生成报告
- 📊 **数据可视化**: 展示播客消费统计和趋势

**适用场景**: 个人播客爱好者管理订阅、研究者分析播客数据

**核心优势**:
- ⚡ **超高性能**: 489 个播客仅需 19.5 秒导入（25 个/秒）
- 🎯 **高匹配率**: PodcastIndex 匹配率 85.7%（相比传统 feed_url 匹配的 30-40%）
- 🔄 **智能匹配**: 优先使用 title 精准匹配，自动获取完整元数据

## 🎯 产品目标

解决小宇宙 APP 无法提供的站外数据管理功能，基于个人播客消费工作流提升信息筛选与吸收效率。

**MVP 范围**:
- ✅ OPML 文件导入（支持小宇宙等平台导出）
- ✅ PodcastIndex 数据库集成（快速获取播客元数据）
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
- **外部数据源**:
  - [PodcastIndex Database](https://github.com/Podcastindex-org/database) (4.5GB 本地数据库，含 480万+ 播客)
  - [ultrazg/xyz](https://github.com/ultrazg/xyz) (小宇宙 API 代理，可选)
- **性能优化**:
  - 数据库索引优化（title、url 字段）
  - 并发导入机制（Worker Pool，默认 10 并发）

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
│   │   │   ├── sync/           # OPML 导入与同步服务
│   │   │   ├── podcast/        # 播客业务逻辑
│   │   │   ├── podcastindex/   # PodcastIndex 数据库查询
│   │   │   └── opml/           # OPML 文件解析
│   │   ├── middleware/         # Gin 中间件
│   │   └── database/           # 数据库初始化
│   ├── pkg/                    # 公共库
│   │   ├── logger/             # 日志工具 (logrus)
│   │   └── errors/             # 错误处理
│   ├── scripts/                # 独立脚本
│   │   ├── add_indexes.go      # PodcastIndex 索引创建工具
│   │   └── init_db.go          # 数据库初始化
│   ├── configs/                # 配置文件
│   │   └── config.yaml
│   ├── data/                   # 本地数据存储
│   │   ├── magicpodcast.db     # 应用数据库
│   │   └── podcastindex_feeds.db  # PodcastIndex 数据库（需下载）
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

#### 1. 下载 PodcastIndex 数据库（可选但推荐）

```bash
cd backend/data

# 从 PodcastIndex GitHub Releases 下载最新数据库
# 下载地址: https://github.com/Podcastindex-org/database/releases
# 例如: wget https://github.com/Podcastindex-org/database/releases/download/v1.0/podcastindex_feeds.db

# 或使用 Torrent 下载（推荐，4.5GB 文件）
# Torrent 文件: https://github.com/Podcastindex-org/database

# 创建数据库索引（约 30 秒）
cd ../
go run scripts/add_indexes.go -db="./data/podcastindex_feeds.db"
```

**为什么需要 PodcastIndex 数据库？**
- 🚀 超快速查询：489 个播客 19.5 秒导入
- 🎯 高匹配率：85.7% 的播客可自动匹配
- 📚 丰富元数据：自动获取热度、单集数、最新单集等信息

#### 2. 启动后端

```bash
cd backend

# 复制配置文件
cp configs/config.example.yaml configs/config.yaml

# 编辑配置文件，设置 PodcastIndex 数据库路径
# vim configs/config.yaml
# 确保 podcast_index.path = "./data/podcastindex_feeds.db"

# 安装依赖
go mod download

# 初始化数据库
go run scripts/init_db.go

# 启动 API 服务
go run cmd/api/main.go
```

后端将运行在 http://localhost:8080

#### 3. 测试 OPML 导入

```bash
# 使用 curl 测试导入
curl -X POST http://localhost:8080/api/v1/sync/import \
  -F "opml_file=@/path/to/your/opml_file.opml"

# 或通过前端界面上传 OPML 文件
# 访问 http://localhost:3000 并使用 OPML 导入功能
```

#### 4. 启动前端

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

## 💡 使用指南

### OPML 导入功能

**支持的 OPML 格式**：
- ✅ 小宇宙播客 APP 导出的 OPML 文件
- ✅ 苹果播客、Overcast 等标准 OPML 文件
- ✅ 任意包含 RSS feed URL 的 OPML 文件

**导入流程**：
1. 从播客 APP 导出 OPML 文件
2. 在前端界面点击"导入 OPML"
3. 选择文件并上传
4. 系统自动：
   - 解析 OPML 文件
   - 从 PodcastIndex 匹配播客（85.7% 匹配率）
   - 对于未匹配的播客，在线抓取 RSS feed
   - 保存到本地数据库

**数据来源**（自动标注）：
- `podcastindex`: 来自 PodcastIndex 数据库（含完整元数据）
- `rss`: 在线抓取的 RSS feed

**字段映射**：
- 保留 OPML 字段：`title`（标题）、`description`（描述）、`feed_url`（RSS 地址）
- PodcastIndex 补充：`author`、`cover_url`、`episode_count`、`popularity_score`、`newest_enclosure_url` 等

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
- [x] 搭建基础架构
- [x] 设计数据库模型
- [x] 配置日志和测试环境

### ✅ 阶段 2: OPML 导入与 PodcastIndex 集成 (MVP 核心)
- [x] OPML 文件解析（支持小宇宙等平台）
- [x] PodcastIndex 数据库集成
- [x] 智能匹配策略（title 优先，feed_url 备选）
- [x] 并发导入机制（10 并发 worker pool）
- [x] 数据库索引优化（title、url 字段）
- [x] 性能测试（489 个播客 19.5 秒，85.7% 匹配率）

### 🔄 阶段 3: 播客本地信息管理
- [x] 标签系统 CRUD
- [x] 备注功能
- [x] 前端订阅列表页
- [x] 前端节目详情页
- [x] 标签视图页

### ⚙️ 阶段 4: 工作流与自动抓取
- [ ] 工作流定义与编辑
- [ ] 自定义节目源解析
- [ ] 定时任务调度
- [ ] 抓取报告生成

### 🚀 阶段 5: 数据初始化与优化
- [x] PodcastIndex 数据集成
- [x] 性能优化（索引、并发）
- [ ] UI/UX 打磨
- [ ] 部署文档

## 📊 性能测试结果

**测试环境**:
- 硬件: MacBook Pro (Apple M系列芯片)
- 数据库: SQLite 3 + PodcastIndex 4.5GB
- 测试数据: 489 个真实播客（小宇宙 OPML 导出）

**性能指标**:

| 指标 | 数值 |
|------|------|
| 总导入时间 | 19.5 秒 |
| 平均速度 | 25.0 个/秒 |
| 成功率 | 99.6% (487/489) |
| PodcastIndex 匹配率 | 85.7% (419/489) |
| 在线抓取占比 | 13.9% (68/489) |

**优化效果对比**:

| 项目 | 优化前 | 优化后 | 提升 |
|------|--------|--------|------|
| 单次查询时间 | ~15 秒 | ~0.04 秒 | **375x** ⬆️ |
| 489 个播客导入 | ~2 小时 | 19.5 秒 | **370x** ⬆️ |
| 导入速度 | 0.07 个/秒 | 25.0 个/秒 | **357x** ⬆️ |
| PodcastIndex 匹配率 | 30-40% | 85.7% | **2.1x** ⬆️ |

**关键优化**:
1. ✅ **数据库索引**: 为 `title` 和 `url` 字段添加索引（31 秒创建时间）
2. ✅ **并发处理**: Worker Pool 模式，10 个并发 worker
3. ✅ **智能匹配**: 优先 title 精准匹配，feed_url 作为备选

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
