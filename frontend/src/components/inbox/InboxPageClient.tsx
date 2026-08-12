"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  IconAlertTriangle,
  IconArrowRight,
  IconCircleCheck,
  IconRefresh,
} from "@tabler/icons-react";
import PageLayout from "@/components/layout/PageLayout";
import {
  consumptionApi,
  getConsumptionErrorDetails,
  requiresFocusConfirmation,
} from "@/lib/api/consumption";
import {
  CONSUMPTION_QUEUES,
  type ConsumptionItem,
  type ConsumptionQueue,
  type ConsumptionSummary,
} from "@/types/consumption";
import ConsumptionDetailPanel from "./ConsumptionDetailPanel";
import ConsumptionQueueColumn from "./ConsumptionQueueColumn";
import FocusLimitDialog from "./FocusLimitDialog";
import { sortQueueItems } from "./presentation";
import styles from "./InboxPage.module.css";

interface QueueViewState {
  items: ConsumptionItem[];
  isLoading: boolean;
  error: string | null;
}

type QueueViewStateMap = Record<ConsumptionQueue, QueueViewState>;

interface FocusPrompt {
  item: ConsumptionItem;
  currentCount: number;
  limit: number;
}

interface FailedAction {
  item: ConsumptionItem;
  target: ConsumptionQueue;
  acknowledgeFocusLimit: boolean;
  message: string;
}

function makeInitialQueues(): QueueViewStateMap {
  return {
    inbox: { items: [], isLoading: true, error: null },
    focus: { items: [], isLoading: true, error: null },
    someday: { items: [], isLoading: true, error: null },
    done: { items: [], isLoading: true, error: null },
  };
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
    next[item.queue_state] = {
      ...next[item.queue_state],
      items: sortQueueItems([item, ...next[item.queue_state].items]),
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
  const [announcement, setAnnouncement] = useState("");
  const queuesRef = useRef(queues);
  const summaryRef = useRef(summary);
  const busyEpisodesRef = useRef(new Set<number>());
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
      setQueues((previous) => ({
        ...previous,
        [queue]: {
          items: sortQueueItems(payload.items),
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
        reconcileItem(canonical);
        setAnnouncement(
          `${item.episode_title} 已移至 ${target === "done" ? "Done" : target}.`,
        );
        void loadSummary();
        return canonical;
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
    [loadQueue, loadSummary, reconcileItem],
  );

  const requestMove = useCallback(
    (item: ConsumptionItem, target: ConsumptionQueue) =>
      performMove(item, target),
    [performMove],
  );

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

  return (
    <PageLayout
      toolbar={false}
      maxWidth={false}
      rootClassName={styles.shell}
      className={styles.layout}
    >
      <main className={styles.page}>
        <h1 className={styles.srOnly}>Inbox</h1>

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
              《{failedAction.item.episode_title}》移动失败，已恢复原队列：
              {failedAction.message}
            </span>
            <button
              type="button"
              className={styles.iconButton}
              onClick={() => {
                const action = failedAction;
                setFailedAction(null);
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
          </div>
        )}

        <section
          className={styles.boardViewport}
          aria-label="消费队列横向总览"
          tabIndex={0}
        >
          <div className={styles.board}>
            {CONSUMPTION_QUEUES.map((queue) => (
              <ConsumptionQueueColumn
                key={queue}
                queue={queue}
                items={queues[queue].items}
                count={summary?.counts[queue] ?? queues[queue].items.length}
                isLoading={queues[queue].isLoading}
                error={queues[queue].error}
                focusLimit={focusLimit}
                isFocusOverLimit={isFocusOverLimit}
                busyEpisodes={busyEpisodes}
                onRetry={() => void loadQueue(queue)}
                onOpen={openDetail}
                onMove={requestMove}
              />
            ))}
          </div>
        </section>

        <footer className={styles.pageFooter}>
          <span>
            <IconArrowRight size={16} stroke={1.8} aria-hidden="true" />
            打开原节目只记录“进行中”
          </span>
          <span>
            <IconCircleCheck size={16} stroke={1.8} aria-hidden="true" />
            Done 必须手动确认
          </span>
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
            void performMove(prompt.item, "focus", true);
          }}
        />
      )}
    </PageLayout>
  );
}
