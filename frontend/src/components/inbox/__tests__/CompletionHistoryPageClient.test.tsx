import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import CompletionHistoryPageClient from "../CompletionHistoryPageClient";
import type {
  CompletionHistoryItem,
  CompletionHistoryPayload,
  ConsumptionQueue,
} from "@/types/consumption";

const apiMocks = vi.hoisted(() => ({
  listCompletionHistory: vi.fn(),
  setQueue: vi.fn(),
  getConsumptionErrorDetails: vi.fn((error: unknown) => ({
    message: error instanceof Error ? error.message : "请求失败",
    currentCount: undefined as number | undefined,
    focusLimit: undefined as number | undefined,
  })),
  requiresFocusConfirmation: vi.fn(() => false),
}));

vi.mock("@/components/layout/PageLayout", () => ({
  default: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

vi.mock("@/components/ui/PlainImage", () => ({
  default: (props: React.ImgHTMLAttributes<HTMLImageElement>) => (
    // eslint-disable-next-line @next/next/no-img-element
    <img {...props} alt={props.alt ?? ""} />
  ),
}));

vi.mock("@/lib/imageOptimization", () => ({
  getOptimizedImageUrl: vi.fn(() => ""),
}));

vi.mock("@/lib/api/consumption", () => ({
  consumptionApi: {
    listCompletionHistory: apiMocks.listCompletionHistory,
    setQueue: apiMocks.setQueue,
  },
  getConsumptionErrorDetails: apiMocks.getConsumptionErrorDetails,
  requiresFocusConfirmation: apiMocks.requiresFocusConfirmation,
}));

const completedAt = "2026-08-23T08:30:00Z";

function historyItem(
  episodeId: number,
  status: CompletionHistoryItem["current_status"],
  title = `历史单集 ${episodeId}`,
): CompletionHistoryItem {
  return {
    episode_id: episodeId,
    podcast_id: 10,
    podcast_title: "历史节目",
    podcast_cover_url: "",
    episode_title: title,
    episode_no: String(episodeId),
    image_url: "",
    completed_at: completedAt,
    current_status: status,
  };
}

function payload(
  items: CompletionHistoryItem[],
  overrides: Partial<CompletionHistoryPayload> = {},
): CompletionHistoryPayload {
  return {
    items,
    total_count: items.length,
    match_count: items.length,
    has_more: false,
    search_query: "",
    ...overrides,
  };
}

function historyCard(title: string) {
  return screen.getByRole("heading", { name: title }).closest("article") as HTMLElement;
}

describe("CompletionHistoryPageClient", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMocks.setQueue.mockImplementation(
      async (_episodeId: number, queue: ConsumptionQueue) => ({
        queue_state: queue,
      }),
    );
  });

  it("does not expose a false empty state before the first request completes", async () => {
    let resolveHistory!: (value: CompletionHistoryPayload) => void;
    apiMocks.listCompletionHistory.mockReturnValueOnce(
      new Promise<CompletionHistoryPayload>((resolve) => {
        resolveHistory = resolve;
      }),
    );

    render(<CompletionHistoryPageClient />);

    expect(screen.getByText("正在加载完成历史…")).toBeInTheDocument();
    expect(screen.queryByText("还没有完成记录")).toBeNull();

    resolveHistory(payload([]));
    expect(await screen.findByText("还没有完成记录")).toBeInTheDocument();
  });

  it("shows total, current status, locate links, and defaults reprocessing to Inbox", async () => {
    const inbox = historyItem(1, "inbox", "行动中的历史");
    const dismissed = historyItem(2, "dismissed", "不感兴趣的历史");
    apiMocks.listCompletionHistory.mockResolvedValue(
      payload([inbox, dismissed], {
        total_count: 57,
        match_count: 57,
      }),
    );

    render(<CompletionHistoryPageClient />);

    expect(await screen.findByText("行动中的历史")).toBeInTheDocument();
    expect(screen.getByText("57")).toBeInTheDocument();
    const locate = within(historyCard("行动中的历史")).getByRole("link", {
      name: /定位到 Inbox/,
    });
    expect(locate).toHaveAttribute("href", "/inbox?queue=inbox&episode=1");

    const dismissedCard = historyCard("不感兴趣的历史");
    expect(
      within(dismissedCard).getByText("曾完成 · 当前不感兴趣"),
    ).toBeInTheDocument();
    expect(within(dismissedCard).getByLabelText("重新处理到")).toHaveValue(
      "inbox",
    );
    fireEvent.click(
      within(dismissedCard).getByRole("button", { name: "重新处理" }),
    );

    await waitFor(() =>
      expect(apiMocks.setQueue).toHaveBeenCalledWith(2, "inbox", {
        acknowledgeFocusLimit: false,
      }),
    );
    expect(
      await within(dismissedCard).findByText("曾完成 · 当前 Inbox"),
    ).toBeInTheDocument();
    expect(within(dismissedCard).getByText(/最近完成于/)).toBeInTheDocument();
  });

  it("keeps current records when a new server-side search fails", async () => {
    const current = historyItem(3, "done", "保留的历史");
    apiMocks.listCompletionHistory
      .mockResolvedValueOnce(payload([current]))
      .mockRejectedValueOnce(new Error("搜索服务暂不可用"));

    render(<CompletionHistoryPageClient />);
    expect(await screen.findByText("保留的历史")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("搜索单集或节目"), {
      target: { value: "目标节目" },
    });
    fireEvent.click(screen.getByRole("button", { name: "搜索全部历史" }));

    expect(
      await screen.findByText(/更新失败，当前记录仍可用/),
    ).toBeInTheDocument();
    expect(screen.getByText("保留的历史")).toBeInTheDocument();
    expect(apiMocks.listCompletionHistory).toHaveBeenLastCalledWith({
      query: "目标节目",
    });
  });

  it("keeps loaded records on page failure and retries without duplicates", async () => {
    const first = historyItem(4, "done", "第一页记录");
    const second = historyItem(5, "unassigned", "第二页记录");
    apiMocks.listCompletionHistory
      .mockResolvedValueOnce(
        payload([first], {
          total_count: 2,
          match_count: 2,
          has_more: true,
          next_cursor: "page-2",
        }),
      )
      .mockRejectedValueOnce(new Error("下一页超时"))
      .mockResolvedValueOnce(
        payload([first, second], {
          total_count: 2,
          match_count: 2,
        }),
      );

    render(<CompletionHistoryPageClient />);
    expect(await screen.findByText("第一页记录")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /继续加载/ }));

    expect(
      await screen.findByText(/下一页加载失败，已加载的 1 条记录保持可用/),
    ).toBeInTheDocument();
    expect(screen.getByText("第一页记录")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /重试加载下一页/ }));

    expect(await screen.findByText("第二页记录")).toBeInTheDocument();
    expect(screen.getAllByRole("article")).toHaveLength(2);
    expect(screen.queryByRole("button", { name: /继续加载/ })).toBeNull();
  });

  it("preserves the Focus soft-limit confirmation while reprocessing", async () => {
    const dismissed = historyItem(6, "dismissed", "需要确认 Focus");
    apiMocks.listCompletionHistory.mockResolvedValue(payload([dismissed]));
    apiMocks.setQueue
      .mockRejectedValueOnce(new Error("focus confirmation"))
      .mockResolvedValueOnce({ queue_state: "focus" });
    apiMocks.requiresFocusConfirmation.mockReturnValueOnce(true);
    apiMocks.getConsumptionErrorDetails.mockReturnValueOnce({
      message: "Focus soft limit confirmation required",
      currentCount: 7,
      focusLimit: 7,
    });

    render(<CompletionHistoryPageClient />);
    await screen.findByText("需要确认 Focus");
    const card = historyCard("需要确认 Focus");
    fireEvent.change(within(card).getByLabelText("重新处理到"), {
      target: { value: "focus" },
    });
    fireEvent.click(within(card).getByRole("button", { name: "重新处理" }));

    expect(
      await screen.findByRole("dialog", { name: "Focus 已有明确承诺" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "取消" })).toHaveFocus();
    fireEvent.click(screen.getByRole("button", { name: "仍然加入 Focus" }));

    await waitFor(() =>
      expect(apiMocks.setQueue).toHaveBeenLastCalledWith(6, "focus", {
        acknowledgeFocusLimit: true,
      }),
    );
    expect(
      await within(card).findByText("曾完成 · 当前 Focus"),
    ).toBeInTheDocument();
  });

  it("closes the Focus confirmation with Escape", async () => {
    const dismissed = historyItem(7, "dismissed", "键盘取消 Focus");
    apiMocks.listCompletionHistory.mockResolvedValue(payload([dismissed]));
    apiMocks.setQueue.mockRejectedValueOnce(new Error("focus confirmation"));
    apiMocks.requiresFocusConfirmation.mockReturnValueOnce(true);

    render(<CompletionHistoryPageClient />);
    await screen.findByText("键盘取消 Focus");
    const card = historyCard("键盘取消 Focus");
    fireEvent.change(within(card).getByLabelText("重新处理到"), {
      target: { value: "focus" },
    });
    fireEvent.click(within(card).getByRole("button", { name: "重新处理" }));

    const dialog = await screen.findByRole("dialog", {
      name: "Focus 已有明确承诺",
    });
    fireEvent.keyDown(dialog.parentElement as HTMLElement, { key: "Escape" });

    await waitFor(() => expect(dialog).not.toBeInTheDocument());
  });
});
