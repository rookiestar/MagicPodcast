import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi, beforeEach } from "vitest";
import WorkflowReportWorkbench from "@/components/discovery/WorkflowReportWorkbench";
import type { HomepageReport } from "@/types/discovery";

const fetchDetailMock = vi.hoisted(() => vi.fn());

vi.mock("@/components/workflows/MarkdownViewer", () => ({
  default: ({ content }: { content: string }) => (
    <div data-testid="markdown-body">{content}</div>
  ),
}));

vi.mock("@/lib/discoveryReports", async () => {
  const actual = await vi.importActual<typeof import("@/lib/discoveryReports")>(
    "@/lib/discoveryReports",
  );
  return {
    ...actual,
    fetchHomepageReportDetail: (...args: unknown[]) => fetchDetailMock(...args),
  };
});

function makeReport(
  overrides: Partial<HomepageReport> &
    Pick<HomepageReport, "id" | "workflow_name">,
): HomepageReport {
  return {
    job_id: overrides.id,
    workflow_id: overrides.id,
    report_type: "daily",
    title: `${overrides.workflow_name} title`,
    content: `# ${overrides.workflow_name}\n\n正文内容`,
    completed_at: "2026-08-10T08:00:00Z",
    generated_at: "2026-08-10T08:00:00Z",
    episode_count: overrides.episodes?.length ?? 1,
    episodes: overrides.episodes ?? [
      {
        episode_id: overrides.id * 10,
        order: 1,
        podcast_id: 1,
        podcast_title: "节目",
        episode_title: `单集 ${overrides.id}`,
        recommendation: "",
        context: "节目上下文",
        decision_state: "pending",
      },
    ],
    ...overrides,
  };
}

describe("WorkflowReportWorkbench", () => {
  beforeEach(() => {
    fetchDetailMock.mockReset();
  });

  it("renders the editorial heading and a single report without switch controls", () => {
    render(
      <WorkflowReportWorkbench
        todayReports={[makeReport({ id: 1, workflow_name: "晨间日报" })]}
      />,
    );

    expect(
      screen.getByRole("region", { name: "精选报告" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "精选报告", level: 2 }),
    ).toBeInTheDocument();
    expect(screen.getByText("CURATED REPORTS")).toBeInTheDocument();
    expect(
      screen.queryByRole("listbox", { name: "当天报告" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("group", { name: "切换当天报告" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("1 / 1")).not.toBeInTheDocument();
    expect(screen.getByText("晨间日报")).toBeInTheDocument();
    expect(screen.getByText("日报")).toHaveClass("is-daily");

    const markdownSections = screen.getAllByTestId("markdown-body");
    expect(markdownSections).toHaveLength(2);
    expect(markdownSections[0]).toHaveTextContent("# 晨间日报");
    expect(markdownSections[1]).toHaveTextContent("正文内容");

    const episode = screen.getByRole("article");
    expect(
      markdownSections[0].compareDocumentPosition(episode) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      episode.compareDocumentPosition(markdownSections[1]) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("keeps a zero-episode report as one complete markdown document", () => {
    render(
      <WorkflowReportWorkbench
        todayReports={[
          makeReport({
            id: 10,
            workflow_name: "空报告",
            episode_count: 0,
            episodes: [],
            content: "# 空报告\n\n完整正文",
          }),
        ]}
      />,
    );

    expect(screen.queryByRole("article")).not.toBeInTheDocument();
    expect(screen.getAllByTestId("markdown-body")).toHaveLength(1);
    expect(screen.getByTestId("markdown-body")).toHaveTextContent(
      "# 空报告 完整正文",
    );
  });

  it("shows every report from the latest historical day when today is empty", async () => {
    const firstMetadata = makeReport({
      id: 9,
      workflow_name: "科技日报",
      metadata_only: true,
      content: "",
      completed_at: "2026-08-10T08:00:00Z",
    });
    const secondMetadata = makeReport({
      id: 8,
      workflow_name: "投资日报",
      metadata_only: true,
      content: "",
      completed_at: "2026-08-10T07:00:00Z",
    });
    const olderMetadata = makeReport({
      id: 7,
      workflow_name: "更早报告",
      metadata_only: true,
      content: "",
      completed_at: "2026-08-09T08:00:00Z",
    });
    const firstDetail = makeReport({
      id: 9,
      workflow_name: "科技日报",
      content: "# 科技日报\n\n科技正文",
      metadata_only: false,
      completed_at: "2026-08-10T08:00:00Z",
    });
    const secondDetail = makeReport({
      id: 8,
      workflow_name: "投资日报",
      content: "# 投资日报\n\n投资正文",
      metadata_only: false,
      completed_at: "2026-08-10T07:00:00Z",
    });
    fetchDetailMock.mockImplementation((id: number) =>
      Promise.resolve(id === 9 ? firstDetail : secondDetail),
    );

    render(
      <WorkflowReportWorkbench
        todayReports={[]}
        historyReports={[firstMetadata, secondMetadata, olderMetadata]}
      />,
    );

    expect(
      screen.getByRole("region", { name: "精选报告" }),
    ).toBeInTheDocument();
    expect(screen.getByText("最新往期 · 1 / 2")).toBeInTheDocument();
    expect(fetchDetailMock).toHaveBeenCalledWith(9);
    await waitFor(() =>
      expect(screen.getByText("科技正文")).toBeInTheDocument(),
    );

    fireEvent.click(screen.getByRole("button", { name: "下一份报告" }));
    expect(screen.getByText("最新往期 · 2 / 2")).toBeInTheDocument();
    expect(fetchDetailMock).toHaveBeenCalledWith(8);
    await waitFor(() =>
      expect(screen.getByText("投资正文")).toBeInTheDocument(),
    );
    expect(screen.queryByText("更早报告")).not.toBeInTheDocument();
  });

  it("switches reports via compact header controls and collapses episodes", () => {
    render(
      <WorkflowReportWorkbench
        todayReports={[
          makeReport({
            id: 1,
            workflow_name: "早报",
            completed_at: "2026-08-10T09:00:00Z",
            episodes: [
              {
                episode_id: 11,
                order: 1,
                podcast_id: 1,
                podcast_title: "P",
                episode_title: "早报单集",
                context: "A",
                decision_state: "pending",
              },
            ],
          }),
          makeReport({
            id: 2,
            workflow_name: "午报",
            report_type: "weekly",
            completed_at: "2026-08-10T12:00:00Z",
            episodes: [
              {
                episode_id: 21,
                order: 1,
                podcast_id: 1,
                podcast_title: "P",
                episode_title: "午报单集",
                context: "B",
                decision_state: "pending",
              },
            ],
          }),
        ]}
      />,
    );

    fireEvent.click(screen.getByText("早报单集"));
    expect(screen.getByText("Show Notes")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "下一份报告" }));
    expect(screen.getByText("午报单集")).toBeInTheDocument();
    expect(screen.getByText("2 / 2")).toBeInTheDocument();
    expect(screen.getByText("周报")).toHaveClass("is-weekly");
    // Collapsed after switch.
    expect(screen.queryByText("B")).not.toBeInTheDocument();
  });

  it("keeps expand and collect as independent controls and shows report-authored rationale", async () => {
    const user = userEvent.setup();
    const onDecision = vi.fn(
      async (_episodeID: number, state: "pending" | "shortlisted") => ({
        state,
        decision_updated_at: "2026-08-10T12:00:00Z",
      }),
    );

    render(
      <WorkflowReportWorkbench
        todayReports={[
          makeReport({
            id: 3,
            workflow_name: "双集日报",
            episodes: [
              {
                episode_id: 31,
                order: 1,
                podcast_id: 1,
                podcast_title: "P1",
                episode_title: "第一集",
                recommendation: "不应展示的推荐依据",
                context: "Shownotes 一",
                decision_state: "pending",
              },
              {
                episode_id: 32,
                order: 2,
                podcast_id: 1,
                podcast_title: "P2",
                episode_title: "第二集",
                context: "Shownotes 二",
                decision_state: "pending",
              },
            ],
          }),
        ]}
        onDecision={onDecision}
      />,
    );

    const expandFirst = screen.getByRole("button", {
      name: /第一集/,
    });
    fireEvent.click(expandFirst);
    expect(screen.getByText("报告推荐")).toBeInTheDocument();
    expect(screen.getByText("不应展示的推荐依据")).toBeInTheDocument();
    expect(screen.getByText("Show Notes")).toBeInTheDocument();
    expect(screen.getByText("Shownotes 一")).toBeInTheDocument();

    const collect = screen.getAllByLabelText("收集到 Inbox")[0];
    expect(expandFirst.contains(collect)).toBe(false);

    collect.focus();
    await user.keyboard("{Enter}");
    await waitFor(() => {
      expect(onDecision).toHaveBeenCalledWith(31, "shortlisted");
    });
    // Expand state remains open; Discovery does not remove an existing queue.
    expect(screen.getByText("Shownotes 一")).toBeInTheDocument();
    expect(screen.getAllByLabelText("已在 Inbox")[0]).toBeDisabled();
    expect(onDecision).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /第二集/ }));
    expect(screen.getByText("Shownotes 一")).toBeInTheDocument();
    expect(screen.getByText("Shownotes 二")).toBeInTheDocument();
    expect(
      screen.getByText("这份报告未提供可核对的推荐理由。"),
    ).toBeInTheDocument();
  });

  it("preserves an existing Focus state instead of resetting it to Inbox", () => {
    const onDecision = vi.fn();
    render(
      <WorkflowReportWorkbench
        todayReports={[
          makeReport({
            id: 4,
            workflow_name: "Focus 日报",
            episodes: [
              {
                episode_id: 41,
                order: 1,
                podcast_id: 1,
                podcast_title: "P",
                episode_title: "已投入单集",
                decision_state: "shortlisted",
                queue_state: "focus",
              },
            ],
          }),
        ]}
        onDecision={onDecision}
      />,
    );

    const state = screen.getByRole("button", { name: "已在 Focus" });
    expect(state).toBeDisabled();
    fireEvent.click(state);
    expect(onDecision).not.toHaveBeenCalled();
  });

  it("loads history on demand and restores prior today index on back (#94/#95)", async () => {
    fetchDetailMock.mockResolvedValue(
      makeReport({
        id: 9,
        workflow_name: "上周周报",
        report_type: "weekly",
        completed_at: "2026-08-03T08:00:00Z",
        content: "# 往期全文\n",
        metadata_only: false,
      }),
    );

    render(
      <WorkflowReportWorkbench
        todayReports={[
          makeReport({ id: 1, workflow_name: "今日甲" }),
          makeReport({
            id: 2,
            workflow_name: "今日乙",
            completed_at: "2026-08-10T12:00:00Z",
          }),
        ]}
        historyReports={[
          makeReport({
            id: 9,
            workflow_name: "上周周报",
            report_type: "weekly",
            completed_at: "2026-08-03T08:00:00Z",
            metadata_only: true,
            content: "",
          }),
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "下一份报告" }));
    expect(screen.getByText("2 / 2")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    expect(
      screen.getByRole("dialog", { name: "往期报告" }),
    ).toBeInTheDocument();
    fireEvent.click(
      within(screen.getByRole("dialog", { name: "往期报告" })).getByText(
        "上周周报",
      ),
    );

    await waitFor(() => {
      expect(fetchDetailMock).toHaveBeenCalledWith(9);
    });
    await waitFor(() => {
      expect(screen.getByText("上周周报")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "回到今日" }));
    expect(screen.getByText("2 / 2")).toBeInTheDocument();
    expect(screen.getByText("单集 2")).toBeInTheDocument();
  });

  it("traps focus in the history drawer and closes on Escape (#94)", () => {
    render(
      <WorkflowReportWorkbench
        todayReports={[makeReport({ id: 1, workflow_name: "今日" })]}
        historyReports={[
          makeReport({
            id: 9,
            workflow_name: "往期",
            metadata_only: true,
            content: "",
          }),
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    expect(
      screen.getByRole("dialog", { name: "往期报告" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "关闭" })).toHaveFocus();

    fireEvent.keyDown(window, { key: "Escape" });
    expect(
      screen.queryByRole("dialog", { name: "往期报告" }),
    ).not.toBeInTheDocument();
  });

  it("groups history by the report timezone and keeps each day newest first", () => {
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
        todayReports={[makeReport({ id: 1, workflow_name: "今日" })]}
        historyReports={[
          makeReport({
            id: 12,
            workflow_name: "科技日报",
            completed_at: "2026-08-12T00:30:00Z",
            metadata_only: true,
            content: "",
          }),
          makeReport({
            id: 11,
            workflow_name: "投资日报",
            completed_at: "2026-08-11T23:30:00Z",
            metadata_only: true,
            content: "",
          }),
          makeReport({
            id: 10,
            workflow_name: "前日晚报",
            completed_at: "2026-08-11T15:30:00Z",
            metadata_only: true,
            content: "",
          }),
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    const latestDay = screen.getByRole("region", {
      name: "2026年8月12日 · 周三",
    });
    const priorDay = screen.getByRole("region", {
      name: "2026年8月11日 · 周二",
    });

    expect(within(latestDay).getByText("2 份")).toBeInTheDocument();
    expect(within(latestDay).getAllByRole("button")).toHaveLength(2);
    expect(within(latestDay).getByText("08:30")).toBeInTheDocument();
    expect(within(latestDay).getByText("07:30")).toBeInTheDocument();
    expect(within(priorDay).getByText("1 份")).toBeInTheDocument();
    expect(within(priorDay).getByText("23:30")).toBeInTheDocument();
  });

  it("shows compact retry feedback when report load fails", () => {
    const onRetry = vi.fn();
    render(
      <WorkflowReportWorkbench todayReports={[]} failed onRetry={onRetry} />,
    );
    expect(screen.getByText("精选报告暂时无法读取")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "重新尝试" }));
    expect(onRetry).toHaveBeenCalled();
  });

  it("shows a real report recommendation even when Show Notes are unavailable", () => {
    render(
      <WorkflowReportWorkbench
        todayReports={[
          makeReport({
            id: 6,
            workflow_name: "无简介日报",
            episodes: [
              {
                episode_id: 60,
                order: 1,
                podcast_id: 1,
                podcast_title: "P",
                episode_title: "无简介单集",
                link: "https://example.com/episode",
                recommendation: "不应展示",
                context: "",
                excerpt: "",
                decision_state: "pending",
              },
            ],
          }),
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /无简介单集/ }));
    expect(screen.queryByText("Show Notes")).not.toBeInTheDocument();
    expect(screen.getByText("报告推荐")).toBeInTheDocument();
    expect(screen.getByText("不应展示")).toBeInTheDocument();
    expect(screen.getByText("打开原单集")).toBeInTheDocument();
  });

  it("strips dangerous episode links from the expand detail", () => {
    render(
      <WorkflowReportWorkbench
        todayReports={[
          makeReport({
            id: 5,
            workflow_name: "链接日报",
            episodes: [
              {
                episode_id: 50,
                order: 1,
                podcast_id: 1,
                podcast_title: "P",
                episode_title: "危险链接单集",
                link: "javascript:alert(1)",
                context: "ctx",
                decision_state: "pending",
              },
            ],
          }),
        ]}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /危险链接单集/ }));
    expect(screen.queryByText("打开原单集")).not.toBeInTheDocument();
  });
});
