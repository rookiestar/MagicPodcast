# 项目：MagicPodcast

## 1. 产品目标

本产品主要解决个人在使用「小宇宙」播客APP时平台暂时无法提供的个人数据管理功能（站外），并基于日常消费播客节目时所需要的工作流提升信息筛选与吸收的效率，大幅提升播客消费场景中的个人体验。

## 2. 项目范围
### 2.1. MVP版本只做什么

* MagicPodcast（Web应用+定时脚本）
  * 小宇宙个人数据管理（MyPodcasts）
    * 登录态（复用小宇宙）：本地存储用户的小宇宙登录凭据（例如通过第三方封装 API 或自建代理服务获取 token），不做独立账号体系。
    * 数据同步：
      * 每天定时拉取「我的订阅」节目列表。
      * 支持按需手动「立即同步」。
    * 本地建模与持久化：
      * 储存实体：节目（podcast）、单集（episode）、标签（tag）。
      * 支持手动为节目 / 单集打标签、写备注等自定义信息。
    * 展示界面（管理端）：
      * 订阅列表页：按节目展示 + 搜索 / 筛选 / 排序。
      * 节目详情页：所有单集、本地标签、备注与其他自定义信息（仅标签及自定义信息支持读写，其他来源小宇宙的原始信息只读不可删改）。
      * 标签视图：按标签浏览关联的节目 / 单集。
  * 播客数据处理工作流（MagicFlow）
    * 工作流展示界面：
      * 工作流列表页：展示已创建的工作流，支持创建、编辑、删除、禁用（暂停定时任务）。
      * 工作流详情页：
        * 定义：在「我的订阅」节目列表和自定义节目源（需解析）的范围内筛选待自动抓取的节目。
        * 报告：成功抓取的工作流支持按照任务完成时序展示信息报告与抓取日志。
      * 自定义节目源解析：支持通过单个或批量节目名称调用搜索引擎获取播客在小宇宙平台上的URL，再通过ID抽取和组装生成RSS源URL。解析成功的自定义节目保存下来。
    * 数据抓取：
      * 条件判定：基于工作流定义的节目范围逐个判断是否符合抓取条件。
      * 信息抓取：将该节目符合条件的单集相关信息抓取到本地。
    * 本地建模与持久化：
      * 储存实体：工作流（workflow）、定时任务（job）、报告（report）。

### 2.2. MVP版本不做 / 延后

* 不做多租户（先支持我自己用）。
* 不做移动APP，先做响应式网页。
* 不做收藏单集和评论的同步。
* 不做基于LLM的智能总结与推荐，先验证播客元信息可被正常抓取与展示。
* 不做站内外的消息推送。

## 3. 技术栈与架构
### 3.1. 后端 / 同步层：

- **语言**: Go 1.21+
- **框架**:
  - Web 框架: Gin (github.com/gin-gonic/gin)
  - ORM: GORM (gorm.io/gorm)
  - 配置管理: Viper (github.com/spf13/viper)
  - 日志: logrus (github.com/sirupsen/logrus)
  - HTTP 客户端: resty (github.com/go-resty/resty)
  - 定时任务: robfig/cron (github.com/robfig/cron/v3)
- **职责**:
  - 封装小宇宙 API：
    - 集成 ultrazg/xyz 服务（HTTP 调用或直接导入 Go 代码）
    - 提供统一的 REST 接口给前端（如：`GET /api/subscriptions`、`POST /api/sync`）
  - 维护数据库：
    - 节目、单集、本地标签、备注
    - 工作流、定时任务、报告
  - 同步任务：
    - 使用 robfig/cron 实现定时任务（每天凌晨 2 点同步）
    - 支持手动触发同步
  - 数据初始化：
    - 对「我的订阅」列表中的节目，可以批量从PodcastIndex.org下载的本地数据库中复制相关数据已实现快速初始化（阶段 5 实现）。
    - 对于PodcastIndex.org中没有索引到的节目，再通过「节目源解析」功能从小宇宙拉取相关数据完成初始化。

### 3.2. 数据库 / 持久层：

* 选择：本地自用SQLite。
* 核心表（简化草稿版本）：
  * `podcasts`：id, xyz_id, title, feed_url, itunes_id, podcast_guid, desc, author, cover, added_date, episodes, newest_episode_date, is_subscribed, is_dead, my_rate, notes。
  * `episodes`：id, xyz_id, podcast_id, episode_no, title, medium_url, show_notes, published_date, my_rate, notes。
  * `tags`：id, name, desc。
  * `episodes_tags` / `podcasts_tags`：多对多关联。
  * `workflows`: id, title, desc, conditions, is_enabled。
  * `jobs`: id, workflow_id, status, last_execution。
  * `reports`: id, jobs_id, execution_id, report_body。
  * `jobs_reports`: 一对多关联。
  * `job_executions`: id, job_id, start_time, end_time, status, log_info。

### 3.3. 前端 / 展示层：

- **框架**: React + Next.js 14+ (App Router)
- **UI**: Tailwind CSS + shadcn/ui
- **语言**: TypeScript
- **状态管理**: React Hooks + Context API (或 Zustand，如需要)
- **HTTP 客户端**: fetch 或 axios
- 页面：
  - `/`：总览 / 快速入口。
  - `/podcasts`：订阅节目列表。
  - `/podcasts/[id]`：节目详情 + 单集列表 + 标签/备注操作。
  - `/tags`：标签列表 + 点击看对应内容。
  - `/workflows`：工作流列表。
  - `/workflows[id]`：工作流详情 + 报告列表 + 定时任务执行日志 + 工作流编辑操作。

### 3.4. 小宇宙集成层：

- 使用开源 API 代理（如 ultrazg/xyz 等），通过手机号登录获取「我的订阅」与节目/单集详情。

## 4. 功能切片
按「纵向切片」一条一条完成，每一条都能从前端点到后端到数据库。

### 4.1. 项目骨架 + DB 初始化

### 4.2. 订阅列表页（假数据）

### 4.3. 节目详情页（假数据）

### 4.4. 标签与备注系统

### 4.5. 打通小宇宙手动同步

### 4.6. 自动定时同步

### 4.7. 工作流列表页（假数据）

### 4.8. 工作流详情页（假数据）

### 4.9. 自动定时任务抓取与报告生成

### 4.10. UI 打磨

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
  - 在数据库模型中保留“最后抓取时间”等字段，便于实现增量策略。
- 定时运行场景
  - 可在无交互的命令行环境执行。
  - 通过环境变量或配置文件读取必要配置（例如代理、并发限制）。
  - 当某些外部条件不满足时（网络错误、目标站点变更等），要有可理解的日志输出，而不是静默失败或崩溃。

### 5.3. Vibe Coding偏好

- 每次只实现一个切片，端到端打通。
- 每个切片先写 5～10 步小计划，再编写代码。
- 发现重复逻辑时，提出抽象和重构建议。
- 变更后立即本地运行和手测，必要时重构和补测试。
- 在合适的时机建议添加简单的可视化（例如用文本/表格输出热门节目统计）。
- 提醒我注意部署和定时任务相关的配置，并帮我生成对应的配置文件草稿。
