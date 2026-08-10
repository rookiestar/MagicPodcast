import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DiscoveryPageClient from "../DiscoveryPageClient";
import { writeDiscoveryCandidatesCache } from "@/lib/discoveryCandidates";
import type { DiscoveryCandidate, HomepageReportsData } from "@/types/discovery";

const useSWRMock = vi.hoisted(() => vi.fn());
const apiPutMock = vi.hoisted(() => vi.fn());

vi.mock("swr", () => ({
  default: useSWRMock,
}));

vi.mock("@/lib/fetcher", () => ({
  apiClient: {
    put: apiPutMock,
  },
}));

vi.mock("@/components/layout/PageLayout", () => ({
  SimplePageLayout: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

vi.mock("@/components/discovery/DiscoveryDesk", () => ({
  default: ({
    candidates,
    onDecision,
  }: {
    candidates: DiscoveryCandidate[];
    onDecision?: (
      episodeID: number,
      state: "pending" | "shortlisted",
    ) => Promise<unknown>;
  }) => (
    <main aria-label="个人库最近更新">
      {candidates.map((candidate) => (
        <article key={candidate.episode_id}>{candidate.episode_title}</article>
      ))}
      <button
        type="button"
        onClick={() => {
          void onDecision?.(1, "shortlisted");
        }}
      >
        最近更新加入备选
      </button>
      <button
        type="button"
        onClick={() => {
          void onDecision?.(1, "pending");
        }}
      >
        最近更新略过
      </button>
    </main>
  ),
}));

vi.mock("@/components/discovery/WorkflowReportWorkbench", () => ({
  default: ({
    todayReports,
    failed,
    onDecision,
  }: {
    todayReports: { workflow_name: string }[];
    failed?: boolean;
    onDecision?: (episodeID: number, state: "shortlisted") => Promise<unknown>;
  }) =>
    failed ? (
      <section aria-label="精选报告">报告失败</section>
    ) : todayReports.length > 0 ? (
      <section aria-label="精选报告">
        {todayReports.map((report) => (
          <div key={report.workflow_name}>{report.workflow_name}</div>
        ))}
        <button
          type="button"
          onClick={() => {
            void onDecision?.(1, "shortlisted");
          }}
        >
          报告加入备选
        </button>
      </section>
    ) : null,
}));

const candidates: DiscoveryCandidate[] = [
  {
    episode_id: 1,
    podcast_id: 1,
    podcast_title: "默认首页节目",
    podcast_author: "作者",
    podcast_cover_url: "",
    episode_title: "保留的最近更新",
    episode_no: "E1",
    duration: 1800,
    candidate_time: "2026-07-29T08:00:00+08:00",
    time_basis: "published_date",
    source: "最近更新",
    show_notes: "<p>默认首页摘要来源</p>",
    show_notes_status: "available",
    original_url: "https://example.com/1",
    image_url: "",
    decision_state: "pending",
    pre_reads: [],
  },
];

const emptyReports: HomepageReportsData = {
  date: "2026-08-10",
  timezone: "Asia/Shanghai",
  today: [],
  history: [],
};

function mockSWRPair(options: {
  candidates?: {
    data?: DiscoveryCandidate[];
    error?: Error | null;
    isValidating?: boolean;
    mutate?: ReturnType<typeof vi.fn>;
  };
  reports?: {
    data?: HomepageReportsData;
    error?: Error | null;
    isValidating?: boolean;
    mutate?: ReturnType<typeof vi.fn>;
  };
}) {
  const candidatesMutate = options.candidates?.mutate ?? vi.fn();
  const reportsMutate = options.reports?.mutate ?? vi.fn();
  useSWRMock.mockImplementation((key: string) => {
    if (typeof key === "string" && key.includes("/discovery/reports")) {
      return {
        data: options.reports?.data,
        error: options.reports?.error ?? null,
        isValidating: options.reports?.isValidating ?? false,
        mutate: reportsMutate,
      };
    }
    return {
      data: options.candidates?.data,
      error: options.candidates?.error ?? null,
      isLoading: options.candidates?.isValidating && !options.candidates?.data,
      isValidating: options.candidates?.isValidating ?? false,
      mutate: candidatesMutate,
    };
  });
  return { candidatesMutate, reportsMutate };
}

describe("DiscoveryPageClient", () => {
  beforeEach(() => {
    useSWRMock.mockReset();
    apiPutMock.mockReset();
    window.sessionStorage.clear();
  });

  it("shows a structured skeleton while the first client request is retrying", () => {
    mockSWRPair({
      candidates: {
        data: undefined,
        error: new Error("retrying"),
        isValidating: true,
      },
      reports: { data: emptyReports },
    });

    render(<DiscoveryPageClient />);

    expect(
      screen.getByRole("main", { name: "正在读取个人库最近更新" }),
    ).toHaveAttribute("aria-busy", "true");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(useSWRMock).toHaveBeenCalledWith(
      "/api/v1/discovery/candidates?limit=30",
      expect.any(Function),
      expect.objectContaining({
        keepPreviousData: true,
        shouldRetryOnError: false,
      }),
    );
  });

  it("keeps existing content visible while refreshing in the background", () => {
    mockSWRPair({
      candidates: { data: candidates, isValidating: true },
      reports: { data: emptyReports },
    });

    render(<DiscoveryPageClient initialCandidates={candidates} />);

    expect(screen.getByText("保留的最近更新")).toBeInTheDocument();
    expect(screen.getByText("正在后台更新最近内容…")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("restores recent session content while a reload keeps retrying", async () => {
    writeDiscoveryCandidatesCache(window.sessionStorage, candidates);
    mockSWRPair({
      candidates: { data: undefined, isValidating: true },
      reports: { data: emptyReports },
    });

    render(<DiscoveryPageClient />);

    expect(await screen.findByText("保留的最近更新")).toBeInTheDocument();
    expect(
      screen.getByText("正在后台更新，当前显示上次加载结果…"),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("main", { name: "正在读取个人库最近更新" }),
    ).not.toBeInTheDocument();
  });

  it("keeps stale content and offers retry after background attempts fail", () => {
    const { candidatesMutate } = mockSWRPair({
      candidates: {
        data: candidates,
        error: new Error("unavailable"),
        isValidating: false,
      },
      reports: { data: emptyReports },
    });

    render(<DiscoveryPageClient initialCandidates={candidates} />);

    expect(screen.getByText("保留的最近更新")).toBeInTheDocument();
    expect(
      screen.getByText("最近更新暂时无法刷新，当前显示上次加载结果。"),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "重新尝试" }));
    expect(candidatesMutate).toHaveBeenCalledTimes(1);
  });

  it("keeps the skeleton and only reports failure after retries are exhausted", () => {
    const { candidatesMutate } = mockSWRPair({
      candidates: {
        data: undefined,
        error: new Error("unavailable"),
        isValidating: false,
      },
      reports: { data: emptyReports },
    });

    render(<DiscoveryPageClient />);

    expect(
      screen.getByRole("main", { name: "正在读取个人库最近更新" }),
    ).toHaveAttribute("aria-busy", "false");
    expect(screen.getByRole("alert")).toHaveTextContent(
      "最近更新暂时无法读取",
    );

    fireEvent.click(screen.getByRole("button", { name: "重新尝试" }));
    expect(candidatesMutate).toHaveBeenCalledTimes(1);
  });

  it("shows workflow reports above recent updates without blocking them", () => {
    mockSWRPair({
      candidates: { data: candidates },
      reports: {
        data: {
          date: "2026-08-10",
          timezone: "Asia/Shanghai",
          today: [
            {
              id: 1,
              job_id: 1,
              workflow_id: 1,
              workflow_name: "晨间日报",
              report_type: "daily",
              title: "t",
              content: "body",
              completed_at: "2026-08-10T08:00:00Z",
              generated_at: "2026-08-10T08:00:00Z",
              episode_count: 1,
              episodes: [],
            },
          ],
          history: [],
        },
      },
    });

    render(<DiscoveryPageClient initialCandidates={candidates} />);

    expect(screen.getByText("晨间日报")).toBeInTheDocument();
    expect(screen.getByText("保留的最近更新")).toBeInTheDocument();
  });

  it("keeps recent updates when report request fails", () => {
    mockSWRPair({
      candidates: { data: candidates },
      reports: {
        data: undefined,
        error: new Error("report down"),
        isValidating: false,
      },
    });

    render(<DiscoveryPageClient initialCandidates={candidates} />);

    expect(screen.getByText("保留的最近更新")).toBeInTheDocument();
    expect(screen.getByText("报告失败")).toBeInTheDocument();
  });

  it("does not mount report workbench when only history exists (#94)", () => {
    mockSWRPair({
      candidates: { data: candidates },
      reports: {
        data: {
          date: "2026-08-10",
          timezone: "Asia/Shanghai",
          today: [],
          history: [
            {
              id: 9,
              job_id: 9,
              workflow_id: 1,
              workflow_name: "往期",
              report_type: "daily",
              title: "t",
              content: "",
              completed_at: "2026-08-01T08:00:00Z",
              generated_at: "2026-08-01T08:00:00Z",
              episode_count: 1,
              episodes: [],
              metadata_only: true,
            },
          ],
        },
      },
    });

    render(<DiscoveryPageClient initialCandidates={candidates} />);
    expect(screen.getByText("保留的最近更新")).toBeInTheDocument();
    expect(screen.queryByLabelText("精选报告")).not.toBeInTheDocument();
  });

  it("deduplicates the same episode decision across report and recent updates (#94)", async () => {
    let resolveRequest: ((value: unknown) => void) | undefined;
    apiPutMock.mockReturnValue(
      new Promise((resolve) => {
        resolveRequest = resolve;
      }),
    );
    mockSWRPair({
      candidates: { data: candidates },
      reports: {
        data: {
          date: "2026-08-10",
          timezone: "Asia/Shanghai",
          today: [
            {
              id: 1,
              job_id: 1,
              workflow_id: 1,
              workflow_name: "晨间日报",
              report_type: "daily",
              title: "晨间日报",
              completed_at: "2026-08-10T08:00:00Z",
              generated_at: "2026-08-10T08:00:00Z",
              episode_count: 1,
              episodes: [],
            },
          ],
          history: [],
        },
      },
    });

    render(<DiscoveryPageClient initialCandidates={candidates} />);
    fireEvent.click(screen.getByRole("button", { name: "报告加入备选" }));
    fireEvent.click(screen.getByRole("button", { name: "最近更新加入备选" }));

    expect(apiPutMock).toHaveBeenCalledTimes(1);
    expect(apiPutMock).toHaveBeenCalledWith(
      "/api/v1/discovery/candidates/1/decision",
      { state: "shortlisted" },
    );

    resolveRequest?.({
      data: {
        data: {
          episode_id: 1,
          state: "shortlisted",
          decision_updated_at: "2026-08-10T12:00:00Z",
        },
      },
    });
    await screen.findByText("保留的最近更新");
  });

  it("serializes different decisions for the same episode without losing the later intent (#94)", async () => {
    let resolveFirst: ((value: unknown) => void) | undefined;
    apiPutMock
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
      )
      .mockResolvedValueOnce({
        data: {
          data: {
            episode_id: 1,
            state: "pending",
            decision_updated_at: "2026-08-10T12:01:00Z",
          },
        },
      });
    mockSWRPair({
      candidates: { data: candidates },
      reports: {
        data: {
          date: "2026-08-10",
          timezone: "Asia/Shanghai",
          today: [
            {
              id: 1,
              job_id: 1,
              workflow_id: 1,
              workflow_name: "晨间日报",
              report_type: "daily",
              title: "晨间日报",
              completed_at: "2026-08-10T08:00:00Z",
              generated_at: "2026-08-10T08:00:00Z",
              episode_count: 1,
              episodes: [],
            },
          ],
          history: [],
        },
      },
    });

    render(<DiscoveryPageClient initialCandidates={candidates} />);
    fireEvent.click(screen.getByRole("button", { name: "报告加入备选" }));
    fireEvent.click(screen.getByRole("button", { name: "最近更新略过" }));

    expect(apiPutMock).toHaveBeenCalledTimes(1);
    resolveFirst?.({
      data: {
        data: {
          episode_id: 1,
          state: "shortlisted",
          decision_updated_at: "2026-08-10T12:00:00Z",
        },
      },
    });

    await waitFor(() => expect(apiPutMock).toHaveBeenCalledTimes(2));
    expect(apiPutMock.mock.calls.map(([, body]) => body)).toEqual([
      { state: "shortlisted" },
      { state: "pending" },
    ]);
  });
});
