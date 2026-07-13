# AGENTS.md

## 项目定位

MagicPodcast 是个人播客管理与自动化处理工具。后端位于 `backend/`，使用 Go、Gin、GORM 和 SQLite；前端位于 `frontend/`，使用 Next.js、React、TypeScript、Tailwind CSS 和 SWR。

仓库中的当前源码、测试结果和运行结果优先于历史总结。`docs/archive/` 与根目录 `archive/` 仅用于历史追溯，不能直接作为当前实现依据。

## 开始前必读

非琐碎任务至少阅读：

1. `README.md`
2. `docs/README.md`
3. 与任务直接相关的源码、测试和文档

按任务补充阅读：

- 重构或清理：`docs/REFACTORING_ROADMAP.md`、`docs/HUMAN_REVIEW_QUEUE.md`、`docs/RELEASE_CHECKLIST.md`
- 数据库、迁移或维护命令：`docs/BACKUP_RECOVERY.md`、`docs/migration/MIGRATION_GUIDE.md`、`backend/cmd/README.md`
- 工作流或调度：`docs/WORKFLOW_SCHEDULER.md`、`docs/WORKFLOW_EXECUTION_HISTORY.md`
- 前端视觉或交互：`docs/design/DESIGN_SYSTEM.md`、`docs/FRONTEND_TESTING_SETUP.md`
- 性能：`docs/PERFORMANCE_TESTING_GUIDE.md`、`docs/performance/BASELINE_2026-05-31.md`

小范围文字、注释或单一文档修正，只需阅读直接相关文件并完成对应检查。

## 标准入口

生产模式启动：

```bash
./scripts/start.sh --prod
```

重启与健康检查：

```bash
./scripts/restart.sh --prod
./scripts/health-check.sh
```

后端验证：

```bash
(cd backend && go test ./...)
(cd backend && go vet ./...)
```

前端验证：

```bash
(cd frontend && npm run type-check)
(cd frontend && npm run test:run)
```

前端生产构建只在涉及构建、依赖、路由或发布风险时运行：

```bash
(cd frontend && npm run build)
```

性能检查只在性能、启动或接口时延相关任务中运行：

```bash
node scripts/performance-audit.mjs \
  --base-url http://localhost:3000 \
  --api-url http://localhost:8080 \
  --runs 3 \
  --strict
```

## 安全与授权

无需额外确认即可：

- 阅读、列出和搜索仓库文件。
- 查看 Git 状态和差异。
- 运行与当前改动直接相关、不会写入真实数据的测试和静态检查。
- 在用户要求的范围内修改仓库文件，并同步更新相关文档。

执行以下操作前必须取得用户明确授权：

- `git add`、`git commit`、`git push` 或创建 PR。
- 安装、删除或升级依赖，以及修改 `go.mod`、`go.sum`、`package.json`、`package-lock.json`。
- 启动、停止或重启本机服务，运行生产构建或完整性能巡检。
- 删除文件或目录、深度清理、覆盖配置或使用破坏性 Git 命令。
- 对真实数据库执行迁移、恢复、导入、同步、维护命令或手工 SQL。
- 运行 `backend/cmd/migrate`、`backend/cmd/maint/` 或其他可能写入真实数据的入口。
- 改变数据库结构、公开 API、搜索排序、缓存语义、调度行为、通知策略或用户可见交互。
- 跨主版本升级 Next.js、Go 或其他核心依赖。

不得自动删除或提交本地配置、数据库、日志、备份及可能包含敏感信息的文件。不要使用 `--force` 或跳过安全备份来绕过数据保护。

## 改动原则

- 先确认真实仓库根目录和 `git status --short`，保护用户已有改动。
- 优先小范围、可审阅、可验证的修改，不做无关重写。
- 保持前端、处理器、服务、仓储和数据库之间的行为一致。
- 不为同一功能增加第二套并行实现。
- 不通过删除测试、放宽校验、吞掉错误或降低安全检查来让验证通过。
- 前端改动应保持现有设计系统；交互改动除单元测试外，还应在可用时做页面查看和点击验证。
- 涉及真实数据前，先确认目标数据库、备份、服务状态和恢复方法。

## 项目级技能

仓库级技能位于 `.agents/skills/`：

- `magicpodcast-code-change-verification`：任何代码或脚本改动完成后使用。
- `magicpodcast-refactor-planning`：重构、清理或模块边界调整前使用。
- `magicpodcast-database-change-guard`：迁移、恢复、导入、维护脚本或任何真实数据写入前使用。
- `magicpodcast-remote-deployment`：部署、启动、重启或验收 Mac mini 生产环境前使用。

技能不能扩大用户授权范围。技能要求暂停确认时，必须先取得用户回复再继续。

## Agent skills

### Issue tracker

PRD 和任务统一存放在 GitHub Issues；外部 Pull Request 不作为需求入口。详见 `docs/agents/issue-tracker.md`。

### Triage labels

使用 `needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix` 五种标准状态。详见 `docs/agents/triage-labels.md`。

### Domain docs

本项目采用单一领域上下文，统一读取根目录 `CONTEXT.md` 和 `docs/adr/`。详见 `docs/agents/domain.md`。

## 验证与完成标准

每次修改后先运行最小相关检查，再按风险增加检查。不要用全量检查代替更有针对性的验证。

任务只有在以下条件满足时才算完成：

1. 用户要求的结果已经实现。
2. 相关验证实际运行，结果已检查。
3. 失败或跳过的检查及原因明确说明。
4. 没有把无关文件或用户已有改动混入本次范围。
5. 文档、命令和当前行为保持一致。

最终汇报使用简洁中文，固定说明：改了什么、验证了什么、跳过了什么、剩余风险。不得把未运行的检查说成通过。

## 文档维护

优先更新最接近主题的现有文档，不随意新增阶段总结：

- 项目入口或常用命令：`README.md`
- 文档索引：`docs/README.md`
- 重构状态与后续边界：`docs/REFACTORING_ROADMAP.md`
- 需要人工确认的事项：`docs/HUMAN_REVIEW_QUEUE.md`
- 发布验证：`docs/RELEASE_CHECKLIST.md`
- 数据库与恢复：`docs/BACKUP_RECOVERY.md` 或 `docs/migration/`
- 工作流：`docs/WORKFLOW_SCHEDULER.md` 或 `docs/WORKFLOW_EXECUTION_HISTORY.md`
- 前端规范：`docs/design/DESIGN_SYSTEM.md`

## 子代理与 Git

- 未经用户明确批准，禁止启用 Subagent。
- 仅在用户明确要求委派、并行处理或使用子代理时启用子代理。
- 不并行修改同一批文件；主代理负责最终判断、验证和汇报。
- 不覆盖用户改动，不执行破坏性 Git 命令。
- 未经明确要求，不提交、不推送、不创建 PR。
