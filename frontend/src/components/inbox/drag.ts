import type { ConsumptionItem, ConsumptionQueue } from "@/types/consumption";

export const queueDropId = (queue: ConsumptionQueue) => `queue:${queue}`;
export const episodeDragId = (episodeId: number) => `episode:${episodeId}`;

export interface QueueDragData {
  kind: "item" | "queue";
  queue: ConsumptionQueue;
  episodeId?: number;
}

export interface QueuePlacementPreview {
  queue: ConsumptionQueue;
  beforeEpisodeId: number | null;
}

interface ResolveQueuePlacementOptions {
  sourceQueue: ConsumptionQueue;
  targetQueue: ConsumptionQueue;
  activeEpisodeId: number;
  targetItems: ConsumptionItem[];
  overEpisodeId: number | null;
  placeAfter: boolean;
}

export function resolveQueuePlacement({
  sourceQueue,
  targetQueue,
  activeEpisodeId,
  targetItems,
  overEpisodeId,
  placeAfter,
}: ResolveQueuePlacementOptions): QueuePlacementPreview | null {
  if (targetQueue === "done") {
    return { queue: "done", beforeEpisodeId: null };
  }
  const targetIDs = targetItems.map((item) => item.episode_id);
  const withoutActive = targetIDs.filter((id) => id !== activeEpisodeId);

  if (overEpisodeId === null) {
    return { queue: targetQueue, beforeEpisodeId: null };
  }

  const overIndex = targetIDs.indexOf(overEpisodeId);
  if (overIndex < 0) return null;

  let insertionIndex = overIndex + (placeAfter ? 1 : 0);
  const activeIndex = targetIDs.indexOf(activeEpisodeId);
  if (
    sourceQueue === targetQueue &&
    activeIndex >= 0 &&
    activeIndex < insertionIndex
  ) {
    insertionIndex--;
  }

  return {
    queue: targetQueue,
    beforeEpisodeId: withoutActive[insertionIndex] ?? null,
  };
}

export function isNoOpQueuePlacement(
  sourceQueue: ConsumptionQueue,
  targetQueue: ConsumptionQueue,
  sourceItems: ConsumptionItem[],
  activeEpisodeId: number,
  beforeEpisodeId: number | null,
) {
  if (sourceQueue === "done" && targetQueue === "done") return true;
  if (sourceQueue !== targetQueue) return false;
  const activeIndex = sourceItems.findIndex(
    (item) => item.episode_id === activeEpisodeId,
  );
  if (activeIndex < 0) return false;
  return (sourceItems[activeIndex + 1]?.episode_id ?? null) === beforeEpisodeId;
}
