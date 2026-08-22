"use client";

import { useEffect, useRef, useState } from "react";
import {
  IconAlertTriangle,
  IconArrowRight,
  IconArrowsExchange,
  IconBookmarkPlus,
  IconCircleCheck,
  IconClock,
  IconRefresh,
  IconTargetArrow,
} from "@tabler/icons-react";
import PlainImage from "@/components/ui/PlainImage";
import { getOptimizedImageUrl } from "@/lib/imageOptimization";
import {
  CONSUMPTION_QUEUES,
  type ConsumptionItem,
  type ConsumptionQueue,
} from "@/types/consumption";
import {
  formatActivityDate,
  formatDuration,
  formatPublishedDate,
  QUEUE_PRESENTATION,
} from "./presentation";
import styles from "./InboxPage.module.css";

interface ConsumptionQueueColumnProps {
  queue: ConsumptionQueue;
  items: ConsumptionItem[];
  count: number;
  isLoading: boolean;
  error: string | null;
  focusLimit: number;
  isFocusOverLimit: boolean;
  busyEpisodes: Set<number>;
  onRetry: () => void;
  onOpen: (item: ConsumptionItem, trigger: HTMLButtonElement) => void;
  onMove: (
    item: ConsumptionItem,
    target: ConsumptionQueue,
  ) => Promise<ConsumptionItem | undefined>;
}

function QueueIcon({
  queue,
  size = 18,
}: {
  queue: ConsumptionQueue;
  size?: number;
}) {
  const common = { size, stroke: 1.8, "aria-hidden": true } as const;
  switch (queue) {
    case "inbox":
      return <IconBookmarkPlus {...common} />;
    case "focus":
      return <IconTargetArrow {...common} />;
    case "someday":
      return <IconClock {...common} />;
    case "done":
      return <IconCircleCheck {...common} />;
  }
}

function ConsumptionCard({
  item,
  busy,
  onOpen,
  onMove,
}: {
  item: ConsumptionItem;
  busy: boolean;
  onOpen: (item: ConsumptionItem, trigger: HTMLButtonElement) => void;
  onMove: (
    item: ConsumptionItem,
    target: ConsumptionQueue,
  ) => Promise<ConsumptionItem | undefined>;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const currentQueue = item.queue_state;
  const coverSource = getOptimizedImageUrl(
    item.image_url || item.podcast_cover_url,
    96,
  );

  useEffect(() => {
    if (!menuOpen) return;
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    };
    document.addEventListener("mousedown", closeOnOutsideClick);
    return () => document.removeEventListener("mousedown", closeOnOutsideClick);
  }, [menuOpen]);

  const handleMove = async (target: ConsumptionQueue) => {
    setMenuOpen(false);
    await onMove(item, target);
  };

  return (
    <article
      className={`${styles.card}${menuOpen ? ` ${styles.cardMenuOpen}` : ""}`}
      aria-busy={busy}
      data-episode-id={item.episode_id}
    >
      <button
        type="button"
        className={styles.cardMain}
        onClick={(event) => onOpen(item, event.currentTarget)}
        aria-label={`打开 ${item.episode_title} 明细`}
      >
        <span className={styles.coverFrame} aria-hidden="true">
          {coverSource ? (
            <PlainImage
              src={coverSource}
              alt=""
              width={54}
              height={54}
              loading="lazy"
              decoding="async"
              className={styles.cover}
            />
          ) : (
            <span className={styles.coverFallback}>
              {item.podcast_title.trim().slice(0, 1) || "M"}
            </span>
          )}
        </span>
        <span className={styles.cardCopy}>
          <span className={styles.podcastLine}>
            {item.podcast_title}
            {item.episode_no ? ` · ${item.episode_no}` : ""}
          </span>
          <span className={styles.episodeTitle}>{item.episode_title}</span>
          <span className={styles.cardMeta}>
            {formatDuration(item.duration)}
            <span aria-hidden="true">·</span>
            {formatPublishedDate(item.published_date)}
          </span>
        </span>
      </button>

      <div className={styles.cardFooter}>
        <div className={styles.cardSignals}>
          {item.in_progress_at && currentQueue !== "done" && (
            <span className={styles.progressSignal}>
              <IconArrowRight size={14} stroke={1.8} aria-hidden="true" />
              进行中
            </span>
          )}
          {item.attention === "stale" && (
            <span className={styles.staleSignal}>
              <IconClock size={14} stroke={1.8} aria-hidden="true" />
              已放置 7 天
            </span>
          )}
          {item.attention === "review" && (
            <span className={styles.reviewSignal}>
              <IconAlertTriangle size={14} stroke={1.8} aria-hidden="true" />
              需要复盘
            </span>
          )}
          {!item.in_progress_at && !item.attention && item.activity_at && (
            <span className={styles.activitySignal}>
              更新于 {formatActivityDate(item.activity_at)}
            </span>
          )}
        </div>

        <div className={styles.cardActions}>
          {currentQueue !== "done" && (
            <button
              type="button"
              className={styles.iconButton}
              disabled={busy}
              onClick={() => void handleMove("done")}
              aria-label={`将 ${item.episode_title} 标记 Done`}
              title="标记 Done"
            >
              <IconCircleCheck size={18} stroke={1.8} aria-hidden="true" />
            </button>
          )}
          <div className={styles.moveMenu} ref={menuRef}>
            <button
              type="button"
              className={styles.iconButton}
              disabled={busy}
              onClick={() => setMenuOpen((open) => !open)}
              aria-label={`将 ${item.episode_title} 移动到其他队列`}
              aria-haspopup="menu"
              aria-expanded={menuOpen}
              title="移动到其他队列"
            >
              <IconArrowsExchange size={18} stroke={1.8} aria-hidden="true" />
            </button>
            {menuOpen && (
              <div
                className={styles.moveMenuPopover}
                role="menu"
                aria-label={`移动 ${item.episode_title}`}
              >
                {CONSUMPTION_QUEUES.filter(
                  (queue) => queue !== currentQueue,
                ).map((queue) => (
                  <button
                    key={queue}
                    type="button"
                    role="menuitem"
                    onClick={() => void handleMove(queue)}
                  >
                    <QueueIcon queue={queue} size={16} />
                    移至 {QUEUE_PRESENTATION[queue].label}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
      {busy && (
        <span className={styles.cardBusy} role="status">
          正在保存队列状态…
        </span>
      )}
    </article>
  );
}

export default function ConsumptionQueueColumn({
  queue,
  items,
  count,
  isLoading,
  error,
  focusLimit,
  isFocusOverLimit,
  busyEpisodes,
  onRetry,
  onOpen,
  onMove,
}: ConsumptionQueueColumnProps) {
  const presentation = QUEUE_PRESENTATION[queue];

  return (
    <section
      className={`${styles.queueColumn} ${styles[`queue_${queue}`]}`}
      aria-labelledby={`consumption-queue-${queue}`}
      data-queue={queue}
    >
      <header className={styles.queueHeader}>
        <div className={styles.queueHeading}>
          <span className={styles.queueIcon}>
            <QueueIcon queue={queue} />
          </span>
          <h2 id={`consumption-queue-${queue}`}>{presentation.label}</h2>
        </div>
        <span className={styles.queueCount} aria-label={`${count} 项`}>
          {queue === "focus" ? `${count} / ${focusLimit}` : count}
        </span>
      </header>
      <p className={styles.queuePolicy}>{presentation.policy}</p>

      {queue === "focus" && isFocusOverLimit && (
        <div className={styles.focusOverLimit} role="status">
          <IconAlertTriangle size={17} stroke={1.8} aria-hidden="true" />
          <span>
            已超过 {focusLimit} 项。请主动移出低优先内容；系统不会自动处理。
          </span>
        </div>
      )}

      <div className={styles.queueItems}>
        {error && (
          <div className={styles.queueError} role="alert">
            <p>{presentation.label} 加载失败，不影响其他队列。</p>
            <button
              type="button"
              className={styles.iconButton}
              onClick={onRetry}
              aria-label={`重试加载 ${presentation.label}`}
              title="重试"
            >
              <IconRefresh size={18} stroke={1.8} aria-hidden="true" />
            </button>
          </div>
        )}
        {isLoading && items.length === 0 ? (
          <div className={styles.queueLoading} role="status">
            <span />
            <span />
            <span />
            正在加载 {presentation.label}…
          </div>
        ) : items.length === 0 && !error ? (
          <p className={styles.queueEmpty}>{presentation.empty}</p>
        ) : (
          items.map((item) => (
            <ConsumptionCard
              key={item.episode_id}
              item={item}
              busy={busyEpisodes.has(item.episode_id)}
              onOpen={onOpen}
              onMove={onMove}
            />
          ))
        )}
      </div>
    </section>
  );
}
