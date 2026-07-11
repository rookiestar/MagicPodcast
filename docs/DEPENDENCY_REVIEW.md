# 依赖健康检查

最后更新：2026-06-01

本文记录当前依赖审计结果和后续处理边界。依赖升级可能改变运行时行为，因此只自动处理同一主版本内、可通过现有测试验证的修复；需要跨主版本升级的内容放入人审队列。

## 前端依赖

检查命令：

```bash
cd frontend
npm audit --omit=dev
npm audit
```

本轮已自动更新：

| 依赖 | 更新后版本 | 说明 |
| --- | --- | --- |
| `next` | 14.2.35 | 同一主版本内升级，清除原 critical 项 |
| `eslint-config-next` | 14.2.35 | 与 Next 版本保持一致 |
| `axios` | 1.16.1 | 清除运行时 high 项 |
| `dompurify` | 3.4.7 | 清除运行时 moderate 项 |
| `postcss` | 8.5.15 | 根依赖已更新；Next 内置依赖仍受 Next 大版本限制 |

本轮已自动移除未使用依赖：

| 依赖 | 原因 |
| --- | --- |
| `@testing-library/user-event` | 测试代码中没有使用 |
| `jsdom` | 当前 Vitest 配置使用 `happy-dom`，不使用 `jsdom` |
| `@types/dompurify` | `dompurify` 已自带类型定义 |

当前审计结果：

| 范围 | 结果 | 剩余原因 |
| --- | --- | --- |
| 生产依赖，2026-06-01 复跑 | 2 项：1 high、1 moderate | 都要求升级到 `next@16.2.6` |
| 全量依赖，2026-05-31 复跑 | 5 项：4 high、1 moderate | `next` / `eslint-config-next` 需要 16.x；当前不自动跨主版本 |

剩余风险主要来自：

- `next`: GitHub advisories GHSA-9g9p-9gw9-jx7f、GHSA-h25m-26qc-wcjf、GHSA-ggv3-7p47-pfv8、GHSA-3x4c-7xq6-9pq8、GHSA-q4gf-8mx6-v5v3、GHSA-8h8q-6873-q5fj、GHSA-3g8h-86w9-wvmq、GHSA-ffhc-5mcf-pf4q、GHSA-vfv6-92ff-j949、GHSA-gx5p-jg67-6x7h、GHSA-h64f-5h5j-jqjh、GHSA-c4j6-fc7j-m34r、GHSA-wfc6-r584-vfw7、GHSA-36qx-fr4f-26g5。
- `eslint-config-next` / `@next/eslint-plugin-next`: 依赖的 `glob` 仍有 high 项 GHSA-5j98-mcp5-4vw2，修复路径要求 `eslint-config-next@16.2.6`。
- `postcss`: Next 内置的 PostCSS 版本仍受 Next 大版本限制。

后续人审建议：

1. 单独安排 Next 14 -> 16 升级验证。
2. 升级后完整执行类型检查、Lint、测试、生产构建、页面/API 巡检和浏览器点击验证。
3. 如果暂不升级 Next 16，需要确认当前部署是否暴露相关能力，例如 image optimizer、rewrites、middleware/proxy、WebSocket upgrades、Server Components 等。

## 后端 Go 依赖

已完成：

- `go test ./...` 通过。
- `go vet ./...` 通过。
- 关键依赖版本已从 `go.mod` 和本地模块缓存读取。

未完成：

- `govulncheck` 扫描在 2026-06-01 重试仍失败：访问 `proxy.golang.org` 超时；改用 `GOPROXY=direct` 后访问 `golang.org` 仍超时，未得到有效漏洞结论。
- `staticcheck` 扫描在 2026-06-01 重试仍失败：访问 `proxy.golang.org` 超时；改用 `GOPROXY=direct` 后访问 GitHub 仍失败。

建议后续在网络稳定时重跑：

```bash
cd backend
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
```
