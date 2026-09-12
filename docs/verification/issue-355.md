# #355 人物弹层与 SSE 验收

日期：2026-09-12。范围：[Spec #355](https://github.com/rookiestar/MagicPodcast/issues/355)，依赖顺序 #356 → #357 → #358。实现为 `d7b3ca4`、`55df85f`、`3a61467`、`a360f42`，基于 `730dc72`；最终审查修复以 [PR #359](https://github.com/rookiestar/MagicPodcast/pull/359) 的合并版本为准。

设计依据：[已确认原型](https://github.com/rookiestar/MagicPodcast/blob/261538b87a736fbb6b3e1082f13382978bb787e1/docs/research/transcript-people-modal-sse/prototype.png)。管理改为居中弹层；重新识别、保存、明确应用位于固定底部；进度期间仅展示真实阶段，已应用内容仍留在逐字稿。没有新增 schema、持久任务平台或扩大人物识别标准。

## 验收环境与证据边界

- 独立工作树：`codex/issue-355-people-modal-sse`；原工作树、生产端口及真实数据库均未修改。
- 一次性 SQLite、独立 ArtifactStore，使用现有 `TestPersonaRealBrowser` 启动普通 API。接口 `127.0.0.1:18355`；开发页面 `13355`，构建页面 `13356`，最终构建页面 `13357/inbox`。后两个页面使用 `next build`/`next start`，没有开发工具覆盖层。
- [验收语料](issue-355/corpus.json)是专门编写的合成访谈，含两位具名说话人和一位未知 Speaker。人物识别及 Copilot 回答实际调用已安装的 Runtime，没有回放预制模型答案。此证据证明运行链路，不证明真实播客的人名识别准确率。
- 断线、读回失败使用浏览器网络拦截；丢失完成事件的场景先让真实后台完成保存，再剔除完成事件。音频回归使用浏览器实际 HTMLAudioElement 播放 90 秒静音 WAV，媒体响应为受控 Fixture；没有下载或修改真实音频。
- 手机证据是 Chromium 的 390px 可视区；390×500 用于模拟软键盘占用空间。已验证字段、滚动和操作按钮可达，未宣称经过实体手机输入法测试。

## AC 对照

| AC | 可观察结果与证据 |
| --- | --- |
| AC1 | Copilot 同开时，1133px 下正文宽度开关前后均为 649.78125px；1440px 下均为 852.40625px。没有第三阅读栏。[测量](issue-355/layout.json)。 |
| AC2 | 实际检查 1133×1354、1440×1000、390×844，以及 390×500 键盘占位；多人物、长姓名、证据展开均可滚动。手机弹层 clientWidth/scrollWidth 均 372px，底部按钮区域下沿 831px，小于 844px 视高。[手机](issue-355/mobile.png)、[编辑](issue-355/mobile-editor.png)。 |
| AC3 | 已应用事实与新建议分开；Speaker 3 显示姓名待确认和填写入口。模型返回的未绑定候选不会自动成为正式人物，应用前 `people`/attributions 不发布。重新识别保留人工姓名“林言老师”。[桌面](issue-355/desktop.png)。 |
| AC4 | 姓名/角色表单与详细证据按需展开；点击“查看原文片段 1”收起弹层并显示真实片段，重开仍保留展开状态。范围测试取消一段后显示 1 段，提交 `orders:[1]`；排除项不计入影响数量。[证据展开](issue-355/evidence.png)，`TranscriptPeople.test.tsx` 的 selected fragments 用例。 |
| AC5 | 普通 UI 修改姓名→保存草稿→关闭重开→刷新后重新打开单集，仍为“林言老师”；未保存编辑关闭重开保留。旧 revision 保存失败后，读取按钮不会覆盖本地修改。数据库关闭重开由 `TestReviewSurvivesDatabaseCloseAndReopen` 验证。 |
| AC6 | 识别/保存时原 Speaker 和人物问答资格不变；明确应用后 8 段更新为两位人物。随后实际 @周宁提问，引用其第 4 段。未知 Speaker 手动填写→确认 1 段→双击编辑→解除匹配→恢复 Speaker 3 全部通过。 |
| AC7 | 四个真实阶段先于完成；完成后可读到已持久化 draft 4。HTTP 测试在数据库创建草稿处注入失败，只得到 error，没有 complete；Runtime 失败也不泄露内部错误。见时序及 `TestPersonPreparationStreamsBeforePersistenceAndReturnsSavedDraft`。 |
| AC8 | 真实识别持续约 33.6 秒，已有逐字稿及已应用结果保留，阶段与耗时可见；10 秒心跳未推动阶段。收起重开仍是同一次请求；可取消。[进度](issue-355/progress.png)。 |
| AC9 | 首次打开入口不调用模型，由“开始识别”启动；空草稿明确提示并保留手动入口。自动化用例 `keeps manual entry available when identification saves an empty draft` 验证此路径。 |
| AC10 | 提交前受控断线不会重复 POST；真实后台保存后剔除完成事件，页面读回新草稿并显示待确认，正式归属保留。读回 503 时禁用重试识别，恢复读取后解禁。[丢失完成事件恢复](issue-355/completion-recovered.png)、[读回失败](issue-355/readback-failed.png)。 |
| AC11 | 浏览器取消请求约 223ms 后后端处理结束；JSON/SSE 两种真实 HTTP POST 都验证 context 取消。`TestCancellationDuringSpeechReviewCancelsRuntimeAndDoesNotPublish` 验证取消当前 Runtime；HTTP `cancel_after_commit` 用例在草稿提交后才取消，断线不撤销已保存草稿。 |
| AC12 | 同页重复点击仅一次请求；离开单集 abort 并忽略迟到结果；API 丢弃外单集、外请求、外来源事件。当前服务端草稿与正在阅读的来源不同时，编辑/应用禁用、影响数为 0。旧 revision、多页面并发、运行中换源分别由 draft/lifecycle 测试验证，未提交编辑不被读回覆盖。 |
| AC13 | 收起与重开不会再启动模型；审阅滚动位置恢复；离开取消，刷新只恢复持久状态。没有宣称跨刷新后台执行。浏览器与 `shows real phases…`、`cancels on leaving…` 用例交叉验证。 |
| AC14 | 真浏览器 Tab 末项回首项、Esc 关闭顶层、焦点回触发入口，外层单集及 Copilot 仍在。嵌套手动编辑 Esc 返回管理弹层；触屏可见入口可用。耗时 `aria-live=off`，阶段独立 status，减弱动画关闭 spinner 动画。播放时开关弹层没有暂停或回退播放位置。 |
| AC15 | 最终构建页经真实前端代理：点击首个 UI 反馈 2.8ms，首个服务端事件 22ms，核对阶段 22001.8ms，保存阶段 33619ms，完成 33622ms，草稿可见 33627.1ms。各事件属于同一 request id，保存前即有阶段与心跳。[完整时序](issue-355/runtime-timing.json)。 |
| AC16 | 阅读/播放器、片段定位、历史草稿、人工决定、源版本与 Copilot 测试通过。真实 @周宁回答“不赞成无限制加班”，引用单集 7355 / artifact-1 / 片段 4，未引用主持人或未知 Speaker。[回答与来源](issue-355/copilot-source.png)。#351 核对时仍 OPEN，主线未落地其 URL 导航，不宣称已验收该专项。 |

## 检查命令与结果

在仓库根目录执行，Frontend 命令使用 `npm --prefix frontend`：

- `go -C backend test ./...`：通过。
- `go -C backend vet ./...`：通过。
- `go -C backend test -race ./internal/handlers ./internal/personidentity`：通过；新增取消/提交竞争后定向重跑 `-run TestPersonPreparation -count=1`，通过。
- `npm --prefix frontend run test:run`：137 文件、971 用例通过；随后补充空结果、范围/冲突与证据展开恢复三项测试；相关人物/播放器/Copilot 的 57 项全部通过（其中人物核对 15 项）。远端 CI 对最终提交重新执行完整套件。
- `npm --prefix frontend run type-check`、`npm --prefix frontend run lint`：通过。
- `BACKEND_URL=http://127.0.0.1:18355 MAGICPODCAST_NEXT_DIST_DIR=.next-issue355-final npm --prefix frontend run build`：通过；上述最终视觉与时序来自对应构建后的 `next start`。
- `git diff --check`：通过。构建自动生成的类型配置和 Agent 入口未纳入提交。

最终审查发现定位原文后重开弹层会丢失原生 details 的展开状态，已由 `3a61467` 修复：收起只隐藏弹层，保留原生审阅状态。真实浏览器复验“展开证据→定位片段→重开”通过，[复验截图](issue-355/reopened-evidence.png)；类型检查、Lint、57 项相关测试和 `.next-issue355-reviewed` 构建均通过。另由 `a360f42` 补齐成功读回后刷新识别历史，避免新草稿未进入历史选择框；随后按 PR 审查补齐“选中旧历史记录后识别失败”的用例：读回比较以请求前最新服务端草稿为基线，而不是正在查看的历史记录，避免误报新结果。人物核对 17 项、类型检查和 Lint 通过。前述真实 SSE 时序对应 `55df85f` 的传输实现，该补丁未修改请求、解析或持久化流程。

本地 HTTP/播放器的故障测试与浏览器故障注入是异常场景证据；不以它们代替上面的真实 Runtime、持久草稿和实际人物问答。CI/合并结果由对应 PR 的当前状态确认。

## 复验入口

使用新的临时目录，避免复用或覆盖任何现有数据库：

```sh
PERSONA_BROWSER_DIR=/tmp/magicpodcast-355-new-run \
PERSONA_REAL_CORPUS="$PWD/docs/verification/issue-355/corpus.json" \
PERSONA_BROWSER_ADDRESS=127.0.0.1:18355 \
go -C backend test ./internal/handlers -run '^TestPersonaRealBrowser$' -count=1 -timeout=120m -v
```

该命令需要本机已有可用 Runtime。前端设置 `BACKEND_URL=http://127.0.0.1:18355`，使用独立端口。普通 UI 路径：Inbox → 单集明细 → 转写 → 逐字稿 → 人物入口。完成识别、编辑、保存、刷新、应用后，从单集助手 @人物提问。关闭测试服务按 harness 输出创建该临时目录中的 `stop` 文件。

本次未部署生产、未迁移、未修改真实人物结果。测试数据库、Runtime 日志、构建目录均不进入 Git。
