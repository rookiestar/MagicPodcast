# Focus 自动化播客加工与单集助手 Spec

日期：2026-08-24

状态：已确认设计；#179 Foundation 已在实施分支完成，尚未合并、迁移或部署

关联：[领域词汇](../../CONTEXT.md)、[ADR-0007](../adr/0007-use-local-codex-runtime-for-intelligent-processing.md)、[ADR-0008](../adr/0008-separate-action-processing-and-knowledge-delivery-state.md)

GitHub Spec：[#178](https://github.com/rookiestar/MagicPodcast/issues/178)

## 1. 结论

MagicPodcast 新增一个面向单集的深度加工闭环：

```text
Focus 加工资格
  ├─ 手动触发 ─┐
  └─ 定时选择 ─┴─> 加工运行
                    ├─ Mac mini Codex Runtime
                    ├─ 飞书妙记逐字稿
                    ├─ 本地规范化逐字稿 + 单集纪要
                    └─ 知识桥
                        ├─ Gemini Notebook Enterprise
                        └─ ima 标准导入包

当前单集划词/提问 ─> MagicPodcast API ─> 只读 Codex 会话 ─> 带来源答案
```

核心分工：

- Codex 负责需要模型判断的摘要、结构化提炼、资料关联、公开网页检索和回答生成。
- MagicPodcast 负责权威状态、幂等、调度、重试、取消、产物版本、权限和审计。
- 飞书妙记首期负责音频转写；供应商能力位于 Adapter 后，不进入核心领域接口。
- 外部知识中心只接收规范化逐字稿、单集纪要和来源信息，默认不接收音频。

本方案借鉴 LibraXG M0 的每次执行隔离、原生身份绑定、流式事件、定向取消和 fail-closed 原则，但不复制其进程内执行跟踪器，也不把产品状态交给 Codex Runtime。

## 2. 已确认决策

1. 生产控制面位于 Mac mini，由本地 Codex CLI/Runtime 承担全部智能化操作。
2. “本地”不等于离线推理；OpenAI、飞书和 Google 是可配置的白名单外部处理方。
3. Focus 只赋予加工资格，入列本身不启动运行。
4. 手动和定时加工复用同一入口；定时模式目标为无人值守。
5. 原始音频继续本地保留；MagicPodcast 保存规范化逐字稿、单集纪要和来源追溯。
6. 首期转写只接飞书妙记，但保留真实的测试 Adapter 和未来替换 Adapter。
7. 加工失败、取消或第三方交付失败均不改变 Focus、Done 或完成事实。
8. 外部知识中心默认接收文本知识包和来源链接，不上传音频。
9. Google 知识中心只采用官方 Gemini Notebook Enterprise（NotebookLM Enterprise API）；先做真实 14 天试用 Spike，再决定生产接入。
10. ima 首期不做浏览器自动化、逆向接口或无人值守上传，只交付标准导入包和占位 Adapter。
11. 单集助手首期只围绕当前单集、只读，可检索公开网页，答案必须附来源。
12. 数据库迁移、生产凭据配置、云试用开通、部署和真实数据运行均需另行授权。

## 3. 当前事实与问题

### 3.1 已实现事实

- [`EpisodeTriageDecision`](../../backend/internal/models/episode_triage_decision.go) 保存 Inbox、Focus、Someday、Done 等行动状态，没有加工状态。
- 现有 [`Workflow` / `Job`](../../backend/internal/models/workflow.go) 面向 Feed 同步和工作流报告，并按工作流约束活动 Job；不适合作为单集加工运行的别名。
- 现有 [`SchedulerRun`](../../backend/internal/models/scheduler.go) 和调度器已有 cron、跳过记录与错过触发补偿，可复用调度思想，但不能让单集加工依附于工作流 Feed 语义。
- 现有 [`ExecutionTracker`](../../backend/internal/workflow/execution_tracker.go) 是进程内 `map`，只能辅助取消，不能承担重启后的权威队列。

### 3.2 需要解决的问题

- Focus 当前只能表达投入意图，不能说明某集是否已转写、正在加工或交付失败。
- 飞书妙记转换是异步外部过程，服务重启、重试和未知结果都可能造成重复上传。
- Gemini Notebook Enterprise 和 ima 的自动化成熟度不同，不能用一个“已同步”布尔值掩盖差异。
- 浏览器直连 Codex 会泄露凭据并绕过服务端权限、审计与上下文裁剪。

## 4. 目标与非目标

### 4.1 首期目标

- Focus 单集可手动发起加工，并查看步骤、结果、失败原因、取消和重试。
- 可配置 cron 定时选择 Focus 单集，无人值守执行同一加工管道。
- 通过飞书妙记生成逐字稿，再由 Codex 基于逐字稿独立生成单集纪要。
- 产物以稳定格式保存到本地，并可独立交付到 Gemini Notebook Enterprise 或导出为 ima 标准包。
- 在单集详情/show notes 中支持划词提问、当前单集问答和公开关联资源检索。

### 4.2 首期非目标

- 入 Focus 立即自动加工。
- 全库 RAG、跨单集聊天、个性化长期记忆或 Copilot 自动修改内容。
- 本地 Whisper/其他 ASR fallback；首期飞书不可用即明确失败。
- 把飞书妙记 AI Summary 直接当作规范单集纪要。
- 向外部知识中心上传音频。
- 消费者版 NotebookLM 的浏览器自动化。
- ima 自动登录、DOM 自动化、私有接口调用或抓取。
- 多用户权限模型、移动端离线加工、生产级分布式队列。

## 5. 领域状态

### 5.1 三类独立状态

| 状态 | 回答的问题 | 权威来源 |
| --- | --- | --- |
| 行动队列 | 用户下一步想把注意力放在哪里？ | 既有消费状态 |
| 加工运行 | 这一集的一次加工请求进行到哪里？ | 新的持久加工记录 |
| 外部交付 | 某版产物是否已交给某个知识中心？ | 每目标独立交付记录 |

规则：

- 手动触发时单集必须仍在 Focus。
- 定时器按 Focus 持久顺序选择当前候选；排队前再次核对资格。
- 运行一旦被权威记录为已开始，之后移出 Focus 不会隐式取消；用户可显式取消。
- 运行完成、失败或取消均不自动移动单集，也不创建或删除完成事实。
- 已完成的本地产物不会因外部交付失败而降级。

### 5.2 加工运行状态

```text
queued -> running -> waiting_external -> running -> completed
   |         |              |             |
   └─────────┴──────────────┴─────────────┼-> failed
                                         └-> cancelled
```

- `queued`：已持久入队，尚未占用 Runtime。
- `running`：正在执行本地步骤或 Codex Turn。
- `waiting_external`：已提交飞书/Google 请求，等待异步结果；保存远端身份后才能进入。
- `completed`：规范化逐字稿和单集纪要已原子发布为当前产物集。
- `failed`：必需产物未完成；保留已完成检查点和可读失败码。
- `cancelled`：停止本地后续工作；已经提交的外部任务可能继续，必须在详情中说明。

服务重启不得把非终态永久留为“运行中”。恢复器按最后检查点继续安全轮询或重试；如果无法确认外部结果，fail-closed 为可重试失败，不猜测成功，也不重复创建远端资源。
普通 API 启动保持只读；只有显式启动的加工 Worker 才在领取持久任务前执行恢复并写回收口结果。

### 5.3 外部交付状态

每个 `产物集 × 目标知识中心 × 目标位置` 独立记录：

```text
pending -> delivering -> delivered
                  └----> failed
                  └----> cancelled
```

Gemini Notebook Enterprise 失败不影响 ima 包，ima 未自动上传也不影响 Google 目标；再次交付只重试目标记录。

## 6. Processing Module

### 6.1 外部 Interface

加工 Module 对 UI、手动入口和调度器只暴露小接口：

- `StartEpisodeProcessing`：给定单集、触发来源和是否强制重加工，返回现有或新运行。
- `CancelProcessingRun`：幂等取消一个非终态运行。
- `GetProcessingRun`：返回状态、步骤、产物、失败与交付摘要。
- `ListEpisodeProcessingRuns`：读取一集的历史运行。

手动和定时入口必须调用同一个 `StartEpisodeProcessing`，不能各自实现上传、重试或去重逻辑。
调用方只提交单集、触发来源和是否强制重加工；音频摘要与管道版本由服务端
`ProcessingInputResolver` 从受管音频和当前管道配置解析，浏览器或调度请求不能自行指定幂等身份。
在后续阶段尚未接入受管音频 Resolver 时，Start 契约必须以
`processing_input_unavailable` 失败关闭，不能用 URL 指纹或客户端值冒充音频摘要。

### 6.2 内部 seams

| Seam | 生产 Adapter | 测试 Adapter | 职责 |
| --- | --- | --- | --- |
| Codex Runtime | Mac mini 本地 Codex CLI/Runtime | 脚本化 Fake Runtime | 执行受限 Turn、流式事件、取消和结构化结果 |
| Transcription | 飞书妙记 CLI/Skill | 确定性 Fake Minutes | 音频上传、妙记创建、异步查询和原始产物读取 |
| Artifact Store | 本地受管目录 + 数据库元数据 | 临时目录 | 原子发布、版本、校验和与来源追溯 |
| Knowledge Bridge | Gemini Notebook / ima Adapter | 内存 Fake Bridge | 幂等交付标准知识包并返回回执 |

这些是 Module 内部 seam，不暴露给页面。核心状态只依赖规范结果，不依赖 `minute_token`、Notebook ID 等供应商字段。

### 6.3 Runtime Host

- 每个加工运行或助手请求拥有独立 Runtime execution identity、工作目录、取消句柄和事件流。
- Codex 会话只提供执行能力；数据库中的加工运行才是权威状态。
- 批处理优先采用官方推荐的自动化集成面；需要会话历史和流式交互的助手可使用 app-server 稳定 `stdio` JSONL。
- 不使用 app-server 实验性 WebSocket 作为生产边界；浏览器只连接 MagicPodcast API。
- Runtime 启动前验证 Codex CLI 版本、认证、目标工作目录和必需 Skill/CLI；失败时标记 `runtime_unavailable`。
- 不继承 `danger-full-access`、免审批或任意 shell 权限。权限按运行类型声明，未知工具请求 fail-closed。

允许的首期运行类型：

| 类型 | 可读 | 可写 / 可调用 |
| --- | --- | --- |
| 飞书转写 | 当前音频、运行清单 | 受管产物目录、`lark-cli drive/minutes` |
| 单集纪要 | 当前逐字稿、show notes、单集元数据 | 受管产物目录 |
| 知识交付 | 当前标准知识包 | 已配置的目标 Adapter |
| 单集助手 | 当前单集上下文、公开网页 | 不写产品或知识中心 |

## 7. 触发、幂等与恢复

### 7.1 触发规则

- 手动：Focus 卡片和单集详情提供“加工”；重复点击返回同一活动运行。
- 定时：用户显式启用 cron 后，按本机时区和 Focus 顺序选择候选；每批有可配置上限，首期默认串行。
- 定时任务只处理当前没有活动运行、且没有同版本最新产物的 Focus 单集。
- 手动默认复用最新同版本成功结果；“强制重加工”必须是单独确认的动作。

### 7.2 幂等键

- 加工键：`episode identity + audio digest + pipeline version`。
- 活动互斥：同一单集最多一个非终态加工运行。
- 飞书步骤在调用前持久化请求意图，在成功后立即保存 `file_token`、`minute_token` 和状态；恢复时优先查询已知远端身份。
- 外部交付键：`artifact set + target + destination + adapter version`。
- 自动重试属于同一逻辑运行的有界 attempt；用户强制重加工创建新运行并关联上一运行。

### 7.3 重试与取消

- 仅对明确可重试的网络、限流、暂态外部状态使用指数退避和抖动。
- 认证、权限、格式超限、缺少音频、Schema 不匹配等错误立即失败并给出行动建议。
- 自动重试必须有次数和总时长上限；不得无限占用 Mac mini。
- 取消先中断 Codex Turn，再停止后续步骤；不能撤回的飞书/Google请求只记录“远端可能继续”。

## 8. 飞书妙记加工

### 8.1 标准流程

按当前 `lark-minutes` Skill 合同执行：

```text
本地音频
  -> lark-cli drive +upload（user 身份，取得 file_token）
  -> lark-cli minutes +upload（取得 minute_url / minute_token）
  -> 异步等待
  -> lark-cli minutes +detail --transcript --summary --chapter --keyword
  -> 保存原始供应商产物
  -> Codex 基于 Transcript 独立生成规范单集纪要
```

限制：

- 支持的原始媒体必须满足飞书当前格式、最长 6 小时、最大 6 GB 限制。
- 首期使用 user 身份和最小权限；不得在日志中输出 token、授权链接或完整音频路径。
- `minutes +upload` 返回链接不等于转写完成；必须持久轮询实际产物状态。
- 逐字稿是规范单集纪要的主要事实来源；飞书 AI Summary 只作为可追溯供应商产物，不直接成为最终纪要。
- 上传后的飞书文件和妙记首期保留，保存远端来源信息；自动清理需另行设计和授权。

### 8.2 失败语义

- 飞书不可用时不自动改用其他 ASR，不伪造空逐字稿。
- 逐字稿已完成而纪要生成失败时，原始产物与检查点保留，运行标记失败，可从纪要步骤重试。
- 权限不足、登录过期或文件超限给出明确失败码，不把它们归类为网络重试。

## 9. 本地产物合同

每次成功加工原子发布一个不可变产物集，至少包含：

| 产物 | 内容 |
| --- | --- |
| `manifest.json` | 单集身份、音频摘要、管道/提示词/Skill/Adapter 版本、时间、来源和远端身份的受限引用 |
| `transcript.md` | 规范化逐字稿，保留时间戳和可用的说话人信息 |
| `episode-notes.md` | 单集概览、关键观点、结构章节、提及资源、待核问题和逐字稿引用 |
| `raw/` | 飞书返回的原始逐字稿与选定 AI 产物，不作为默认外部交付正文 |

要求：

- 原始下载音频继续保留在现有本地素材位置，产物清单只引用并记录校验和。
- 新产物集完全写入、校验后再切换“当前版本”；失败不得覆盖上一成功版本。
- 产物文本采用 UTF-8 和稳定 Markdown；外部 Adapter 不直接读取任意本地路径。
- Token、凭据和私有授权 URL 不进入 Markdown、日志或外部知识包。

## 10. Knowledge Bridge

### 10.1 标准知识包

所有目标先接收同一个标准包：

- 单集标题、节目、发布日期、来源 URL、show notes；
- 规范化逐字稿；
- 单集纪要；
- 产物版本和来源说明；
- 不包含音频、凭据、飞书 token、私有本地路径。

Adapter 只能返回规范回执：目标、远端对象身份、状态、时间、可重试错误和公开链接。供应商字段留在 Adapter 内。

### 10.2 Gemini Notebook Enterprise

已验证的官方能力：

- Notebook/source API 仍标记为 Preview。
- API 支持上传 Markdown 和多种音频类型；本项目首期只上传 Markdown。
- Google Cloud 通用认证支持 ADC、服务账号模拟和 Workload Identity Federation。
- 许可文档明确登录 Gemini Notebook Enterprise 界面的用户需要席位，订阅至少 15 席；14 天试用提供 5000 席。文档未明确服务账号 API 是否需要用户席位。

因此生产 Adapter 前必须完成真实 Spike：

1. 开通 14 天试用和隔离测试项目。
2. 用计划中的无人值守身份创建 Notebook。
3. 上传标准 Markdown 包，轮询 Source 到完成。
4. 重复调用验证幂等与更新/替换策略。
5. 验证服务账号/WIF、IAM、区域、许可和费用边界。
6. 决定 Notebook 分组策略；当前不预设“一集一个 Notebook”或“一个节目一个 Notebook”。

任一生产门槛不成立时，只保留标准知识包，不实现浏览器自动化 fallback。

### 10.3 ima

截至 2026-08-24，未发现 ima.copilot 面向知识库写入的公开官方 API/CLI；现有公开服务协议对未授权技术调用、第三方工具接入和抓取有明确限制。官方 Skill 的存在不能证明它支持无人值守写入知识库。

首期只交付：

- ima 兼容的 Markdown/JSON 标准导入包；
- `manual_import` 能力的占位 Adapter 和明确状态；
- 官方 API/Skill 可写能力的持续研究记录。

首期禁止：

- 自动登录和操作 ima 网页；
- 调用逆向或未公开内部接口；
- DOM/浏览器批量上传、抓取或绕过访问限制；
- 把“导入包已生成”显示为“ima 已交付”。

## 11. 单集助手

### 11.1 首期能力

- 在 show notes 或逐字稿中划词后提问。
- 围绕当前单集解释概念、核对说法、总结选段。
- 查找单集提及或语义相关的公开网页、论文、书籍和项目。
- 回答附可点击网页来源，以及可定位的 show notes/逐字稿引用。

### 11.2 上下文与权限

- 页面把问题、选中文本和单集 ID 发给 MagicPodcast API，不发送本地路径、密钥或 Runtime 地址。
- 服务端从当前成功产物加载最小上下文，并通过 Runtime Host 建立只读执行。
- 没有逐字稿时可降级到 show notes，但答案必须明确“未使用逐字稿”。
- 私有备注默认不进入 Codex 上下文；用户对单次问题显式选择“包含我的备注”后才可加入，且不得拼入公开网页搜索查询或作为外部来源公开。
- 首期只保留当前页面会话和必要审计，不建立跨单集长期记忆。

### 11.3 答案合同

每个事实性答案包含：

- 简洁答案；
- `单集内部来源`：show notes 段落或逐字稿时间戳；
- `公开外部来源`：标题、URL 和访问时间；
- 不确定性或来源冲突说明。

没有足够来源时应直接说明，不允许生成无来源的“关联资源”。助手不能修改笔记、队列、逐字稿、纪要或外部知识中心。

## 12. 安全、隐私与审计

- 首次启用时明确展示数据流：Mac mini → OpenAI/Codex、飞书、Google；ima 首期只生成本地包。
- 凭据仅存于主机受限配置或系统凭据存储，不入数据库、产物、Issue 或日志。
- 每次运行记录 Codex CLI/Runtime、Skill、Adapter 和管道版本，以及触发来源、步骤耗时、取消方式和脱敏外部回执。
- Runtime 工作目录限制在该运行的受管目录；不得把仓库、数据库、备份和其他单集作为默认上下文。
- 公网检索只允许 HTTP(S) 公开资源；私网、回环、文件 URL 和凭据化 URL 默认拒绝。
- 音频上传飞书和文本上传 Google 属真实外部写入；测试和生产凭据启用均需独立授权与审计。

## 13. 用户体验不变量

### 13.1 加工

| 场景 | 用户结果 |
| --- | --- |
| 正常 | 可见当前步骤、完成产物、来源和各目标交付状态；Focus 位置不变。 |
| 慢请求 | 显示正在等待 Codex/飞书/Google及最近更新时间；页面其他内容可用。 |
| 请求失败 | 保留上一成功产物和已完成检查点，显示可行动失败原因与重试；不移动队列。 |
| 首次访问 | 先显示“尚未加工”，不把未加载误写为“没有逐字稿”。 |
| 服务重启 | 非终态运行被恢复或明确收口，不永久显示假运行。 |

### 13.2 单集助手

| 场景 | 用户结果 |
| --- | --- |
| 正常 | 流式显示答案和来源；首个可读内容出现后仍可继续加载来源。 |
| 慢请求 | 保留问题和选区，显示可取消的局部等待态，不阻塞单集阅读。 |
| 请求失败 | 保留当前单集内容和问题，显示重试；不出现空白详情页。 |
| 首次访问 | 明确可用上下文范围；无逐字稿时提前说明降级。 |

实现验收必须同时记录 API 完成时间与首个有效内容出现时间，不能只证明请求被快速接受。

## 14. 分阶段实施

截至 2026-08-24，阶段 A 已在 `codex/focus-processing-spec` 实现并通过本地自动测试；这不代表 PR 已合并、生产数据库已迁移或运行态已部署。阶段 B–H 尚未实现。

| 阶段 | 交付 | 依赖 |
| --- | --- | --- |
| A | 持久加工运行、产物合同、Fake Adapters | 无 |
| B | Mac mini Codex Runtime Host 与受限执行 | A |
| C | 飞书妙记手动加工闭环 | B |
| D | Focus 定时加工、恢复、重试、取消与状态展示 | C |
| E | Gemini Notebook Enterprise 无人值守 API Spike | 可与 A–D 并行 |
| F | Gemini Notebook Enterprise Adapter | E 成功 |
| G | 当前单集只读助手 | B |
| H | ima 标准导入包与占位 Adapter | A |

实施依赖：

```text
A Foundation -> B Runtime -> C 飞书手动 -> D 定时加工
                         └-> G 单集助手
E Gemini Notebook Spike -> F Gemini Notebook Adapter
A Foundation -> H ima 导入包
```

## 15. 验收证据

### 15.1 Foundation / Runtime

- 数据库约束证明同一单集只有一个活动运行，幂等键和交付键无重复。
- 重启收口、取消、有限重试、检查点恢复和上一成功产物保护均有自动测试。
- Fake Runtime/Minutes/Bridge 通过同一 Module interface 的 conformance。
- 真实 Codex Smoke 证明创建、流式、取消、终态和无孤儿进程；保存脱敏事件证据。

### 15.2 飞书手动与定时

- 使用获批测试音频完成真实 `drive +upload → minutes +upload → transcript → episode-notes`。
- 真实证据包含逐字稿、独立纪要、来源追溯和远端身份回读，不包含凭据或完整私有正文。
- 定时器证明按 Focus 顺序选择、重复触发不重复、移出 Focus 的排队项会跳过、已开始项不被隐式取消。
- 飞书权限过期、异步慢、文件超限、重启和取消路径均有可观察结果。

### 15.3 知识桥

- Gemini Notebook Spike 真实证明无人值守认证、创建、上传、轮询和幂等；许可/费用未确认则不得进入 Adapter 票。
- Gemini Notebook Adapter 通过 Fake 和真实测试，外部失败不影响本地产物。
- ima 只能验收“包可导入”和状态诚实，不得宣称自动交付。

### 15.4 单集助手

- 只使用当前单集上下文；无逐字稿降级可见。
- 每个事实性回答有单集或公开网页来源；无来源时明确拒绝推断。
- 划词、慢响应、失败、取消和公开网页检索在真实页面验收。
- 测试证明不能修改任何产品状态、产物或外部知识中心。

### 15.5 授权边界

本 Spec 和实施票据不授权：

- 数据库迁移 apply；
- 飞书/Google 生产凭据配置或试用开通；
- 上传真实播客音频；
- commit、push、合并、部署或生产验收。

## 16. 一手来源与未验证门槛

- [OpenAI Codex app-server](https://learn.chatgpt.com/docs/app-server)：深度产品集成、稳定 stdio JSONL、会话/流式/取消；WebSocket 标记为实验且不支持生产。
- [Gemini Enterprise authentication](https://docs.cloud.google.com/gemini/enterprise/docs/authentication)：ADC、服务账号模拟和 WIF。
- [Gemini Notebook Enterprise notebooks/sources API](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/api-notebooks-sources)：Preview、Markdown/音频来源和异步 Source 状态。
- [Gemini Notebook Enterprise licensing](https://docs.cloud.google.com/gemini/enterprise/notebooklm-enterprise/docs/set-up-licensing)：界面用户许可、最低 15 席和 14 天试用。
- [腾讯 ima 用户服务协议](https://rule.tencent.com/rule/202410140002)、[ima 隐私保护指引](https://rule.tencent.com/rule/202503260010)：当前生产集成必须服从的官方规则入口。

仍需真实验证：

- Mac mini 当前 Codex/lark-cli 版本、认证、权限和运行资源；
- Codex 自动化批处理最终采用 SDK/exec 还是 app-server stdio；
- 飞书妙记异步状态、限流、远端资源保留和生产账号配额；
- Gemini Notebook 服务账号/WIF 是否可完成全链路、许可是否适用于 API 身份；
- Notebook 分组与更新策略；
- ima 是否发布受支持的知识库写入 API/CLI/Skill。
