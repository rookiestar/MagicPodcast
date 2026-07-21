# MagicPodcast 文档中心

最后更新：2026-07-21

这份索引用来区分“当前维护文档”和“历史记录”。日常开发、部署、测试和重构优先看当前维护文档；阶段性总结和历史分析只作为查证背景，不作为最新状态依据。

## 当前维护文档

| 主题 | 文档 | 用途 |
| --- | --- | --- |
| 项目总览 | [../README.md](../README.md) | 项目定位、启动方式、常用检查 |
| 维护路线 | [REFACTORING_ROADMAP.md](REFACTORING_ROADMAP.md) | 当前重构进度和下一步优先级 |
| 专项收尾 | [AUTOMATED_REFACTORING_CLOSEOUT.md](AUTOMATED_REFACTORING_CLOSEOUT.md) | 本轮自动化重构专项总结、验证结果和下一步 |
| 人审队列 | [HUMAN_REVIEW_QUEUE.md](HUMAN_REVIEW_QUEUE.md) | 自动跳过、需要确认后再处理的事项 |
| 发布检查 | [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) | 重构、升级、部署前的固定验证步骤 |
| 性能基线 | [performance/BASELINE_2026-05-31.md](performance/BASELINE_2026-05-31.md) | 最新页面、接口和后端并发基线 |
| 基线索引 | [BASELINE.md](BASELINE.md) | 当前基线入口和历史基线归档位置 |
| 部署运维 | [DEPLOYMENT.md](DEPLOYMENT.md) | 启动、部署、服务管理 |
| 环境配置 | [ENV_SETUP.md](ENV_SETUP.md) | 前后端环境变量说明 |
| 备份恢复 | [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md) | SQLite 备份、验证和恢复 |
| 数据库迁移 | [migration/MIGRATION_GUIDE.md](migration/MIGRATION_GUIDE.md) | 当前迁移入口、索引补齐和高风险手工迁移边界 |
| 数据库索引 | [DATABASE_INDEX_GUIDE.md](DATABASE_INDEX_GUIDE.md) | 索引与查询性能说明 |
| 清理规则 | [CLEAN_GUIDE.md](CLEAN_GUIDE.md) | 本地构建、日志和临时文件清理 |
| 前端测试 | [FRONTEND_TESTING_SETUP.md](FRONTEND_TESTING_SETUP.md) | Vitest 和前端测试配置 |
| 性能测试 | [PERFORMANCE_TESTING_GUIDE.md](PERFORMANCE_TESTING_GUIDE.md) | 性能测试方法和工具 |
| 依赖健康 | [DEPENDENCY_REVIEW.md](DEPENDENCY_REVIEW.md) | 前后端依赖审计、升级边界和剩余风险 |
| 工作流调度 | [WORKFLOW_SCHEDULER.md](WORKFLOW_SCHEDULER.md) | 当前工作流定时执行行为和调度接口 |
| 工作流执行历史 | [WORKFLOW_EXECUTION_HISTORY.md](WORKFLOW_EXECUTION_HISTORY.md) | 当前执行历史、任务详情和报告入口行为 |
| 设计系统 | [design/DESIGN_SYSTEM.md](design/DESIGN_SYSTEM.md) | 前端视觉与组件规范 |

## 专题目录

| 目录 | 内容 |
| --- | --- |
| [design/](design/) | 设计系统和界面规范 |
| [migration/](migration/) | 数据迁移、去重和兼容处理 |
| [optimization/README.md](optimization/README.md) | 当前性能优化入口和下一步边界 |
| [performance/](performance/) | 当前和后续性能基线 |
| [research/](research/) | 一手研究、已确认决策与设计提案；引用时需区分生产事实、ADR 和尚未实现的 Spec |
| [archive/](archive/) | 已从当前入口移出的历史阶段报告和测试报告 |

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

1. 新任务先看 [../README.md](../README.md) 和 [REFACTORING_ROADMAP.md](REFACTORING_ROADMAP.md)。
2. 遇到不适合自动处理的清理项，记录到 [HUMAN_REVIEW_QUEUE.md](HUMAN_REVIEW_QUEUE.md)。
3. 改动前后先按 [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) 复查，再按 [performance/BASELINE_2026-05-31.md](performance/BASELINE_2026-05-31.md) 里的命令做性能对比。
4. 涉及真实数据前，先看 [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md)。
5. 历史报告只作背景，不直接当成当前事实。
