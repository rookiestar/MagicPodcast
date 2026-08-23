"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { useDroppable } from "@dnd-kit/core";
import {
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import {
  IconAlertTriangle,
  IconArrowRight,
  IconArrowsExchange,
  IconBookmarkPlus,
  IconCircleCheck,
  IconClock,
  IconGripVertical,
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
import {
  episodeDragId,
  queueDropId,
  type QueueDragData,
  type QueuePlacementPreview,
} from "./drag";
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
  dragEnabled: boolean;
  dragPreview: QueuePlacementPreview | null;
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
  cardRef,
  cardStyle,
  isDragging = false,
  dragHandle,
  showInsertBefore = false,
  showInsertAfter = false,
}: {
  item: ConsumptionItem;
  busy: boolean;
  onOpen: (item: ConsumptionItem, trigger: HTMLButtonElement) => void;
  onMove: (
    item: ConsumptionItem,
    target: ConsumptionQueue,
  ) => Promise<ConsumptionItem | undefined>;
  cardRef?: (node: HTMLElement | null) => void;
  cardStyle?: CSSProperties;
  isDragging?: boolean;
  dragHandle?: ReactNode;
  showInsertBefore?: boolean;
  showInsertAfter?: boolean;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const [menuPosition, setMenuPosition] = useState<{
    top: number;
    right: number;
  } | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuTriggerRef = useRef<HTMLButtonElement>(null);
  const currentQueue = item.queue_state;
  const menuId = `consumption-move-menu-${item.episode_id}`;
  const coverSource = getOptimizedImageUrl(
    item.image_url || item.podcast_cover_url,
    96,
  );

  const updateMenuPosition = useCallback(() => {
    const trigger = menuTriggerRef.current;
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    setMenuPosition({
      top: rect.bottom + 5,
      right: Math.max(8, window.innerWidth - rect.right),
    });
  }, []);

  useEffect(() => {
    if (!menuOpen) return;
    menuRef.current
      ?.querySelector<HTMLButtonElement>('[role="menuitem"]')
      ?.focus();
    const closeOnOutsidePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (
        target instanceof Node &&
        (menuRef.current?.contains(target) ||
          menuTriggerRef.current?.contains(target))
      ) {
        return;
      }
      setMenuOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      setMenuOpen(false);
      menuTriggerRef.current?.focus();
    };
    const visualViewport = window.visualViewport;
    document.addEventListener("pointerdown", closeOnOutsidePointerDown, true);
    document.addEventListener("keydown", closeOnEscape);
    window.addEventListener("resize", updateMenuPosition);
    window.addEventListener("scroll", updateMenuPosition, true);
    visualViewport?.addEventListener("resize", updateMenuPosition);
    visualViewport?.addEventListener("scroll", updateMenuPosition);
    return () => {
      document.removeEventListener(
        "pointerdown",
        closeOnOutsidePointerDown,
        true,
      );
      document.removeEventListener("keydown", closeOnEscape);
      window.removeEventListener("resize", updateMenuPosition);
      window.removeEventListener("scroll", updateMenuPosition, true);
      visualViewport?.removeEventListener("resize", updateMenuPosition);
      visualViewport?.removeEventListener("scroll", updateMenuPosition);
    };
  }, [menuOpen, updateMenuPosition]);

  const handleMove = async (target: ConsumptionQueue) => {
    setMenuOpen(false);
    await onMove(item, target);
  };

  const handleMenuKeyDown = (
    event: ReactKeyboardEvent<HTMLDivElement>,
  ) => {
    const items = Array.from(
      menuRef.current?.querySelectorAll<HTMLButtonElement>(
        '[role="menuitem"]:not(:disabled)',
      ) ?? [],
    );
    if (items.length === 0) return;

    const currentIndex = items.indexOf(
      document.activeElement as HTMLButtonElement,
    );
    let nextIndex: number;
    switch (event.key) {
      case "ArrowDown":
        nextIndex = (currentIndex + 1) % items.length;
        break;
      case "ArrowUp":
        nextIndex = (currentIndex - 1 + items.length) % items.length;
        break;
      case "Home":
        nextIndex = 0;
        break;
      case "End":
        nextIndex = items.length - 1;
        break;
      default:
        return;
    }

    event.preventDefault();
    items[nextIndex]?.focus();
  };

  const toggleMenu = () => {
    if (menuOpen) {
      setMenuOpen(false);
      return;
    }
    updateMenuPosition();
    setMenuOpen(true);
  };

  return (
    <article
      ref={cardRef}
      style={cardStyle}
      className={`${styles.card}${isDragging ? ` ${styles.cardDragging}` : ""}${
        showInsertBefore ? ` ${styles.cardInsertBefore}` : ""
      }${
        showInsertAfter ? ` ${styles.cardInsertAfter}` : ""
      }`}
      aria-busy={busy}
      data-episode-id={item.episode_id}
    >
      {dragHandle}
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
          <div>
            <button
              ref={menuTriggerRef}
              type="button"
              className={styles.iconButton}
              disabled={busy}
              onClick={toggleMenu}
              aria-label={`将 ${item.episode_title} 移动到其他队列`}
              aria-haspopup="menu"
              aria-expanded={menuOpen}
              aria-controls={menuOpen ? menuId : undefined}
              title="移动到其他队列"
            >
              <IconArrowsExchange size={18} stroke={1.8} aria-hidden="true" />
            </button>
          </div>
        </div>
      </div>
      {menuOpen && menuPosition && typeof document !== "undefined"
        ? createPortal(
            <div
              ref={menuRef}
              id={menuId}
              className={styles.moveMenuPopover}
              style={menuPosition}
              role="menu"
              aria-label={`移动 ${item.episode_title}`}
              onKeyDown={handleMenuKeyDown}
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
            </div>,
            document.body,
          )
        : null}
      {busy && (
        <span className={styles.cardBusy} role="status">
          正在保存队列状态…
        </span>
      )}
    </article>
  );
}

function SortableConsumptionCard({
  item,
  queue,
  busy,
  onOpen,
  onMove,
  showInsertBefore,
  showInsertAfter,
}: {
  item: ConsumptionItem;
  queue: ConsumptionQueue;
  busy: boolean;
  onOpen: (item: ConsumptionItem, trigger: HTMLButtonElement) => void;
  onMove: (
    item: ConsumptionItem,
    target: ConsumptionQueue,
  ) => Promise<ConsumptionItem | undefined>;
  showInsertBefore: boolean;
  showInsertAfter: boolean;
}) {
  const {
    attributes,
    listeners,
    setActivatorNodeRef,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({
    id: episodeDragId(item.episode_id),
    disabled: busy,
    data: {
      kind: "item",
      queue,
      episodeId: item.episode_id,
    } satisfies QueueDragData,
  });

  const cardStyle: CSSProperties = {
    transform: transform
      ? `translate3d(${transform.x}px, ${transform.y}px, 0)`
      : undefined,
    transition,
  };

  return (
    <ConsumptionCard
      item={item}
      busy={busy}
      onOpen={onOpen}
      onMove={onMove}
      cardRef={setNodeRef}
      cardStyle={cardStyle}
      isDragging={isDragging}
      showInsertBefore={showInsertBefore}
      showInsertAfter={showInsertAfter}
      dragHandle={
        <button
          ref={setActivatorNodeRef}
          type="button"
          className={styles.dragHandle}
          disabled={busy}
          aria-label={`拖动《${item.episode_title}》调整队列`}
          title="拖动调整队列"
          {...attributes}
          {...listeners}
        >
          <IconGripVertical size={18} stroke={1.9} aria-hidden="true" />
        </button>
      }
    />
  );
}

function QueueCards({
  queue,
  items,
  busyEpisodes,
  onOpen,
  onMove,
  dragEnabled,
  dragPreview,
}: {
  queue: ConsumptionQueue;
  items: ConsumptionItem[];
  busyEpisodes: Set<number>;
  onOpen: (item: ConsumptionItem, trigger: HTMLButtonElement) => void;
  onMove: (
    item: ConsumptionItem,
    target: ConsumptionQueue,
  ) => Promise<ConsumptionItem | undefined>;
  dragEnabled: boolean;
  dragPreview: QueuePlacementPreview | null;
}) {
  const previewForQueue = dragPreview?.queue === queue ? dragPreview : null;
  const renderCard = (item: ConsumptionItem, index: number) => {
    const showInsertBefore =
      previewForQueue?.beforeEpisodeId === item.episode_id;
    const showInsertAfter =
      previewForQueue?.beforeEpisodeId === null && index === items.length - 1;

    return dragEnabled ? (
      <SortableConsumptionCard
        key={item.episode_id}
        item={item}
        queue={queue}
        busy={busyEpisodes.has(item.episode_id)}
        onOpen={onOpen}
        onMove={onMove}
        showInsertBefore={showInsertBefore}
        showInsertAfter={showInsertAfter}
      />
    ) : (
      <ConsumptionCard
        key={item.episode_id}
        item={item}
        busy={busyEpisodes.has(item.episode_id)}
        onOpen={onOpen}
        onMove={onMove}
        showInsertBefore={showInsertBefore}
        showInsertAfter={showInsertAfter}
      />
    );
  };

  const content = <>{items.map(renderCard)}</>;

  if (!dragEnabled) return content;
  return (
    <SortableContext
      items={items.map((item) => episodeDragId(item.episode_id))}
      strategy={verticalListSortingStrategy}
    >
      {content}
    </SortableContext>
  );
}

function DroppableQueueItems({
  queue,
  isPreviewTarget,
  children,
}: {
  queue: ConsumptionQueue;
  isPreviewTarget: boolean;
  children: ReactNode;
}) {
  const { isOver, setNodeRef } = useDroppable({
    id: queueDropId(queue),
    data: { kind: "queue", queue } satisfies QueueDragData,
  });

  return (
    <div
      ref={setNodeRef}
      className={`${styles.queueItems}${
        isOver || isPreviewTarget ? ` ${styles.queueItemsDropActive}` : ""
      }`}
      data-queue-drop={queue}
    >
      {children}
    </div>
  );
}

function QueueItemsContainer({
  queue,
  dragEnabled,
  isPreviewTarget,
  children,
}: {
  queue: ConsumptionQueue;
  dragEnabled: boolean;
  isPreviewTarget: boolean;
  children: ReactNode;
}) {
  if (!dragEnabled) {
    return <div className={styles.queueItems}>{children}</div>;
  }
  return (
    <DroppableQueueItems queue={queue} isPreviewTarget={isPreviewTarget}>
      {children}
    </DroppableQueueItems>
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
  dragEnabled,
  dragPreview,
}: ConsumptionQueueColumnProps) {
  const presentation = QUEUE_PRESENTATION[queue];
  const canDragInQueue = dragEnabled && !isLoading && !error;

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

      <QueueItemsContainer
        queue={queue}
        dragEnabled={canDragInQueue}
        isPreviewTarget={dragPreview?.queue === queue}
      >
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
          <QueueCards
            queue={queue}
            items={items}
            busyEpisodes={busyEpisodes}
            onOpen={onOpen}
            onMove={onMove}
            dragEnabled={canDragInQueue}
            dragPreview={dragPreview}
          />
        )}
      </QueueItemsContainer>
    </section>
  );
}
