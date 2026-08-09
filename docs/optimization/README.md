# MagicPodcast 性能优化入口

最后更新：2026-08-09

本文作为当前性能优化入口。阶段性优化记录已移入归档，当前判断以最新可复跑基线和巡检脚本为准。

## 当前依据

- 性能专项工作方法：[PERFORMANCE_PLAYBOOK.md](PERFORMANCE_PLAYBOOK.md)
- 性能专项验收模板：[PERFORMANCE_ACCEPTANCE_TEMPLATE.md](PERFORMANCE_ACCEPTANCE_TEMPLATE.md)
- `/podcasts` 性能专项与上海私网中继总结：[PODCASTS_PERFORMANCE_AND_SHANGHAI_RELAY_2026-08-09.md](PODCASTS_PERFORMANCE_AND_SHANGHAI_RELAY_2026-08-09.md)
- 最新基线：[../performance/BASELINE_2026-05-31.md](../performance/BASELINE_2026-05-31.md)
- 性能测试方法：[../PERFORMANCE_TESTING_GUIDE.md](../PERFORMANCE_TESTING_GUIDE.md)
- 历史优化记录：[../archive/reports/PERFORMANCE_OPTIMIZATION_PLAN_2026-05-19.md](../archive/reports/PERFORMANCE_OPTIMIZATION_PLAN_2026-05-19.md)
- 旧播客列表视觉计划：[../archive/planning/PODCASTS_PAGE_OPTIMIZATION_PLAN.md](../archive/planning/PODCASTS_PAGE_OPTIMIZATION_PLAN.md)

## 基线记录中的待关注项

以下数值来自 2026-05-31 基线，只用于说明复查方向。开始新专项前必须重新测量，不能直接当作当前生产事实。

| 方向 | 基线状态 | 下一步边界 |
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

1. 新专项先按 [PERFORMANCE_PLAYBOOK.md](PERFORMANCE_PLAYBOOK.md) 定义用户体验不变量、证据链和方案边界。
2. 正常、慢、失败、首次访问与关键指标写入 [PERFORMANCE_ACCEPTANCE_TEMPLATE.md](PERFORMANCE_ACCEPTANCE_TEMPLATE.md)。
3. 新性能数据写入 [../performance/](../performance/) 下的最新基线。
4. 阶段性过程记录完成后移入 [../archive/](../archive/)。
5. 不把旧公开域名、旧本地端口或单次历史截图当成当前事实。
6. 任何可能改变搜索结果顺序、接口字段、加载态、空态、错误态、超时、缓存、陈旧数据或重试策略的优化，必须单独人审；需要跟踪时进入 [../HUMAN_REVIEW_QUEUE.md](../HUMAN_REVIEW_QUEUE.md)。
