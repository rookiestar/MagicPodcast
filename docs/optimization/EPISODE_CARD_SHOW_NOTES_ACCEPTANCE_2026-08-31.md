# Episode Card Show Notes 验收

日期：2026-08-31
Issue：#224 / #226
分支：`codex/issue-226-episode-card-show-notes`
环境：本地 production build + `fixture/journey`（schema 25）

## 用户体验不变量

首次访问、慢请求和失败时，已有单集身份、三行预览与原节目入口始终可用；移动端不读取完整正文。

## 状态矩阵

| 场景 | 受控输入 | 结果 | 证据 |
| --- | --- | --- | --- |
| 首次访问 | 浏览器禁用缓存后进入 `/podcasts/1001` | summary 约 96ms 可见；列表 1 次、1023 bytes；完整正文 0 次 | 1440px Playwright Fixture |
| 正常请求 | hover / focus Episode 2003 | 约 92ms 显示完整文档；同集回访仍为 1 次请求 | Playwright + 最高层用户流测试 |
| 慢请求 | 完整正文延迟 900ms | 三行预览与卡片结构保留，局部显示“正在读取”；随后显示全文 | Playwright；加载前/中卡片高约 155.7/180.1px |
| 请求失败 | 完整正文返回 503 | 预览、原节目链接和其他卡片保留；显示局部错误与“重试全文” | Playwright + 最高层用户流测试 |
| 缓存回访 | 同一 Episode 再次 hover / focus | 直接复用成功文档，不重复请求 | Store 与最高层用户流测试 |
| 快速切换 | A、B 请求乱序返回 | B 保持当前正文，迟到 A 不覆盖 B | Store、Card 与最高层用户流测试 |
| 390px 移动端 | hover 与 focus 均注入 | summary 可见、8 个原节目入口、完整正文 0 次、无横向溢出 | 390px Playwright Fixture |

## 有界阅读与视觉

- 1440px 长文 Fixture：阅读区可视高 384px、内容高 2565px，可内部滚动；卡片高约 499px。
- 1440px / 390px 均无横向溢出；白色渐隐节点为 0。
- 键盘 focus 可打开同一阅读区；鼠标离开但焦点仍在卡片内时不会收起。

## 缓存与 HTTP 合同

| 项目 | 结论 |
| --- | --- |
| 键 | Episode ID |
| 容量 | 当前页面会话最多 12 份成功文档，LRU 淘汰 |
| 生命周期 | EpisodeListSection 挂载期间；不持久化 |
| 并发 | 同 Episode 单飞合并 |
| 失败 | 不写缓存，可显式重试 |
| 串集防御 | 校验响应 Episode ID，并在组件层校验当前身份与请求序列 |
| HTTP 缓存 | `private, max-age=60, stale-while-revalidate=300` |
| 返回字段 | 仅 `episode_id` 与 `show_notes_document` |

## 自动化验证

- 后端定向：`go test ./internal/handlers ./internal/router ./internal/utils`
- 前端定向：7 个相关文件，49 项通过
- 后端全量：`go test ./...`、`go vet ./...`
- 前端全量：116 个文件、714 项通过
- `npm run type-check`、`npm run lint`、`npm run build`
- `./.agents/skills/code-change-verification/scripts/verify.sh`

浏览器使用现有只读 Fixture 数据与受控 Show Notes 端点桥接；后端新增入口由真实 Gin + SQLite HTTP 合同测试独立覆盖。未执行 CI、commit、push、PR、Issue 关闭、部署、迁移、生产同步或生产验收。
