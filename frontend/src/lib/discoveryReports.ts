import { apiClient } from "@/lib/fetcher";
import type { HomepageReport, HomepageReportsData } from "@/types/discovery";

export const DISCOVERY_REPORTS_PATH = "/api/v1/discovery/reports";

/** First paint uses a bounded history metadata list — not 30 full Markdown bodies (#95). */
export const HOMEPAGE_HISTORY_METADATA_LIMIT = 30;

export async function fetchHomepageReports(
  historyLimit = HOMEPAGE_HISTORY_METADATA_LIMIT,
): Promise<HomepageReportsData> {
  const response = await apiClient.get<{
    success: boolean;
    data: HomepageReportsData;
  }>(`${DISCOVERY_REPORTS_PATH}?history_limit=${historyLimit}`);
  if (!response.data.success || !response.data.data) {
    throw new Error("Failed to load homepage reports");
  }
  return response.data.data;
}

/** On-demand full body for a history (or any) publishable report (#95). */
export async function fetchHomepageReportDetail(
  reportID: number,
): Promise<HomepageReport> {
  const response = await apiClient.get<{
    success: boolean;
    data: HomepageReport;
  }>(`${DISCOVERY_REPORTS_PATH}/${reportID}`);
  if (!response.data.success || !response.data.data) {
    throw new Error("Failed to load homepage report detail");
  }
  return response.data.data;
}

export function reportTypeLabel(reportType: string): string {
  switch (reportType) {
    case "daily":
      return "日报";
    case "weekly":
      return "周报";
    default:
      return reportType || "报告";
  }
}

export function formatReportDate(value: string): string {
  if (!value) return "";
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

/** Theme/topic line for Banner cards, with compatibility fallbacks for old APIs. */
export function reportThemeLabel(report: HomepageReport): string {
  const theme = report.theme?.trim();
  if (theme) return theme;

  const title = report.title?.trim();
  const workflowName = report.workflow_name?.trim();
  if (
    title &&
    title !== workflowName &&
    !title.startsWith(`${workflowName} - `)
  ) {
    return title;
  }

  const firstEpisodeTitle = report.episodes[0]?.episode_title?.trim();
  if (firstEpisodeTitle) {
    return report.episodes.length > 1
      ? `${firstEpisodeTitle} 等精选`
      : firstEpisodeTitle;
  }

  return report.workflow_name || "工作流报告";
}
