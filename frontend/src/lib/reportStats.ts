export type ReportAIStatus =
  | "generated"
  | "not_generated"
  | "incomplete"
  | "disabled"
  | "not_needed"
  | "unknown";

export type ReportFieldStatus = "known" | "unknown" | "unused";

export interface ReportStats {
  podcasts_count: number;
  episodes_count: number;
  ai_status: ReportAIStatus | string;
  ai_status_label: string;
  model?: string;
  model_status: ReportFieldStatus | string;
  tokens?: number | null;
  tokens_status: ReportFieldStatus | string;
  line: string;
}

const SYSTEM_EXEC_LINE = /^>\s*\*\*🕐 执行\*\*/;
const SYSTEM_FEED_LINE = /^>\s*\*\*📡 Feed覆盖\*\*/;

export function stripReportSystemMetadata(content: string): string {
  if (!content) return content;
  const lines = content.split("\n");
  const kept: string[] = [];
  for (const line of lines) {
    const trimmed = line.trim();
    if (SYSTEM_EXEC_LINE.test(trimmed) || SYSTEM_FEED_LINE.test(trimmed)) {
      continue;
    }
    kept.push(line);
  }
  return kept.join("\n");
}
