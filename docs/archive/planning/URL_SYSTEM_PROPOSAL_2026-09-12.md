# 全站 URL 体系排查与建议

日期：2026-09-12。状态：建议方案，未实施、未形成已批准 ADR。

## 结论与证据边界

问题成立：当前 URL 主要标识入口页，不能完整标识用户正在阅读或处理的对象及位置。修复应覆盖“从 URL 恢复界面”和“操作界面更新 URL”两个方向，不能只增加复制链接按钮。

本次通过用户指定的 `http://127.0.0.1:18089` 进行只读 HTTP 检查：

- `/inbox` 返回 200；`/health` 报告 production，release 为 `20260912T013104Z-730dc72-51898`。
- Focus API 确认《#714.Tibo 谈 Codex：harness 总比模型快一步》的单集 ID 为 **66413**。714 是标题中的节目编号，不是数据库 ID。
- 线上 Inbox JavaScript 包含“人物与发言”及现有 `queue/episode/detail` 读取逻辑，与本地可读取的 `730dc72` Git 对象交叉核对。
- 当前主工作区为 `c299f68` 且有用户已有改动，本地数据 Profile 为 Fixture。线上排查以发布版本为依据，不把当前工作区当成生产版本。
- 未操作用户浏览器、未修改生产数据或服务。以下导航缺口来自发布源码与线上脚本；HTTP 200 不代表浏览器交互验收通过。

现有临时链接：

`http://127.0.0.1:18089/inbox?queue=focus&episode=66413&detail=1`

它符合现有详情入口协议，且当前该单集确在 Focus；仍不能直达转写和人物面板，也没有实际浏览器点击验收。单集移出 Focus 后该链接可能失去定位能力。

## 全站排查

以下文件路径均相对仓库根目录，事实以 `git show 730dc72:<path>` 为准。

| 页面/区域 | 当前 URL 能力 | 缺口 | 证据文件 |
| --- | --- | --- | --- |
| 首页、Discovery | `/` 与 `/discovery` 显示同一页面 | 两个入口未统一；筛选是局部状态，选中单集仅存 history.state，复制链接丢失 | `frontend/src/app/page.tsx`、`frontend/src/components/discovery/DiscoveryDesk.tsx` |
| 首页报告台 | 首页地址不变 | 当前报告、历史报告选择、工作流筛选及展开条目不可定位 | `frontend/src/components/discovery/WorkflowReportWorkbench.tsx` |
| Inbox / Focus Detail | 挂载时读取 `queue/episode/detail` | 打开、关闭详情不写 URL；没有配套历史恢复；只在指定队列已加载项中查找，未命中就提示不在原队列，且不接受 Done | `frontend/src/components/inbox/InboxPageClient.tsx` |
| 详情内容与助手 | 顶层仍为 Inbox 地址 | Show Notes、转写、笔记、助手开关均为局部状态 | `frontend/src/components/inbox/ConsumptionDetailPanel.tsx` |
| 加工产物 | 局部切换内容 | 产物页签、具体产物版本与片段不能独立恢复 | `frontend/src/components/inbox/EpisodeProcessingPanel.tsx`、`TranscriptAudioPlayer.tsx` |
| 人物与发言 | 转写内的工具面板 | 面板开关、人物证据、片段编辑位置未入 URL；editor 对象还混有未保存字段，不能整体序列化 | `frontend/src/components/inbox/TranscriptPeople.tsx`、`EpisodePersonEvidence.tsx` |
| 完成历史 | `/inbox/history` | 已提交搜索词为局部状态；现有返回行动队列链接没有独立资源兜底 | `frontend/src/components/inbox/CompletionHistoryPageClient.tsx` |
| 播客列表 | 支持 `sort_by`、重复 `tag_id` | 默认 replace 历史；导航语义与其他页不统一。无限滚动无需把每次加载写入 URL | `frontend/src/app/podcasts/PodcastsContent.tsx`、`frontend/src/hooks/useUrlState.ts` |
| 节目详情 | `/podcasts/:id?episode_id=…`，读取排序和标签参数 | 可定位单集卡片但不是单集工作台地址；`episode_id` 与 Inbox 的 `episode` 命名不统一 | `frontend/src/app/podcasts/[id]/page.tsx`、`frontend/src/lib/searchResultDisplay.ts` |
| 全局搜索 | 侧栏局部状态 | 搜索词、类型、搜索侧栏开关不能通过 URL 恢复 | `frontend/src/contexts/SearchContext.tsx`、`frontend/src/hooks/useSearchSidebar.ts` |
| 标签 | `/tags?podcast_id=…` | 排序及创建/编辑弹层是局部状态；批量选择无需持久链接 | `frontend/src/app/tags/page.tsx` |
| 工作流列表 | 写入 `sort_by` | 创建/编辑弹层无地址，排序使用独立 history 写法 | `frontend/src/app/workflows/page.tsx` |
| 工作流详情 | `/workflows/:id?tab=overview/jobs/config`，已有 popstate 处理 | 页签采用 replace；执行分页、展开 Job、报告弹窗仍是局部状态 | `frontend/src/app/workflows/[id]/page.tsx` |
| 导入与同步 | `/import` | 页签主要依赖局部状态及日志恢复，URL 不能表达当前操作区域 | `frontend/src/app/import/page.tsx` |

公共问题：`useUrlState`、页面自行写 history、`useSearchParams`、纯局部状态同时存在。公共 Hook 默认 replace、按单个参数更新、传入空 history state；应统一一次导航的原子更新及既有 history 元数据保留，并定向验证 Next.js 的实际集成行为。不能仅由这一写法断言路由器已损坏。

## 建议原则

1. **路径标识长期对象，查询参数标识查看方式。** 单集不隶属于某个行动队列；从 Focus 移到 Done 不应使内容链接失效。
2. **可重访的位置进入 URL，操作过程不进入。** URL 恢复已保存内容和阅读位置，不恢复未提交的人名、笔记、提示词、勾选结果或文件。
3. **视图与展示容器分离。** 同一个单集地址，站内进入可以保留现在的详情浮层，直接打开必须独立可用；不要求用户先打开 Inbox。
4. **地址不执行动作。** 打开人物面板只读已保存结果，不自动识别人名、运行模型、提交纠正、迁移队列、导入或重试加工。
5. **来源引用必须绑定版本。** 片段序号会随重新转写改变，单独的 `fragment=12` 不能作为永久证据地址。
6. **不强求复原每个像素。** 滚动、光标、悬停、抽屉尺寸、播放中的每一秒属于会话状态。浏览器后退应尽量恢复，但不是共享链接的承诺。

## 建议路由表

本表全部为待实施设计。尖括号代表运行时真实 ID，不按标题、姓名或数组位置生成地址。

| 目的 | 建议地址 |
| --- | --- |
| Discovery | `/discovery`；`/` 保留为跳转入口，保留兼容参数 |
| 发现台筛选、选中单集 | `/discovery?filter=<existing-filter>&episode=<id>` |
| 首页报告上下文 | `/discovery?report=<job-id>&report_history=1&workflow=<id>`；多工作流重复 `workflow` |
| 行动工作台 | `/inbox`；`queue=focus&episode=<id>` 仅表达看板定位，不作为单集身份 |
| 完成历史搜索 | `/inbox/history?q=<query>` |
| 单集详情 | `/episodes/<id>`，默认 Show Notes |
| 单集笔记 | `/episodes/<id>?tab=notes` |
| 转写与产物 | `/episodes/<id>?tab=transcript&artifact=transcript`；`artifact` 允许现有 summary/minutes/transcript |
| 人物与发言面板 | `/episodes/<id>?tab=transcript&artifact=transcript&panel=people` |
| 指定人物 | 上一地址追加 `person=<person-id>`；只定位人物，不声明其发言已获确认 |
| 发言/证据片段 | 上一地址追加 `source=<source-version>&fragment=<order>` |
| 明确跳到音频时刻 | 转写地址追加 `t=<seconds>`；不自动播放，不随播放进度持续写 URL |
| 单集助手 | `/episodes/<id>?tab=<underlying-tab>&assistant=1`；按需加 `profile=<id>&target_person=<id>` |
| 播客列表 | `/podcasts?sort_by=<value>&tag_id=<id>`，保留重复 tag_id 的现有协议 |
| 节目详情 | `/podcasts/<id>`；保留 `episode_id=<id>` 的卡片定位用途，打开深度处理使用单集地址 |
| 搜索 | `/search?q=<query>&type=all/podcasts/episodes`；实际枚举沿用现有搜索能力 |
| 标签 | `/tags?podcast_id=<id>&sort_by=<value>` |
| 标签创建/编辑 | `/tags?dialog=create` 或 `/tags?dialog=edit&tag=<id>` |
| 工作流列表 | `/workflows?sort_by=<value>` |
| 工作流创建/编辑 | `/workflows?dialog=create`、`/workflows/<id>?dialog=edit` |
| 工作流详情 | 保留 `/workflows/<id>?tab=overview/jobs/config` |
| 执行列表与指定执行 | `/workflows/<id>?tab=jobs&page=<n>&job=<job-id>`；指定 Job 独立加载，不依赖它恰好在该页 |
| 工作流报告 | `/workflows/<id>/reports/<job-id>`；不与单集加工运行混同 |
| 导入/同步 | `/import?tab=import/sync`；不恢复已选文件，不触发执行 |

人物面板与助手可同时打开，故使用 `panel=people` 和 `assistant=1` 两个正交状态；不以一个互斥 `panel` 参数强行改变现有并排能力。人物纠正的小编辑框、确认对话框及拖拽状态暂不独立路由，链接定位至人物或片段即可。

为当前例子推荐的最终地址：

`/episodes/66413?tab=transcript&artifact=transcript&panel=people`

现有代码以 `artifact-<artifactSetId>` 表达转写来源版本，因此引用地址可以复用 `source=artifact-<真实产物ID>&fragment=<真实片段序号>`，无需另造一套指纹。未查实具体产物/片段，不生成假证据链接。

## 状态与导航合同

### 恢复、返回与关闭

- 进入单集、切换实质内容页签、打开人物面板/助手/报告、提交搜索、切换结果分页或筛选：一次 push，后退撤销一次可辨识的导航。
- 输入过程、自动补齐默认参数、规范化旧地址：replace 或暂不写地址，不制造逐字历史。重复选择当前状态不新增历史。
- 多参数联动一次提交。例如打开人物面板，原子设置 tab、artifact、panel；切换单集时清除旧 person/source/fragment/t，避免短暂显示旧单集状态。
- 站内打开详情时，在 history 元数据保存可信来源列表地址、滚动和触发元素。直接访问时不依赖这些元数据也必须完成读取。
- 可选 `from=inbox/history/discovery/podcast/search` 仅决定无会话时“返回列表”的默认入口；不是资源 ID，也不改变读取和加工资格。复杂原筛选仅承诺同会话后退恢复，不虚构跨设备完整列表快照。
- 浏览器后退逐步恢复实际访问历史；前进能够重新打开相同对象与面板。
- 关闭按钮按层级关闭：编辑框 → 人物/助手面板 → 单集详情。若上一条历史恰是父状态才 back；否则 replace 为父状态。直接访问的单集没有已知站内来源时，“返回列表”进入 Inbox，不把用户带出站点。
- 同会话未保存修改采用明确保留或离开确认策略；URL 不能保存草稿。原生刷新/关闭的草稿提示与应用内导航分开验证。
- 程序主动切换和浏览器 popstate 都由同一解析后的 URL 状态驱动，不能留下“地址已变但面板不变”的第二套状态。

### 参数与内容安全

- ID 必须是合法正整数，枚举限定白名单；单值参数重复视为无效并规范化，多值标签去重并稳定排序。空搜索词省略；默认值不强制冗余写入。
- 列表字段保留既有 `sort_by/tag_id` 名称；只对明确的历史别名提供兼容，不为了美观同时改所有参数协议。
- `panel=people` 必须归一到转写内容；`person` 必须属于当前单集可见人物集合。`target_person` 另按助手现有可靠归属要求校验，不等于 panel 中的浏览选择。
- `fragment` 必须与 `source` 成对；旧版本可读取则打开旧版本，无法读取或片段不存在时明确显示“原引用不可定位”，保留请求地址，提供用户主动打开当前版本的入口。不得静默按新稿同序号定位。
- 请求的单集已移队列：仍打开同一单集，展示真实当前状态；已删除：明确不存在。未入队列不自动入队，非 Focus 不获得手动加工权限。
- 未生成转写或人物资料：保留目标位置并显示真实缺失状态，不自动触发生成。读取失败与不存在区分；重试留在目标 URL。
- 导航竞争时只接纳当前对象对应的响应，避免快速 A→B 导致 A 的人物或转写覆盖 B。
- 原始个人笔记、待提交姓名、凭据、模型问题和完整日志不进 URL。搜索词本身会留在浏览器历史，故 URL 只保存用户已提交查询。
- 内部链接生成相对路径；`127.0.0.1:18089` 只能用于当前设备。跨设备分享沿用现有正式域名及访问控制，URL 改造不新增公网服务、不绕过认证。

## 实施切片

### 最小必要方案

1. 从已核实的生产基线建立独立工作树，保护当前主目录的已有改动。先统一 URL 解析、构造及原子导航入口，复用 Next.js 现有机制，不新增状态管理依赖。
2. 优先完成 `/episodes/:id` 与现有详情组件的接入，包括 tab、artifact、people、assistant，以及按单集 ID 独立读取。单集读取不能靠遍历所有队列；核对已有 episode/consumption API 返回能力，只补实际缺失的数据读取能力。
3. 再完成 source/fragment 引用、报告和 Job、搜索与各列表/表单/导入页签的协议；没有实际稳定资源 ID 的临时状态不创造资源。
4. 最后统一全部入口：Discovery 卡片/Focus、完成历史、搜索结果、节目详情、报告条目、人物证据及助手来源链接。每个入口分别判断是“定位列表卡片”还是“打开单集工作台”。

### 兼容与附加项

- 旧 `/inbox?queue=…&episode=…&detail=1` 规范化至单集详情地址；旧 queue 只作为来源信息，不阻止读取。`detail` 缺省时保留原看板定位语义。
- 旧 `/podcasts/:id?episode_id=…` 继续定位卡片，不擅自改变为强制打开详情；站内新的“打开详情”操作改用新路由。
- 保留现有工作流 tab 链接及列表参数；只在新路由实际可用后更新链接，避免先发链接后补页面。
- 独立人物主页、持久聊天会话 URL、跨设备草稿同步、永久历史快照属于附加产品能力，不纳入本次 URL 体系的必要实现。
- 直达刷新展示完整详情；是否采用 Next.js 路由拦截来保留站内浮层，是实现选择，不应形成两套业务详情组件。

## 验收标准

实施后必须使用隔离数据和真实浏览器验证；这些是待执行标准，不是本次已通过结果。

| 场景 | 必须观察到的结果 |
| --- | --- |
| 当前例子 | Inbox 打开单集→转写→人物面板，地址逐步匹配；复制到新标签和硬刷新均恢复相同位置 |
| 前进/后退/关闭 | 按导航层次恢复；关闭和 Esc 行为一致；键盘焦点回到正确触发点；无历史时不意外离站 |
| 从所有内容入口打开 | Discovery、历史、搜索、节目、报告、助手引用定位同一 ID 和目标内容 |
| 换队列、列表未加载目标 | Focus→Done/Someday 后旧内容链接有效；目标在分页之外仍独立加载 |
| 发言引用 | 同来源版本定位准确；换稿、片段缺失明确提示，绝不指向不相关的新稿片段 |
| 草稿与写操作 | 直达只读取；不自动识别、执行、保存、移队列；导航未悄然丢弃未保存编辑 |
| 异常参数 | 无效/重复 ID、枚举、错单集人物、Job 不属于工作流均不会误定位或写入 |
| 慢/失败/首次访问 | URL 保持所请求对象；可辨识正在读取、缺失和失败，重试不丢目标；慢 A 的响应不覆盖 B |
| 列表/搜索/报告/导入 | 已提交筛选、搜索、分页、报告和页签刷新可恢复；分页游标/临时文件不被当成长久资源 |
| 窄屏/宽屏 | 同一地址内容一致，面板可达且焦点正确；助手/人物面板并排状态不丢失 |
| 访问路径 | 当前电脑主路径与已授权正式入口使用相同相对路由，认证边界保持不变 |

验证顺序：解析/构造及非法参数单测 → 组件双向同步与导航竞争测试 → 上表真实浏览器流程 → 相关静态检查和生产构建。生产发布及其验收另行授权。

## 决策状态

推荐采用上述“独立单集地址 + 视图参数 + 版本化引用”，保留现有浮层体验。当前需求足以给出方案，无需把源码可查的事实反问用户；新增独立人物页、跨设备草稿等未提出的能力不默认纳入。若要改变推荐的产品语义，再通过 grill-with-docs 定向讨论，确认后才将实质决策写入 ADR；不把本草案写进领域词汇表当作现状。
