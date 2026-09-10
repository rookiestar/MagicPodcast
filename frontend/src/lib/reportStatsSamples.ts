import type { ReportStats } from "@/lib/reportStats";

export const reportStatsSamples: Array<{
  name: string;
  stats: ReportStats;
  content: string;
}> = [
  {
    name: "generated",
    stats: {
      podcasts_count: 3,
      episodes_count: 3,
      ai_status: "generated",
      ai_status_label: "AI 已生成",
      model: "deepseek-v4-flash",
      model_status: "known",
      tokens: 7245,
      tokens_status: "known",
      line: "3 个节目 · 3 集 · AI 已生成 · deepseek-v4-flash · 7.2K Token",
    },
    content:
      "# 标题\n\n## 🤖 AI智能摘要\n\n短摘要\n\n---\n\n> **🕐 执行**: 2026-09-10 08:00:00 | **⏱️ 窗口**: 7天 (cron) | **📊 统计**: 3节目/3单集 | **🤖 AI**: 7.2K (deepseek-v4-flash)\n\n> **📡 Feed覆盖**: 3/3 已尝试\n\n> 用户普通引用块\n\n单集详情",
  },
  {
    name: "not_generated",
    stats: {
      podcasts_count: 3,
      episodes_count: 3,
      ai_status: "not_generated",
      ai_status_label: "AI 未生成",
      model: "deepseek-v4-flash",
      model_status: "known",
      tokens: 7245,
      tokens_status: "known",
      line: "3 个节目 · 3 集 · AI 未生成 · deepseek-v4-flash · 7.2K Token",
    },
    content: "# 标题\n\n> 用户普通引用块\n\n单集详情",
  },
  {
    name: "incomplete",
    stats: {
      podcasts_count: 1,
      episodes_count: 2,
      ai_status: "incomplete",
      ai_status_label: "AI 不完整",
      model: "deepseek-v4-flash",
      model_status: "known",
      tokens: 120,
      tokens_status: "known",
      line: "1 个节目 · 2 集 · AI 不完整 · deepseek-v4-flash · 120 Token",
    },
    content: "# 标题\n\n半份结论\n",
  },
  {
    name: "disabled",
    stats: {
      podcasts_count: 2,
      episodes_count: 4,
      ai_status: "disabled",
      ai_status_label: "AI 未启用",
      model_status: "unused",
      tokens: null,
      tokens_status: "unused",
      line: "2 个节目 · 4 集 · AI 未启用 · 未调用 · —",
    },
    content: "# 标题\n\n单集详情",
  },
  {
    name: "not_needed",
    stats: {
      podcasts_count: 0,
      episodes_count: 0,
      ai_status: "not_needed",
      ai_status_label: "AI 无需生成",
      model_status: "unused",
      tokens: null,
      tokens_status: "unused",
      line: "0 个节目 · 0 集 · AI 无需生成 · 未调用 · —",
    },
    content: "# 标题\n\n未匹配到单集",
  },
  {
    name: "unknown",
    stats: {
      podcasts_count: 3,
      episodes_count: 3,
      ai_status: "unknown",
      ai_status_label: "AI 状态未知",
      model_status: "unknown",
      tokens: null,
      tokens_status: "unknown",
      line: "3 个节目 · 3 集 · AI 状态未知 · 模型未知 · Token 未知",
    },
    content: "# 标题\n\n> 用户普通引用块\n",
  },
  {
    name: "long-summary",
    stats: {
      podcasts_count: 3,
      episodes_count: 3,
      ai_status: "generated",
      ai_status_label: "AI 已生成",
      model: "deepseek-v4-flash",
      model_status: "known",
      tokens: 12,
      tokens_status: "known",
      line: "3 个节目 · 3 集 · AI 已生成 · deepseek-v4-flash · 12 Token",
    },
    content: `# 标题\n\n${"很长的摘要段落。".repeat(40)}\n\n> 用户普通引用块\n`,
  },
];
