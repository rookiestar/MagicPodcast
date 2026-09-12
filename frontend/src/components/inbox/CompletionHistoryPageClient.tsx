"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { createPortal } from "react-dom";
import Link from "next/link";
import EpisodeLink from "@/components/episodes/EpisodeLink";
import { singleParam, updateQuery, useLocationHref } from "@/lib/navigation";
import {
  IconAlertTriangle,
  IconArrowLeft,
  IconArrowRight,
  IconBookmarkPlus,
  IconCircleCheck,
  IconClock,
  IconDots,
  IconRefresh,
  IconSearch,
  IconTargetArrow,
  IconX,
} from "@tabler/icons-react";
import PageLayout from "@/components/layout/PageLayout";
import {
  consumptionApi,
  getConsumptionErrorDetails,
  requiresFocusConfirmation,
} from "@/lib/api/consumption";
import type {
  CompletionHistoryItem,
  CompletionHistoryStatus,
  ConsumptionQueue,
} from "@/types/consumption";
import { formatCompletedDate, QUEUE_PRESENTATION } from "./presentation";
import { useMenuPopover } from "./useMenuPopover";
import styles from "./CompletionHistoryPage.module.css";

const ACTION_QUEUES: ConsumptionQueue[] = ["inbox", "focus", "someday"];

const REPROCESS_TARGETS: {
  queue: ConsumptionQueue;
  label: string;
}[] = [
  { queue: "inbox", label: "加入 Inbox" },
  { queue: "focus", label: "加入 Focus" },
  { queue: "someday", label: "加入 Someday" },
];

function queueIcon(queue: ConsumptionQueue) {
  const props = { size: 16, stroke: 1.8, "aria-hidden": true } as const;
  switch (queue) {
    case "inbox":
      return <IconBookmarkPlus {...props} />;
    case "focus":
      return <IconTargetArrow {...props} />;
    case "someday":
      return <IconClock {...props} />;
    default:
      return <IconCircleCheck {...props} />;
  }
}

function statusHint(status: CompletionHistoryStatus) {
  if (status === "dismissed") return "当前不感兴趣";
  if (status === "unassigned") return "当前未安排";
  return "";
}

function locateLabel(status: CompletionHistoryStatus) {
  switch (status) {
    case "inbox":
      return "已在 Inbox";
    case "focus":
      return "已在 Focus";
    case "someday":
      return "已在 Someday";
    default:
      return "";
  }
}

interface FocusPrompt {
  item: CompletionHistoryItem;
  currentCount: number;
  limit: number;
}

function appendUniqueHistoryItems(
  current: CompletionHistoryItem[],
  incoming: CompletionHistoryItem[],
) {
  const seen = new Set(current.map((item) => item.episode_id));
  return [
    ...current,
    ...incoming.filter((item) => {
      if (seen.has(item.episode_id)) return false;
      seen.add(item.episode_id);
      return true;
    }),
  ];
}

/** 次要操作菜单：选择目标即触发重新处理；portal 挂到 body，避免被列表裁切。 */
function ReprocessMenu({
  item,
  busy,
  onSelect,
}: {
  item: CompletionHistoryItem;
  busy: boolean;
  onSelect: (item: CompletionHistoryItem, target: ConsumptionQueue) => void;
}) {
  const {
    open,
    menuId,
    triggerRef,
    menuRef,
    closeMenu,
    toggleMenu,
    handleMenuKeyDown,
  } = useMenuPopover();
  // 首帧以离屏位置挂载 portal，保证 useMenuPopover 的聚焦 effect 能拿到 menuRef；
  // useLayoutEffect 会在 paint 前把菜单移到触发器旁，用户看不到离屏帧。
  const [position, setPosition] = useState<{ top: number; right: number }>(
    () => ({ top: -9999, right: 8 }),
  );

  const updatePosition = useCallback(() => {
    const trigger = triggerRef.current;
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    const menuHeight = menuRef.current?.getBoundingClientRect().height ?? 0;
    const viewportMargin = 8;
    const triggerGap = 5;
    const belowTop = rect.bottom + triggerGap;
    const fitsBelow =
      menuHeight === 0 ||
      belowTop + menuHeight <= window.innerHeight - viewportMargin;
    setPosition({
      top: fitsBelow
        ? belowTop
        : Math.max(viewportMargin, rect.top - menuHeight - triggerGap),
      right: Math.max(viewportMargin, window.innerWidth - rect.right),
    });
  }, [menuRef, triggerRef]);

  useLayoutEffect(() => {
    if (!open) return;
    updatePosition();
    const onViewportChange = () => updatePosition();
    window.addEventListener("resize", onViewportChange);
    window.addEventListener("scroll", onViewportChange, true);
    return () => {
      window.removeEventListener("resize", onViewportChange);
      window.removeEventListener("scroll", onViewportChange, true);
    };
  }, [open, updatePosition]);

  const select = (target: ConsumptionQueue) => {
    closeMenu();
    onSelect(item, target);
  };

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        className={styles.moreButton}
        aria-label={
          busy
            ? `正在保存《${item.episode_title}》的队列调整`
            : `《${item.episode_title}》的更多操作`
        }
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        aria-disabled={busy}
        onClick={() => {
          if (!busy) toggleMenu();
        }}
      >
        <IconDots size={19} stroke={1.9} aria-hidden="true" />
      </button>
      {open && typeof document !== "undefined"
        ? createPortal(
            <div
              ref={menuRef}
              id={menuId}
              className={styles.rowMenuPopover}
              style={position}
              role="menu"
              aria-label={`重新处理《${item.episode_title}》`}
              onKeyDown={handleMenuKeyDown}
            >
              <span className={styles.rowMenuTitle} aria-hidden="true">
                重新处理到
              </span>
              {REPROCESS_TARGETS.map(({ queue, label }) => (
                <button
                  key={queue}
                  type="button"
                  role="menuitem"
                  aria-disabled={busy}
                  onClick={() => select(queue)}
                >
                  {queueIcon(queue)}
                  {label}
                </button>
              ))}
            </div>,
            document.body,
          )
        : null}
      {busy && (
        <span className={styles.srOnly} role="status">
          正在保存队列调整…
        </span>
      )}
    </>
  );
}

export default function CompletionHistoryPageClient() {
  const [items, setItems] = useState<CompletionHistoryItem[]>([]);
  const [draftQuery, setDraftQuery] = useState("");
  const href = useLocationHref();
  const queryParams = new URL(href || "/", "http://navigation.local").searchParams;
  const activeQuery = (singleParam(queryParams, "q") ?? "").trim();
  const locationReady = Boolean(href);
  const [totalCount, setTotalCount] = useState<number | null>(null);
  const [matchCount, setMatchCount] = useState<number | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [isInitialLoading, setIsInitialLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [initialError, setInitialError] = useState<string | null>(null);
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const [pageError, setPageError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [announcement, setAnnouncement] = useState("");
  const [busyEpisodes, setBusyEpisodes] = useState<Set<number>>(
    () => new Set(),
  );
  const [focusPrompt, setFocusPrompt] = useState<FocusPrompt | null>(null);
  const itemsRef = useRef(items);
  const requestVersion = useRef(0);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const focusCancelRef = useRef<HTMLButtonElement>(null);
  const actionCells = useRef(new Map<number, HTMLDivElement>());
  const restoreActionFocus = useRef<number | null>(null);

  useLayoutEffect(() => {
    const episodeID = restoreActionFocus.current;
    if (episodeID === null) return;
    restoreActionFocus.current = null;
    actionCells.current.get(episodeID)?.querySelector<HTMLAnchorElement>("a")?.focus({ preventScroll: true });
  }, [items]);

  useEffect(() => {
    itemsRef.current = items;
  }, [items]);

  useEffect(() => {
    if (!focusPrompt) return;
    const trigger = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    focusCancelRef.current?.focus();
    return () => {
      if (trigger?.isConnected) trigger.focus({ preventScroll: true });
    };
  }, [focusPrompt]);

  const loadFirstPage = useCallback(async (query: string) => {
    const version = ++requestVersion.current;
    const hasCurrentItems = itemsRef.current.length > 0;
    if (hasCurrentItems) {
      setIsRefreshing(true);
    } else {
      setIsInitialLoading(true);
    }
    setIsLoadingMore(false);
    setHasMore(false);
    setNextCursor(null);
    setInitialError(null);
    setRefreshError(null);
    setPageError(null);
    try {
      const payload = await consumptionApi.listCompletionHistory({ query });
      if (version !== requestVersion.current) return;
      setItems(payload.items);
      setTotalCount(payload.total_count);
      setMatchCount(payload.match_count);
      setHasMore(payload.has_more);
      setNextCursor(payload.next_cursor ?? null);

    } catch (error) {
      if (version !== requestVersion.current) return;
      const message = getConsumptionErrorDetails(error).message;
      if (hasCurrentItems) {
        setRefreshError(message);
      } else {
        setInitialError(message);
      }
    } finally {
      if (version === requestVersion.current) {
        setIsInitialLoading(false);
        setIsRefreshing(false);
      }
    }
  }, []);

  useEffect(() => {
    if (!href) return;
    const params = new URL(href, "http://navigation.local").searchParams;
    if (params.has("q") && (params.getAll("q").length !== 1 || params.get("q") !== activeQuery || !activeQuery)) {
      updateQuery({ q: activeQuery || null }, true);
    }
  }, [href, activeQuery]);

  useEffect(() => {
    if (!locationReady) return;
    setDraftQuery(activeQuery);
    void loadFirstPage(activeQuery);
    const version = requestVersion;
    return () => { version.current++; };
  }, [activeQuery, locationReady, loadFirstPage]);

  const loadNextPage = useCallback(async () => {
    if (!hasMore || !nextCursor || isLoadingMore) return;
    const version = ++requestVersion.current;
    setIsLoadingMore(true);
    setPageError(null);
    try {
      const payload = await consumptionApi.listCompletionHistory({
        query: activeQuery,
        cursor: nextCursor,
      });
      if (version !== requestVersion.current) return;
      setItems((current) => appendUniqueHistoryItems(current, payload.items));
      setTotalCount(payload.total_count);
      setMatchCount(payload.match_count);
      setHasMore(payload.has_more);
      setNextCursor(payload.next_cursor ?? null);
    } catch (error) {
      if (version !== requestVersion.current) return;
      setPageError(getConsumptionErrorDetails(error).message);
    } finally {
      if (version === requestVersion.current) {
        setIsLoadingMore(false);
      }
    }
  }, [activeQuery, hasMore, isLoadingMore, nextCursor]);

  const handleSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const query = draftQuery.trim();
    if (query === activeQuery) void loadFirstPage(query);
    else updateQuery({ q: query || null });
  };

  const clearSearch = () => {
    setDraftQuery("");
    searchInputRef.current?.focus();
    if (activeQuery) updateQuery({ q: null });
    else void loadFirstPage("");
  };

  const performReprocess = useCallback(
    async (
      item: CompletionHistoryItem,
      target: ConsumptionQueue,
      acknowledgeFocusLimit = false,
    ) => {
      if (busyEpisodes.has(item.episode_id)) return;
      setBusyEpisodes((current) => new Set(current).add(item.episode_id));
      setActionError(null);
      try {
        const updated = await consumptionApi.setQueue(item.episode_id, target, {
          acknowledgeFocusLimit,
        });
        if (actionCells.current.get(item.episode_id)?.contains(document.activeElement)) {
          restoreActionFocus.current = item.episode_id;
        }
        setItems((current) =>
          current.map((candidate) =>
            candidate.episode_id === item.episode_id
              ? {
                  ...candidate,
                  current_status: updated.queue_state ?? target,
                }
              : candidate,
          ),
        );
        setAnnouncement(
          `《${item.episode_title}》已移至 ${QUEUE_PRESENTATION[target].label}。`,
        );
      } catch (error) {
        const details = getConsumptionErrorDetails(error);
        if (target === "focus" && requiresFocusConfirmation(error)) {
          setFocusPrompt({
            item,
            currentCount: details.currentCount ?? 7,
            limit: details.focusLimit ?? 7,
          });
        } else {
          setActionError(
            `《${item.episode_title}》重新处理失败：${details.message}`,
          );
        }
      } finally {
        setBusyEpisodes((current) => {
          const next = new Set(current);
          next.delete(item.episode_id);
          return next;
        });
      }
    },
    [busyEpisodes],
  );

  const showInitialLoading = isInitialLoading && items.length === 0;
  const showInitialError = Boolean(initialError) && items.length === 0;
  const showEmpty =
    !showInitialLoading && !showInitialError && items.length === 0;

  return (
    <PageLayout
      toolbar={false}
      maxWidth={false}
      rootClassName={styles.shell}
      className={styles.layout}
    >
      <main className={styles.page}>
        <header className={styles.pageHeader}>
          <nav className={styles.contextNav} aria-label="完成历史路径">
            <Link href="/inbox" prefetch={false}>
              <IconArrowLeft size={16} stroke={1.9} aria-hidden="true" />
              返回 Inbox
            </Link>
          </nav>
          <div className={styles.titleRow}>
            <h1>完成历史</h1>
            <span className={styles.totalCount} aria-live="polite">
              {totalCount === null ? "…" : totalCount} 个单集
            </span>
          </div>
        </header>

        <form className={styles.searchBar} role="search" onSubmit={handleSearch}>
          <div className={styles.searchControl}>
            <IconSearch size={17} stroke={1.8} aria-hidden="true" />
            <input
              ref={searchInputRef}
              id="completion-history-search"
              type="search"
              value={draftQuery}
              onChange={(event) => setDraftQuery(event.target.value)}
              placeholder="搜索单集或节目…"
              aria-label="搜索单集或节目"
              autoComplete="off"
            />
            {(draftQuery || activeQuery) && (
              <button
                type="button"
                className={styles.clearSearch}
                onClick={clearSearch}
                aria-label="清除完成历史搜索"
              >
                <IconX size={17} stroke={1.8} aria-hidden="true" />
              </button>
            )}
          </div>
          <button
            type="submit"
            className={styles.searchButton}
            disabled={isRefreshing}
          >
            搜索
          </button>
        </form>

        <div className={styles.resultBar}>
          <p aria-live="polite">
            {activeQuery
              ? `“${activeQuery}”找到 ${matchCount ?? 0} 个单集`
              : matchCount === null
                ? "正在读取完成事实"
                : "按最近完成时间排列"}
          </p>
          {isRefreshing && (
            <span role="status">
              <IconRefresh size={14} stroke={1.8} aria-hidden="true" />
              正在更新，现有记录保持可用
            </span>
          )}
        </div>

        {refreshError && (
          <div className={styles.inlineError} role="alert">
            <IconAlertTriangle size={18} stroke={1.8} aria-hidden="true" />
            <span>更新失败，当前记录仍可用：{refreshError}</span>
            <button type="button" onClick={() => void loadFirstPage(activeQuery)}>
              重试
            </button>
          </div>
        )}

        {actionError && (
          <div className={styles.inlineError} role="alert">
            <IconAlertTriangle size={18} stroke={1.8} aria-hidden="true" />
            <span>{actionError}</span>
            <button type="button" onClick={() => setActionError(null)}>
              关闭
            </button>
          </div>
        )}

        {showInitialLoading && (
          <section className={styles.loadingList} aria-label="正在加载完成历史">
            <p className={styles.srOnly} role="status">正在加载完成历史…</p>
            {Array.from({ length: 8 }, (_, index) => (
              <div key={index} className={styles.loadingRow} aria-hidden="true" />
            ))}
          </section>
        )}

        {showInitialError && (
          <section className={styles.centerState} role="alert">
            <IconAlertTriangle size={28} stroke={1.6} aria-hidden="true" />
            <h2>完成历史暂时无法加载</h2>
            <p>{initialError}</p>
            <button type="button" onClick={() => void loadFirstPage(activeQuery)}>
              <IconRefresh size={17} stroke={1.8} aria-hidden="true" />
              重试加载
            </button>
          </section>
        )}

        {showEmpty && (
          <section className={styles.centerState}>
            <IconCircleCheck size={28} stroke={1.6} aria-hidden="true" />
            <h2>{activeQuery ? "没有匹配的完成记录" : "还没有完成记录"}</h2>
            <p>
              {activeQuery
                ? "换一个单集标题或节目名称试试。"
                : "在 Inbox 中明确完成单集后，它会永久保留在这里。"}
            </p>
            {activeQuery && (
              <button type="button" onClick={clearSearch}>
                查看全部历史
              </button>
            )}
          </section>
        )}

        {items.length > 0 && (
          <section className={styles.historyList} aria-label="完成历史记录">
            {items.map((item) => {
              const isActionQueue = ACTION_QUEUES.includes(
                item.current_status as ConsumptionQueue,
              );
              const hint = statusHint(item.current_status);
              return (
                <article
                  key={item.episode_id}
                  className={styles.historyRow}
                  data-episode-id={item.episode_id}
                >
                  <p className={styles.podcastCell}>{item.podcast_title}</p>
                  <h2 className={styles.titleCell}>
                    <EpisodeLink
                      episodeID={item.episode_id}
                      source="history"
                      href={`/episodes/${item.episode_id}?from=history`}
                      data-editorial-display-text="true"
                    >
                      {item.episode_title}
                    </EpisodeLink>
                  </h2>
                  <span className={styles.dateCell}>
                    <IconCircleCheck size={14} stroke={1.8} aria-hidden="true" />
                    <span className={styles.srOnly}>最近完成于 </span>
                    {formatCompletedDate(item.completed_at)}
                  </span>
                  <div className={styles.actionCell} ref={(node) => {
                    if (node) actionCells.current.set(item.episode_id, node);
                    else actionCells.current.delete(item.episode_id);
                  }}>
                    {isActionQueue ? (
                      <Link
                        className={styles.locateLink}
                        href={`/inbox?queue=${item.current_status}&episode=${item.episode_id}`}
                        prefetch={false}
                      >
                        {locateLabel(item.current_status)}
                        <IconArrowRight
                          size={14}
                          stroke={1.8}
                          aria-hidden="true"
                        />
                      </Link>
                    ) : (
                      <>
                        {hint && (
                          <span className={styles.statusHint}>{hint}</span>
                        )}
                        <ReprocessMenu
                          item={item}
                          busy={busyEpisodes.has(item.episode_id)}
                          onSelect={(menuItem, queue) =>
                            void performReprocess(menuItem, queue)
                          }
                        />
                      </>
                    )}
                  </div>
                </article>
              );
            })}
          </section>
        )}

        {items.length > 0 && (hasMore || pageError) && (
          <div className={styles.pagination}>
            {pageError && (
              <p role="alert">
                下一页加载失败，已加载的 {items.length} 条记录保持可用：
                {pageError}
              </p>
            )}
            <button
              type="button"
              disabled={isLoadingMore}
              onClick={() => void loadNextPage()}
            >
              {isLoadingMore
                ? "正在加载下一页…"
                : pageError
                  ? "重试加载下一页"
                  : "继续加载"}
              {!isLoadingMore && (
                <IconArrowRight size={17} stroke={1.8} aria-hidden="true" />
              )}
            </button>
          </div>
        )}

        <p className={styles.srOnly} aria-live="polite">
          {announcement}
        </p>
      </main>

      {focusPrompt && (
        <div
          className={styles.dialogBackdrop}
          onMouseDown={(event) => {
            if (event.currentTarget === event.target) setFocusPrompt(null);
          }}
          onKeyDown={(event) => {
            if (event.key === "Escape") setFocusPrompt(null);
          }}
        >
          <section
            className={styles.confirmDialog}
            role="dialog"
            aria-modal="true"
            aria-labelledby="completion-history-focus-title"
            aria-describedby="completion-history-focus-description"
          >
            <span className={styles.dialogMark} aria-hidden="true">
              <IconTargetArrow size={22} stroke={1.8} />
            </span>
            <h2 id="completion-history-focus-title">Focus 已有明确承诺</h2>
            <p id="completion-history-focus-description">
              当前已有 {focusPrompt.currentCount} 项，建议上限为{" "}
              {focusPrompt.limit} 项。仍要把《{focusPrompt.item.episode_title}》
              加入 Focus 吗？
            </p>
            <div className={styles.dialogActions}>
              <button
                ref={focusCancelRef}
                type="button"
                onClick={() => setFocusPrompt(null)}
              >
                取消
              </button>
              <button
                type="button"
                className={styles.dialogPrimary}
                onClick={() => {
                  const prompt = focusPrompt;
                  setFocusPrompt(null);
                  void performReprocess(prompt.item, "focus", true);
                }}
              >
                仍然加入 Focus
              </button>
            </div>
          </section>
        </div>
      )}
    </PageLayout>
  );
}
