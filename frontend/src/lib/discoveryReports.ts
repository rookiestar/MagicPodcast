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
    case "custom":
      return "自定义周期";
    default:
      return reportType || "报告";
  }
}

export function reportTypeClassName(reportType: string): string {
  switch (reportType) {
    case "weekly":
      return "is-weekly";
    case "custom":
      return "is-custom";
    default:
      return "is-daily";
  }
}

function reportDate(value: string): Date | null {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

function reportTimeZone(timeZone?: string) {
  return timeZone ? { timeZone } : {};
}

export function reportDayKey(value: string, timeZone?: string): string {
  const date = reportDate(value);
  if (!date) return value;

  const parts = new Intl.DateTimeFormat("zh-CN", {
    ...reportTimeZone(timeZone),
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(date);
  const read = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((part) => part.type === type)?.value ?? "";

  return `${read("year")}-${read("month")}-${read("day")}`;
}

export function formatReportDay(value: string, timeZone?: string): string {
  const date = reportDate(value);
  if (!date) return value;

  const dateLabel = new Intl.DateTimeFormat("zh-CN", {
    ...reportTimeZone(timeZone),
    year: "numeric",
    month: "long",
    day: "numeric",
  }).format(date);
  const weekday = new Intl.DateTimeFormat("zh-CN", {
    ...reportTimeZone(timeZone),
    weekday: "short",
  }).format(date);

  return `${dateLabel} · ${weekday}`;
}

export function formatReportTime(value: string, timeZone?: string): string {
  const date = reportDate(value);
  if (!date) return value;

  return new Intl.DateTimeFormat("zh-CN", {
    ...reportTimeZone(timeZone),
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(date);
}

export function formatReportDate(value: string, timeZone?: string): string {
  const date = reportDate(value);
  if (!date) return value;

  return new Intl.DateTimeFormat("zh-CN", {
    ...reportTimeZone(timeZone),
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(date);
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
