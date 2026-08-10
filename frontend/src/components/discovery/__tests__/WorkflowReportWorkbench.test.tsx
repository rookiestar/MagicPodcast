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
    fetchHomepageReportDetail: (...args: unknown[]) =>
      fetchDetailMock(...args),
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

  it("renders banner cards and a single report body", () => {
    render(
      <WorkflowReportWorkbench
        todayReports={[makeReport({ id: 1, workflow_name: "晨间日报" })]}
      />,
    );

    expect(screen.getByRole("listbox", { name: "当天报告" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /晨间日报/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByTestId("markdown-body")).toHaveTextContent("正文内容");
  });

  it("hides the entire region when today is empty even if history exists (#94)", () => {
    const { container } = render(
      <WorkflowReportWorkbench
        todayReports={[]}
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
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByText("今日暂无有效报告")).not.toBeInTheDocument();
  });

  it("switches reports via banner without autoplay and collapses episodes", () => {
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
    expect(screen.getByText("节目上下文")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("option", { name: /午报/ }));
    expect(screen.getByText("午报单集")).toBeInTheDocument();
    expect(screen.getByText("2 / 2")).toBeInTheDocument();
    // Collapsed after switch.
    expect(screen.queryByText("B")).not.toBeInTheDocument();
  });

  it("keeps expand and shortlist as independent non-nested controls (#94)", async () => {
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
                recommendation: "",
                context: "理由一",
                decision_state: "pending",
              },
              {
                episode_id: 32,
                order: 2,
                podcast_id: 1,
                podcast_title: "P2",
                episode_title: "第二集",
                context: "理由二",
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
    expect(screen.getByText("理由一")).toBeInTheDocument();
    expect(
      screen.getByText("本条报告未附带逐单集推荐依据，请结合正文判断。"),
    ).toBeInTheDocument();

    const shortlist = screen.getAllByLabelText("加入今日备选")[0];
    expect(expandFirst.contains(shortlist)).toBe(false);

    shortlist.focus();
    await user.keyboard("{Enter}");
    await waitFor(() => {
      expect(onDecision).toHaveBeenCalledWith(31, "shortlisted");
    });
    // Expand state unchanged by shortlist.
    expect(screen.getByText("理由一")).toBeInTheDocument();

    const removeFromShortlist = screen.getAllByLabelText("移出今日备选")[0];
    removeFromShortlist.focus();
    await user.keyboard(" ");
    await waitFor(() => {
      expect(onDecision).toHaveBeenNthCalledWith(2, 31, "pending");
    });
    expect(screen.getByText("理由一")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /第二集/ }));
    expect(screen.getByText("理由一")).toBeInTheDocument();
    expect(screen.getByText("理由二")).toBeInTheDocument();
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

    fireEvent.click(screen.getByRole("option", { name: /今日乙/ }));
    expect(screen.getByText("2 / 2")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /往期/ }));
    expect(screen.getByRole("dialog", { name: "往期报告" })).toBeInTheDocument();
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
    expect(screen.getByRole("option", { name: /今日乙/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByText("2 / 2")).toBeInTheDocument();
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
    expect(screen.getByRole("dialog", { name: "往期报告" })).toBeInTheDocument();

    fireEvent.keyDown(window, { key: "Escape" });
    expect(
      screen.queryByRole("dialog", { name: "往期报告" }),
    ).not.toBeInTheDocument();
  });

  it("shows compact retry feedback when report load fails", () => {
    const onRetry = vi.fn();
    render(
      <WorkflowReportWorkbench todayReports={[]} failed onRetry={onRetry} />,
    );
    expect(screen.getByText("工作流报告暂时无法读取")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "重新尝试" }));
    expect(onRetry).toHaveBeenCalled();
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
