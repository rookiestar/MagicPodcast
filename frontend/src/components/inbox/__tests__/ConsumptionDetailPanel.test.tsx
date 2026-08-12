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
    onItemChange: OnItemChange;
    onMove: OnMove;
  }> = {},
) {
  const onItemChange = overrides.onItemChange ?? vi.fn<OnItemChange>();
  const onMove =
    overrides.onMove ??
    vi.fn<OnMove>().mockResolvedValue({ ...item, queue_state: "done" });
  render(
    <ConsumptionDetailPanel
      item={item}
      isQueueBusy={false}
      onClose={vi.fn()}
      onItemChange={onItemChange}
      onMove={onMove}
    />,
  );
  return { onItemChange, onMove };
}

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

  it("only enters Done after the explicit Done action", async () => {
    const onMove = vi
      .fn<OnMove>()
      .mockResolvedValue({ ...item, queue_state: "done" });
    renderDetail({ onMove });

    fireEvent.click(screen.getByRole("button", { name: "标记 Done" }));

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
