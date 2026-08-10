import { describe, expect, it } from "vitest";
import { reportThemeLabel } from "@/lib/discoveryReports";
import type { HomepageReport } from "@/types/discovery";

function makeReport(overrides: Partial<HomepageReport> = {}): HomepageReport {
  return {
    id: 1,
    job_id: 1,
    workflow_id: 1,
    workflow_name: "晨间日报",
    report_type: "daily",
    title: "晨间日报 - 2026-08-10 08:00:00",
    completed_at: "2026-08-10T08:00:00Z",
    generated_at: "2026-08-10T08:00:00Z",
    episode_count: 1,
    episodes: [
      {
        episode_id: 1,
        order: 1,
        podcast_id: 1,
        podcast_title: "节目",
        episode_title: "AI 组织变革进入落地期",
        decision_state: "pending",
      },
    ],
    ...overrides,
  };
}

describe("reportThemeLabel", () => {
  it("prefers the report-authored theme", () => {
    expect(reportThemeLabel(makeReport({ theme: "本周关注 DeepSeek 开源" }))).toBe(
      "本周关注 DeepSeek 开源",
    );
  });

  it("does not repeat a generated workflow title as the theme", () => {
    expect(reportThemeLabel(makeReport())).toBe("AI 组织变革进入落地期");
  });

  it("marks a first-episode fallback as a multi-item selection", () => {
    const report = makeReport();
    expect(
      reportThemeLabel({
        ...report,
        episode_count: 2,
        episodes: [
          ...report.episodes,
          {
            ...report.episodes[0],
            episode_id: 2,
            order: 2,
            episode_title: "第二条",
          },
        ],
      }),
    ).toBe("AI 组织变革进入落地期 等精选");
  });
});
