# 依赖健康检查

最后更新：2026-07-12

本文记录当前生产依赖审计结果、升级边界和仍需人工处理的风险。依赖升级必须保持生产代理、图片路径、内容净化和现有测试入口不变；构建失败时使用发布脚本保留并恢复上一组已验证产物。

## 前端依赖

检查命令（在 Mac mini 生产工作区执行）：

```bash
cd frontend
npm audit --omit=dev
npm audit
npm run type-check
npm run lint
npm run test:run
```

本轮已升级并验证的依赖：

| 依赖 | 当前版本 | 说明 |
| --- | --- | --- |
| `next` | 16.2.10 | 从 14.x 跨主版本升级；已单独授权，并通过兼容修正、构建和回退发布流程验证 |
| `eslint-config-next` | 16.2.10 | 与 Next 保持同版本 |
| `eslint` | 9.39.5 | 满足 Next 16 的 ESLint 配置要求 |
| `axios` | 1.18.1 | 清除生产审计中的 multipart 相关风险 |
| `dompurify` | 3.4.12 | 清除生产审计中的 HTML 净化风险 |
| `postcss` | 8.5.17 | 与 Next 16 构建链保持一致 |
| `form-data` | 4.0.6 | 通过根依赖 override 固定安全版本 |

已移除的未使用开发依赖：

| 依赖 | 原因 |
| --- | --- |
| `@testing-library/user-event` | 测试代码没有使用 |
| `jsdom` | Vitest 使用 `happy-dom`，没有使用 `jsdom` |
| `@types/dompurify` | `dompurify` 已自带类型定义 |

### 当前审计证据（2026-07-12）

| 范围 | 结果 | 结论 |
| --- | --- | --- |
| 生产依赖 `npm audit --omit=dev` | `0 vulnerabilities` | #6 的生产依赖安全门通过 |
| 全量依赖 `npm audit` | 4 项开发依赖风险（2 low、1 high、1 critical） | 不随生产构建部署；保留在开发工具链，后续单独评估升级 |

全量审计剩余项来自 `@babel/core`、`esbuild`、`vite` 和 `vitest`，影响的是开发/测试工具（其中 Vitest 风险描述要求 UI 服务监听时才成立）。本事项只要求生产依赖审计，未通过扩大锁文件变更来处理无关开发工具升级。

Next 16 兼容性修正保持最小范围：

- 将播客页的客户端组件直接置于客户端边界，避免 Server Component 中使用 `dynamic(..., { ssr: false })`。
- 移除会让 Turbopack 生成非法 CSS 伪元素顺序的 `disabled:file:*` 变体，改用禁用态透明度。
- 内容净化仍保留危险标签、事件属性、样式和不安全 URL 的清理；Markdown 语法不再交给 DOMPurify 破坏。

验证结果：62 个前端测试文件、315 个测试通过；TypeScript、ESLint 和 Next 16 生产构建通过。生产发布使用 `scripts/release.sh` 的临时产物、健康校验和自动回退路径，失败不会停止旧版本。

### 后端 Go 依赖

已完成：

- Mac mini 上 `go test ./...` 通过。
- Mac mini 上 `go vet ./...` 通过。
- 生产服务重启后健康接口返回 `database: ok`，没有触发写入型迁移。

未完成：

- `govulncheck` 和 `staticcheck` 尚未得到有效结论；此前因访问 Go 模块源超时而失败，需在网络稳定时单独重跑。

建议命令：

```bash
cd backend
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
```

## 回退边界

生产发布前使用：

```bash
./scripts/release.sh --prepare
./scripts/restart.sh --prod
```

构建、启动或健康校验失败时，发布脚本会保留当前 release 并自动恢复；也可使用：

```bash
./scripts/release.sh --rollback
```

本轮未执行 Git add、commit、push 或 PR；依赖锁文件和源码改动仍需在后续单独审阅后提交。
