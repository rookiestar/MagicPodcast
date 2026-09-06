import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ConsumptionDetailPanel from "../ConsumptionDetailPanel";
import type { ConsumptionItem } from "@/types/consumption";
import type {
  ArtifactContent,
  EpisodeArtifactSet,
  ProcessingRun,
} from "@/types/processing";

const apiMocks = vi.hoisted(() => ({
  getItem: vi.fn(),
  markInProgress: vi.fn(),
  getConsumptionErrorDetails: vi.fn(() => ({ message: "保存失败" })),
  getNotes: vi.fn(),
  getTags: vi.fn(),
  updateNotes: vi.fn(),
  addTag: vi.fn(),
  removeTag: vi.fn(),
  getShowNotes: vi.fn(),
  listTags: vi.fn(),
  listEpisodeRuns: vi.fn(),
  getLatestAudio: vi.fn(),
  getScheduleStatus: vi.fn(),
  getRun: vi.fn(),
  startProcessing: vi.fn(),
  cancelProcessing: vi.fn(),
  retryProcessing: vi.fn(),
  getArtifactContent: vi.fn(),
  getCopilotContext: vi.fn(),
  askCopilot: vi.fn(),
}));

vi.mock("@/lib/api/consumption", () => ({
  consumptionApi: {
    getItem: apiMocks.getItem,
    markInProgress: apiMocks.markInProgress,
  },
  getConsumptionErrorDetails: apiMocks.getConsumptionErrorDetails,
}));

vi.mock("@/lib/api", () => ({
  episodeApi: {
    getShowNotes: apiMocks.getShowNotes,
    getNotes: apiMocks.getNotes,
    getTags: apiMocks.getTags,
    updateNotes: apiMocks.updateNotes,
    addTag: apiMocks.addTag,
    removeTag: apiMocks.removeTag,
  },
  tagApi: {
    list: apiMocks.listTags,
  },
}));

vi.mock("@/lib/api/processing", () => ({
  processingApi: {
    listEpisodeRuns: apiMocks.listEpisodeRuns,
    getLatestAudio: apiMocks.getLatestAudio,
    getScheduleStatus: apiMocks.getScheduleStatus,
    getRun: apiMocks.getRun,
    start: apiMocks.startProcessing,
    cancel: apiMocks.cancelProcessing,
    retry: apiMocks.retryProcessing,
    getArtifactContent: apiMocks.getArtifactContent,
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

const showNotesContent = [
  '<p><a href="https://example.com/transcript">安全链接</a></p>',
  '<p><a href="mailto:owner@example.com">邮件链接</a></p>',
  '<p><a href="tel:+8613800000000">电话链接</a></p>',
  '<p><a href="javascript:alert(1)">危险链接</a></p>',
  '<img src="https://i.typlog.com/cover.png" alt="允许图片">',
  '<img src="https://evil.example/track.png" alt="拒绝图片">',
].join("");

const item: ConsumptionItem = {
  episode_id: 201,
  podcast_id: 20,
  podcast_title: "测试节目",
  podcast_author: "测试作者",
  podcast_cover_url: "",
  episode_title: "站外消费测试",
  episode_no: "201",
  duration: 2400,
  published_date: "2026-08-10T08:00:00Z",
  show_notes: showNotesContent,
  original_url: "https://example.com/episode/201",
  image_url: "",
  notes: "旧备注",
  tags: [],
  queue_state: "focus",
  queue_updated_at: "2026-08-10T08:00:00Z",
};

type OnItemChange = (item: ConsumptionItem) => void;
type OnMove = (
  item: ConsumptionItem,
  target: ConsumptionItem["queue_state"],
) => Promise<ConsumptionItem | undefined>;

function renderDetail(
  overrides: Partial<{
    item: ConsumptionItem;
    onItemChange: OnItemChange;
    onMove: OnMove;
  }> = {},
) {
  const currentItem = overrides.item ?? item;
  const onItemChange = overrides.onItemChange ?? vi.fn<OnItemChange>();
  const onMove =
    overrides.onMove ??
    vi.fn<OnMove>().mockResolvedValue({ ...currentItem, queue_state: "done" });
  render(
    <ConsumptionDetailPanel
      item={currentItem}
      isQueueBusy={false}
      onClose={vi.fn()}
      onItemChange={onItemChange}
      onMove={onMove}
    />,
  );
  return { onItemChange, onMove };
}

const xyzItem: ConsumptionItem = {
  ...item,
  original_url:
    "https://www.xiaoyuzhoufm.com/episode/6a8cf80a1352af56ff3b7e2d?utm_source=rss",
};

describe("ConsumptionDetailPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMocks.getItem.mockResolvedValue(item);
    apiMocks.markInProgress.mockResolvedValue({
      ...item,
      in_progress_at: "2026-08-11T08:00:00Z",
    });
    apiMocks.getShowNotes.mockResolvedValue({
      episode_id: item.episode_id,
      show_notes_document: { content: showNotesContent, format: "html" },
    });
    apiMocks.getNotes.mockResolvedValue("旧备注");
    apiMocks.getTags.mockResolvedValue([]);
    apiMocks.updateNotes.mockResolvedValue(undefined);
    apiMocks.addTag.mockResolvedValue(undefined);
    apiMocks.removeTag.mockResolvedValue(undefined);
    apiMocks.listTags.mockResolvedValue([
      { id: 9, name: "AI", color: "#d7681d" },
    ]);
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
    apiMocks.getCopilotContext.mockResolvedValue({
      episode_id: item.episode_id,
      show_notes_available: true,
      transcript_available: false,
      private_note_available: true,
    });
    vi.spyOn(window, "open").mockImplementation(() => null);
  });

  it("renders safe Show Notes links and approved images while blocking dangerous content", async () => {
    const { container } = render(
      <ConsumptionDetailPanel
        item={item}
        isQueueBusy={false}
        onClose={vi.fn()}
        onItemChange={vi.fn()}
        onMove={vi.fn()}
      />,
    );

    expect(await screen.findByRole("link", { name: "安全链接" })).toHaveAttribute(
      "href",
      "https://example.com/transcript",
    );
    expect(screen.getByRole("link", { name: "邮件链接" })).toHaveAttribute(
      "href",
      "mailto:owner@example.com",
    );
    expect(screen.getByRole("link", { name: "电话链接" })).toHaveAttribute(
      "href",
      "tel:+8613800000000",
    );
    expect(
      screen.queryByRole("link", { name: "危险链接" }),
    ).not.toBeInTheDocument();
    expect(container.querySelector('img[alt="允许图片"]')).toBeInTheDocument();
    expect(container.querySelector('img[alt="拒绝图片"]')).not.toHaveAttribute(
      "src",
    );
    expect(
      screen.queryByRole("heading", { name: "单集助手" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "单集助手" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    expect(screen.getByRole("button", { name: "单集助手" })).toHaveTextContent(
      "AI",
    );
    const queueTrigger = screen.getByRole("button", {
      name: "当前队列 Focus，打开切换菜单",
    });
    expect(queueTrigger).toHaveAttribute("aria-haspopup", "menu");
    expect(queueTrigger).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("更多操作")).not.toBeInTheDocument();
    expect(
      container.querySelector('[data-copilot-source="show_notes"]'),
    ).toHaveAttribute("data-copilot-episode-id", "201");
    await waitFor(() => {
      expect(screen.queryByText("正在读取…")).not.toBeInTheDocument();
      expect(screen.queryByText("正在同步最新状态…")).not.toBeInTheDocument();
    });
  });

  it("keeps the episode identity visible while Show Notes load", async () => {
    let resolveShowNotes!: (value: {
      episode_id: number;
      show_notes_document: { content: string; format: "html" };
    }) => void;
    apiMocks.getShowNotes.mockImplementationOnce(
      () => new Promise((resolve) => (resolveShowNotes = resolve)),
    );

    renderDetail();

    expect(screen.getByRole("heading", { name: item.episode_title })).toBeVisible();
    expect(screen.getByRole("status", { name: "转写状态：正在读取" })).toBeVisible();
    expect(screen.getByText("正在读取 Show Notes…")).toBeVisible();

    await act(async () => {
      resolveShowNotes({
        episode_id: item.episode_id,
        show_notes_document: { content: showNotesContent, format: "html" },
      });
    });
    expect(await screen.findByRole("link", { name: "安全链接" })).toBeVisible();
  });

  it("keeps the detail usable when Show Notes fail and retries locally", async () => {
    apiMocks.getShowNotes
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({
        episode_id: item.episode_id,
        show_notes_document: { content: showNotesContent, format: "html" },
      });

    renderDetail();

    expect(await screen.findByText(/Show Notes 读取失败：offline/)).toBeVisible();
    expect(screen.getByRole("heading", { name: item.episode_title })).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "重试读取 Show Notes" }));
    expect(await screen.findByRole("link", { name: "安全链接" })).toBeVisible();
    expect(apiMocks.getShowNotes).toHaveBeenCalledTimes(2);
  });

  it("hides the unstarted transcript and navigates only visible tabs", async () => {
    renderDetail();

    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((tab) => tab.textContent)).toEqual(["Show Notes", "笔记"]);
    expect(tabs[0]).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel", { name: "Show Notes" })).toBeVisible();
    expect(screen.queryByText("YOUR CONTEXT")).not.toBeInTheDocument();

    tabs[0].focus();
    fireEvent.keyDown(tabs[0], { key: "ArrowRight" });
    await waitFor(() => expect(tabs[1]).toHaveFocus());
    expect(tabs[1]).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByText("备注与标签")).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "备注" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "标签" })).toBeVisible();
  });

  it("exposes the current transcription action in the compact header", async () => {
    apiMocks.startProcessing.mockResolvedValue({
      reused_active: false,
      reused_successful: false,
      preparing_audio: true,
      audio_asset: {
        id: 51,
        episode_id: item.episode_id,
        status: "queued",
        size_bytes: 0,
        duration_seconds: 0,
        queued_at: "2026-08-29T08:00:00Z",
        created_at: "2026-08-29T08:00:00Z",
        updated_at: "2026-08-29T08:00:00Z",
      },
    });
    renderDetail();

    const start = await screen.findByRole("button", { name: "开始转写" });
    fireEvent.click(start);

    await waitFor(() =>
      expect(apiMocks.startProcessing).toHaveBeenCalledWith(item.episode_id),
    );
    expect(await screen.findByRole("tab", { name: "转写" })).toBeVisible();
    expect(
      await screen.findByRole("status", { name: "转写状态：准备音频" }),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "原节目" })).toBeVisible();
    expect(
      screen.queryByRole("link", { name: "原节目" }),
    ).not.toBeInTheDocument();
  });

  it("keeps the idle transcription status concise", async () => {
    renderDetail();

    expect(
      await screen.findByRole("status", { name: "转写状态：未转写" }),
    ).toBeVisible();
    expect(screen.queryByText("可开始飞书妙记转写")).not.toBeInTheDocument();
  });

  it("keeps identity, tabs, and Show Notes visible while regional requests are slow", async () => {
    apiMocks.getItem.mockReturnValue(new Promise(() => undefined));
    apiMocks.listEpisodeRuns.mockReturnValue(new Promise(() => undefined));
    apiMocks.getNotes.mockReturnValue(new Promise(() => undefined));
    apiMocks.getTags.mockReturnValue(new Promise(() => undefined));
    apiMocks.listTags.mockReturnValue(new Promise(() => undefined));

    renderDetail();

    expect(screen.getByRole("heading", { name: "站外消费测试" })).toBeVisible();
    expect(screen.getByRole("tablist", { name: "单集详情内容" })).toBeVisible();
    expect(await screen.findByRole("link", { name: "安全链接" })).toBeVisible();
    expect(
      screen.getByRole("status", { name: "转写状态：正在读取" }),
    ).toBeVisible();
  });

  it("keeps other content usable when detail, processing, and metadata regions fail", async () => {
    apiMocks.getItem.mockRejectedValue(new Error("详情离线"));
    apiMocks.listEpisodeRuns.mockRejectedValue(new Error("转写离线"));
    apiMocks.getNotes.mockRejectedValue(new Error("笔记离线"));

    renderDetail();

    expect(
      await screen.findByText("最新状态读取失败，当前内容仍可查看：保存失败"),
    ).toBeVisible();
    expect(await screen.findByRole("link", { name: "安全链接" })).toBeVisible();
    expect(screen.getByRole("tab", { name: "转写" })).toBeEnabled();

    fireEvent.click(screen.getByRole("tab", { name: "笔记" }));
    expect(
      await screen.findByText("备注与标签加载失败：笔记离线"),
    ).toBeVisible();
    fireEvent.click(screen.getByRole("tab", { name: "Show Notes" }));
    expect(screen.getByRole("link", { name: "安全链接" })).toBeVisible();
  });

  it("opens the original URL even when saving in-progress fails and never auto-completes", async () => {
    apiMocks.markInProgress.mockRejectedValue(new Error("离线"));
    const onMove = vi.fn();
    renderDetail({ onMove });

    fireEvent.click(screen.getByRole("button", { name: "原节目" }));

    expect(window.open).toHaveBeenCalledWith(
      "https://example.com/episode/201",
      "_blank",
      "noopener,noreferrer",
    );
    expect(apiMocks.markInProgress).toHaveBeenCalledWith(201);
    expect(
      await screen.findByText(
        "原节目已打开，但进行中记录未保存。队列没有改变。",
      ),
    ).toBeInTheDocument();
    expect(onMove).not.toHaveBeenCalled();
  });

  it("opens the original URL only from the openable state", async () => {
    renderDetail();

    fireEvent.click(screen.getByRole("button", { name: "原节目" }));

    expect(window.open).toHaveBeenCalledWith(
      "https://example.com/episode/201",
      "_blank",
      "noopener,noreferrer",
    );
  });

  it("shows the shared missing copy when the original link is empty", () => {
    renderDetail({ item: { ...item, original_url: "" } });

    expect(screen.getByText("原节目链接暂缺")).toHaveAttribute(
      "data-original-access",
      "missing",
    );
    expect(
      screen.queryByRole("button", { name: "原节目" }),
    ).not.toBeInTheDocument();
    expect(window.open).not.toHaveBeenCalled();
  });

  it("shows the shared rejected copy for dangerous original links", () => {
    renderDetail({ item: { ...item, original_url: "javascript:alert(1)" } });

    expect(screen.getByText("原节目链接不可安全打开")).toHaveAttribute(
      "data-original-access",
      "rejected",
    );
    expect(
      screen.queryByRole("button", { name: "原节目" }),
    ).not.toBeInTheDocument();
    expect(window.open).not.toHaveBeenCalled();
  });

  it("keeps Xiaoyuzhou recovery available without changing the queue", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    const onMove = vi.fn();
    renderDetail({ item: xyzItem, onMove });

    fireEvent.click(screen.getByRole("button", { name: "原节目" }));

    expect(window.open).toHaveBeenCalledWith(
      xyzItem.original_url,
      "_blank",
      "noopener,noreferrer",
    );
    expect(apiMocks.markInProgress).toHaveBeenCalledTimes(1);
    expect(
      screen.getByRole("region", { name: "原节目页恢复" }),
    ).toHaveTextContent("如果新页面是 403");

    fireEvent.click(screen.getByRole("button", { name: "重试打开" }));
    expect(window.open).toHaveBeenLastCalledWith(
      "https://www.xiaoyuzhoufm.com/episode/6a8cf80a1352af56ff3b7e2d",
      "_blank",
      "noopener,noreferrer",
    );

    fireEvent.click(screen.getByRole("button", { name: "用小宇宙打开" }));
    expect(window.open).toHaveBeenLastCalledWith(
      "cosmos://page.cos/episode/6a8cf80a1352af56ff3b7e2d",
      "_blank",
      "noopener,noreferrer",
    );

    fireEvent.click(screen.getByRole("button", { name: "复制页面链接" }));
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(
        "https://www.xiaoyuzhoufm.com/episode/6a8cf80a1352af56ff3b7e2d",
      ),
    );
    expect(apiMocks.markInProgress).toHaveBeenCalledTimes(1);
    expect(onMove).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "关闭原节目页恢复" }));
    expect(
      screen.queryByRole("region", { name: "原节目页恢复" }),
    ).not.toBeInTheDocument();
  });

  it("shows a copy failure instead of pretending the link was copied", async () => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
    });
    renderDetail({ item: xyzItem });

    fireEvent.click(screen.getByRole("button", { name: "原节目" }));
    fireEvent.click(screen.getByRole("button", { name: "复制页面链接" }));

    expect(
      await screen.findByText("复制失败，请改用重试或用小宇宙打开。"),
    ).toBeInTheDocument();
  });

  it("keeps recovery actions visible when saving and copying both fail", async () => {
    apiMocks.markInProgress.mockRejectedValue(new Error("离线"));
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
    });
    renderDetail({ item: xyzItem });

    fireEvent.click(screen.getByRole("button", { name: "原节目" }));
    expect(
      await screen.findByText(
        "原节目已打开，但进行中记录未保存。队列没有改变。",
      ),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "复制页面链接" }));
    expect(
      await screen.findByText("复制失败，请改用重试或用小宇宙打开。"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("region", { name: "原节目页恢复" }),
    ).toBeInTheDocument();
  });

  it("does not show original-page recovery for ordinary hosts", async () => {
    renderDetail();

    fireEvent.click(screen.getByRole("button", { name: "原节目" }));

    expect(
      screen.queryByRole("region", { name: "原节目页恢复" }),
    ).not.toBeInTheDocument();
    await waitFor(() =>
      expect(apiMocks.markInProgress).toHaveBeenCalledWith(201),
    );
  });

  it("only enters Done after the explicit Done command in the queue menu", async () => {
    const onMove = vi
      .fn<OnMove>()
      .mockResolvedValue({ ...item, queue_state: "done" });
    renderDetail({ onMove });

    fireEvent.click(
      screen.getByRole("button", {
        name: "当前队列 Focus，打开切换菜单",
      }),
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "标记完成" }));

    await waitFor(() => expect(onMove).toHaveBeenCalledWith(item, "done"));
    expect(apiMocks.markInProgress).not.toHaveBeenCalled();
  });

  it("reuses the existing notes and tags APIs instead of creating parallel metadata", async () => {
    renderDetail();

    fireEvent.click(screen.getByRole("tab", { name: "笔记" }));
    expect(await screen.findByText("旧备注")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "编辑单集备注" }));
    fireEvent.change(screen.getByRole("textbox", { name: "单集备注" }), {
      target: { value: "新备注" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存单集备注" }));
    await waitFor(() =>
      expect(apiMocks.updateNotes).toHaveBeenCalledWith(201, "新备注"),
    );

    fireEvent.change(screen.getByRole("combobox", { name: "选择已有标签" }), {
      target: { value: "9" },
    });
    fireEvent.click(screen.getByRole("button", { name: "添加所选标签" }));
    await waitFor(() => expect(apiMocks.addTag).toHaveBeenCalledWith(201, 9));
  });

  it("uses aligned note and tag cards without the redundant section heading", async () => {
    apiMocks.getNotes.mockResolvedValue("");
    apiMocks.getTags.mockResolvedValue([]);
    renderDetail();

    fireEvent.click(screen.getByRole("tab", { name: "笔记" }));

    expect(
      await screen.findByRole("region", { name: "单集笔记与标签" }),
    ).toBeVisible();
    expect(screen.queryByText("备注与标签")).not.toBeInTheDocument();
    expect(
      screen.getByText("暂无备注。记录这一集值得留下的判断。"),
    ).toBeVisible();
    expect(screen.getByText("暂无标签。")).toBeVisible();
    expect(
      screen.getByRole("combobox", { name: "选择已有标签" }),
    ).toBeVisible();
  });

  describe("Focus Detail queue switcher", () => {
    const doneItem: ConsumptionItem = { ...item, queue_state: "done" };

    function queueTrigger(queueLabel: string) {
      const context =
        queueLabel === "Done" || queueLabel === "未收集"
          ? "当前状态"
          : "当前队列";
      return screen.getByRole("button", {
        name: `${context} ${queueLabel}，打开切换菜单`,
      });
    }

    function openQueueMenu(queueLabel: string) {
      fireEvent.click(queueTrigger(queueLabel));
      return screen.getByRole("menu", { name: "切换至" });
    }

    function menuTargets(menu: HTMLElement) {
      return Array.from(
        menu.querySelectorAll<HTMLElement>('[role="menuitem"]'),
      ).map((entry) => entry.textContent);
    }

    it("shows Done inline and reprocesses into an action queue with a single click", async () => {
      const onMove = vi
        .fn<OnMove>()
        .mockResolvedValue({ ...doneItem, queue_state: "focus" });
      const onItemChange = vi.fn<OnItemChange>();
      renderDetail({ item: doneItem, onMove, onItemChange });

      expect(queueTrigger("Done")).toHaveAttribute("aria-haspopup", "menu");
      const menu = openQueueMenu("Done");
      expect(queueTrigger("Done")).toHaveAttribute("aria-expanded", "true");
      expect(menuTargets(menu)).toEqual(["Inbox", "Focus", "Someday"]);
      expect(
        within(menu).queryByRole("menuitem", { name: "标记完成" }),
      ).not.toBeInTheDocument();

      fireEvent.click(within(menu).getByRole("menuitem", { name: "Focus" }));
      expect(onMove).toHaveBeenCalledTimes(1);
      expect(onMove).toHaveBeenCalledWith(doneItem, "focus");
      await waitFor(() =>
        expect(onItemChange).toHaveBeenCalledWith(
          expect.objectContaining({ queue_state: "focus" }),
        ),
      );
      expect(
        screen.queryByRole("menu", { name: "切换至" }),
      ).not.toBeInTheDocument();
    });

    it("keeps an unassigned item safe without changing its available commands", async () => {
      const unassignedItem: ConsumptionItem = { ...item, queue_state: null };
      const onMove = vi
        .fn<OnMove>()
        .mockResolvedValue({ ...unassignedItem, queue_state: "focus" });
      renderDetail({ item: unassignedItem, onMove });

      expect(queueTrigger("未收集")).toHaveTextContent("未收集");
      const menu = openQueueMenu("未收集");
      expect(menuTargets(menu)).toEqual([
        "Inbox",
        "Focus",
        "Someday",
        "标记完成",
      ]);
      expect(menu.querySelector('[role="separator"]')).not.toBeNull();

      fireEvent.click(within(menu).getByRole("menuitem", { name: "Focus" }));
      await waitFor(() =>
        expect(onMove).toHaveBeenCalledWith(unassignedItem, "focus"),
      );
    });

    it("lists the other action queues and keeps 标记完成 as a separated command", async () => {
      const onMove = vi.fn<OnMove>().mockResolvedValue(item);
      renderDetail({ item: { ...item, queue_state: "inbox" }, onMove });

      const menu = openQueueMenu("Inbox");
      expect(menuTargets(menu)).toEqual(["Focus", "Someday", "标记完成"]);
      expect(menu.querySelector('[role="separator"]')).not.toBeNull();
      expect(
        within(menu).queryByRole("menuitem", { name: "Inbox" }),
      ).not.toBeInTheDocument();

      fireEvent.click(within(menu).getByRole("menuitem", { name: "Someday" }));
      await waitFor(() =>
        expect(onMove).toHaveBeenCalledWith(
          expect.objectContaining({ queue_state: "inbox" }),
          "someday",
        ),
      );
      expect(
        screen.queryByRole("menu", { name: "切换至" }),
      ).not.toBeInTheDocument();
      expect(onMove).toHaveBeenCalledTimes(1);
    });

    it("keeps the confirmed queue visible after a failed move so it can be retried", async () => {
      const onMove = vi.fn<OnMove>().mockResolvedValue(undefined);
      renderDetail({ onMove });

      const menu = openQueueMenu("Focus");
      fireEvent.click(within(menu).getByRole("menuitem", { name: "Someday" }));
      await waitFor(() => expect(onMove).toHaveBeenCalledTimes(1));

      expect(queueTrigger("Focus")).toBeVisible();
      await waitFor(() =>
        expect(
          screen.queryByRole("menu", { name: "切换至" }),
        ).not.toBeInTheDocument(),
      );
      const retryMenu = openQueueMenu("Focus");
      expect(
        within(retryMenu).getByRole("menuitem", { name: "Someday" }),
      ).toBeEnabled();
    });

    it("keeps pending feedback visible and closes only after success", async () => {
      let resolveMove!: (item: ConsumptionItem) => void;
      const onMove = vi.fn<OnMove>().mockReturnValue(
        new Promise<ConsumptionItem>((resolve) => {
          resolveMove = resolve;
        }),
      );
      renderDetail({ onMove });

      const menu = openQueueMenu("Focus");
      fireEvent.click(within(menu).getByRole("menuitem", { name: "Someday" }));

      expect(screen.getByRole("menu", { name: "切换至" })).toBeVisible();
      expect(within(menu).getByRole("status")).toHaveTextContent(
        "正在保存队列…",
      );
      for (const target of menuTargets(menu)) {
        expect(
          within(menu).getByRole("menuitem", { name: target ?? "" }),
        ).toBeDisabled();
      }
      expect(queueTrigger("Focus")).toHaveAttribute("aria-disabled", "true");

      await act(async () => {
        resolveMove({ ...item, queue_state: "someday" });
      });
      await waitFor(() =>
        expect(
          screen.queryByRole("menu", { name: "切换至" }),
        ).not.toBeInTheDocument(),
      );
    });

    it("blocks duplicate submissions while a queue save is in flight", () => {
      render(
        <ConsumptionDetailPanel
          item={item}
          isQueueBusy
          onClose={vi.fn()}
          onItemChange={vi.fn()}
          onMove={vi.fn<OnMove>().mockResolvedValue(item)}
        />,
      );

      const trigger = queueTrigger("Focus");
      expect(trigger).toHaveAttribute("aria-disabled", "true");
      fireEvent.click(trigger);
      expect(
        screen.queryByRole("menu", { name: "切换至" }),
      ).not.toBeInTheDocument();
    });

    it("opens, traverses, activates with Enter and Space, and restores focus", async () => {
      const user = userEvent.setup();
      const onMove = vi.fn<OnMove>().mockResolvedValue(item);
      renderDetail({ onMove });

      const trigger = queueTrigger("Focus");
      trigger.focus();
      await user.keyboard("{Enter}");
      const menu = screen.getByRole("menu", { name: "切换至" });
      const targets = within(menu).getAllByRole("menuitem");
      expect(targets[0]).toHaveFocus();

      fireEvent.keyDown(menu, { key: "ArrowDown" });
      expect(targets[1]).toHaveFocus();
      fireEvent.keyDown(menu, { key: "End" });
      expect(targets[2]).toHaveFocus();
      fireEvent.keyDown(menu, { key: "Home" });
      expect(targets[0]).toHaveFocus();
      fireEvent.keyDown(menu, { key: "ArrowUp" });
      expect(targets[2]).toHaveFocus();

      await user.keyboard("{Enter}");
      await waitFor(() => expect(onMove).toHaveBeenCalledTimes(1));
      await waitFor(() =>
        expect(
          screen.queryByRole("menu", { name: "切换至" }),
        ).not.toBeInTheDocument(),
      );

      trigger.focus();
      await user.keyboard(" ");
      const spaceMenu = screen.getByRole("menu", { name: "切换至" });
      expect(within(spaceMenu).getAllByRole("menuitem")[0]).toHaveFocus();
      await user.keyboard(" ");
      await waitFor(() => expect(onMove).toHaveBeenCalledTimes(2));
      await waitFor(() =>
        expect(
          screen.queryByRole("menu", { name: "切换至" }),
        ).not.toBeInTheDocument(),
      );

      // Escape closes and restores focus to the trigger.
      const reopened = openQueueMenu("Focus");
      fireEvent.keyDown(reopened, { key: "Escape" });
      expect(
        screen.queryByRole("menu", { name: "切换至" }),
      ).not.toBeInTheDocument();
      expect(trigger).toHaveFocus();

      // A press outside the menu also closes it.
      openQueueMenu("Focus");
      fireEvent.pointerDown(document.body);
      expect(
        screen.queryByRole("menu", { name: "切换至" }),
      ).not.toBeInTheDocument();
    });
  });

  describe("transcript artifact switcher flow", () => {
    const focusRun: ProcessingRun = {
      id: 301,
      episode_id: item.episode_id,
      pipeline_version: "focus-processing-v2",
      trigger_source: "manual",
      status: "completed",
      current_step: "",
      attempt_count: 1,
      max_attempts: 3,
      error_retryable: false,
      created_at: "2026-09-05T10:00:00Z",
      updated_at: "2026-09-05T10:05:00Z",
    };

    const nativeArtifact: EpisodeArtifactSet = {
      id: 301,
      run_id: focusRun.id,
      episode_id: item.episode_id,
      pipeline_version: focusRun.pipeline_version,
      manifest_path: "manifest.json",
      manifest_sha256: "a".repeat(64),
      minutes_summary_sha256: "b".repeat(64),
      transcript_sha256: "c".repeat(64),
      notes_sha256: "",
      capabilities: {
        minutes_summary: true,
        transcript: true,
        structured_timeline: true,
        matching_audio: true,
        legacy_episode_notes: false,
      },
      is_current: true,
      created_at: "2026-09-05T10:05:00Z",
    };

    const visualMinutesContent: ArtifactContent = {
      kind: "minutes_summary",
      content: "# 纪要\n\n正文锚点",
      sha256: nativeArtifact.minutes_summary_sha256 ?? "",
      media_available: false,
      whiteboard: {
        media_id: "whiteboard",
        media_type: "image/png",
        width: 320,
        height: 180,
        sha256: "d".repeat(64),
        alt: "总结画板",
      },
      visual_items: [
        {
          type: "whiteboard",
          media_id: "whiteboard",
          media_type: "image/png",
          width: 320,
          height: 180,
          sha256: "d".repeat(64),
          alt: "总结画板",
        },
        {
          type: "image",
          media_id: "image-1",
          media_type: "image/png",
          width: 240,
          height: 135,
          sha256: "e".repeat(64),
          alt: "正文插图",
        },
      ],
      inline_images: [
        {
          media_id: "image-1",
          section: "body",
          anchor_text: "正文锚点",
          anchor_occurrence: 1,
        },
      ],
    };

    const plainMinutesContent: ArtifactContent = {
      kind: "minutes_summary",
      content: "# 纪要\n\n正文锚点",
      sha256: nativeArtifact.minutes_summary_sha256 ?? "",
      media_available: false,
      visual_items: [
        {
          type: "image",
          media_id: "image-1",
          media_type: "image/png",
          width: 240,
          height: 135,
          sha256: "e".repeat(64),
          alt: "正文插图",
        },
      ],
      inline_images: [
        { media_id: "image-1", anchor_text: "正文锚点", anchor_occurrence: 1 },
      ],
    };

    const transcriptContent: ArtifactContent = {
      kind: "transcript",
      content: "# 妙记逐字稿",
      sha256: nativeArtifact.transcript_sha256,
      media_available: true,
      segments: [
        { order: 1, speaker: "主持人", start_ms: 0, text: "逐字稿正文" },
      ],
    };

    function mockTranscriptFlow(
      currentArtifact: EpisodeArtifactSet = nativeArtifact,
      run: ProcessingRun = focusRun,
      minutes: ArtifactContent = visualMinutesContent,
    ) {
      apiMocks.listEpisodeRuns.mockResolvedValue([run]);
      apiMocks.getRun.mockResolvedValue({
        run,
        current_artifact: currentArtifact,
        deliveries: [],
      });
      apiMocks.getArtifactContent.mockImplementation(
        (_artifactSetId: number, kind: string) =>
          Promise.resolve(kind === "transcript" ? transcriptContent : minutes),
      );
    }

    async function openTranscriptArea() {
      const dialog = screen.getByRole("dialog", { name: item.episode_title });
      fireEvent.click(await within(dialog).findByRole("tab", { name: "转写" }));
      return dialog;
    }

    it.each([1280, 390])(
      "keeps the switcher subordinate to the main tabs with stable content ownership at %ipx",
      async (viewportWidth) => {
        const previousWidth = window.innerWidth;
        Object.defineProperty(window, "innerWidth", {
          configurable: true,
          value: viewportWidth,
        });
        window.dispatchEvent(new Event("resize"));
        mockTranscriptFlow();
        const view = render(
          <ConsumptionDetailPanel
            item={item}
            isQueueBusy={false}
            onClose={vi.fn()}
            onItemChange={vi.fn()}
            onMove={vi.fn()}
          />,
        );

        try {
          const dialog = await openTranscriptArea();

          const mainTablist = within(dialog).getByRole("tablist", {
            name: "单集详情内容",
          });
          const subTablist = within(dialog).getByRole("tablist", {
            name: "转写产物",
          });
          expect(within(mainTablist).getByRole("tab", { name: "转写" })).toBeVisible();
          expect(await within(dialog).findByText("飞书智能纪要")).toBeVisible();
          expect(within(dialog).getByText("当前版本")).toBeVisible();

          const subTabs = within(subTablist).getAllByRole("tab");
          expect(subTabs.map((tab) => tab.textContent)).toEqual([
            "总结",
            "纪要",
            "逐字稿",
          ]);
          const summaryTab = subTabs[0];
          await waitFor(() =>
            expect(summaryTab).toHaveAttribute("aria-selected", "true"),
          );
          expect(
            await within(dialog).findByRole("img", { name: "总结画板" }),
          ).toBeVisible();
          expect(
            within(dialog).queryByRole("img", { name: "正文插图" }),
          ).not.toBeInTheDocument();

          fireEvent.click(within(dialog).getByRole("tab", { name: "纪要" }));
          expect(
            await within(dialog).findByRole("img", { name: "正文插图" }),
          ).toBeVisible();
          expect(within(dialog).getByText("正文锚点")).toBeVisible();
          expect(
            within(dialog).queryByRole("img", { name: "总结画板" }),
          ).not.toBeInTheDocument();

          fireEvent.click(within(dialog).getByRole("tab", { name: "逐字稿" }));
          expect(
            await within(dialog).findByText("逐字稿 · 1 段"),
          ).toBeVisible();
          expect(within(dialog).getByText("逐字稿正文")).toBeVisible();
          expect(
            within(dialog).queryByText(/暂时显示上一成功内容/),
          ).not.toBeInTheDocument();
        } finally {
          view.unmount();
          Object.defineProperty(window, "innerWidth", {
            configurable: true,
            value: previousWidth,
          });
          window.dispatchEvent(new Event("resize"));
        }
      },
    );

    it("marks updating products, keeps the previous version readable, and never auto-switches after user selection", async () => {
      const updatingRun: ProcessingRun = {
        ...focusRun,
        id: 311,
        status: "waiting_external",
        current_step: "transcription",
      };
      const nextRun: ProcessingRun = { ...focusRun, id: 312 };
      const nextArtifact: EpisodeArtifactSet = {
        ...nativeArtifact,
        id: 312,
        run_id: nextRun.id,
        created_at: "2026-09-05T11:05:00Z",
      };
      const nextMinutes: ArtifactContent = {
        ...visualMinutesContent,
        content: "# 新版纪要\n\n新版正文锚点",
      };
      const nextTranscript: ArtifactContent = {
        ...transcriptContent,
        segments: [
          { order: 1, speaker: "主持人", start_ms: 0, text: "新版逐字稿正文" },
        ],
      };
      apiMocks.listEpisodeRuns.mockResolvedValue([updatingRun]);
      apiMocks.getRun
        .mockResolvedValueOnce({
          run: updatingRun,
          current_artifact: nativeArtifact,
          deliveries: [],
        })
        .mockResolvedValue({
          run: nextRun,
          current_artifact: nextArtifact,
          deliveries: [],
        });
      apiMocks.getArtifactContent.mockImplementation(
        (_artifactSetId: number, kind: string) => {
          const isNew = _artifactSetId === nextArtifact.id;
          return Promise.resolve(
            kind === "transcript"
              ? isNew
                ? nextTranscript
                : transcriptContent
              : isNew
                ? nextMinutes
                : visualMinutesContent,
          );
        },
      );
      renderDetail();
      const dialog = await openTranscriptArea();

      fireEvent.click(await within(dialog).findByRole("tab", { name: "纪要" }));
      const minutesTab = within(dialog).getByRole("tab", { name: "纪要" });
      expect(await within(dialog).findByText("正文锚点")).toBeVisible();
      expect(
        within(dialog).getByText("正在生成新版纪要，当前展示上一成功版本。"),
      ).toBeVisible();
      expect(minutesTab.querySelector('[data-state="working"]')).not.toBeNull();
      expect(
        within(dialog).getByText("纪要，正在生成新版，当前展示上一成功版本"),
      ).toBeVisible();

      fireEvent.click(within(dialog).getByRole("tab", { name: "逐字稿" }));
      expect(await within(dialog).findByText("逐字稿正文")).toBeVisible();

      expect(
        await within(dialog).findByText("新版逐字稿正文", undefined, {
          timeout: 6000,
        }),
      ).toBeVisible();
      expect(
        within(dialog).getByRole("tab", { name: "逐字稿" }),
      ).toHaveAttribute("aria-selected", "true");
      expect(
        within(dialog).queryByText("正在生成新版纪要，当前展示上一成功版本。"),
      ).not.toBeInTheDocument();
      expect(within(dialog).getByText("纪要，已可用")).toBeVisible();
      expect(
        within(dialog).getByRole("status", { name: "转写产物状态" }),
      ).toHaveTextContent("逐字稿，已可用");
    });

    it("keeps expected products visible with stable processing states on a first run", async () => {
      const queuedRun: ProcessingRun = {
        ...focusRun,
        id: 321,
        status: "queued",
        current_step: "transcription",
      };
      apiMocks.getRun.mockResolvedValue({ run: queuedRun, deliveries: [] });
      apiMocks.startProcessing.mockResolvedValue({
        run: queuedRun,
        reused_active: false,
        reused_successful: false,
        preparing_audio: false,
      });
      renderDetail();

      fireEvent.click(
        await screen.findByRole("button", { name: "开始转写" }),
      );
      const dialog = await openTranscriptArea();
      const subTablist = await within(dialog).findByRole("tablist", {
        name: "转写产物",
      });
      const subTabs = within(subTablist).getAllByRole("tab");
      expect(subTabs.map((tab) => tab.textContent)).toEqual([
        "总结",
        "纪要",
        "逐字稿",
      ]);
      for (const tab of subTabs) {
        expect(tab.querySelector('[data-state="working"]')).not.toBeNull();
      }
      expect(
        within(dialog).getByText("总结，生成中"),
      ).toBeVisible();
      expect(
        within(dialog).queryByText("正在读取纪要…"),
      ).not.toBeInTheDocument();

      for (const tab of subTabs) {
        fireEvent.click(tab);
        expect(within(dialog).getByText("转写进行中")).toBeVisible();
      }
    });

    it("isolates a read failure to its own view and keeps the existing retry", async () => {
      let transcriptReads = 0;
      mockTranscriptFlow();
      apiMocks.getArtifactContent.mockImplementation(
        (_artifactSetId: number, kind: string) => {
          if (kind === "transcript") {
            transcriptReads += 1;
            return transcriptReads === 1
              ? Promise.reject(new Error("逐字稿暂时打不开"))
              : Promise.resolve(transcriptContent);
          }
          return Promise.resolve(visualMinutesContent);
        },
      );
      renderDetail();
      const dialog = await openTranscriptArea();

      fireEvent.click(await within(dialog).findByRole("tab", { name: "纪要" }));
      expect(await within(dialog).findByText("正文锚点")).toBeVisible();
      fireEvent.click(within(dialog).getByRole("tab", { name: "逐字稿" }));
      expect(
        await within(dialog).findByText("暂时无法读取逐字稿，请重试。"),
      ).toBeVisible();
      expect(
        within(dialog).getByRole("button", { name: "重试读取逐字稿" }),
      ).toBeEnabled();
      expect(
        within(dialog).getByText("逐字稿，读取失败", {
          selector: '[id^="processing-artifact-tab-state-"]',
        }),
      ).toBeVisible();
      expect(
        within(dialog).getByRole("status", { name: "转写产物状态" }),
      ).toHaveTextContent("逐字稿，读取失败");

      fireEvent.click(
        within(dialog).getByRole("button", { name: "重试读取逐字稿" }),
      );
      expect(await within(dialog).findByText("逐字稿正文")).toBeVisible();
      expect(
        within(dialog).queryByText("暂时无法读取逐字稿，请重试。"),
      ).not.toBeInTheDocument();

      fireEvent.click(within(dialog).getByRole("tab", { name: "纪要" }));
      expect(await within(dialog).findByText("正文锚点")).toBeVisible();
    });

    it("keeps the summary entry during slow reads and hides it only after confirming no managed visuals", async () => {
      mockTranscriptFlow();
      let resolveMinutes!: (content: ArtifactContent) => void;
      apiMocks.getArtifactContent.mockImplementation(
        (_artifactSetId: number, kind: string) =>
          kind === "transcript"
            ? Promise.resolve(transcriptContent)
            : new Promise<ArtifactContent>((resolve) => {
                resolveMinutes = resolve;
              }),
      );
      renderDetail();
      const dialog = await openTranscriptArea();

      const summaryTab = await within(dialog).findByRole("tab", {
        name: "总结",
      });
      expect(within(dialog).getByText("正在读取纪要…")).toBeVisible();
      fireEvent.click(summaryTab);
      expect(
        within(dialog).getByRole("tabpanel", { name: "总结" }),
      ).toBeVisible();

      await act(async () => {
        resolveMinutes(plainMinutesContent);
      });
      expect(
        await within(dialog).findByText(
          "本版本没有受管画板或图片，视觉总结不存在，已切换到纪要。",
        ),
      ).toBeVisible();
      expect(
        within(dialog).queryByRole("tab", { name: "总结" }),
      ).not.toBeInTheDocument();
      expect(
        within(dialog).getByRole("tab", { name: "纪要" }),
      ).toHaveAttribute("aria-selected", "true");
      expect(
        await within(dialog).findByRole("img", { name: "正文插图" }),
      ).toBeVisible();
    });

    it("keeps every visible state reachable by keyboard", async () => {
      mockTranscriptFlow();
      renderDetail();
      const dialog = await openTranscriptArea();

      const summaryTab = await within(dialog).findByRole("tab", {
        name: "总结",
      });
      await within(dialog).findByRole("img", { name: "总结画板" });
      const minutesTab = within(dialog).getByRole("tab", { name: "纪要" });
      const transcriptTab = within(dialog).getByRole("tab", {
        name: "逐字稿",
      });

      summaryTab.focus();
      fireEvent.keyDown(summaryTab, { key: "ArrowRight" });
      expect(minutesTab).toHaveFocus();
      expect(minutesTab).toHaveAttribute("aria-selected", "true");
      fireEvent.keyDown(minutesTab, { key: "ArrowLeft" });
      expect(summaryTab).toHaveFocus();
      fireEvent.keyDown(summaryTab, { key: "End" });
      expect(transcriptTab).toHaveFocus();
      expect(transcriptTab).toHaveAttribute("aria-selected", "true");
      fireEvent.keyDown(transcriptTab, { key: "Home" });
      expect(summaryTab).toHaveFocus();
      expect(summaryTab).toHaveAttribute("tabindex", "0");
      expect(minutesTab).toHaveAttribute("tabindex", "-1");
      expect(transcriptTab).toHaveAttribute("tabindex", "-1");
    });

    it("reports 转写就绪 without duplicated copy and drops 查看转写 on the transcript tab", async () => {
      mockTranscriptFlow();
      renderDetail();

      const headline = await screen.findByRole("status", {
        name: "转写状态：转写就绪",
      });
      expect(headline).not.toHaveTextContent("已完成");
      expect(headline).not.toHaveTextContent("已有可阅读的转写产物");
      expect(screen.getByRole("button", { name: "查看转写" })).toBeVisible();

      // Reading the transcript replaces the duplicate entry point instead of
      // repeating it.
      const dialog = await openTranscriptArea();
      expect(
        screen.queryByRole("button", { name: "查看转写" }),
      ).not.toBeInTheDocument();
      expect(
        screen.getByRole("status", { name: "转写状态：转写就绪" }),
      ).toBeVisible();

      const subTablist = await within(dialog).findByRole("tablist", {
        name: "转写产物",
      });
      fireEvent.click(within(subTablist).getByRole("tab", { name: "逐字稿" }));
      expect(await within(dialog).findByText("逐字稿正文")).toBeVisible();
      expect(
        screen.queryByRole("button", { name: "查看转写" }),
      ).not.toBeInTheDocument();
    });
  });
});
