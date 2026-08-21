import { describe, expect, it } from "vitest";
import {
  buildWorkflowFilterOptions,
  filterReportsByWorkflowSelection,
  normalizeWorkflowFilterKeyword,
  workflowOptionMatchesKeyword,
} from "@/lib/homepageReportWorkflowFilter";
import type { HomepageReport } from "@/types/discovery";

function makeReport(
  overrides: Pick<
    HomepageReport,
    "id" | "workflow_id" | "workflow_name" | "completed_at"
  > &
    Partial<HomepageReport>,
): HomepageReport {
  return {
    job_id: overrides.id,
    report_type: "daily",
    title: `${overrides.workflow_name} title`,
    content: "",
    generated_at: overrides.completed_at,
    episode_count: 0,
    episodes: [],
    ...overrides,
  };
}

describe("buildWorkflowFilterOptions", () => {
  it("aggregates options by workflow_id with report counts", () => {
    const options = buildWorkflowFilterOptions([
      makeReport({
        id: 1,
        workflow_id: 10,
        workflow_name: "科技日报",
        completed_at: "2026-08-11T08:00:00Z",
      }),
      makeReport({
        id: 2,
        workflow_id: 10,
        workflow_name: "科技日报",
        completed_at: "2026-08-10T08:00:00Z",
      }),
      makeReport({
        id: 3,
        workflow_id: 20,
        workflow_name: "投资周报",
        completed_at: "2026-08-12T08:00:00Z",
      }),
    ]);

    expect(options).toHaveLength(2);
    expect(options.map((option) => option.workflowId)).toEqual([20, 10]);
    const tech = options.find((option) => option.workflowId === 10);
    expect(tech?.reportCount).toBe(2);
    expect(tech?.label).toBe("科技日报");
    expect(tech?.names).toEqual(["科技日报"]);
    expect(tech?.latestCompletedAt).toBe("2026-08-11T08:00:00Z");
  });

  it("keeps one option across renames and picks the latest name as label", () => {
    const options = buildWorkflowFilterOptions([
      makeReport({
        id: 1,
        workflow_id: 10,
        workflow_name: "旧名称",
        completed_at: "2026-08-09T08:00:00Z",
      }),
      makeReport({
        id: 2,
        workflow_id: 10,
        workflow_name: "新名称",
        completed_at: "2026-08-11T08:00:00Z",
      }),
      makeReport({
        id: 3,
        workflow_id: 10,
        workflow_name: "旧名称",
        completed_at: "2026-08-10T08:00:00Z",
      }),
    ]);

    expect(options).toHaveLength(1);
    expect(options[0].label).toBe("新名称");
    expect(options[0].names).toEqual(["旧名称", "新名称"]);
    expect(options[0].latestCompletedAt).toBe("2026-08-11T08:00:00Z");
    expect(options[0].reportCount).toBe(3);
  });

  it("orders workflows by latest output, newest first", () => {
    const options = buildWorkflowFilterOptions([
      makeReport({
        id: 1,
        workflow_id: 30,
        workflow_name: "较早工作流",
        completed_at: "2026-08-01T08:00:00Z",
      }),
      makeReport({
        id: 2,
        workflow_id: 10,
        workflow_name: "最新工作流",
        completed_at: "2026-08-12T08:00:00Z",
      }),
      makeReport({
        id: 3,
        workflow_id: 20,
        workflow_name: "居中工作流",
        completed_at: "2026-08-06T08:00:00Z",
      }),
    ]);

    expect(options.map((option) => option.label)).toEqual([
      "最新工作流",
      "居中工作流",
      "较早工作流",
    ]);
  });

  it("treats invalid completion dates as the oldest with stable ordering", () => {
    const options = buildWorkflowFilterOptions([
      makeReport({
        id: 1,
        workflow_id: 30,
        workflow_name: "坏时间工作流",
        completed_at: "not-a-date",
      }),
      makeReport({
        id: 2,
        workflow_id: 10,
        workflow_name: "科技日报",
        completed_at: "2026-08-12T08:00:00Z",
      }),
      makeReport({
        id: 3,
        workflow_id: 40,
        workflow_name: "另一个坏时间",
        completed_at: "",
      }),
    ]);

    // Invalid dates count as 0 (oldest) and keep the id tiebreak stable.
    expect(options.map((option) => option.workflowId)).toEqual([10, 30, 40]);
    expect(options[0].latestCompletedAt).toBe("2026-08-12T08:00:00Z");
  });
});

describe("normalizeWorkflowFilterKeyword", () => {
  it("trims surrounding whitespace and lowercases", () => {
    expect(normalizeWorkflowFilterKeyword("  Foo BAR  ")).toBe("foo bar");
    expect(normalizeWorkflowFilterKeyword("　全角空格　")).toBe("全角空格");
  });
});

describe("workflowOptionMatchesKeyword", () => {
  const option = {
    workflowId: 10,
    label: "新名称",
    names: ["旧名称", "新名称"],
    latestCompletedAt: "2026-08-11T08:00:00Z",
    reportCount: 2,
  };

  it("matches the latest label and historical names, case-insensitively", () => {
    expect(workflowOptionMatchesKeyword(option, "新名")).toBe(true);
    expect(workflowOptionMatchesKeyword(option, "旧名")).toBe(true);
    expect(workflowOptionMatchesKeyword(option, "  新名称 ")).toBe(true);
  });

  it("treats empty or whitespace-only keywords as matching everything", () => {
    expect(workflowOptionMatchesKeyword(option, "")).toBe(true);
    expect(workflowOptionMatchesKeyword(option, "   ")).toBe(true);
  });

  it("does not match unrelated keywords", () => {
    expect(workflowOptionMatchesKeyword(option, "不存在")).toBe(false);
  });
});

describe("filterReportsByWorkflowSelection", () => {
  const reports = [
    makeReport({
      id: 1,
      workflow_id: 10,
      workflow_name: "科技日报",
      completed_at: "2026-08-11T08:00:00Z",
    }),
    makeReport({
      id: 2,
      workflow_id: 20,
      workflow_name: "投资周报",
      completed_at: "2026-08-10T08:00:00Z",
    }),
    makeReport({
      id: 3,
      workflow_id: 10,
      workflow_name: "科技日报",
      completed_at: "2026-08-09T08:00:00Z",
    }),
  ];

  it("returns all reports when the selection is empty", () => {
    expect(filterReportsByWorkflowSelection(reports, new Set())).toEqual(
      reports,
    );
  });

  it("keeps only the selected workflow for a single selection", () => {
    const filtered = filterReportsByWorkflowSelection(reports, new Set([10]));
    expect(filtered.map((report) => report.id)).toEqual([1, 3]);
  });

  it("applies OR semantics across multiple selections", () => {
    const filtered = filterReportsByWorkflowSelection(
      reports,
      new Set([10, 20]),
    );
    expect(filtered.map((report) => report.id)).toEqual([1, 2, 3]);
  });

  it("returns nothing when the selection does not exist in the data", () => {
    const filtered = filterReportsByWorkflowSelection(reports, new Set([99]));
    expect(filtered).toEqual([]);
  });
});
