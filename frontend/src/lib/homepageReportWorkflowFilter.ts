import type { HomepageReport } from "@/types/discovery";

/**
 * Workflow filter option derived from the homepage history metadata (#144).
 * Identity is the stable workflow_id; names cover every name the workflow
 * used inside the loaded window so renames stay searchable.
 */
export interface WorkflowFilterOption {
  workflowId: number;
  /** Name of the workflow's most recent report in the window. */
  label: string;
  /** Distinct names seen in the window (label included), first-seen order. */
  names: string[];
  latestCompletedAt: string;
  reportCount: number;
}

function completedTime(report: HomepageReport): number {
  const time = new Date(report.completed_at).getTime();
  return Number.isNaN(time) ? 0 : time;
}

export function normalizeWorkflowFilterKeyword(raw: string): string {
  return raw.trim().toLowerCase();
}

/** Aggregate options by workflow_id, newest output first (#144). */
export function buildWorkflowFilterOptions(
  reports: HomepageReport[],
): WorkflowFilterOption[] {
  const byId = new Map<
    number,
    {
      label: string;
      latestCompletedAt: string;
      latestTime: number;
      names: string[];
      nameSet: Set<string>;
      reportCount: number;
    }
  >();

  for (const report of reports) {
    const entry = byId.get(report.workflow_id) ?? {
      label: "",
      latestCompletedAt: "",
      latestTime: -1,
      names: [],
      nameSet: new Set<string>(),
      reportCount: 0,
    };
    const name = report.workflow_name?.trim() || `工作流 ${report.workflow_id}`;
    if (!entry.nameSet.has(name)) {
      entry.nameSet.add(name);
      entry.names.push(name);
    }
    entry.reportCount += 1;
    const time = completedTime(report);
    if (time > entry.latestTime) {
      entry.latestTime = time;
      entry.latestCompletedAt = report.completed_at;
      entry.label = name;
    }
    byId.set(report.workflow_id, entry);
  }

  return [...byId.entries()]
    .map(([workflowId, entry]) => ({
      workflowId,
      label: entry.label,
      names: entry.names,
      latestCompletedAt: entry.latestCompletedAt,
      reportCount: entry.reportCount,
    }))
    .sort((a, b) => {
      const byTime =
        new Date(b.latestCompletedAt).getTime() -
        new Date(a.latestCompletedAt).getTime();
      return byTime !== 0 ? byTime : a.workflowId - b.workflowId;
    });
}

/** Keyword matches when any name seen in the window contains it (#144). */
export function workflowOptionMatchesKeyword(
  option: WorkflowFilterOption,
  rawKeyword: string,
): boolean {
  const keyword = normalizeWorkflowFilterKeyword(rawKeyword);
  if (!keyword) return true;
  return option.names.some((name) =>
    normalizeWorkflowFilterKeyword(name).includes(keyword),
  );
}

/** Empty selection means no filtering; otherwise OR across selected ids (#144). */
export function filterReportsByWorkflowSelection(
  reports: HomepageReport[],
  selectedWorkflowIds: ReadonlySet<number>,
): HomepageReport[] {
  if (selectedWorkflowIds.size === 0) return reports;
  return reports.filter((report) =>
    selectedWorkflowIds.has(report.workflow_id),
  );
}
