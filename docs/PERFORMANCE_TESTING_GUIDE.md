# MagicPodcast 性能测试指南

最后更新：2026-05-31

本文只保留当前可复跑的性能检查方式。基线入口见 [BASELINE.md](BASELINE.md)，最新基线见 [performance/BASELINE_2026-05-31.md](performance/BASELINE_2026-05-31.md)。

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

## 加载性能回归验收

加载性能回归验收用于守住“封面永久灰块、页尾永久‘加载更多…’、分页失败丢失已加载节目”等可见症状（对应 PRD #11 / 子任务 #13、#14、#12）。它分为两层，分别对应不同授权范围，不要混用。

### 本地受控验收（无需后端、不触碰真实数据）

纯前端 vitest，使用 happy-dom 与受控 `fetch` 模拟，不发起到真实后端的请求，也不写入数据库或触发同步/工作流，可随时运行：

```bash
(cd frontend && npx vitest run \
  src/hooks/__tests__/usePodcastListInfinite.acceptance.test.tsx \
  src/components/podcasts/__tests__/podcastLoadingAcceptance.test.tsx)
```

覆盖内容：

- 无限滚动：三页连续加载、分页请求长时间不返回时必须离开永久“加载更多…”、分页失败保留已加载节目且可单独重试、快速触发不重复请求同一页、到底后不再额外请求。
- 封面可见层：图片长时间未完成时收敛到稳定占位（🎧）、图片加载失败立即显示占位、分页失败保留已加载节目并提供重试、首屏失败仍显示整页错误态。

修复实施前，回归用例应失败；修复实施后，所有用例通过。守卫用例（已正确行为）始终通过，用于防止二次回归。

### 生产授权验收（需重启/部署授权）

真实链路的封面代理、缓存与分页时延只能在运行中的生产模式服务上观察，属于 AGENTS.md 中需要明确授权的操作。取得授权后，先按本指南顶部准备服务，再运行页面/API 巡检：

```bash
node scripts/performance-audit.mjs \
  --base-url http://localhost:3000 \
  --api-url http://localhost:8080 \
  --runs 3 --strict
```

重点关注播客列表首屏与分页接口的耗时、`/images/proxy` 封面链路的成功率与体积。未取得重启/部署授权前，不要凭巡检结论修改排序、搜索、标签、数据库结构或真实数据。

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
