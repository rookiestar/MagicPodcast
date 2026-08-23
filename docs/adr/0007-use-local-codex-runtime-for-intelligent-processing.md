---
status: accepted
---

# 由 Mac mini 本地 Codex Runtime 承担智能加工

MagicPodcast 的摘要、资料关联、工具编排和问答统一交给 Mac mini 上的本地 Codex CLI/Runtime；MagicPodcast 自身只负责确定性的持久状态、幂等、调度、重试、取消、权限策略和审计。Runtime 通过可替换 Adapter 接入，每次运行使用受限工作目录和明确工具权限，浏览器不得直连 Codex。

“本地 Runtime”只表示控制面和工具执行位于 Mac mini，不表示模型推理或数据完全离线；启用后，内容可按配置发送给 OpenAI，以及白名单中的飞书和 Google。批处理与交互问答可在 Adapter 内采用不同的官方 Codex 集成面，但不得把 Codex 会话或进程内状态当作 MagicPodcast 的权威任务队列。

这一选择牺牲了直接调用各家模型 API 的短期简单性，换取统一的 Skill/CLI 编排、权限治理和可替换性。详细边界见 [Focus 自动化播客加工与单集助手 Spec](../research/FOCUS_AUTOMATED_PODCAST_PROCESSING_SPEC_2026-08-24.md)；Codex app-server 的官方定位和协议见 [OpenAI 官方文档](https://learn.chatgpt.com/docs/app-server)。
