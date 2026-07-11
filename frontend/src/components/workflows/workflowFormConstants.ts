/**
 * 工作流表单相关常量和工具函数
 */

// 预设的Cron表达式（6位格式：秒 分 时 日 月 周）
export const CRON_PRESETS = [
  { label: "每天凌晨2点", value: "0 0 2 * * *" },
  { label: "每天早上8点", value: "0 0 8 * * *" },
  { label: "每天晚上8点", value: "0 0 20 * * *" },
  { label: "每周日凌晨2点", value: "0 0 2 * * 0" },
  { label: "每周一早上6点", value: "0 0 6 * * 1" },
];

// 默认User Prompt模板
export const DEFAULT_USER_PROMPT = `# 工作流执行报告

工作流名称: {{.WorkflowName}}
匹配的单集总数: {{.TotalEpisodes}}
节目数量: {{.NumPodcasts}}

## 数据来源

{{range .Podcasts}}
### {{.PodcastTitle}}
单集数: {{len .Episodes}}
{{range .Episodes}}
- **{{.Title}}** ({{.PublishedDate.Format "2006-01-02"}})
  {{if ne .ShowNotes ""}}{{.ShowNotes}}{{end}}
{{end}}
{{end}}

## 分析要求

请按照以下维度生成分析报告：

### 1. 总体概览
简要描述本次抓取的整体情况（1-2句话）

### 2. 核心内容
按节目分类列出重要单集的要点：
- 理解播客节目的主题和内容风格
- 提取每期单集的核心观点和关键信息
- 识别跨节目的主题关联和趋势

### 3. 关键洞察
提炼3-5个关键主题或趋势，指出值得关注的亮点

## 输出格式要求

1. 统一使用bullet point（-），所有列表项不要使用数字序号
2. **核心内容部分必须使用嵌套bullet列表**，格式示例：
   - **AI前沿**（一级bullet）下面列出该节目的单集要点（二级bullet）
   - **高能量**（一级bullet）下面列出该节目的单集要点（二级bullet）
3. 列表项之间保持一个换行即可
4. 避免连续的多个空行
5. 用简洁专业的语言生成摘要，避免过度解读
6. 客观准确，不添加原文没有的信息
7. 简洁明了，避免冗余表述`;

/**
 * Cron表达式校验函数
 */
export function validateCronExpression(
  cronExpr: string,
): { valid: boolean; error?: string } {
  const trimmed = cronExpr.trim();
  const parts = trimmed.split(/\s+/);

  // 检查位数（支持5位或6位）
  if (parts.length !== 5 && parts.length !== 6) {
    return {
      valid: false,
      error: "Cron表达式必须包含5段或6段（秒 分 时 日 月 周）",
    };
  }

  // 检查每段是否为通配符、数字、范围、间隔或列表
  const validPatterns = [
    /^\*$/, // 通配符
    /^\d+$/, // 单个数字
    /^\d+-\d+$/, // 范围 (如 1-5)
    /^\*\/\d+$/, // 间隔 (如 */6)
    /^\d+(,\d+)+$/, // 列表 (如 1,3,5)
  ];

  for (let i = 0; i < parts.length; i++) {
    if (!validPatterns.some((pattern) => pattern.test(parts[i]))) {
      return {
        valid: false,
        error: `第 ${i + 1} 段 "${parts[i]}" 格式不正确（支持: * 数字 范围1-5 间隔*/6 列表1,3,5）`,
      };
    }
  }

  return { valid: true };
}
