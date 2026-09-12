# #349 最终验收覆盖表

状态：本地实现与验收完成。来源为当前 GitHub #349；生产发布与真实数据操作未执行。

## 用户故事

| # | 要求 | 当前证据与结论 |
| --- | --- | --- |
| 1 | As an 正在处理单集的用户, I want 地址显示当前单集、内容页签和人物面板, so that 复制链接后能继续同一项工作。 | 通过：#350/#351 详情、转写、人物面板双向导航；当前前端全量回归覆盖。 |
| 2 | As an 收藏单集的用户, I want 单集拥有与队列无关的固定地址, so that 移到 Someday 或 Done 后链接仍然有效。 | 通过：按 episode ID 独立读取；Done/未入队浏览器证据及消费服务测试，旧队列不参与身份读取。 |
| 3 | As an 从外部打开链接的用户, I want 无需先访问 Inbox 就能打开单集详情, so that 直接抵达目标内容。 | 通过：多个来源及独立新标签读取 /episodes/:id，不要求先加载 Inbox。 |
| 4 | As an 刷新页面的用户, I want 恢复相同单集和已保存内容的位置, so that 不必重新寻找入口。 | 通过：#350/#351 刷新页签、人物、版本片段；#354 搜索/报告等刷新与来源恢复。 |
| 5 | As an 站内浏览的用户, I want 进入单集时保留现有详情浮层体验, so that 保持来源列表上下文。 | 通过：Inbox 内 native history 保留原看板浮层；InboxPageClient 回归及 #350 实测。 |
| 6 | As an 使用浏览器导航的用户, I want 前进和后退恢复实际访问过的面板与页签, so that 界面始终与地址一致。 | 通过：分域前进后退；#354 来源前进/关闭及公共 navigation 测试。 |
| 7 | As an 关闭详情的用户, I want 回到可信来源列表并恢复焦点和滚动, so that 继续原来的筛选工作。 | 通过：#354 搜索/历史/节目/发现/Focus/报告返回原查询与触发焦点；节目 scrollY=298.5 恢复。 |
| 8 | As an 直接打开详情的用户, I want 无站内来源时能返回 Inbox, so that 不会因关闭操作意外离开站点。 | 通过：独立详情无来源映射 Inbox；来源地址白名单与 episode ID 匹配，不接受任意外站。 |
| 9 | As an 阅读单集的用户, I want 直达 Show Notes、笔记或指定加工产物, so that 不必逐层切换。 | 通过：#350 三类页签与产物视图、独立页面测试与浏览器。 |
| 10 | As an 核对人物的用户, I want 链接定位到人物与发言面板及指定人物, so that 继续核对已保存资料。 | 通过：#351 person=9 的人物区域与浏览选择恢复；错单集人物处理测试。 |
| 11 | As an 核对发言的用户, I want 引用绑定来源版本和片段, so that 重新转写后不会定位到无关发言。 | 通过：artifact-34902/34903 差异内容；#354 助手引用实际打开历史第二段。 |
| 12 | As an 查看失效引用的用户, I want 明确知道原版本或片段不可定位, so that 自行决定是否查看当前版本。 | 通过：失效版本明确报错不替换；#351 浏览器及 processing/people 测试。 |
| 13 | As an 试听原文的用户, I want 用链接明确指定音频时刻且不自动播放, so that 在自己准备好后试听。 | 通过：t=25 为00:25且audio src=null；播放器22项及全量回归覆盖，不验证真实音频质量。 |
| 14 | As an 使用单集助手的用户, I want 恢复助手开关和仍有效的配置及目标人物, so that 继续查看同一工作区域。 | 通过：#351 assistant/profile/target_person 组合恢复；失效目标禁提交，Copilot测试。 |
| 15 | As an 同时核对与提问的用户, I want 人物面板和助手开关相互独立, so that 保留现有并排能力。 | 通过：#351 人物与助手并排、分别关闭、Esc、后退实测及测试。 |
| 16 | As an 谨慎的用户, I want 打开链接只读取已保存资料, so that 不会自动识别、执行模型、提交纠正或移动单集。 | 通过：直接打开及引用导航记录业务写请求0；后端重复GET只读测试。受控提问回复另行标注。 |
| 17 | As an 编辑资料的用户, I want 导航不悄然丢失未保存修改, so that 可以明确保存、保留或放弃编辑。 | 通过：备注/人物/标签/工作流/助手草稿保护；取消离开保留输入。#354 补直接加载 dirty 页的popstate保护。 |
| 18 | As an 关注隐私的用户, I want 未提交姓名、笔记、提示词和凭据不进入 URL, so that 避免它们泄漏到地址及历史记录。 | 通过：URL只写已提交搜索及公开位置参数；原始字段草稿保留在组件内；浏览器输入与URL分离断言。 |
| 19 | As an 浏览 Discovery 的用户, I want 链接保存已提交筛选和选中单集, so that 重开时恢复内容定位。 | 通过：Discovery filter/episode 实测直达、刷新、关闭及后退；列表外对象独立读取测试。 |
| 20 | As an 阅读首页报告的用户, I want 链接标识所选报告、历史视图和工作流筛选, so that 重新找到同一报告上下文。 | 通过：report绑定job_id、往期与重复workflow；窗口外真实报告读取及多选刷新浏览器。 |
| 21 | As an 查看完成历史的用户, I want 链接保存已提交搜索词, so that 重复使用相同查询。 | 通过：完成历史q=Fixture/Done提交、后退、刷新实测及7项测试。 |
| 22 | As an 浏览个人播客库的用户, I want 保留排序和多标签 URL, so that 已有书签继续可用。 | 通过：播客排序、多tag_id反向/前向/刷新；非法与重复参数清理保留keep参数。 |
| 23 | As an 从节目页定位内容的用户, I want 旧单集卡片链接继续定位卡片, so that 已有链接不会被强制改成详情入口。 | 通过：/podcasts/1002?episode_id=2012仍定位卡片，dialog=0，卡片位于视口。 |
| 24 | As an 从搜索或报告进入的用户, I want 打开同一单集工作台及指定内容, so that 不同入口不会形成不同身份。 | 通过：各来源使用EpisodeLink；助手受控回答的普通来源按钮打开正确历史版本片段，未触发新业务写。 |
| 25 | As an 搜索内容的用户, I want 搜索词和类型拥有可重访地址, so that 能刷新或分享已提交的查询。 | 通过：/search独立页面、同会话侧栏、查询提交及types刷新；节目3项与单集1项内容实测。 |
| 26 | As an 管理标签的用户, I want URL 恢复排序、节目范围和创建或编辑表单, so that 能返回同一管理位置。 | 通过：标签排序、节目范围、独立编辑和创建；#354 从节目点击管理标签生成podcast_id并恢复真实节目。 |
| 27 | As an 管理工作流的用户, I want URL 恢复排序、页签和创建或编辑表单, so that 操作位置可以复访。 | 通过：#352 工作流排序、页签和创建/编辑，草稿取消与关闭历史修复。 |
| 28 | As an 排查工作流执行的用户, I want 直接打开指定 Job 和执行列表页码, so that 不依赖该 Job 恰好位于已加载列表中。 | 通过：#352 page=99&job=5001页外读取；跨工作流拒绝；分页浏览器前进后退。 |
| 29 | As an 阅读工作流报告的用户, I want 报告拥有绑定工作流和 Job 的独立地址, so that 准确引用该次知识产物。 | 通过：独立报告UI、新标签、刷新；Job归属和缺失状态实测；未生成和读取失败测试。 |
| 30 | As an 导入或同步的用户, I want 链接恢复当前操作页签且不触发执行, so that 不会因打开链接重复导入或同步。 | 通过：/import的URL页签与服务端initialTab，直达不恢复文件、不执行导入同步，17项测试。 |
| 31 | As an 打开旧链接的用户, I want 旧详情链接规范化到新单集地址, so that 无需手动修复书签。 | 通过：旧detail链接规范化与移队列目标；根别名跳转保留重复参数；保留旧卡片/列表协议。 |
| 32 | As an 打开不存在内容的用户, I want 区分对象删除、内容未生成和读取失败, so that 知道能否重试或返回。 | 通过：单集/Job/报告/标签缺失与读取失败有区分，重试保持目标；分域实测与HTTP测试。 |
| 33 | As an 快速切换单集的用户, I want 迟到请求不覆盖当前单集内容, so that 不会核对错人或错稿。 | 通过：详情旧响应不覆盖新ID、processing版本竞争与Job取消；前端全量和后端归属测试。 |
| 34 | As an 使用移动端或键盘的用户, I want 同一链接的内容和操作入口完整可达, so that 不受屏幕尺寸与输入设备限制。 | 通过：宽窄屏实测各分域；#354 修复报告遮挡、返回焦点和备注关闭；键盘/Esc/focus测试。 |
| 35 | As an 跨设备访问的用户, I want 链接沿用既有正式入口与认证, so that 无需新增公网服务或暴露本机地址。 | 通过本地范围：内部入口相对路径，未改正式域名/认证/公网配置；未执行未授权跨设备生产验收。 |
| 36 | As an 遇到慢请求的用户, I want 保留目标地址和仍可用的有效内容, so that 不会因定位功能退化为整页空白。 | 通过：#350 1500ms延迟、#352 1200ms延迟的壳/正文时间；错误重试保留目标；原有缓存与有效内容测试。 |

## 路由双向检查

| 目的 | 地址合同 | UI → URL | 独立加载 → UI |
| --- | --- | --- | --- |
| Discovery | `/discovery`；`/` 保留为跳转入口，保留兼容参数 | 首页入口跳转 canonical discovery | 根别名保留重复参数；服务端预读保持有效内容 |
| 发现台筛选、选中单集 | `/discovery?filter=all/unread/uncollected&episode=<id>` | 未读/未收集点击及预读按钮；慢A→B下一项 | episode=2003刷新；SSR initialHref与列表外对象测试 |
| 首页报告上下文 | `/discovery?report=<job-id>&report_history=1&workflow=<id>`；多工作流重复 `workflow` | 报告切换、往期、勾选4001+4003 | 5003窗口外真实报告；往期/已选2刷新 |
| 行动工作台 | `/inbox`；`queue=focus&episode=<id>` 仅表达看板定位，不作为单集身份 | 队列与卡片打开仍保留看板；新详情地址 | 旧queue+episode定位；未带detail不打开详情 |
| 完成历史搜索 | `/inbox/history?q=<query>` | 提交Fixture/无匹配搜索，不逐字写地址 | q=Fixture刷新恢复输入与Done单集 |
| 单集详情 | `/episodes/<id>`，默认 Show Notes | 六类来源EpisodeLink及Inbox卡片 | 新标签、Done、未入队、独立详情读取 |
| 单集笔记 | `/episodes/<id>?tab=notes` | 笔记页签进入tab=notes | 独立notes刷新、编辑取消、直接dirty页历史保护 |
| 转写与产物 | `/episodes/<id>?tab=transcript&artifact=transcript`；`artifact` 允许现有 summary/minutes/transcript | 转写/纪要/摘要切换及组件回归 | artifact direct/刷新与无产物真实状态 |
| 人物与发言面板 | `/episodes/<id>?tab=transcript&artifact=transcript&panel=people` | 转写人物管理入口及关闭按钮 | 最终组合链接、新标签刷新，截图url354-combined-desktop.png |
| 指定人物 | 上一地址追加 `person=<person-id>`；只定位人物，不声明其发言已获确认 | 人物浏览选择与证据按钮 | person=9当前人物；无效目标不冒充他人 |
| 发言/证据片段 | 上一地址追加 `source=<source-version>&fragment=<order>` | 人物证据及助手普通来源按钮 | 34902/34903第二段不同，历史来源点击后高亮真实对应文本 |
| 明确跳到音频时刻 | 转写地址追加 `t=<seconds>`；不自动播放，不随播放进度持续写 URL | 播放器显式调整写时间，tick不写 | t=25为00:25，audio src=null；冲突/超范围测试 |
| 单集助手 | `/episodes/<id>?tab=<underlying-tab>&assistant=1`；按需加 `profile=<id>&target_person=<id>` | 打开/关闭、profile与目标人物选择 | 组合地址恢复deep及目标9，未执行模型 |
| 播客列表 | `/podcasts?sort_by=<value>&tag_id=<id>`，保留重复 tag_id 的现有协议 | title排序与科技/产品两标签、前进后退 | 双标签刷新与规范化；服务端过滤HTML只有深度科技 |
| 节目详情 | `/podcasts/<id>`；保留 `episode_id=<id>` 的卡片定位用途，打开深度处理使用单集地址 | 播客列表卡片仍生成节目地址 | 旧episode_id=2012定位视口卡片，无详情弹层 |
| 搜索 | `/search?q=<query>&type=all/podcasts/episodes` | 侧栏入口、提交查询、节目范围 | 独立query/type刷新、关闭返回、搜索来源回返焦点及容器250px |
| 标签 | `/tags?podcast_id=<id>&sort_by=<value>` | 字母/热度；节目页管理标签入口 | podcast_id=1001显示实际节目和标签 |
| 标签创建/编辑 | `/tags?dialog=create` 或 `/tags?dialog=edit&tag=<id>` | 新建、编辑产品标签3002 | 创建刷新空表单、3001直接编辑、缺失目标和取消离开 |
| 工作流列表 | `/workflows?sort_by=<value>` | 排序execution、后退updated | 直接排序与列表内容；原协议保留 |
| 工作流创建/编辑 | `/workflows?dialog=create`、`/workflows/<id>?dialog=edit` | 创建与编辑按钮、Esc关闭 | 独立create/edit，修改取消后URL及输入保留 |
| 工作流详情 | 保留 `/workflows/<id>?tab=overview/jobs/config` | config/jobs页签生成地址 | 原tab直达与浏览器历史；非法重复参数规范化 |
| 执行列表与指定执行 | `/workflows/<id>?tab=jobs&page=<n>&job=<job-id>`；指定 Job 独立加载，不依赖它恰好在该页 | 展开job5001、下一页、前进后退 | page99仍读job5001；跨工作流拒绝 |
| 工作流报告 | `/workflows/<id>/reports/<job-id>`；不与单集加工运行混同 | 报告按钮生成工作流+Job路径 | 新标签、硬刷新、503恢复、错误归属及未就绪测试 |
| 导入/同步 | `/import?tab=import/sync`；不恢复已选文件，不触发执行 | import/sync页签及后退 | SSR sync=true，刷新正确；不执行、不恢复文件 |

24行均已列出两个方向的内容/导航证据；默认、非法、重复参数由对应组件与公共导航测试覆盖。证据详情、受控响应范围见实施记录。

## 当前全站检查

- 最终前端全量通过141文件995项；最后版本 lint、类型检查和生产构建通过。
- Go 全量测试与 go vet ./... 通过（/tmp/url354-go-all.txt、/tmp/url354-go-vet.txt）；后端此后未变。
- 完整浏览器闭环已复验，包括所有来源返回、人物/助手并排与版本化片段、慢请求竞争及宽窄屏。
- 生产未发布、真实数据未写入；验收记录阶段尚未执行 Git 提交/推送，本次交付不关闭 Issue。

## 证据边界

上述表引用同目录 URL_SYSTEM_IMPLEMENTATION_2026-09-12.md 的分域实测记录及本轮工具结果；人物、版本正文、模型回答属于受控浏览器响应，Go HTTP/数据库测试独立验证实际归属和只读接口。不是生产人物质量或真实模型运行的证据。

源码与最终差异已复核，24 行路由双向证据和 36 条用户故事已映射。生成文件已移到仓库外保留，真实配置与数据未改变。本次交付只处理 Git 提交、推送和合并，不关闭票或发布生产。
