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
import type {
  EpisodeArtifactSet,
  ProcessingRun,
  ProcessingRunDetail,
} from "@/types/processing";

const apiMocks = vi.hoisted(() => ({
  getSummary: vi.fn(),
  listQueue: vi.fn(),
  getItem: vi.fn(),
  setQueue: vi.fn(),
  placeQueue: vi.fn(),
  undoCompletion: vi.fn(),
  markInProgress: vi.fn(),
  getConsumptionErrorDetails: vi.fn((error: unknown) => ({
    message: error instanceof Error ? error.message : "请求失败",
  })),
  requiresFocusConfirmation: vi.fn(() => false),
  isQueueOrderConflict: vi.fn(() => false),
  isCompletionUndoConflict: vi.fn(() => false),
  isCompletionUndoExpired: vi.fn(() => false),
  listEpisodeRuns: vi.fn(),
  getLatestAudio: vi.fn(),
  getScheduleStatus: vi.fn(),
  getProcessingRun: vi.fn(),
  getArtifactContent: vi.fn(),
  startProcessing: vi.fn(),
  cancelProcessing: vi.fn(),
  retryProcessing: vi.fn(),
  getCopilotContext: vi.fn(),
  askCopilot: vi.fn(),
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
    undoCompletion: apiMocks.undoCompletion,
    markInProgress: apiMocks.markInProgress,
  },
  getConsumptionErrorDetails: apiMocks.getConsumptionErrorDetails,
  requiresFocusConfirmation: apiMocks.requiresFocusConfirmation,
  isQueueOrderConflict: apiMocks.isQueueOrderConflict,
  isCompletionUndoConflict: apiMocks.isCompletionUndoConflict,
  isCompletionUndoExpired: apiMocks.isCompletionUndoExpired,
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
      return React.createElement(
        "div",
        { "data-testid": "dnd-context" },
        children,
      );
    },
    DragOverlay: ({ children }: { children: React.ReactNode }) =>
      React.createElement(React.Fragment, null, children),
    MouseSensor: class MouseSensor {},
    TouchSensor: class TouchSensor {},
    closestCorners: vi.fn(() => []),
    pointerWithin: vi.fn(() => []),
    useSensor: vi.fn(() => ({})),
    useSensors: vi.fn((...sensors: unknown[]) => sensors),
    useDraggable: vi.fn(() => ({
      attributes: {},
      listeners: {},
      setActivatorNodeRef: () => undefined,
      setNodeRef: () => undefined,
      transform: null,
      isDragging: false,
    })),
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

vi.mock("@/lib/api/processing", () => ({
  processingApi: {
    listEpisodeRuns: apiMocks.listEpisodeRuns,
    getLatestAudio: apiMocks.getLatestAudio,
    getScheduleStatus: apiMocks.getScheduleStatus,
    getRun: apiMocks.getProcessingRun,
    getArtifactContent: apiMocks.getArtifactContent,
    start: apiMocks.startProcessing,
    cancel: apiMocks.cancelProcessing,
    retry: apiMocks.retryProcessing,
  },
  getProcessingErrorDetails: vi.fn((error: unknown) => ({
    message: error instanceof Error ? error.message : "加工状态读取失败",
    status: (error as { response?: { status?: number } })?.response?.status,
  })),
}));

vi.mock("@/lib/api/episodeCopilot", () => ({
  episodeCopilotApi: {
    getContext: apiMocks.getCopilotContext,
    ask: apiMocks.askCopilot,
  },
  isEpisodeCopilotCancellation: vi.fn(() => false),
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
    has_more: false,
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
              : "最近完成",
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
      addEventListener: (
        _type: string,
        listener: (event: MediaQueryListEvent) => void,
      ) => {
        dragMediaListeners.add(listener);
      },
      removeEventListener: (
        _type: string,
        listener: (event: MediaQueryListEvent) => void,
      ) => {
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
    window.history.replaceState({}, "", "/inbox");
    HTMLElement.prototype.scrollIntoView = vi.fn();
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
    apiMocks.listEpisodeRuns.mockResolvedValue([]);
    apiMocks.getLatestAudio.mockRejectedValue({
      response: { status: 404 },
    });
    apiMocks.getScheduleStatus.mockResolvedValue({
      enabled: false,
      cron: "",
      timezone: "",
      batch_size: 0,
    });
    apiMocks.getArtifactContent.mockImplementation(
      (_artifactSetId: number, kind: string) =>
        Promise.resolve({
          kind,
          content: "",
          sha256: "a".repeat(64),
          media_available: false,
        }),
    );
    apiMocks.getCopilotContext.mockResolvedValue({
      episode_id: inboxItem.episode_id,
      show_notes_available: true,
      transcript_available: false,
      private_note_available: false,
    });
    apiMocks.askCopilot.mockResolvedValue(undefined);
    apiMocks.setQueue.mockImplementation(
      async (_episodeId: number, queue: ConsumptionQueue) => ({
        ...inboxItem,
        queue_state: queue,
        in_progress_at: queue === "done" ? undefined : inboxItem.in_progress_at,
      }),
    );
    apiMocks.placeQueue.mockResolvedValue({ queues: {} });
    apiMocks.undoCompletion.mockResolvedValue({ queues: {} });
  });

  it("links recent completions to the independent history view", async () => {
    render(<InboxPageClient />);

    const recent = queueSection("done");
    expect(
      await within(recent).findByRole("link", { name: "查看全部" }),
    ).toHaveAttribute("href", "/inbox/history");
  });

  it("locates and focuses an action-queue item linked from history", async () => {
    window.history.replaceState({}, "", "/inbox?queue=inbox&episode=101");
    render(<InboxPageClient />);

    const trigger = await screen.findByRole("button", {
      name: "打开 可处理单集 明细",
    });
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(HTMLElement.prototype.scrollIntoView).toHaveBeenCalledWith({
      behavior: "auto",
      block: "center",
      inline: "center",
    });
  });

  it("loads four queues independently and keeps healthy queues visible when one fails", async () => {
    apiMocks.listQueue.mockImplementation(async (queue: ConsumptionQueue) => {
      if (queue === "someday") throw new Error("Someday 暂不可用");
      return queuePayload(queue);
    });

    render(<InboxPageClient />);

    expect(await screen.findByText("可处理单集")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Focus" })).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "最近完成" }),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("Someday 加载失败，不影响其他队列。"),
    ).toBeInTheDocument();
    expect(
      within(queueSection("inbox")).getByText("可处理单集"),
    ).toBeInTheDocument();
  });

  it("keeps action queues usable while recent completions load or fail", async () => {
    let rejectDone!: (error: Error) => void;
    const pendingDone = new Promise<ConsumptionQueuePayload>(
      (_resolve, reject) => {
        rejectDone = reject;
      },
    );
    apiMocks.listQueue.mockImplementation((queue: ConsumptionQueue) =>
      queue === "done" ? pendingDone : Promise.resolve(queuePayload(queue)),
    );

    render(<InboxPageClient />);

    expect(
      await within(queueSection("inbox")).findByText("可处理单集"),
    ).toBeInTheDocument();
    expect(
      within(queueSection("done")).getByText("正在加载 最近完成…"),
    ).toBeInTheDocument();
    expect(
      within(queueSection("done")).queryByText("最近 7 天还没有完成的单集。"),
    ).toBeNull();

    rejectDone(new Error("最近完成暂不可用"));
    expect(
      await within(queueSection("done")).findByText(
        "最近完成 加载失败，不影响其他队列。",
      ),
    ).toBeInTheDocument();
    expect(
      within(queueSection("inbox")).getByText("可处理单集"),
    ).toBeInTheDocument();
  });

  it("shows the actual bounded recent-completion count, overflow, and completion time", async () => {
    const completedItem: ConsumptionItem = {
      ...inboxItem,
      queue_state: "done",
      completed_at: "2026-08-23T08:30:00Z",
    };
    apiMocks.getSummary.mockResolvedValue({
      ...emptySummary,
      counts: { ...emptySummary.counts, done: 37 },
    });
    apiMocks.listQueue.mockImplementation(async (queue: ConsumptionQueue) => ({
      ...queuePayload(queue),
      items: queue === "done" ? [completedItem] : queuePayload(queue).items,
      has_more: queue === "done",
    }));

    render(<InboxPageClient />);

    const recentSection = queueSection("done");
    expect(
      await within(recentSection).findByText("可处理单集"),
    ).toBeInTheDocument();
    expect(within(recentSection).getByLabelText("1 项")).toHaveTextContent("1");
    expect(
      within(recentSection).getByText("最近 7 天还有未展示的完成记录。"),
    ).toBeInTheDocument();
    expect(within(recentSection).getByText(/完成于 8\/23/)).toBeInTheDocument();
  });

  it("offers a page-session undo and restores the canonical queue position", async () => {
    let currentQueue: ConsumptionQueue = "inbox";
    const completedItem: ConsumptionItem = {
      ...inboxItem,
      queue_state: "done",
      completed_at: new Date().toISOString(),
      completion_undo: {
        token: "signed-page-session-token",
        expires_at: new Date(Date.now() + 15_000).toISOString(),
      },
    };
    apiMocks.listQueue.mockImplementation(async (queue: ConsumptionQueue) => ({
      queue_state: queue,
      revision: currentQueue === queue ? 2 : 1,
      items:
        currentQueue === queue
          ? [{ ...inboxItem, queue_state: currentQueue }]
          : [],
      has_more: false,
    }));
    apiMocks.setQueue.mockImplementation(async () => {
      currentQueue = "done";
      return completedItem;
    });
    apiMocks.undoCompletion.mockImplementation(async () => {
      currentQueue = "inbox";
      return {
        queues: {
          inbox: {
            queue_state: "inbox",
            revision: 3,
            items: [inboxItem],
            has_more: false,
          },
          done: {
            queue_state: "done",
            revision: 3,
            items: [],
            has_more: false,
          },
        },
      };
    });

    render(<InboxPageClient />);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "将 可处理单集 标记完成",
      }),
    );

    expect(
      await screen.findByText(/《可处理单集》已完成，\d+ 秒内可撤销。/),
    ).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "撤销完成 可处理单集" }),
    );

    await waitFor(() =>
      expect(apiMocks.undoCompletion).toHaveBeenCalledWith(
        101,
        "signed-page-session-token",
      ),
    );
    expect(
      await within(queueSection("inbox")).findByText("可处理单集"),
    ).toBeInTheDocument();
    expect(within(queueSection("done")).queryByText("可处理单集")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "撤销完成 可处理单集" }),
    ).toBeNull();
  });

  it("refreshes canonical state when completion undo conflicts", async () => {
    const completedItem: ConsumptionItem = {
      ...inboxItem,
      queue_state: "done",
      completion_undo: {
        token: "conflicting-token",
        expires_at: new Date(Date.now() + 15_000).toISOString(),
      },
    };
    apiMocks.setQueue.mockResolvedValue(completedItem);
    apiMocks.undoCompletion.mockRejectedValue(new Error("state changed"));
    apiMocks.isCompletionUndoConflict.mockReturnValue(true);

    render(<InboxPageClient />);
    const completeButton = await screen.findByRole("button", {
      name: "将 可处理单集 标记完成",
    });
    const initialLoads = apiMocks.listQueue.mock.calls.length;
    fireEvent.click(completeButton);
    await screen.findByRole("button", {
      name: "撤销完成 可处理单集",
    });
    await waitFor(() =>
      expect(apiMocks.listQueue).toHaveBeenCalledTimes(initialLoads + 2),
    );
    const loadsBeforeUndo = apiMocks.listQueue.mock.calls.length;
    fireEvent.click(
      screen.getByRole("button", {
        name: "撤销完成 可处理单集",
      }),
    );

    expect(
      await screen.findByText(
        "状态已在另一设备改变，无法撤销；已刷新，请从最近完成重新处理。",
      ),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(apiMocks.listQueue).toHaveBeenCalledTimes(loadsBeforeUndo + 4),
    );
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
      has_more: false,
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
        name: "将 可处理单集 标记完成",
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
    await waitFor(() =>
      expect(
        screen.getByRole("menuitem", { name: "移至 Focus" }),
      ).toHaveFocus(),
    );

    fireEvent.pointerDown(menu);
    expect(menu).toBeInTheDocument();

    fireEvent.pointerDown(document.body);
    await waitFor(() =>
      expect(
        screen.queryByRole("menu", { name: "移动 可处理单集" }),
      ).toBeNull(),
    );
  });

  it("places the move menu above its trigger when it would overflow", async () => {
    render(<InboxPageClient />);
    const trigger = await screen.findByRole("button", {
      name: "将 可处理单集 移动到其他队列",
    });
    vi.spyOn(trigger, "getBoundingClientRect").mockReturnValue({
      top: 700,
      right: 300,
      bottom: 744,
      left: 256,
      width: 44,
      height: 44,
      x: 256,
      y: 700,
      toJSON: () => ({}),
    });

    fireEvent.click(trigger);
    const menu = screen.getByRole("menu", { name: "移动 可处理单集" });
    vi.spyOn(menu, "getBoundingClientRect").mockReturnValue({
      top: 0,
      right: 300,
      bottom: 144,
      left: 144,
      width: 156,
      height: 144,
      x: 144,
      y: 0,
      toJSON: () => ({}),
    });
    fireEvent(window, new Event("resize"));

    await waitFor(() => {
      const menuTop = Number.parseFloat(menu.style.top);
      expect(menuTop).toBeLessThan(700);
      expect(menuTop + 144).toBeLessThanOrEqual(window.innerHeight);
    });
  });

  it("closes the move menu with Escape and restores trigger focus", async () => {
    render(<InboxPageClient />);
    const trigger = await screen.findByRole("button", {
      name: "将 可处理单集 移动到其他队列",
    });

    fireEvent.click(trigger);
    await waitFor(() =>
      expect(
        screen.getByRole("menuitem", { name: "移至 Focus" }),
      ).toHaveFocus(),
    );
    fireEvent.keyDown(document, { key: "Escape" });

    await waitFor(() =>
      expect(
        screen.queryByRole("menu", { name: "移动 可处理单集" }),
      ).toBeNull(),
    );
    expect(trigger).toHaveFocus();
  });

  it("traverses move menu items with menu navigation keys", async () => {
    render(<InboxPageClient />);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "将 可处理单集 移动到其他队列",
      }),
    );
    const menu = screen.getByRole("menu", { name: "移动 可处理单集" });
    const focusItem = screen.getByRole("menuitem", { name: "移至 Focus" });
    const somedayItem = screen.getByRole("menuitem", {
      name: "移至 Someday",
    });
    const doneItem = screen.getByRole("menuitem", { name: "标记完成" });

    await waitFor(() => expect(focusItem).toHaveFocus());
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(somedayItem).toHaveFocus();
    fireEvent.keyDown(menu, { key: "End" });
    expect(doneItem).toHaveFocus();
    fireEvent.keyDown(menu, { key: "ArrowDown" });
    expect(focusItem).toHaveFocus();
    fireEvent.keyDown(menu, { key: "ArrowUp" });
    expect(doneItem).toHaveFocus();
    fireEvent.keyDown(menu, { key: "Home" });
    expect(focusItem).toHaveFocus();
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

  it("keeps the full action-workbench flow usable through the new detail tabs", async () => {
    render(<InboxPageClient />);
    const trigger = await screen.findByRole("button", {
      name: "打开 可处理单集 明细",
    });

    fireEvent.click(trigger);
    const dialog = await screen.findByRole("dialog", {
      name: "可处理单集",
    });
    const tabs = within(dialog).getAllByRole("tab");
    expect(tabs.map((tab) => tab.textContent)).toEqual(["Show Notes", "笔记"]);
    expect(within(dialog).getByText("正文")).toBeVisible();
    expect(
      within(dialog).queryByRole("tab", { name: "转写" }),
    ).not.toBeInTheDocument();

    fireEvent.click(within(dialog).getByRole("tab", { name: "笔记" }));
    expect(within(dialog).queryByText("备注与标签")).not.toBeInTheDocument();
    expect(
      within(dialog).getByRole("region", { name: "单集笔记与标签" }),
    ).toBeVisible();
    expect(within(dialog).queryByText("YOUR CONTEXT")).not.toBeInTheDocument();

    fireEvent.click(
      within(dialog).getByRole("button", { name: "关闭单集明细" }),
    );
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it("promotes the desktop detail drawer into a two-column Copilot workspace and restores context", async () => {
    render(<InboxPageClient />);
    const trigger = await screen.findByRole("button", {
      name: "打开 可处理单集 明细",
    });
    const boardViewport = screen.getByRole("region", {
      name: "消费队列横向总览",
    });
    boardViewport.scrollLeft = 137;

    fireEvent.click(trigger);
    const dialog = await screen.findByRole("dialog", {
      name: "可处理单集",
    });
    const showNotes = within(dialog).getByRole("tabpanel", {
      name: "Show Notes",
    });
    const source = within(showNotes).getByText("正文");
    const range = document.createRange();
    range.selectNodeContents(source);
    const selectionSpy = vi.spyOn(window, "getSelection").mockReturnValue({
      isCollapsed: false,
      rangeCount: 1,
      getRangeAt: () => range,
      toString: () => "正文",
    } as unknown as Selection);
    fireEvent(document, new Event("selectionchange"));

    const detailScroll = within(dialog).getByRole("tablist", {
      name: "单集详情内容",
    }).parentElement as HTMLElement;
    detailScroll.scrollTop = 164;
    fireEvent.click(within(dialog).getByRole("tab", { name: "笔记" }));

    const openCopilot = within(dialog).getByRole("button", {
      name: "单集助手",
    });
    fireEvent.click(openCopilot);

    const workspace = await within(dialog).findByRole("complementary", {
      name: "单集助手双栏工作台",
    });
    expect(within(dialog).getByRole("tab", { name: "笔记" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(within(workspace).getByText("已选 Show Notes")).toBeVisible();
    const closeCopilot = within(workspace).getByRole("button", {
      name: "关闭助手",
    });
    await waitFor(() => expect(closeCopilot).toHaveFocus());
    expect(detailScroll.scrollTop).toBe(164);

    boardViewport.scrollLeft = 0;
    fireEvent.click(closeCopilot);

    await waitFor(() => expect(openCopilot).toHaveFocus());
    expect(
      within(dialog).queryByRole("complementary", {
        name: "单集助手双栏工作台",
      }),
    ).not.toBeInTheDocument();
    expect(within(dialog).getByRole("tab", { name: "笔记" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(detailScroll.scrollTop).toBe(164);
    expect(boardViewport.scrollLeft).toBe(137);
    selectionSpy.mockRestore();
  });

  it.each([390, 800])(
    "uses a dedicated narrow-screen Copilot at %ipx and returns to the same detail state",
    async (viewportWidth) => {
      const previousWidth = window.innerWidth;
      Object.defineProperty(window, "innerWidth", {
        configurable: true,
        value: viewportWidth,
      });
      try {
        render(<InboxPageClient />);
        fireEvent.click(
          await screen.findByRole("button", {
            name: "打开 可处理单集 明细",
          }),
        );
        const detailDialog = await screen.findByRole("dialog", {
          name: "可处理单集",
        });
        fireEvent.click(
          within(detailDialog).getByRole("tab", { name: "笔记" }),
        );
        const openCopilot = within(detailDialog).getByRole("button", {
          name: "单集助手",
        });
        fireEvent.click(openCopilot);

        const mobileDialog = await screen.findByRole("dialog", {
          name: "单集助手",
        });
        const workspace = within(mobileDialog).getByRole("complementary", {
          name: "移动端单集助手",
        });
        const returnButton = within(workspace).getByRole("button", {
          name: "返回单集",
        });
        await waitFor(() => expect(returnButton).toHaveFocus());
        expect(
          within(mobileDialog).queryByRole("tablist", {
            name: "单集详情内容",
          }),
        ).not.toBeInTheDocument();

        fireEvent.click(returnButton);

        const restoredDialog = await screen.findByRole("dialog", {
          name: "可处理单集",
        });
        expect(
          within(restoredDialog).getByRole("tab", { name: "笔记" }),
        ).toHaveAttribute("aria-selected", "true");
        await waitFor(() => expect(openCopilot).toHaveFocus());
      } finally {
        Object.defineProperty(window, "innerWidth", {
          configurable: true,
          value: previousWidth,
        });
        fireEvent(window, new Event("resize"));
      }
    },
  );

  it("keeps the Copilot question, quote, and partial answer through failure, close, and retry", async () => {
    apiMocks.askCopilot
      .mockImplementationOnce(async (_episodeId, _request, onEvent) => {
        onEvent({
          type: "answer_delta",
          message: "已生成的部分回答。",
          transcript_used: false,
          private_note_included: false,
        });
        throw new Error("连接中断");
      })
      .mockImplementationOnce(async (_episodeId, _request, onEvent) => {
        onEvent({
          type: "answer_delta",
          message: "重试后的完整回答。",
          transcript_used: false,
          private_note_included: false,
        });
        onEvent({
          type: "complete",
          message: "回答完成",
          transcript_used: false,
          private_note_included: false,
          first_content_ms: 180,
          total_ms: 640,
        });
      });

    render(<InboxPageClient />);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "打开 可处理单集 明细",
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "可处理单集",
    });
    const source = within(dialog).getByText("正文");
    const range = document.createRange();
    range.selectNodeContents(source);
    const selectionSpy = vi.spyOn(window, "getSelection").mockReturnValue({
      isCollapsed: false,
      rangeCount: 1,
      getRangeAt: () => range,
      toString: () => "正文",
    } as unknown as Selection);
    fireEvent(document, new Event("selectionchange"));
    fireEvent.click(within(dialog).getByRole("button", { name: "单集助手" }));

    const question = await within(dialog).findByRole("textbox", {
      name: "向单集助手提问",
    });
    fireEvent.change(question, { target: { value: "这段内容说明什么？" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "提问" }));

    expect(await within(dialog).findByText("已生成的部分回答。")).toBeVisible();
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "问题、选区和已有答案已保留",
    );
    expect(
      within(
        within(dialog).getByRole("tabpanel", { name: "Show Notes" }),
      ).getByText("正文"),
    ).toBeInTheDocument();

    fireEvent.click(within(dialog).getByRole("button", { name: "关闭助手" }));
    await waitFor(() =>
      expect(
        within(dialog).getByRole("button", { name: "单集助手" }),
      ).toHaveFocus(),
    );
    fireEvent.click(within(dialog).getByRole("button", { name: "单集助手" }));

    expect(await within(dialog).findByText("已生成的部分回答。")).toBeVisible();
    expect(
      within(dialog).getByRole("textbox", { name: "向单集助手提问" }),
    ).toHaveValue("这段内容说明什么？");
    expect(within(dialog).getByText("已选 Show Notes")).toBeVisible();
    fireEvent.click(within(dialog).getByRole("button", { name: "重试" }));

    expect(await within(dialog).findByText("重试后的完整回答。")).toBeVisible();
    expect(within(dialog).queryByText("已选 Show Notes")).toBeNull();
    selectionSpy.mockRestore();
  });

  it("reads native Minutes, synchronizes managed audio, and preserves transcript selection", async () => {
    const completedRun: ProcessingRun = {
      id: 81,
      episode_id: inboxItem.episode_id,
      pipeline_version: "focus-processing-v2",
      trigger_source: "manual",
      status: "completed",
      current_step: "",
      attempt_count: 1,
      max_attempts: 3,
      error_retryable: false,
      created_at: "2026-08-29T08:00:00Z",
      updated_at: "2026-08-29T08:05:00Z",
    };
    const nativeArtifact: EpisodeArtifactSet = {
      id: 82,
      run_id: completedRun.id,
      episode_id: inboxItem.episode_id,
      pipeline_version: completedRun.pipeline_version,
      manifest_path: "manifest.json",
      manifest_sha256: "1".repeat(64),
      minutes_summary_sha256: "2".repeat(64),
      transcript_sha256: "3".repeat(64),
      transcript_timeline_sha256: "4".repeat(64),
      notes_sha256: "",
      capabilities: {
        minutes_summary: true,
        transcript: true,
        structured_timeline: true,
        matching_audio: true,
        legacy_episode_notes: false,
      },
      is_current: true,
      created_at: "2026-08-29T08:05:00Z",
    };
    apiMocks.listEpisodeRuns.mockResolvedValue([completedRun]);
    apiMocks.getProcessingRun.mockResolvedValue({
      run: completedRun,
      current_artifact: nativeArtifact,
      deliveries: [],
    } satisfies ProcessingRunDetail);
    apiMocks.getArtifactContent.mockImplementation(
      (_artifactSetId: number, kind: string) =>
        Promise.resolve(
          kind === "minutes_summary"
            ? {
                kind,
                content: "# 妙记原生纪要",
                sha256: nativeArtifact.minutes_summary_sha256,
                media_available: false,
              }
            : {
                kind,
                content: "# 妙记结构化逐字稿",
                sha256: nativeArtifact.transcript_sha256,
                timeline_sha256: nativeArtifact.transcript_timeline_sha256,
                segments: [
                  {
                    order: 1,
                    speaker: "主持人",
                    start_ms: 0,
                    text: "开场",
                  },
                  {
                    order: 2,
                    speaker: "嘉宾",
                    start_ms: 30_000,
                    text: "中段",
                  },
                  {
                    order: 3,
                    speaker: "主持人",
                    start_ms: 60_000,
                    text: "尾段",
                  },
                ],
                media_available: true,
              },
        ),
    );

    render(<InboxPageClient />);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "打开 可处理单集 明细",
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "可处理单集",
    });
    fireEvent.click(within(dialog).getByRole("tab", { name: "转写" }));

    expect(
      await within(dialog).findByRole("heading", {
        name: "妙记原生纪要",
      }),
    ).toBeVisible();
    expect(within(dialog).getByRole("tab", { name: "纪要" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(apiMocks.getArtifactContent).toHaveBeenCalledWith(
      nativeArtifact.id,
      "minutes_summary",
    );

    fireEvent.click(within(dialog).getByRole("tab", { name: "逐字稿" }));
    expect(await within(dialog).findByText("开场")).toBeVisible();
    expect(within(dialog).getByText("中段")).toBeVisible();
    expect(within(dialog).getByText("尾段")).toBeVisible();
    expect(within(dialog).getByText("逐字稿 · 3 段")).toBeVisible();
    expect(within(dialog).getByText("音频可用")).toBeVisible();
    expect(within(dialog).getByText("正在加载音频…")).toBeVisible();

    const audio = dialog.querySelector("audio")!;
    let paused = true;
    Object.defineProperties(audio, {
      duration: { configurable: true, value: 90 },
      currentTime: { configurable: true, writable: true, value: 0 },
      paused: { configurable: true, get: () => paused },
    });
    const play = vi.fn(async () => {
      paused = false;
      fireEvent.play(audio);
    });
    const pause = vi.fn(() => {
      paused = true;
      fireEvent.pause(audio);
    });
    const load = vi.fn();
    Object.defineProperties(audio, {
      play: { configurable: true, value: play },
      pause: { configurable: true, value: pause },
      load: { configurable: true, value: load },
    });
    fireEvent.loadedMetadata(audio);
    fireEvent.click(within(dialog).getByRole("button", { name: "播放音频" }));
    await waitFor(() => expect(play).toHaveBeenCalledTimes(1));

    audio.currentTime = 31;
    fireEvent.timeUpdate(audio);
    const middleSegment = within(dialog).getByRole("button", {
      name: "00:30 嘉宾：中段",
    });
    expect(middleSegment).toHaveAttribute("aria-current", "true");
    expect(within(dialog).getByText("正在播放")).toBeVisible();

    const slider = within(dialog).getByRole("slider", { name: "音频进度" });
    slider.focus();
    fireEvent.keyDown(slider, { key: "End" });
    expect(slider).toHaveFocus();
    expect(audio.currentTime).toBe(90);
    expect(
      within(dialog).getByRole("button", {
        name: "01:00 主持人：尾段",
      }),
    ).toHaveAttribute("aria-current", "true");

    middleSegment.focus();
    fireEvent.click(middleSegment);
    expect(middleSegment).toHaveFocus();
    expect(audio.currentTime).toBe(30);

    const transcriptRegion = within(dialog).getByRole("region", {
      name: "同步逐字稿",
    });
    const tailSegment = within(dialog).getByRole("button", {
      name: "01:00 主持人：尾段",
    });
    Object.defineProperty(transcriptRegion, "getBoundingClientRect", {
      configurable: true,
      value: () => ({ top: 0, bottom: 100 }),
    });
    Object.defineProperty(tailSegment, "getBoundingClientRect", {
      configurable: true,
      value: () => ({ top: 130, bottom: 170 }),
    });
    const scrollIntoView = vi.fn();
    Object.defineProperty(tailSegment, "scrollIntoView", {
      configurable: true,
      value: scrollIntoView,
    });
    fireEvent.scroll(transcriptRegion);
    audio.currentTime = 61;
    fireEvent.timeUpdate(audio);
    expect(scrollIntoView).not.toHaveBeenCalled();
    fireEvent.play(audio);
    expect(scrollIntoView).toHaveBeenCalledWith({
      block: "nearest",
      behavior: "auto",
    });

    const mediaSource = audio.getAttribute("src");
    fireEvent.error(audio);
    expect(
      within(dialog).getByText("音频加载失败，逐字稿仍可阅读。"),
    ).toBeVisible();
    expect(within(dialog).getByText("中段")).toBeVisible();
    fireEvent.click(within(dialog).getByRole("button", { name: "重试" }));
    expect(load).toHaveBeenCalledTimes(1);
    expect(audio).toHaveAttribute("src", mediaSource);

    const transcriptSource = within(dialog).getByText("开场");
    const range = document.createRange();
    range.selectNodeContents(transcriptSource);
    const selectionSpy = vi.spyOn(window, "getSelection").mockReturnValue({
      isCollapsed: false,
      rangeCount: 1,
      getRangeAt: () => range,
      toString: () => "开场",
    } as unknown as Selection);
    fireEvent(document, new Event("selectionchange"));
    fireEvent.click(within(dialog).getByRole("button", { name: "单集助手" }));
    expect(await within(dialog).findByText("已选 逐字稿")).toBeVisible();
    fireEvent.click(within(dialog).getByRole("button", { name: "关闭助手" }));
    await waitFor(() =>
      expect(
        within(dialog).getByRole("button", { name: "单集助手" }),
      ).toHaveFocus(),
    );

    fireEvent.click(within(dialog).getByRole("tab", { name: "Show Notes" }));
    fireEvent.click(within(dialog).getByRole("tab", { name: "转写" }));
    expect(within(dialog).getByRole("tab", { name: "逐字稿" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(
      within(dialog).getByRole("button", {
        name: "00:00 主持人：开场",
      }),
    ).toBeVisible();
    expect(
      within(dialog).queryByText("来自同一条飞书妙记"),
    ).not.toBeInTheDocument();
    selectionSpy.mockRestore();
  });

  it("lets a Focus user re-transcribe a completed legacy artifact", async () => {
    const focusItem: ConsumptionItem = {
      ...inboxItem,
      queue_state: "focus",
    };
    const legacyRun: ProcessingRun = {
      id: 89,
      episode_id: focusItem.episode_id,
      pipeline_version: "focus-processing-v1",
      trigger_source: "manual",
      status: "completed",
      current_step: "",
      attempt_count: 1,
      max_attempts: 3,
      error_retryable: false,
      created_at: "2026-08-28T08:00:00Z",
      updated_at: "2026-08-28T08:05:00Z",
    };
    const legacyArtifact: EpisodeArtifactSet = {
      id: 90,
      run_id: legacyRun.id,
      episode_id: focusItem.episode_id,
      pipeline_version: legacyRun.pipeline_version,
      manifest_path: "manifest.json",
      manifest_sha256: "5".repeat(64),
      transcript_sha256: "6".repeat(64),
      notes_sha256: "7".repeat(64),
      capabilities: {
        minutes_summary: false,
        transcript: true,
        structured_timeline: false,
        matching_audio: false,
        legacy_episode_notes: true,
      },
      is_current: true,
      created_at: "2026-08-28T08:05:00Z",
    };
    const pendingRun: ProcessingRun = {
      ...legacyRun,
      id: 91,
      pipeline_version: "focus-processing-v2",
      status: "queued",
      current_step: "transcription",
      created_at: "2026-08-29T10:00:00Z",
      updated_at: "2026-08-29T10:00:00Z",
    };
    apiMocks.listQueue.mockImplementation(
      async (queue: ConsumptionQueue): Promise<ConsumptionQueuePayload> => ({
        queue_state: queue,
        revision: 1,
        items: queue === "focus" ? [focusItem] : [],
        has_more: false,
      }),
    );
    apiMocks.getItem.mockResolvedValue(focusItem);
    apiMocks.listEpisodeRuns.mockResolvedValue([legacyRun]);
    apiMocks.getProcessingRun
      .mockResolvedValueOnce({
        run: legacyRun,
        current_artifact: legacyArtifact,
        deliveries: [],
      } satisfies ProcessingRunDetail)
      .mockResolvedValue({
        run: pendingRun,
        current_artifact: legacyArtifact,
        deliveries: [],
      } satisfies ProcessingRunDetail);
    apiMocks.startProcessing.mockResolvedValue({
      run: pendingRun,
      reused_active: false,
      reused_successful: false,
      preparing_audio: false,
    });
    apiMocks.getArtifactContent.mockImplementation(
      (_artifactSetId: number, kind: string) =>
        Promise.resolve({
          kind,
          content: kind === "episode_notes" ? "# 旧版纪要" : "# 旧版逐字稿",
          sha256:
            kind === "episode_notes"
              ? legacyArtifact.notes_sha256
              : legacyArtifact.transcript_sha256,
          media_available: false,
        }),
    );

    render(<InboxPageClient />);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "打开 可处理单集 明细",
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "可处理单集",
    });
    fireEvent.click(within(dialog).getByRole("tab", { name: "转写" }));

    expect(
      await within(dialog).findByRole("button", { name: "重新转写" }),
    ).toBeEnabled();
    fireEvent.click(within(dialog).getByRole("button", { name: "重新转写" }));

    await waitFor(() =>
      expect(apiMocks.startProcessing).toHaveBeenCalledWith(
        focusItem.episode_id,
      ),
    );
    await waitFor(() =>
      expect(apiMocks.getProcessingRun).toHaveBeenCalledWith(pendingRun.id),
    );
    expect(
      within(dialog).queryByRole("button", { name: "重新转写" }),
    ).not.toBeInTheDocument();
  });

  it("starts v2 from the top-level action when a terminal legacy run has no artifact", async () => {
    const focusItem: ConsumptionItem = {
      ...inboxItem,
      queue_state: "focus",
    };
    const legacyRun: ProcessingRun = {
      id: 93,
      episode_id: focusItem.episode_id,
      pipeline_version: "focus-processing-v1",
      trigger_source: "manual",
      status: "failed",
      current_step: "episode_notes",
      attempt_count: 1,
      max_attempts: 3,
      error_code: "RUNTIME_UNAVAILABLE",
      error_message: "旧版加工失败",
      error_retryable: true,
      created_at: "2026-08-29T08:00:00Z",
      updated_at: "2026-08-29T08:05:00Z",
    };
    const pendingRun: ProcessingRun = {
      ...legacyRun,
      id: 95,
      pipeline_version: "focus-processing-v2",
      status: "queued",
      current_step: "transcription",
      error_code: undefined,
      error_message: undefined,
      error_retryable: false,
      created_at: "2026-08-29T10:00:00Z",
      updated_at: "2026-08-29T10:00:00Z",
    };
    const startResult = {
      run: pendingRun,
      reused_active: false,
      reused_successful: false,
      preparing_audio: false,
    };
    let resolveStart: (result: typeof startResult) => void = () => undefined;
    apiMocks.listQueue.mockImplementation(
      async (queue: ConsumptionQueue): Promise<ConsumptionQueuePayload> => ({
        queue_state: queue,
        revision: 1,
        items: queue === "focus" ? [focusItem] : [],
        has_more: false,
      }),
    );
    apiMocks.getItem.mockResolvedValue(focusItem);
    apiMocks.listEpisodeRuns.mockResolvedValue([legacyRun]);
    apiMocks.getProcessingRun
      .mockResolvedValueOnce({
        run: legacyRun,
        deliveries: [],
      } satisfies ProcessingRunDetail)
      .mockResolvedValue({
        run: pendingRun,
        deliveries: [],
      } satisfies ProcessingRunDetail);
    apiMocks.startProcessing.mockReturnValue(
      new Promise<typeof startResult>((resolve) => {
        resolveStart = resolve;
      }),
    );
    render(<InboxPageClient />);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "打开 可处理单集 明细",
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "可处理单集",
    });
    fireEvent.click(within(dialog).getByRole("tab", { name: "转写" }));

    const actions = await within(dialog).findAllByRole("button", {
      name: "重新转写",
    });
    expect(actions).toHaveLength(1);
    expect(
      within(dialog).queryByRole("button", { name: "重试转写" }),
    ).not.toBeInTheDocument();
    fireEvent.click(actions[0]);

    await waitFor(() =>
      expect(apiMocks.startProcessing).toHaveBeenCalledWith(
        focusItem.episode_id,
      ),
    );
    expect(actions[0]).toBeDisabled();
    expect(within(dialog).getByText("正在重试转写")).toBeVisible();
    expect(apiMocks.retryProcessing).not.toHaveBeenCalled();
    await act(async () => {
      resolveStart(startResult);
    });
    await waitFor(() =>
      expect(apiMocks.getProcessingRun).toHaveBeenCalledWith(pendingRun.id),
    );
  });

  it("blocks legacy restart while an external result remains unknown", async () => {
    const focusItem: ConsumptionItem = {
      ...inboxItem,
      queue_state: "focus",
    };
    const unknownRun: ProcessingRun = {
      id: 96,
      episode_id: focusItem.episode_id,
      pipeline_version: "focus-processing-v1",
      trigger_source: "manual",
      status: "cancelled",
      current_step: "",
      attempt_count: 1,
      max_attempts: 3,
      error_code: "cancelled_external_result_unknown",
      error_message:
        "已取消本机加工；飞书端任务可能继续，已创建的远端资源会保留。",
      error_retryable: false,
      created_at: "2026-08-29T08:00:00Z",
      updated_at: "2026-08-29T08:05:00Z",
    };
    const legacyArtifact: EpisodeArtifactSet = {
      id: 97,
      run_id: 95,
      episode_id: focusItem.episode_id,
      pipeline_version: unknownRun.pipeline_version,
      manifest_path: "manifest.json",
      manifest_sha256: "c".repeat(64),
      transcript_sha256: "d".repeat(64),
      notes_sha256: "e".repeat(64),
      capabilities: {
        minutes_summary: false,
        transcript: true,
        structured_timeline: false,
        matching_audio: false,
        legacy_episode_notes: true,
      },
      is_current: true,
      created_at: "2026-08-28T08:05:00Z",
    };
    apiMocks.listQueue.mockImplementation(
      async (queue: ConsumptionQueue): Promise<ConsumptionQueuePayload> => ({
        queue_state: queue,
        revision: 1,
        items: queue === "focus" ? [focusItem] : [],
        has_more: false,
      }),
    );
    apiMocks.getItem.mockResolvedValue(focusItem);
    apiMocks.listEpisodeRuns.mockResolvedValue([unknownRun]);
    apiMocks.getProcessingRun.mockResolvedValue({
      run: unknownRun,
      current_artifact: legacyArtifact,
      deliveries: [],
      action_suggestion:
        "请先在飞书确认转写是否仍在继续或远端资源是否已创建；确认前不可重新加工。",
    } satisfies ProcessingRunDetail);

    render(<InboxPageClient />);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "打开 可处理单集 明细",
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "可处理单集",
    });
    fireEvent.click(within(dialog).getByRole("tab", { name: "转写" }));

    expect(
      await within(dialog).findAllByText(
        "请先在飞书确认转写是否仍在继续或远端资源是否已创建；确认前不可重新加工。",
      ),
    ).not.toHaveLength(0);
    expect(
      within(dialog).queryByRole("button", { name: "重新转写" }),
    ).not.toBeInTheDocument();
    expect(
      within(dialog).queryByRole("button", { name: "重试转写" }),
    ).not.toBeInTheDocument();
    expect(apiMocks.startProcessing).not.toHaveBeenCalled();
    expect(apiMocks.retryProcessing).not.toHaveBeenCalled();
  });

  it("keeps a legacy previous success readable when the new run fails", async () => {
    const failedRun: ProcessingRun = {
      id: 91,
      episode_id: inboxItem.episode_id,
      pipeline_version: "focus-processing-v2",
      trigger_source: "manual",
      status: "failed",
      current_step: "transcription",
      attempt_count: 1,
      max_attempts: 3,
      error_code: "TRANSCRIPT_TIMELINE_INVALID",
      error_message: "妙记逐字稿时间轴格式无法解析",
      error_retryable: false,
      created_at: "2026-08-29T09:00:00Z",
      updated_at: "2026-08-29T09:05:00Z",
    };
    const legacyArtifact: EpisodeArtifactSet = {
      id: 92,
      run_id: 90,
      episode_id: inboxItem.episode_id,
      pipeline_version: "focus-processing-v1",
      manifest_path: "manifest.json",
      manifest_sha256: "5".repeat(64),
      transcript_sha256: "6".repeat(64),
      notes_sha256: "7".repeat(64),
      capabilities: {
        minutes_summary: false,
        transcript: true,
        structured_timeline: false,
        matching_audio: false,
        legacy_episode_notes: true,
      },
      is_current: true,
      created_at: "2026-08-28T08:00:00Z",
    };
    apiMocks.listEpisodeRuns.mockResolvedValue([failedRun]);
    apiMocks.getProcessingRun.mockResolvedValue({
      run: failedRun,
      current_artifact: legacyArtifact,
      deliveries: [],
      action_suggestion: "检查妙记格式后重试。",
    } satisfies ProcessingRunDetail);
    apiMocks.getArtifactContent.mockImplementation(
      (_artifactSetId: number, kind: string) =>
        Promise.resolve({
          kind,
          content:
            kind === "episode_notes"
              ? "# 仍可阅读的旧版纪要"
              : "# 仍可阅读的旧版逐字稿",
          sha256:
            kind === "episode_notes"
              ? legacyArtifact.notes_sha256
              : legacyArtifact.transcript_sha256,
          media_available: false,
        }),
    );

    render(<InboxPageClient />);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "打开 可处理单集 明细",
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "可处理单集",
    });
    fireEvent.click(within(dialog).getByRole("tab", { name: "转写" }));

    expect(
      await within(dialog).findByRole("status", { name: "转写状态：转写失败" }),
    ).toBeVisible();
    expect(within(dialog).getByText("上一成功版本")).toBeVisible();
    expect(
      within(dialog).getByText(
        "这是旧版纪要；重新转写后可获得妙记纪要和同步逐字稿。",
      ),
    ).toBeVisible();
    expect(
      await within(dialog).findByRole("heading", {
        name: "仍可阅读的旧版纪要",
      }),
    ).toBeVisible();
  });

  it("keeps the transcription shell stable while processing detail is slow", async () => {
    const completedRun: ProcessingRun = {
      id: 101,
      episode_id: inboxItem.episode_id,
      pipeline_version: "focus-processing-v2",
      trigger_source: "manual",
      status: "completed",
      current_step: "",
      attempt_count: 1,
      max_attempts: 3,
      error_retryable: false,
      created_at: "2026-08-29T10:00:00Z",
      updated_at: "2026-08-29T10:05:00Z",
    };
    const nativeArtifact: EpisodeArtifactSet = {
      id: 102,
      run_id: completedRun.id,
      episode_id: inboxItem.episode_id,
      pipeline_version: completedRun.pipeline_version,
      manifest_path: "manifest.json",
      manifest_sha256: "8".repeat(64),
      minutes_summary_sha256: "9".repeat(64),
      transcript_sha256: "a".repeat(64),
      transcript_timeline_sha256: "b".repeat(64),
      notes_sha256: "",
      capabilities: {
        minutes_summary: true,
        transcript: true,
        structured_timeline: true,
        matching_audio: false,
        legacy_episode_notes: false,
      },
      is_current: true,
      created_at: "2026-08-29T10:05:00Z",
    };
    let resolveDetail: (detail: ProcessingRunDetail) => void = () => undefined;
    apiMocks.listEpisodeRuns.mockResolvedValue([completedRun]);
    apiMocks.getProcessingRun.mockReturnValue(
      new Promise<ProcessingRunDetail>((resolve) => {
        resolveDetail = resolve;
      }),
    );
    apiMocks.getArtifactContent.mockResolvedValue({
      kind: "minutes_summary",
      content: "# 慢请求完成后的纪要",
      sha256: nativeArtifact.minutes_summary_sha256,
      media_available: false,
    });

    render(<InboxPageClient />);
    fireEvent.click(
      await screen.findByRole("button", {
        name: "打开 可处理单集 明细",
      }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "可处理单集",
    });
    fireEvent.click(within(dialog).getByRole("tab", { name: "转写" }));
    expect(within(dialog).queryByText("自动加工")).not.toBeInTheDocument();
    expect(within(dialog).getByText("正在读取转写内容")).toBeVisible();
    expect(
      within(dialog).queryByRole("tab", { name: "纪要" }),
    ).not.toBeInTheDocument();

    await act(async () => {
      resolveDetail({
        run: completedRun,
        current_artifact: nativeArtifact,
        deliveries: [],
      });
    });
    expect(
      await within(dialog).findByRole("heading", {
        name: "慢请求完成后的纪要",
      }),
    ).toBeVisible();
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
      has_more: false,
    }));
    apiMocks.placeQueue.mockResolvedValue({
      queues: {
        inbox: {
          queue_state: "inbox",
          revision: 7,
          items: [second, first],
          has_more: false,
        },
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
        inbox: {
          queue_state: "inbox",
          revision: 2,
          items: [],
          has_more: false,
        },
        done: {
          queue_state: "done",
          revision: 2,
          items: [
            { ...inboxItem, queue_state: "done", in_progress_at: undefined },
          ],
          has_more: false,
        },
      },
    });
    dragMediaMatches = true;

    render(<InboxPageClient />);
    await screen.findByRole("button", { name: "拖动《可处理单集》调整队列" });
    await waitFor(() => expect(dndMocks.onDragEnd).toBeDefined());
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
    expect(
      within(queueSection("done")).getByText("可处理单集"),
    ).toBeInTheDocument();
  });

  it("recent completions cannot be reordered but can be dragged out precisely", async () => {
    const completedItem: ConsumptionItem = {
      ...inboxItem,
      episode_id: 202,
      episode_title: "最近完成项",
      queue_state: "done",
      completed_at: "2026-08-23T08:30:00Z",
    };
    apiMocks.listQueue.mockImplementation(async (queue: ConsumptionQueue) => ({
      queue_state: queue,
      revision: queue === "done" ? 3 : 1,
      items:
        queue === "done"
          ? [completedItem]
          : queue === "inbox"
            ? [inboxItem]
            : [],
      has_more: false,
    }));
    apiMocks.placeQueue.mockResolvedValue({
      queues: {
        done: {
          queue_state: "done",
          revision: 4,
          items: [],
          has_more: false,
        },
        inbox: {
          queue_state: "inbox",
          revision: 2,
          items: [{ ...completedItem, queue_state: "inbox" }, inboxItem],
          has_more: false,
        },
      },
    });
    dragMediaMatches = true;

    render(<InboxPageClient />);
    await screen.findByRole("button", {
      name: "拖动《最近完成项》重新处理",
    });
    await waitFor(() => expect(dndMocks.onDragEnd).toBeDefined());
    act(() =>
      dndMocks.onDragStart?.(
        dragEvent({
          source: "done",
          activeEpisodeId: completedItem.episode_id,
          target: "done",
        }),
      ),
    );
    act(() =>
      dndMocks.onDragEnd?.(
        dragEvent({
          source: "done",
          activeEpisodeId: completedItem.episode_id,
          target: "inbox",
          overEpisodeId: inboxItem.episode_id,
        }),
      ),
    );

    await waitFor(() =>
      expect(apiMocks.placeQueue).toHaveBeenCalledWith(
        completedItem.episode_id,
        {
          queue_state: "inbox",
          before_episode_id: inboxItem.episode_id,
          expected_revisions: { inbox: 1, done: 3 },
          acknowledge_focus_limit: false,
        },
      ),
    );
    expect(
      within(queueSection("inbox")).getAllByRole("button", {
        name: /打开 .* 明细/,
      })[0],
    ).toHaveAttribute("aria-label", "打开 最近完成项 明细");
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
          has_more: false,
        });
      }
      return Promise.resolve({
        queue_state: queue,
        revision: 1,
        items: queue === "inbox" ? [inboxItem] : [],
        has_more: false,
      });
    });
    apiMocks.setQueue.mockResolvedValue({ ...doneItem, queue_state: "inbox" });
    apiMocks.placeQueue.mockResolvedValue({
      queues: {
        inbox: {
          queue_state: "inbox",
          revision: 2,
          items: [],
          has_more: false,
        },
        done: {
          queue_state: "done",
          revision: 2,
          items: [{ ...inboxItem, queue_state: "done" }],
          has_more: false,
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
    await waitFor(() => expect(dndMocks.onDragEnd).toBeDefined());

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
    expect(
      within(queueSection("done")).getByText("可处理单集"),
    ).toBeInTheDocument();

    await act(async () => {
      resolveStaleDone({
        queue_state: "done",
        revision: 1,
        items: [],
        has_more: false,
      });
      await Promise.resolve();
    });

    expect(
      within(queueSection("done")).getByText("可处理单集"),
    ).toBeInTheDocument();
  });

  it("队列版本冲突时恢复并提示重新拖放", async () => {
    apiMocks.placeQueue.mockRejectedValueOnce(new Error("过期布局"));
    apiMocks.isQueueOrderConflict.mockReturnValueOnce(true);
    dragMediaMatches = true;

    render(<InboxPageClient />);
    await screen.findByRole("button", { name: "拖动《可处理单集》调整队列" });
    await waitFor(() => expect(dndMocks.onDragEnd).toBeDefined());
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
    expect(
      within(queueSection("inbox")).getByText("可处理单集"),
    ).toBeInTheDocument();
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
      items:
        queue === "inbox" ? [inboxItem] : queue === "focus" ? focusItems : [],
      has_more: false,
    }));
    apiMocks.getItem.mockResolvedValue(inboxItem);
    apiMocks.placeQueue.mockResolvedValue({
      queues: {
        inbox: {
          queue_state: "inbox",
          revision: 5,
          items: [],
          has_more: false,
        },
        focus: {
          queue_state: "focus",
          revision: 10,
          items: [...focusItems, { ...inboxItem, queue_state: "focus" }],
          has_more: false,
        },
      },
    });
    dragMediaMatches = true;

    render(<InboxPageClient />);
    await screen.findByRole("button", { name: "拖动《可处理单集》调整队列" });
    await within(queueSection("focus")).findByText("Focus 7");
    await waitFor(() => expect(dndMocks.onDragEnd).toBeDefined());
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
    expect(
      within(queueSection("inbox")).getByText("可处理单集"),
    ).toBeInTheDocument();

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
      has_more: false,
    }));
    apiMocks.getItem.mockImplementation(async () => {
      remoteMoveObserved = true;
      return remotelyMoved;
    });
    dragMediaMatches = true;

    render(<InboxPageClient />);
    await screen.findByRole("button", { name: "拖动《可处理单集》调整队列" });
    await waitFor(() => expect(dndMocks.onDragEnd).toBeDefined());
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
    await screen.findByRole(
      "alertdialog",
      { name: "Focus 已有 7 项" },
      { timeout: 3000 },
    );

    fireEvent.click(screen.getByRole("button", { name: "仍加入 Focus" }));

    expect(
      await screen.findByText("队列顺序已在另一设备修改，请重新拖放。"),
    ).toBeInTheDocument();
    expect(apiMocks.placeQueue).not.toHaveBeenCalled();
    expect(
      within(queueSection("someday")).getByText("可处理单集"),
    ).toBeInTheDocument();
  });
});
