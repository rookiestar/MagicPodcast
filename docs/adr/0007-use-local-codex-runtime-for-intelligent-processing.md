---
status: accepted
---

# 由 Mac mini 本地 Codex Runtime 承担智能加工

MagicPodcast 的摘要、资料关联、工具编排和问答统一交给 Mac mini 上的本地 Codex CLI/Runtime；MagicPodcast 自身只负责确定性的持久状态、幂等、调度、重试、取消、权限策略和审计。Runtime 通过可替换 Adapter 接入，每次运行使用受限工作目录和明确工具权限，浏览器不得直连 Codex。

稳定集成边界固定为 `openai-codex==0.147.0` Python SDK，而不是直接调用实验性 `codex app-server`。Go Runtime Host 每次执行启动一个受管 Python 进程，通过供应商中立的 `stdio` JSONL 交换执行、事件、取消和结构化结果；SDK 内部使用配套 CLI Runtime，并显式关闭实验 API。批处理使用结构化输出，交互复用 Turn/Stream/Interrupt；两者共享身份校验、进程隔离和定向清理。

“本地 Runtime”只表示控制面和工具执行位于 Mac mini，不表示模型推理或数据完全离线；启用后，内容可按配置发送给 OpenAI，以及白名单中的飞书和 Google。Codex 会话或进程内状态不得成为 MagicPodcast 的权威任务队列。

这一选择增加了本机 Python 运行环境和进程监督成本，换取稳定 SDK、统一 Skill/CLI 编排、权限治理和可替换性。直接 app-server 与 WebSocket 不作为生产边界。详细边界见 [Focus 自动化播客加工与单集助手 Spec](../research/FOCUS_AUTOMATED_PODCAST_PROCESSING_SPEC_2026-08-24.md)；运行方式见 [Codex Runtime Host Runbook](../runbooks/CODEX_RUNTIME_HOST.md)。
