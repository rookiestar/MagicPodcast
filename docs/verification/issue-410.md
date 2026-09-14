# Issue #410 验证记录：collection.xiaoyuzhoufm.com 专题导入

日期：2026-09-14。分支 `codex/issue-410-collection-campaign`（基于 origin/main f5897e9）。

## 来源核实结论（一手验证）

- `https://collection.xiaoyuzhoufm.com/wavesfilm2026` 返回 HTTP 200，无跳转；页面是 Vite SPA 客户端渲染壳，HTML 内无清单数据。
- 页面数据来自公开接口 `GET https://api.xiaoyuzhoufm.com/v1/campaign/get?slug={slug}`（无需鉴权、无特殊请求头，普通 curl 可访问）。
- 响应结构：`data.config.share.title/desc` 为专题标题与描述；`data.components[]` 中 `kind=EPISODE_LIST` 的组件携带条目；条目 `kind=EPISODE`，已发布条目含 `episode.eid/pid/title/...`（结构与既有清单 target 条目兼容），未发布条目没有 `episode` 对象（预告位）。
- 旧格式 `www.xiaoyuzhoufm.com/collection/episode/…` 未被替换：专题 SPA 自身仍链接该格式，两种入口并存。用户推测的"URL 格式变化"实为新增专题入口。

## 实现要点

- `backend/internal/collection/source.go`：URL 白名单扩展两种来源形态（`sourceKind`）——既有 `www…/collection/episode/{24hex}` 与新增 `collection.xiaoyuzhoufm.com/{slug}`（slug 字符集 `[0-9a-zA-Z][0-9a-zA-Z_-]{0,63}`）；生产抓取器按形态分发：清单页直抓规范化页面，专题抓取由 slug 构造的 `api.xiaoyuzhoufm.com/v1/campaign/get` 地址；逐跳重定向校验按形态收紧（专题每一跳必须仍是同一 slug 的接口地址）；保留私网/CGNAT 封禁、禁代理、3MB 上限、20s 超时。
- `backend/internal/collection/campaign.go`（新增）：`ParseCampaignJSON` 确定性解析；响应 slug 与请求 slug 不一致即拒绝；未发布预告位跳过；条目映射与清单解析共用 `itemDraftFromPayload`，推荐语取条目 `quote`。
- 去重身份：`(source_platform, external_id)`，专题 `external_id` = slug；保存的 `source_url` 是规范化专题页地址，刷新沿用该地址并按形态重新分发。
- 前端 `ImportCollectionModal` 提示明确两种链接格式；后端 `UNSUPPORTED_SOURCE` 文案同步更新。

## 验证证据

自动化（本地，全部通过）：

- `go test ./...`（backend 全量，无失败）。
- `go test ./internal/collection/ ./internal/handlers/ -run Collection/Campaign`：新增回归覆盖——URL 接受/拒绝矩阵（含 `api.` 主机伪装页面、多级路径、路径穿越、http 降级、端口）、解析器（真实响应裁剪 fixture `testdata/campaign_sample.json`：已发布条目解析、预告位跳过、slug 不匹配拒绝、非单集专题拒绝、全预告位空清单、重复 eid、私密音频不留快照、非法时间不编造日期）、service 层（专题预览抓取规范化页面地址、导入保存来源身份、重复预览去重、刷新差异）、逐跳校验（异 slug/附加参数/异主机/http/端口拒绝）。
- 前端 `vitest run src/components/collections src/app/collections`：4 文件 23 例通过（含新增"专题链接可预览、原样透传后端、作者缺失不编造"用例）；`eslint` 与 `tsc --noEmit` 通过。

真实来源与浏览器导入流程（本地 127.0.0.1:18096 后端 + :3100 前端，一次性临时库，未触碰生产与用户实例）：

- 真实专题链接预览：读取 6 条已发布单集（当日现场专题仍在更新；初抓 3 条发布+3 条预告位，数小时后再抓 6 条发布，预告位转正被正确跳过/收录），标题、逐集推荐语、来源身份与页面地址符合预期。
- 真实旧格式清单回归：`www…/collection/episode/6a20323b78a52c96d821a769`（穿透半导体迷雾）解析 8 条不变。
- 导入与去重：确认导入生成清单；同 slug 带不同查询参数再次预览返回 `duplicate=true` 并指向已有清单 ID。
- 单集按需收录：adopt 后 `podcast_subscribed=false`、`collection_only=true`、`inbox_written=true`，Inbox 队列出现该单集；未批量入库、未关注节目。
- 刷新：对已导入专题 refresh-preview 读取 6 条、无伪差异。
- 浏览器全流程（playwright，真实页面）：/collections → 导入清单 → 粘贴真实专题链接 → 预览（截图 `preview.png`）→ 导入清单 → 详情页展示全部 6 集（`imported-detail.png`）；Inbox 出现已收录单集（`inbox-adopted.png`）。

## 边界与限制

- 抓取仅限两个已核实主机与固定接口路径，未扩大为任意 URL 抓取；重定向逐跳按来源形态校验。
- 专题接口为公开只读端点，若源站后续调整结构，解析按既有失败分类明确报错（不冒充成功）。
- 生产部署不在本次范围；生产可用性未验证。
