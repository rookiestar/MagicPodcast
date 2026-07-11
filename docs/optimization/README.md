# MagicPodcast 性能优化入口

最后更新：2026-05-31

本文作为当前性能优化入口。阶段性优化记录已移入归档，当前判断以最新可复跑基线和巡检脚本为准。

## 当前依据

- 最新基线：[../performance/BASELINE_2026-05-31.md](../performance/BASELINE_2026-05-31.md)
- 性能测试方法：[../PERFORMANCE_TESTING_GUIDE.md](../PERFORMANCE_TESTING_GUIDE.md)
- 历史优化记录：[../archive/reports/PERFORMANCE_OPTIMIZATION_PLAN_2026-05-19.md](../archive/reports/PERFORMANCE_OPTIMIZATION_PLAN_2026-05-19.md)
- 旧播客列表视觉计划：[../archive/planning/PODCASTS_PAGE_OPTIMIZATION_PLAN.md](../archive/planning/PODCASTS_PAGE_OPTIMIZATION_PLAN.md)

## 当前瓶颈

| 方向 | 当前状态 | 下一步边界 |
| --- | --- | --- |
| 搜索冷启动 | 预热后平均约 127ms、P95 约 127ms；重启后第一跳仍可能超过 500ms | 继续调整排序、缓存或查询策略前需要确认搜索结果顺序是否允许变化 |
| 页面资源体积 | 核心页面资源集中在约 648KB 到 752KB | 后续优先看播客详情、播客列表和工作流详情 |
| 后端并发 | 当前基准成功率 100%，全文搜索 P95 约 187ms | 作为后续重构的回归参考，不在无人值守阶段改变接口语义 |

## 复查命令

```bash
./scripts/restart.sh --prod

node scripts/performance-audit.mjs \
  --base-url http://localhost:3000 \
  --api-url http://localhost:8080 \
  --runs 3 \
  --strict

(cd backend && BENCHMARK_DURATION=5s BENCHMARK_WORKERS=10 go run ./cmd/benchmark)
```

搜索相关改动还应运行：

```bash
(cd backend && go test ./internal/services)
(cd backend && go test -race ./internal/services)
(cd backend && go test ./internal/services -run '^$' \
  -bench 'Benchmark(SearchService|TextProcessing|RelevanceCalculation)$' \
  -benchtime=10x -count=1)
```

## 更新规则

1. 新性能数据写入 [../performance/](../performance/) 下的最新基线。
2. 阶段性过程记录完成后移入 [../archive/](../archive/)。
3. 不把旧公开域名、旧本地端口或单次历史截图当成当前事实。
4. 任何可能改变搜索结果顺序、接口字段或页面行为的优化，先进入 [../HUMAN_REVIEW_QUEUE.md](../HUMAN_REVIEW_QUEUE.md)。
