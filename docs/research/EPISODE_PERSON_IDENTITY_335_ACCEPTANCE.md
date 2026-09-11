# #335 人物识别修复验收记录

状态：PR #340 已合并（`f8ea534`）；本次记录包含合并后审查修复。代码、相关必需检查和最终Runtime/浏览器隔离验收已完成；生产迁移/回填/部署及关票仍未执行。

## 版本与范围

- 父合同：GitHub #335；依赖 #336 → #337 → #338 → #339。过程与失败记录见 [实施记录](EPISODE_PERSON_IDENTITY_335_IMPLEMENTATION.md)。
- 原专项隔离工作树 `issue-335-identity-repair` 已通过 PR #340 合入；当前 follow-up 只包含合并后审查发现的维护命令备份绑定和同集同名 ID 稳定性修复。
- Schema29；识别版本 `identity-evidence-v3`。沿用现有Runtime的balanced：`gpt-5.6-luna / max / priority`，身份阶段禁用所有工具，不含私有备注。
- 每集先识别身份，再对建议归属作1–3组并行复核，最多4次调用，共用原150秒总deadline。小集减少调用，不截断输入、不增设网关或后台平台。失败、取消或复核缺项不发布部分结果。
- 新调用预算与体验影响已说明；在当前Goal明确授权的识别修复内实施。生产、Git写入和代理委派仍各自遵守授权边界。

## 验收证据逐项映射

下表文件名所指Go人物测试位于backend/internal/personidentity；前端面板位于frontend/src/components/inbox，HTTP测试位于backend/internal/handlers；其他明确路径相对于仓库根目录。实际Runtime结果与自动化合同测试分开，不把受控输出当作模型质量证据。

| 票/验收项 | 证据 |
| --- | --- |
| #336-1 旧规则反例与七伪候选排除 | `identity_repair_test.go` 的 `TestPrepareDoesNotPromoteUnreviewedPhrases`、`TestPrepareWithoutRuntimeDoesNotInferPeople`；目标真实名单 |
| #336-2 原始资料得张小珺/主播、曾鸣/嘉宾 | `closeout-role-targets/real-66314.json`；最终浏览器v17再次验证；未预填规范姓名，原文“小俊”保留 |
| #336-3 角色更新且不混同身份 | `TestPrepareKeepsSemanticHostRole`、`TestNameConfirmationDoesNotFreezeAutomaticRole`、角色纠正HTTP与浏览器保存；同源unknown保留已知角色、冲突待确认、旧引文失效测试；`TestCurrentRoleEvidenceSurvivesUnknownAndConflictsRequireCorrection`及依据失效回归 |
| #336-4 代班/机构/同名/近音/昵称/英文/复姓/被提及者 | 五个预标注合成留出；`TestPrepareKeepsSameNamePeopleDistinctAndSupportsNicknames`、`TestCrossEpisodeIdentityRequiresDistinctiveEvidence`、`TestHomophonousSourceNameDoesNotMergeDistinctCanonicalParticipants` |
| #336-5 介绍/转述/冲突不误归属 | 错Speaker锚点测试、`TestNamedResponseSurvivesOverSpecificFirstPersonBasis`、分组复核高层测试；910003中引用他人不生成人物；66224-100等固定边界 |
| #336-6 真实基线、留出与最终重跑 | 下方九集、固定62段与五个留出；66078首次使用前标注，其余开发样本参与过调试 |
| #336-7 公共来源与原文保护 | `TestPrepareSuppliesPublicPodcastContext`（含发布日期）、Runtime复核请求无私有备注断言、原文不改写HTTP测试 |
| #337-1 旧九候选原库升级 | `legacy_upgrade_test.go` 的 `TestSchema28NineLegacyCandidatesUpgradeWithoutClearingFacts` |
| #337-2 同源替换与重复准备 | `TestSameSourceReprepareReplacesAutomaticPeopleAndPreservesConfirmation`；同音及完全同名候选重复准备ID稳定；同名排除跨算法与来源版本保留 |
| #337-3 人工事实独立保留 | `TestAppearanceCorrectionsPersistIndependentlyAndExclusionIsReversible`、`TestSameEpisodeNamesKeepDistinctSourceBackedIdentities`、姓名不确认全篇与真实排除/撤销 |
| #337-4 原文换版与历史确认 | `TestCorrectionReplacementAndVersionIsolation`、`TestSameVersionRebuildRemovesMissingFragmentsButKeepsConfirmationHistory` |
| #337-5 索引/取消/并发原子性 | `TestIndexFailureRollsBackIdentityPublicationAndCorrections`、晚到准备/索引测试、复核遗漏与取消不发布测试 |
| #337-6 来源变更/删除与一般检索 | 元数据失效、最后artifact删除测试；混合片段和排除人物的原文仍可一般检索且无人物归属 |
| #337-7 纠正HTTP与跨集隔离 | `TestAppearanceCorrectionHTTPValidatesAndSeparatesRoleFromIdentity`、`TestConflictingNameCorrectionDoesNotRenameOtherEpisodes` |
| #337-8 只读预览/限定重建/失败报告 | CLI `TestPreviewDoesNotWriteAndApplyRequiresExplicitScope`、`TestApplyRejectsHealthyBackupFromAnotherDatabase`、`TestLogicalFingerprintMatchesAnExactDatabaseCopy`；独立副本真实CLI执行，未选集不修改 |
| #337-9 新迁移与只读启动 | 仅新增29，未改27/28；影子升级/恢复、迁移声明与必需表检查测试，全后端通过 |
| #338-1 正确名单/无技术标记 | 实际@列表截图与API；默认角色名称、真实待确认状态分开 |
| #338-2 姓名/角色/片段证据与原文 | `EpisodePersonEvidence`、源片段跳转测试与真实页面 |
| #338-3 角色/排除/撤销持久化 | 实际API写回、重开读取与业务重复准备测试 |
| #338-4 身份与角色待确认分开 | pending人物阻止提问；unknown角色显示“角色待确认”；角色确认不确认身份 |
| #338-5 选择失效/迟到响应 | 面板测试与真实排除后保留问题，要求重选或明确普通问答 |
| #338-6 四态与取消 | 初次/旧资料待准备、慢请求保留输入、取消与失败重试；HTTP断连取消红绿回归、代理45秒测试 |
| #338-7 桌面/移动/键盘/IME | 实际桌面与390×844、Chromium composition+Enter不提交；不声称覆盖操作系统候选窗 |
| #338-8 原问答/档位/备注/来源 | 全前端951测试、lint/build/type-check；真实普通与人物回答、来源跳转 |
| #339-1 最终链无预填标准答案 | 原始资料→Runtime→持久化→实际页面；v17-prepare.log、v17-answer.log、v17-source-visible.log |
| #339-2 名单/角色/锚点/首中尾与冲突 | 九集逐人检查、固定62段文本层审查；其余1765段不计入准确率 |
| #339-3 指标与分母 | 见下方独立姓名/角色/归属指标，自动归属数量不作正确数 |
| #339-4 固定集无已证实错误归属 | 固定62段与五个留出；明确文本判断边界，不宣称普遍零错误 |
| #339-5 升级/人工/并发/取消/索引失败 | 对应#337高层回归，全后端与复核race通过 |
| #339-6 真Runtime与真浏览器整链 | 最终v17准备→问答→来源；桌面与390×844截图；其余未改变路径复用已通过的真实交互证据 |
| #339-7 版本/模型/来源/耗时 | 本节代码指纹、v3与Schema29、实际档位、逐集耗时、原始artifact版本 |
| #339-8 检查与剩余风险 | 本地检查全部通过；GitHub CI尚未执行，生产未变更 |
| #339-9 生产预览/顺序/恢复 | 下方可审阅方案，操作待另行授权 |

## 真实质量范围

只读盘点入口为Focus与完成历史：7集Focus、4集完成历史均无后续分页，共11个不同单集，其中9集有成功转写。当前artifact版本与导出核对证据为 `final-source-refresh.json`。这不是全库历史成功转写的总量证明。

| 单集 | 核对后的名单/角色 | 实际准备秒数 |
| --- | --- | ---: |
| 66074 | 泓君/主播；黄东旭/嘉宾；张宏江/嘉宾 | 83.29 |
| 66078 | 伊凯/主播；Rich Sutton/嘉宾；Khurram Javed/嘉宾；Sonya Huang/主播；Alfred Lin/主播 | 92.39 |
| 66125 | 薛铁鏻/主播；蓝华峰/嘉宾 | 75.09 |
| 66224 | 曲凯/主播；逯雨鑫/嘉宾；陈皮/角色待确认 | 54.24 |
| 66252 | Koji/主播；Kevin Ding/嘉宾 | 79.06 |
| 66271 | Roy/主播 | 33.72 |
| 66314 | 张小珺/主播；曾鸣/嘉宾 | 93.80 |
| 66339 | 老庄/主播（身份待确认）；李俊/角色待确认；王老师/角色待确认 | 83.11 |
| 66413 | Tibo Sottiaux/嘉宾 | 57.37 |

- 人物精确率21/21、可确认具名出场者召回21/21；另1人身份待确认。姓名/称呼21/21有来源支持；已确认人物角色18/21有据，另外3人角色待确认。身份待确认的老庄不计入21个已确认人物。
- 固定62段：47段自动归属均匹配，13段混合/不确定均未确认，另2个原标注可归属片段保守待确认。归属精确率47/47，覆盖47/49（95.9%）。保持原49分母，不改变金标准以提高指标。固定首/中/尾与边界样本不是随机抽样，不能外推全量准确率。
- 514/541原文本标注支持主持人连贯表达；新实现将未引用的独立应答/短句回声边界保守降级，故记为覆盖损失。392保持不确定。过滤本身不识别人名、不确认归属，人工确认仍可覆盖；未做音频级真值验证。
- 全部1827段中1409段自动归属只是执行数量；其余1765段未纳入62段定量审查，不能把自动绑定数当作正确数。
- 五个预标注合成留出：12个人物、23个明确片段归属均匹配，4个不确定片段均未确认。覆盖代班、机构作者、近音、复姓、英文名、第一人称具名作品、引用、混标签与完全同名不同身份。910005是新留出；此前四个已参与回归，不能继续称全新未知样本。

最终语料记录采用可追溯组合：`closeout-nine/`为保守发言过滤及同名键修复后的九集真实重跑；随后发现66339仅凭点名提问被误判为嘉宾，补充圆桌角色判断边界，以 `closeout-role-targets/`重新执行66339、66224和目标66314。其余六集的明确角色来源未受该限定规则影响，复用仍覆盖的结果。目标集另由最终代码的浏览器v17从原始产物重新准备；没有读取旧候选缓存代替识别。

逐集名单、角色及每个Speaker身份锚点已核对，固定62段与所有已知冲突对应 `closeout-final-fixed-frame.json`；逐集数量/来源为 `closeout-final-corpus-summary.json`。合成结果为 `closeout-holdouts/`，完全同名首次结果为 `final-same-name-holdout/`。本地证据根目录 `/tmp/persona335-evidence/`；原始语料、数据库、日志不进入Git。

## 检查与运行证据

- 最终后端全量test/vet：`/tmp/persona335-role-closeout-go.log`、`/tmp/persona335-role-closeout-vet.log`，退出0。
- 四个相关包personidentity/contentsearch/handlers/episodecopilot的race通过：`/tmp/persona335-namesake-upgrade-green.log`；不称全部后端race覆盖。随后仅追加普通圆桌讨论不默认嘉宾的提示，已通过全后端检查。
- 前端136文件951项测试、全量lint/build/type-check通过：`/tmp/persona335-followup-ui-tests.log`、`/tmp/persona335-closeout-ui-lint.log`、`/tmp/persona335-closeout-ui-build.log`、`/tmp/persona335-closeout-final-type.log`；follow-up 另新增3项后端回归测试。随后仅恢复Next自动改写的生成配置，type-check再次通过。
- 实际姓名改名并恢复、同ID且无测试别名：`/tmp/persona335-browser-name-correction.log`。角色/排除/撤销、问题保留、移动面板与原文跳转已实际验证。
- Chromium原生组合输入Enter未提交：`/tmp/persona335-native-ime-result.log`；不是操作系统输入法全覆盖。
- 实际普通回答约117秒；人物回答约17.77秒并显示“基于公开表达的AI模拟，非本人回复”、带原文来源。记录 `/tmp/persona335-normal-answer.log`、`/tmp/persona335-playwright-answer.log`、`/tmp/persona335-answer-source-result.log`。最终v17问答见下条，历史耗时不作当前性能承诺。
- 当前CLI从旧v2结果副本预览needs_preparation=true，再按指定66314使用磁盘原始产物和v3复核重建成功：`/tmp/persona335-v3-cli-results.jsonl`；此前两集真实限定重建见current-cli-results。不是生产回填。
- 历史单步轮次66078曾150秒超时；保留失败记录，未增大上限。最终分组轮次95.09秒完成。分组复核增加等待及调用成本，不承诺每次成功或比原先更快。

- 最终浏览器v17：44ms出现可取消处理状态、86789ms人物可用，原问题保留；恰为张小珺/主播、曾鸣/嘉宾，无七伪候选。原文来自新隔离库磁盘产物，准备未预填正确姓名。证据 `/tmp/persona335-v17-prepare.log`、`/tmp/persona335-evidence/v17-browser-people.json`。
- 最终实际@曾鸣：110772ms首字、111186ms完成，引用片段2/236的原话，明确AI模拟。公开来源校验失败后仅依据库内内容回答，不能说本次公开检索成功或未尝试补搜。证据 `/tmp/persona335-v17-answer.log`。这是已存在问答降级分支的实际运行结果，不影响库内引文正确性；公开搜索时延/成功率不作保证。
- 点击片段2后，选中“转写/逐字稿”，焦点为对应ARTICLE，文本“科层制管理的公司制度会衰亡。”，问题保留。证据 `/tmp/persona335-v17-source-visible.log`。已检查桌面和390×844截图：`output/playwright/persona335-v17-people.png`、`persona335-v17-answer.png`、`persona335-v17-mobile-answer.png`。
- 同名排除/升级与一般检索修复先红后绿：`/tmp/persona335-exclusion-red.log`、`/tmp/persona335-namesake-upgrade-red.log`、`/tmp/persona335-namesake-upgrade-green.log`。角色提示的受影响三集重跑：`/tmp/persona335-closeout-role-targets.log`，无新增模型平台或调用预算。
- managed Fixture在只读status中仍为旧schema26、ready=false；没有切换或升级它。验收使用单独创建的`/tmp/persona335-browser-v17/browser.db`（schema29，unmanaged），普通运行实例与生产数据未改动。

## 生产启用方案（未执行）

1. 另获Git授权后提交、推送、完成CI/审查并固定发布SHA；当前未提交指纹不能冒充正式迁移commit。
2. 另获生产授权后，重新只读预览受影响单集、当前产物与各类人工记录。九集是本次隔离验收集，不等于生产全部待修范围。
3. 按 [迁移指南](../migration/MIGRATION_GUIDE.md) 和 [备份恢复流程](../BACKUP_RECOVERY.md) 完成停写窗口、匹配代码/产物的备份、影子预检与恢复验证。quick_check不代替恢复演练。
4. 通过既有门禁升至Schema29，不改27/28、不通过API启动自动迁移。升级不调用模型、不清空原文或人工记录。
5. 先对66314执行 [限定清单重建](../../backend/cmd/README.md#人物资料定向重建335)，确认两人正确、七伪候选退出、人工记录保留、普通检索有效，再处理批准清单；失败逐集报告和重试，不扩大为全库。
6. 按 [发布清单](../RELEASE_CHECKLIST.md) 配对代码与Schema，复核真实加载版本、人物API、检索与原18089对应产品入口。隔离验收不能替代生产结果。
7. 回退须使用验证过的代码/数据库/产物配对，不能重新开放已知错误自动人物；若回退版本不能维持此边界，保持维护状态，不能擅自恢复错误能力。

最后一次收口修复：v12仅名单正确但曾鸣无归属，实际回答“无法判断”，因此未算整链通过。已修正有独立点名回应依据时的证据类别误标，原引文与安全边界保留；v16已完成实际问答与来源跳转，v17已通过最终角色提示的准备、实际问答和来源定位。记录 `/tmp/persona335-named-response-red.log`、`/tmp/persona335-named-response-green.log`、`/tmp/persona335-v12-answer.log`。
