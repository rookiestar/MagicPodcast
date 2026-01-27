# 项目：MagicPodcast

## 1. 产品目标

本产品主要解决个人在使用「小宇宙」播客APP时平台暂时无法提供的个人数据管理功能（站外），并基于日常消费播客节目时所需要的工作流提升信息筛选与吸收的效率，大幅提升播客消费场景中的个人体验。

## 2. 项目范围
### 2.1. MVP版本功能清单

**当前实现状态：✅ 核心功能已完成（约85%）**

* MagicPodcast（Web应用+定时脚本）
  * 小宇宙个人数据管理（MyPodcasts）
    * 登录态（复用小宇宙）：本地存储用户的小宇宙登录凭据（通过第三方封装 API ultrazg/xyz），不做独立账号体系。
    * 数据同步：
      * ✅ 每天定时拉取「我的订阅」节目列表（定时调度已实现）
      * ✅ 支持按需手动「立即同步」（API + SSE实时进度推送）
    * 本地建模与持久化：
      * ✅ 储存实体：节目（podcast）、单集（episode）、标签（tag）
      * ✅ 支持手动为节目 / 单集打标签、写备注等自定义信息
    * 展示界面（管理端）：
      * ✅ 订阅列表页：按节目展示 + 搜索 / 筛选 / 排序（分页、多条件筛选）
      * ✅ 节目详情页：所有单集、本地标签、备注与其他自定义信息（仅标签及自定义信息支持读写）
      * ✅ 标签视图：按标签浏览关联的节目 / 单集

  * 播客数据处理工作流（MagicFlow）
    * 工作流展示界面：
      * ✅ 工作流列表页：展示已创建的工作流，支持创建、编辑、删除、禁用
      * ✅ 工作流详情页：
        * 定义：在「我的订阅」节目列表和自定义节目源的范围内筛选待抓取的节目
        * 报告：成功抓取的工作流支持按照任务完成时序展示信息报告与抓取日志
      * ✅ 自定义节目源解析：支持通过单个或批量节目名称调用搜索引擎获取播客在小宇宙平台上的URL，再通过ID抽取和组装生成RSS源URL（通过PodcastIndex.org快速初始化）

    * 数据抓取：
      * ✅ 条件判定：基于工作流定义的节目范围逐个判断是否符合抓取条件（时间窗口、关键词、标签等）
      * ✅ 信息抓取：将该节目符合条件的单集相关信息抓取到本地（RSS feed解析）
      * ✅ 抓取状态跟踪：失败重试、错误计数、最后抓取时间记录

    * 本地建模与持久化：
      * ✅ 储存实体：工作流（workflow）、定时任务（job）、报告（report）、执行记录（job_execution）
      * ✅ 调度系统：基于robfig/cron的定时任务调度器

  * 额外实现功能：
    * ✅ OPML导入：支持批量导入播客订阅列表
    * ✅ 全文搜索：基于SQLite FTS的全文搜索功能
    * ✅ PodcastIndex.org集成：快速初始化播客元数据
    * ✅ SSE实时推送：同步和抓取进度实时更新
    * ✅ 健康检查和监控端点

### 2.2. MVP版本不做 / 延后

* 不做多租户（先支持我自己用）。
* 不做移动APP，先做响应式网页。
* 不做收藏单集和评论的同步。
* 不做基于LLM的智能总结与推荐，先验证播客元信息可被正常抓取与展示。
* 不做站内外的消息推送。
* 不做搜索增强，如：热门搜索：统计高频搜索词、搜索建议：输入时显示相关建议（基于标题前缀）等。

## 3. 技术栈与架构

### 3.1. 后端 / 同步层（Go 1.24）

**已实现架构组件：**

- **Web框架**: Gin (github.com/gin-gonic/gin)
- **ORM**: GORM (gorm.io/gorm) + SQLite
- **配置管理**: Viper (github.com/spf13/viper)
- **日志**: logrus (github.com/sirupsen/logrus) + 文件轮转
- **HTTP客户端**: resty (github.com/go-resty/resty)
- **定时任务**: robfig/cron (github.com/robfig/cron/v3)
- **RSS解析**: gofeed (github.com/mmcdole/gofeed)
- **OPML解析**: go-opml (github.com/bobbykaz/opml-go)
- **爬虫**: colly (github.com/gocolly/colly)

**项目结构**（Standard Go Project Layout）：
```
backend/
├── cmd/
│   ├── api/main.go          # API服务入口
│   └── migrate/main.go      # 数据库迁移工具
├── internal/
│   ├── config/              # 配置管理
│   ├── database/            # 数据库连接和迁移
│   ├── feed/                # RSS feed解析
│   ├── handlers/            # HTTP handlers (15个)
│   ├── logger/              # 日志系统
│   ├── middleware/          # 中间件 (CORS等)
│   ├── models/              # 数据模型
│   ├── opml/                # OPML导入
│   ├── podcastindex/        # PodcastIndex集成
│   ├── router/              # 路由配置
│   ├── scheduler/           # 定时任务调度器
│   ├── scraper/             # 网页抓取
│   ├── services/            # 业务服务层
│   ├── sync/                # 小宇宙同步服务
│   ├── utils/               # 工具函数
│   ├── validation/          # 数据验证
│   └── workflow/            # 工作流引擎
├── pkg/                     # 可复用的包
├── configs/                 # 配置文件
└── data/                    # 数据库文件目录
```

**职责实现**：

- ✅ 封装小宇宙 API：
  - 集成 ultrazg/xyz 服务（通过HTTP调用）
  - 提供统一的 REST 接口给前端（/api/v1/*）
  - API handlers实现：podcasts, episodes, tags, workflows, sync, search, import等

- ✅ 维护数据库：
  - 使用GORM自动迁移管理schema更新
  - 节目、单集、本地标签、备注
  - 工作流、定时任务、报告
  - 自定义索引优化查询性能

- ✅ 同步任务：
  - 使用 robfig/cron 实现定时任务（默认每天凌晨 2 点同步）
  - 支持手动触发同步（POST /api/v1/sync）
  - SSE实时推送同步进度

- ✅ 数据初始化：
  - 对「我的订阅」列表中的节目，可以批量从PodcastIndex.org下载的本地数据库中复制相关数据
  - 对于PodcastIndex.org中没有索引到的节目，通过RSS feed解析完成初始化
  - 支持OPML批量导入

### 3.2. 数据库 / 持久层（SQLite）

**核心表结构（已实现）**：
- `podcasts`：id, xyz_id, title, feed_url, itunes_id, podcast_guid, description, author, cover_url, link, episode_count, newest_episode_date, is_subscribed, is_dead, my_rate, notes, last_fetched_at, fetch_error_count, data_source, popularity_score, priority, update_frequency等
- `episodes`：id, xyz_id, podcast_id, episode_no, title, medium_url, show_notes, published_date, my_rate, notes等
- `tags`：id, name, description, color
- `podcasts_tags`：podcast_id, tag_id (多对多关联)
- `episodes_tags`：episode_id, tag_id (多对多关联)
- `workflows`: id, title, description, conditions, is_enabled, schedule_config
- `jobs`: id, workflow_id, status, last_execution, next_execution
- `job_executions`: id, job_id, start_time, end_time, status, log_info
- `reports`: id, job_execution_id, report_type, content, created_at
- `sync_configs`: id, xyz_cookie, last_sync_at等

**数据库特性**：
- ✅ GORM自动迁移
- ✅ 自定义索引（优化搜索和排序性能）
- ✅ 外键关联
- ✅ 软删除支持（BaseModel）
- ✅ 时间戳自动管理

### 3.3. 前端 / 展示层（Next.js 14.2）

**技术栈**：
- **框架**: Next.js 14.2.0 (App Router) + React 18
- **UI**: Tailwind CSS + shadcn/ui
- **语言**: TypeScript 5
- **状态管理**: React Hooks + Context API
- **HTTP客户端**: Axios
- **Markdown**: react-markdown + DOMPurify

**项目结构**：
```
frontend/
├── src/
│   ├── app/                 # Next.js App Router页面
│   │   ├── (home)/          # 首页
│   │   ├── podcasts/        # 播客列表和详情页
│   │   ├── tags/            # 标签页
│   │   ├── workflows/       # 工作流页面
│   │   ├── import/          # OPML导入页
│   │   └── api/             # API路由（可选）
│   ├── components/          # React组件
│   │   ├── ui/              # shadcn/ui基础组件
│   │   ├── podcasts/        # 播客相关组件
│   │   ├── tags/            # 标签组件
│   │   └── workflows/       # 工作流组件
│   ├── hooks/               # 自定义Hooks
│   ├── lib/                 # 工具库
│   │   └── api.ts           # API客户端
│   └── types/               # TypeScript类型定义
├── public/                  # 静态资源
└── package.json
```

**已实现页面**：
- ✅ `/`：总览 / 快速入口
- ✅ `/podcasts`：订阅节目列表（分页、搜索、筛选、排序）
- ✅ `/podcasts/[id]`：节目详情 + 单集列表 + 标签/备注操作
- ✅ `/tags`：标签列表 + 点击看对应内容
- ✅ `/workflows`：工作流列表
- ✅ `/workflows/[id]`：工作流详情 + 报告列表 + 执行日志
- ✅ `/import`：OPML文件导入页面

**已实现组件**：
- ✅ UI组件库（shadcn/ui）：Button, Input, Card, Dialog, Badge等
- ✅ PodcastCard, PodcastCover, PodcastSearchBar
- ✅ TagBadge, TagInput, TagSelector
- ✅ WorkflowFormModal, ReportModal, MarkdownViewer
- ✅ EpisodeList（集成在播客详情页）

### 3.4. 小宇宙集成层

- ✅ 使用开源API代理 ultrazg/xyz
- ✅ 通过手机号登录获取「我的订阅」与节目/单集详情
- ✅ Cookie本地存储和管理
- ✅ 同步进度实时推送（SSE）

## 4. 功能切片实现进度

**总体进度：85%**

### ✅ 4.1. 项目骨架 + DB 初始化（已完成）
- Go + Next.js项目结构搭建
- SQLite数据库初始化
- GORM模型定义和迁移
- 基础配置系统

### ✅ 4.2. 订阅列表页（已完成）
- 播客列表API实现
- 前端分页、搜索、筛选、排序
- 单播封面展示
- 播客状态标识（订阅、失效等）

### ✅ 4.3. 节目详情页（已完成）
- 播客详情API实现
- 单集列表展示（分页）
- 标签和备注管理UI
- 元数据展示（更新频率、受欢迎程度等）

### ✅ 4.4. 标签与备注系统（已完成）
- 标签CRUD API
- 前端标签选择器组件
- 多对多关联实现
- 备注字段存储和展示

### ✅ 4.5. 打通小宇宙手动同步（已完成）
- 小宇宙API集成
- Cookie配置和管理
- 手动同步API
- OPML导入功能
- SSE实时进度推送

### ✅ 4.6. 自动定时同步（已完成）
- robfig/cron调度器实现
- 工作流定时任务配置
- 任务执行状态跟踪
- 执行日志记录

### ✅ 4.7. 工作流列表页（已完成）
- 工作流CRUD API
- 前端工作流列表展示
- 创建/编辑/删除工作流
- 启用/禁用工作流

### ✅ 4.8. 工作流详情页（已完成）
- 工作流详情API
- 条件配置界面
- 报告列表展示
- 执行日志查看
- Markdown渲染器集成

### ⚠️ 4.9. 自动定时任务抓取与报告生成（基本完成）
- RSS feed抓取服务
- 工作流条件判定逻辑
- 时间窗口计算
- 报告生成器
- 失败重试机制
- **待优化**：抓取性能、错误处理、报告格式

### ⚠️ 4.10. UI 打磨（进行中）
- ✅ 响应式设计（Tailwind）
- ✅ shadcn/ui组件集成
- ✅ 基础交互反馈（加载状态、错误提示）
- **待优化**：动画效果、深色模式、无障碍访问

## 5. 开发准则

### 5.1. 编码风格与质量要求

- **Go 代码规范**
  - 遵守 [Effective Go](https://go.dev/doc/effective_go) 和 [Uber Go Style Guide](https://github.com/uber-go/guide)。
  - 使用 `gofmt` 和 `goimports` 格式化代码。
  - 包名使用小写单词（如 `models`、`services`）。
  - 导出的函数、类型、常量必须添加文档注释。
  - 接口命名: `-er` 后缀（如 `Reader`、`Syncer`）。
  - 错误处理: 显式处理所有错误，不要忽略。
  - 项目布局: 遵守 [Standard Go Project Layout](https://github.com/golang-standards/project-layout)

- **TypeScript 代码规范**
  - 使用 ESLint + Prettier。
  - 组件使用 PascalCase: `PodcastCard.tsx`。
  - 工具函数使用 camelCase: `formatDate.ts`。

- **错误处理与日志**
  - 抓取逻辑中：
    - 对网络错误（超时、连接错误等）做明确捕获和重试。
    - 在日志中记录失败原因，但不要泄露敏感信息（如 cookie / token）。
  - 使用统一的 logging 设置（logrus），方便未来对接远程日志服务。

- **测试**
  - Go: 使用内置 `testing` 包和 `testify` 断言库。
  - 前端: 使用 Vitest 或 Jest。
  - 新功能优先编写基础单元测试或集成测试（哪怕是很小的 happy path 测试），尤其是抓取和解析逻辑。

### 5.2. 抓取与调度相关约束

- 抓取频率与合规
  - 默认每天仅运行一次完整抓取任务。
  - 在实现代码时，务必考虑：
    - 请求间隔（例如在循环中适当 sleep）。
    - 避免对小宇宙服务造成负载压力。
  - 如果需要访问任何需要登录 / 授权的内容，请先向我确认流程和风险。
- 增量更新
  - 设计抓取流水线时，优先支持增量抓取（例如只抓取新节目或最近 N 天的内容），避免每天全量重抓。
  - 在数据库模型中保留"最后抓取时间"等字段，便于实现增量策略。
- 定时运行场景
  - 可在无交互的命令行环境执行。
  - 通过环境变量或配置文件读取必要配置（例如代理、并发限制）。
  - 当某些外部条件不满足时（网络错误、目标站点变更等），要有可理解的日志输出，而不是静默失败或崩溃。

### 5.3. Vibe Coding偏好
- 每次只实现一个切片，端到端打通。
- 每个切片先写 5～10 步小计划，再编写代码。
- 发现重复逻辑时，提出抽象和重构建议。
- 变更后立即本地运行和手测，必要时重构和补测试。
- 执行任何对数据库schema做修改的操作前，先对数据库做好备份。
- 在合适的时机建议添加简单的可视化（例如用文本/表格输出热门节目统计）。
- 涉及前端的修改，注意在交付前清理缓存、重启服务等，帮助提高调试效率。
- 提醒我注意部署和定时任务相关的配置，并帮我生成对应的配置文件草稿。

## 6. 部署和运维

### 6.1. 本地开发（已配置）

```bash
# 启动所有服务
./dev.sh

# 或分别启动
cd backend && go run cmd/api/main.go
cd frontend && npm run dev
```

### 6.2. 生产部署（已配置）

- ✅ Docker镜像配置（Dockerfile.backend, Dockerfile.frontend）
- ✅ docker-compose编排
- ✅ 配置文件管理（configs/config.yaml）
- ⚠️ **待补充**：生产环境部署文档、备份策略、监控告警

## 7. 已知问题和改进方向

### 7.1. 已修复的问题
- ✅ 工作流时间窗口计算使用硬编码时间的bug
- ✅ OPML导入时为不可访问的feed创建无效记录的问题
- ✅ 播客484标题为描述文字的问题
- ✅ 调度器重载时的竞态条件

### 7.2. 待优化项
- ⚠️ 测试覆盖率提升（目标：>70%）
- ⚠️ 前端性能优化（虚拟滚动、懒加载）
- ⚠️ API响应缓存策略
- ⚠️ 数据库查询优化（N+1问题）
- ⚠️ 错误处理和用户提示优化
- ⚠️ 深色模式支持
- ⚠️ 国际化支持（i18n）

### 7.3. 功能增强（未来考虑）
- 基于LLM的智能总结与推荐
- 多租户支持
- 移动端APP（React Native / Flutter）
- 站内外消息推送（邮件、Webhook）
- 播客播放统计
- 收藏单集和评论同步
- 高级搜索功能（热门搜索、搜索建议）

---

**文档版本**: v2.0
**最后更新**: 2025-01-27
**项目状态**: MVP核心功能已完成，可投入实际使用
