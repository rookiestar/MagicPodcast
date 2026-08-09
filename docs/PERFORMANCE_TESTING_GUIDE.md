# MagicPodcast 性能测试指南

最后更新：2026-08-09

本文只保留当前可复跑的性能检查方式。基线入口见 [BASELINE.md](BASELINE.md)，最新基线见 [performance/BASELINE_2026-05-31.md](performance/BASELINE_2026-05-31.md)。

性能专项开始前先阅读 [性能专项工作手册](optimization/PERFORMANCE_PLAYBOOK.md)，并使用 [性能专项验收模板](optimization/PERFORMANCE_ACCEPTANCE_TEMPLATE.md) 记录用户体验不变量、冷暖态、失败路径和生产证据。

## 准备服务

推荐先用生产模式启动本地服务：

```bash
./scripts/restart.sh --prod
```

确认服务可访问：

```bash
curl http://localhost:8080/health
curl -I http://localhost:3000
```

## 页面和 API 巡检

```bash
node scripts/performance-audit.mjs \
  --base-url http://localhost:3000 \
  --api-url http://localhost:8080 \
  --runs 3
```

检查范围：

- 页面：首页、播客列表、播客详情、标签、导入、工作流列表、工作流详情。
- API：健康检查、播客列表、标签列表、工作流列表、搜索、播客详情、单集列表、播客标签、播客备注、工作流详情、工作流任务。

常用参数：

```bash
node scripts/performance-audit.mjs --runs 5
node scripts/performance-audit.mjs --warmup-runs 0
node scripts/performance-audit.mjs --strict
node scripts/performance-audit.mjs --json
```

默认阈值：

- 页面平均耗时超过 2.5 秒标记为 `SLOW`。
- API 平均耗时超过 800ms 标记为 `SLOW`。
- 页面静态资源超过约 1.5MB 标记为 `HEAVY`。
- 请求失败标记为 `FAIL`。
- 默认每个目标先预热 1 次再采样；如需观察冷态首跳，可设置 `--warmup-runs 0`。

该巡检只测量页面 HTTP 响应、接口和静态资源总量，不能单独证明：

- 有效内容何时出现在页面；
- 慢请求时已有内容是否保留；
- 首次访问是否暴露空白或过早错误态；
- 缓存回访、分页失败或滚动返回是否重复请求；
- 认证公网入口在持续时间窗内是否稳定。

## 性能专项固定验收

用户可见性能改动至少覆盖：

| 场景 | 必须观察 |
| --- | --- |
| 正常请求 | 内容、排序和交互不退化 |
| 慢请求 | 有历史内容时继续可用；无历史内容时骨架稳定 |
| 请求失败 | 保留已有内容，多次失败后才提示并允许重试 |
| 首次访问 | 无缓存时不暴露空白或提前错误态 |
| 缓存回访 | 先显示上次成功内容，后台刷新状态诚实 |
| 分页 / 滚动返回 | 已加载内容保留，不重复下载大资源 |

除 TTFB、API 平均/P95/P99 和静态资源体积外，按专项补充：

- 有效内容出现时间；
- 空态暴露率；
- 请求数、下载字节和重复请求数；
- 缓存命中和热回访传输量；
- 超时、重试和失败率；
- 持续生产门禁成功率。

冷态和热态必须分别报告。`--warmup-runs 0` 可观察 HTTP 冷态，但真实浏览器的首次内容、资源调度和缓存行为仍需单独验收。

## 加载性能回归验收（#13）

这组失败优先验收只使用受控前端假数据，不写真实数据库、不触发同步、工作流或付费能力：

```bash
(cd frontend && npm run test:run -- \
  src/hooks/__tests__/usePodcastListInfinite.acceptance.test.tsx \
  src/components/podcasts/__tests__/podcastLoadingAcceptance.test.tsx)
```

验收覆盖首屏与连续分页、分页挂起超时、服务错误、失败页重试、快速触发去重、封面超时/有限失败重试，以及分页失败时保留已加载节目。生产或集成环境验收必须先确认运行版本标识，并单独取得重启/部署授权；本地受控测试不能替代生产证明。

## 后端并发基准

```bash
(cd backend && BENCHMARK_DURATION=5s BENCHMARK_WORKERS=10 go run ./cmd/benchmark)
```

可调整参数：

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `BENCHMARK_BASE_URL` | `http://localhost:8080` | 后端地址 |
| `BENCHMARK_WORKERS` | `10` | 并发 worker 数 |
| `BENCHMARK_DURATION` | `30s` | 每个接口测试时长 |

该工具会覆盖健康检查、播客列表、全文搜索、标签列表和工作流列表，并输出成功率、吞吐量、平均耗时、P95 和 P99。

## 搜索微基准

搜索服务微基准使用内存数据库和固定样本，不依赖本机真实数据库：

```bash
(cd backend && go test ./internal/services -run '^$' \
  -bench 'Benchmark(SearchService|TextProcessing|RelevanceCalculation)$' \
  -benchtime=10x -count=1)
```

搜索相关改动还应额外跑：

```bash
(cd backend && go test ./internal/services)
(cd backend && go test -race ./internal/services)
```

## 发布前建议

性能相关改动完成后，至少执行：

```bash
(cd backend && go test ./...)
(cd backend && go vet ./...)
(cd frontend && npm run type-check)
(cd frontend && npm run lint)
(cd frontend && npm run test:run)
(cd frontend && npm run build)
```

然后重新跑页面/API 巡检和后端并发基准，并把结果更新到最新基线文档。
