import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ConsumptionDetailPanel from "../ConsumptionDetailPanel";
import type { ConsumptionItem } from "@/types/consumption";

const apiMocks = vi.hoisted(() => ({
  getItem: vi.fn(),
  markInProgress: vi.fn(),
  getConsumptionErrorDetails: vi.fn(() => ({ message: "保存失败" })),
  getNotes: vi.fn(),
  getTags: vi.fn(),
  updateNotes: vi.fn(),
  addTag: vi.fn(),
  removeTag: vi.fn(),
  listTags: vi.fn(),
  listEpisodeRuns: vi.fn(),
  getLatestAudio: vi.fn(),
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
  },
  getProcessingErrorDetails: vi.fn((error: unknown) => ({
    message: error instanceof Error ? error.message : "加工状态读取失败",
  })),
}));

vi.mock("@/lib/api/episodeCopilot", () => ({
  episodeCopilotApi: {
    getContext: apiMocks.getCopilotContext,
    ask: apiMocks.askCopilot,
  },
  isEpisodeCopilotCancellation: vi.fn(() => false),
}));

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
  show_notes: [
    '<p><a href="https://example.com/transcript">安全链接</a></p>',
    '<p><a href="mailto:owner@example.com">邮件链接</a></p>',
    '<p><a href="tel:+8613800000000">电话链接</a></p>',
    '<p><a href="javascript:alert(1)">危险链接</a></p>',
    '<img src="https://i.typlog.com/cover.png" alt="允许图片">',
    '<img src="https://evil.example/track.png" alt="拒绝图片">',
  ].join(""),
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

    expect(screen.getByRole("link", { name: "安全链接" })).toHaveAttribute(
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
      screen.getByRole("heading", { name: "单集助手" }),
    ).toBeInTheDocument();
    expect(
      container.querySelector('[data-copilot-source="show_notes"]'),
    ).toHaveAttribute("data-copilot-episode-id", "201");
    await waitFor(() => {
      expect(screen.queryByText("正在读取…")).not.toBeInTheDocument();
      expect(screen.queryByText("正在同步最新状态…")).not.toBeInTheDocument();
    });
  });

  it("opens the original URL even when saving in-progress fails and never auto-completes", async () => {
    apiMocks.markInProgress.mockRejectedValue(new Error("离线"));
    const onMove = vi.fn();
    renderDetail({ onMove });

    fireEvent.click(screen.getByRole("button", { name: "打开原节目" }));

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

  it("keeps Xiaoyuzhou recovery available without changing the queue", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    const onMove = vi.fn();
    renderDetail({ item: xyzItem, onMove });

    fireEvent.click(screen.getByRole("button", { name: "打开原节目" }));

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

    fireEvent.click(screen.getByRole("button", { name: "打开原节目" }));
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

    fireEvent.click(screen.getByRole("button", { name: "打开原节目" }));
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

    fireEvent.click(screen.getByRole("button", { name: "打开原节目" }));

    expect(
      screen.queryByRole("region", { name: "原节目页恢复" }),
    ).not.toBeInTheDocument();
    await waitFor(() =>
      expect(apiMocks.markInProgress).toHaveBeenCalledWith(201),
    );
  });

  it("only enters Done after the explicit Done action", async () => {
    const onMove = vi
      .fn<OnMove>()
      .mockResolvedValue({ ...item, queue_state: "done" });
    renderDetail({ onMove });

    fireEvent.click(screen.getByRole("button", { name: "标记完成" }));

    await waitFor(() => expect(onMove).toHaveBeenCalledWith(item, "done"));
    expect(apiMocks.markInProgress).not.toHaveBeenCalled();
  });

  it("reuses the existing notes and tags APIs instead of creating parallel metadata", async () => {
    renderDetail();

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
});
