import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import InboxPageClient from "../InboxPageClient";
import type {
  ConsumptionItem,
  ConsumptionQueue,
  ConsumptionSummary,
} from "@/types/consumption";

const apiMocks = vi.hoisted(() => ({
  getSummary: vi.fn(),
  listQueue: vi.fn(),
  getItem: vi.fn(),
  setQueue: vi.fn(),
  markInProgress: vi.fn(),
  getConsumptionErrorDetails: vi.fn((error: unknown) => ({
    message: error instanceof Error ? error.message : "请求失败",
  })),
  requiresFocusConfirmation: vi.fn(() => false),
}));

vi.mock("@/components/layout/PageLayout", () => ({
  default: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

vi.mock("@/lib/api/consumption", () => ({
  consumptionApi: {
    getSummary: apiMocks.getSummary,
    listQueue: apiMocks.listQueue,
    getItem: apiMocks.getItem,
    setQueue: apiMocks.setQueue,
    markInProgress: apiMocks.markInProgress,
  },
  getConsumptionErrorDetails: apiMocks.getConsumptionErrorDetails,
  requiresFocusConfirmation: apiMocks.requiresFocusConfirmation,
}));

vi.mock("@/lib/api", () => ({
  episodeApi: {
    getNotes: vi.fn().mockResolvedValue(""),
    getTags: vi.fn().mockResolvedValue([]),
    updateNotes: vi.fn().mockResolvedValue(undefined),
    addTag: vi.fn().mockResolvedValue(undefined),
    removeTag: vi.fn().mockResolvedValue(undefined),
  },
  tagApi: {
    list: vi.fn().mockResolvedValue([]),
  },
}));

const inboxItem: ConsumptionItem = {
  episode_id: 101,
  podcast_id: 10,
  podcast_title: "声东击西",
  podcast_author: "声动活泼",
  podcast_cover_url: "",
  episode_title: "可处理单集",
  episode_no: "101",
  duration: 1800,
  published_date: "2026-08-11T08:00:00Z",
  show_notes: "<p>正文</p>",
  original_url: "https://example.com/101",
  image_url: "",
  notes: "",
  tags: [],
  queue_state: "inbox",
  queue_updated_at: "2026-08-11T08:00:00Z",
};

const emptySummary: ConsumptionSummary = {
  counts: { inbox: 1, focus: 0, someday: 0, done: 0 },
  focus_limit: 7,
  focus_over_limit: false,
};

function queuePayload(queue: ConsumptionQueue) {
  return {
    queue_state: queue,
    items: queue === "inbox" ? [inboxItem] : [],
  };
}

function queueSection(name: ConsumptionQueue) {
  return screen
    .getByRole("heading", {
      level: 2,
      name:
        name === "inbox"
          ? "Inbox"
          : name === "focus"
            ? "Focus"
            : name === "someday"
              ? "Someday"
              : "Done",
    })
    .closest("section") as HTMLElement;
}

describe("InboxPageClient", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMocks.getSummary.mockResolvedValue(emptySummary);
    apiMocks.listQueue.mockImplementation(async (queue: ConsumptionQueue) =>
      queuePayload(queue),
    );
    apiMocks.getItem.mockResolvedValue(inboxItem);
    apiMocks.setQueue.mockImplementation(
      async (_episodeId: number, queue: ConsumptionQueue) => ({
        ...inboxItem,
        queue_state: queue,
        in_progress_at: queue === "done" ? undefined : inboxItem.in_progress_at,
      }),
    );
  });

  it("loads four queues independently and keeps healthy queues visible when one fails", async () => {
    apiMocks.listQueue.mockImplementation(async (queue: ConsumptionQueue) => {
      if (queue === "someday") throw new Error("Someday 暂不可用");
      return queuePayload(queue);
    });

    render(<InboxPageClient />);

    expect(await screen.findByText("可处理单集")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Focus" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Done" })).toBeInTheDocument();
    expect(
      await screen.findByText("Someday 加载失败，不影响其他队列。"),
    ).toBeInTheDocument();
    expect(
      within(queueSection("inbox")).getByText("可处理单集"),
    ).toBeInTheDocument();
  });

  it("rolls a failed move back to the server-known queue and offers retry", async () => {
    apiMocks.setQueue.mockRejectedValueOnce(new Error("保存失败"));
    render(<InboxPageClient />);

    fireEvent.click(
      await screen.findByRole("button", {
        name: "将 可处理单集 标记 Done",
      }),
    );

    expect(
      await screen.findByText(/移动失败，已恢复原队列/),
    ).toBeInTheDocument();
    expect(
      within(queueSection("inbox")).getByText("可处理单集"),
    ).toBeInTheDocument();
    expect(within(queueSection("done")).queryByText("可处理单集")).toBeNull();
    expect(
      screen.getByRole("button", { name: "重试移动 可处理单集" }),
    ).toBeInTheDocument();
  });

  it("requires explicit confirmation before adding an eighth Focus item", async () => {
    apiMocks.getSummary.mockResolvedValue({
      ...emptySummary,
      counts: { ...emptySummary.counts, focus: 7 },
    });
    render(<InboxPageClient />);

    fireEvent.click(
      await screen.findByRole("button", {
        name: "将 可处理单集 移动到其他队列",
      }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "移至 Focus" }));

    expect(
      screen.getByRole("alertdialog", { name: "Focus 已有 7 项" }),
    ).toBeInTheDocument();
    expect(apiMocks.setQueue).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "仍加入 Focus" }));
    await waitFor(() => {
      expect(apiMocks.setQueue).toHaveBeenCalledWith(101, "focus", {
        acknowledgeFocusLimit: true,
      });
    });
  });

  it("returns focus to the originating card after closing detail", async () => {
    render(<InboxPageClient />);
    const trigger = await screen.findByRole("button", {
      name: "打开 可处理单集 明细",
    });

    fireEvent.click(trigger);
    expect(
      await screen.findByRole("dialog", { name: "可处理单集" }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "关闭单集明细" }));

    await waitFor(() => expect(trigger).toHaveFocus());
  });
});
