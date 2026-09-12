# #349 全站 URL 实施记录

状态：专项本地实现与验收完成，现进入 Git 交付阶段；未关闭 Issue、未部署。此文件记录本地证据，不替代 GitHub Spec。

## 工作区与基线

- 工作树：`/Users/bytedance/.codex/worktrees/issue-349-url-system/MagicPodcast`。
- 分支：`codex/issue-349-url-system`；实施基线为 `730dc72f9b3e04000d403feb4031377f9faf6b8e`。
- 用户提供的 cf0b 工作树带有任务外修改且停在 c299f68，保持原样；本专项代码仅在上述新工作树。
- 已读取 #349–#354 的当前正文/评论/依赖，均 OPEN，无评论。依赖 350 → 351/352/353 → 354。发布与源代码状态不从 Issue 开关推断。

## 切片状态

| 票 | 本地状态 | 下一步 |
| --- | --- | --- |
| #350 | 已完成本地实现与本节验收；票未关闭 | 后续改动继续保护本票回归 |
| #351 | 已完成本地实现与下述验收；票未关闭 | 后续整体验收继续覆盖 |
| #352 | 已完成本地实现和分域验收；票未关闭 | 最终全站回归继续覆盖 |
| #353 | 已完成本地实现与分域验收；票未关闭 | 最终跨域回归继续覆盖 |
| #354 | 本地完成 | 后续提交/关闭票/生产发布须单独授权 |

#350 本地实现完成允许按用户目标推进依赖票；不代表 GitHub 的原生阻塞边已解除，也不授权关闭票。

## #350 实现与验证

- 新 `/episodes/:id` 复用原详情组件；Inbox 使用受 Next.js 支持的 native history 更新地址并保留原看板/浮层，硬刷新独立路由读取。
- URL 同步详情 tab 与产物 artifact；第一次解析非法枚举、重复单值时规范化，关联参数原子更新。未指定 artifact 时，在产物可读后将实际选项 replace 入地址，防止刷新改变视图。
- 统一基础导航保留框架 history 元数据，并允许 Next 自行补入其私有标志；直接携带 `__NA` 会绕过其同步流程，已对照当前 Next 文档和运行代码修正。
- 备注在同对象页签中保留；离开确认拒绝后，地址/内容/草稿不丢失。覆盖浏览器 popstate、关闭按钮、站内链接及原生刷新离开提示。
- 旧 `queue/episode/detail=1` 详情地址规范化；未指定 detail 的看板定位及节目旧 episode_id 卡片定位保持原语义。
- 真实 Fixture 暴露原 getItem 的缺口：只存在 episode、尚无 triage decision 时返回 404。后端已改为读取 episode，缺少 decision 时只投影空队列状态，不写入 decision 或完成事实，不改 schema。
- 独立页沿用 Inbox 的样式作用域，修复真实手机截图发现的透明背景；宽屏原浮层体验保留。

### 自动化与构建

- 前端 5 个相关测试文件合计 **129 passed**：navigation、InboxPageClient、ConsumptionDetailPanel、EpisodeProcessingPanel、EpisodeDetailPage。
- 随后独立页面样式作用域修复后，EpisodeDetailPage **4 passed**；恢复自动生成配置后 type-check 再次通过。
- 后端 `go test ./internal/handlers ./internal/services -run 'TestConsumption' -count=1` 通过；覆盖重复 GET 未入队单集不产生 triage/completion 或队列 revision 写入，删除后 404。
- 后端相关包 `go vet` 通过；前端改动文件 ESLint 通过。
- 使用独立构建目录执行生产构建通过，包含 `/episodes/[id]`；没有覆盖当前开发服务构建目录。
- `git diff --check` 通过。全站最终检查仍归 #354，不能以这些定向证据声称 #349 完成。

### 真实浏览器证据

使用 Playwright CLI 独立会话 `url349`，访问 `127.0.0.1:18350`；后台为本专项独立 Fixture 服务。

| 验收 | 观察 |
| --- | --- |
| Inbox 开详情 | 单集 2003 地址变为 `/episodes/2003?from=inbox`，看板仍在浮层后 |
| tab/历史/刷新 | 转写 tab 进入地址；后退回 Show Notes，前进回转写；硬刷新仍显示相同 tab |
| 新标签/窄屏 | `/episodes/2003?tab=notes` 在新标签直接打开笔记；390×844 可达，无内容透明问题 |
| 未保存关闭 | 编辑未保存备注，关闭触发确认；拒绝后 URL 与 textarea 文本均保留 |
| 未保存浏览器后退 | Inbox→详情→笔记→填写；后退至 Show Notes 保留草稿，再后退欲离开时确认；拒绝后恢复 `/episodes/2003?from=inbox`，详情与草稿均保留 |
| 无来源返回 | 直接打开详情，取消编辑后关闭，回到 Inbox |
| 旧队列链接 | 旧 `queue=focus&episode=2012&detail=1` 打开当前 Done 单集，显示 Done 内容 |
| 未入队单集 | 修复后实际 `/consumption/episodes/2013` 返回 success、queue=null；页面显示真实标题与“未收集” |
| 慢请求与写入 | 注入 1500ms 只读延迟，加载提示约 128ms、笔记可操作约 1913ms；目标地址未变，观察业务写请求为 []。为本次受控观察，不是生产性能结论 |
| 失败与重试 | 单集 GET 注入 503，页面说明读取失败且保留 notes 地址；解除注入后点击原重试按钮恢复内容 |
| 迟到响应 | 独立页面组件测试证明 A→B 后 A 迟到不覆盖 B |
| 产物视图 | 使用受控 processing/artifact API 响应，在真实页面直达逐字稿，点击纪要更新 artifact=minutes，刷新仍显示纪要。此证据验证浏览器 URL/展示集成，不证明真实加工或人物识别质量 |

截图：`output/playwright/url349-legacy-desktop.png`、`output/playwright/url349-unassigned-mobile.png`。
产物受控响应脚本：`output/playwright/url349-artifact-fixture.js`。Fixture 服务本身无真实飞书加工产物；不能把浏览器响应注入当作原生后端产物证据。

## 下一轮执行上下文

- 必须继续在本记录指定工作树，默认 cwd 可能仍是 cf0b；不要在旧工作树重做或覆盖已有修改。
- 开发前端由当前工具 session **80918** 启动，监听 **18350**；后端由 data-profile 管理，监听 **18349**。继续前重新检查进程/端口，不凭记录重启。
- Profile home 为 `/tmp/magicpodcast-349-profile`，当前场景 focus-7，schema 30；实例 `0216df5726ac6daf`，版本 `complete-v3-focus-7-20260912T16-schema-30`。先执行带该 home 的 status。
- Node PATH：`/Users/bytedance/.nvm/versions/node/v24.19.0/bin`；Go PATH：`/Users/bytedance/.homebrew/bin`。非 login shell 需显式加入 PATH。
- Playwright 会话中为单集 2003 的 processing-runs、运行 34901、产物 34902 配有 context.route 受控响应；需要原生后端证据时先移除这些本任务路由。不要误认运行源。
- Next 会生成 frontend 的 AGENTS/CLAUDE、next-env、tsconfig 及独立构建目录；均为本任务工具产物，最终交付前排除，不能把它们带入产品 diff。
- （过程记录）当时全部源码未 commit，#351–#354 与全站完整验收仍待完成；后续收口结果见“最终本地收口”。


## #351 本地验收（继续任务）

- 人物、助手、profile、target_person、source/fragment/t 已接入统一导航。独立浏览人物与助手提问目标分开校验；失效目标禁止提交，不替换成同名人物。无效 profile 明确提示。
- 新只读接口 `/api/v1/episodes/:id/artifact-sets/:artifactID` 按单集与版本共同读取，复用已有 capabilities 填充，不泄露 RootPath、不修改版本或队列。对应真实数据库/HTTP 集成测试覆盖匹配、错单集和只读结果。
- 显式来源版本独立读取；缺失/错单集/非法版本不回退到当前稿。旧版本可读时保留历史标识，禁止人物编辑。未指定版本的原有“新版慢/失败时继续阅读上一成功产物”行为保留。
- 时间戳和人物证据入口生成绑定版本的片段链接。音频进度在用户完成调整时进入地址，普通播放 tick 不写历史；直达只定位，不自动播放。
- 已知当前产物复用既有归属读取结果，避免点击同版本证据时卸载编辑器。StrictMode 双重 effect 曾将片段定位归零，已修复并加入对应测试。
- 面板互相独立关闭；手机 Esc 先关闭人物层，保留单集详情。此前从助手返回后的手机 Esc 会关闭整层，已修复并增加回归测试。

### #351 证据

- 8 个前端相关测试文件 **184 passed**；后续 Esc 新增定向测试 **1 passed**，播放器时间与 StrictMode 回归另行通过。TypeScript、改动模块 ESLint、生产构建均通过。
- `go test ./internal/handlers ./internal/processing ./internal/router` 全部通过；对应 go vet 通过。
- 真实浏览器：组合地址同时恢复人物甲、人物面板、助手、深度档位和目标人物；fragment=2 刷新后 marker=2、时间 00:30。
- 关闭人物层后 assistant=1 保留；关闭助手后 people 面板保留；后退重新打开人物层。手机 Esc 后 URL 保留 episodes/2003/source/fragment，peopleOpen=false、detailOpen=true。
- 人物证据按钮“查看原文片段 2”生成 source=artifact-34902&fragment=2，并实际高亮第二段。
- 历史 artifact-34903 读取不同的“URL 历史第二段”，marker=2，人物编辑入口不出现；artifact-99999 明确报原引用不可定位且不出现当前稿替代。
- `t=25` 直接访问显示 `00:25 / 02:00`，audio src=null。记录的业务 POST/PUT/PATCH/DELETE 请求数为 0。
- 桌面截图 `output/playwright/url351-combined-desktop.png`（初版定位归零截图，仅记录布局，不作为最终 fragment 定位证据）；手机截图 `output/playwright/url351-people-mobile.png`。最终定位结果以 browser eval 及 StrictMode 测试为准。
- 浏览器 processing/people/context/版本响应由 `output/playwright/url351-fixture.js` 控制，真实页面与 episode 读取仍来自隔离服务；证明 URL 用户流，不证明真实人物识别质量或真实音频播放。scoped metadata 的归属/能力校验另由真实 Go HTTP/数据库测试证明。

后端已通过本任务独立 data-profile 切换重建，当前实例见上文。下一步 #352；#353、#354 未实施。GitHub 全部票仍 OPEN，未改变原生依赖或提交生产操作。


## #352 当前验证补记

本地代码已接入工作流排序、创建/编辑、详情页签、分页、独立 Job 和报告地址。尚未宣称整票验收完成。

- 本轮重新读取 GitHub #349–#354 正文与评论；评论均为空。原生父子仍为 #350–#354，#354 阻塞项仍为 #351/#352/#353；不修改票状态。
- 当前分支确认为 `codex/issue-349-url-system`，前端 18350 的 node PID 33353 仍在监听，直接复用。
- 真实浏览器点击工作流 4001 的“编辑”，地址变为 `/workflows/4001?dialog=edit`；未修改表单按 Esc 回 `/workflows/4001`。
- 真实浏览器从列表点击“创建工作流”，地址变为 `/workflows?dialog=create`；空表单 Esc 回 `/workflows`，没有误报未保存。
- 本轮重跑 workflow 页面、ReportModal、WorkflowFormNavigation，共 5 文件 29 项通过；相关五个实现模块 ESLint 和 TypeScript 检查通过。
- 交接中的报告直达/刷新、跨工作流拒绝、页外 Job、窄屏和草稿保护证据仍待与最终覆盖表合并；本轮没有重复宣称它们已经重新验收。
- 下一步补齐 #352 排序/页签/分页历史、独立报告新标签及异常目标证据，再推进 #353；#354 全站验收仍未完成。


### #352 导航闭环与修复证据

- 排序 UI 选择 execution 后地址同步；浏览器后退为 updated，前进为 execution，选择值与地址同时恢复。
- 配置页签生成 tab=config；后退恢复 jobs 的 aria-selected=true；展开生成 job=5001 并显示“收起”。
- 报告按钮生成 `/workflows/4001/reports/5001`，新标签独立加载真实 Fixture 科技日报正文。
- 不存在 Job 99999999 显示“执行记录不存在”；4002/5001 组合明确拒绝所属关系；重复 tab 与非法 page 被 replace 清理，unrelated=keep 保留。该组请求业务写入为 0。
- Fixture 只有一页，本轮仅覆写分页总数为 2，保留真实 API 返回正文。点击下一页、后退、前进分别显示第 2/1/2 页及一致 URL；随后移除此受控响应。真实页外 Job 验证另见交接证据。
- 报告 503 受控失败保留目标地址，解除失败后普通重试恢复真实正文。1200ms 受控延迟下，可见弹层约 883ms、真实正文约 2182ms，等待时有关闭入口；不是生产性能结论。
- 修复详情编辑关闭新增 history 的问题，改为父地址关闭；真实浏览器 Esc 回 config，前进重开 edit，证明关闭使用回退而不是重复 push。
- 手机报告底栏原被移动导航遮挡，提升报告层到既有表单相同层级；截图 `output/playwright/url352-report-mobile.png` 已检查，正文和底部按钮完整可达。
- 修复重试按钮卸载后焦点落到 body，恢复到报告关闭按钮；真实 503→重试→正文→Esc 回 jobs、dialog=0，新增焦点回归测试通过。
- 本轮工作流页面和表单 22 项通过，ReportModal 8 项通过；最终类型检查与相关 ESLint 通过，diff check 通过。此前生产构建证据继续保留，#354 对最终集成版本重跑构建。

#352 分域验收已完成，继续 #353；（过程记录）当时全部 Issue 保持 OPEN，未提交推送或生产操作。


## #353 实施起点：导入页签

- `/import?tab=import/sync` 接入共同导航；URL 负责当前操作页签，日志仍按原模式显示，不把旧日志模式覆盖到已指定页签。重复或未知 tab 清理为默认，保留无关参数。
- 导入页测试 17 项通过，类型检查、相关 ESLint 通过；增加已指定页签不受历史日志覆盖、导航不启动操作、重复参数规范化断言。旧测试补充 history 隔离以免上一条测试的 URL 泄漏。
- 390px 真实浏览器：sync 直达、点击 import 更新地址、后退恢复 sync；硬刷新在 hydration 后恢复 sync，未选文件。观察到业务写请求为 0。
- 初始服务端渲染仍先显示默认 import，hydration 后恢复 sync；本轮首次立即读取 aria-selected 曾为 false，等实际恢复后为 true。最终首屏审计需处理或验证该短暂默认态，不能将立即恢复视为已通过。
- #353 其余发现、报告台、搜索、历史、播客、标签尚未实施，#354 未开始。当前浏览器 tab2 在 `/import?tab=sync`，viewport 390×844；tab3 为独立报告。


### #353 完成历史、导入首屏与根别名

- 完成历史查询以已提交 q 为准，输入过程不改 URL；提交/清空用共同导航，后退重新读取对应查询并恢复输入。失败重试使用已提交 q，避免输入中的未提交文字悄然变成查询；新查询使旧请求失效并清除旧分页游标，仍保留原有效记录。
- 完成历史 7 项测试通过，类型检查通过，相关 lint 已消除本轮引入的 ref cleanup warning。真实 Fixture 手机路径：q=Fixture 返回 Done 已完成单集；输入“ 不存在349 ”期间 URL 不变，提交后无匹配，后退及硬刷新恢复 Fixture 和原单集（实际输入没有两端空格）。
- 导入拆成服务端入口和 ImportPageClient，服务端 initialTab 直接来自 searchParams，客户端仍以共同导航负责交互。解决先显示 import 再恢复 sync 的短暂错误态。
- 导入 17 项测试继续通过；真实浏览器 reload 后立即读取 sync 的 aria-selected=true。只读 HTTP HTML 也包含 import=false、sync=true。禁用 JS 的额外浏览器实验因流式内容未可见超时，未作为通过证据；普通浏览器路径已通过。
- `/` 改为服务端 redirect 到 `/discovery`，保留 query 和重复 workflow；原首页预加载测试改为直接验证 discovery 页面，新增根别名参数保留测试，共 3 项通过。浏览器 `/?filter=unread&workflow=4001&workflow=4002` 实际到达对应 discovery 地址；仅证明别名参数保留，发现页内部尚未接入该状态。
- 尚余 #353：Discovery filter/episode、报告台 report/history/workflow、独立搜索与侧栏、播客多标签排序、标签表单及参数；#354 全站入口、完整覆盖审计及最终构建仍待完成。


### #353 Discovery 筛选与预读接入

- DiscoveryDesk 的 filter/episode 改为共同 URL 导航驱动，移除仅存在于 history.state 的选中单集账本。默认/非法 filter 规范化；关闭预读移除 episode，保留筛选。
- 指定单集不在当前候选列表时用既有详情 API 读取，目标 ID 匹配后展示；迟到请求忽略，失败保留目标并提供重试。直达恢复不调用原 UI 的 onRead，普通点击预读继续沿用既有标记已读行为。
- 预读备注加入 URL 离开保护；编辑对象切换、收起和 Esc 在有未保存内容时询问。导航拒绝时不继续 onRead 或关闭编辑器。该新增草稿保护尚需专门浏览器验收，不能把前序单集工作台草稿证据代用。
- DiscoveryDesk 46 项通过（新增列表外直达且不标记已读），DiscoveryDesk+PageClient 上一轮合计 55 项通过；测试清理补齐 URL 隔离，原历史状态测试改为显式 episode 地址。TypeScript 和三个实现模块 lint 通过。
- 390px 真实 Fixture `/discovery?filter=unread&episode=2003` 显示正确 Focus 混合格式 Show Notes；刷新仍同一标题；Esc 返回 filter=unread；点击未收集生成 filter=uncollected，后退回 unread。刷新与关闭期间记录业务写请求为 0。
- 下一步优先报告台：确认 HomepageReport.id 是报告 ID，URL 合同 report 是 job_id，两者不能混用。已有 `/api/v1/jobs/:id/report` 返回报告 ID，可复用后再用既有 discovery report detail 查询，不应凭候选列表推断不存在。
- 报告台、搜索、播客与标签仍未接入；#353 未完成，#354 未开始。浏览器 tab2 目前在 discovery?filter=unread，手机尺寸。


### #353 首页报告台 URL 接入

- report 绑定 job_id，report_history=1 控制往期抽屉，重复 workflow 保存筛选，去重排序并规范化非法值；输入中的筛选关键词仍为临时状态。
- 报告选择从 URL 推导。已在候选中直接复用；窗口外复用 `/jobs/:jobID/report` 取得报告 ID，再调用既有 discovery published report 查询，并核对返回 job_id。没有新增后端接口或报告生成逻辑。
- 加载失败保留选中报告，404 明确“不存在或尚未发布”，不会退回另一报告。旧测试的失败回退、刷新清空筛选、候选窗口变化自动删筛选与新 Spec 冲突，已改为保留目标和恢复 URL 的断言；保留返回之前今日报告的同会话行为。
- 当 report 明确指定，手机默认展开正文且展开按钮 aria 状态一致，用户仍可收起。往期抽屉独立关闭，保留 report/workflow。
- Workbench + DiscoveryPageClient 62 项通过（含窗口外 Job 读取、缺失不替换、筛选刷新恢复）；类型检查及相关 lint 通过。最终整站构建仍归 #354。
- 真实 Fixture：report=5003&report_history=1&workflow=4003 展示 Fixture 往期周报、完整正文、已选1；刷新相同，Esc 仅去掉 report_history。该组业务写请求为0。
- 受控隐藏 discovery 报告候选（today/history 都为空），仍由真实后端成功取得 report=5003 的完整周报；该证据验证不依赖候选窗口，不宣称改变了数据库数据。
- report=99999999 实际明确报不存在/尚未发布，目标地址保留，报告区域没有默认报告替代。
- 普通 UI 点击“查看更早一份报告”生成 report=5001，点击往期追加 report_history=1；刷新抽屉仍开。
- 剩余 #353：搜索独立页/侧栏、播客排序多标签、标签 URL 与草稿；Discovery 元数据草稿专门浏览器验收、多工作流组合 UI 路径仍需补齐。#354 未开始。


### #353 搜索独立页与侧栏

- 新 `/search` 复用 SearchSidebar 的查询/结果组件，独立页全宽显示；站内搜索按钮用 native history 保留背景页面，关闭/Esc 回来源。独立页关闭用 Next router 返回 discovery。
- q 仅在点击搜索、Enter 或选择历史查询后写入，输入时仍保留原有实时只读搜索体验；type 由 URL 驱动，直接加载恢复正确范围。非法/重复参数只规范化搜索自有字段。
- 修复 Next router 转场不触发原 navigation 订阅的问题：共同 hook 监听 pathname 完成后的事件，SearchProvider 的开关直接读取框架 pathname。初版关闭独立页曾留下侧栏，真实页面暴露后已修复并复验 dialog=0。
- 真实 Fixture 手机路径：Fixture/episodes 直达只显示单集结果；输入 Done 期间地址仍 Fixture，提交后 q=Done 且显示 Done 已完成单集；后退、硬刷新恢复 Fixture。独立关闭回 discovery 无残留侧栏；站内搜索打开 /search，Esc 返回 discovery。
- 搜索单集结果改为统一 `/episodes/:id?from=search`，保留节目卡片旧定位用途的改造留在原节目入口；浏览器 Done 结果实际链接 `/episodes/2012?from=search`。精确搜索来源返回与跨页完整闭环仍须 #354 继续验收。
- 搜索/Hook/display 30 项通过；共同导航、搜索、导航栏、独立详情 16 项通过。公共订阅引入的单集测试 mock 缺少 usePathname 已补齐，非产品失败。类型与相关 lint 通过。
- 手机截图 `output/playwright/url353-search-mobile.png` 已检查。下一步 #353 播客排序多标签、标签页地址与草稿，以及本票剩余专门验收；#354 尚未开始。


### #353 播客排序多标签与标签表单

- useUrlState 改为读取共同导航快照，移除独立 state/popstate/history 写入账本。功能型更新读取当前 URL，保留框架元数据；共同 navigate 的 replace 不再多写一次初始化历史。
- 播客 sort_by 继续原枚举，tag_id 继续重复参数；用户筛选 push，非法/重复/乱序标签 replace 规范化。渲染只读取合法正整数标签，排序白名单防止无效值进入列表 API。
- 播客与公共导航 18 项通过。真实手机 title 排序下选择科技+产品生成 tag_id=3001&tag_id=3002，后退只保留3001，前进和硬刷新两个按钮均 aria-pressed=true。
- 标签 sort_by、podcast_id、dialog=create/edit、tag 接入共同导航。编辑按单独 tagApi.get 读取，不依赖列表；非法目标/读取失败有明确提示与重试。关闭保留排序和节目范围。
- TagFormModal 增加草稿离开保护、Esc/焦点循环/恢复；初始值使用字段依赖，避免相同对象导航时因父组件新对象引用清空输入。成功保存后解除 guard，不二次确认。
- 标签既有9文件44项通过，新增直达编辑与草稿拒绝后，受影响2文件14项通过；类型检查与相关 lint 最终通过。曾有新测试使用不支持的 exact 参数，已修正为匹配角色名称，非产品行为变更。
- 真实 Fixture 手机 `/tags?dialog=edit&tag=3001&sort_by=alphabetical` 恢复 Fixture 科技；填写“URL 未保存标签”后关闭并拒绝，读回原 URL、原输入、dialog=1。随后将输入恢复原值，Esc 回排序列表；普通新建生成 dialog=create，刷新仍为空创建表单，无自动提交。
- 本轮 dialog 工具出现“无 modal”的返回，因此拒绝成功以随后页面输入/地址读回为证据，不以该工具状态为证据。
- #353 主要路由已有实现，但仍需补齐该票剩余证据：标签节目范围/错误与浏览器历史、播客非法参数/旧节目卡片定位、搜索类型/宽屏/无写、Discovery备注草稿与多workflow UI；共同导航更改后的跨域回归。#354 仍未开始，不能将主要实现齐备视为专项完成。


### #353 分域收口与 #354 起点

- 跨页定向回归18文件241项中240项通过；唯一失败为 DiscoveryInboxToggle 测试未清理上条 filter URL。补齐测试隔离后该文件4项通过，业务回滚路径未改。随后发现页新增收起编辑草稿测试，DiscoveryDesk47项通过。
- 真实标签节目范围1001显示深度科技与正确标签；排序热度→后退字母一致；编辑产品标签3002后关闭仍保留节目范围和排序。标签不存在99999999明确提示；播客重复/非法排序和乱序重复标签最终规范化为 keep=yes&tag_id=3001&tag_id=3002，两个标签仍选中，该组业务写请求0。
- 旧 `/podcasts/1002?episode_id=2012` 实测定位 Done 卡片（y约308、height约229），dialog=0，保留卡片定位含义。
- Discovery“收起编辑”漏确认被真实验收发现，已修复；拒绝后读回 episode=2003 与“发现页未保存349”保持。随后清除本任务未保存输入退出，没有保存。
- 多工作流组合用受控报告候选（把真实 today 元数据加入 history，不写库）验证：UI勾选科技与周报生成 workflow=4001&workflow=4003，刷新已选2且结果正确；响应注入已移除。
- 桌面搜索切节目生成 type=podcasts，实际三份 Fixture 节目标题可见，刷新仍选中。发现200ms等待期间出现短暂“无结果”，已改为输入更新同步进入搜索中，重复相同输入不重开loading；搜索Hook+侧栏18项通过，最后重复输入防护仍须最终回归覆盖。
- 独立目录生产构建通过（含 /search、/episodes/[id]、/workflows/[id]/reports/[jobId]、动态/import）；没有覆盖当前dev目录或启动生产。TypeScript与相关lint通过。
- #353 可进入依赖票的本地收口标准已满足；票不关闭，最终#354需继续复核所有路由双向证据和完整用户故事。
- #354 明确待补：Focus仍生成旧详情URL；Discovery/历史/节目/报告需有统一单集工作台入口；搜索结果已为新URL但当前_blank，精确来源回返不应只靠from默认值。EpisodeDetailPage目前关闭仅映射固定列表，需补可信同会话来源的history元数据、返回焦点和滚动；最终全量自动检查、浏览器跨域闭环与完成覆盖表尚未执行。


## 最终本地收口

- 完成 #349 的24行路由与36条用户故事覆盖，详见同目录 `URL_SYSTEM_ACCEPTANCE_2026-09-12.md`。前面“下一步/待验收”段落为过程记录，以本节和顶部状态表为当前结论。
- 新 EpisodeLink 统一跨页单集入口并保存可信来源历史元数据；搜索、历史、节目、Discovery预读、Focus、报告实际进入正确对象，关闭恢复原查询、触发焦点。节目窗口scrollY=298.5、搜索容器scrollTop=250分别恢复。Inbox打开后硬刷新为独立页，再关闭恢复 queue=focus&episode=2003 和原按钮焦点。
- 请求竞争用真实页面的“下一项”验证：阻塞2004详情，切至2005后释放2004，最终地址和标题仍为2005。首次桌面脚本因移动按钮不可见超时，改用390px正常移动端入口后通过；没有将超时写成通过。
- 助手受控SSE回答经普通来源按钮导航到2003/artifact-34903/fragment2，显示历史第二段与“当前段落”，打开来源业务写请求0。SSE为本地响应注入，不执行真实模型；随后移除该请求注入。
- 最终组合URL在1440px同时恢复人物甲、助手深度档位/目标、34902片段2；新标签及硬刷新仍为当前第二段，记录业务写请求0。截图 `output/playwright/url354-combined-desktop.png` 已检查。
- 首屏审阅补齐 Discovery/播客 initialHref，播客服务端按URL排序/标签预读；HTTP HTML的tag3001仅包含深度科技，不先显示其他节目。对应SSR及路由测试79项通过；无效资源采用共享纯参数解析，不猜测其他对象。
- 最终前端 `npm run test:run`：141文件 **995 passed**；`npm run lint`、`npm run type-check`、独立目录生产构建均通过。日志分别位于 `/tmp/url354-frontend-final.txt`、`/tmp/url354-lint-final.txt`、`/tmp/url354-type-final.txt`、`/tmp/url354-build-final.txt`。
- 后端 `go test ./...` 与 `go vet ./...` 全部通过；后端检查后未再变动。日志 `/tmp/url354-go-all.txt`、`/tmp/url354-go-vet.txt`。
- 生成的Next配置恢复为本任务干净基线；编译目录及生成AGENTS/CLAUDE已移到仓库外保留，没有删除数据/日志，也没有启停既有服务。本次最终生成物备份路径：/var/folders/bm/j238_8pd20s0gd8x9f2jxdk40000gn/T/magicpodcast-349-final-generated-7sxbxbpd。
- 验收阶段工作树为 issue-349-url-system，分支 codex/issue-349-url-system，HEAD 为实施基线 730dc72；随后进入 Git 交付。未关闭 GitHub 票、未部署、未迁移、未写入生产数据。截图与浏览器受控 Fixture 脚本保留在本任务 output 目录，属于验收证据，不作为生产代码提交。
- 已有真实数据/服务保持不动；人物质量、真实模型执行、真实音频播放和生产发布不属于本地URL验收结论。生产部署另需授权。
