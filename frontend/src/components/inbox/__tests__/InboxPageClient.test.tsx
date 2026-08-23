import {
  act,
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
  ConsumptionQueuePayload,
  ConsumptionSummary,
} from "@/types/consumption";

const apiMocks = vi.hoisted(() => ({
  getSummary: vi.fn(),
  listQueue: vi.fn(),
  getItem: vi.fn(),
  setQueue: vi.fn(),
  placeQueue: vi.fn(),
  markInProgress: vi.fn(),
  getConsumptionErrorDetails: vi.fn((error: unknown) => ({
    message: error instanceof Error ? error.message : "请求失败",
  })),
  requiresFocusConfirmation: vi.fn(() => false),
  isQueueOrderConflict: vi.fn(() => false),
}));

const dndMocks = vi.hoisted(() => ({
  onDragStart: undefined as ((event: unknown) => void) | undefined,
  onDragMove: undefined as ((event: unknown) => void) | undefined,
  onDragOver: undefined as ((event: unknown) => void) | undefined,
  onDragEnd: undefined as ((event: unknown) => void) | undefined,
  onDragCancel: undefined as ((event: unknown) => void) | undefined,
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
    placeQueue: apiMocks.placeQueue,
    markInProgress: apiMocks.markInProgress,
  },
  getConsumptionErrorDetails: apiMocks.getConsumptionErrorDetails,
  requiresFocusConfirmation: apiMocks.requiresFocusConfirmation,
  isQueueOrderConflict: apiMocks.isQueueOrderConflict,
}));

vi.mock("@dnd-kit/core", async () => {
  const React = await import("react");
  return {
    DndContext: ({
      children,
      onDragStart,
      onDragMove,
      onDragOver,
      onDragEnd,
      onDragCancel,
    }: {
      children: React.ReactNode;
      onDragStart?: (event: unknown) => void;
      onDragMove?: (event: unknown) => void;
      onDragOver?: (event: unknown) => void;
      onDragEnd?: (event: unknown) => void;
      onDragCancel?: (event: unknown) => void;
    }) => {
      dndMocks.onDragStart = onDragStart;
      dndMocks.onDragMove = onDragMove;
      dndMocks.onDragOver = onDragOver;
      dndMocks.onDragEnd = onDragEnd;
      dndMocks.onDragCancel = onDragCancel;
      return React.createElement("div", { "data-testid": "dnd-context" }, children);
    },
    DragOverlay: ({ children }: { children: React.ReactNode }) =>
      React.createElement(React.Fragment, null, children),
    MouseSensor: class MouseSensor {},
    TouchSensor: class TouchSensor {},
    closestCorners: vi.fn(() => []),
    pointerWithin: vi.fn(() => []),
    useSensor: vi.fn(() => ({})),
    useSensors: vi.fn((...sensors: unknown[]) => sensors),
    useDroppable: vi.fn(() => ({ isOver: false, setNodeRef: () => undefined })),
  };
});

vi.mock("@dnd-kit/sortable", async () => {
  const React = await import("react");
  return {
    SortableContext: ({ children }: { children: React.ReactNode }) =>
      React.createElement(React.Fragment, null, children),
    useSortable: vi.fn(() => ({
      attributes: {},
      listeners: {},
      setActivatorNodeRef: () => undefined,
      setNodeRef: () => undefined,
      transform: null,
      transition: undefined,
      isDragging: false,
    })),
    verticalListSortingStrategy: vi.fn(),
  };
});

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

function queuePayload(queue: ConsumptionQueue): ConsumptionQueuePayload {
  return {
    queue_state: queue,
    revision: 1,
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

let dragMediaMatches = false;
const dragMediaListeners = new Set<(event: MediaQueryListEvent) => void>();

function installDragMediaQuery() {
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: vi.fn(() => ({
      get matches() {
        return dragMediaMatches;
      },
      media: "(min-width: 900px) and (orientation: landscape)",
      onchange: null,
      addEventListener: (_type: string, listener: (event: MediaQueryListEvent) => void) => {
        dragMediaListeners.add(listener);
      },
      removeEventListener: (_type: string, listener: (event: MediaQueryListEvent) => void) => {
        dragMediaListeners.delete(listener);
      },
      addListener: (listener: (event: MediaQueryListEvent) => void) => {
        dragMediaListeners.add(listener);
      },
      removeListener: (listener: (event: MediaQueryListEvent) => void) => {
        dragMediaListeners.delete(listener);
      },
      dispatchEvent: () => true,
    })),
  });
}

function setDragMediaMatches(matches: boolean) {
  dragMediaMatches = matches;
  const event = { matches } as MediaQueryListEvent;
  for (const listener of dragMediaListeners) listener(event);
}

function dragEvent({
  source,
  activeEpisodeId,
  target,
  overEpisodeId,
  activeTop = 0,
  overTop = 0,
}: {
  source: ConsumptionQueue;
  activeEpisodeId: number;
  target: ConsumptionQueue;
  overEpisodeId?: number;
  activeTop?: number;
  overTop?: number;
}) {
  return {
    active: {
      data: {
        current: { kind: "item", queue: source, episodeId: activeEpisodeId },
      },
      rect: {
        current: {
          initial: { top: activeTop, height: 40 },
          translated: { top: activeTop, height: 40 },
        },
      },
    },
    over: {
      data: {
        current: overEpisodeId
          ? { kind: "item", queue: target, episodeId: overEpisodeId }
          : { kind: "queue", queue: target },
      },
      rect: { top: overTop, height: 40 },
    },
    delta: { x: 0, y: 0 },
  };
}

describe("InboxPageClient", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    dragMediaMatches = false;
    dragMediaListeners.clear();
    installDragMediaQuery();
    dndMocks.onDragStart = undefined;
    dndMocks.onDragMove = undefined;
    dndMocks.onDragOver = undefined;
    dndMocks.onDragEnd = undefined;
    dndMocks.onDragCancel = undefined;
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
    apiMocks.placeQueue.mockResolvedValue({ queues: {} });
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

  it("uses the canonical server order instead of queue activity timestamps", async () => {
    const serverFirst: ConsumptionItem = {
      ...inboxItem,
      episode_id: 102,
      episode_title: "服务端首项",
      queue_updated_at: "2026-08-01T08:00:00Z",
    };
    apiMocks.listQueue.mockImplementation(async (queue: ConsumptionQueue) => ({
      queue_state: queue,
      revision: 6,
      items: queue === "inbox" ? [serverFirst, inboxItem] : [],
    }));

    render(<InboxPageClient />);

    const cards = await within(queueSection("inbox")).findAllByRole("button", {
      name: /打开 .* 明细/,
    });
    expect(cards.map((card) => card.getAttribute("aria-label"))).toEqual([
      "打开 服务端首项 明细",
      "打开 可处理单集 明细",
    ]);
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

  it("mounts the move menu outside its card stacking context", async () => {
    render(<InboxPageClient />);
    const trigger = await screen.findByRole("button", {
      name: "将 可处理单集 移动到其他队列",
    });
    const card = trigger.closest("article");

    fireEvent.click(trigger);

    const menu = screen.getByRole("menu", { name: "移动 可处理单集" });
    expect(menu.parentElement).toBe(document.body);
    expect(card).not.toContainElement(menu);

    fireEvent.pointerDown(menu);
    expect(menu).toBeInTheDocument();

    fireEvent.pointerDown(document.body);
    await waitFor(() =>
      expect(
        screen.queryByRole("menu", { name: "移动 可处理单集" }),
      ).toBeNull(),
    );
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

  it("仅在宽屏横屏显示独立拖拽把手", async () => {
    render(<InboxPageClient />);
    await screen.findByText("可处理单集");

    expect(
      screen.queryByRole("button", { name: "拖动《可处理单集》调整队列" }),
    ).toBeNull();

    act(() => setDragMediaMatches(true));
    expect(
      await screen.findByRole("button", {
        name: "拖动《可处理单集》调整队列",
      }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("dnd-context")).toBeInTheDocument();

    act(() => setDragMediaMatches(false));
    await waitFor(() => {
      expect(
        screen.queryByRole("button", {
          name: "拖动《可处理单集》调整队列",
        }),
      ).toBeNull();
    });
  });

  it("同泳道拖放按目标卡后方保存规范顺序", async () => {
    const first: ConsumptionItem = {
      ...inboxItem,
      episode_id: 201,
      episode_title: "第一项",
    };
    const second: ConsumptionItem = {
      ...inboxItem,
      episode_id: 202,
      episode_title: "第二项",
    };
    apiMocks.getSummary.mockResolvedValue({
      ...emptySummary,
      counts: { ...emptySummary.counts, inbox: 2 },
    });
    apiMocks.listQueue.mockImplementation(async (queue: ConsumptionQueue) => ({
      queue_state: queue,
      revision: queue === "inbox" ? 6 : 1,
      items: queue === "inbox" ? [first, second] : [],
    }));
    apiMocks.placeQueue.mockResolvedValue({
      queues: {
        inbox: { queue_state: "inbox", revision: 7, items: [second, first] },
      },
    });
    dragMediaMatches = true;

    render(<InboxPageClient />);
    await screen.findByRole("button", { name: "拖动《第一项》调整队列" });
    await waitFor(() => expect(dndMocks.onDragStart).toBeDefined());

    act(() =>
      dndMocks.onDragStart?.(
        dragEvent({
          source: "inbox",
          activeEpisodeId: first.episode_id,
          target: "inbox",
        }),
      ),
    );
    act(() =>
      dndMocks.onDragOver?.(
        dragEvent({
          source: "inbox",
          activeEpisodeId: first.episode_id,
          target: "inbox",
          overEpisodeId: second.episode_id,
          activeTop: 100,
          overTop: 0,
        }),
      ),
    );
    act(() =>
      dndMocks.onDragEnd?.(
        dragEvent({
          source: "inbox",
          activeEpisodeId: first.episode_id,
          target: "inbox",
          overEpisodeId: second.episode_id,
          activeTop: 100,
          overTop: 0,
        }),
      ),
    );

    await waitFor(() => {
      expect(apiMocks.placeQueue).toHaveBeenCalledWith(first.episode_id, {
        queue_state: "inbox",
        before_episode_id: null,
        expected_revisions: { inbox: 6 },
        acknowledge_focus_limit: false,
      });
    });
    const cards = within(queueSection("inbox")).getAllByRole("button", {
      name: /打开 .* 明细/,
    });
    expect(cards.map((card) => card.getAttribute("aria-label"))).toEqual([
      "打开 第二项 明细",
      "打开 第一项 明细",
    ]);
    expect(apiMocks.setQueue).not.toHaveBeenCalled();
  });

  it("支持跨泳道落入空队列", async () => {
    apiMocks.placeQueue.mockResolvedValue({
      queues: {
        inbox: { queue_state: "inbox", revision: 2, items: [] },
        done: {
          queue_state: "done",
          revision: 2,
          items: [{ ...inboxItem, queue_state: "done", in_progress_at: undefined }],
        },
      },
    });
    dragMediaMatches = true;

    render(<InboxPageClient />);
    await screen.findByRole("button", { name: "拖动《可处理单集》调整队列" });
    act(() =>
      dndMocks.onDragStart?.(
        dragEvent({ source: "inbox", activeEpisodeId: 101, target: "inbox" }),
      ),
    );
    act(() =>
      dndMocks.onDragEnd?.(
        dragEvent({ source: "inbox", activeEpisodeId: 101, target: "done" }),
      ),
    );

    await waitFor(() => {
      expect(apiMocks.placeQueue).toHaveBeenCalledWith(101, {
        queue_state: "done",
        before_episode_id: null,
        expected_revisions: { inbox: 1, done: 1 },
        acknowledge_focus_limit: false,
      });
    });
    expect(within(queueSection("done")).getByText("可处理单集")).toBeInTheDocument();
  });

  it("拖放成功后不让旧的队列加载结果覆盖规范顺序", async () => {
    const doneItem: ConsumptionItem = {
      ...inboxItem,
      episode_id: 202,
      episode_title: "已有完成项",
      queue_state: "done",
    };
    let resolveStaleDone!: (payload: ConsumptionQueuePayload) => void;
    const staleDone = new Promise<ConsumptionQueuePayload>((resolve) => {
      resolveStaleDone = resolve;
    });
    let doneLoadCount = 0;
    apiMocks.getSummary.mockResolvedValue({
      ...emptySummary,
      counts: { ...emptySummary.counts, done: 1 },
    });
    apiMocks.listQueue.mockImplementation((queue: ConsumptionQueue) => {
      if (queue === "done") {
        doneLoadCount += 1;
        if (doneLoadCount === 2) return staleDone;
        return Promise.resolve({
          queue_state: "done",
          revision: 1,
          items: [doneItem],
        });
      }
      return Promise.resolve({
        queue_state: queue,
        revision: 1,
        items: queue === "inbox" ? [inboxItem] : [],
      });
    });
    apiMocks.setQueue.mockResolvedValue({ ...doneItem, queue_state: "inbox" });
    apiMocks.placeQueue.mockResolvedValue({
      queues: {
        inbox: { queue_state: "inbox", revision: 2, items: [] },
        done: {
          queue_state: "done",
          revision: 2,
          items: [{ ...inboxItem, queue_state: "done" }],
        },
      },
    });
    dragMediaMatches = true;

    render(<InboxPageClient />);
    await screen.findByRole("button", { name: "拖动《可处理单集》调整队列" });
    await within(queueSection("done")).findByText("已有完成项");

    fireEvent.click(
      screen.getByRole("button", { name: "将 已有完成项 移动到其他队列" }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "移至 Inbox" }));
    await waitFor(() => expect(doneLoadCount).toBe(2));

    act(() =>
      dndMocks.onDragStart?.(
        dragEvent({ source: "inbox", activeEpisodeId: 101, target: "inbox" }),
      ),
    );
    act(() =>
      dndMocks.onDragEnd?.(
        dragEvent({ source: "inbox", activeEpisodeId: 101, target: "done" }),
      ),
    );

    await waitFor(() => {
      expect(apiMocks.placeQueue).toHaveBeenCalledWith(101, {
        queue_state: "done",
        before_episode_id: null,
        expected_revisions: { inbox: 1, done: 1 },
        acknowledge_focus_limit: false,
      });
    });
    expect(within(queueSection("done")).getByText("可处理单集")).toBeInTheDocument();

    await act(async () => {
      resolveStaleDone({ queue_state: "done", revision: 1, items: [] });
      await Promise.resolve();
    });

    expect(within(queueSection("done")).getByText("可处理单集")).toBeInTheDocument();
  });

  it("队列版本冲突时恢复并提示重新拖放", async () => {
    apiMocks.placeQueue.mockRejectedValueOnce(new Error("过期布局"));
    apiMocks.isQueueOrderConflict.mockReturnValueOnce(true);
    dragMediaMatches = true;

    render(<InboxPageClient />);
    await screen.findByRole("button", { name: "拖动《可处理单集》调整队列" });
    const initialQueueLoads = apiMocks.listQueue.mock.calls.length;
    act(() =>
      dndMocks.onDragStart?.(
        dragEvent({ source: "inbox", activeEpisodeId: 101, target: "inbox" }),
      ),
    );
    act(() =>
      dndMocks.onDragEnd?.(
        dragEvent({ source: "inbox", activeEpisodeId: 101, target: "done" }),
      ),
    );

    expect(
      await screen.findByText("队列顺序已在另一设备修改，请重新拖放。"),
    ).toBeInTheDocument();
    expect(within(queueSection("inbox")).getByText("可处理单集")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "重试移动 可处理单集" }),
    ).toBeNull();
    await waitFor(() => {
      expect(apiMocks.listQueue).toHaveBeenCalledTimes(initialQueueLoads + 4);
    });
  });

  it("Focus 满额时确认前不移动，确认后以最新队列版本保存", async () => {
    const focusItems = Array.from({ length: 7 }, (_, index) => ({
      ...inboxItem,
      episode_id: 301 + index,
      episode_title: `Focus ${index + 1}`,
      queue_state: "focus" as const,
    }));
    apiMocks.getSummary.mockResolvedValue({
      ...emptySummary,
      counts: { ...emptySummary.counts, focus: 7 },
    });
    apiMocks.listQueue.mockImplementation(async (queue: ConsumptionQueue) => ({
      queue_state: queue,
      revision: queue === "focus" ? 9 : 4,
      items: queue === "inbox" ? [inboxItem] : queue === "focus" ? focusItems : [],
    }));
    apiMocks.getItem.mockResolvedValue(inboxItem);
    apiMocks.placeQueue.mockResolvedValue({
      queues: {
        inbox: { queue_state: "inbox", revision: 5, items: [] },
        focus: {
          queue_state: "focus",
          revision: 10,
          items: [...focusItems, { ...inboxItem, queue_state: "focus" }],
        },
      },
    });
    dragMediaMatches = true;

    render(<InboxPageClient />);
    await screen.findByRole("button", { name: "拖动《可处理单集》调整队列" });
    act(() =>
      dndMocks.onDragStart?.(
        dragEvent({ source: "inbox", activeEpisodeId: 101, target: "inbox" }),
      ),
    );
    act(() =>
      dndMocks.onDragEnd?.(
        dragEvent({ source: "inbox", activeEpisodeId: 101, target: "focus" }),
      ),
    );

    expect(
      await screen.findByRole("alertdialog", { name: "Focus 已有 7 项" }),
    ).toBeInTheDocument();
    expect(apiMocks.placeQueue).not.toHaveBeenCalled();
    expect(within(queueSection("inbox")).getByText("可处理单集")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "仍加入 Focus" }));
    await waitFor(() => {
      expect(apiMocks.placeQueue).toHaveBeenCalledWith(101, {
        queue_state: "focus",
        before_episode_id: null,
        expected_revisions: { inbox: 4, focus: 9 },
        acknowledge_focus_limit: true,
      });
    });
    expect(apiMocks.getItem).toHaveBeenCalledWith(101);
  });

  it("Focus 确认不覆盖另一设备已改变的归属", async () => {
    const focusItems = Array.from({ length: 7 }, (_, index) => ({
      ...inboxItem,
      episode_id: 301 + index,
      episode_title: `Focus ${index + 1}`,
      queue_state: "focus" as const,
    }));
    const remotelyMoved = { ...inboxItem, queue_state: "someday" as const };
    let remoteMoveObserved = false;
    apiMocks.getSummary.mockResolvedValue({
      ...emptySummary,
      counts: { ...emptySummary.counts, focus: 7 },
    });
    apiMocks.listQueue.mockImplementation(async (queue: ConsumptionQueue) => ({
      queue_state: queue,
      revision: queue === "focus" ? 9 : 4,
      items:
        queue === "inbox"
          ? remoteMoveObserved
            ? []
            : [inboxItem]
          : queue === "focus"
            ? focusItems
            : queue === "someday" && remoteMoveObserved
              ? [remotelyMoved]
              : [],
    }));
    apiMocks.getItem.mockImplementation(async () => {
      remoteMoveObserved = true;
      return remotelyMoved;
    });
    dragMediaMatches = true;

    render(<InboxPageClient />);
    await screen.findByRole("button", { name: "拖动《可处理单集》调整队列" });
    act(() =>
      dndMocks.onDragStart?.(
        dragEvent({ source: "inbox", activeEpisodeId: 101, target: "inbox" }),
      ),
    );
    act(() =>
      dndMocks.onDragEnd?.(
        dragEvent({ source: "inbox", activeEpisodeId: 101, target: "focus" }),
      ),
    );
    await screen.findByRole("alertdialog", { name: "Focus 已有 7 项" });

    fireEvent.click(screen.getByRole("button", { name: "仍加入 Focus" }));

    expect(
      await screen.findByText("队列顺序已在另一设备修改，请重新拖放。"),
    ).toBeInTheDocument();
    expect(apiMocks.placeQueue).not.toHaveBeenCalled();
    expect(within(queueSection("someday")).getByText("可处理单集")).toBeInTheDocument();
  });
});
