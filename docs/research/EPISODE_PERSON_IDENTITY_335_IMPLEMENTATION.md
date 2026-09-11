# #335 人物识别修复实施记录

状态：集成实现与主要浏览器验证已完成，#339 最终质量审查仍在进行，尚未证明专项全部验收满足。下文为历史轮次；最新状态见末尾“最终审计接续”，旧进程与旧待办不代表当前现场。

## 范围与基线

- 父合同 #335；依赖顺序 #336 → #337 → #338 → #339。
- 新隔离工作树：issue-335-identity-repair；分支 codex/issue-335-identity-repair；基线 origin/main 为 6aaeed3。
- 用户指定的其他工作树和已有脏文件未修改。
- 本次 Goal 授权实施与隔离真实 Runtime/浏览器验收；不包含提交、推送、合并、关票、代理委派或生产迁移、回填、部署、重启。

## 已实施但尚未全面验收

- Prepare 不再合入规则候选，也不在无 Runtime 时用正则确认人物。
- 服务端加载节目名称、作者、简介和单集标题，加入实际 Runtime 的只读输入，不加载私有备注。
- 名字、出场、角色、转写称呼与发言绑定分别给证据；校验来源/原文/说话人标签。转写误写只作为本集 SourceNames，不直接成为全局别名。
- 空角色依据保留 unknown；既有规则 unknown 不再遮蔽 Runtime 角色。
- 增加点名邀请后的回应证据核对：回应不用重述姓名，但必须有相邻点名和对应说话人绑定。
- 既有合成持久化/HTTP 测试现在显式提供受控识别决定；这些测试不是模型质量证明。SeedBaseline 在无 Runtime 时只装载明确合成标注，不恢复生产回退。
- 新增的证据记录目前借助原有 evidence_locator 保存结构化依据；后续需在 #337/#338 统一有效事实与面向用户的证据展示。

## 已取得证据

- 三个新增业务反例在修改前均失败：七类伪候选、主持角色丢失、节目上下文缺失。日志 /tmp/persona335-red.log。
- 修改后人物模块与 HTTP 测试通过；最新定向日志 /tmp/persona335-targeted-v2.log。
- 第一轮后端全量 go test ./... 与 go vet ./... 通过；日志 /tmp/persona335-go-first.log、/tmp/persona335-vet-first.log。此后增加点名回应校验和测试，已跑受影响包；不能把先前全量结果称为后续所有版本的验证。
- 目标集从原始导出重新调用实际 Runtime，约 83 秒得到张小珺（host）、曾鸣（guest），没有七个伪候选；保留 21 个待归属片段。此数量不代表全量归属准确率。初轮材料 /tmp/persona335-real-initial/。
- 随后从运行服务只读获取当前 Focus 与完成历史，11 个不同单集中 8 个有当前成功转写；这是这两个入口的盘点范围，不是全库所有历史成功产物的最终证明。
- 重新下载这 8 集当前 Show Notes、转写和节目公开元数据，保存在 /tmp/persona335-evidence/sources.json；inventory.json 记录盘点。真实材料不提交 Git。
- 第一轮扩大真实回归在执行，日志 /tmp/persona335-evidence/real-round1.log、产物 real-round1/。该轮运行的是补充点名回应校验之前编译的代码，因此不能用来证明该新增校验最终质量。
- 已发现并修正初轮 Tibo 被过度降为 pending 的原因：回应“谢谢邀请”未重述名字，但上句有点名邀请。后续必须从原始来源重新执行受影响真实样本，不用 replay 代替。
- 未运行最终浏览器验收，未处理真实数据库旧候选，未发布。

## 下一步（保持原目标）

1. 继续等待/检查当前真实测试，不因观察超时重启。当前执行命令为不带 identities replay 的 TestPersonaRealCorpus；检查日志和实际进程/会话确认状态。
2. 对扩大样本逐集人工核对人物名单、角色和说话人绑定，保留留出样本，补全 #336 的证据与反例。重新跑点名回应变化涉及的真实样本；若识别再次改动，再定向复测。
3. #336 验收满足后，按 #337 实现完整候选替换、算法版本与来源有效性、人工确认保护、角色/参与排除后端合同、索引一致性及定向修复预览。生产旧数据尚不会自动变正确。
4. 按 #338 实现证据/纠正/排除/撤销、选中人物失效、四态和迟到响应。Goal 已要求这些场景；如确有额外状态改变，完成可独立工作后集中说明具体授权需求。
5. 按 #339 对最终准备链重新跑真实模型和浏览器，逐条映射父 Spec。不得以初轮成功、缓存识别产物或仅有候选数量提前宣布完成。

## 第二轮 Goal 推进

- 第一轮 8 集实际执行已正常结束（500 秒），并逐项检查了名字、角色和归属；“测试 PASS”只证明执行完成，不等于质量通过。
- 发现并修复三种过度弃用：出场证据使用已核对短名而不是全名；被点名后直接回应未重复姓名；同一自我介绍片段的姓名出现在 binding 证据而未出现在 presence 的短节选中。仍拒绝不相符说话人锚点，不能将主持人的介绍句当作嘉宾说出。
- 发现 66074 三位人物正确但模型只枚举 6 个示例片段。新增明确的 binding scope：模型核对全文后可声明 stable_speaker，后端展开该标签并应用 excluded_orders；混标签只能 listed_fragments。不能隐式把任何单段确认扩大成整组。
- 当前证据校验新增稳定标签展开/例外片段、短名身份、点名回应等回归；人物和 HTTP 包通过，日志 /tmp/persona335-alias-scope-tests.log。后续又补同片段自我介绍证据，人物包通过。
- 第二次全体真实运行 real-scope-round 已结束（341.59 秒）。该轮编译于短名和同片段补丁之前，仍有可解释的 pending/遗漏，不算最终 #336 验收。
- 第三次全体真实运行正在进行：/tmp/persona335-evidence/real-candidate-final.log 与 real-candidate-final/。实际执行句柄为 exec session 93972（跨 turn 先核实该句柄，丢失时查日志及进程，不盲目重启）。不带 identities replay，从当前下载的原始语料执行。
- 66074 的稳定标签扩展和 66224 的多角色/额外发言人仍需逐片段抽查，不能只看归属总数。
- #336 尚未完成，#337 尚未实施。下一步应先读取第三轮结果及相应证据，再决定是否满足前置验收；不可绕过已发现质量问题直接宣称 #336 完成。


## 当前收尾进展：准备请求并发保护

- #337 已加入同源自动候选完整替换、旧自动事实隔离、人工确认保护、元数据失效及 Schema 29 准备记录；共享检索保留一般原文搜索并拒绝失效人物归属。完整 #337 仍未验收。
- 现有单集准备记录复用 revision：Runtime 执行前事务预留请求版本，发布只接受仍匹配的版本；人工确认同步递增版本。新请求进行中或失败不使此前有效结果失效。
- 确定性交错回归覆盖后发准备先完成、模型期间人工改名、Runtime 失败、取消；验证迟到结果拒绝且其人物写入回滚，先前有效结果保留。没有增加 schema 字段或第二套并发账本。
- 验证：人物、共享检索、HTTP 三包通过；日志 /tmp/persona335-revision-fence.log。git diff --check 通过。这不是最终 Runtime 或浏览器质量验收。
- 下一步：补齐旧索引写入版本保护与读取快照一致性；实现角色/参与状态独立纠正、片段残留清理、保守跨集身份匹配及定向重建预览，再进入 #338/#339。后续最终版本须重新执行真实 Runtime。
- 前轮 real-candidate-final 与 real-callname-check 已结束；不要依据上文历史 session 重新启动或等待。未执行生产写入或 Git 提交。

## 索引并发与同源片段替换

- ListEpisodePeople 在一个数据库读事务内读取人物、归属和准备版本，避免拼接不同提交的事实。
- 人物驱动的共享索引更新携带同一准备记录的 revision / published_revision；ReplaceEpisode 在写事务内比较二者，拒绝迟到文档。必须比较发布版本：请求执行中读取旧事实时，request revision 已递增，新结果发布时它不会再次变化。普通无人物索引调用不要求准备记录。
- 定向测试真实连接人物服务、SQLite 和共享检索，覆盖上述同请求版本的迟到索引、人工拒绝归属后的迟到索引；验证旧写拒绝、新命中保留、一般原文仍可查。HTTP 将索引过期映射为冲突，与来源变更一致。
- 同一来源版本重建时清除已不在输入中的派生片段，保留独立人工确认历史；测试验证已删除片段不再出现在列表或一般搜索中。
- 当前人物、共享检索、HTTP 包全部通过；日志 /tmp/persona335-index-fence.log；git diff --check 通过。
- 仍需完成：索引失败时完整发布一致性验收、角色/参与状态独立纠正、保守跨集身份匹配、定向重建预览；随后 #338/#339。未执行最终真实 Runtime/浏览器验收，未提交或发布。

## 姓名与角色确认解耦

- 查证 CorrectName 过去把 appearance.Role 写入 person_name 确认，随后 upsertAppearance 又把它视作人工角色覆盖模型结果；这会让未知/错误自动角色在改名后永久固定。
- 已取消姓名确认中的角色写入，以及对历史姓名确认所夹带角色的自动采用。姓名与人工归属确认仍保留；单独的人工角色纠正功能尚待实现。
- 新回归从 unknown 角色开始，确认姓名后模拟历史记录夹带 unknown，再由新识别得出 host；验证姓名 ID 保持、角色可更新、没有连带确认发言。
- 人物及 HTTP 包通过，日志 /tmp/persona335-name-role.log；git diff --check 通过。
- 跨集匹配仍待修复：findExistingPerson 使用 identityCompatible 的描述词重叠可能把同称呼/泛化职业误合并。不能用全禁跨集关联替代既定历史检索需求，需基于可核对身份依据采取保守匹配并补正反例。

## 独立角色纠正与人物排除后端

- 在尚未发布的 Schema 29 中新增单集/人物唯一的 person_appearance_overrides，独立保存可空角色和排除决定。既有姓名/片段确认 kind 约束不变，不改已发布 Schema 27/28。该表解决角色选择不能冒充姓名确认、排除不能删除可撤销事实的具体缺口，没有引入通用审核机制。
- 新增现有人物处理器下的 appearance-corrections POST：role 和 excluded 分别可选，空操作、非法角色、错误类型及多余字段拒绝。纠正递增既有准备 revision，阻止旧 Runtime/索引覆盖。
- 人物列表分别返回可用 people 与 excluded_people，role_user_confirmed 表示独立人工角色；排除名单用于后续撤销 UI。角色纠正不会确认 pending 身份或发言。
- 重新准备保留有人工出场决定的人物关系，自动候选再出现时继续应用排除。排除立即在共享检索 SQL 及可靠发言接口生效；一般内容仍可检索，撤销后依据仍有效的归属恢复。
- Snapshot 清除新表且有实际私有记录回归；新增表已在迁移 29 精确声明。迁移保护、Schema24 升级/幂等及 DataProfile 包通过，日志 /tmp/persona335-override-migration.log；人物/检索/HTTP/router 包通过，日志 /tmp/persona335-override-api.log。
- 已核对新 schema fingerprint 为 1d505a4b1d8bda564fd7ac72f16142b48a249ddcc6631299319687ed2a72d9d1；所有验证在临时数据库，未迁移共享 Fixture 或生产。
- 当前仍未完成 #337：跨集身份匹配、索引写失败的一致性验收、定向重建工具及最终领域/迁移说明；#338 UI 和 #339 最终真实验收尚待执行。先前真实 Runtime 结果不能证明本轮持久化修改的最终质量。

## 人物事实与索引原子发布

- 原链路先提交人物再写索引，索引失败时 API 报错但事实已改变。现在共享检索可绑定调用方既有 SQLite 事务；Prepare、姓名/片段/出场纠正在同一事务内发布事实及索引，失败一起回滚。未引入补偿队列或第二套索引。
- 新增真实 SQLite BEFORE INSERT 故障注入，覆盖四种操作：准备、改名、归属拒绝、人物排除。逐项验证 API 服务返回错误、人物与片段保持原状、没有新确认/排除/幽灵人物、旧有效索引仍可查，解除故障后重试成功。
- 人物/检索/HTTP/router 包通过，日志 /tmp/persona335-atomic-index.log。全后端 go test ./... 通过，日志 /tmp/persona335-go-integrated.log；go vet ./... 通过，日志 /tmp/persona335-vet-integrated.log；git diff --check 通过。
- 这补齐索引写失败的原子性证据，仍不代表 #337/#335 完成。剩余优先项为跨集身份匹配与定向修复工具，然后文档合同核对、#338 UI、#339 最终原始语料真实 Runtime 与浏览器验收。

## 跨集身份匹配改为明确依据

- 删除用身份描述包含/词重叠判定跨集同人的路径。当前同集记录仍可重用；跨集须规范姓名一致、两次准备都提供相同有辨识度身份依据，旧方须当前来源且未被人工排除。多个匹配不自动选择。
- Runtime 结构增加 name_type 与 identity_anchor。模型负责判断具体机构身份或公开主页是否有辨识度；服务端验证原文、姓名与 key 同一引用关系。本集称呼、pending 身份或不合法引文清除跨集依据，但保留有效本集人物识别。
- 对照测试覆盖：同称呼/泛化职业分开、规范姓名与具体身份相同可合并、不同机构身份分开、本集称呼即使带相同机构也不合并、同集重建幂等。原王芳跨集角色互换回归仍通过。Fixture adapter/SeedBaseline 仅供预标注合成决定，不是模型质量证据。
- 原文核对反例覆盖伪造来源、缺少姓名关系、key 不在引用内、本集称呼不作为全局依据；无效跨集证据不丢弃有效本集出场者。
- 人物/HTTP/检索包及对应 vet 通过，日志 /tmp/persona335-cross-identity.log。身份依据仍依赖模型语义判断，不能宣称确定性规则保证没有误并；最终真实语料验收必须核对此项。
- 本轮新增匹配字段后，目标集 66314 从原始来源重新调用实际 Runtime，37.7 秒正常完成；名单为张小珺/host、曾鸣/guest，无伪候选。实际材料 /tmp/persona335-evidence/identity-anchor-check/real-66314.json；日志 identity-anchor-check.log。核对名单、身份锚点及首/中/尾归属样本，不把 574 段总数当成全量准确率；这仍不是 #339 全体语料与浏览器最终验收。

## 定向重建工具

- 增加 PreviewRebuild 只读盘点和 RebuildSelected 限定清单执行；命令入口 backend/cmd/maint/rebuild_episode_people。默认只读，兼容旧版缺少 preparation/override 表的预览，不自动迁移。
- 输出旧候选、当前产物、姓名/片段/角色/排除人工记录数量；写模式要求明确清单、确认串、备份及 Runtime/产物路径。写入复用 PrepareCurrent，每集 150 秒，与交互路径一致；逐集报告差异，失败不写成功，剩余已选集继续，可单独重试。
- CLI 测试比对预览前后数据库文件 SHA-256 完全相同；空/重复/非法/不存在清单与缺少写授权参数被拒绝。业务测试使用实际磁盘产物读取器、临时 SQLite 和受控 Runtime，验证成功/失败分别报告、未选集不写入、重试 ID 稳定。日志 /tmp/persona335-rebuild-tests.log。
- 备份完整性检查不代替目标匹配、停写窗口和恢复演练；生产运行未授权且未执行。实际全链真实 Runtime 命令验收仍待最终集成阶段。
- 重新核对 #337 发现还须修复：当前 CorrectName 修改全局 Person，已跨集关联时可能连带改变其他集姓名。应按本集纠正合同隔离影响，补双集回归后才能宣布 #337 完成。

## 本集纠正隔离与真实旧 Schema 升级回归

- 修改共享人物姓名/说明/别名时，若其他集仍引用该人物，只在当前集分离身份；其人工姓名/发言/角色/排除记录随同当前集关系保持，其他集保留原 ID 和资料。未共享的人物改名保持 ID。重复改名和重建不重新合回旧身份。
- 本集已核对 SourceNames 参与旧候选匹配，但不写全局 aliases；有来源依据且未有冲突人工姓名确认时自动正名。若旧人物跨集共享，按同样本集隔离规则处理。
- 新 Schema28 升级测试按实际注册迁移建立旧库，写入观察到的九个候选及代表性人工记录，随后 ApplyMigrations 升级而不预清数据。升级前预览九人，升级后未重建只使用人工确认身份；受控决定重建后恰为张小珺/曾鸣，七伪候选退出，旧主持 ID 保留、误写不成为全局别名、转写原文不变，重复重建和迁移幂等。此测试证明升级/持久化合同，不冒充真实模型质量。
- 人物/HTTP/检索包通过，日志 /tmp/persona335-scoped-upgrade.log；git diff --check 通过。迁移指南已更新 Schema29 与回退/定向重建边界。

### 给 #338 的后端合同

- GET `/api/v1/episodes/:id/people`：`people` 为可用/真实待确认人物，`excluded_people` 为可撤销排除名单；人物字段新增 `role_user_confirmed`。`evidence_locator` 内为结构化原始依据，UI 应解码为字段/片段/引用，不直接展示 JSON。
- POST `/api/v1/episodes/:id/people/:personId/appearance-corrections`：请求 `role`（host/guest/unknown）与 `excluded`（布尔）可分别提供，至少一个；响应同人物列表。false 撤销排除，不构造无依据身份或发言确认。
- 姓名与片段纠正入口保留。共享身份纠正可能返回本集新 ID，UI 必须核对当前选择而不能静默把问题转给另一个人。
- 来源/准备/索引冲突返回 HTTP409；失败不替代旧有效事实。`index_ready=false` 目前尚不足以区分首次准备/旧版待识别，#338 接入时需补明确准备状态并复用既有事实账本，不重新推断或展示旧伪候选。
- 本轮最后后端全量 go test ./... 与 go vet ./... 均通过，日志 /tmp/persona335-backend-stage337.log、/tmp/persona335-vet-stage337.log；没有运行中的测试句柄。下一步逐条复核 #337 证据后进入 #338，前端入口为 frontend/src/components/inbox/EpisodeCopilotPanel.tsx 与 episodeCopilotMention.ts。

## #337 阶段复核与 #338 初步接入

- #337 既有验收已分别由旧 Schema28 九候选升级、同源替换/人工保留/原始换版、元数据失效、请求/索引并发、SQLite 索引故障回滚、独立纠正 HTTP、限定清单 CLI 及全后端测试覆盖，进入 #338；未关票。最终 #339 仍须对集成版本重新跑真实 Runtime、重建命令与浏览器。
- 后端人物列表与 Copilot context 增 preparation_state（required/outdated/ready/no_transcript），沿用准备记录区分首次与旧版，转发 excluded_people 和 role_user_confirmed。旧自动角色未经复核或人工确认时不作为可靠角色显示。
- 前端接入独立角色保存、本集排除及撤销；未知角色显示“角色待确认”，未确认身份明确区分。排除或改名导致选中 ID 消失时保留问题、禁止自动提交，要求重新选人或明确转普通问答。
- 结构化识别依据解码为姓名、出场、角色、转写称呼与发言原文；引用支持定位原始转写片段，不展示技术 JSON。保留原 @列表与纠正面板，没有新人物管理页面。
- 取消/新请求/切集沿现有 generation 失效迟到响应；请求完成时核对当前选择而非发起时旧闭包。慢请求和失败保留问题、有效内容与选择；原有普通问答、输入法、档位、备注授权、来源与取消测试继续通过。
- 已验证：前端 type-check、变更文件 eslint、EpisodeCopilotPanel 22 项测试通过，日志 /tmp/persona335-ui-type.log、/tmp/persona335-ui-lint.log、/tmp/persona335-ui-tests.log；人物/episodecopilot/HTTP 后端包通过，日志 /tmp/persona335-ui-backend.log。依赖已在本隔离 worktree npm ci 安装，无现存测试会话。
- #338 仍未完成：真实浏览器（桌面/移动/键盘/输入法/四态耗时/原文跳转）、实际 API 持久化交互及全前端 lint/test/build 待做。#339 全体最终真实质量与交付审计待做。18089 继续只读；需建立真正隔离 API/前端端口，不能对该生产转发入口写入。

## 真实浏览器发现的问题与当前现场

- 全前端 134 文件/942 测试、全量 lint、初次 build 通过；新代理相关修改后已重跑定向测试并重新 build，通过。最终全量仍应覆盖此后补丁。
- 新 opt-in TestPersonaRealBrowser 从授权 sources.json 构建独立磁盘库和真实转写产物，普通 GORM context loader、人物 API、真实 Runtime 与生产前端 build 共用正常路径。没有读取身份缓存或预填正确姓名；目标集先保存九个旧候选。
- 真实页面发现两层缺口：Next 通用 rewrite 约30秒断开长请求；后端 Prepare 不读取 POST 正文，使 HTTP/1 断连迟迟不能取消 request context。新增具体 prepare Route Handler，API rewrite 作为 fallback，其他图片/健康路由仍保留；后端有界读取正文后执行既有150秒准备预算。
- 真实 HTTP 取消回归：移除正文读取时测试失败（/tmp/persona335-http-cancel-red.log），恢复后通过（/tmp/persona335-http-cancel-test.log）。代理慢请求/冲突/取消测试5项通过（/tmp/persona335-proxy-tests.log）。浏览器 v4 的取消请求已终止，数据库 revision=1/published_revision=0，未发布人物；输入保留。不能用此前 v2/v3 的仅前端取消结果证明成功。
- 隔离 CLI 对全部8集从原始磁盘转写执行成功：/tmp/persona335-cli.db，备份 /tmp/persona335-cli-backup.db，逐集结果 /tmp/persona335-cli-real-results.jsonl。进程 session55675 已正常结束。尚未完整核对这一轮语义质量，且下面补丁后须重跑受影响样本，不能当作最终质量通过。
- 浏览器 v4 完整准备28.99秒后页面正常收到结果；张小珺为主播，但曾鸣被降为待确认，未满足目标验收。证据 /tmp/persona335-browser-v4/target-people.json。短称呼“曾教授”的模型引用只摘了同片段的“曾鸣教授”，使该称呼未通过原文校验，后续点名回应无法核对。
- 已补最小来源扩展：只有引文有效且短称呼确实出现在同一定位片段时，用该实际完整片段保存称呼证据；不存在的称呼仍拒绝，不跨片段猜称呼，不升级为全局别名。通用陈教授反例通过；人物/HTTP包通过 /tmp/persona335-shortform-regression.log。该补丁尚未加载到现有浏览器API，也未重跑真实 Runtime，不能宣布目标集已通过。

### 下轮直接接续

- 实施目录仍为 issue-335-identity-repair；所有改动未提交。
- 活着的独立 API：exec session28197，端口18145，数据 /tmp/persona335-browser-v4；日志 /tmp/persona335-browser-api-v4.log。它运行短称呼扩展前代码。正常停止：创建 /tmp/persona335-browser-v4/stop，再等待原 session 结束；不要重启共享服务。随后用新的 PERSONA_BROWSER_DIR 启动同一 TestPersonaRealBrowser，不能复用/覆盖已有目录。
- 活着的前端：exec session94413，端口18045，日志 /tmp/persona335-browser-frontend-v4.log；已加载包含 Route Handler 与 fallback rewrite 的 build。最近变更仅后端证据校验，前端无需因此重建。
- CUA 持久绑定 browser（in-app id1）和 tab（id2），URL http://127.0.0.1:18045/inbox?queue=focus&episode=66314&detail=1；已标 handoff。当前 @列表显示张小珺/主播、曾鸣/嘉宾但曾鸣待确认。后续先刷新最新后端，再真实识别，不能手工确认目标姓名来过验收。
- 下一步：新后端重跑目标集并核对；实际纠正/排除/撤销、来源跳转、普通与人物问答、移动端和输入法/键盘、四态耗时仍需真实浏览器验证。CLI/全体样本质量审查及留出测试、最终全部门禁与交付审计仍未完成。Goal保持 active，生产18089只读。

## 最终审计接续

更新于 2026-09-11。代码未提交，生产未变更，全部 Issue 仍 OPEN。

### 最新验证

- 最后一轮全后端 `go test ./...` 与 `go vet ./...` 均以 exit 0 结束，包含最后产物删除保护。日志：`/tmp/persona335-audit-final-go.log`、`/tmp/persona335-audit-final-vet.log`。
- 全前端946测试、lint、构建与恢复生成文件后的 type-check，证据分别为 `/tmp/persona335-delivery-ui-tests.log`、`/tmp/persona335-delivery-ui-lint.log`、`/tmp/persona335-ui-build-sse.log`、`/tmp/persona335-restored-typecheck.log`。结果需与最终代码范围配对，后端单独改动不要求重跑无关前端检查。
- 人工改名不再把旧写法自动加入全局 aliases；旧写法只保存在本集人工确认来源。真实浏览器完成张小珺→临时验收名→张小珺，ID始终为8，最终aliases为空。已读回证据 `/tmp/persona335-browser-name-correction.log`。
- 最后一个转写产物删除后，准备发布、当前归属、检索命中与coverage不得复活旧artifact来源。SQLite高层测试覆盖准备中删除及人工确认历史保留；合成来源改用fixture前缀。定向证据 `/tmp/persona335-deleted-source.log`，随后全后端通过。
- 浏览器v7从原始资料自动得到张小珺/host、曾鸣/guest，无七个伪候选，38.57秒；发生于人工姓名纠正之前。证据 `/tmp/persona335-browser-v7/automatic-people.json`。此运行尚不含随后最后产物删除补丁，正常产物的识别输入未因该补丁变化。
- 桌面与390×844移动页面已验证角色保存、排除/撤销、选择失效与问题保留、原文跳转。人物实际回答约17.77秒，明确AI模拟并引用本集；普通实际回答约117秒并保留来源。证据 `/tmp/persona335-playwright-answer.log`、`/tmp/persona335-normal-answer.log`、`/tmp/persona335-answer-source-result.log`。
- Chromium原生composition + Enter未提交请求；此证据不覆盖操作系统输入法候选窗。记录 `/tmp/persona335-native-ime-result.log`。
- 手机纠正面板限高并保留提问框；prepare和questions具体代理保留取消与SSE流，解决通用代理30秒断开。

### 质量审查范围与未完成项

- 盘点覆盖Focus7集与完成历史4集，共11个不同单集，其中9集有成功转写；不是全库历史总量证明。来源记录 `/tmp/persona335-evidence/final-inventory.json`、`sources-final-nine.json`。
- 九集最后完整执行轮次 `/tmp/persona335-evidence/continuous-quote-final/` 仍错误归属66224片段100，不能标整轮质量通过。随后 `/tmp/persona335-evidence/mixed-fragment-corrected/real-66224.json` 将100保留pending，105/106归陈皮，角色待确认。
- 旧摘录 `final-quality-review.json` 含52个首中尾样本，100须用修复结果替换；不能直接声称52/52或51/51正确。复读发现66125片段36含“是的”、94及66074末段含多次告别/致谢；仅凭这些文字不能证明多人，也不能无核对地认定单人。列为审查边界，未列为已证实错误。
- 正用最终代码重新执行66314、66224、66125原始来源，以及预标注两个合成留出。结果目录 `/tmp/persona335-evidence/final-audit-runtime/`、`final-audit-holdout/`。执行成功与语义通过必须分别核对。
- 仍须完成逐人依据与召回分母、已知冲突及抽样归属审查，单列未审查发言；最终验收记录、完整diff审阅和生产启用可审阅方案待收口。Goal保持active。

### 最终代码定向真实重跑结果

- 上述两组执行均已exit 0结束：三集105.745秒、两个合成留出75.546秒；不再有对应运行中的测试句柄。
- 66314：34.51秒，最终名单恰为张小珺/host/confirmed、曾鸣/guest/confirmed；开场片段1归张小珺、片段2归曾鸣。没有预填人物姓名。
- 66224：33.49秒，曲凯/host、逯雨鑫/guest、陈皮/unknown；已知混合片段100为pending且无person_id，105/106归陈皮。
- 66125：36.84秒，薛铁鏻/host、蓝华峰Frank/guest；片段1和94保留pending，36仍归蓝华峰Frank。36中的短应答是否他人仍待证据判断，不能宣称所有混合发言已解决。
- 两个预标注合成留出逐项比对通过：人物4/4，已有确定标注归属7/7，两个不确定片段仍pending且无person_id，机构/被提及者与伪短语未进入名单。这只说明这两个合成样本，不代替真实语料准确率。
- 比对记录 `/tmp/persona335-evidence/final-audit-comparison.json`；当前仍需完整质量分母、身份锚点与发言审查、最终diff复核、生产方案。未标记Goal完成。

### 日期输入与Schema完整性审计补丁

- 复读父Spec发现日期已参与失效但未传入Runtime。扩展已有来源结构为服务端读取的EpisodePublishedDate，缺失为空；客户端值不作为权威。增强原公共上下文测试先失败、补丁后人物/HTTP/检索/问答四包通过，证据 `/tmp/persona335-published-date-red.log`、`/tmp/persona335-published-date-green.log`。
- 新增人工纠正表未列入既有必需表清单，删除该表后检查误认为Schema完整。复现测试 `/tmp/persona335-overrides-schema-red.log`；在既有requiredTables补全该表，没有新建检查框架。全后端go test与vet以exit0完成：`/tmp/persona335-dated-all-go.log`、`/tmp/persona335-dated-all-vet.log`。
- 因识别输入变化，九集重新调用真实Runtime（`dated-final-runtime/`）；此前结果不作为这版最终语义结果。两个预标注合成留出再次通过，4个人物、7个确定片段及2个不确定片段均与标注相符，日志 `/tmp/persona335-dated-final-holdout.log`。
- 旧隔离API v7已通过stop文件正常退出；新v8加载本轮最终代码，仍使用18145，数据库 `/tmp/persona335-browser-v8/browser.db`。生产未操作。
- v8真实浏览器从原始磁盘产物重新准备目标集，39.041秒、HTTP200，张小珺/host/confirmed、曾鸣/guest/confirmed，输入保留。随后真实@列表仅这两人，截图 `output/playwright/persona335-v8-final-people.png` 已目视检查。证据 `/tmp/persona335-v8-browser-prepare.log`、`/tmp/persona335-v8-visible-list.log`。
- 已新增最终验收与生产启用记录 `EPISODE_PERSON_IDENTITY_335_ACCEPTANCE.md`，当前明确标为未收口，未声称质量全过。

### 当前明确缺口（下一轮入口）

九集日期输入轮次已结束：66078首轮150秒超时，单独重试56.29秒成功。没有对应运行中测试。最终后验质量记录为 `/tmp/persona335-evidence/dated-quality-review.json`，48段抽样47段支持、1段未定；额外3段可归属内容漏绑定，quality_pass=false。具体是66125片段36的短应答整段归属疑点，以及19/20/48对自己具名资料库的叙述未归薛铁鏻。详见新的最终验收记录，不将这些缺口藏进总体成功率。

独立API当前v8，session66912，18145，停止文件 `/tmp/persona335-browser-v8/stop`；前端仍session7176，18045。所有代码未提交，生产未变更。下一步聚焦上述两个语义边界，保留实际Runtime、保守归属与覆盖要求；修复后定向重跑并更新指纹，随后收口完整AC映射。

### 第一人称身份引用与保守片段边界

- 已按真实缺口补first_person_identity证据类别：说话人明确描述自己的具名作品/资料库并有姓名原文关系，可作为身份锚点；只提及、阅读或转述别人不成立。服务端要求引用含已核对本集姓名且Speaker匹配，仍沿用原证据、排除片段与原子发布机制，不增加模型调用或预算。
- 受控测试先红后绿，人物/HTTP/问答/检索测试通过：`/tmp/persona335-firstperson-red.log`、`/tmp/persona335-firstperson-green.log`。后续全后端test/vet通过，日志firstperson-all-go/vet。
- 最终真实66125重跑37.03秒：19/20/48归薛铁鏻，36与94不归属。新增提前标注合成留出910003，人物乔宁/Mira Wu，5个确定发言正确，混合5待确认；引用朋友路然的话未制造人物。实际材料firstperson-targets/、firstperson-holdout/。
- 原两个合成留出再次通过，原标注4个人物/7个片段及2个不确定片段一致。九集复测由目标2集和其余7集组成，所有执行已结束。
- 复测发现66078两位明确列出的主持人被错误降pending：模型使用无法对应人的转写句作为出场依据。补充提示要求明确本集名单直接作为出场依据；无法绑定Speaker不能降级已证实出场身份。针对66078、66224与目标66314正在定向重跑，目录roster-final/；待读结果，不以旧缓存替代。
- 当前隔离API已到v10（18145，stop文件 `/tmp/persona335-browser-v10/stop`），旧v9已正常停止。前端18045未改。最终浏览器准备/提问和当前CLI从真实磁盘产物的限定重建仍在进行。生产18089仅只读GET。

### 分组复核已接入，尚待完整实际验收

- 前述增加调用说明不等同新增授权门槛：父Spec要求说明预算/影响，当前Goal已授权持续修复与真实Runtime；接口、模型与150秒总deadline不变，已向用户说明最多4次调用后在该范围内推进。没有启动subagent或生产操作。
- 新增内部speech_review阶段，身份建议产生后对每个唯一候选归属显式复核。小集1组，大集最多3组并行；分组保留相邻上下文，不重复发布上下文，未归属内容不被删除。复核只可保留或否定建议，不能创造新人物或绕过身份依据。
- 输出必须覆盖本组每个待核对片段且无重复/越界，mixed/uncertain不作人物观点。任一组失败或取消整体不发布，沿用既有事务和人工事实保护。新增覆盖校验针对已观察到的批量Speaker展开漏审，而非新建通用门禁。
- 新识别版本为identity-evidence-v3，防止先前单次识别v2的自动结果被当作已通过分组复核；Schema仍29。
- 高层业务测试覆盖混合片段可普通搜索但不归人物、复核遗漏不发布；251片段最多4次调用、上下文不误发布；取消真正传递Runtime并且不发布。定向与race已通过：`/tmp/persona335-speech-review-tests.log`、`/tmp/persona335-speech-review-race.log`。
- 第一组实际66125完成68.44秒，19/20/48归薛铁鏻，36/94待确认；目标66314仍在同一测试运行，目录reviewed-targets/。这不代表完整质量验收。

### 分组复核结果与同音候选修复

- 分组复核目标66314实际96.58秒完成；在初次身份建议之外，第二阶段另排除了178/224/350/392四段（存在短回应/话轮交替）。66125的36保持待确认、19/20/48归薛铁鏻。其余7集全部完成467.03秒，单集26.77–95.09秒；没有扩大150秒预算。
- 对此前514/541的怀疑重新核对了513–515、540–542完整邻接语境：仅出现“对”不能证明换人；两段均可由主持人连续总结/提问，现有标签和语义无明确矛盾。将其记为文本层支持的归属，不声称经过音频核验，不用关键词黑名单强制排除。
- 固定先前52段样本、额外2段具名自述、2段上述语境核对和6段明确待确认边界，形成62段固定审查表：49个可归属片段均正确归属，13个不确定/混合片段均未确认。记录 `/tmp/persona335-evidence/reviewed-fixed-frame.json`，仅限文本层固定样本，非全量音频准确率。
- 新发现并复现不同规范名共享转写误名的合并错误：李景/李璟均转写为李景时，第二候选误复用本轮第一人。现在每轮不同候选不复用已占用的人物ID；已有规范姓名优先于他人的转写误名。重复准备ID稳定。红绿证据 `/tmp/persona335-homophones-red.log`、`/tmp/persona335-homophones-green.log`。
- 新提前标注合成留出910004已真实执行：冯宁、李景、李璟三人独立、别名为空，5个发言分别正确归属；没有把误写升级为全局别名。32.61秒完成，目录homophones-holdout/。
- 同音修复后的全后端test与vet以exit0结束：`/tmp/persona335-final-source-go.log`、`/tmp/persona335-final-source-vet.log`。新复核并发与取消race通过。最终源码已停止扩展，剩余为最新浏览器、证据汇总与授权边界收口。
- v3 CLI在独立旧v2结果副本上预览needs_preparation=true，按指定66314调用同一原始产物链重建成功，日志 `/tmp/persona335-v3-cli-results.jsonl`。它运行同音补丁前代码，后者无该集候选冲突，不作为同音案例证据。
- 当前独立API为v12（session55587、18145，stop文件 `/tmp/persona335-browser-v12/stop`），已加载同音补丁与v3分组复核；前端仍18045/session7176。v11已正常停止。

### 最终问答暴露并修复枚举误分类

- v12真实准备87.27秒后名单正确，但实际@曾鸣问原话回答“无法判断”。读回发现曾鸣发言数为0：模型把片段28的被点名后生平回应标为first_person_identity，片段本身不含姓名，严格姓名锚点校验丢掉了整个绑定。
- 新反例先失败，补丁仅在已有点名回应合同独立成立时规范化证据类别：当前Speaker与锚点一致、前句为另一Speaker且含已核对本集姓名/称呼；无点名的普通生平叙述仍不通过。保留原引文，不靠某人/某集硬编码，不直接凭职业认人。四包测试通过：`/tmp/persona335-named-response-red.log`、`/tmp/persona335-named-response-green.log`。
- v12准备和失败回答证据保留，不将其当完整问答通过：`/tmp/persona335-v12-browser-prepare.log`、`/tmp/persona335-v12-answer.log`。此前代码指纹f912已过时，待最新收口刷新。
- 当前API v13（session51454、18145，stop文件 `/tmp/persona335-browser-v13/stop`）加载上述修复，v12已正常停止；准备/实际问答/引用将在该版本重跑。

### 角色证据保留与冲突处理补齐

- 合同复核发现同源重建时unknown会覆盖既有可靠角色。新增反例先红后绿：同源、同算法且依据仍存在时保留已知角色；不同的有效自动角色不任意选一方，保存两份依据并保持角色待确认，直到人工纠正或来源变化。身份确认不受角色冲突影响。
- 依据必须仍存在于当前Show Notes或相同定位的原文片段；即便同一source_version下片段内容变化，也不能继承已经消失的角色证据。对应红例 `/tmp/persona335-role-quote-red.log`。
- 复用准备记录的来源/算法/元数据有效性，不新增表。自动冲突写入既有evidence_locator的role_conflicts；人工角色仍由独立覆盖记录优先。现有证据面板显示两份不同角色依据，没有新增管理页面。
- 最新后端全量test/vet已exit0：`/tmp/persona335-delivery-final-go.log`、`/tmp/persona335-delivery-final-vet.log`。前端136文件947测试、lint、build与恢复生成配置后的type-check均通过：role-final-ui-tests/lint/build/type日志。
- 旧前端7176已按确认过的工作目录和端口正常停止，最新前端session47966仍为18045；API目前v14/session4349、18145，stop文件 `/tmp/persona335-browser-v14/stop`。v13已正常停止。
- v13点名回应修复实际准备97.79秒成功，张小珺251段、曾鸣292段可用，已不再整组丢弃嘉宾。因随后补角色合同，完整问答和来源在v14重新收口，尚不标Goal完成。

## 收尾补丁：同名排除与一般检索

- 先以两个失败的业务断言复现：同集同名人物排除后准备创建新ID；人物排除后普通检索也找不到有效原文。红例见 /tmp/persona335-exclusion-red.log。
- 本集同名身份用独立核对的规范名与身份依据匹配原ID；允许继续命中本集已排除记录，跨集仍要求当前可靠且未排除的依据。另复现算法升级导致排除失效（/tmp/persona335-namesake-upgrade-red.log），已补同名排除跨算法/来源版本重建与撤销的回归。
- 一般检索继续严格核对来源、版本、位置和原文；人物归属一致性移入人物可靠性条件。被排除或失效归属不能成为人物命中或返回人物标签，原文本身保留可检索。
- 四个相关后端包的race通过：/tmp/persona335-namesake-upgrade-green.log。最终全后端test/vet见 /tmp/persona335-closeout-final-go.log 与 /tmp/persona335-closeout-final-vet.log。
- 新预标注合成留出910005通过实际Runtime：主持人顾言、两个不同身份的陈晨保持不同ID，6个明确片段正确归属，1个不确定片段pending；41.56秒。证据 /tmp/persona335-evidence/final-same-name-holdout/real-910005.json。此处是合成留出，不称真实播客。
- @选项的角色和身份文字通过aria-describedby向辅助技术提供描述，同名选择回归验证发送对应ID。前端136文件948测试、类型/lint/build通过，恢复Next生成配置后type-check再通过。
- 最新九集真实Runtime仍在运行；浏览器v16加载最终后端与新前端。此前浏览器准备观察脚本把“取消人物处理”误写成“取消”，观察超时并非应用失败，未重复发起请求。
- 最终验收文档的旧指纹与旧49/49覆盖率尚待本轮重算，不得当作当前结论。固定49个可归属分母不改变；目标集514/541保守降为pending，392保持pending。

## 最终授权内收口

- 圆桌角色误判已修正：单次点名、提问、回答或不是本轮主持，均不足以单独判guest。新真实重跑66339的李俊/王老师为unknown，66224陈皮仍unknown，目标集仍张小珺host/曾鸣guest；沿用原Runtime、150秒预算，未增加调用层。
- 九集真实重跑加受影响三集最终重跑、五个预标注合成留出、固定62段复核均有结果。最终标注归属47/47正确、覆盖47/49；13个不确定片段均未确认；不计未审查1765段为正确。细表和唯一当前结论以验收记录为准。
- v17最终代码从原始资料重新准备，44ms出现状态，86789ms名单可用，问题保留。实际@回复111186ms完成，公开来源校验失败后仅依据本集回答，原话与片段2/236匹配。来源点击真正定位到原文ARTICLE并保留问题，桌面与移动截图已查看。
- 最终全后端test/vet、前端136文件948测试/type/lint/build通过；恢复Next生成配置后的type再次通过。四相关包race通过，最终仅角色提示有后续改动并已重跑全后端和受影响模型链。
- 临时只读搜索诊断已执行并删除临时测试文件，证据日志保留。其结果确认目标库内搜索返回8条可靠发言，两个原话片段命中；没有把公开搜索等待误诊成原文丢失。
- Git写入、关票、生产迁移/回填/部署未执行。角色/发言模型仍有泛化不确定性，验收只证明所列固定范围，不能声称普遍零错误或生产旧资料已经修好。
