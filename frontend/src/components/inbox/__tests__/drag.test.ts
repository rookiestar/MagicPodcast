import { describe, expect, it } from "vitest";
import type { ConsumptionItem } from "@/types/consumption";
import {
  isNoOpQueuePlacement,
  resolveQueuePlacement,
} from "../drag";

function item(episodeId: number): ConsumptionItem {
  return {
    episode_id: episodeId,
    podcast_id: 1,
    podcast_title: "测试节目",
    podcast_author: "测试作者",
    podcast_cover_url: "",
    episode_title: `单集 ${episodeId}`,
    episode_no: "",
    duration: 60,
    published_date: "2026-08-22T00:00:00Z",
    show_notes: "",
    original_url: "",
    image_url: "",
    notes: "",
    tags: [],
    queue_state: "inbox",
  };
}

describe("resolveQueuePlacement", () => {
  it("将同泳道卡片准确转换为插在目标卡之后", () => {
    const items = [item(1), item(2), item(3)];

    expect(
      resolveQueuePlacement({
        sourceQueue: "inbox",
        targetQueue: "inbox",
        activeEpisodeId: 1,
        targetItems: items,
        overEpisodeId: 2,
        placeAfter: true,
      }),
    ).toEqual({ queue: "inbox", beforeEpisodeId: 3 });
  });

  it("支持跨泳道精确插入和空泳道落点", () => {
    const targetItems = [item(11), item(12)];

    expect(
      resolveQueuePlacement({
        sourceQueue: "inbox",
        targetQueue: "focus",
        activeEpisodeId: 1,
        targetItems,
        overEpisodeId: 11,
        placeAfter: false,
      }),
    ).toEqual({ queue: "focus", beforeEpisodeId: 11 });
    expect(
      resolveQueuePlacement({
        sourceQueue: "inbox",
        targetQueue: "done",
        activeEpisodeId: 1,
        targetItems,
        overEpisodeId: 11,
        placeAfter: false,
      }),
    ).toEqual({ queue: "done", beforeEpisodeId: null });
  });

  it("识别同泳道原位释放", () => {
    const items = [item(1), item(2), item(3)];

    expect(isNoOpQueuePlacement("inbox", "inbox", items, 2, 3)).toBe(true);
    expect(isNoOpQueuePlacement("inbox", "inbox", items, 2, 1)).toBe(false);
    expect(isNoOpQueuePlacement("inbox", "focus", items, 2, 3)).toBe(false);
    expect(isNoOpQueuePlacement("done", "done", items, 1, 2)).toBe(true);
  });
});
