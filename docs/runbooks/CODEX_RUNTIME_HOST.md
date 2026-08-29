# Codex Runtime Host Runbook

最后更新：2026-08-29

本 Runbook 说明 #180 的 Runtime 实现，以及 #206 后 Focus 加工如何直接发布飞书妙记原生产物。Runtime 继续服务其他获批用途；新转写管道不再调用它生成第二份纪要。本文件不授权生产部署、数据库迁移、真实音频上传或凭据变更。

## 1. 固定边界

```text
Processing / Assistant
  -> provider-neutral Go Runtime Module
  -> one managed Python Host per execution
  -> openai-codex==0.147.0
  -> bundled Codex CLI Runtime 0.147.0
```

- Go 与 Python 只通过版本化 `stdio` JSONL 通信，不使用 WebSocket。
- 直接 `codex app-server` 命令是实验性能力，不作为生产边界。
- Python 必须显式使用 `CodexConfig(experimental_api=False)`。
- 每个执行有独立 identity、受管工作目录、事件流、取消句柄和进程组。
- 数据库加工运行是权威状态；Runtime Snapshot 只描述当前本机执行。

## 2. 权限与失败关闭

- 固定 `ApprovalMode.deny_all`。
- 默认 `read_only`；只有受管写入类型可声明 `workspace_write`；拒绝 `full_access`。
- 每次执行使用临时 `CODEX_HOME`/`HOME`，只链接主机 `auth.json`；不继承用户配置、MCP、Plugin 或 Skill，结束后删除临时状态。
- Shell、文件写入、图片、外部工具、Plugin、Skill 和子代理在模型获得工具目录前关闭；助手只可显式声明 `web_search`。`item/started` 检查继续作为纵深防御。
- SDK 与 CLI Runtime 都必须匹配 `0.147.0`；版本、文件认证、工作目录或必需能力缺失时返回 `runtime_unavailable`。
- 事件必须携带唯一 execution identity 和连续序号；缺失、冲突、乱序、超限或终态后继续输出均标记 `runtime_protocol_error` 并定向清理。
- 主机 Codex 登录必须提供受限权限的 `auth.json`；Runtime 可把令牌刷新写回该文件，但不得读取同目录其他配置。
- Go 只传递 PATH、区域、证书、临时目录和 Codex 认证位置等白名单环境变量；提示词、结果、凭据和完整私有路径不得进入诊断输出。
- Go Host 默认只保留最近 256 个已终结且进程已关闭的执行；活动执行不淘汰，数据库加工运行仍是长期权威记录。

## 3. 取消与清理

1. 执行观察到首个真实 SDK 事件后才报告 `running`。
2. 取消先调用目标 Turn 的原生 `interrupt`。
3. 原生取消超时后向目标进程组发送 `SIGTERM`。
4. 仍未退出才发送 `SIGKILL`。
5. Snapshot 记录 `native_interrupt`、`sigterm` 或 `sigkill`。
6. 父进程 EOF、Host Close、协议错误和异常退出使用同一目标进程组收口，不影响其他执行。

## 4. 安装与升级

隔离环境必须使用 Python 3.10+；当前 Mac mini 使用 Python 3.12。

```bash
python3.12 -m venv /absolute/path/venv-0.147.0
/absolute/path/venv-0.147.0/bin/python -m pip install \
  --disable-pip-version-check --no-input \
  -r backend/internal/codexruntime/requirements.txt
```

升级必须同时修改并验证：

1. `requirements.txt` 的固定 SDK 版本；
2. `runtime_host.py` 的 SDK 与 Runtime 固定版本；
3. Fake conformance 与 Python SDK Host 测试；
4. Mac mini 脱敏 Smoke；
5. Spec、ADR 和兼容性说明。

不能在生产环境浮动升级 SDK 或配套 CLI Runtime。

## 5. 脱敏 Smoke

先在隔离目录构建 `backend/cmd/codex-runtime-smoke`，再执行：

```bash
codex-runtime-smoke \
  --python /absolute/path/venv-0.147.0/bin/python \
  --host-script /absolute/path/runtime_host.py \
  --work-root /absolute/path/smoke-work \
  --evidence /absolute/path/evidence.json \
  --timeout 5m
```

Smoke 必须证明：

- 一次执行完成并产生流式文本和结构化结果；
- 另一次执行通过原生 `interrupt` 取消；
- 两次 identity 不同；
- 事件序号连续；
- Host 关闭后活动执行和存活进程组均为 0。

证据只保存主机、版本、状态、时延、事件计数、取消方式和清理结果，不保存提示词、输出正文、账号、路径或凭据。当前证据见 [`CODEX_RUNTIME_SMOKE_2026-08-24.json`](../research/evidence/CODEX_RUNTIME_SMOKE_2026-08-24.json)。

## 6. 飞书原生产物真实 Smoke

`backend/cmd/processing-real-smoke` 运行 #206 的真实 Adapter 链：读取同一妙记的 Summary、Transcript 和结构化时间轴，再原子发布本地产物；不调用 Codex Runtime。它必须从与待验收提交一致的 Git worktree 构建；最终证据的 `build.vcs_revision` 必须等于该提交，`build.vcs_modified` 必须为 `false`。

首次运行会创建外部资源，只有在单独授权真实音频上传和用户凭据后才能执行：

```bash
commit=$(git rev-parse HEAD)
(cd backend && go build -buildvcs=false \
  -ldflags "-X main.buildRevision=${commit} -X main.buildModified=false" \
  -o /absolute/path/processing-real-smoke ./cmd/processing-real-smoke)
/absolute/path/processing-real-smoke \
  --audio /absolute/path/episode.m4a \
  --lark-cli /absolute/path/lark-cli-isolated \
  --lark-work-root /absolute/path/lark-work \
  --artifact-root /absolute/path/artifacts \
  --evidence /absolute/path/processing-real-smoke.json \
  --timeout 2h
```

重启恢复验收使用已有 `lark-work/smoke-checkpoint.json` 时必须加 `--resume-only`；该模式不调用新的 Drive/Minutes 上传。证据应回读 `events`、`build`、Summary/Transcript 字节数、段落数和全部产物校验值，并明确是否为首次上传或恢复运行。现存 [`FOCUS_PROCESSING_REAL_SMOKE_2026-08-25.json`](../research/evidence/FOCUS_PROCESSING_REAL_SMOKE_2026-08-25.json) 仅是旧 #181 管道证据，不能替代 #206 的另行授权真实验收。

## 7. 当前未启用项

- 仓库已支持在 `processing.enabled=true` 时显式实例化飞书 Adapter 与持久 Worker；Runtime Host 仍可供其他能力使用。默认仍关闭，生产尚未启用。
- Worker 已支持进程重启后的持久运行恢复；生产资源上限、LaunchAgent 配置和运行证据尚未完成。
- 飞书 Adapter 已实现；真实 Smoke 仅作为获批测试音频的隔离验收，不等于生产 Worker 已启用；Google 能力不属于 #181。
- 未执行数据库 migration apply、部署或真实数据运行。
