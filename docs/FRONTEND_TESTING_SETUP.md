# 前端测试配置

最后更新：2026-05-31

前端测试使用 Vitest、Testing Library 和 happy-dom。当前测试入口和依赖以本文件为准，旧的 `jsdom`、`@testing-library/user-event` 和 `src/test-utils.tsx` 已确认未使用并删除。

## 运行命令

```bash
cd frontend
npm run type-check
npm run lint
npm run test:run
npm run build
```

## 当前配置

| 文件 | 作用 |
| --- | --- |
| `vitest.config.ts` | Vitest 配置，使用 `happy-dom` 作为测试环境 |
| `vitest.setup.ts` | 全局测试设置，包含 jest-dom、Next 路由和图片组件 mock |
| `tsconfig.spec.json` | 测试用 TypeScript 配置 |
| `package.json` | 测试、类型检查、Lint 和构建脚本 |

## 当前结果

最近一次验证结果：

| 检查 | 结果 |
| --- | --- |
| 类型检查 | 通过 |
| Lint | 通过 |
| 测试 | 55 个测试文件、294 个用例通过 |
| 生产构建 | 通过 |

## 维护原则

1. 新增组件或状态逻辑时，优先补充靠近代码的 `__tests__`。
2. 对纯状态计算、路径生成、展示规则，优先写轻量单元测试。
3. 对页面级交互，优先覆盖关键流程，不复制实现细节。
4. 不再新增全局测试工具文件，除非多个测试文件确实重复同一套 Provider 或数据构造。
