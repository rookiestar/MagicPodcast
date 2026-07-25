# Feed 403 诊断 Runbook

最后更新：2026-07-21
关联：[Feed 抓取可靠性 Spec §7.2](../research/FEED_FETCHER_RELIABILITY_SPEC_2026-07-21.md)、issue #29

本 Runbook 把 `feed.xyzfm.space` 等域名的 403 / `access_denied` 现象组织为**待验证假设**，而不是根因结论。它依赖 #28 的结构化失败日志（`failure_phase`、`configured_egress_label`）和 #29 的受保护 admin 诊断入口。它不预设 403 必然由 IP/ASN、CDN、频率或请求指纹中某一类触发。

## 诊断数据来源

- **结构化失败日志**（`feed fetch failed`，Warn 级）：字段见 `backend/internal/feed/fetch_trace.go` 的白名单。关键字段：`target_domain`、`http_status`、`error_category`、`failure_phase`、`configured_egress_label`、`circuit_state`、`response_time_ms`、`retry_after`。
- **受保护 admin JSON**：`GET /api/v1/admin/feed-diagnostics`（仅 loopback 绑定 + Cloudflare Access，无 `/metrics`）。返回 `feed_fetch_total{domain,status,category,source}`、`feed_fetch_duration_seconds`（固定分桶）、`circuit_state{domain}`、`circuit_transitions_total{domain,from,to}`、`conditional_get_total{result}`、`last_good_hits_total`、`retry_total{domain}`、`snapshot_store`（容量/淘汰/写失败）、`configured_egress_label`。

> 字段白名单：诊断数据**不含**完整 Feed URL、正文、Cookie、Token、代理凭据或任意响应头。`target_domain` 是唯一的 per-Feed 标签（低基数）。`configured_egress_label` 仅是配置标签。

## 决策树（依据结构化字段）

```
观测到 ErrorCategory=access_denied / HTTPStatus=403
│
├─ failure_phase = response_header/body_read 且响应耗时 ~100ms 级（快速 HTTP 拒绝）
│   └─ 假设：上游或中间 HTTP 层快速返回 403。
│        能确认：拒绝发生在 HTTP 层（头部已收到），不是 TCP/TLS/连接问题。
│        不能确认：无法据此区分 IP/ASN、CDN、节奏或请求策略。
│        动作：按 target_domain + 时间窗 + configured_egress_label 聚合 admin 计数，
│              并与网络侧实际出口证据配对后再下结论。
│
├─ 仅部分 Feed 失败，且与请求频率/并发相关
│   └─ 假设：频率/并发触发。
│        动作：检查 retry_total{domain}、circuit_transitions（open/probe）是否密集；
│              核对 MaxConcurrency / MinRefreshInterval / 错峰是否生效。
│
├─ Conditional GET 未启用 / 304 未被识别为成功
│   └─ 这是客户端标准能力缺口（conditional_get_total 的 miss/304/200 分布可见），
│      但不是 403 根因证据。
│      动作：核对 User-Agent、Accept、验证器与快照是否成对生效（ETag/Last-Modified）。
│
├─ 伴随 Retry-After 或间歇性成功
│   └─ 假设：存在服务端等待提示或动态策略。
│        能确认：服务端发了 Retry-After（日志 retry_after 字段），或同域间歇成功。
│        动作：遵守 Retry-After（#25 已实现带上限的处理）；仍不单独归因 CDN。
│
└─ 持续全失败且无 Retry-After
    └─ 走研究记录决策门槛：
        - 若低频直连可恢复 → 倾向节奏/频率问题。
        - 若直连 + 已验证备用源均败 → 倾向上游统一策略或公共故障。
        - 任何“固定出口可解”的结论必须先满足 #22 双窗口证据 + #24 决策 + 明确生产授权。
```

## ConfiguredEgressLabel 的边界（重要）

`configured_egress_label`（默认 `direct`）只记录“应用被配置为走哪条出口”，**不是实际公网出口证明**。它出现在失败日志和 admin 诊断里，用于把 403 现象与配置标签配对观察。

- 本 Runbook 不凭它断言 IP/ASN 归属。
- 固定出口实验仍属 **No-Go**：必须先满足 [XIAOYUZHOU_FEED_ACCESS_RESILIENCE_2026-07-19](../research/XIAOYUZHOU_FEED_ACCESS_RESILIENCE_2026-07-19.md) 的 #22 双窗口证据、#24 决策门槛，并获得明确生产授权后才能改变网络出口。在此之前，`configured_egress_label` 只用于观察，不用于切换。

## 处置动作清单

1. **先看 failure_phase**：若为 `connect`/`tls`/`dns`，这不是 403 问题，走网络层排查；若为 `response_header`/`body_read`，进入本 Runbook。
2. **聚合 admin 计数**：按 `target_domain` 看 `feed_fetch_total{...,source=primary}` 的 403 占比与时间分布，配对 `circuit_transitions` 和 `retry_total`。
3. **核对节奏**：当前源码仍会让 `feed.xyzfm.space` 首次 `access_denied`/403 触发整域 OPEN，因此后续同域 `circuit_open` 是派生策略结果，不是新的上游 403。#35/#36 已批准删除该硬断路，改为共享单队列、简单自适应软限速和 15 分钟批次内的有界分类重试；在 #36 落地前，必须把现行行为标成待替换实现，不得称为目标设计或永久不变量。其他域名默认也不因单个 Feed 的一次 403 触发域名熔断。
4. **遵守 Retry-After**：日志有 `retry_after` 时按其等待，#25 已对齐并设上限。
5. **恢复链验证**：403 后先尝试已验证的 PodcastIndex 替代源；按 #35，last-good 仅用于匹配验证器的 304 恢复和诊断，不得把普通失败后的快照命中计作本批成功、写入新报告或推进抓取时间。分别核对替代源结果、`last_good_hits_total` 与 `snapshot_store`，并区分当前实现和 #35 目标语义。
6. **结论门槛**：任何根因结论（IP/ASN、CDN、固定出口）都需网络侧证据 + #22/#24 门槛，不单独凭本 Runbook 的字段下结论。
