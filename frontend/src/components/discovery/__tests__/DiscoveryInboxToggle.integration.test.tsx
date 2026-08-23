import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DiscoveryPageClient from "../DiscoveryPageClient";
import type {
  DiscoveryCandidate,
  HomepageReportsData,
} from "@/types/discovery";

const useSWRMock = vi.hoisted(() => vi.fn());
const apiPutMock = vi.hoisted(() => vi.fn());
const apiDeleteMock = vi.hoisted(() => vi.fn());
const apiPostMock = vi.hoisted(() => vi.fn());
const revalidateConsumptionSummaryMock = vi.hoisted(() => vi.fn());

vi.mock("swr", () => ({
  default: useSWRMock,
}));

vi.mock("@/lib/fetcher", () => ({
  apiClient: {
    put: apiPutMock,
    delete: apiDeleteMock,
    post: apiPostMock,
  },
}));

vi.mock("@/lib/api/consumption", () => ({
  revalidateConsumptionSummary: revalidateConsumptionSummaryMock,
}));

vi.mock("@/components/layout/PageLayout", () => ({
  SimplePageLayout: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
}));

vi.mock("@/components/discovery/DiscoveryFocusSummary", () => ({
  default: () => <aside aria-label="Focus 快捷摘要" />,
}));

vi.mock("@/components/workflows/MarkdownViewer", () => ({
  default: ({ content }: { content: string }) => (
    <div data-testid="markdown-body">{content}</div>
  ),
}));

const candidate: DiscoveryCandidate = {
  episode_id: 173,
  podcast_id: 17,
  podcast_title: "同一节目",
  podcast_author: "作者",
  podcast_cover_url: "",
  episode_title: "跨区域同步单集",
  episode_no: "E173",
  duration: 2400,
  candidate_time: "2026-08-23T08:00:00+08:00",
  time_basis: "fetched_at",
  source: "最近更新",
  excerpt: "同一 episode 同时出现在最近更新与精选报告。",
  show_notes: "<p>可核对 Show Notes</p>",
  show_notes_status: "available",
  original_url: "https://example.com/episodes/173",
  image_url: "",
  decision_state: "pending",
  queue_state: null,
  read_at: "2026-08-23T08:01:00+08:00",
  pre_reads: [],
};

const reports: HomepageReportsData = {
  date: "2026-08-23",
  timezone: "Asia/Shanghai",
  today: [
    {
      id: 173,
      job_id: 173,
      workflow_id: 17,
      workflow_name: "同步日报",
      report_type: "daily",
      title: "同步日报",
      content: "# 同步日报\n\n正文",
      completed_at: "2026-08-23T08:00:00+08:00",
      generated_at: "2026-08-23T08:00:00+08:00",
      episode_count: 1,
      episodes: [
        {
          episode_id: 173,
          order: 1,
          podcast_id: 17,
          podcast_title: "同一节目",
          episode_title: "跨区域同步单集",
          context: "可核对 Show Notes",
          decision_state: "pending",
          queue_state: null,
        },
      ],
    },
  ],
  history: [],
};

function mockDiscoveryData() {
  const candidatesMutate = vi.fn().mockResolvedValue(undefined);
  const reportsMutate = vi.fn().mockResolvedValue(undefined);
  useSWRMock.mockImplementation((key: string) =>
    key.includes("/discovery/reports")
      ? {
          data: reports,
          error: null,
          isValidating: false,
          mutate: reportsMutate,
        }
      : {
          data: [candidate],
          error: null,
          isValidating: false,
          mutate: candidatesMutate,
        },
  );
  return { candidatesMutate, reportsMutate };
}

function queueResponse(queueState: "inbox" | null) {
  return {
    data: {
      data: {
        episode_id: 173,
        queue_state: queueState,
        queue_updated_at: "2026-08-23T08:10:00+08:00",
        read_at: candidate.read_at,
      },
    },
  };
}

describe("Discovery Inbox toggle integration", () => {
  beforeEach(() => {
    useSWRMock.mockReset();
    apiPutMock.mockReset();
    apiDeleteMock.mockReset();
    apiPostMock.mockReset();
    revalidateConsumptionSummaryMock.mockReset();
    window.history.replaceState({}, "");
  });

  it("round-trips one episode through all three entry points and filters", async () => {
    mockDiscoveryData();
    apiPutMock.mockResolvedValue(queueResponse("inbox"));
    apiDeleteMock.mockResolvedValue(queueResponse(null));

    render(<DiscoveryPageClient initialCandidates={[candidate]} />);

    const report = screen.getByRole("region", { name: "精选报告" });
    const recent = screen.getByRole("region", { name: "工作流最近更新" });
    fireEvent.click(
      within(recent).getByRole("button", {
        name: "预读 跨区域同步单集",
      }),
    );
    const preview = screen.getByRole("dialog", {
      name: "跨区域同步单集",
    });
    expect(screen.getAllByRole("button", { name: "收集到 Inbox" })).toHaveLength(
      3,
    );

    fireEvent.click(
      within(recent).getByRole("button", { name: "收集到 Inbox" }),
    );
    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: "从 Inbox 移除" }),
      ).toHaveLength(3);
    });
    for (const toggle of screen.getAllByRole("button", {
      name: "从 Inbox 移除",
    })) {
      expect(toggle).toBeEnabled();
      expect(toggle).toHaveAttribute("aria-pressed", "true");
      expect(
        toggle.querySelector(".tabler-icon-bookmark-filled"),
      ).toBeInTheDocument();
    }
    expect(apiPutMock).toHaveBeenCalledWith(
      "/api/v1/consumption/episodes/173/queue",
      { queue_state: "inbox" },
    );

    fireEvent.click(
      within(preview).getByRole("button", { name: "从 Inbox 移除" }),
    );
    await waitFor(() => {
      expect(
        screen.getAllByRole("button", { name: "收集到 Inbox" }),
      ).toHaveLength(3);
    });
    expect(apiDeleteMock).toHaveBeenCalledWith(
      "/api/v1/consumption/episodes/173/queue",
    );

    fireEvent.click(
      within(report).getByRole("button", { name: "收集到 Inbox" }),
    );
    await waitFor(() => {
      expect(
        within(report).getByRole("button", { name: "从 Inbox 移除" }),
      ).toBeEnabled();
    });
    fireEvent.click(
      document.querySelector<HTMLButtonElement>(".discovery-preview-close")!,
    );
    fireEvent.click(screen.getByRole("button", { name: /未收集/ }));
    expect(
      within(recent).queryByRole("button", {
        name: "预读 跨区域同步单集",
      }),
    ).not.toBeInTheDocument();

    fireEvent.click(
      within(report).getByRole("button", { name: "从 Inbox 移除" }),
    );
    await waitFor(() => {
      expect(
        within(recent).getByRole("button", {
          name: "预读 跨区域同步单集",
        }),
      ).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: /未收集/ })).toHaveTextContent(
      "1",
    );
    expect(revalidateConsumptionSummaryMock).toHaveBeenCalledTimes(4);
  });

  it("rolls back failed collection and removal before allowing retry", async () => {
    mockDiscoveryData();
    apiPutMock
      .mockRejectedValueOnce(new Error("collect failed"))
      .mockResolvedValue(queueResponse("inbox"));
    apiDeleteMock
      .mockRejectedValueOnce(new Error("remove failed"))
      .mockResolvedValue(queueResponse(null));

    render(<DiscoveryPageClient initialCandidates={[candidate]} />);

    const report = screen.getByRole("region", { name: "精选报告" });
    const recent = screen.getByRole("region", { name: "工作流最近更新" });
    fireEvent.click(
      within(report).getByRole("button", { name: "收集到 Inbox" }),
    );
    await waitFor(() => {
      expect(within(report).getByRole("alert")).toHaveTextContent(
        "收集失败，已恢复原状态，可重试。",
      );
    });
    expect(
      within(report).getByRole("button", { name: "收集到 Inbox" }),
    ).toBeEnabled();
    expect(
      within(recent).getByRole("button", { name: "收集到 Inbox" }),
    ).toBeEnabled();
    expect(revalidateConsumptionSummaryMock).not.toHaveBeenCalled();

    fireEvent.click(
      within(report).getByRole("button", { name: "收集到 Inbox" }),
    );
    await waitFor(() => {
      expect(
        within(recent).getByRole("button", { name: "从 Inbox 移除" }),
      ).toBeEnabled();
    });
    fireEvent.click(
      within(recent).getByRole("button", {
        name: "预读 跨区域同步单集",
      }),
    );
    const preview = screen.getByRole("dialog", {
      name: "跨区域同步单集",
    });
    fireEvent.click(
      within(preview).getByRole("button", { name: "从 Inbox 移除" }),
    );
    await waitFor(() => {
      expect(within(preview).getByRole("alert")).toHaveTextContent(
        "移除失败，已恢复服务端原状态，可重试。",
      );
    });
    expect(
      within(preview).getByRole("button", { name: "从 Inbox 移除" }),
    ).toBeEnabled();
    expect(
      within(report).getByRole("button", { name: "从 Inbox 移除" }),
    ).toBeEnabled();

    fireEvent.click(
      within(report).getByRole("button", { name: "从 Inbox 移除" }),
    );
    await waitFor(() => {
      expect(
        within(preview).getByRole("button", { name: "收集到 Inbox" }),
      ).toBeEnabled();
    });
    expect(revalidateConsumptionSummaryMock).toHaveBeenCalledTimes(2);
  });
});
