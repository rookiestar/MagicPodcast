# 清单导入：h5 活动页链接适配验证记录

日期：2026-09-14。分支 `codex/collections-h5-activity`（基于 origin/main 5a52003）。承接 #410/#411 的清单来源扩展，新增第三种已核实来源形态。

## 来源核实结论（一手验证）

- `https://h5.xiaoyuzhoufm.com/xyz-activity/forgenz` 返回 HTTP 200，无跳转；页面是“小宇宙活动页”SPA 壳，HTML 内无清单数据。
- 数据来自公开接口 `POST https://web-api.xiaoyuzhoufm.com/web/activity-page/get-by-code`，JSON 请求体 `{"code":…}`；普通 curl 即可访问，无 Origin/Referer/鉴权要求。
- 响应结构：`data.title` 为活动标题；`data.modules[]` 中 `type=EPISODE_VERTICAL_LIST` 的模块携带 `episodes[]`；条目字段为 `id/pid/title/podcastTitle/image.picUrl/duration/payType/media.source/mode/recommendation`，没有 Show Notes、发布时间与节目元数据对象。未知 code 返回 `{"data":null}`（实测大小写敏感）。
- 实测页面：标题《00后的宇宙必听｜给正在长大的你》，7 个图片模块 + 5 个单集列表，共 18 条单集、无跨列表重复、全部 FREE/PUBLIC。

## 实现要点

- `source.go`：来源主机白名单扩为三个；`h5.xiaoyuzhoufm.com/xyz-activity/{code}` 为第三种来源形态（code 与专题 slug 共用同一字符集规则）。生产抓取器按形态分发：活动抓取 POST 由 code 构造的固定接口地址（请求体仅含已校验 code）；活动接口是 POST 端点，任何重定向都直接拒绝；保留私网封禁、禁代理、3MB、20s 边界。
- `activity.go`（新增）：`ParseActivityJSON` 确定性解析；响应 code 与请求不一致即拒绝；图片模块跳过、单集列表按页面顺序展平为一份有序清单；音频快照仅保留 payType 显式为 FREE 且 `media.source.mode=PUBLIC` 的条目（付费状态未知一律留空）；节目封面一律留空（活动载荷的条目图片是单集封面，不冒充节目封面）；缺失字段（Show Notes/发布时间/节目作者）保留空值，不编造。
- 去重身份：`(source_platform, activity:<code>)` 命名空间化，与清单 ID、专题 slug 身份两两可区分；保存规范化活动页地址，刷新沿用并按形态分发。
- 前端弹窗提示与后端 `UNSUPPORTED_SOURCE` 文案列出全部三种受支持格式。

## 验证证据

自动化（本地，全部通过）：

- `go test ./...`（backend 全量，无失败）。
- 新增回归：活动 URL 接受/拒绝矩阵（根路径、其他前缀、多级路径、空 code、非 slug 字符、http、端口、接口主机伪装页面）、解析器（真实响应裁剪 fixture：跨模块展平顺序、推荐语映射、音频快照、缺字段保留空值、code 不匹配/空 data/重复单集/缺节目名拒绝、纯图片页拒绝、空列表）、身份命名空间三形态两两可区分、service 层预览/导入/去重/刷新、`activityAPIURL`/`isActivityAPIURL` 构造与识别。
- 前端 collections 相关 4 文件 24 例（新增活动页链接用例）、eslint、tsc 通过。

真实来源与浏览器导入流程（本地 127.0.0.1:18096 后端 + :3100 前端，一次性临时库）：

- 真实活动页预览：18 条单集、标题与逐集推荐语正确、来源身份 `activity:forgenz`、查询参数剥离。
- 导入 → 同 URL 再预览返回 duplicate 指向同一清单；收录第 1 条 → `podcast_subscribed=false`、`collection_only=true`、`inbox_written=true`（节目仅以名称建库，后续可经 RSS 刷新补全）；刷新 18 条无伪差异。
- 三格式并存回归：旧格式清单（穿透半导体迷雾）与专题（海浪电影周）真实来源预览不受影响；不受支持主机仍拒绝（UNSUPPORTED_SOURCE）。
- 浏览器全流程（playwright，真实页面）：导入弹窗 → 粘贴真实活动页链接 → 预览 18 集（`activity-preview.png`）→ 导入 → 详情页展示（`activity-imported.png`）。

## 边界与限制

- 抓取仅限三个已核实主机与固定接口路径；活动接口为 POST，重定向一律拒绝，未扩大为任意 URL 抓取。
- 活动页的多主题分组（图片模块）在导入结果中展平为单一顺序清单，分组视觉不保留；逐集推荐语与页序保留。
- 活动载荷缺少节目作者/封面/集数与单集发布时间、Show Notes，对应条目字段为空；经收录建库的节目资料比 RSS 来源薄。
- 生产部署不在本次范围；生产可用性未验证。
