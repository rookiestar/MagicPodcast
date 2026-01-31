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
      * ✅ 储存实体：节目（podcast，50+字段）、单集（episode）、标签（tag）
      * ✅ 支持手动为节目 / 单集打标签、写备注等自定义信息
    * 展示界面（管理端）：
      * ✅ 订阅列表页：按节目展示 + 搜索 / 筛选 / 排序（分页、多条件筛选、无限滚动）
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
      * ✅ 储存实体：工作流（workflow）、定时任务（job）、执行记录（job_execution）、报告（report）、调度器运行记录（scheduler_run）
      * ✅ 调度系统：基于robfig/cron的定时任务调度器（支持秒级精度、错过任务补偿）

  * API服务（已实现46+个端点，13个功能模块）
    * ✅ 健康检查（2个）：/health, /ping
    * ✅ 图片代理（2个）：/images/proxy, /images/health
    * ✅ 播客管理（4个）：列表、批量获取、详情、单集列表
    * ✅ 搜索服务（1个）：全文搜索（支持播客和单集，带标签筛选）
    * ✅ 标签管理（5个）：CRUD完整操作
    * ✅ 备注管理（4个）：播客和单集的备注读写
    * ✅ 标签关联（6个）：播客和单集的标签关联管理
    * ✅ 同步服务（7个）：订阅同步、OPML导入、元数据同步、单集同步（支持SSE）
    * ✅ 工作流管理（9个）：CRUD、启用/禁用、手动触发、执行历史
    * ✅ 任务管理（2个）：任务详情、报告查看
    * ✅ 调度器管理（4个）：重载、状态查询、暂停/恢复
    * ✅ LLM功能（7个）：统计查询、Prompt模板管理（可选功能）

  * 额外实现功能：
    * ✅ OPML导入：支持批量导入播客订阅列表
    * ✅ 全文搜索：基于SQLite FTS的全文搜索功能
    * ✅ PodcastIndex.org集成：快速初始化播客元数据
    * ✅ SSE实时推送：同步和抓取进度实时更新
    * ✅ 健康检查和监控端点
    * ✅ 图片代理服务：跨域图片请求代理

### 2.2. MVP版本不做 / 延后

* 不做多租户（先支持我自己用）。
* 不做移动APP，先做响应式网页。
* 不做收藏单集和评论的同步。
* 不做基于LLM的推荐，创意模式（类似google lucky）和导入/同步后的自动打标。
* 不做站内外的消息推送。
* 不做搜索增强，如：热门搜索：统计高频搜索词、搜索建议：输入时显示相关建议（基于标题前缀）等。

## 3. 技术栈与架构

### 3.1. 后端 / 同步层（Go）

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
- **SSE服务**: 内置基于Gin的SSE推送实现
- **Markdown渲染**: goldmark (github.com/yuin/goldmark)

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
│   ├── handlers/            # HTTP handlers (13个功能模块)
│   ├── logger/              # 日志系统
│   ├── middleware/          # 中间件 (CORS等)
│   ├── models/              # 数据模型
│   ├── opml/                # OPML导入
│   ├── podcastindex/        # PodcastIndex集成
│   ├── prompt/              # Prompt模板管理
│   ├── router/              # 路由配置
│   ├── scheduler/           # 定时任务调度器
│   ├── scraper/             # 网页抓取（Colly）
│   ├── search/              # 全文搜索服务
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
  - API handlers实现：podcasts, episodes, tags, workflows, sync, search, import, scheduler等

- ✅ 维护数据库：
  - 使用GORM自动迁移管理schema更新
  - 节目（50+字段）、单集、本地标签、备注
  - 工作流、定时任务、执行记录、报告、调度器运行记录
  - 自定义索引优化查询性能

- ✅ 同步任务：
  - 使用 robfig/cron 实现定时任务（默认每天凌晨 2 点同步）
  - 支持秒级精度的Cron表达式（6位格式）
  - 支持手动触发同步（POST /api/v1/sync）
  - SSE实时推送同步进度

- ✅ 数据初始化：
  - 对「我的订阅」列表中的节目，可以批量从PodcastIndex.org下载的本地数据库中复制相关数据
  - 对于PodcastIndex.org中没有索引到的节目，通过RSS feed解析完成初始化
  - 支持OPML批量导入

- ✅ 搜索服务：
  - 基于SQLite FTS5的全文搜索
  - 支持播客和单集的联合搜索
  - 标签筛选和分页

- ✅ 报告生成：
  - Markdown格式的执行报告
  - LLM智能摘要（可选功能）
  - 详细的执行日志记录

### 3.2. 数据库 / 持久层（SQLite）

**核心表结构（已实现）**：

- `podcasts`：播客节目表（50+字段）
  - 基础信息：id, xyz_id, title, feed_url, itunes_id, podcast_guid, description, author
  - 元数据：cover_url, link, episode_count, newest_episode_date
  - 状态标识：is_subscribed, is_dead, my_rate, notes
  - 抓取管理：last_fetched_at, fetch_error_count, data_source
  - PodcastIndex数据：popularity_score, priority, update_frequency等

- `episodes`：播客单集表
  - 基础信息：id, xyz_id, podcast_id, episode_no, title, medium_url
  - 内容：show_notes, published_date
  - 用户数据：my_rate, notes

- `tags`：标签表
  - id, name, description, color

- `podcasts_tags`：播客-标签关联表（多对多）
  - podcast_id, tag_id

- `episodes_tags`：单集-标签关联表（多对多）
  - episode_id, tag_id

- `workflows`：工作流表
  - id, title, description, is_enabled
  - schedule_config（调度配置）
  - scope_config（范围配置）
  - rules_config（规则配置）

- `jobs`：任务表
  - id, workflow_id, status
  - last_execution, next_execution

- `job_executions`：任务执行详情表
  - id, job_id, podcast_id
  - start_time, end_time, status
  - log_info, error_info

- `reports`：工作流执行报告表
  - id, job_execution_id, report_type
  - content（Markdown格式）
  - created_at

- `scheduler_runs`：调度器运行记录表
  - id, start_time, end_time, status
  - workflows_triggered, total_jobs

- `sync_configs`：同步配置键值对表
  - id, key, value

**数据库特性**：
- ✅ GORM自动迁移
- ✅ 自定义索引（优化搜索和排序性能）
- ✅ 外键关联
- ✅ 软删除支持（BaseModel）
- ✅ 时间戳自动管理（created_at, updated_at）
- ✅ FTS5全文搜索支持

### 3.3. 前端 / 展示层（Next.js 14.2）

**技术栈**：
- **框架**: Next.js 14.2.0 (App Router) + React 18
- **UI**: Tailwind CSS + shadcn/ui
- **语言**: TypeScript 5
- **状态管理**: React Hooks + Context API
- **HTTP客户端**: Axios
- **Markdown渲染**: react-markdown + rehype-raw + remark-gfm
- **图片优化**: Next.js Image + 图片代理服务

**项目结构**：
```
frontend/
├── src/
│   ├── app/                 # Next.js App Router页面
│   │   ├── (home)/          # 首页
│   │   ├── podcasts/        # 播客列表和详情页
│   │   ├── tags/            # 标签页
│   │   ├── workflows/       # 工作流页面
│   │   └── import/          # OPML导入页
│   ├── components/          # React组件
│   │   ├── ui/              # shadcn/ui基础组件
│   │   ├── podcasts/        # 播客相关组件
│   │   ├── tags/            # 标签组件
│   │   └── workflows/       # 工作流组件
│   ├── hooks/               # 自定义Hooks
│   ├── lib/                 # 工具库
│   │   ├── api.ts           # API客户端
│   │   ├── imageProxy.ts    # 图片代理服务
│   │   ├── imageLoader.ts   # 图片加载优化
│   │   ├── textUtils.ts     # 文本处理工具
│   │   └── timeUtils.ts     # 时间格式化工具
│   └── types/               # TypeScript类型定义
├── public/                  # 静态资源
└── package.json
```

**已实现页面**（7个）：
- ✅ `/`：首页总览，4个功能快捷入口
- ✅ `/podcasts`：订阅节目列表（5列网格布局、无限滚动、分页、搜索、筛选、排序）
- ✅ `/podcasts/[id]`：节目详情 + 单集列表（渐进式加载）+ 标签/备注操作
- ✅ `/tags`：标签列表 + 点击查看关联内容
- ✅ `/workflows`：工作流列表（创建、编辑、删除、启用/禁用）
- ✅ `/workflows/[id]`：工作流详情 + 报告列表 + 执行日志
- ✅ `/import`：OPML文件导入页面

**已实现组件**：

1. **UI基础组件**（shadcn/ui）：
   - Button, Input, Card, Dialog, Badge
   - Dropdown Menu, Select, Tabs
   - Toast, Alert, Skeleton

2. **播客相关组件**：
   - PodcastCard：播客卡片（封面、标题、状态标识）
   - PodcastCover：封面图片（支持图片代理、优先级加载）
   - EpisodeCard：单集卡片
   - PodcastListItem：播客列表项
   - PodcastSearchBar：搜索栏

3. **标签相关组件**：
   - TagBadge：标签展示（自定义颜色）
   - TagInput：标签输入（支持搜索、创建、键盘导航）
   - TagSelector：标签选择器

4. **工作流相关组件**：
   - WorkflowFormModal：工作流编辑模态框
   - ReportModal：报告查看模态框
   - MarkdownViewer：Markdown内容渲染器
   - WorkflowStatusBadge：工作流状态标识

5. **其他功能组件**：
   - SearchSidebar：搜索侧边栏
   - RichText：富文本展示组件
   - EpisodeList：单集列表（集成在播客详情页）

**状态管理方式**：
- **页面级状态**：useState管理组件状态
- **URL状态同步**：useSearchParams同步URL参数（排序、筛选等）
- **实时更新**：visibilitychange事件监听，页面从后台返回时刷新数据
- **无限滚动**：IntersectionObserver实现
- **全局状态**：Context API提供全局Toast消息

**设计系统**：
- **主色调**：violet-600 到 indigo-600 的渐变
- **中性色**：slate系列（slate-50 到 slate-900）
- **圆角**：xl (12px)
- **阴影**：sm、md、lg层级
- **响应式**：container mx-auto + 断点系统（sm、md、lg、xl）
- **交互动画**：transition-colors、transition-shadow、hover效果

### 3.4. 小宇宙集成层

- ✅ **API代理服务**：使用开源API代理 ultrazg/xyz
- ✅ **登录态管理**：通过手机号登录获取「我的订阅」与节目/单集详情
- ✅ **Cookie管理**：本地存储和管理，支持配置更新
- ✅ **实时推送**：SSE（Server-Sent Events）同步进度实时推送
- ✅ **搜索引擎集成**：使用Colly爬虫框架
  - 支持通过节目名称搜索小宇宙平台URL
  - ID抽取和RSS源URL生成
- ✅ **PodcastIndex集成**：
  - 快速初始化播客元数据
  - 本地数据库查询优化
  - 批量元数据获取

## 4. 功能切片实现进度

**总体进度：85%**

### ✅ 4.1. 项目骨架 + DB 初始化（已完成）
- Go + Next.js项目结构搭建
- SQLite数据库初始化
- GORM模型定义和迁移（10个核心表）
- 基础配置系统（Viper + YAML）
- 健康检查端点

### ✅ 4.2. 订阅列表页（已完成）
- 播客列表API实现（支持分页、搜索、筛选、排序）
- 前端5列网格布局 + 无限滚动
- 搜索侧边栏（实时搜索）
- 标签筛选和多条件排序
- 单播封面展示（图片代理、优先级加载）
- 播客状态标识（订阅、失效、新更新等）

### ✅ 4.3. 节目详情页（已完成）
- 播客详情API实现（50+字段）
- 单集列表展示（渐进式加载、分页）
- 标签和备注管理UI（TagInput、TagSelector）
- 元数据展示（更新频率、受欢迎程度、单集数量等）
- 单集卡片展示（标题、发布时间、时长等）

### ✅ 4.4. 标签与备注系统（已完成）
- 标签CRUD API（5个端点）
- 前端标签选择器组件（搜索、创建、键盘导航）
- 多对多关联实现（播客-标签、单集-标签）
- 备注字段存储和展示（播客和单集）
- 标签颜色自定义

### ✅ 4.5. 打通小宇宙手动同步（已完成）
- 小宇宙API集成（ultrazg/xyz）
- Cookie配置和管理
- 手动同步API（订阅、元数据、单集）
- OPML导入功能（支持SSE流式）
- SSE实时进度推送
- 同步状态跟踪和错误处理

### ✅ 4.6. 自动定时同步（已完成）
- robfig/cron调度器实现（支持秒级精度）
- 工作流定时任务配置（Cron表达式）
- 任务执行状态跟踪（Job、JobExecution）
- 执行日志记录（详细日志、错误信息）
- 失败重试机制
- 调度器热重载
- 错过任务补偿执行

### ✅ 4.7. 工作流列表页（已完成）
- 工作流CRUD API（9个端点）
- 前端工作流列表展示
- 创建/编辑/删除工作流（WorkflowFormModal）
- 启用/禁用工作流
- 工作流状态展示（最后执行时间、下次执行时间）

### ✅ 4.8. 工作流详情页（已完成）
- 工作流详情API
- 条件配置界面（节目范围、规则配置）
- 报告列表展示（按时间序）
- 执行日志查看（详细日志、错误信息）
- Markdown渲染器集成（react-markdown）
- 报告模态框（ReportModal）
- 手动触发工作流

### ⚠️ 4.9. 自动定时任务抓取与报告生成（基本完成）
- ✅ RSS feed抓取服务（gofeed解析）
- ✅ 工作流条件判定逻辑（时间窗口、关键词、标签、时长等）
- ✅ 时间窗口计算（相对时间、绝对时间）
- ✅ 报告生成器（Markdown格式）
- ✅ 失败重试机制（错误计数、阈值控制）
- ✅ 执行记录存储（JobExecution）
- ✅ LLM智能摘要框架（可选功能，Prompt模板管理）
- **待优化**：抓取性能优化、错误处理细化、报告格式丰富化

### ⚠️ 4.10. 全文搜索与UI打磨（进行中）
- ✅ 响应式设计（Tailwind，container + 断点系统）
- ✅ shadcn/ui组件集成（完整的UI组件库）
- ✅ 基础交互反馈（加载状态、错误提示、Toast消息）
- ✅ 全文搜索功能（SQLite FTS5，支持播客和单集联合搜索）
- ✅ 标签筛选集成
- ✅ 无限滚动加载（IntersectionObserver）
- ✅ 图片优化（Next.js Image + 图片代理）
- **待优化**：动画效果、深色模式、无障碍访问、搜索性能优化

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

> **部署和运维指南**：详细的部署配置、运维脚本和最佳实践请参考 [部署运维文档](docs/DEPLOYMENT.md)

## 6. 已知问题和改进方向

### 6.1. 已修复的问题
- ✅ 工作流时间窗口计算使用硬编码时间的bug
- ✅ OPML导入时为不可访问的feed创建无效记录的问题
- ✅ 播客484标题为描述文字的问题
- ✅ 调度器重载时的竞态条件

### 6.2. 待优化项
- ⚠️ 测试覆盖率提升（目标：>70%）
- ⚠️ 前端性能优化（虚拟滚动、懒加载）
- ⚠️ API响应缓存策略
- ⚠️ 数据库查询优化（N+1问题）
- ⚠️ 错误处理和用户提示优化
- ⚠️ 深色模式支持
- ⚠️ 国际化支持（i18n）

### 6.3. 功能增强（未来考虑）
- 基于LLM的智能总结与推荐
- 多租户支持
- 移动端APP（React Native / Flutter）
- 站内外消息推送（邮件、Webhook）
- 播客播放统计
- 收藏单集和评论同步
- 高级搜索功能（热门搜索、搜索建议）

---

**文档版本**: v2.1
**最后更新**: 2025-01-31
**项目状态**: MVP核心功能已完成（约85%），可投入实际使用

> **部署和运维指南**: 详细的部署配置、运维脚本和最佳实践请参考 [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)
