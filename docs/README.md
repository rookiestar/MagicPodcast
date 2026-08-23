# MagicPodcast 文档中心

最后更新：2026-08-23

这份索引用来区分“当前维护文档”和“历史记录”。日常开发、部署、测试和重构优先看当前维护文档；阶段性总结和历史分析只作为查证背景，不作为最新状态依据。

Agent 治理以根目录 [../AGENTS.md](../AGENTS.md) 为唯一权威合同；[../CLAUDE.md](../CLAUDE.md) 仅转发到该合同。日常验证与 Issue 验收见 [AGENT_VERIFICATION.md](AGENT_VERIFICATION.md)；生产发布与回退见 [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md)。

## 当前维护文档

| 主题 | 文档 | 用途 |
| --- | --- | --- |
| 项目总览 | [../README.md](../README.md) | 项目定位、启动方式、常用检查 |
| Agent 合同 | [../AGENTS.md](../AGENTS.md) | 唯一权威 Agent 治理入口（权限、验证、生产/库、Git/Subagent） |
| Agent 验证 | [AGENT_VERIFICATION.md](AGENT_VERIFICATION.md) | 按风险定向检查与 Issue 验收证据分层 |
| 维护路线 | [REFACTORING_ROADMAP.md](REFACTORING_ROADMAP.md) | 当前重构进度和下一步优先级 |
| 专项收尾 | [AUTOMATED_REFACTORING_CLOSEOUT.md](AUTOMATED_REFACTORING_CLOSEOUT.md) | 本轮自动化重构专项总结、验证结果和下一步 |
| 人审队列 | [HUMAN_REVIEW_QUEUE.md](HUMAN_REVIEW_QUEUE.md) | 自动跳过、需要确认后再处理的事项 |
| 发布检查 | [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) | 重构、升级、部署前的固定验证步骤 |
| 远程生产发布 | [REMOTE_PRODUCTION_DEPLOYMENT.md](REMOTE_PRODUCTION_DEPLOYMENT.md) | 唯一远程操作手册：Self-hosted Runner、审批、固定 SHA 发布与回退 |
| 性能基线 | [performance/BASELINE_2026-05-31.md](performance/BASELINE_2026-05-31.md) | 最新页面、接口和后端并发基线 |
| 性能专项方法 | [optimization/PERFORMANCE_PLAYBOOK.md](optimization/PERFORMANCE_PLAYBOOK.md) | 性能专项的体验不变量、根因分析、方案顺序和生产收口 |
| 性能验收模板 | [optimization/PERFORMANCE_ACCEPTANCE_TEMPLATE.md](optimization/PERFORMANCE_ACCEPTANCE_TEMPLATE.md) | 正常、慢、失败、首次访问与冷暖态验收记录 |
| Inbox 最近完成验收 | [optimization/INBOX_RECENT_COMPLETIONS_ACCEPTANCE_2026-08-23.md](optimization/INBOX_RECENT_COMPLETIONS_ACCEPTANCE_2026-08-23.md) | #169 本地正常、慢、失败、首次访问和多视口证据 |
| 播客列表性能专项 | [optimization/PODCASTS_PERFORMANCE_AND_SHANGHAI_RELAY_2026-08-09.md](optimization/PODCASTS_PERFORMANCE_AND_SHANGHAI_RELAY_2026-08-09.md) | `/podcasts` 应用优化、公网瓶颈、上海私网双路径、运维与回退 |
| 首页主路径验收 | [optimization/HOME_PRIMARY_PATH_ACCEPTANCE_2026-08-16.md](optimization/HOME_PRIMARY_PATH_ACCEPTANCE_2026-08-16.md) | 上海中继正式主路径的运行态、冷载证据和剩余人审 |
| 正式访问路径 | [runbooks/PRIMARY_ACCESS_PATH.md](runbooks/PRIMARY_ACCESS_PATH.md) | 上海中继主路径、Cloudflare 备用、验收和故障切换 |
| 基线索引 | [BASELINE.md](BASELINE.md) | 当前基线入口和历史基线归档位置 |
| 部署运维 | [DEPLOYMENT.md](DEPLOYMENT.md) | 启动、部署、服务管理 |
| 环境配置 | [ENV_SETUP.md](ENV_SETUP.md) | 前后端环境变量说明 |
| 数据 Profile | [DATA_PROFILES.md](DATA_PROFILES.md) | 隔离网络下 Fixture/Snapshot 切换、刷新与安全边界 |
| 数据 Profile Skill | [../.agents/skills/magicpodcast-data-profile/SKILL.md](../.agents/skills/magicpodcast-data-profile/SKILL.md) | Agent 的 status-first 安全操作入口；只调用项目统一命令 |
| 备份恢复 | [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md) | SQLite 备份、验证和恢复 |
| 数据库迁移 | [migration/MIGRATION_GUIDE.md](migration/MIGRATION_GUIDE.md) | 当前迁移入口、索引补齐和高风险手工迁移边界 |
| 数据库索引 | [DATABASE_INDEX_GUIDE.md](DATABASE_INDEX_GUIDE.md) | 索引与查询性能说明 |
| 清理规则 | [CLEAN_GUIDE.md](CLEAN_GUIDE.md) | 本地构建、日志和临时文件清理 |
| 前端测试 | [FRONTEND_TESTING_SETUP.md](FRONTEND_TESTING_SETUP.md) | Vitest 和前端测试配置 |
| 字体排版 | [design/TYPOGRAPHY.md](design/TYPOGRAPHY.md) | 字体角色、语义字号与统一富文本规范 |
| 性能测试 | [PERFORMANCE_TESTING_GUIDE.md](PERFORMANCE_TESTING_GUIDE.md) | 性能测试方法和工具 |
| 依赖健康 | [DEPENDENCY_REVIEW.md](DEPENDENCY_REVIEW.md) | 前后端依赖审计、升级边界和剩余风险 |
| 工作流调度 | [WORKFLOW_SCHEDULER.md](WORKFLOW_SCHEDULER.md) | 当前工作流定时执行行为和调度接口 |
| 工作流执行历史 | [WORKFLOW_EXECUTION_HISTORY.md](WORKFLOW_EXECUTION_HISTORY.md) | 当前执行历史、任务详情和报告入口行为 |
| 设计系统 | [design/DESIGN_SYSTEM.md](design/DESIGN_SYSTEM.md) | 前端视觉与组件规范 |
| 领域词汇表 | [DOMAIN_GLOSSARY.md](DOMAIN_GLOSSARY.md) | 项目共享领域词与相近词边界 |

## 专题目录

| 目录 | 内容 |
| --- | --- |
| [design/](design/) | 设计系统和界面规范 |
| [migration/](migration/) | 数据迁移、去重和兼容处理 |
| [optimization/README.md](optimization/README.md) | 当前性能优化入口和下一步边界 |
| [performance/](performance/) | 当前和后续性能基线 |
| [research/](research/) | **研究与方案**（非默认生产事实）：一手研究、ADR、设计提案与尚未完全落地的 Spec；落地前须用源码/测试核对 |
| [runbooks/](runbooks/) | 运维诊断手册；步骤须与当前实现和已批准 Spec 的细粒度策略一致 |
| [archive/](archive/) | **历史材料**：已从当前入口移出的阶段报告和测试报告，不作为最新状态依据 |

分阶段落地的方案：

- [Inbox Done 与完成历史设计](research/INBOX_DONE_HISTORY_DESIGN_2026-08-23.md)：#169 已交付完成事实、最近完成与 15 秒撤销；独立完成历史由 #170 继续交付。两阶段的本地实现均不代表生产已迁移或部署。

文档角色速查：

| 角色 | 位置 | Agent 用法 |
| --- | --- | --- |
| 当前事实 | 本页「当前维护文档」、README、源码 | 可直接作为现状依据 |
| 研究 / 方案 | `research/`、部分 planning 归档前草案 | 区分已实现 / 仅决策 / 未实现；冲突时以源码为准 |
| 历史归档 | `docs/archive/`、根目录 `archive/` | 只追溯，引用前必须重验证 |

## 历史记录

以下文档主要记录过去的阶段性工作，内容可能与当前代码不完全一致。需要引用时，应先用测试、构建或源码验证：

- [archive/legacy-reports/](archive/legacy-reports/) 下的 `PHASE*`、`FINAL*` 和 `*_REPORT` 历史文档
- [archive/legacy-reports/BASELINE_2026-02-01.md](archive/legacy-reports/BASELINE_2026-02-01.md) 是 2026-02-01 原始基线长文，不代表当前项目状态
- [archive/backend-reports/](archive/backend-reports/) 下的后端历史重构总结
- [archive/reports/](archive/reports/) 下的历史修复和分析报告
- [archive/planning/](archive/planning/) 下的旧项目分析、清理计划和阶段规划
- [archive/planning/WORKFLOW_SCHEDULER_DESIGN.md](archive/planning/WORKFLOW_SCHEDULER_DESIGN.md) 是旧工作流调度设计草案，不代表当前实现
- [archive/planning/LAYOUT_MIGRATION_GUIDE.md](archive/planning/LAYOUT_MIGRATION_GUIDE.md) 是旧布局迁移草案，当前页面已统一使用布局组件
- [archive/planning/JOB_EXECUTION_DETAIL_DESIGN.md](archive/planning/JOB_EXECUTION_DETAIL_DESIGN.md) 是旧执行历史详情设计草案，不代表当前实现
- [archive/reports/PROJECT_LOCATION_MIGRATION_2026-01-21.md](archive/reports/PROJECT_LOCATION_MIGRATION_2026-01-21.md) 是旧项目搬家记录，不代表当前启动和迁移方式
- [archive/reports/PERFORMANCE_OPTIMIZATION_PLAN_2026-05-19.md](archive/reports/PERFORMANCE_OPTIMIZATION_PLAN_2026-05-19.md) 是旧性能优化阶段记录，当前性能数据以最新基线为准
- [archive/reports/UNIQUE_PODCASTS_VIEW_SCHEMA_2026-01-21.md](archive/reports/UNIQUE_PODCASTS_VIEW_SCHEMA_2026-01-21.md) 是旧 PodcastIndex 视图 Schema 长文，当前说明以 `migration/PODCASTINDEX_DEDUP.md` 为准
- [archive/planning/PODCASTS_PAGE_OPTIMIZATION_PLAN.md](archive/planning/PODCASTS_PAGE_OPTIMIZATION_PLAN.md) 是旧播客列表视觉优化计划，不代表当前待办

## 使用建议

1. 编码代理先读 [../AGENTS.md](../AGENTS.md)；验证按 [AGENT_VERIFICATION.md](AGENT_VERIFICATION.md)，不要把发布清单当日常验证入口。
2. 新任务再看 [../README.md](../README.md) 和 [REFACTORING_ROADMAP.md](REFACTORING_ROADMAP.md)。
3. 遇到不适合自动处理的清理项，记录到 [HUMAN_REVIEW_QUEUE.md](HUMAN_REVIEW_QUEUE.md)。
4. 准备部署或回退时按 [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) 执行；性能对比用 [performance/BASELINE_2026-05-31.md](performance/BASELINE_2026-05-31.md) 中的命令。
5. 涉及真实数据前，先看 [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md)。
6. 历史报告与 `research/` 中未落地 Spec 只作背景，不直接当成当前生产事实。
