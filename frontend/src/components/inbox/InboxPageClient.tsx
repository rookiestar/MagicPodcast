"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  closestCorners,
  DndContext,
  DragOverlay,
  MouseSensor,
  pointerWithin,
  TouchSensor,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragCancelEvent,
  type DragEndEvent,
  type DragMoveEvent,
  type DragOverEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import {
  IconAlertTriangle,
  IconArrowBackUp,
  IconArrowRight,
  IconCircleCheck,
  IconRefresh,
} from "@tabler/icons-react";
import PageLayout from "@/components/layout/PageLayout";
import {
  consumptionApi,
  getConsumptionErrorDetails,
  isCompletionUndoConflict,
  isCompletionUndoExpired,
  isQueueOrderConflict,
  requiresFocusConfirmation,
} from "@/lib/api/consumption";
import {
  CONSUMPTION_QUEUES,
  type CompletionUndo,
  type ConsumptionItem,
  type ConsumptionQueue,
  type ConsumptionSummary,
} from "@/types/consumption";
import ConsumptionDetailPanel from "./ConsumptionDetailPanel";
import ConsumptionQueueColumn from "./ConsumptionQueueColumn";
import FocusLimitDialog from "./FocusLimitDialog";
import {
  isNoOpQueuePlacement,
  resolveQueuePlacement,
  type QueueDragData,
  type QueuePlacementPreview,
} from "./drag";
import styles from "./InboxPage.module.css";

interface QueueViewState {
  items: ConsumptionItem[];
  revision: number | null;
  hasMore: boolean;
  isLoading: boolean;
  error: string | null;
}

type QueueViewStateMap = Record<ConsumptionQueue, QueueViewState>;

interface FocusPrompt {
  item: ConsumptionItem;
  currentCount: number;
  limit: number;
  placement?: QueuePlacementPreview;
}

interface FailedAction {
  item: ConsumptionItem;
  target: ConsumptionQueue;
  acknowledgeFocusLimit: boolean;
  message: string;
  placement?: QueuePlacementPreview;
  isQueueOrderConflict?: boolean;
}

interface ActiveQueueDrag {
  item: ConsumptionItem;
  source: ConsumptionQueue;
}

interface CompletionUndoNotice {
  episodeId: number;
  episodeTitle: string;
  token: string;
  expiresAt: number;
}

const queueCollisionDetection: CollisionDetection = (args) => {
  const droppableContainers = args.droppableContainers.filter(
    (container) => container.id !== args.active.id,
  );
  const pointerCollisions = pointerWithin({ ...args, droppableContainers });
  const itemCollision = pointerCollisions.find(
    (collision) =>
      collision.data?.droppableContainer.data.current?.kind === "item",
  );
  if (itemCollision) return [itemCollision];
  if (pointerCollisions.length > 0) return pointerCollisions;
  return closestCorners({ ...args, droppableContainers });
};

function isSameQueuePlacement(
  left: QueuePlacementPreview | null,
  right: QueuePlacementPreview | null,
) {
  return (
    left?.queue === right?.queue &&
    left?.beforeEpisodeId === right?.beforeEpisodeId
  );
}

function makeInitialQueues(): QueueViewStateMap {
  return {
    inbox: {
      items: [],
      revision: null,
      hasMore: false,
      isLoading: true,
      error: null,
    },
    focus: {
      items: [],
      revision: null,
      hasMore: false,
      isLoading: true,
      error: null,
    },
    someday: {
      items: [],
      revision: null,
      hasMore: false,
      isLoading: true,
      error: null,
    },
    done: {
      items: [],
      revision: null,
      hasMore: false,
      isLoading: true,
      error: null,
    },
  };
}

function cloneQueues(previous: QueueViewStateMap): QueueViewStateMap {
  return {
    inbox: { ...previous.inbox, items: [...previous.inbox.items] },
    focus: { ...previous.focus, items: [...previous.focus.items] },
    someday: { ...previous.someday, items: [...previous.someday.items] },
    done: { ...previous.done, items: [...previous.done.items] },
  };
}

function replaceQueueSnapshots(
  previous: QueueViewStateMap,
  snapshots: Partial<
    Record<
      ConsumptionQueue,
      { revision: number; items: ConsumptionItem[]; has_more: boolean }
    >
  >,
): QueueViewStateMap {
  const next = cloneQueues(previous);
  for (const queue of CONSUMPTION_QUEUES) {
    const snapshot = snapshots[queue];
    if (!snapshot) continue;
    next[queue] = {
      ...next[queue],
      items: snapshot.items,
      revision: snapshot.revision,
      hasMore: snapshot.has_more,
      isLoading: false,
      error: null,
    };
  }
  return next;
}

function affectedQueues(
  source: ConsumptionQueue | null,
  target: ConsumptionQueue,
) {
  return source && source !== target ? [source, target] : [target];
}

function queuesForRefresh(
  ...candidates: Array<ConsumptionQueue | null | undefined>
) {
  return CONSUMPTION_QUEUES.filter((queue) => candidates.includes(queue));
}

function expectedRevisionsForPlacement(
  source: ConsumptionQueue | null,
  target: ConsumptionQueue,
  queues: QueueViewStateMap,
) {
  const revisions: Partial<Record<ConsumptionQueue, number>> = {};
  for (const queue of affectedQueues(source, target)) {
    const revision = queues[queue].revision;
    if (revision !== null) revisions[queue] = revision;
  }
  return revisions;
}

function hasExpectedRevisions(
  source: ConsumptionQueue | null,
  target: ConsumptionQueue,
  revisions: Partial<Record<ConsumptionQueue, number>>,
) {
  return affectedQueues(source, target).every(
    (queue) => typeof revisions[queue] === "number" && revisions[queue]! > 0,
  );
}

function previewQueuePlacement(
  previous: QueueViewStateMap,
  item: ConsumptionItem,
  source: ConsumptionQueue | null,
  placement: QueuePlacementPreview,
): QueueViewStateMap {
  const next = cloneQueues(previous);
  const target = placement.queue;
  const movedItem: ConsumptionItem =
    source === target
      ? item
      : {
          ...item,
          queue_state: target,
          queue_updated_at: new Date().toISOString(),
          completed_at:
            target === "done" ? new Date().toISOString() : item.completed_at,
          in_progress_at: target === "done" ? undefined : item.in_progress_at,
          attention: "",
        };

  if (source) {
    next[source] = {
      ...next[source],
      items: next[source].items.filter(
        (candidate) => candidate.episode_id !== item.episode_id,
      ),
    };
  }

  const targetItems = next[target].items.filter(
    (candidate) => candidate.episode_id !== item.episode_id,
  );
  const insertionIndex =
    target === "done"
      ? 0
      : placement.beforeEpisodeId === null
      ? targetItems.length
      : targetItems.findIndex(
          (candidate) => candidate.episode_id === placement.beforeEpisodeId,
        );
  targetItems.splice(
    insertionIndex < 0 ? targetItems.length : insertionIndex,
    0,
    movedItem,
  );
  next[target] = { ...next[target], items: targetItems };
  return next;
}

function readQueueDragData(value: unknown): QueueDragData | null {
  if (!value || typeof value !== "object") return null;
  const candidate = value as Partial<QueueDragData>;
  if (
    (candidate.kind !== "item" && candidate.kind !== "queue") ||
    !candidate.queue ||
    !CONSUMPTION_QUEUES.includes(candidate.queue)
  ) {
    return null;
  }
  if (candidate.kind === "item" && typeof candidate.episodeId !== "number") {
    return null;
  }
  return candidate as QueueDragData;
}

function updateItemAcrossQueues(
  previous: QueueViewStateMap,
  item: ConsumptionItem,
): QueueViewStateMap {
  const next = { ...previous };
  for (const queue of CONSUMPTION_QUEUES) {
    next[queue] = {
      ...previous[queue],
      items: previous[queue].items.filter(
        (candidate) => candidate.episode_id !== item.episode_id,
      ),
    };
  }
  if (item.queue_state) {
    const existingIndex = previous[item.queue_state].items.findIndex(
      (candidate) => candidate.episode_id === item.episode_id,
    );
    const targetItems = [...next[item.queue_state].items];
    targetItems.splice(existingIndex >= 0 ? existingIndex : 0, 0, item);
    next[item.queue_state] = {
      ...next[item.queue_state],
      items: targetItems,
    };
  }
  return next;
}

function adjustSummary(
  previous: ConsumptionSummary | null,
  source: ConsumptionQueue | null,
  target: ConsumptionQueue,
  direction: 1 | -1,
) {
  if (!previous || source === target) return previous;
  const counts = { ...previous.counts };
  if (source) {
    counts[source] = Math.max(0, counts[source] - direction);
  }
  counts[target] = Math.max(0, counts[target] + direction);
  return {
    ...previous,
    counts,
    focus_over_limit: counts.focus > previous.focus_limit,
  };
}

export default function InboxPageClient() {
  const [queues, setQueues] = useState<QueueViewStateMap>(makeInitialQueues);
  const [summary, setSummary] = useState<ConsumptionSummary | null>(null);
  const [summaryError, setSummaryError] = useState<string | null>(null);
  const [busyEpisodes, setBusyEpisodes] = useState<Set<number>>(
    () => new Set(),
  );
  const [detailItem, setDetailItem] = useState<ConsumptionItem | null>(null);
  const [focusPrompt, setFocusPrompt] = useState<FocusPrompt | null>(null);
  const [failedAction, setFailedAction] = useState<FailedAction | null>(null);
  const [completionUndos, setCompletionUndos] = useState<
    CompletionUndoNotice[]
  >([]);
  const [completionUndoError, setCompletionUndoError] = useState<string | null>(
    null,
  );
  const [undoNow, setUndoNow] = useState(() => Date.now());
  const [announcement, setAnnouncement] = useState("");
  const [dragEnabled, setDragEnabled] = useState(false);
  const [activeDrag, setActiveDrag] = useState<ActiveQueueDrag | null>(null);
  const [dragPreview, setDragPreview] =
    useState<QueuePlacementPreview | null>(null);
  const queuesRef = useRef(queues);
  const summaryRef = useRef(summary);
  const busyEpisodesRef = useRef(new Set<number>());
  const dragSnapshotRef = useRef<QueueViewStateMap | null>(null);
  const queueRequestVersion = useRef<Record<ConsumptionQueue, number>>({
    inbox: 0,
    focus: 0,
    someday: 0,
    done: 0,
  });
  const summaryRequestVersion = useRef(0);
  const detailTriggerRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    queuesRef.current = queues;
  }, [queues]);

  useEffect(() => {
    summaryRef.current = summary;
  }, [summary]);

  useEffect(() => {
    if (completionUndos.length === 0) return;
    const pruneExpired = () => {
      const now = Date.now();
      setUndoNow(now);
      setCompletionUndos((current) =>
        current.filter((notice) => notice.expiresAt > now),
      );
    };
    pruneExpired();
    const timer = window.setInterval(pruneExpired, 250);
    return () => window.clearInterval(timer);
  }, [completionUndos.length]);

  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const media = window.matchMedia(
      "(min-width: 900px) and (orientation: landscape)",
    );
    const updateDragEnabled = () => setDragEnabled(media.matches);
    updateDragEnabled();
    if (media.addEventListener) {
      media.addEventListener("change", updateDragEnabled);
      return () => media.removeEventListener("change", updateDragEnabled);
    }
    media.addListener(updateDragEnabled);
    return () => media.removeListener(updateDragEnabled);
  }, []);

  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 250, tolerance: 5 },
    }),
  );

  const loadQueue = useCallback(async (queue: ConsumptionQueue) => {
    const requestVersion = ++queueRequestVersion.current[queue];
    setQueues((previous) => ({
      ...previous,
      [queue]: {
        ...previous[queue],
        isLoading: true,
        error: null,
      },
    }));
    try {
      const payload = await consumptionApi.listQueue(queue);
      if (requestVersion !== queueRequestVersion.current[queue]) return;
      const currentRevision = queuesRef.current[queue].revision;
      if (currentRevision !== null && payload.revision < currentRevision) return;
      setQueues((previous) => ({
        ...previous,
        [queue]: {
          items: payload.items,
          revision: payload.revision,
          hasMore: payload.has_more,
          isLoading: false,
          error: null,
        },
      }));
    } catch (error) {
      if (requestVersion !== queueRequestVersion.current[queue]) return;
      setQueues((previous) => ({
        ...previous,
        [queue]: {
          ...previous[queue],
          isLoading: false,
          error: getConsumptionErrorDetails(error).message,
        },
      }));
    }
  }, []);

  const loadSummary = useCallback(async () => {
    const requestVersion = ++summaryRequestVersion.current;
    setSummaryError(null);
    try {
      const nextSummary = await consumptionApi.getSummary();
      if (requestVersion !== summaryRequestVersion.current) return;
      setSummary(nextSummary);
    } catch (error) {
      if (requestVersion !== summaryRequestVersion.current) return;
      setSummaryError(getConsumptionErrorDetails(error).message);
    }
  }, []);

  useEffect(() => {
    void loadSummary();
    for (const queue of CONSUMPTION_QUEUES) {
      void loadQueue(queue);
    }
  }, [loadQueue, loadSummary]);

  const reconcileItem = useCallback((item: ConsumptionItem) => {
    setQueues((previous) => updateItemAcrossQueues(previous, item));
    setDetailItem((previous) =>
      previous?.episode_id === item.episode_id ? item : previous,
    );
  }, []);

  const registerCompletionUndo = useCallback(
    (
      item: Pick<ConsumptionItem, "episode_id" | "episode_title">,
      undo?: CompletionUndo,
    ) => {
      if (!undo) return;
      const expiresAt = new Date(undo.expires_at).getTime();
      if (!Number.isFinite(expiresAt) || expiresAt <= Date.now()) return;
      setCompletionUndoError(null);
      setUndoNow(Date.now());
      setCompletionUndos((current) => [
        ...current.filter((notice) => notice.episodeId !== item.episode_id),
        {
          episodeId: item.episode_id,
          episodeTitle: item.episode_title,
          token: undo.token,
          expiresAt,
        },
      ]);
    },
    [],
  );

  const performMove = useCallback(
    async (
      item: ConsumptionItem,
      target: ConsumptionQueue,
      acknowledgeFocusLimit = false,
    ): Promise<ConsumptionItem | undefined> => {
      if (busyEpisodesRef.current.has(item.episode_id)) return undefined;

      const source = item.queue_state;
      const currentSummary = summaryRef.current;
      const focusLimit = currentSummary?.focus_limit ?? 7;
      const focusCount =
        currentSummary?.counts.focus ?? queuesRef.current.focus.items.length;

      if (
        target === "focus" &&
        source !== "focus" &&
        !acknowledgeFocusLimit &&
        focusCount >= focusLimit
      ) {
        setFocusPrompt({
          item,
          currentCount: focusCount,
          limit: focusLimit,
        });
        return undefined;
      }

      busyEpisodesRef.current.add(item.episode_id);
      setBusyEpisodes(new Set(busyEpisodesRef.current));
      setFailedAction(null);

      const optimistic: ConsumptionItem = {
        ...item,
        queue_state: target,
        queue_updated_at: new Date().toISOString(),
        completed_at:
          target === "done" ? new Date().toISOString() : item.completed_at,
        in_progress_at: target === "done" ? undefined : item.in_progress_at,
        attention: "",
      };
      setQueues((previous) => updateItemAcrossQueues(previous, optimistic));
      setDetailItem((previous) =>
        previous?.episode_id === item.episode_id ? optimistic : previous,
      );
      setSummary((previous) => adjustSummary(previous, source, target, 1));

      try {
        const canonical = await consumptionApi.setQueue(
          item.episode_id,
          target,
          { acknowledgeFocusLimit },
        );
        const canonicalItem = { ...canonical, completion_undo: undefined };
        reconcileItem(canonicalItem);
        if (target === "done") {
          registerCompletionUndo(canonical, canonical.completion_undo);
        }
        setAnnouncement(
          `${item.episode_title} ${target === "done" ? "已完成" : `已移至 ${target}`}。`,
        );
        void loadSummary();
        const queuesToReload = new Set<ConsumptionQueue>([target]);
        if (source) queuesToReload.add(source);
        void Promise.allSettled(
          Array.from(queuesToReload).map((queue) => loadQueue(queue)),
        );
        return canonicalItem;
      } catch (error) {
        setQueues((previous) => updateItemAcrossQueues(previous, item));
        setDetailItem((previous) =>
          previous?.episode_id === item.episode_id ? item : previous,
        );
        setSummary((previous) => adjustSummary(previous, source, target, -1));

        const details = getConsumptionErrorDetails(error);
        if (target === "focus" && requiresFocusConfirmation(error)) {
          setFocusPrompt({
            item,
            currentCount: details.currentCount ?? focusCount,
            limit: details.focusLimit ?? focusLimit,
          });
        } else {
          setFailedAction({
            item,
            target,
            acknowledgeFocusLimit,
            message: details.message,
          });
        }

        const queuesToReload = new Set<ConsumptionQueue>([target]);
        if (source) queuesToReload.add(source);
        void Promise.allSettled([
          ...Array.from(queuesToReload).map((queue) => loadQueue(queue)),
          loadSummary(),
        ]);
        return undefined;
      } finally {
        busyEpisodesRef.current.delete(item.episode_id);
        setBusyEpisodes(new Set(busyEpisodesRef.current));
      }
    },
    [loadQueue, loadSummary, reconcileItem, registerCompletionUndo],
  );

  const requestMove = useCallback(
    (item: ConsumptionItem, target: ConsumptionQueue) =>
      performMove(item, target),
    [performMove],
  );

  const performPlacement = useCallback(
    async (
      item: ConsumptionItem,
      source: ConsumptionQueue | null,
      placement: QueuePlacementPreview,
      rollback: QueueViewStateMap,
      expectedRevisions: Partial<Record<ConsumptionQueue, number>>,
      acknowledgeFocusLimit = false,
    ) => {
      if (busyEpisodesRef.current.has(item.episode_id)) return;
      const target = placement.queue;
      if (!hasExpectedRevisions(source, target, expectedRevisions)) {
        setFailedAction({
          item,
          target,
          acknowledgeFocusLimit,
          placement,
          message: "队列正在刷新，请重新拖放。",
        });
        return;
      }

      const currentSummary = summaryRef.current;
      const focusLimit = currentSummary?.focus_limit ?? 7;
      const focusCount =
        currentSummary?.counts.focus ?? queuesRef.current.focus.items.length;
      if (
        target === "focus" &&
        source !== "focus" &&
        !acknowledgeFocusLimit &&
        focusCount >= focusLimit
      ) {
        setFocusPrompt({ item, currentCount: focusCount, limit: focusLimit, placement });
        return;
      }

      busyEpisodesRef.current.add(item.episode_id);
      setBusyEpisodes(new Set(busyEpisodesRef.current));
      setFailedAction(null);

      const optimistic = previewQueuePlacement(rollback, item, source, placement);
      queuesRef.current = optimistic;
      setQueues(optimistic);
      const optimisticItem = optimistic[target].items.find(
        (candidate) => candidate.episode_id === item.episode_id,
      );
      if (optimisticItem) {
        setDetailItem((previous) =>
          previous?.episode_id === item.episode_id ? optimisticItem : previous,
        );
      }
      setSummary((previous) => adjustSummary(previous, source, target, 1));

      try {
        const result = await consumptionApi.placeQueue(item.episode_id, {
          queue_state: target,
          before_episode_id: placement.beforeEpisodeId,
          expected_revisions: expectedRevisions,
          acknowledge_focus_limit: acknowledgeFocusLimit,
        });
        const canonicalQueues = replaceQueueSnapshots(
          queuesRef.current,
          result.queues,
        );
        queuesRef.current = canonicalQueues;
        setQueues(canonicalQueues);
        let canonicalItem: ConsumptionItem | undefined;
        for (const snapshot of Object.values(result.queues)) {
          canonicalItem = snapshot?.items.find(
            (candidate) => candidate.episode_id === item.episode_id,
          );
          if (canonicalItem) break;
        }
        if (canonicalItem) {
          setDetailItem((previous) =>
            previous?.episode_id === item.episode_id ? canonicalItem : previous,
          );
        }
        if (target === "done") {
          registerCompletionUndo(item, result.completion_undo);
        }
        setAnnouncement(
          `${item.episode_title} ${
            target === "done"
              ? "已完成"
              : source === target
                ? "已调整顺序"
                : `已移至 ${target}`
          }。`,
        );
        void loadSummary();
      } catch (error) {
        const restored = cloneQueues(rollback);
        queuesRef.current = restored;
        setQueues(restored);
        const restoredItem = (source ? restored[source] : restored[target]).items.find(
          (candidate) => candidate.episode_id === item.episode_id,
        );
        if (restoredItem) {
          setDetailItem((previous) =>
            previous?.episode_id === item.episode_id ? restoredItem : previous,
          );
        }
        setSummary((previous) => adjustSummary(previous, source, target, -1));

        const details = getConsumptionErrorDetails(error);
        const queueOrderConflict = isQueueOrderConflict(error);
        if (target === "focus" && requiresFocusConfirmation(error)) {
          setFocusPrompt({
            item,
            currentCount: details.currentCount ?? focusCount,
            limit: details.focusLimit ?? focusLimit,
            placement,
          });
        } else if (queueOrderConflict) {
          setFailedAction({
            item,
            target,
            acknowledgeFocusLimit,
            placement,
            isQueueOrderConflict: true,
            message: "队列顺序已在另一设备修改，请重新拖放。",
          });
        } else {
          setFailedAction({
            item,
            target,
            acknowledgeFocusLimit,
            placement,
            message: details.message,
          });
        }

        const queuesToRefresh = queueOrderConflict
          ? CONSUMPTION_QUEUES
          : affectedQueues(source, target);
        void Promise.allSettled([
          ...queuesToRefresh.map((queue) => loadQueue(queue)),
          loadSummary(),
        ]);
      } finally {
        busyEpisodesRef.current.delete(item.episode_id);
        setBusyEpisodes(new Set(busyEpisodesRef.current));
      }
    },
    [loadQueue, loadSummary, registerCompletionUndo],
  );

  const retryPlacement = useCallback(
    (action: FailedAction) => {
      if (!action.placement) return;
      const rollback = cloneQueues(queuesRef.current);
      let currentItem: ConsumptionItem | undefined;
      for (const queue of CONSUMPTION_QUEUES) {
        currentItem = rollback[queue].items.find(
          (candidate) => candidate.episode_id === action.item.episode_id,
        );
        if (currentItem) break;
      }
      const item = currentItem ?? action.item;
      const source = item.queue_state;
      const expectedRevisions = expectedRevisionsForPlacement(
        source,
        action.placement.queue,
        rollback,
      );
      void performPlacement(
        item,
        source,
        action.placement,
        rollback,
        expectedRevisions,
        action.acknowledgeFocusLimit,
      );
    },
    [performPlacement],
  );

  const confirmFocusPlacement = useCallback(
    async (prompt: FocusPrompt) => {
      if (!prompt.placement) {
        await performMove(prompt.item, "focus", true);
        return;
      }

      const target = prompt.placement.queue;
      const source = prompt.item.queue_state;
      try {
        const latestItem = await consumptionApi.getItem(prompt.item.episode_id);
        const queueStates = queuesForRefresh(
          source,
          latestItem.queue_state,
          target,
        );
        for (const queue of queueStates) {
          queueRequestVersion.current[queue] += 1;
        }
        const payloads = await Promise.all(
          queueStates.map(async (queue) => [
            queue,
            await consumptionApi.listQueue(queue),
          ] as const),
        );
        const snapshots: Partial<
          Record<
            ConsumptionQueue,
            { revision: number; items: ConsumptionItem[]; has_more: boolean }
          >
        > = {};
        for (const [queue, payload] of payloads) snapshots[queue] = payload;
        const freshQueues = replaceQueueSnapshots(queuesRef.current, snapshots);
        queuesRef.current = freshQueues;
        setQueues(freshQueues);

        if (!source || latestItem.queue_state !== source) {
          setFailedAction({
            item: prompt.item,
            target,
            acknowledgeFocusLimit: true,
            placement: prompt.placement,
            isQueueOrderConflict: true,
            message: "队列顺序已在另一设备修改，请重新拖放。",
          });
          void Promise.allSettled([
            ...CONSUMPTION_QUEUES.map((queue) => loadQueue(queue)),
            loadSummary(),
          ]);
          return;
        }

        const listedItem = freshQueues[source].items.find(
          (candidate) => candidate.episode_id === latestItem.episode_id,
        );
        if (!listedItem) {
          setFailedAction({
            item: prompt.item,
            target,
            acknowledgeFocusLimit: true,
            placement: prompt.placement,
            isQueueOrderConflict: true,
            message: "队列顺序已在另一设备修改，请重新拖放。",
          });
          void Promise.allSettled([
            ...CONSUMPTION_QUEUES.map((queue) => loadQueue(queue)),
            loadSummary(),
          ]);
          return;
        }

        await performPlacement(
          listedItem,
          source,
          prompt.placement,
          freshQueues,
          expectedRevisionsForPlacement(source, target, freshQueues),
          true,
        );
      } catch (error) {
        const details = getConsumptionErrorDetails(error);
        setFailedAction({
          item: prompt.item,
          target,
          acknowledgeFocusLimit: true,
          placement: prompt.placement,
          message: details.message,
        });
        void Promise.allSettled([
          ...affectedQueues(prompt.item.queue_state, target).map((queue) =>
            loadQueue(queue),
          ),
          loadSummary(),
        ]);
      }
    },
    [loadQueue, loadSummary, performMove, performPlacement],
  );

  const undoCompletion = useCallback(
    async (notice: CompletionUndoNotice) => {
      if (
        notice.expiresAt <= Date.now() ||
        busyEpisodesRef.current.has(notice.episodeId)
      ) {
        return;
      }
      busyEpisodesRef.current.add(notice.episodeId);
      setBusyEpisodes(new Set(busyEpisodesRef.current));
      setCompletionUndoError(null);
      try {
        const result = await consumptionApi.undoCompletion(
          notice.episodeId,
          notice.token,
        );
        const canonicalQueues = replaceQueueSnapshots(
          queuesRef.current,
          result.queues,
        );
        queuesRef.current = canonicalQueues;
        setQueues(canonicalQueues);
        setCompletionUndos((current) =>
          current.filter((candidate) => candidate.token !== notice.token),
        );
        setAnnouncement(`${notice.episodeTitle} 已恢复到完成前的位置。`);
        void loadSummary();
        void consumptionApi
          .getItem(notice.episodeId)
          .then(reconcileItem)
          .catch(() => undefined);
      } catch (error) {
        setCompletionUndos((current) =>
          current.filter((candidate) => candidate.token !== notice.token),
        );
        if (isCompletionUndoConflict(error)) {
          setCompletionUndoError(
            "状态已在另一设备改变，无法撤销；已刷新，请从最近完成重新处理。",
          );
          void Promise.allSettled([
            ...CONSUMPTION_QUEUES.map((queue) => loadQueue(queue)),
            loadSummary(),
          ]);
        } else if (isCompletionUndoExpired(error)) {
          setCompletionUndoError(
            "15 秒撤销窗口已结束；可从最近完成重新处理。",
          );
        } else {
          setCompletionUndoError(
            `撤销失败：${getConsumptionErrorDetails(error).message}`,
          );
        }
      } finally {
        busyEpisodesRef.current.delete(notice.episodeId);
        setBusyEpisodes(new Set(busyEpisodesRef.current));
      }
    },
    [loadQueue, loadSummary, reconcileItem],
  );

  const resolvePlacementFromDragEvent = useCallback(
    (
      event: DragMoveEvent | DragOverEvent | DragEndEvent,
      view: QueueViewStateMap,
    ) => {
      if (!event.over) return null;
      const activeData = readQueueDragData(event.active.data.current);
      const overData = readQueueDragData(event.over.data.current);
      if (activeData?.kind !== "item" || !overData) return null;

      const overEpisodeId =
        overData.kind === "item" ? overData.episodeId ?? null : null;
      const translated = event.active.rect.current.translated;
      const initial = event.active.rect.current.initial;
      const activeCenter = translated
        ? translated.top + translated.height / 2
        : initial
          ? initial.top + initial.height / 2 + event.delta.y
          : event.over.rect.top;
      return resolveQueuePlacement({
        sourceQueue: activeData.queue,
        targetQueue: overData.queue,
        activeEpisodeId: activeData.episodeId,
        targetItems: view[overData.queue].items,
        overEpisodeId,
        placeAfter:
          overEpisodeId !== null &&
          activeCenter > event.over.rect.top + event.over.rect.height / 2,
      });
    },
    [],
  );

  const handleDragStart = useCallback((event: DragStartEvent) => {
    const activeData = readQueueDragData(event.active.data.current);
    if (activeData?.kind !== "item") return;
    if (busyEpisodesRef.current.has(activeData.episodeId)) return;
    const snapshot = cloneQueues(queuesRef.current);
    const item = snapshot[activeData.queue].items.find(
      (candidate) => candidate.episode_id === activeData.episodeId,
    );
    if (!item) return;
    dragSnapshotRef.current = snapshot;
    setFailedAction(null);
    setActiveDrag({ item, source: activeData.queue });
    setDragPreview(null);
  }, []);

  const handleDragMove = useCallback(
    (event: DragMoveEvent | DragOverEvent) => {
      const next = resolvePlacementFromDragEvent(event, queuesRef.current);
      setDragPreview((current) =>
        isSameQueuePlacement(current, next) ? current : next,
      );
    },
    [resolvePlacementFromDragEvent],
  );

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const snapshot = dragSnapshotRef.current;
      const activeData = readQueueDragData(event.active.data.current);
      const placement = snapshot
        ? resolvePlacementFromDragEvent(event, snapshot)
        : null;
      dragSnapshotRef.current = null;
      setActiveDrag(null);
      setDragPreview(null);
      if (activeData?.kind !== "item" || !snapshot || !placement) return;

      const sourceItems = snapshot[activeData.queue].items;
      const item = sourceItems.find(
        (candidate) => candidate.episode_id === activeData.episodeId,
      );
      if (!item) return;
      if (
        isNoOpQueuePlacement(
          activeData.queue,
          placement.queue,
          sourceItems,
          item.episode_id,
          placement.beforeEpisodeId,
        )
      ) {
        return;
      }
      void performPlacement(
        item,
        activeData.queue,
        placement,
        snapshot,
        expectedRevisionsForPlacement(
          activeData.queue,
          placement.queue,
          snapshot,
        ),
      );
    },
    [performPlacement, resolvePlacementFromDragEvent],
  );

  const handleDragCancel = useCallback((_event: DragCancelEvent) => {
    dragSnapshotRef.current = null;
    setActiveDrag(null);
    setDragPreview(null);
  }, []);

  const openDetail = (item: ConsumptionItem, trigger: HTMLButtonElement) => {
    detailTriggerRef.current = trigger;
    setDetailItem(item);
  };

  const closeDetail = () => {
    const episodeId = detailItem?.episode_id;
    const originalTrigger = detailTriggerRef.current;
    setDetailItem(null);
    window.requestAnimationFrame(() => {
      if (originalTrigger?.isConnected) {
        originalTrigger.focus();
        return;
      }
      if (episodeId) {
        document
          .querySelector<HTMLButtonElement>(
            `[data-episode-id="${episodeId}"] button`,
          )
          ?.focus();
      }
    });
  };

  const focusLimit = summary?.focus_limit ?? 7;
  const focusCount = summary?.counts.focus ?? queues.focus.items.length;
  const isFocusOverLimit = summary?.focus_over_limit ?? focusCount > focusLimit;
  const board = (
    <div className={styles.board}>
      {CONSUMPTION_QUEUES.map((queue) => (
        <ConsumptionQueueColumn
          key={queue}
          queue={queue}
          items={queues[queue].items}
          count={
            queue === "done"
              ? queues.done.items.length
              : (summary?.counts[queue] ?? queues[queue].items.length)
          }
          hasMore={queues[queue].hasMore}
          isLoading={queues[queue].isLoading}
          error={queues[queue].error}
          focusLimit={focusLimit}
          isFocusOverLimit={isFocusOverLimit}
          busyEpisodes={busyEpisodes}
          onRetry={() => void loadQueue(queue)}
          onOpen={openDetail}
          onMove={requestMove}
          dragEnabled={dragEnabled}
          dragPreview={dragPreview}
        />
      ))}
    </div>
  );

  return (
    <PageLayout
      toolbar={false}
      maxWidth={false}
      rootClassName={styles.shell}
      className={styles.layout}
    >
      <main className={styles.page}>
        <h1 className={styles.srOnly}>Inbox</h1>

        {completionUndos.length > 0 && (
          <div className={styles.undoStack} aria-label="可撤销的完成操作">
            {completionUndos.map((notice) => {
              const secondsLeft = Math.max(
                1,
                Math.ceil((notice.expiresAt - undoNow) / 1000),
              );
              return (
                <div
                  key={notice.token}
                  className={styles.undoNotice}
                  role="status"
                >
                  <IconCircleCheck size={18} stroke={1.8} aria-hidden="true" />
                  <span>
                    《{notice.episodeTitle}》已完成，{secondsLeft} 秒内可撤销。
                  </span>
                  <button
                    type="button"
                    className={styles.undoButton}
                    disabled={busyEpisodes.has(notice.episodeId)}
                    onClick={() => void undoCompletion(notice)}
                    aria-label={`撤销完成 ${notice.episodeTitle}`}
                  >
                    <IconArrowBackUp size={17} stroke={1.8} aria-hidden="true" />
                    撤销
                  </button>
                </div>
              );
            })}
          </div>
        )}

        {completionUndoError && (
          <div className={styles.actionError} role="alert">
            <IconAlertTriangle size={18} stroke={1.8} aria-hidden="true" />
            <span>{completionUndoError}</span>
          </div>
        )}

        {summaryError && (
          <div className={styles.summaryError} role="alert">
            <IconAlertTriangle size={18} stroke={1.8} aria-hidden="true" />
            <span>总数暂未同步；各队列仍可独立使用。</span>
            <button
              type="button"
              className={styles.iconButton}
              onClick={() => void loadSummary()}
              aria-label="重试加载队列总数"
              title="重试"
            >
              <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
            </button>
          </div>
        )}

        {failedAction && (
          <div className={styles.actionError} role="alert">
            <IconAlertTriangle size={18} stroke={1.8} aria-hidden="true" />
            <span>
              {failedAction.isQueueOrderConflict
                ? failedAction.message
                : failedAction.placement &&
                    failedAction.item.queue_state === failedAction.target
                  ? `《${failedAction.item.episode_title}》调整失败，已恢复原顺序：${failedAction.message}`
                  : `《${failedAction.item.episode_title}》移动失败，已恢复原队列：${failedAction.message}`}
            </span>
            {!failedAction.isQueueOrderConflict && (
              <button
                type="button"
                className={styles.iconButton}
                onClick={() => {
                  const action = failedAction;
                  setFailedAction(null);
                  if (action.placement) {
                    retryPlacement(action);
                    return;
                  }
                  void performMove(
                    action.item,
                    action.target,
                    action.acknowledgeFocusLimit,
                  );
                }}
                aria-label={`重试移动 ${failedAction.item.episode_title}`}
                title="重试移动"
              >
                <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
              </button>
            )}
          </div>
        )}

        <section
          className={styles.boardViewport}
          aria-label="消费队列横向总览"
          tabIndex={0}
        >
          {dragEnabled ? (
            <DndContext
              sensors={sensors}
              collisionDetection={queueCollisionDetection}
              onDragStart={handleDragStart}
              onDragMove={handleDragMove}
              onDragOver={handleDragMove}
              onDragEnd={handleDragEnd}
              onDragCancel={handleDragCancel}
            >
              {board}
              <DragOverlay>
                {activeDrag && (
                  <div className={styles.dragOverlay} aria-hidden="true">
                    <span>{activeDrag.item.podcast_title}</span>
                    <strong>{activeDrag.item.episode_title}</strong>
                  </div>
                )}
              </DragOverlay>
            </DndContext>
          ) : (
            board
          )}
        </section>

        <footer className={styles.pageFooter}>
          <p>
            <span>
              <IconArrowRight size={14} stroke={1.8} aria-hidden="true" />
              打开原节目只记录“进行中”
            </span>
            <span className={styles.pageFooterDivider} aria-hidden="true">
              ·
            </span>
            <span>
              <IconCircleCheck size={14} stroke={1.8} aria-hidden="true" />
              最近完成只来自手动确认
            </span>
          </p>
        </footer>

        <p className={styles.srOnly} aria-live="polite">
          {announcement}
        </p>
      </main>

      {detailItem && (
        <ConsumptionDetailPanel
          item={detailItem}
          isQueueBusy={busyEpisodes.has(detailItem.episode_id)}
          onClose={closeDetail}
          onItemChange={reconcileItem}
          onMove={requestMove}
        />
      )}

      {focusPrompt && (
        <FocusLimitDialog
          item={focusPrompt.item}
          currentCount={focusPrompt.currentCount}
          limit={focusPrompt.limit}
          isSaving={busyEpisodes.has(focusPrompt.item.episode_id)}
          onCancel={() => setFocusPrompt(null)}
          onConfirm={() => {
            const prompt = focusPrompt;
            setFocusPrompt(null);
            void confirmFocusPlacement(prompt);
          }}
        />
      )}
    </PageLayout>
  );
}
