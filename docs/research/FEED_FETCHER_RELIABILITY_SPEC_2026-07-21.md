# Feed 抓取可靠性升级 Spec

日期：2026-07-21
状态：设计 Spec（已基于当前代码实现只读分析；未修改代码、配置或生产状态）
对齐基线：[XIAOYUZHOU_FEED_ACCESS_RESILIENCE_2026-07-19.md](XIAOYUZHOU_FEED_ACCESS_RESILIENCE_2026-07-19.md)、[XIAOYUZHOU_ALTERNATIVE_FEED_CANDIDATES_2026-07-20.md](XIAOYUZHOU_ALTERNATIVE_FEED_CANDIDATES_2026-07-20.md)、固定出口 NO-GO 决策（#22 / #24）
范围：把 MagicPodcast 建设成长期稳定、合规、可观测的 RSS Fetcher；**不**设计任何绕过上游风控的方案。

---

## 1. Problem Statement（当前架构问题分析）

### 1.1 背景

小宇宙主 Feed（`feed.xyzfm.space`）在生产中持续出现 HTTP `403 access_denied`。生产证据（来自韧性研究记录）：2026-05-28 至 06-02 该域名约 350 次成功；06-03 至 07-18 约 2562 次失败、涉及 146 个 Feed，失败平均耗时约 115ms——更像“快速拒绝”而非连接超时。研究结论明确：**403 不足以证明按出口 IP 封锁**，可能源自频率、并发、CDN 边缘策略或未公开规则。

当前系统已经具备：请求降频（域名级单并发 + 最小刷新间隔 + 抖动）、请求去重（in-flight 合并 + 成功结果短期共享缓存）、域名级 circuit breaker、失败观测（`JobExecution` 持久化）、内存级 last-good cache、PodcastIndex 已验证替代源、跨平台 episode 去重。这些机制避免了失败扩散，但**没有解决三个核心问题**。

### 1.2 逐层问题分析

**HTTP 客户端层**

- User-Agent 是裸字符串 `MagicPodcast/1.0`，能标识产品但没有提供项目说明或联系入口；同时存在一处死代码双写点（给解析器字段赋值，但实际走 `Parse(io.Reader)`，该值永不生效）。RFC 9309 并不强制把联系方式写进 UA，本 Spec 将其作为可审计性改进，而不是把当前 UA 定性为违规。
- 请求只显式设置 `User-Agent`，缺少 RSS/XML `Accept` 与 `If-None-Match`、`If-Modified-Since`。`Accept-Language` 可能改变内容和缓存键，不作为默认 Header，仅允许有明确业务需要的域名单独配置。
- 条件 GET **完全未实装**：响应侧已经把 `ETag/Last-Modified/Cache-Control/Expires/Age` 采集进 `AccessOutcome` 并持久化到 `JobExecution`，`CacheStatusNotModified` 常量也已定义，但请求侧从不回带验证器；304 会被 `status < 200 || status >= 300` 分支当成错误。响应观测字段已具备，但安全处理 304 还依赖“验证器与可恢复正文原子保存”，不是简单加两个请求头。
- 超时统一为 30s，未拆分 connect / TLS handshake / response-header / body read，慢服务器只要持续吐字节就会用满整桶；Transport 也缺少连接阶段诊断。Feed 是用户订阅数据，直接套用图片代理的私网阻断或跨域重定向白名单会破坏局域网 Feed 与正常迁移跳转，须作为独立兼容性决策，不能混入低风险 Transport 改造。
- 重试分两层且不一致：外层业务重试（3 次，2s→4s→8s 纯指数）**无抖动**、**忽略 `Retry-After`**，并发重试多 Feed 会形成同步峰值；可重试/不可重试分类已存在但只在 OPML/元数据同步路径生效。

**断路器层**

- 当前 `Probe` 已承担简化 HALF_OPEN 语义，但只允许一个探测，成功即清零回到 Closed，没有可配置恢复滞后。是否升级为显式 `HalfOpen` 是可观测性和策略表达改进，不是因为当前状态机无效。
- 对 `feed.xyzfm.space`，一次 403 即 OPEN 10 分钟是既有 ADR 明确要求，生产样本也证明它能阻止失败扩散，必须保留。另一方面，timeout、网络错误、500/502/504 当前不进熔断；但这些错误可能只影响同域某一个 Feed，不能不加区分地升级为域名故障。
- 覆盖面刻意保守：`DefaultCoordinatorConfig` 只对 `feed.xyzfm.space` 启用域名策略，其他域名仍有全局同 URL in-flight 去重，但没有默认域名级并发、抖动和熔断。可以增加安全的通用负载整形默认值；域名级熔断仍须按域配置或由多个不同 Feed 的共同失败证据触发。
- 不可配置、不可热更新：阈值与冷却全部硬编码为 Go 常量，YAML/ENV 都改不动；`Scheduler.Reload` 只重载 cron 表达式，不重建 Coordinator。
- 无 per-feed 维度信号：同一域名下一个坏 Feed URL 反复失败会拖累整域（当前按域聚合既是优点也是风险点）。

**内容韧性层**

- last-good 是**纯内存** LRU（`MemorySnapshotStore`），进程重启即丢；冷启动后首次失败无任何 fallback。
- `SnapshotStore` 接口（`Save/Load/Stats`）已留好持久化 seam，但缺一个磁盘/SQLite 实现。
- Podcast 表**没有** `ETag / Last-Modified / LastSuccessAt / LastFailureAt / ConsecutiveFailures` 字段；`FetchErrorCount` 字段与仓储方法齐全但**生产代码从不递增**（死字段），“连续失败次数”在 Podcast 维度不可得，只能事后 SQL 聚合 `JobExecution`。

**观测层**

- **无实时指标暴露**（无 Prometheus / expvar / `/metrics` / admin 计数器），所有“指标”只能事后聚合 `JobExecution`，无法实时观测。
- Feed 抓取失败日志全是字符串拼接（`logger.Infof`），**无结构化字段**，缺 `feed_id/podcast_id/retry_count/attempt/failure_phase/egress_id` 等 key-value，排查需正则解析。
- 告警只在 workflow 级“连续 3 次失败打一条日志”，无 feed 级告警；邮件通知只在成功时发。
- 403 诊断信息不足：`EgressID` 在直连时恒为常量 `"direct"`，它只是应用标签，不是实际公网出口证据；同时缺少 DNS/连接阶段等分类字段。

### 1.3 三个核心问题

1. 如何**降低** Feed 请求触发上游风控的概率（客户端质量 + 降流 + 条件请求）。
2. 如何让 Feed 抓取行为更**符合标准 RSS Client**（诚实 UA + 标准 Header + Conditional GET + robots 尊重）。
3. 如何在 403 后**安全恢复并持续诊断原因**（保留立即断路 + 可观测恢复探测 + 持久化 fallback + 结构化观测）。

---

## 2. Solution（Feed Fetcher 目标架构）

### 2.1 目标

把 MagicPodcast 表现为一个**合规、高质量、低负载、可观测**的 RSS Reader，而不是异常 crawler。在不改变用户可见行为、不改 fallback 链顺序、不引入任何绕过风控手段的前提下，分四层（P0–P3）建设长期稳定的 RSS Fetcher。

### 2.2 目标架构

```
                       ┌─────────────────────────────────────────────┐
调度层 (cron/workflow) │  错峰 + per-job jitter + retry budget        │
                       └───────────────┬─────────────────────────────┘
                                       ▼
┌──────────────────────────────────────────────────────────────────────┐
│ Coordinator（进程单例，既有）                                          │
│  in-flight 去重 │ 域名级并发队列 │ Circuit Breaker(新状态机) │ Snapshot│
└───────────────┬──────────────────────────────────────┬─────────────┘
                ▼                                        ▼
┌──────────────────────────────────┐   ┌──────────────────────────────┐
│ RSS Client Quality Layer (P0 新) │   │ Snapshot Store (P2 升级)      │
│  诚实 UA + 标准 Header            │   │  L1 内存 LRU → L2 持久化       │
│  Conditional GET (304 复用快照)   │   │  + 验证器原子持久化             │
│  分层超时 + Transport 加固        │   └──────────────────────────────┘
│  jittered backoff + Retry-After  │
│  robots 缓存 (24h)               │
└───────────────┬──────────────────┘
                ▼
        AccessOutcome（唯一观测 seam：贯穿 P0/P1/P2/P3）
        HTTPStatus / ErrorCategory / CacheStatus / Freshness / CircuitState
        EgressLabel / ETag / LastModified / RetryAfter / ResponseTimeMs
        SourceType / IdentityVerification / SnapshotRetrievedAt / ValidatedAt(新)
        FailurePhase(新)
                │
        ┌───────┴────────┬──────────────────────┐
        ▼                ▼                      ▼
  JobExecution(既有)  结构化日志(P3)       指标JSON(admin)(P3)
```

### 2.3 四层目标

- **P0 RSS Client Quality Layer**：标准 Fetcher 行为，让每次请求都像高质量 RSS Reader。
- **P1 Feed Failure Recovery**：显式 `CLOSED → OPEN → HALF_OPEN → CLOSED` 状态机，保留小宇宙首次 403 立即断路，按错误类型和证据范围分级恢复。
- **P2 Persistent Content Resilience**：last-good 持久化，重启不丢，长期失败 Feed 可降级兜底。
- **P3 403 Diagnostic System**：结构化日志 + 连接阶段 + 轻量内存计数器（经独立、受保护的 admin JSON 入口暴露，**不修改公开 health 契约、不引入 Prometheus、不新增 `/metrics`**）+ 配置标签，为后续对照实验提供应用侧证据，但不单独判定 IP/ASN、频率、指纹或 CDN 根因。

---

## 3. User Stories

**听众 / 订阅者**

1. 作为听众，我希望订阅的播客在 Feed 暂时不可达时仍能看到上一集列表，这样我不会因为上游波动而“丢节目”。
2. 作为听众，我希望服务重启后已缓存的 Feed 内容仍可访问，这样重启不会让所有节目短暂消失。
3. 作为听众，我希望新的剧集在 Feed 恢复后被正确补齐（既不漏也不重复），这样内容始终完整一致。
4. 作为听众，我希望系统不会因为单个播客 Feed 坏掉而拖慢或影响其他播客的更新，这样整体体验稳定。

**播客管理者 / 运维**

5. 作为运维，我希望 403 后系统能自动进入恢复探测并在上游恢复时无人工干预地回到正常，这样我不必每次手动处理。
6. 作为运维，我希望系统能降低触发上游风控的概率（更标准的请求、更低频、支持 304），这样 403 本身越来越少发生。
7. 作为运维，我希望 last-good 缓存持久化到磁盘，这样重启和长期失败都不会丢失兜底内容。
8. 作为运维，我希望过期的 last-good 被明确标记为 stale 而不是伪装成 fresh，这样我不会误判数据时效。
9. 作为运维，我希望 fallback 链顺序（主 → 已验证替代 → last-good）保持不变，这样既有可审计决策不被破坏。
10. 作为运维，我希望断路器参数和域名策略可在配置文件调整；若热更新不能安全保持并发与状态一致性，则首版允许通过服务重启生效，不把无停机热更新作为 P1 前置。
11. 作为运维，我希望所有域名都有保守的请求去重与负载整形默认值；只有已配置域名或多个不同 Feed 共同证明域名故障时，才启用域名级熔断。
12. 作为运维，我希望系统尊重 Feed 自带的 `<ttl>` / `skipHours` / `skipDays` 与 HTTP 缓存头，这样我不会比上游建议更频繁地请求。

**上游 Feed 提供方（合规视角）**

13. 作为上游提供方，我希望 MagicPodcast 用诚实、稳定、带联系方式的 User-Agent 访问我，这样我能识别并联系到它。
14. 作为上游提供方，我希望 MagicPodcast 尊重我的 robots.txt 与 `Retry-After`，这样它表现得像一个负责任的客户端。
15. 作为上游提供方，我希望 MagicPodcast 用条件请求避免重复下载未变化内容，这样我的带宽不被浪费。
16. 作为上游提供方，我希望 MagicPodcast 不会用代理池、轮换 IP 或浏览器伪装来访问我，这样它的行为可解释、可审计。

**开发 / 数据观测者**

17. 作为 SRE，我希望 Feed 抓取失败日志是结构化的（含 feed_id、状态码、错误类别、重试次数、失败阶段），这样我能用日志系统直接聚合查询。
18. 作为 SRE，我希望有一个受保护的 admin JSON 指标入口，暴露抓取计数、延迟、断路状态、last-good 命中率、304 命中率且不改变公开 health 响应，这样我不用事后跑 SQL。
19. 作为 SRE，我希望日志记录“失败发生在哪个连接阶段”（DNS / connect / TLS / 读头 / 读体），这样我能区分“115ms 快速拒绝”与“超时”。
20. 作为 SRE，我希望记录明确命名为“配置出口标签”的字段，而不是把配置值误称为真实公网出口；未来对照实验仍以网络侧出口证据为准。
21. 作为 SRE，我希望有一条 403 诊断 runbook，把结构化字段组织成待验证假设与对照证据，而不是直接宣称 IP/ASN、频率、指纹或 CDN 根因。
22. 作为开发者，我希望新增抓取行为继续通过 `FetchResult.AccessOutcome` 观测；条件请求所需的验证器必须通过一个明确、可注入的请求状态 seam 进入 Fetcher，不把数据库依赖塞进 HTTP 层。
23. 作为开发者，我希望条件 GET、状态机、快照兜底使用 httptest 假上游验证，并用可控 Dialer、Listener、TLS server 和时钟分别覆盖 DNS/connect/TLS/读头/读体阶段。
24. 作为开发者，我希望持久化 last-good 走版本化迁移流程（`cmd/migrate`），这样生产库结构变更可审计、可回滚。
25. 作为开发者，我希望 `FetchErrorCount` 这类既有死字段要么被正确接线、要么被移除，这样数据模型不保留误导性字段。
26. 作为开发者，我希望诊断/日志字段遵守既有白名单（不记录正文、Cookie、凭据、任意响应头），这样观测能力不带来泄露风险。
27. 作为运维，我希望断路器在服务重启后从保守状态（CLOSED）起步、并由条件请求与降频保护首波，这样重启不会瞬间冲击上游。
28. 作为运维，我希望 HALF_OPEN 在存在同一 Feed 的可恢复快照时使用条件请求；没有匹配快照时执行一次普通 GET，绝不把无正文的 304 当成可用内容。

---

## 4. Implementation Decisions

> 术语沿用项目既有领域词汇：RSS 源元数据 = **Podcast**（无独立 Feed 表）；单集 = **Episode**；每次抓取观测 = **JobExecution**；last-good 内容 = **FeedSnapshot**；抓取协调 = **Coordinator**；域名策略 = **DomainPolicy**；抓取结果观测载体 = **AccessOutcome**；状态枚举 `CircuitState` / `AccessSource` / `ErrorCategory` / `CacheStatus` / `Freshness` / `IdentityVerification`。既有 `EgressID` 字段为兼容可保留，但新代码和文档统一按 `ConfiguredEgressLabel` 解释，禁止称为真实出口证明。

### 4.1 核心模块设计（改/新增什么，接口契约）

| 模块 | 现状 | 本 Spec 决策 | 关键接口契约（外部行为） |
| --- | --- | --- | --- |
| **RSS Client Quality Layer**（新） | Header/UA/超时/重试散落在 Fetcher 与外层 sync 重试 | 收口为 Fetcher 内单一权威 HTTP 构造点；外层重试改为透传有限策略 | 给定 URL+ctx+`RequestValidators` → 返回带 `CacheStatus`/`ETag`/`LastModified` 的 `AccessOutcome`；304 只在可恢复快照存在时成功 |
| **Coordinator / Circuit Breaker** | `Open/Probe/Closed`、xyzfm 首次拒绝即 OPEN、硬编码 | 将 Probe 显式化为 `HalfOpen`；保留 xyzfm 首次 403 立即 OPEN；其他域名仅在显式策略或多 Feed 故障证据下启用域名熔断 | OPEN 返回 `circuit_open` outcome 并触发 fallback；HALF_OPEN 限量 probe；策略变更首版可重启生效 |
| **SnapshotStore** | 仅内存实现，`Load` 无错误返回 | 新增 SQLite L2；扩展接口返回存储错误；设置条数、单条和总量上限及清理策略 | L1 miss 回源 L2；live 成功始终刷新 L1，L2 失败明确标记 durability degraded；损坏/锁冲突与普通 miss 可区分 |
| **Feed 条件状态** | 验证器只记录在 JobExecution，不能用于下一次请求 | 验证器与对应 snapshot 在同一 `feed_snapshots` 行原子更新；不在 Podcast 表重复保存第二份权威值 | 只有快照指纹和验证器成对存在时才发送条件请求；304 复用该快照并记录验证时刻 |
| **观测（日志/metrics）** | 字符串拼接日志、无 metrics | 结构化日志（logrus WithFields）+ 轻量内存计数器 + 独立受保护 admin JSON + 连接阶段字段 | 失败日志带诊断字段；计数器按有限维度聚合；公开 health 契约不变 |

**改动边界**：HTTP 客户端的唯一权威实现点保持单一（Fetcher）；统一改造主战场是 Fetcher（Client/Transport/Header）与 Coordinator（请求状态、域名策略、状态机、retry 预算）。允许新增一个窄的生产接口 `RequestValidators` / `FeedStateStore`，解决条件状态注入和持久化错误传播；不引入 Fetcher 对 GORM/SQLite 的直接依赖。外层重试散落在 sync 包多处，需一并收敛为调用 Fetcher 提供的有限策略。维护脚本走独立 `gofeed.ParseURL` 路径，不在本 Spec 范围。

### 4.2 P0 — RSS Client Quality Layer

1. **诚实 User-Agent**：采用 `MagicPodcast/<version> (+<project-url>; mailto:<contact>)` 形式，稳定、带联系方式，符合 RFC 9309 §2.2.1。**不**伪装浏览器。移除死代码双写点，UA 单一来源。可通过配置覆盖。
2. **标准请求头**：每次请求带 `Accept: application/rss+xml, application/atom+xml, application/xml;q=0.9, */*;q=0.8`。不显式设置 `Accept-Encoding`：由 Go Transport 自动添加 gzip 并透明解压；显式添加会关闭该自动解压行为。`Accept-Language` 默认不发送，仅允许按域配置。
3. **Conditional GET（核心高 ROI）**：
   - Coordinator 从 `FeedStateStore` 读取同一规范化 Feed URL 的 snapshot 与验证器。只有 snapshot 通过指纹校验且正文可恢复时，才把验证器作为 `RequestValidators` 传给 Fetcher，写入 `If-None-Match` / `If-Modified-Since`。
   - 304 视为“上次内容仍有效”：从同一状态行恢复 Feed，`CacheStatus = not_modified`，`HTTPStatus = 304`，记录新的 `ValidatedAt`，保留原 `RetrievedAt`；不计失败、不触发熔断、不消耗重试预算。
   - 若收到 304 但快照缺失、损坏或无法解析，立即清除本次验证器并在同一次总预算内执行**一次**无条件 GET；绝不返回 nil Feed 或“空增量”伪成功。
   - 200 成功后在一个 SQLite 事务中原子更新 snapshot 正文、指纹、ETag、Last-Modified、RetrievedAt 与 ValidatedAt；无验证器的 200 会清理旧验证器，防止错误复用。
4. **分层超时 + Transport 加固**：引入 `net.Dialer{Timeout}`（connect）、`TLSHandshakeTimeout`、`ResponseHeaderTimeout`，保留整体请求超时和正文大小上限；GET 无请求体，不把 `ExpectContinueTimeout` 作为关键验收项。重定向限制为 HTTP(S)、最多 5 跳并逐跳记录脱敏目标；默认允许合法跨域迁移。私网/回环阻断属于独立安全兼容项，需先审计现有 Feed 并提供显式局域网允许策略，本阶段不默认启用。
5. **jittered backoff + Retry-After**：退避改为“指数 + full jitter”；429/503 优先遵守 `Retry-After`（秒数或 HTTP-date，上限沿用既有 24h）；按域名设 retry budget（有限总重试、有限并发重试），避免重试风暴。可重试集合（timeout / network / 429 / 5xx）与不可重试集合（403 / 401 / 404 / 402 / parse）沿用既有分类。
6. **robots 缓存**：按域名缓存 robots.txt（≤24h，RFC 9309 §2.4），抓取前校验路径准入；必须把 robots 请求计入同一域名预算，缓存失败不得形成逐 Feed 重试。robots 是抓取政策提示而不是访问授权；若通用规则明确禁止目标路径则停止抓取并记录政策拒绝。
7. **尊重 Feed 刷新提示**：以 HTTP 缓存头与 RSS `<ttl>` / `skipHours` / `skipDays` 为刷新下限；无提示时用保守的本地最小刷新间隔 + 错峰（既有 `MinRefreshInterval` + `MaxJitter` 已是该语义，保留）。

### 4.3 P1 — Feed Failure Recovery（状态机设计）

升级为经典三态 + 错误分级 + 恢复滞后。状态机以决策编码呈现（来自原型，仅保留决策关键部分）：

```
States: CLOSED, OPEN, HALF_OPEN        // 取消既有 Probe 布尔，HALF_OPEN 成为一等状态

Transitions:
  CLOSED    -- xyzfm access_denied(403) --> OPEN(cooldown_403)       // 首次立即断路
  CLOSED    -- domain_failure(cat) && evidence >= DomainThreshold --> OPEN(cooldown(cat, attempt))
  OPEN      -- now >= openUntil --> HALF_OPEN   (admit up to HalfOpenMaxRequests concurrent probes)
  HALF_OPEN -- probe success && successes >= SuccessesToClose --> CLOSED (reset counters)
  HALF_OPEN -- probe failure --> OPEN(cooldown(cat, attempt+1))  (reset success count)

Cooldown(category, attempt):
  access_denied(403) : xyzfm 首次即 base_403；probe 再失败后分级递增、有上限
  rate_limited(429)  : min(Retry-After, maxRetryAfter) if present else expBackoff(attempt)
  service_unavailable(5xx) : expBackoff(attempt, fullJitter)   // 覆盖 500/502/503/504，不只 503
  timeout / network_error : expBackoff(attempt, fullJitter)    // 新增：上游不可达也短路
  parse / 4xx(除 429) : 不进熔断（per-feed 内容信号，非域名健康信号）
```

**设计要点**：

- **403 规则**：`feed.xyzfm.space` 保留首次 403 立即 OPEN，符合既有 ADR 和生产验证；其他域名默认不因单个 Feed 的一次 403 触发域名熔断，可按域显式配置。冷却在 probe 再失败后分级递增且有上限，永远保留低频周期性 probe。
- **错误类别证据门槛**：timeout / 网络错误 / 全部 5xx 可纳入域名健康判断，但默认要求一个短窗口内至少 `N` 个**不同 Feed URL**出现同类失败，或由域名策略显式声明共享基础设施；单个 Feed 的错误只影响自身，不拖累同域其他节目。
- **HALF_OPEN 渐进恢复**：`HalfOpenMaxRequests` 限量放行 probe，`SuccessesToClose`（> 1）提供滞后，防止抖动链路反复 OPEN/CLOSED。
- **probe 用条件请求**：HALF_OPEN 探测优先走 Conditional GET（304 即视为成功），探测本身低成本。
- **粒度**：保持 **域名级**（`TargetDomain`）作为共享基础设施信号（403/429/5xx/timeout）的熔断维度；per-feed 内容错误（404/parse）走 Feed 级跳过逻辑，不污染域名熔断。
- **默认策略**：所有域名继续获得同 URL in-flight 去重；可以增加保守并发上限和小幅错峰，但域名级熔断默认仅对 `feed.xyzfm.space` 和显式配置域名启用。后续若要扩大，必须先用观测证明共享故障粒度。
- **重启行为**：进程级 circuit 状态是易失的（反映实时上游健康），重启后全部回到 CLOSED（保守放行），由条件请求、降频和 retry budget 保护首波；首版不从 Podcast 失败计数预热，避免把历史单 Feed 失败误当成当前域名健康状态。

### 4.4 P2 — Persistent Content Resilience（数据模型设计）

1. **持久化 FeedStateStore**：把既有 `SnapshotStore` 演进为可报告错误的 `FeedStateStore`：`Save(snapshot) error`、`Load(feedURL) (*FeedSnapshot, bool, error)`、`Delete(feedURL) error`、`Stats() (SnapshotStoreStats, error)`。内存 LRU 作 L1、SQLite 作 L2；L1 miss 回源 L2。有效的 live 结果始终可更新 L1；L2 在独立事务中写入，失败时返回并记录 durability error，明确标记“本进程可用但未持久化”，不能伪报 durable success。
2. **数据模型（决策编码，来自原型）**：

```
// 新表：feed_snapshots（last-good 持久化，仅在失败兜底时读取）
feed_snapshots {
  feed_url        TEXT UNIQUE      // CanonicalizeURL(feedURL)
  retrieved_at    INTEGER          // 抓取时刻
  fingerprint     TEXT             // sha256，完整性校验
  raw_content     BLOB             // ≤ 2MiB；永不进日志/API/响应
  content_length  INTEGER
  etag            TEXT             // 与正文同一行原子保存
  last_modified   TEXT
  validated_at    INTEGER          // 最近一次 200/304 成功验证
  source_at_capture TEXT           // primary/alternative（捕获时来源）
}
```

**一致性理由**：验证器只对产生它的响应正文有效，因此必须和 snapshot 原子保存，不能在 Podcast 与 snapshot 各保存一份权威值。Podcast 既有 `FetchErrorCount` 是否改成连续失败语义另开数据语义票，不作为 Conditional GET 前置，也不在本迁移中顺带改变。

3. **fallback 流程**：保持既有顺序不变（主 → 已验证 PodcastIndex 替代 → last-good）；升级点是 last-good 从内存扩展到“内存→磁盘”两级，且冷启动可从磁盘恢复。last-good 命中仍按既有语义**不更新** `LastFetchedAt`，避免把缓存命中伪装成新鲜上游验证；304 则更新 `ValidatedAt`，并由产品语义决定是否同步更新 `LastFetchedAt`，该决定须在 P0b 票中明确测试。
4. **stale 降级**：定义两个明确阈值：`FreshAge` 内为 fresh；超过 `FreshAge` 且不超过 `MaxStaleAge` 为 stale，可兜底但不伪装 fresh；超过 `MaxStaleAge` 不再作为内容成功返回。该语义与现有 `snapshotUsable` 保持一致。
5. **容量和清理**：延续现有单条 2 MiB、总量 32 MiB、最多 256 条默认上限；SQLite 也必须执行同样的条数/总量边界。保存新快照前按 `validated_at/retrieved_at` 淘汰最旧记录；清理和写入在同一事务中，暴露淘汰数、当前字节数和写失败计数。
6. **持久化安全**：走版本化迁移（`CurrentSchemaVersion` 递增 + `migrationRegistry` 追加），**仅** `cmd/migrate` 在备份+确认后写生产库；API 启动只做只读 schema 校验，不自动改表。snapshot 正文遵守既有白名单（不进日志/API/凭据）。

### 4.5 P3 — 403 Diagnostic System

1. **结构化日志**：Feed 抓取失败改用 `logger.WithFields`，字段白名单：`feed_id` / `podcast_id` / `feed_url`(脱敏) / `target_domain` / `http_status` / `error_category` / `circuit_state` / `attempt` / `retry_count` / `configured_egress_label` / `response_time_ms` / `response_bytes` / `cache_status` / `freshness` / `failure_phase` / `retry_after` / `identity_verification` / `snapshot_retrieved_at` / `validated_at`。**不**记录正文、Cookie、凭据、任意响应头。
2. **连接阶段诊断（`failure_phase`）**：使用 `httptrace` + 包装 Dialer/Response.Body 记录 `dns` / `connect` / `tls` / `response_header` / `body_read`。收到 HTTP 状态（包括 403）时阶段只能是 `response_header` 或 `body_read`，不能标成 connect。阶段字段帮助分类，但不能单独判定 WAF/CDN 或 IP 根因。
3. **轻量内存计数器**：新增进程内计数器 registry（**不引入 Prometheus 依赖、不新增 `/metrics` 端点、不修改 `/health` `/ready` 响应**），经独立、受保护的 admin JSON 入口暴露。计数维度：`feed_fetch_total{domain,status,category,source}`、`feed_fetch_duration_seconds`（固定分桶）、`circuit_state{domain,state}`、`circuit_transitions_total{domain,from,to}`、`last_good_hits_total`、`conditional_get_total{result:304/200/miss}`、`retry_total{domain}`。禁止以完整 Feed URL、podcast_id 等高基数字段作为计数器 label。
4. **配置出口标签**：将字段语义明确为 `ConfiguredEgressLabel`（默认 `direct`），只说明应用被配置成使用哪条路径，不能证明真实公网出口。#22 对照实验仍必须同时记录网络侧实际出口证据；应用标签不得单独用于 Go 判定。
5. **403 诊断 runbook**（见 §7.2）：基于上述字段组织待验证假设和所需对照证据，不输出无证据根因结论。

### 4.6 配置化与热更新

- 新增 `FeedConfig` 配置段（YAML/ENV 可覆盖）：`user_agent`、`timeouts{connect,tls,header,overall}`、`headers{accept,accept_language}`、`retry{budget,jitter}`、`circuit{thresholds_per_category,domain_evidence_min_distinct_feeds,half_open_max,successes_to_close}`、`snapshot{durable,bounds}`、`diagnostics{admin_enabled,configured_egress_label}`、`domain_policies[]`。
- 首版配置在进程启动时加载，通过现有发布/重启流程生效。只有证明能原子替换策略、保持 semaphore/circuit/in-flight 状态一致，并补并发竞态测试后，才新增 `Coordinator.ReloadPolicies`；热更新不是首版验收项。

### 4.7 必须守住的既有约束（不可破坏的决策）

- **fallback 链顺序固定**：主 → 已验证 PodcastIndex 替代（稳定身份校验）→ last-good。替代源必须通过 iTunesID/PodcastGUID + 标题/作者/单集证据，**不得**按标题相似度切换；`feed.xyzfm.space` 候选仍被排除。
- **AccessOutcome / JobExecution 字段白名单**：有意不含正文、Cookie、凭据、任意响应头（既有注释明确）。新增字段遵守同一白名单。
- **明确禁止**（来自 #22/#24/韧性研究 P3）：住宅代理、共享代理池、按请求轮换 IP、浏览器/客户端伪装、绕过验证码、开放任意 URL 转发、无限重试。固定出口组件在 #22 重开门槛达成前**不引入**。
- **合规前置**：诚实 UA、尊重 robots（24h 缓存）、低频诚实抓取、尊重 `Retry-After`；任何真实“换出口”动作继续受 #22 双窗口证据和所有者明确生产授权约束。本 Spec 不把联系上游设为普通 Fetcher 改造的前置，也不代替法律判断。
- **迁移纪律**：版本化迁移 + `CurrentSchemaVersion` 递增，`cmd/migrate` 唯一生产写入口，API 启动不自动改表。

---

## 5. Testing Decisions（验收标准 + seam）

### 5.1 测试 seam

- **主观测 seam（复用）**：`Fetcher.FetchFeedWithContextDetailed` / `Coordinator.Do` 继续以 `*FetchResult.AccessOutcome` 暴露外部结果。P0/P1/P2/P3 分别观测 `CacheStatus`/验证器、`CircuitState`、`SourceType=last_good`/快照时间、`ErrorCategory`/配置出口标签/`FailurePhase`。
- **必要的新生产 seam**：新增窄类型 `RequestValidators` 和可注入 `FeedStateStore`。Coordinator 负责加载/校验状态并把验证器传入 Fetcher；Fetcher 不读取数据库。持久化实现和内存 fake 使用同一接口，错误必须可观测。
- **HTTP 假上游**：`httptest.Server` 录制请求头并脚本化返回 200/304/403/429/503/gzip/慢响应头/慢响应体，覆盖条件请求、304 恢复、Go 自动 gzip 解压和 HTTP 语义。
- **网络阶段 harness**：自定义 Resolver/Dialer、可控 `net.Listener`、`httptest.NewTLSServer` 与包装 Body 分别制造 DNS、connect、TLS、response-header、body-read 错误；注入 fake clock/sleeper 验证退避、Retry-After、冷却和抖动，禁止依赖真实 sleep 或“多跑几次看随机性”。
- **P3 诊断观测**：同一 `AccessOutcome` seam + logrus test hook 捕获结构化字段；计数器通过独立 admin JSON handler 断言，不修改 health handler 测试契约。

**测试哲学**：只测外部行为（AccessOutcome 字段、日志字段、metrics 值），不测内部实现细节（不锁死具体结构体字段名、不依赖私有方法）。现有先例：Coordinator 去重/共享缓存/last-good 单测、断路状态转换单测、access 分类与 URL 规范化单测——新测试沿用这些既有 seam 与风格。

### 5.2 验收标准

**P0 验收**

- 真实发出的请求头包含诚实 UA 与 `Accept`；调用方没有显式设置 `Accept-Encoding`，gzip 响应仍被 Go Transport 自动解压并成功解析。
- 200 的 snapshot 与验证器在同一事务保存；下次请求带 `If-None-Match`。304 时恢复同一快照，`Feed` 非 nil、`CacheStatus == not_modified`、`ValidatedAt` 更新而 `RetrievedAt` 不变，不计失败、不触发熔断、不消耗重试。
- 模拟“有验证器但快照缺失/损坏”时，不发送条件请求；模拟竞态收到 304 却无法恢复时，只补一次无条件 GET，不能返回空增量或 nil Feed。
- 429/503 带 `Retry-After` 时，由 fake clock/sleeper 证明下一请求不会提前；full jitter 使用注入随机源做确定性边界测试。
- 分层超时生效：测试分别稳定识别 DNS、connect、TLS、response-header 和 body-read，不用真实 30s 等待。
- robots 缓存命中时不重复拉取 robots.txt；被 robots 禁止的路径不请求。

**P1 验收**

- 状态序列可观测为 `Closed → Open → HalfOpen → Closed`（经 `AccessOutcome.CircuitState`）。
- `feed.xyzfm.space` 第一次 403 立即 OPEN；OPEN 期间同域名请求被短路且不会再次访问上游。probe 再失败后冷却递增并保留周期性 probe。
- 单个 Feed 的 timeout/5xx 不阻断同域其他 Feed；达到“不同 Feed 数”证据阈值或显式域名策略后才进入域名熔断。
- HALF_OPEN 放行限量 probe；连续成功达 `SuccessesToClose` 才回 CLOSED；任一 probe 失败回 OPEN。
- 配置经重启生效且默认策略不扩大现有域名熔断范围；热更新若后续实现，必须另有并发与状态迁移验收。

**P2 验收**

- 重启后 last-good 仍可从磁盘 `Load` 命中；数据库 miss、锁冲突、损坏分别可观测，不能都降格为 miss。
- `FreshAge < age <= MaxStaleAge` 标为 stale 并可兜底；`age > MaxStaleAge` 不作为成功内容返回。
- `feed_snapshots` 表经版本化迁移创建；`cmd/migrate` 幂等、可回滚；API 启动不改表。
- 超过条数或 32 MiB 总量时按最旧验证时间确定性淘汰；并发保存、L2 写失败和清理回滚后 L1/L2 不产生伪持久化状态。
- snapshot 正文不出现在任何日志/API 响应中（白名单测试）。

**P3 验收**

- 失败日志为结构化字段，断言含 `feed_id`/`error_category`/`failure_phase`/`configured_egress_label` 等键（logrus hook）。
- 独立受保护的 admin JSON 入口暴露抓取计数、延迟、断路状态、last-good 命中率、304 命中率；`/health` 和 `/ready` 响应保持不变。
- `failure_phase` 能区分 DNS/connect/TLS/读头/读体；HTTP 403 只能记录在 response-header/body-read 阶段。
- `ConfiguredEgressLabel` 可配置且默认 `direct`；界面、日志和 runbook 均明确它不是实际公网出口证明。

### 5.3 测试先例

- 现有 Coordinator 单测（去重合并、共享缓存、失败不永久阻塞、跨实例共享）→ P1 状态机与 P2 兜底沿用。
- 现有断路状态转换单测（403→OPEN→probe→CLOSED）→ P1 扩展为含 HALF_OPEN 的序列。
- 现有 access 分类 / URL 规范化 / SanitizeFeedURL 单测 → P0 条件请求头与 P3 脱敏沿用。
- 现有 `alternative_feed` 身份校验单测 → 保证 fallback 链顺序不被破坏。

---

## 6. Out of Scope

- **任何绕过风控手段**：住宅代理、共享代理池、按请求轮换 IP、浏览器/客户端伪装 UA、绕过验证码、开放任意 URL 转发、无限重试。
- **固定出口 / 中继组件**：#22 未达“同窗口同参数固定备用出口成功”双证据门槛、#24 判 NO-GO，本 Spec 不引入；如未来重开，仍须满足两窗口证据并取得所有者明确生产授权，作为独立可回滚变更。
- **改用户可见行为 / fallback 链顺序**。
- **改 `JobExecution` 既有持久化字段的语义或白名单**（仅新增，不破坏）。
- **维护脚本路径**（走独立 `gofeed.ParseURL`，与生产 Fetcher 互不影响）。
- **Next 跨主版本升级、搜索排序/缓存策略变化**（属人审边界，不在本 Spec）。

---

## 7. Further Notes

### 7.1 实施 Roadmap

按“低风险高收益先行、每层独立可回滚”推进：

| 阶段 | 内容 | 依赖 | 风险 | 可独立交付 |
| --- | --- | --- | --- | --- |
| **P0a** | 诚实 UA + `Accept` + Go 自动 gzip + 分层超时 + 有限 jittered backoff + Retry-After + robots 缓存 | 无迁移 | 低到中（会改变 HTTP 行为，需定向回归） | 是 |
| **P3a** | 结构化日志 + `httptrace`/包装 Body 的 `failure_phase` | P0a Transport seam | 低 | 是；先提供 P0/P1 观测基线 |
| **P2a** | 可报告错误的持久化 FeedStateStore + `feed_snapshots` 表 + 容量/淘汰/stale 边界 | 版本化迁移 | 中（新表 + BLOB + 清理事务） | 是 |
| **P0b** | Conditional GET + 304 从匹配快照恢复 + 无快照时一次无条件 GET | P2a | 中（触及抓取成功语义） | 是 |
| **P1** | 显式 HalfOpen + xyzfm 首次 403 立即断路 + 多 Feed 域故障证据 + 启动时配置 | P3a | 中（改抓取准入语义） | 是 |
| **P3b** | 轻量内存计数器 + 独立受保护 admin JSON + ConfiguredEgressLabel + 403 runbook | P3a | 中（新增受保护观测面） | 是 |

**顺序理由**：先做不依赖数据库的 P0a 与 P3a；再用 P2a 建立“正文+验证器”原子持久化，之后 P0b 才能安全处理 304。P1 保留现有 xyzfm 立即断路，只升级恢复与其他错误证据门槛。P3b 在访问控制确认后上线。每阶段独立测试、提交、发布和回滚；数据库迁移与行为变化不混在同一发布。

### 7.2 403 诊断 Runbook（依据结构化字段）

```
观测到 ErrorCategory=access_denied / HTTPStatus=403
│
├─ failure_phase = response_header/body_read 且响应耗时 ~100ms 级（快速 HTTP 拒绝）
│   └─ 只能确认上游或中间 HTTP 层快速返回 403；不能据此区分 IP/ASN、CDN、节奏或请求策略
│        → 按 target_domain+window+ConfiguredEgressLabel 聚合，并与网络侧实际出口证据配对
│
├─ 仅部分 Feed 失败，且与请求频率/并发相关
│   └─ 倾向频率/并发触发 → 检查 P0 retry budget、P1 阈值、调度错峰是否生效
│
├─ Conditional GET 未启用 / 304 未被识别为成功
│   └─ 这是客户端标准能力缺口，但不是 403 根因证据；核对 UA、Accept、验证器和快照是否成对生效
│
├─ 伴随 Retry-After 或间歇性成功
│   └─ 说明存在服务端等待提示或动态策略；遵守 Retry-After，仍不单独归因 CDN
│
└─ 持续全失败且无 Retry-After
    └─ 走研究记录决策门槛：低频直连恢复→节奏问题；直连+备用均败→上游统一策略/公共故障
```

### 7.3 与既有 ADR / 研究记录的对齐

- 本 Spec 是 [XIAOYUZHOU_FEED_ACCESS_RESILIENCE_2026-07-19.md](XIAOYUZHOU_FEED_ACCESS_RESILIENCE_2026-07-19.md) P0（设计约束）的**工程落地**：共享队列（既有 Coordinator）、条件请求（P0）、错峰（既有 + P0 robots/ttl）、`Retry-After`（P0/P1）、诚实 UA + robots（P0）、聚合指标（P3）。
- 本 Spec **不**触碰该研究 P1/P2（固定出口实验与中继）——那些仍 NO-GO；只推进 P0 + 新增 P2 持久化 + P3 诊断。
- fallback 链与 [XIAOYUZHOU_ALTERNATIVE_FEED_CANDIDATES_2026-07-20.md](XIAOYUZHOU_ALTERNATIVE_FEED_CANDIDATES_2026-07-20.md) 一致，仅把 last-good 从内存升级为持久化。
- 用词沿用既有 `feed/access.go` / `feed/coordinator.go` 常量定义的事实术语，不另造新词。

### 7.4 需补进人审队列的事项

以下事项需补入 [../HUMAN_REVIEW_QUEUE.md](../HUMAN_REVIEW_QUEUE.md)（其最后更新停留在 2026-07-13，早于本轮 403 复发窗口）：

1. **Feed 抓取参数配置化与默认值**：把冷却、retry budget、超时、UA 改为启动时配置；所有域名可采用保守负载整形，但域名熔断默认不扩张。`feed.xyzfm.space` 必须保留首次 403 立即断路。
2. **持久化 last-good 迁移**：新增 `feed_snapshots` 表，正文与验证器原子保存，走 `cmd/migrate`；不在 Podcast 表复制验证器，不顺带改变 `FetchErrorCount` 语义。
3. **调度器连续失败通知策略（既有条目延伸）**：本 Spec 的 P3 让 feed 级连续失败可观测，是否把“只打日志”升级为 feed 级通知需单独确认渠道/阈值/静默时段。
4. **指标暴露入口**：新增独立受保护 admin JSON，不改 `/health` `/ready`；需先确认访问控制和是否启用。
5. **ConfiguredEgressLabel**：只是配置标签，不是实际公网出口证据；未来对照实验必须配对网络侧出口记录。
6. **Feed 网络兼容边界**：私网/回环阻断与跨域重定向白名单可能破坏局域网 Feed 或正常迁移；首版只限制 scheme 和跳数。任何进一步收紧须先审计现有订阅并确认局域网允许策略。

---

## 本 Spec 没有证明的事项

- 没有证明 `feed.xyzfm.space` 的 403 必然按 IP/ASN、频率、指纹或 CDN 中某一类触发；本 Spec 只建设**让根因可被诊断**的能力，不预设根因。
- 没有测试或部署任何代理、固定出口或生产配置变更。
- 条件请求的减流效果取决于上游是否实际返回 ETag/Last-Modified（研究记录样本未返回），因此 P0b 是标准能力建设，不承诺解决当前小宇宙 403；减流效果需指标上线后实测。

因此，本 Spec 是合规、低负载、可观测的工程方案，不是对绕过上游访问控制的授权。
