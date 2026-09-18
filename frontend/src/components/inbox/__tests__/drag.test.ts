import { describe, expect, it } from "vitest";
import type { ConsumptionItem } from "@/types/consumption";
import {
  queueCollisionDetection,
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

// Use the real dnd-kit algorithms; page interaction tests mock them.
describe("queueCollisionDetection", () => {
  const rect = (left: number, top: number, width: number, height: number) => ({
    left, top, width, height, right: left + width, bottom: top + height,
  });
  function args(pointer: { x: number; y: number } | null, fullColumn = true) {
    const targets = [
      { id: "queue:someday", kind: "queue", queue: "someday", bounds: rect(612, 100, 276, 700) },
      { id: "item:someday", kind: "item", queue: "someday", bounds: rect(612, 590, 276, 110) },
      { id: "queue:done", kind: "queue", queue: "done", bounds: rect(912, 100, 276, fullColumn ? 700 : 440) },
    ];
    return {
      active: { id: "moving", data: { current: {} }, rect: { current: { initial: null, translated: null } } },
      collisionRect: rect(935, 635, 260, 110),
      droppableRects: new Map(targets.map(target => [target.id, target.bounds])),
      droppableContainers: targets.map(target => ({
        id: target.id, key: target.id, disabled: false,
        data: { current: { kind: target.kind, queue: target.queue } },
        node: { current: null }, rect: { current: target.bounds },
      })),
      pointerCoordinates: pointer,
    };
  }
  it("targets Done blank space even when a Someday card is closer", () => {
    expect(queueCollisionDetection(args({ x: 950, y: 650 }))[0]?.id).toBe("queue:done");
  });
  it("does not redirect uncovered space, gaps or outside drops to a neighbor", () => {
    expect(queueCollisionDetection(args({ x: 950, y: 650 }, false))).toEqual([]);
    expect(queueCollisionDetection(args({ x: 900, y: 650 }))).toEqual([]);
    expect(queueCollisionDetection(args({ x: 1200, y: 650 }))).toEqual([]);
  });
  it("prefers an item over its column for precise ordering", () => {
    expect(queueCollisionDetection(args({ x: 650, y: 650 }))[0]?.id).toBe("item:someday");
  });
  it("preserves fallback without pointer coordinates", () => {
    expect(queueCollisionDetection(args(null, false))[0]?.id).toBe("item:someday");
  });
  it("excludes the active card", () => {
    const input = args({ x: 650, y: 650 });
    input.active.id = "item:someday";
    expect(queueCollisionDetection(input)[0]?.id).toBe("queue:someday");
  });
});
