"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import Link from "next/link";
import {
  IconAlertTriangle,
  IconArrowLeft,
  IconArrowRight,
  IconCircleCheck,
  IconClock,
  IconHistory,
  IconInbox,
  IconRefresh,
  IconSearch,
  IconTargetArrow,
  IconX,
} from "@tabler/icons-react";
import PageLayout from "@/components/layout/PageLayout";
import PlainImage from "@/components/ui/PlainImage";
import { consumptionApi, getConsumptionErrorDetails, requiresFocusConfirmation } from "@/lib/api/consumption";
import { getOptimizedImageUrl } from "@/lib/imageOptimization";
import type {
  CompletionHistoryItem,
  CompletionHistoryStatus,
  ConsumptionQueue,
} from "@/types/consumption";
import { formatCompletedDate } from "./presentation";
import styles from "./CompletionHistoryPage.module.css";

const ACTION_QUEUES: ConsumptionQueue[] = ["inbox", "focus", "someday"];

const STATUS_COPY: Record<
  CompletionHistoryStatus,
  { label: string; short: string }
> = {
  inbox: { label: "曾完成 · 当前 Inbox", short: "Inbox" },
  focus: { label: "曾完成 · 当前 Focus", short: "Focus" },
  someday: { label: "曾完成 · 当前 Someday", short: "Someday" },
  done: { label: "当前 Done", short: "Done" },
  dismissed: { label: "曾完成 · 当前不感兴趣", short: "不感兴趣" },
  unassigned: { label: "曾完成 · 当前未安排", short: "未安排" },
};

interface FocusPrompt {
  item: CompletionHistoryItem;
  currentCount: number;
  limit: number;
}

function statusIcon(status: CompletionHistoryStatus) {
  const props = { size: 15, stroke: 1.8, "aria-hidden": true } as const;
  switch (status) {
    case "inbox":
      return <IconInbox {...props} />;
    case "focus":
      return <IconTargetArrow {...props} />;
    case "someday":
      return <IconClock {...props} />;
    default:
      return <IconCircleCheck {...props} />;
  }
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

export default function CompletionHistoryPageClient() {
  const [items, setItems] = useState<CompletionHistoryItem[]>([]);
  const [draftQuery, setDraftQuery] = useState("");
  const [activeQuery, setActiveQuery] = useState("");
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
  const [selectedTargets, setSelectedTargets] = useState<
    Record<number, ConsumptionQueue>
  >({});
  const [focusPrompt, setFocusPrompt] = useState<FocusPrompt | null>(null);
  const itemsRef = useRef(items);
  const requestVersion = useRef(0);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const focusCancelRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    itemsRef.current = items;
  }, [items]);

  useEffect(() => {
    if (focusPrompt) focusCancelRef.current?.focus();
  }, [focusPrompt]);

  const loadFirstPage = useCallback(async (query: string) => {
    const version = ++requestVersion.current;
    const hasCurrentItems = itemsRef.current.length > 0;
    if (hasCurrentItems) {
      setIsRefreshing(true);
    } else {
      setIsInitialLoading(true);
    }
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
      setActiveQuery(payload.search_query);
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
    void loadFirstPage("");
  }, [loadFirstPage]);

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
    void loadFirstPage(draftQuery);
  };

  const clearSearch = () => {
    setDraftQuery("");
    searchInputRef.current?.focus();
    void loadFirstPage("");
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
        setAnnouncement(`《${item.episode_title}》已移至 ${STATUS_COPY[target].short}。`);
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
        <nav className={styles.contextNav} aria-label="完成历史路径">
          <Link href="/inbox" prefetch={false}>
            <IconArrowLeft size={17} stroke={1.9} aria-hidden="true" />
            返回 Inbox
          </Link>
          <span aria-hidden="true">/</span>
          <span>完成历史</span>
        </nav>

        <header className={styles.hero}>
          <div className={styles.heroMark} aria-hidden="true">
            <IconHistory size={25} stroke={1.55} />
          </div>
          <div className={styles.heroCopy}>
            <span className={styles.kicker}>COMPLETION HISTORY</span>
            <h1>完成历史</h1>
            <p>
              完成意味着一件事已退出当前注意力；重新处理不会抹去这段事实。
            </p>
          </div>
          <div className={styles.tally} aria-live="polite">
            <span>全部完成</span>
            <strong>{totalCount ?? "—"}</strong>
            <small>个唯一单集</small>
          </div>
        </header>

        <form className={styles.searchBar} role="search" onSubmit={handleSearch}>
          <label htmlFor="completion-history-search">
            搜索单集或节目
          </label>
          <div className={styles.searchControl}>
            <IconSearch size={19} stroke={1.8} aria-hidden="true" />
            <input
              ref={searchInputRef}
              id="completion-history-search"
              type="search"
              value={draftQuery}
              onChange={(event) => setDraftQuery(event.target.value)}
              placeholder="输入单集标题或节目名称"
              autoComplete="off"
            />
            {(draftQuery || activeQuery) && (
              <button
                type="button"
                className={styles.clearSearch}
                onClick={clearSearch}
                aria-label="清除完成历史搜索"
              >
                <IconX size={18} stroke={1.8} aria-hidden="true" />
              </button>
            )}
          </div>
          <button
            type="submit"
            className={styles.searchButton}
            disabled={isRefreshing}
          >
            搜索全部历史
          </button>
        </form>

        <div className={styles.resultBar}>
          <p aria-live="polite">
            {activeQuery
              ? `“${activeQuery}”找到 ${matchCount ?? 0} 条`
              : matchCount === null
                ? "正在读取完成事实"
                : `按最近完成时间排列 · ${matchCount} 条`}
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
            <button type="button" onClick={() => void loadFirstPage(draftQuery)}>
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
          <section className={styles.loadingGrid} aria-label="正在加载完成历史">
            <p role="status">正在加载完成历史…</p>
            {Array.from({ length: 6 }, (_, index) => (
              <div key={index} className={styles.loadingCard} aria-hidden="true">
                <span />
                <span />
                <span />
              </div>
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
          <section className={styles.historyGrid} aria-label="完成历史记录">
            {items.map((item, index) => {
              const status = STATUS_COPY[item.current_status];
              const target = selectedTargets[item.episode_id] ?? "inbox";
              const isActionQueue = ACTION_QUEUES.includes(
                item.current_status as ConsumptionQueue,
              );
              const coverSource = getOptimizedImageUrl(
                item.image_url || item.podcast_cover_url,
                128,
              );
              return (
                <article
                  key={item.episode_id}
                  className={styles.historyCard}
                  style={{ "--history-index": index } as React.CSSProperties}
                  data-episode-id={item.episode_id}
                >
                  <div className={styles.cardNumber} aria-hidden="true">
                    {String(index + 1).padStart(2, "0")}
                  </div>
                  <div className={styles.cardBody}>
                    <div className={styles.cardIdentity}>
                      <span className={styles.coverFrame} aria-hidden="true">
                        {coverSource ? (
                          <PlainImage
                            src={coverSource}
                            alt=""
                            width={64}
                            height={64}
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
                      <div>
                        <p className={styles.podcastLine}>
                          {item.podcast_title}
                          {item.episode_no ? ` · ${item.episode_no}` : ""}
                        </p>
                        <h2>{item.episode_title}</h2>
                      </div>
                    </div>
                    <div className={styles.cardFacts}>
                      <span>
                        <IconCircleCheck
                          size={15}
                          stroke={1.8}
                          aria-hidden="true"
                        />
                        最近完成于 {formatCompletedDate(item.completed_at)}
                      </span>
                      <span
                        className={`${styles.statusBadge} ${
                          styles[`status_${item.current_status}`]
                        }`}
                      >
                        {statusIcon(item.current_status)}
                        {status.label}
                      </span>
                    </div>
                  </div>
                  <div className={styles.cardAction}>
                    {isActionQueue ? (
                      <>
                        <p>这条内容已在当前行动工作台中。</p>
                        <Link
                          href={`/inbox?queue=${item.current_status}&episode=${item.episode_id}`}
                          prefetch={false}
                        >
                          定位到 {status.short}
                          <IconArrowRight
                            size={16}
                            stroke={1.8}
                            aria-hidden="true"
                          />
                        </Link>
                      </>
                    ) : (
                      <>
                        <label htmlFor={`history-target-${item.episode_id}`}>
                          重新处理到
                        </label>
                        <div className={styles.reprocessControl}>
                          <select
                            id={`history-target-${item.episode_id}`}
                            value={target}
                            disabled={busyEpisodes.has(item.episode_id)}
                            onChange={(event) =>
                              setSelectedTargets((current) => ({
                                ...current,
                                [item.episode_id]: event.target
                                  .value as ConsumptionQueue,
                              }))
                            }
                          >
                            <option value="inbox">Inbox</option>
                            <option value="focus">Focus</option>
                            <option value="someday">Someday</option>
                          </select>
                          <button
                            type="button"
                            disabled={busyEpisodes.has(item.episode_id)}
                            onClick={() => void performReprocess(item, target)}
                          >
                            {busyEpisodes.has(item.episode_id)
                              ? "正在保存…"
                              : "重新处理"}
                          </button>
                        </div>
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
            <span className={styles.kicker}>FOCUS LIMIT</span>
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
