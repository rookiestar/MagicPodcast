# Codex Runtime Host Runbook

最后更新：2026-08-24

本 Runbook 说明 #180 的仓库实现与验证方式。它不授权生产部署、数据库迁移、真实音频上传或凭据变更。

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
- Shell、文件写入、外部工具、子代理和未知工具默认拒绝；助手只可显式声明 `web_search`。
- SDK/Runtime 版本、认证、工作目录或必需能力缺失时返回 `runtime_unavailable`。
- 事件必须携带唯一 execution identity 和连续序号；缺失、冲突、乱序、超限或终态后继续输出均标记 `runtime_protocol_error` 并定向清理。
- Go 只传递 HOME、PATH、区域、证书和 Codex 配置目录等白名单环境变量；提示词、结果、凭据和完整私有路径不得进入诊断输出。

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
2. `runtime_host.py` 的 `EXPECTED_SDK_VERSION`；
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

## 6. 当前未启用项

- 生产 Worker 尚未实例化 Runtime Host 或 Processing Adapter。
- 未配置生产日志、资源上限、LaunchAgent 或重启恢复。
- 未安装或配置飞书、Google 生产能力。
- 未执行数据库 migration apply、部署或真实数据运行。
