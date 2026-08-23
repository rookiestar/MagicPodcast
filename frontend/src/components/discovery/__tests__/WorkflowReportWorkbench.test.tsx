import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import WorkflowReportWorkbench, {
  HistoryDrawer,
} from "@/components/discovery/WorkflowReportWorkbench";
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

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the editorial heading and a single report without switch controls", () => {
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
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
      screen.getByRole("heading", { name: "晨间日报", level: 3 }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("晨间日报")).toHaveLength(1);
    expect(
      screen.getByText("日报 · 完成于 2026/08/10 16:00"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("listbox", { name: "当天报告" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("group", { name: "按完成时间浏览报告" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("1 / 1")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "查看更新一份报告" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "查看更早一份报告" }),
    ).not.toBeInTheDocument();

    const markdownSections = screen.getAllByTestId("markdown-body");
    expect(markdownSections).toHaveLength(1);
    expect(markdownSections[0]).toHaveTextContent("正文内容");
    expect(markdownSections[0]).not.toHaveTextContent("# 晨间日报");

    const heading = screen.getByRole("heading", { name: "晨间日报", level: 3 });
    const episode = screen.getByRole("article");
    expect(
      heading.compareDocumentPosition(episode) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      episode.compareDocumentPosition(markdownSections[0]) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("keeps a zero-episode report complete after removing the leading heading", () => {
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
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
    expect(
      screen.getByRole("heading", { name: "空报告", level: 3 }),
    ).toBeInTheDocument();
    expect(screen.getAllByTestId("markdown-body")).toHaveLength(1);
    expect(screen.getByTestId("markdown-body")).toHaveTextContent("完整正文");
    expect(screen.getByTestId("markdown-body")).not.toHaveTextContent("# 空报告");
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
        timezone="Asia/Shanghai"
        todayReports={[]}
        historyReports={[firstMetadata, secondMetadata, olderMetadata]}
      />,
    );

    expect(
      screen.getByRole("region", { name: "精选报告" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "科技日报", level: 3 }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("日报 · 完成于 2026/08/10 16:00"),
    ).toBeInTheDocument();
    expect(screen.queryByText("最新往期")).not.toBeInTheDocument();
    const latestPager = screen.getByRole("group", {
      name: "按完成时间浏览报告",
    });
    expect(within(latestPager).getByRole("status")).toHaveTextContent("1 / 2");
    expect(fetchDetailMock).toHaveBeenCalledWith(9);
    await waitFor(() =>
      expect(screen.getByText("科技正文")).toBeInTheDocument(),
    );

    fireEvent.click(screen.getByRole("button", { name: "查看更早一份报告" }));
    expect(within(latestPager).getByRole("status")).toHaveTextContent("2 / 2");
    expect(
      screen.getByRole("heading", { name: "投资日报", level: 3 }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("日报 · 完成于 2026/08/10 15:00"),
    ).toBeInTheDocument();
    expect(fetchDetailMock).toHaveBeenCalledWith(8);
    await waitFor(() =>
      expect(screen.getByText("投资正文")).toBeInTheDocument(),
    );
    expect(screen.queryByText("更早报告")).not.toBeInTheDocument();
  });

  it("switches reports via compact header controls while collapsing episodes", () => {
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
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

    expect(
      screen.getByRole("heading", { name: "早报", level: 3 }),
    ).toBeInTheDocument();
    const pager = screen.getByRole("group", { name: "按完成时间浏览报告" });
    expect(within(pager).getByRole("status")).toHaveTextContent("1 / 2");
    expect(
      within(pager).getByRole("button", { name: "查看更新一份报告" }),
    ).toBeDisabled();
    expect(
      within(pager).getByRole("button", { name: "查看更早一份报告" }),
    ).toBeEnabled();

    fireEvent.click(screen.getByRole("button", { name: "查看更早一份报告" }));
    expect(screen.getByText("午报单集")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "午报", level: 3 }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "早报" })).not.toBeInTheDocument();
    expect(
      screen.getByText("周报 · 完成于 2026/08/10 20:00"),
    ).toBeInTheDocument();
    expect(within(pager).getByRole("status")).toHaveTextContent("2 / 2");
    expect(
      within(pager).getByRole("button", { name: "查看更早一份报告" }),
    ).toBeDisabled();
    // Collapsed after switch.
    expect(screen.queryByText("B")).not.toBeInTheDocument();
  });

  it("keeps expand and collect as independent controls and shows source-backed Show Notes", async () => {
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
    expect(screen.getByText("Show Notes")).toBeInTheDocument();
    expect(screen.getByText("Shownotes 一")).toBeInTheDocument();

    const collect = screen.getAllByLabelText("收集到 Inbox")[0];
    expect(expandFirst.contains(collect)).toBe(false);

    collect.focus();
    await user.keyboard("{Enter}");
    await waitFor(() => {
      expect(onDecision).toHaveBeenCalledWith(31, "shortlisted");
    });
    // Expand state remains open while the same control supports reversal.
    expect(screen.getByText("Shownotes 一")).toBeInTheDocument();
    const remove = screen.getAllByLabelText("从 Inbox 移除")[0];
    expect(remove).toBeEnabled();
    expect(remove).toHaveAttribute("aria-pressed", "true");
    expect(
      remove.querySelector(".tabler-icon-bookmark-filled"),
    ).toBeInTheDocument();

    remove.focus();
    await user.keyboard("{Enter}");
    await waitFor(() => {
      expect(onDecision).toHaveBeenLastCalledWith(31, "pending");
    });
    expect(screen.getAllByLabelText("收集到 Inbox")[0]).toBeEnabled();
    expect(onDecision).toHaveBeenCalledTimes(2);

    fireEvent.click(screen.getByRole("button", { name: /第二集/ }));
    expect(screen.getByText("Shownotes 一")).toBeInTheDocument();
    expect(screen.getByText("Shownotes 二")).toBeInTheDocument();
  });

  it("keeps Focus, Someday, and Done as display-only states", () => {
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
              {
                episode_id: 42,
                order: 2,
                podcast_id: 1,
                podcast_title: "P",
                episode_title: "稍后单集",
                decision_state: "shortlisted",
                queue_state: "someday",
              },
              {
                episode_id: 43,
                order: 3,
                podcast_id: 1,
                podcast_title: "P",
                episode_title: "完成单集",
                decision_state: "shortlisted",
                queue_state: "done",
              },
            ],
          }),
        ]}
        onDecision={onDecision}
      />,
    );

    for (const label of ["已在 Focus", "已在 Someday", "已在 Done"]) {
      const state = screen.getByRole("button", { name: label });
      expect(state).toBeDisabled();
      expect(state).toHaveAttribute("aria-pressed", "false");
      expect(
        state.querySelector(".tabler-icon-bookmark-filled"),
      ).toBeInTheDocument();
      fireEvent.click(state);
    }
    expect(onDecision).not.toHaveBeenCalled();
  });

  it("exposes a busy state and blocks duplicate Inbox writes", async () => {
    let resolveDecision:
      | ((value: {
          state: "shortlisted";
          decision_updated_at: string;
        }) => void)
      | undefined;
    const onDecision = vi.fn(
      () =>
        new Promise<{
          state: "shortlisted";
          decision_updated_at: string;
        }>((resolve) => {
          resolveDecision = resolve;
        }),
    );
    render(
      <WorkflowReportWorkbench
        todayReports={[
          makeReport({
            id: 5,
            workflow_name: "忙碌态日报",
            episodes: [
              {
                episode_id: 51,
                order: 1,
                podcast_id: 1,
                podcast_title: "P",
                episode_title: "写入中的单集",
                decision_state: "pending",
              },
              {
                episode_id: 52,
                order: 2,
                podcast_id: 1,
                podcast_title: "P",
                episode_title: "等待中的单集",
                decision_state: "pending",
              },
            ],
          }),
        ]}
        onDecision={onDecision}
      />,
    );

    fireEvent.click(screen.getAllByRole("button", { name: "收集到 Inbox" })[0]);
    const busy = screen.getByRole("button", { name: "从 Inbox 移除" });
    const waiting = screen.getByRole("button", { name: "收集到 Inbox" });
    expect(busy).toBeDisabled();
    expect(busy).toHaveAttribute("aria-busy", "true");
    expect(waiting).toBeDisabled();
    fireEvent.click(busy);
    fireEvent.click(waiting);
    expect(onDecision).toHaveBeenCalledTimes(1);

    resolveDecision?.({
      state: "shortlisted",
      decision_updated_at: "2026-08-10T12:00:00Z",
    });
    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "从 Inbox 移除" }),
      ).toBeEnabled();
    });
    expect(screen.getByRole("button", { name: "收集到 Inbox" })).toBeEnabled();
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
        timezone="Asia/Shanghai"
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

    fireEvent.click(screen.getByRole("button", { name: "查看更早一份报告" }));
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

    expect(
      screen.getByRole("heading", { name: "上周周报", level: 3 }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("group", { name: "按完成时间浏览报告" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText("周报 · 完成于 2026/08/03 16:00"),
    ).toBeInTheDocument();
    await waitFor(() => {
      expect(fetchDetailMock).toHaveBeenCalledWith(9);
    });

    fireEvent.click(screen.getByRole("button", { name: "回到今日" }));
    expect(screen.getByText("2 / 2")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "今日乙", level: 3 }),
    ).toBeInTheDocument();
    expect(screen.getByText("单集 2")).toBeInTheDocument();
  });

  it("marks and returns to the current history report without moving focus (#153)", () => {
    const scrollIntoView = vi
      .spyOn(HTMLElement.prototype, "scrollIntoView")
      .mockImplementation(() => {});

    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
        todayReports={[makeReport({ id: 1, workflow_name: "今日报告" })]}
        historyReports={[
          makeReport({
            id: 9,
            workflow_name: "上周周报",
            completed_at: "2026-08-03T08:00:00Z",
          }),
          makeReport({
            id: 8,
            workflow_name: "更早日报",
            completed_at: "2026-08-02T08:00:00Z",
          }),
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    const firstDrawer = screen.getByRole("dialog", { name: "往期报告" });
    expect(within(firstDrawer).queryByText("当前查看")).not.toBeInTheDocument();
    expect(scrollIntoView).not.toHaveBeenCalled();

    fireEvent.click(
      within(firstDrawer).getByRole("button", { name: /上周周报/ }),
    );
    fireEvent.click(screen.getByRole("button", { name: /往期/ }));

    const reopenedDrawer = screen.getByRole("dialog", { name: "往期报告" });
    const currentReport = within(reopenedDrawer).getByRole("button", {
      name: /上周周报/,
    });
    expect(currentReport).toHaveAttribute("aria-current", "true");
    expect(within(currentReport).getByText("当前查看")).toBeInTheDocument();
    expect(scrollIntoView).toHaveBeenCalledWith({
      behavior: "auto",
      block: "center",
    });
    expect(screen.getByRole("button", { name: "关闭" })).toHaveFocus();
  });

  it("updates the history anchor and clears it after returning to today (#153)", () => {
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
        todayReports={[makeReport({ id: 1, workflow_name: "今日报告" })]}
        historyReports={[
          makeReport({
            id: 9,
            workflow_name: "上周周报",
            completed_at: "2026-08-03T08:00:00Z",
          }),
          makeReport({
            id: 8,
            workflow_name: "更早日报",
            completed_at: "2026-08-02T08:00:00Z",
          }),
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    fireEvent.click(
      within(screen.getByRole("dialog", { name: "往期报告" })).getByRole(
        "button",
        { name: /上周周报/ },
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    fireEvent.click(
      within(screen.getByRole("dialog", { name: "往期报告" })).getByRole(
        "button",
        { name: /更早日报/ },
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: /往期/ }));

    const updatedDrawer = screen.getByRole("dialog", { name: "往期报告" });
    expect(
      within(updatedDrawer).getByRole("button", { name: /上周周报/ }),
    ).not.toHaveAttribute("aria-current");
    expect(
      within(updatedDrawer).getByRole("button", { name: /更早日报/ }),
    ).toHaveAttribute("aria-current", "true");

    fireEvent.click(
      within(updatedDrawer).getByRole("button", { name: "关闭" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "回到今日" }));
    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    expect(
      within(screen.getByRole("dialog", { name: "往期报告" })).queryByText(
        "当前查看",
      ),
    ).not.toBeInTheDocument();
  });

  it("applies a pending history pick before the body request resolves", async () => {
    let resolveDetail: (report: HomepageReport) => void = () => {};
    fetchDetailMock.mockImplementation(
      () =>
        new Promise<HomepageReport>((resolve) => {
          resolveDetail = resolve;
        }),
    );

    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
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

    fireEvent.click(screen.getByRole("button", { name: "查看更早一份报告" }));
    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    fireEvent.click(
      within(screen.getByRole("dialog", { name: "往期报告" })).getByText(
        "上周周报",
      ),
    );

    expect(
      screen.getByRole("heading", { name: "上周周报", level: 3 }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("周报 · 完成于 2026/08/03 16:00"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("group", { name: "按完成时间浏览报告" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("正在加载往期报告…")).toBeInTheDocument();
    expect(screen.queryByText("往期正文")).not.toBeInTheDocument();

    resolveDetail(
      makeReport({
        id: 9,
        workflow_name: "上周周报",
        report_type: "weekly",
        completed_at: "2026-08-03T08:00:00Z",
        content: "# 往期全文\n\n往期正文",
        metadata_only: false,
      }),
    );
    await waitFor(() => {
      expect(screen.getByText("往期正文")).toBeInTheDocument();
    });
    expect(screen.getByTestId("markdown-body")).not.toHaveTextContent(
      "# 往期全文",
    );
  });

  it("restores the previous report if a pending history body fails", async () => {
    fetchDetailMock.mockRejectedValue(new Error("load failed"));

    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
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

    fireEvent.click(screen.getByRole("button", { name: "查看更早一份报告" }));
    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    fireEvent.click(
      within(screen.getByRole("dialog", { name: "往期报告" })).getByText(
        "上周周报",
      ),
    );

    await waitFor(() => {
      expect(screen.getByText("往期报告加载失败，可重试选择。")).toBeInTheDocument();
    });
    expect(
      screen.getByRole("heading", { name: "今日乙", level: 3 }),
    ).toBeInTheDocument();
    expect(screen.getByText("2 / 2")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    expect(
      within(screen.getByRole("dialog", { name: "往期报告" })).queryByText(
        "当前查看",
      ),
    ).not.toBeInTheDocument();
  });

  it("does not revive a pending history pick after returning to today", async () => {
    let resolveDetail: (report: HomepageReport) => void = () => {};
    fetchDetailMock.mockImplementation(
      () =>
        new Promise<HomepageReport>((resolve) => {
          resolveDetail = resolve;
        }),
    );

    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
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

    fireEvent.click(screen.getByRole("button", { name: "查看更早一份报告" }));
    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    fireEvent.click(
      within(screen.getByRole("dialog", { name: "往期报告" })).getByText(
        "上周周报",
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "回到今日" }));
    expect(
      screen.getByRole("heading", { name: "今日乙", level: 3 }),
    ).toBeInTheDocument();

    resolveDetail(
      makeReport({
        id: 9,
        workflow_name: "上周周报",
        report_type: "weekly",
        completed_at: "2026-08-03T08:00:00Z",
        content: "# 往期全文\n\n往期正文",
        metadata_only: false,
      }),
    );
    await waitFor(() => {
      expect(fetchDetailMock).toHaveBeenCalledWith(9);
    });
    expect(
      screen.getByRole("heading", { name: "今日乙", level: 3 }),
    ).toBeInTheDocument();
    expect(screen.queryByText("往期正文")).not.toBeInTheDocument();
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

  it("does not show report recommendations when Show Notes are unavailable", () => {
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
    expect(screen.queryByText("报告推荐")).not.toBeInTheDocument();
    expect(screen.queryByText("不应展示")).not.toBeInTheDocument();
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

  it("keeps indented markdown after removing a leading heading", () => {
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
        todayReports={[
          makeReport({
            id: 24,
            workflow_name: "晨间日报",
            episode_count: 0,
            episodes: [],
            content: "# 晨间日报\n\n    const x = 1\n",
          }),
        ]}
      />,
    );

    expect(screen.getByTestId("markdown-body").textContent).toBe(
      "    const x = 1\n",
    );
  });

  it("strips a CommonMark heading indented by up to three spaces", () => {
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
        todayReports={[
          makeReport({
            id: 26,
            workflow_name: "晨间日报",
            episode_count: 0,
            episodes: [],
            content: "  \n   # 晨间日报\n\n摘要一段\n",
          }),
        ]}
      />,
    );

    expect(screen.getAllByText("晨间日报")).toHaveLength(1);
    expect(screen.getByTestId("markdown-body").textContent).toBe("摘要一段\n");
    expect(screen.getByTestId("markdown-body").textContent).not.toContain(
      "# 晨间日报",
    );
  });

  it("does not treat an indented heading as the report title source", () => {
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
        todayReports={[
          makeReport({
            id: 25,
            workflow_name: "晨间日报",
            episode_count: 0,
            episodes: [],
            content: "    # 这不是标题\n    const x = 1\n",
          }),
        ]}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "晨间日报", level: 3 }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("markdown-body").textContent).toBe(
      "    # 这不是标题\n    const x = 1\n",
    );
  });

  it("strips only the first markdown heading and keeps the rest of the body", () => {
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
        todayReports={[
          makeReport({
            id: 21,
            workflow_name: "晨间日报",
            content:
              "# 晨间日报 2026-08-10 08:00:00\n\n摘要一段\n\n## 后续标题\n\n保留段落",
          }),
        ]}
      />,
    );

    expect(screen.getAllByText("晨间日报")).toHaveLength(1);
    expect(screen.getByTestId("markdown-body")).toHaveTextContent("摘要一段");
    expect(screen.getByTestId("markdown-body")).toHaveTextContent("## 后续标题");
    expect(screen.getByTestId("markdown-body")).toHaveTextContent("保留段落");
    expect(screen.getByTestId("markdown-body")).not.toHaveTextContent(
      "# 晨间日报 2026-08-10 08:00:00",
    );
  });

  it("falls back to the report title when workflow_name is missing", () => {
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
        todayReports={[
          makeReport({
            id: 22,
            workflow_name: "   ",
            title: "历史标题回退",
            content: "没有一级标题\n\n正文仍在",
          }),
        ]}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "历史标题回退", level: 3 }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("markdown-body")).toHaveTextContent("没有一级标题");
    expect(screen.getByTestId("markdown-body")).toHaveTextContent("正文仍在");
  });

  it("keeps a long mixed title readable without pager chrome on a single report", () => {
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
        todayReports={[
          makeReport({
            id: 23,
            workflow_name: "DeepSeek Morning Brief 晨间深读长标题",
          }),
        ]}
      />,
    );

    expect(
      screen.getByRole("heading", {
        name: "DeepSeek Morning Brief 晨间深读长标题",
        level: 3,
      }),
    ).toBeInTheDocument();
    expect(screen.queryByText("1 / 1")).not.toBeInTheDocument();
  });

  it("switches reports with arrow keys and ignores them while the drawer is open", async () => {
    const user = userEvent.setup();
    render(
      <WorkflowReportWorkbench
        timezone="Asia/Shanghai"
        todayReports={[
          makeReport({ id: 1, workflow_name: "早报" }),
          makeReport({
            id: 2,
            workflow_name: "午报",
            completed_at: "2026-08-10T12:00:00Z",
          }),
        ]}
        historyReports={[
          makeReport({
            id: 9,
            workflow_id: 9,
            workflow_name: "科技日报",
            completed_at: "2026-08-03T08:00:00Z",
            metadata_only: true,
            content: "",
          }),
          makeReport({
            id: 8,
            workflow_id: 8,
            workflow_name: "投资周报",
            report_type: "weekly",
            completed_at: "2026-08-02T08:00:00Z",
            metadata_only: true,
            content: "",
          }),
        ]}
      />,
    );

    const workbench = screen.getByRole("region", { name: "精选报告" });
    workbench.focus();
    fireEvent.keyDown(workbench, { key: "ArrowRight" });
    expect(
      screen.getByRole("heading", { name: "午报", level: 3 }),
    ).toBeInTheDocument();
    expect(screen.getByText("2 / 2")).toBeInTheDocument();

    fireEvent.keyDown(workbench, { key: "ArrowLeft" });
    expect(
      screen.getByRole("heading", { name: "早报", level: 3 }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /往期/ }));
    const drawer = screen.getByRole("dialog", { name: "往期报告" });
    fireEvent.keyDown(workbench, { key: "ArrowRight" });
    expect(
      screen.getByRole("heading", { name: "早报", level: 3 }),
    ).toBeInTheDocument();

    fireEvent.click(within(drawer).getByRole("button", { name: /筛选工作流/ }));
    const search = within(drawer).getByRole("searchbox", { name: "搜索工作流" });
    fireEvent.keyDown(search, { key: "ArrowRight" });
    expect(
      screen.getByRole("heading", { name: "早报", level: 3 }),
    ).toBeInTheDocument();
  });

  describe("history workflow filter (#144)", () => {

    function historyMetadata(
      overrides: Array<{
        id: number;
        workflow_id: number;
        workflow_name: string;
        completed_at: string;
      }>,
    ): HomepageReport[] {
      return overrides.map((item) =>
        makeReport({
          ...item,
          metadata_only: true,
          content: "",
        }),
      );
    }

    // Fixed timezone keeps date grouping deterministic: 8/12 (Wed) and 8/11 (Tue).
    const baseHistory = () =>
      historyMetadata([
        {
          id: 101,
          workflow_id: 10,
          workflow_name: "科技日报",
          completed_at: "2026-08-12T01:00:00Z",
        },
        {
          id: 102,
          workflow_id: 20,
          workflow_name: "投资周报",
          completed_at: "2026-08-11T02:00:00Z",
        },
        {
          id: 103,
          workflow_id: 10,
          workflow_name: "科技日报",
          completed_at: "2026-08-11T01:00:00Z",
        },
      ]);

    function renderWorkbench(historyReports: HomepageReport[]) {
      return render(
        <WorkflowReportWorkbench
          timezone="Asia/Shanghai"
          todayReports={[makeReport({ id: 1, workflow_name: "今日" })]}
          historyReports={historyReports}
        />,
      );
    }

    function openDrawer() {
      fireEvent.click(screen.getByRole("button", { name: /往期/ }));
      return screen.getByRole("dialog", { name: "往期报告" });
    }

    function expandFilter(drawer: HTMLElement) {
      fireEvent.click(
        within(drawer).getByRole("button", { name: /筛选工作流/ }),
      );
    }

    it("hides the filter entry when history has fewer than two workflows", () => {
      renderWorkbench(
        historyMetadata([
          {
            id: 101,
            workflow_id: 10,
            workflow_name: "科技日报",
            completed_at: "2026-08-12T01:00:00Z",
          },
          {
            id: 103,
            workflow_id: 10,
            workflow_name: "科技日报",
            completed_at: "2026-08-11T01:00:00Z",
          },
        ]),
      );
      const drawer = openDrawer();

      expect(
        within(drawer).queryByRole("button", { name: /筛选工作流/ }),
      ).not.toBeInTheDocument();
      expect(
        within(drawer).getAllByRole("button", { name: /科技日报/ }),
      ).toHaveLength(2);
    });

    it("shows the filter entry with the 30-report scope hint and newest-first options", () => {
      renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);

      expect(within(drawer).getByText("筛选最近 30 份报告")).toBeInTheDocument();
      expect(
        within(drawer).getByRole("searchbox", { name: "搜索工作流" }),
      ).toBeInTheDocument();

      const checkboxes = within(drawer).getAllByRole("checkbox");
      expect(checkboxes).toHaveLength(2);
      expect(checkboxes[0].closest("label")).toHaveTextContent("科技日报2 份");
      expect(checkboxes[1].closest("label")).toHaveTextContent("投资周报1 份");
    });

    it("filters to one workflow and hides date groups without matches", () => {
      renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);

      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /投资周报/ }),
      );

      expect(
        within(drawer).getAllByRole("button", { name: /投资周报/ }),
      ).toHaveLength(1);
      expect(
        within(drawer).queryByRole("button", { name: /科技日报/ }),
      ).not.toBeInTheDocument();
      expect(
        within(drawer).getByRole("region", { name: "2026年8月11日 · 周二" }),
      ).toBeInTheDocument();
      expect(
        within(drawer).queryByRole("region", {
          name: "2026年8月12日 · 周三",
        }),
      ).not.toBeInTheDocument();
    });

    it("combines multiple workflows with OR semantics", () => {
      renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);

      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /科技日报/ }),
      );
      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /投资周报/ }),
      );

      expect(
        within(drawer).getAllByRole("button", { name: /科技日报|投资周报/ }),
      ).toHaveLength(3);
      expect(
        within(drawer).getByRole("region", { name: "2026年8月12日 · 周三" }),
      ).toBeInTheDocument();
      expect(
        within(drawer).getByRole("region", { name: "2026年8月11日 · 周二" }),
      ).toBeInTheDocument();
    });

    it("restores the full list after unchecking and clearing", () => {
      renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);

      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /科技日报/ }),
      );
      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /投资周报/ }),
      );
      expect(
        within(drawer).getAllByRole("button", { name: /科技日报|投资周报/ }),
      ).toHaveLength(3);

      // Uncheck one: only the other workflow remains.
      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /科技日报/ }),
      );
      expect(
        within(drawer).queryByRole("button", { name: /科技日报/ }),
      ).not.toBeInTheDocument();

      fireEvent.click(within(drawer).getByRole("button", { name: "清除筛选" }));
      expect(
        within(drawer).getAllByRole("button", { name: /科技日报|投资周报/ }),
      ).toHaveLength(3);
      expect(within(drawer).queryByText(/已选/)).not.toBeInTheDocument();
    });

    it("shows the selected count on the collapsed entry with a direct clear action", () => {
      renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);

      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /科技日报/ }),
      );
      expandFilter(drawer); // collapse again

      expect(within(drawer).getByText("已选 1")).toBeInTheDocument();
      expect(
        within(drawer).queryByRole("searchbox", { name: "搜索工作流" }),
      ).not.toBeInTheDocument();

      fireEvent.click(within(drawer).getByRole("button", { name: "清除筛选" }));
      expect(
        within(drawer).getAllByRole("button", { name: /科技日报|投资周报/ }),
      ).toHaveLength(3);
      expect(within(drawer).queryByText(/已选/)).not.toBeInTheDocument();
    });

    it("narrows only the option list with the keyword, not the reports", () => {
      renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);

      const search = within(drawer).getByRole("searchbox", {
        name: "搜索工作流",
      });
      fireEvent.change(search, { target: { value: "科技" } });

      expect(
        within(drawer).getByRole("checkbox", { name: /科技日报/ }),
      ).toBeInTheDocument();
      expect(
        within(drawer).queryByRole("checkbox", { name: /投资周报/ }),
      ).not.toBeInTheDocument();
      expect(
        within(drawer).getAllByRole("button", { name: /科技日报|投资周报/ }),
      ).toHaveLength(3);

      fireEvent.change(search, { target: { value: "" } });
      expect(
        within(drawer).getByRole("checkbox", { name: /投资周报/ }),
      ).toBeInTheDocument();
    });

    it("matches historical workflow names while showing the latest name", () => {
      renderWorkbench(
        historyMetadata([
          {
            id: 101,
            workflow_id: 10,
            workflow_name: "科技日报",
            completed_at: "2026-08-12T01:00:00Z",
          },
          {
            id: 103,
            workflow_id: 10,
            workflow_name: "科技快报",
            completed_at: "2026-08-11T01:00:00Z",
          },
          {
            id: 102,
            workflow_id: 20,
            workflow_name: "投资周报",
            completed_at: "2026-08-11T02:00:00Z",
          },
        ]),
      );
      const drawer = openDrawer();
      expandFilter(drawer);

      const techOption = within(drawer).getByRole("checkbox", {
        name: /科技日报/,
      });
      expect(techOption.closest("label")).toHaveTextContent("2 份");
      expect(
        within(drawer).queryByRole("checkbox", { name: /科技快报/ }),
      ).not.toBeInTheDocument();

      const search = within(drawer).getByRole("searchbox", {
        name: "搜索工作流",
      });
      fireEvent.change(search, { target: { value: "科技快报" } });

      expect(
        within(drawer).getByRole("checkbox", { name: /科技日报/ }),
      ).toBeInTheDocument();
    });

    it("keeps selections when the keyword matches nothing", () => {
      renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);

      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /投资周报/ }),
      );
      fireEvent.change(
        within(drawer).getByRole("searchbox", { name: "搜索工作流" }),
        { target: { value: "不存在的工作流" } },
      );

      expect(within(drawer).getByText("没有匹配的工作流。")).toBeInTheDocument();
      expect(
        within(drawer).getAllByRole("button", { name: /投资周报/ }),
      ).toHaveLength(1);
      expect(
        within(drawer).queryByRole("button", { name: /科技日报/ }),
      ).not.toBeInTheDocument();
    });

    it("keeps selections across drawer close/reopen but collapses the panel and clears the keyword", () => {
      renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);

      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /科技日报/ }),
      );
      fireEvent.change(
        within(drawer).getByRole("searchbox", { name: "搜索工作流" }),
        { target: { value: "科技" } },
      );
      fireEvent.click(within(drawer).getByRole("button", { name: "关闭" }));
      expect(
        screen.queryByRole("dialog", { name: "往期报告" }),
      ).not.toBeInTheDocument();

      const reopened = openDrawer();
      expect(
        within(reopened).getAllByRole("button", { name: /科技日报/ }),
      ).toHaveLength(2);
      expect(
        within(reopened).queryByRole("button", { name: /投资周报/ }),
      ).not.toBeInTheDocument();
      expect(within(reopened).getByText("已选 1")).toBeInTheDocument();
      expect(
        within(reopened).getByRole("button", { name: /筛选工作流/ }),
      ).toHaveAttribute("aria-expanded", "false");

      expandFilter(reopened);
      expect(
        within(reopened).getByRole("searchbox", { name: "搜索工作流" }),
      ).toHaveValue("");
    });

    it("collapses the panel and clears the keyword when picking a report closes the drawer", async () => {
      fetchDetailMock.mockResolvedValue(
        makeReport({
          id: 101,
          workflow_id: 10,
          workflow_name: "科技日报",
          completed_at: "2026-08-12T01:00:00Z",
          content: "# 科技日报\n\n科技全文",
          metadata_only: false,
        }),
      );
      renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);
      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /科技日报/ }),
      );
      fireEvent.change(
        within(drawer).getByRole("searchbox", { name: "搜索工作流" }),
        { target: { value: "科技" } },
      );
      fireEvent.click(
        within(drawer).getAllByRole("button", { name: /科技日报/ })[0],
      );
      await waitFor(() => {
        expect(
          screen.queryByRole("dialog", { name: "往期报告" }),
        ).not.toBeInTheDocument();
      });

      const reopened = openDrawer();
      expect(within(reopened).getByText("已选 1")).toBeInTheDocument();
      expect(
        within(reopened).getByRole("button", { name: /筛选工作流/ }),
      ).toHaveAttribute("aria-expanded", "false");
      expandFilter(reopened);
      expect(
        within(reopened).getByRole("searchbox", { name: "搜索工作流" }),
      ).toHaveValue("");
    });

    it("keeps the selection in the empty-result state with a direct clear entry", () => {
      const onClearSelection = vi.fn();
      render(
        <HistoryDrawer
          reports={[]}
          timezone="Asia/Shanghai"
          onClose={() => {}}
          onSelect={() => {}}
          filter={{
            options: [
              {
                workflowId: 10,
                label: "科技日报",
                names: ["科技日报"],
                latestCompletedAt: "2026-08-12T01:00:00Z",
                reportCount: 2,
              },
              {
                workflowId: 20,
                label: "投资周报",
                names: ["投资周报"],
                latestCompletedAt: "2026-08-11T02:00:00Z",
                reportCount: 1,
              },
            ],
            selectedIds: new Set([10]),
            keyword: "",
            open: false,
            onToggleOpen: () => {},
            onKeywordChange: () => {},
            onToggleSelection: () => {},
            onClearSelection,
          }}
        />,
      );

      expect(
        screen.getByText("没有符合所选工作流的报告。"),
      ).toBeInTheDocument();
      // The selection is retained (badge still counts it) until cleared.
      expect(screen.getByText("已选 1")).toBeInTheDocument();
      const noresults = screen
        .getByText("没有符合所选工作流的报告。")
        .closest("div");
      expect(noresults).not.toBeNull();
      fireEvent.click(
        within(noresults as HTMLElement).getByRole("button", {
          name: "清除筛选",
        }),
      );
      expect(onClearSelection).toHaveBeenCalledTimes(1);
    });

    it("resets the whole filter after a page refresh (remount)", () => {
      const { unmount } = renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);
      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /科技日报/ }),
      );
      fireEvent.change(
        within(drawer).getByRole("searchbox", { name: "搜索工作流" }),
        { target: { value: "科技" } },
      );
      expect(within(drawer).getByText("已选 1")).toBeInTheDocument();
      unmount();

      renderWorkbench(baseHistory());
      const reopened = openDrawer();
      expect(within(reopened).queryByText("已选 1")).not.toBeInTheDocument();
      expect(
        within(reopened).getAllByRole("button", { name: /投资周报/ }),
      ).toHaveLength(1);
      expect(
        within(reopened).getByRole("button", { name: /筛选工作流/ }),
      ).toHaveAttribute("aria-expanded", "false");
      expandFilter(reopened);
      expect(
        within(reopened).getByRole("searchbox", { name: "搜索工作流" }),
      ).toHaveValue("");
    });

    it("prunes stale selections on data refresh while keeping valid ones", () => {
      const { rerender } = renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);
      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /科技日报/ }),
      );
      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /投资周报/ }),
      );

      // Refresh drops 投资周报 and introduces 环保周报.
      rerender(
        <WorkflowReportWorkbench
          timezone="Asia/Shanghai"
          todayReports={[makeReport({ id: 1, workflow_name: "今日" })]}
          historyReports={historyMetadata([
            {
              id: 101,
              workflow_id: 10,
              workflow_name: "科技日报",
              completed_at: "2026-08-12T01:00:00Z",
            },
            {
              id: 104,
              workflow_id: 30,
              workflow_name: "环保周报",
              completed_at: "2026-08-11T01:00:00Z",
            },
          ])}
        />,
      );
      const refreshed = screen.getByRole("dialog", { name: "往期报告" });
      expect(within(refreshed).getByText("已选 1")).toBeInTheDocument();
      expect(
        within(refreshed).getAllByRole("button", { name: /科技日报/ }),
      ).toHaveLength(1);
      expect(
        within(refreshed).queryByRole("button", { name: /环保周报/ }),
      ).not.toBeInTheDocument();

      // Refresh removes the last selected workflow too: filter resets to none.
      rerender(
        <WorkflowReportWorkbench
          timezone="Asia/Shanghai"
          todayReports={[makeReport({ id: 1, workflow_name: "今日" })]}
          historyReports={historyMetadata([
            {
              id: 104,
              workflow_id: 30,
              workflow_name: "环保周报",
              completed_at: "2026-08-11T01:00:00Z",
            },
          ])}
        />,
      );
      // One workflow left → entry hidden, full list restored.
      const reset = screen.getByRole("dialog", { name: "往期报告" });
      expect(
        within(reset).queryByRole("button", { name: /筛选工作流/ }),
      ).not.toBeInTheDocument();
      expect(
        within(reset).getAllByRole("button", { name: /环保周报/ }),
      ).toHaveLength(1);
    });

    it("does not resurrect a selection pruned by an earlier refresh", () => {
      const { rerender } = renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);
      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /投资周报/ }),
      );

      // Refresh drops 投资周报: the selection is pruned and committed to
      // source state, not just hidden for this render.
      rerender(
        <WorkflowReportWorkbench
          timezone="Asia/Shanghai"
          todayReports={[makeReport({ id: 1, workflow_name: "今日" })]}
          historyReports={historyMetadata([
            {
              id: 101,
              workflow_id: 10,
              workflow_name: "科技日报",
              completed_at: "2026-08-12T01:00:00Z",
            },
          ])}
        />,
      );
      const pruned = screen.getByRole("dialog", { name: "往期报告" });
      expect(within(pruned).queryByText(/已选/)).not.toBeInTheDocument();

      // A later refresh brings 投资周报 back: the removed selection stays
      // removed and the full list renders unfiltered.
      rerender(
        <WorkflowReportWorkbench
          timezone="Asia/Shanghai"
          todayReports={[makeReport({ id: 1, workflow_name: "今日" })]}
          historyReports={baseHistory()}
        />,
      );
      const restored = screen.getByRole("dialog", { name: "往期报告" });
      expect(within(restored).queryByText(/已选/)).not.toBeInTheDocument();
      // Unfiltered: both 科技日报 reports and the restored 投资周报 render.
      expect(
        within(restored).getAllByRole("button", { name: /科技日报/ }),
      ).toHaveLength(2);
      expect(
        within(restored).getAllByRole("button", { name: /投资周报/ }),
      ).toHaveLength(1);
      // The panel stayed expanded across rerenders; the restored workflow's
      // checkbox is unchecked.
      expect(
        within(restored).getByRole("checkbox", { name: /投资周报/ }),
      ).not.toBeChecked();
    });

    it("loads the filtered report body on demand without prefetching others", async () => {
      fetchDetailMock.mockResolvedValue(
        makeReport({
          id: 101,
          workflow_id: 10,
          workflow_name: "科技日报",
          completed_at: "2026-08-12T01:00:00Z",
          content: "# 科技日报\n\n科技全文",
          metadata_only: false,
        }),
      );
      renderWorkbench(baseHistory());
      const drawer = openDrawer();
      expandFilter(drawer);

      fireEvent.click(
        within(drawer).getByRole("checkbox", { name: /科技日报/ }),
      );
      fireEvent.click(
        within(drawer).getAllByRole("button", { name: /科技日报/ })[0],
      );

      expect(fetchDetailMock).toHaveBeenCalledTimes(1);
      expect(fetchDetailMock).toHaveBeenCalledWith(101);
      await waitFor(() => {
        expect(screen.getByText("科技全文")).toBeInTheDocument();
      });
      expect(
        screen.queryByRole("dialog", { name: "往期报告" }),
      ).not.toBeInTheDocument();
    });
  });
});
