# MagicPodcast 历史归档

最后更新：2026-05-31

这里保存已经不作为当前事实依据的阶段报告、测试报告和历史重构总结。它们用于追溯背景，不用于判断当前代码状态。

## 归档目录

| 目录 | 内容 | 说明 |
| --- | --- | --- |
| [legacy-reports/](legacy-reports/) | 历史阶段报告、测试报告、最终总结和原始基线长文 | 多数形成于 2026-02-01 左右，内容可能已被当前路线图和基线取代 |
| [backend-reports/](backend-reports/) | 原 `backend/` 下的历史重构总结 | 只保留追溯用途，当前后端状态以源码和测试为准 |
| [reports/](reports/) | 原 `docs/reports/` 下的历史修复和分析报告，以及旧项目搬家、性能优化阶段记录、旧 PodcastIndex 视图说明 | 不再作为当前状态入口 |
| [planning/](planning/) | 原 `docs/planning/` 下的旧项目分析、清理计划、阶段规划、旧设计草案和旧页面优化计划 | 当前计划以 `REFACTORING_ROADMAP.md` 为准 |

## 使用规则

1. 需要当前状态时，优先看 [../README.md](../README.md)、[../REFACTORING_ROADMAP.md](../REFACTORING_ROADMAP.md) 和 [../performance/BASELINE_2026-05-31.md](../performance/BASELINE_2026-05-31.md)。
2. 引用归档内容前，必须用当前源码、测试、构建或运行结果重新验证。
3. 新的当前维护文档不要放进归档目录；只有过期报告或阶段总结才迁入这里。
4. 旧项目搬家记录见 [reports/PROJECT_LOCATION_MIGRATION_2026-01-21.md](reports/PROJECT_LOCATION_MIGRATION_2026-01-21.md)，当前数据库迁移入口见 [../migration/MIGRATION_GUIDE.md](../migration/MIGRATION_GUIDE.md)。
5. 旧性能优化阶段记录见 [reports/PERFORMANCE_OPTIMIZATION_PLAN_2026-05-19.md](reports/PERFORMANCE_OPTIMIZATION_PLAN_2026-05-19.md)，当前性能入口见 [../optimization/README.md](../optimization/README.md)。
6. 原始基线长文见 [legacy-reports/BASELINE_2026-02-01.md](legacy-reports/BASELINE_2026-02-01.md)，当前基线入口见 [../BASELINE.md](../BASELINE.md)。
7. 旧 PodcastIndex 视图 Schema 长文见 [reports/UNIQUE_PODCASTS_VIEW_SCHEMA_2026-01-21.md](reports/UNIQUE_PODCASTS_VIEW_SCHEMA_2026-01-21.md)，当前去重视图说明见 [../migration/PODCASTINDEX_DEDUP.md](../migration/PODCASTINDEX_DEDUP.md)。
