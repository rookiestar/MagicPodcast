import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { navigate } from "@/lib/navigation";
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

function historyRow(title: string) {
  return screen.getByRole("heading", { name: title }).closest("article") as HTMLElement;
}

function openReprocessMenu(rowTitle: string) {
  fireEvent.click(
    within(historyRow(rowTitle)).getByRole("button", {
      name: `《${rowTitle}》的更多操作`,
    }),
  );
  return screen.getByRole("menu", { name: `重新处理《${rowTitle}》` });
}

describe("CompletionHistoryPageClient", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.history.replaceState({}, "", "/inbox/history");
    apiMocks.setQueue.mockImplementation(
      async (_episodeId: number, queue: ConsumptionQueue) => ({
        queue_state: queue,
      }),
    );
  });

  it("restores committed URL searches and never queries unsubmitted text", async () => {
    window.history.replaceState({}, "", "/inbox/history?q=Codex");
    apiMocks.listCompletionHistory.mockImplementation(async ({ query }: { query: string }) =>
      payload([historyItem(12, "done", query || "全部历史")], { search_query: query }));
    render(<CompletionHistoryPageClient />);
    await screen.findByRole("heading", { name: "Codex" });
    const input = screen.getByPlaceholderText("搜索单集或节目…");
    expect(input).toHaveValue("Codex");
    fireEvent.change(input, { target: { value: "尚未提交" } });
    expect(window.location.search).toBe("?q=Codex");
    expect(apiMocks.listCompletionHistory).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "搜索" }));
    await screen.findByRole("heading", { name: "尚未提交" });
    expect(new URLSearchParams(window.location.search).get("q")).toBe("尚未提交");
    act(() => { navigate("/inbox/history?q=Codex"); });
    await screen.findByRole("heading", { name: "Codex" });
    expect(input).toHaveValue("Codex");
    expect(apiMocks.setQueue).not.toHaveBeenCalled();
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

  it("shows the compact total and separates status expressions per state", async () => {
    apiMocks.listCompletionHistory.mockResolvedValue(
      payload([
        historyItem(1, "inbox", "行动中的历史"),
        historyItem(2, "done", "已完成的历史"),
        historyItem(3, "dismissed", "不感兴趣的历史"),
        historyItem(4, "unassigned", "未安排的历史"),
      ], {
        total_count: 57,
        match_count: 57,
      }),
    );

    render(<CompletionHistoryPageClient />);

    expect(await screen.findByText("57 个单集")).toBeInTheDocument();
    // 默认视图不重复展示总数。
    expect(screen.queryByText(/按最近完成时间排列 ·/)).toBeNull();

    const inboxLocate = within(historyRow("行动中的历史")).getByRole("link", {
      name: "已在 Inbox",
    });
    expect(inboxLocate).toHaveAttribute("href", "/inbox?queue=inbox&episode=1");

    const doneRow = historyRow("已完成的历史");
    expect(within(doneRow).queryByText(/当前 Done/)).toBeNull();
    expect(
      within(doneRow).getByRole("button", { name: "《已完成的历史》的更多操作" }),
    ).toHaveAttribute("aria-haspopup", "menu");

    expect(
      within(historyRow("不感兴趣的历史")).getByText("当前不感兴趣"),
    ).toBeInTheDocument();
    expect(
      within(historyRow("未安排的历史")).getByText("当前未安排"),
    ).toBeInTheDocument();
  });

  it("reprocesses from the menu into Inbox and keeps the completion record", async () => {
    const dismissed = historyItem(2, "dismissed", "不感兴趣的历史");
    apiMocks.listCompletionHistory.mockResolvedValue(payload([dismissed]));

    render(<CompletionHistoryPageClient />);
    await screen.findByText("不感兴趣的历史");

    openReprocessMenu("不感兴趣的历史");
    fireEvent.click(screen.getByRole("menuitem", { name: "加入 Inbox" }));

    await waitFor(() =>
      expect(apiMocks.setQueue).toHaveBeenCalledWith(2, "inbox", {
        acknowledgeFocusLimit: false,
      }),
    );
    const row = historyRow("不感兴趣的历史");
    expect(
      await within(row).findByRole("link", { name: "已在 Inbox" }),
    ).toHaveAttribute("href", "/inbox?queue=inbox&episode=2");
    expect(within(row).getByText(/最近完成于/)).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: "已在 Inbox" })).toHaveFocus();

    // 队列记录不再提供重新处理菜单，只保留定位。
    expect(
      within(row).queryByRole("button", { name: /的更多操作/ }),
    ).toBeNull();
  });

  it("blocks duplicate submissions while a reprocess is saving", async () => {
    let resolveQueue!: (value: { queue_state: ConsumptionQueue }) => void;
    apiMocks.setQueue.mockImplementation(
      () =>
        new Promise<{ queue_state: ConsumptionQueue }>((resolve) => {
          resolveQueue = resolve;
        }),
    );
    apiMocks.listCompletionHistory.mockResolvedValue(
      payload([historyItem(9, "done", "保存中的历史")]),
    );

    render(<CompletionHistoryPageClient />);
    await screen.findByText("保存中的历史");

    openReprocessMenu("保存中的历史");
    fireEvent.click(screen.getByRole("menuitem", { name: "加入 Someday" }));
    await waitFor(() => expect(apiMocks.setQueue).toHaveBeenCalledTimes(1));

    const trigger = within(historyRow("保存中的历史")).getByRole("button", {
      name: "正在保存《保存中的历史》的队列调整",
    });
    expect(trigger).toHaveAttribute("aria-disabled", "true");
    // 保存中触发按钮不打开菜单，也不会重复提交。
    fireEvent.click(trigger);
    expect(screen.queryByRole("menu")).toBeNull();
    expect(apiMocks.setQueue).toHaveBeenCalledTimes(1);

    resolveQueue({ queue_state: "someday" });
    expect(
      await within(historyRow("保存中的历史")).findByRole("link", {
        name: "已在 Someday",
      }),
    ).toBeInTheDocument();
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
    fireEvent.click(screen.getByRole("button", { name: "搜索" }));

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
    openReprocessMenu("需要确认 Focus");
    fireEvent.click(screen.getByRole("menuitem", { name: "加入 Focus" }));

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
      await within(historyRow("需要确认 Focus")).findByRole("link", {
        name: "已在 Focus",
      }),
    ).toBeInTheDocument();
  });

  it("closes the Focus confirmation with Escape", async () => {
    const dismissed = historyItem(7, "dismissed", "键盘取消 Focus");
    apiMocks.listCompletionHistory.mockResolvedValue(payload([dismissed]));
    apiMocks.setQueue.mockRejectedValueOnce(new Error("focus confirmation"));
    apiMocks.requiresFocusConfirmation.mockReturnValueOnce(true);

    render(<CompletionHistoryPageClient />);
    await screen.findByText("键盘取消 Focus");
    openReprocessMenu("键盘取消 Focus");
    fireEvent.click(screen.getByRole("menuitem", { name: "加入 Focus" }));

    const dialog = await screen.findByRole("dialog", {
      name: "Focus 已有明确承诺",
    });
    fireEvent.keyDown(dialog.parentElement as HTMLElement, { key: "Escape" });

    await waitFor(() => expect(dialog).not.toBeInTheDocument());
    expect(within(historyRow("键盘取消 Focus")).getByRole("button", { name: /更多操作/ })).toHaveFocus();
  });
  it("restores menu focus on Escape and leaves focus outside on dismissal", async () => {
    apiMocks.listCompletionHistory.mockResolvedValue(payload([historyItem(10, "done", "键盘菜单")]));
    render(<CompletionHistoryPageClient />);
    await screen.findByText("键盘菜单");
    const menu = openReprocessMenu("键盘菜单");
    expect(screen.getByRole("menuitem", { name: "加入 Inbox" })).toHaveFocus();
    fireEvent.keyDown(menu, { key: "End" });
    expect(screen.getByRole("menuitem", { name: "加入 Someday" })).toHaveFocus();
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(screen.getByRole("menuitem", { name: "加入 Inbox" })).toHaveFocus();
    fireEvent.keyDown(menu, { key: "Escape" });
    expect(screen.queryByRole("menu")).toBeNull();
    expect(screen.getByRole("button", { name: /键盘菜单.*更多操作/ })).toHaveFocus();
    openReprocessMenu("键盘菜单");
    fireEvent.pointerDown(screen.getByRole("searchbox"));
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("keeps the record and allows retry after a queue failure", async () => {
    apiMocks.listCompletionHistory.mockResolvedValue(payload([historyItem(11, "done", "重试记录")]));
    apiMocks.setQueue.mockRejectedValueOnce(new Error("保存失败"))
      .mockResolvedValueOnce({ queue_state: "someday" });
    render(<CompletionHistoryPageClient />);
    await screen.findByText("重试记录");
    openReprocessMenu("重试记录");
    fireEvent.click(screen.getByRole("menuitem", { name: "加入 Someday" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("保存失败");
    expect(within(historyRow("重试记录")).queryByRole("link", { name: "已在 Someday" })).toBeNull();
    expect(screen.getByRole("button", { name: /重试记录.*更多操作/ })).toHaveFocus();
    openReprocessMenu("重试记录");
    fireEvent.click(screen.getByRole("menuitem", { name: "加入 Someday" }));
    expect(await screen.findByRole("link", { name: "已在 Someday" })).toHaveFocus();
  });

});
