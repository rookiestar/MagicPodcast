# PodcastIndex 数据集安全升级方案

日期：2026-07-18
状态：已完成 staging 验收、生产原子切换、服务重启和真实回滚验证；最终保持新 PodcastIndex 库在线

## 结论摘要

1. PodcastIndex 当前公开的全量 Feed 数据库下载入口仍是固定地址：
   `https://public.podcastindex.org/podcastindex_feeds.db.tgz`。官方数据集页把它描述为全部仍在轮询的 Feed，格式为 `tar.gz` 内含 SQLite，更新频率为每周。
2. 这个地址没有版本号，官方页面也没有公开文件大小或 SHA-256/MD5 校验文件。因此不能仅凭文件名判断“最新”；每次下载前后都要保存 HTTP 响应头，并自行计算 SHA-256，形成可审计的版本身份。
3. 官方更新频率存在冲突：当前数据集页写 `weekly`；官网首页进一步写明“每周六 22:00 UTC 生成，数小时后上传”；旧 API 文档仍写 `Updated daily`。升级时应优先采用当前数据集页，并以下载对象实际返回的 `Last-Modified`、`ETag` 和 `Content-Length` 为最终依据。
4. 官方只说明可直接用 SQLite 客户端加载，没有提供面向当前导出文件的独立、版本化 Schema 或导入脚本。`database` 仓库中的 Schema 是旧的 MySQL 结构，不能代替对下载后 SQLite 文件的真实结构检查。
5. 生产机目前约剩 25 GiB，磁盘使用率约 89%；现有数据库约 4.90 GiB（5,260,894,208 bytes），mtime 为 2026-01-21，约 457.4 万条。这个余量不满足本文建议的生产盘内安全 staging 门槛，不应直接在生产目录边下载边覆盖。
6. 升级 PodcastIndex 只会更新可查询的候选目录，不会让现有工作流自动切换到非小宇宙 Feed。当前工作流同步链路没有查询 PodcastIndex；“数据库升级”和“失败时替代源”必须分开设计、分开发布。

## 官方数据集事实

| 项目 | 截至 2026-07-18 可确认事实 | 证据 |
| --- | --- | --- |
| 数据集名称 | All live feeds - SQLite | [PodcastIndex 数据集清单（官方仓库当前副本）](https://github.com/Podcastindex-org/web-ui/blob/c5b106be9acea8e921bc056a837fa44f39fbf5e9/server/data/public_datasets.json) |
| 下载地址 | `https://public.podcastindex.org/podcastindex_feeds.db.tgz` | [官方数据集清单](https://github.com/Podcastindex-org/web-ui/blob/c5b106be9acea8e921bc056a837fa44f39fbf5e9/server/data/public_datasets.json)、[官方 database 仓库](https://github.com/Podcastindex-org/database/blob/c10aaa7baa90f2c6620ca64ad6c4a59d25fc9be2/README.md) |
| 文件命名 | 固定名 `podcastindex_feeds.db.tgz`，文件名不含日期或版本 | 同上 |
| 压缩与内容 | `tar.gz`，内容为 SQLite；可用 sqlite3 或其他 SQLite 客户端直接查询 | [官方数据集清单](https://github.com/Podcastindex-org/web-ui/blob/c5b106be9acea8e921bc056a837fa44f39fbf5e9/server/data/public_datasets.json)、[官网首页源码](https://github.com/Podcastindex-org/web-ui/blob/c5b106be9acea8e921bc056a837fa44f39fbf5e9/ui/src/pages/landing.tsx#L167-L170) |
| 数据范围 | 当前仍被轮询的非 dead Feed；不含节目单集；部分属性被排除 | [官方数据集清单](https://github.com/Podcastindex-org/web-ui/blob/c5b106be9acea8e921bc056a837fa44f39fbf5e9/server/data/public_datasets.json)、[官方 API 文档源码](https://github.com/Podcastindex-org/docs-api/blob/caf2697be05746b47200bb8cc1ea93d1ec3d7c3d/api_src/paths/static/public/podcastindex_feeds_db.yaml) |
| 更新频率 | 当前数据集页：每周；官网首页：周六 22:00 UTC 生成，数小时后上传；旧 API 文档：每日。三者不一致 | [数据集清单](https://github.com/Podcastindex-org/web-ui/blob/c5b106be9acea8e921bc056a837fa44f39fbf5e9/server/data/public_datasets.json)、[首页源码](https://github.com/Podcastindex-org/web-ui/blob/c5b106be9acea8e921bc056a837fa44f39fbf5e9/ui/src/pages/landing.tsx#L167-L170)、[API 文档源码](https://github.com/Podcastindex-org/docs-api/blob/caf2697be05746b47200bb8cc1ea93d1ec3d7c3d/api_src/paths/static/public/podcastindex_feeds_db.yaml) |
| 文件大小 | 官方页面未公布；本次网络无法连通下载域名，未取得可信 `Content-Length` | 上述官方页面均无大小字段 |
| 官方校验值 | 未发现官方 SHA-256、SHA-512 或 MD5 清单 | 上述官方数据集页、database 仓库和 API 文档均未提供 |
| 备用入口 | 官方 database 仓库还列出 IPFS/IPNS 下载入口 | [官方 database 仓库](https://github.com/Podcastindex-org/database/blob/c10aaa7baa90f2c6620ca64ad6c4a59d25fc9be2/README.md) |

### 本次无法确认的动态元数据

本机当前访问 `podcastindex.org` 和 `public.podcastindex.org` 均超时，因此本次没有拿到真实下载对象的响应头，也没有下载大型文件。以下字段必须在正式升级窗口重新只读获取，不能用猜测补齐：

- `Last-Modified`
- `ETag`
- `Content-Length`
- `Content-Type`
- 是否支持断点续传（`Accept-Ranges`）
- 压缩包实际文件名、解压后大小和 SQLite 内部 Schema

“最新版本”应定义为：正式升级时下载地址返回的对象，并由
`下载 URL + Last-Modified + ETag + Content-Length + 本地 SHA-256`
共同标识。若下载前后的响应头发生变化，应丢弃本次产物并重新下载。

## Schema 与兼容性边界

- 官方数据集页只承诺 SQLite 可直接加载，没有发布与每周导出绑定的版本化 Schema。
- 官方 `database` 仓库提供的 `create_table_statement.sql` 和 `pcapi_schema.sql` 是 MySQL 结构，且最近内容停留在 2023 年；它们可帮助理解字段含义，但不能证明 2026 年 SQLite 导出的实际表名、列名或类型。
- MagicPodcast 当前查询依赖 `podcasts` 表以及项目自建的 `v_unique_podcasts` 视图。生产库目前缺少该视图，所以即使数据库文件可打开，也不能直接视为应用兼容。
- 正式切换前必须从候选 SQLite 本身读取 `sqlite_master` 和 `PRAGMA table_info`，逐项核对应用实际读取的表、列与大小写；随后只在候选库上创建项目视图并执行代表性查询。

### 当前旧库的候选基线

针对 2026-06-02 以来因 `feed.xyzfm.space` 拒绝访问而失败的 146 个节目，当前旧库的只读扫描结果为：

- 24 个节目能按完全相同标题找到非小宇宙候选；
- 其中 22 个在旧库中标记为 `dead=0` 且最后状态为 200；
- 现场重新请求后只有 16 个仍能正常返回，另外 6 个超时；
- 标题相同并不能单独证明是同一节目，尚未通过 iTunes ID、Podcast GUID 或单集重合度确认的候选不得自动替换。

新库验收时应使用同一组 146 个节目重跑匹配，把“高置信身份一致且现场可访问的候选数”与上述基线比较。若只有记录总量增长、可用候选没有改善，则升级不能被视为解决工作流问题。

## 许可与使用约束

- 官方数据集页明确写明这些导出可免费镜像和使用，并要求不要高频循环请求；需要批量数据时，应优先下载 SQLite/CSV，而不是抓取在线 API。
- 官方 `database` 仓库 README 声明仓库内容采用 MIT 许可，但仓库没有独立 LICENSE 文件，GitHub 也没有识别出许可证。实际使用时应保留来源记录，不把这句 README 声明扩大解释成对所有第三方 Feed 内容的重新授权。
- PodcastIndex 服务条款明确提醒：Feed 标题、描述、图片、音视频等可能属于第三方内容，使用者仍需遵守第三方权利；API 返回内容也有单独的缓存和数据库化限制。本文方案使用官方批量导出，不用 API 批量抓库，但仍不应把 Feed 内容再次公开分发。
- MagicPodcast 是自托管内部使用场景，建议保留下载 URL、时间、HTTP 元数据和 SHA-256；不要公开镜像数据库，也不要把可能含邮箱等字段的原始库暴露给前端下载。

证据：[官方数据集清单](https://github.com/Podcastindex-org/web-ui/blob/c5b106be9acea8e921bc056a837fa44f39fbf5e9/server/data/public_datasets.json)、[database 仓库 README](https://github.com/Podcastindex-org/database/blob/c10aaa7baa90f2c6620ca64ad6c4a59d25fc9be2/README.md)、[PodcastIndex 服务条款](https://github.com/Podcastindex-org/legal/blob/22931ef37aa0bdf05d00532face626936cc3d74a/TermsOfService.md)。

## 安全升级方案

### 0. 发布边界

本次升级仅替换 PodcastIndex 外部候选库，并补齐项目所需视图；不修改 MagicPodcast 主业务库，不上线 Feed fallback 或工作流状态逻辑。由于当前机器直连 DNS/HTTPS 不可用，本次经明确批准，允许验证器通过本机 FlyingBird 代理完成官方数据集的 staging 下载和候选 URL 可访问性检查；代理不注入生产服务、不改变业务网络配置，且由 manifest 记录代理端点。每项能力单独验证、单独发布，避免无法判断失败来自数据、查询还是网络出口。

### 1. 在非生产盘取得并锁定下载对象

优先在空间充足的可信 staging 主机或外接卷操作，不在当前生产目录直接下载。

1. 先请求响应头，记录 `Last-Modified`、`ETag`、`Content-Length`、`Content-Type`、`Accept-Ranges`。
2. 只有 `Content-Type` 与 gzip/tar 下载相符且长度非零时才下载；文件先使用 `.partial` 后缀。
3. 下载完成后再次请求响应头。下载前后 `ETag`、`Last-Modified` 或长度任一变化，都视为上游正在换包，本次产物作废。
4. 对完整压缩包计算 SHA-256，写入同目录 manifest；ETag 只用于辨别对象变化，不能替代密码学校验。
5. 先验证 gzip 完整性并列出 tar 内容。压缩包只能包含预期的普通 SQLite 文件；拒绝绝对路径、`..` 路径、符号链接和额外可执行文件，再解压到全新目录。

### 2. 空间门槛

正式开始前，生产或 staging 文件系统的可用空间应满足：

`压缩包大小 + 新数据库解压后大小 + max(20 GiB, 文件系统总容量的 15%)`

这里的最后一项是运行和回滚安全余量，不能拿来存放新文件。压缩包和解压后大小必须来自本次真实下载，不能沿用旧库大小估算。

当前生产机约 25 GiB 可用、使用率约 89%，仅 15% 的安全余量就已经高于现有余量，因此当前状态为 **NO-GO**。应选择以下之一：

- 在外接卷或另一台可信机器完成下载、解压和完整校验，只把已验证的新 SQLite 传到生产机；或
- 先清理/扩容，使生产机在新库落盘后仍保留至少 15% 空间，再开始升级。

### 3. 候选库离线验收

所有检查都针对候选库，不碰当前生产库：

1. 确认文件类型为 SQLite，执行只读完整性检查。
2. 读取真实表、列、索引和用户版本，核对 MagicPodcast 所需字段。
3. 比较新旧库记录数、有效 Feed 数、HTTP 200 数、dead 分布和最近更新时间。记录数可以正常增减，但若变动超过预设阈值（建议先以 ±20% 为人工复核线），不得自动切换。
4. 在候选库上执行项目现有 `create_unique_podcasts_view.sql`，确认 `v_unique_podcasts` 创建成功且代表性标题、URL 查询能返回结果。
5. 针对当前失败的 `feed.xyzfm.space` 样本，按标题、作者、iTunes ID、Podcast GUID 等稳定标识查找非小宇宙候选。只把结果输出为审查清单，不改主业务库。
6. 保存 manifest：官方下载元数据、压缩包 SHA-256、解压库 SHA-256、文件大小、记录数、Schema 摘要、视图校验结果和执行时间。

### 4. 原子切换

PodcastIndex 数据库连接会在服务启动时打开，因此不能在服务运行中覆盖文件。

1. 选维护窗口，停止 MagicPodcast 服务，确认旧连接已关闭。
2. 将已验证候选库复制到与正式库相同的文件系统，再校验一次 SHA-256。
3. 保留当前 `backend/data/podcastindex_feeds.db` 为带时间戳的只读回滚副本。
4. 使用同一文件系统内的重命名完成原子切换，不用 `cp` 直接覆盖正式路径。
5. 启动服务，检查健康状态，并实际执行 PodcastIndex 代表性查询和工作流的只读预检。
6. 观察至少一个完整工作流周期后再删除压缩包；旧库至少保留一个明确的回滚窗口，并受磁盘门槛约束。

### 5. 回滚

出现以下任一情况立即回滚：服务无法启动、查询报缺表/缺列、视图查询失败、代表性匹配明显下降、数据库只读检查异常或磁盘逼近安全线。

回滚步骤：停止服务 → 将新库移出正式路径 → 原子恢复旧库文件名 → 重启服务 → 重跑健康检查和代表性查询。失败的新库及其 manifest 暂时保留在 staging 供排查，不在生产目录继续试修。

## 与工作流 Feed fallback 的关系

数据库升级只能让“查找同一节目其他 Feed”拥有更新的候选数据，不能自动改变当前工作流。后续实现应单独立项，并至少满足：

- 只在原 Feed 抓取失败且候选身份达到明确置信门槛时尝试替代源。
- 优先用 iTunes ID、Podcast GUID 等稳定标识，标题/作者只作辅助；不得仅按同名节目自动替换。
- 替代 Feed 要先做可访问性与节目身份抽样验证，并保留原始 Feed、候选来源和选择理由。
- 替代失败时维持“部分完成/失败”的真实状态，不把错误重新隐藏。

## 可选：反屏蔽 / 更换 source IP 实验

这项实验与 PodcastIndex 升级分开进行，也不能与数据库切换同一批发布。

在下一次已知失败窗口，对同一个小样本、相同请求头和超时设置做四组只读对照：

1. 生产机直连，维持当前行为。
2. 生产机直连，但单线程、限速并拉开请求间隔。
3. 从另一条独立出口访问。
4. 通过受控、可信的固定中继访问。

启用域名级中继的必要条件是：同一时间窗口内，生产直连稳定失败，而独立出口或可信中继稳定成功；同时要排除仅靠降并发/限速即可恢复的情况。未满足这个对照证据时，不上线代理。

若最终采用中继，边界必须是：

- 只代理 `feed.xyzfm.space`，不启用全局代理。
- 中继只接受预先允许的域名和请求方法，不开放任意 URL 转发，防止被当作公共代理。
- 不把访问凭据、完整查询参数或敏感响应写入日志；凭据只通过安全配置注入。
- 设置并发、速率、超时和熔断上限；中继失败时回到 PodcastIndex 替代源决策，不进行无限重试。
- 代理方案和 PodcastIndex 数据库升级分别发布、分别回滚，确保每次变化只有一个变量。

## Go / No-Go 清单

只有以下条件全部满足才允许切换：

- [x] 已取得并保存真实 HEAD 元数据，下载前后对象身份一致。
- [x] 压缩包与 SQLite 均有本地 SHA-256，manifest 完整。
- [x] 空间门槛满足；切换后仍保留至少 15% 文件系统余量。
- [x] gzip、tar 安全检查和 SQLite 完整性检查通过。
- [x] 实际 Schema 与 MagicPodcast 查询字段兼容。
- [x] `v_unique_podcasts` 已在候选库创建并通过代表性查询。
- [x] 新旧数据量和质量差异已审查，没有未解释的异常。
- [x] 当前库备份可用，原子切换和回滚命令已在同文件系统演练。
- [x] 服务停机、重启和验证窗口明确。
- [x] 明确接受：本次升级不会自动修复工作流 Feed fallback。

## 实际执行结果（2026-07-19）

- 官方对象通过明确批准的 FlyingBird staging 代理取得；下载前后 HTTP 指纹一致，压缩包 SHA-256 为 `4d2031dac6b50496502ab94aa303c28e494d2a933d8b283adc92183b9bc3a00a`，候选 SQLite SHA-256 为 `fb5aabd5233ac23f282c3e88f415d3c1a659955cd8d08430a9a14df5cf0ec24e`。
- 候选真实 Schema、`v_unique_podcasts`、URL/title/iTunes ID 查询、gzip/tar 安全检查和 146 个失败样本对比已通过；样本结果为匹配 24、现场可访问 22、稳定身份确认 0，未把同名结果当作自动替代源。
- 生产机只接收已验收 SQLite，压缩包不在生产目录长期保留；正式库先保留为时间戳 rollback 副本，再通过同文件系统原子重命名切换。
- 生产重启后的 `/health`、`/ready`、前端首页和代表性查询均通过；随后完成一次真实回滚、旧库启动检查，再将新库切回并恢复 supervisor。
- 生产证据 manifest 保存在生产 staging 的 `validate.manifest.json`、`rollback-verified.manifest.json` 和 `production-final.manifest.json`；本次没有修改 MagicPodcast 主业务库、Feed fallback、代理配置或工作流状态。

## 官方来源

1. [PodcastIndex Datasets 页面](https://podcastindex.org/datasets)
2. [Datasets 页面当前数据源（官方仓库固定提交）](https://github.com/Podcastindex-org/web-ui/blob/c5b106be9acea8e921bc056a837fa44f39fbf5e9/server/data/public_datasets.json)
3. [官网首页下载与周六生成说明（官方仓库固定提交）](https://github.com/Podcastindex-org/web-ui/blob/c5b106be9acea8e921bc056a837fa44f39fbf5e9/ui/src/pages/landing.tsx#L167-L170)
4. [PodcastIndex database 仓库 README](https://github.com/Podcastindex-org/database/blob/c10aaa7baa90f2c6620ca64ad6c4a59d25fc9be2/README.md)
5. [旧 API 文档中的静态数据库说明](https://github.com/Podcastindex-org/docs-api/blob/caf2697be05746b47200bb8cc1ea93d1ec3d7c3d/api_src/paths/static/public/podcastindex_feeds_db.yaml)
6. [PodcastIndex database 旧 Schema](https://github.com/Podcastindex-org/database/blob/c10aaa7baa90f2c6620ca64ad6c4a59d25fc9be2/pcapi_schema.sql)
7. [PodcastIndex 服务条款](https://github.com/Podcastindex-org/legal/blob/22931ef37aa0bdf05d00532face626936cc3d74a/TermsOfService.md)
