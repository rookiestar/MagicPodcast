# 数据 Profile

最后更新：2026-08-24

数据 Profile 让本地开发在隔离网络中继续使用真实 Go 后端与同源 `/api/v1`，不引入前端 Mock 或自动降级。

## Profile

| Profile | 用途 | 边界 |
| --- | --- | --- |
| `fixture` | 默认离线开发 | 版本化确定性基线；运行使用独立可写副本 |
| `snapshot` | 用已脱敏的近期数据验收 | 只读原件；运行使用独立可写副本 |
| `production` | Mac mini 正式服务 | 本地命令拒绝选择；仅规范 release 启动流程可声明 |

网络变化不会触发自动刷新或切换。SQLite 不热切换；每次显式切换都会校验目标、停止本命令管理的后端、启动新进程并等待 `/ready`。失败时保留原 Profile；命令不会接管其他工作树、未知 PID 或端口进程。

正式服务必须显式配置 `MAGICPODCAST_DATA_PROFILE=production` 并带规范生产启动门禁；`release` 模式缺少该声明会拒绝启动。本地统一命令永远不能选择 `production`。固定门禁只防止普通本地/直接二进制误启，不代表用户授权生产操作。

## 日常使用

```bash
./scripts/data-profile.sh status
./scripts/data-profile.sh use fixture
./scripts/data-profile.sh use fixture focus-7
./scripts/data-profile.sh use snapshot latest
./scripts/data-profile.sh use snapshot <snapshot-id>
```

Agent 可使用项目 Skill
[`magicpodcast-data-profile`](../.agents/skills/magicpodcast-data-profile/SKILL.md)。
Skill 只包装上述统一命令；人工使用方式不变。

状态显示 Profile、schema、Fixture 版本/场景/锚点或快照版本/捕获时间；`/ready` 保持只返回非敏感运行元数据，均不显示数据库绝对路径。托管后端显式跳过 `.env`、不继承生产凭据，并禁用后台工作流调度；前端继续通过既有同源代理访问后端。

默认数据目录是系统用户配置目录下的 `MagicPodcast/data-profiles`。可为测试指定：

```bash
MAGICPODCAST_DATA_PROFILE_HOME=/safe/local/path \
  ./scripts/data-profile.sh --port 18080 use fixture
```

指定目录位于仓库内时，只允许使用已被 `.gitignore` 覆盖的 `.magicpodcast-data-profiles/`；其他仓库内路径会被拒绝。目录内的数据库、快照、工作副本、状态、日志和二进制均不得进入 Git。

## Fixture 场景

Fixture 以 Asia/Shanghai 当前整点为固定时间锚点，版本同时包含数据集版本、场景、小时和 schema；同一小时同一场景重复生成稳定且幂等。跨小时后使用新版本，使 14 天窗口与 6/7/29/30 天提示的漂移小于 1 小时。

| 场景 | 用途 |
| --- | --- |
| `journey`（默认） | Discovery 多日期、未读/已读、未收集/已收集；同日两份精选报告及往期；Inbox、Focus 6、Someday、Done、进行中和时间提醒 |
| `empty` | 主要空态 |
| `focus-0` | Focus 空态 |
| `focus-7` | Focus 软上限边界 |
| `focus-over-limit` | 跨端并发后超过 7 项的保留与提示 |
| `completion-history` | 59 条唯一完成事实，覆盖搜索、50 条分页、当前队列定位与不感兴趣后重新处理 |
| `report-empty` | 无可展示精选报告 |
| `report-single` | 单份当日报告 |

使用：

```bash
./scripts/data-profile.sh use fixture
./scripts/data-profile.sh use fixture empty
./scripts/data-profile.sh use fixture focus-0
./scripts/data-profile.sh use fixture focus-7
./scripts/data-profile.sh use fixture focus-over-limit
./scripts/data-profile.sh use fixture completion-history
./scripts/data-profile.sh use fixture report-empty
./scripts/data-profile.sh use fixture report-single
```

默认 `journey` 场景提供 14 天窗口内外、多报告、安全链接、允许图片、危险链接、异常富文本、长标题与缺图等数据。局部请求失败继续由测试层的可控错误注入验收，不通过损坏数据库制造；Fixture 不包含系统推荐、外部笔记连接器或飞书妙记假数据。

## Snapshot 刷新

刷新分为两个明确授权的阶段：

1. 生产侧 `snapshot-export` 只读打开 SQLite，通过 SQLite Online Backup 生成一致副本，在临时隔离目录脱敏并校验。
2. 开发侧 `snapshot refresh` 通过固定传输适配器取得该目录，复核脱敏版本、schema、完整性、必需表和 SHA-256 后原子发布。

生产侧命令示意（**需单独授权后在 Mac mini 执行**）：

```bash
install -d -m 700 /secure/empty/staging
cd backend
go run ./cmd/snapshot-export \
  --source /absolute/production/magicpodcast.db \
  --output /secure/empty/staging \
  --confirm I_AUTHORIZE_READ_ONLY_PRODUCTION_SNAPSHOT_EXPORT
```

`--output` 必须是操作前显式创建的真实空目录；命令拒绝不存在、非空或符号链接目录，并将权限收紧为 `0700`。

开发侧需配置受控传输适配器：

```bash
export MAGICPODCAST_SNAPSHOT_TRANSFER_ADAPTER=/absolute/path/to/approved-adapter
./scripts/data-profile.sh \
  --confirm-refresh I_AUTHORIZE_PRODUCTION_SNAPSHOT_READ_TRANSFER_AND_SANITIZATION \
  snapshot refresh
```

适配器必须是绝对、可执行、非符号链接的普通文件；统一命令会把自己创建的空暂存目录作为最后一个参数传入，适配器只负责将 `magicpodcast.db` 与 `manifest.json` 写入该目录，不输出路径或敏感内容。凭据必须来自受保护配置，不能放入命令参数、日志、清单或 Issue。统一命令会在成功、校验失败或传输失败后清理自己创建的暂存目录。`--transfer-dir` 仅用于已准备好的本地人工交接和自动化测试，其来源目录始终由交接方管理，不会被自动删除。

脱敏规则版本 `v9`：

- 清空 `sync_configs`；
- 替换节目 Feed URL，清空节目/单集音频 URL、备用 Feed URL、工作流自定义源与私人 LLM 提示；
- 清空节目/单集私人备注、私有封面、报告 LLM 错误和执行日志；
- 移除 Show Notes、报告正文及结构化报告链接中的 URL 凭据、查询参数和片段，保留链接主体；
- 删除持久化 Feed 正文；
- 删除加工运行、定时调度历史、供应商检查点、本地产物路径和外部交付身份；数据库快照不携带对应文件，不能保留失真的加工状态或调度历史；
- 对已审查的生产历史残留结构做精确匹配后从副本移除；同名但形状不同的结构仍失败关闭；
- 搜索 FTS 仅在结构与触发器完全匹配已审查版本时允许，并在正文脱敏后重建和复验；
- 对当前 schema 之外的任何新表、新字段、隐藏/生成字段、索引、视图或触发器失败关闭，不依赖名称猜测；先评审规则再刷新。

刷新成功只更新 `latest`，不会切换当前 Profile。默认保留最近 3 份，并保护当前使用中和最新成功的快照。过期快照先原子移入隐藏隔离区，提交后再物理清理；清理失败仅留下脚本拥有的隐藏垃圾目录，不回滚已提交快照。任何导出、脱敏、传输、校验或发布失败都不会改动当前服务或既有 `latest`。

## 故障处理

- `status` 显示 `ready=false`：检查 Profile 管理目录中的 `runtime/backend.log`，再显式重试当前 Profile。
- Fixture/快照校验失败：原件会保留，不会自动重建或迁移；修复来源后重新执行。
- 无网络：继续使用当前 Fixture 或已有 Snapshot；系统不会虚构刷新成功。
- 端口被占用：命令拒绝切换；先人工确认并处理占用者。

真实生产导出、Mac mini 操作、传输、部署、迁移和生产服务切换始终需要单独授权。
