import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DiscoveryFocusSummary from "../DiscoveryFocusSummary";
import type { ConsumptionItem } from "@/types/consumption";

const apiMocks = vi.hoisted(() => ({
  getSummary: vi.fn(),
  listQueue: vi.fn(),
  setQueue: vi.fn(),
  getConsumptionErrorDetails: vi.fn(() => ({ message: "保存失败" })),
  requiresFocusConfirmation: vi.fn(() => false),
}));

vi.mock("@/lib/api/consumption", () => ({
  consumptionApi: {
    getSummary: apiMocks.getSummary,
    listQueue: apiMocks.listQueue,
    setQueue: apiMocks.setQueue,
  },
  getConsumptionErrorDetails: apiMocks.getConsumptionErrorDetails,
  requiresFocusConfirmation: apiMocks.requiresFocusConfirmation,
}));

const inboxItem: ConsumptionItem = {
  episode_id: 11,
  podcast_id: 1,
  podcast_title: "声东击西",
  podcast_author: "声动活泼",
  podcast_cover_url: "",
  episode_title: "值得投入的单集",
  episode_no: "11",
  duration: 1800,
  published_date: "2026-08-11T08:00:00Z",
  show_notes: "<p>正文</p>",
  original_url: "https://example.com/11",
  image_url: "",
  notes: "",
  tags: [],
  queue_state: "inbox",
};

function renderSummary(onQueueChange = vi.fn()) {
  return render(
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      <DiscoveryFocusSummary onQueueChange={onQueueChange} />
    </SWRConfig>,
  );
}

describe("DiscoveryFocusSummary", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMocks.getSummary.mockResolvedValue({
      counts: { inbox: 1, focus: 7, someday: 0, done: 0 },
      focus_limit: 7,
      focus_over_limit: false,
    });
    apiMocks.listQueue.mockImplementation(async (queue: string) => ({
      queue_state: queue,
      items:
        queue === "focus"
          ? Array.from({ length: 7 }, (_, index) => ({
                ...inboxItem,
                episode_id: index + 7,
                episode_title:
                  index === 0 ? "当前 Focus" : `Focus 条目 ${index + 1}`,
                queue_state: "focus",
              }))
          : [inboxItem],
    }));
    apiMocks.setQueue.mockResolvedValue({ ...inboxItem, queue_state: "focus" });
  });

  it("shows a Focus summary and only adds an existing Inbox item", async () => {
    apiMocks.getSummary.mockResolvedValue({
      counts: { inbox: 1, focus: 1, someday: 0, done: 0 },
      focus_limit: 7,
      focus_over_limit: false,
    });
    const onQueueChange = vi.fn();
    renderSummary(onQueueChange);

    expect(await screen.findByText("当前 Focus")).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "从 Inbox 添加到 Focus" }),
    );
    const add = await screen.findByRole("button", {
      name: "将 值得投入的单集 添加到 Focus",
    });
    expect(screen.getByText("值得投入的单集")).toHaveAttribute(
      "data-editorial-display-text",
      "true",
    );
    fireEvent.click(add);

    await waitFor(() => {
      expect(apiMocks.setQueue).toHaveBeenCalledWith(11, "focus", {
        acknowledgeFocusLimit: false,
      });
    });
    expect(onQueueChange).toHaveBeenCalledWith(
      expect.objectContaining({ episode_id: 11, queue_state: "focus" }),
    );
  });

  it("renders every Focus item as a detail link with a full-title tooltip", async () => {
    renderSummary();

    await screen.findByText("Focus 条目 7");
    const links = screen.getAllByRole("link");
    expect(links[0]).toHaveAttribute("href", "/inbox?queue=focus");
    expect(links).toHaveLength(8);
    expect(links[7]).toHaveAttribute("href", "/episodes/13?from=discovery");
    expect(links[7]).toHaveAttribute("title", "Focus 条目 7");
  });

  it("requires explicit confirmation before exceeding the Focus soft limit", async () => {
    apiMocks.getSummary.mockResolvedValue({
      counts: { inbox: 1, focus: 7, someday: 0, done: 0 },
      focus_limit: 7,
      focus_over_limit: false,
    });
    renderSummary();

    fireEvent.click(
      await screen.findByRole("button", { name: "从 Inbox 添加到 Focus" }),
    );
    fireEvent.click(
      await screen.findByRole("button", {
        name: "将 值得投入的单集 添加到 Focus",
      }),
    );

    expect(
      screen.getByRole("alertdialog", { name: "确认超过 Focus 软上限" }),
    ).toBeInTheDocument();
    expect(apiMocks.setQueue).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "仍加入 Focus" }));
    await waitFor(() => {
      expect(apiMocks.setQueue).toHaveBeenCalledWith(11, "focus", {
        acknowledgeFocusLimit: true,
      });
    });
  });
});
