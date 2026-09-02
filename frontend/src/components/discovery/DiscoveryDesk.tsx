"use client";

import {
  Fragment,
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from "react";
import type { ReactNode, TouchEvent } from "react";
import {
  IconBookmarkFilled,
  IconBookmarkPlus,
  IconChevronLeft,
  IconChevronRight,
  IconEye,
  IconEyeOff,
  IconExternalLink,
  IconPencil,
  IconX,
} from "@tabler/icons-react";
import dynamic from "next/dynamic";
import { OriginalEpisodeRecovery } from "@/components/common/OriginalEpisodeRecovery";
import DiscoveryMetadataEditor from "@/components/discovery/DiscoveryMetadataEditor";
import PlainImage from "@/components/ui/PlainImage";
import { useOriginalEpisodeRecovery } from "@/hooks/useOriginalEpisodeRecovery";
import { formatEpisodeNumber } from "@/lib/episodeDisplay";
import { planSafeOriginalEpisodeOpen } from "@/lib/originalEpisodeOpen";
import type {
  DiscoveryConsumptionResponse,
  DiscoveryCandidate,
  TriageDecisionResponse,
  TriageDecisionState,
} from "@/types/discovery";

const RichText = dynamic(() => import("@/components/RichText"), {
  ssr: false,
  loading: () => (
    <p className="discovery-show-notes-loading">正在加载 Show Notes…</p>
  ),
});

interface DiscoveryDeskProps {
  candidates: DiscoveryCandidate[];
  reportContent?: ReactNode;
  focusContent?: ReactNode;
  noticeContent?: ReactNode;
  onDecision?: (
    episodeID: number,
    state: TriageDecisionState,
  ) => Promise<TriageDecisionResponse>;
  onRead?: (episodeID: number) => Promise<DiscoveryConsumptionResponse>;
  onLoadCandidateDetails?: (
    episodeID: number,
  ) => Promise<DiscoveryCandidate>;
}

type RecentFilter = "all" | "unread" | "uncollected";

interface RecentPaginationState {
  page: number;
  pageSize: number;
}

type RecentPaginationAction =
  | { type: "set-page"; page: number }
  | { type: "reset-page" }
  | { type: "resize"; pageSize: number; preferredIndex?: number };

const DEFAULT_RECENT_PAGE_SIZE = 4;

const recentFilterLabels: Record<RecentFilter, string> = {
  all: "全部",
  unread: "未读",
  uncollected: "未收集",
};

function recentPageSizeForViewport(width: number, height: number) {
  const heightBasedSize =
    height < 700
      ? 3
      : height < 900
        ? 4
        : height < 1000
          ? 5
          : height < 1200
            ? 6
            : 8;
  const widthCap = width <= 480 ? 4 : width <= 1100 ? 5 : 8;
  return Math.min(heightBasedSize, widthCap);
}

function recentPaginationReducer(
  state: RecentPaginationState,
  action: RecentPaginationAction,
): RecentPaginationState {
  if (action.type === "set-page") {
    return action.page === state.page ? state : { ...state, page: action.page };
  }
  if (action.type === "reset-page") {
    return state.page === 0 ? state : { ...state, page: 0 };
  }
  if (action.pageSize === state.pageSize) return state;

  const anchorIndex = action.preferredIndex ?? state.page * state.pageSize;
  return {
    page: Math.floor(anchorIndex / action.pageSize),
    pageSize: action.pageSize,
  };
}

const queueLabels = {
  inbox: "Inbox",
  focus: "Focus",
  someday: "Someday",
  done: "Done",
} as const;

function formatCandidateEpisodeMeta(episodeNo: string, duration: number) {
  return [formatEpisodeNumber(episodeNo), formatDuration(duration)]
    .filter(Boolean)
    .join(" · ");
}

function formatDuration(seconds: number) {
  if (seconds <= 0) return "时长未知";
  return `${Math.round(seconds / 60)} 分钟`;
}

function formatCandidateDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function candidateDateGroup(value: string) {
  const date = new Date(value);
  const today = new Date();
  const startOfToday = new Date(
    today.getFullYear(),
    today.getMonth(),
    today.getDate(),
  );
  const startOfCandidate = new Date(
    date.getFullYear(),
    date.getMonth(),
    date.getDate(),
  );
  const dayDifference = Math.round(
    (startOfToday.getTime() - startOfCandidate.getTime()) / 86_400_000,
  );
  if (dayDifference === 0) return "今天";
  if (dayDifference === 1) return "昨天";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "long",
    day: "numeric",
    weekday: "short",
  }).format(date);
}

function candidateExcerpt(candidate: DiscoveryCandidate) {
  if (candidate.excerpt?.trim()) return candidate.excerpt.trim();

  const summary = candidate.pre_reads?.find(
    (preRead) => preRead.kind === "summary",
  )?.content;
  if (summary?.trim()) return summary.trim();

  const showNotesText = (candidate.show_notes ?? "")
    .replace(/<[^>]*>/g, " ")
    .replace(/\s+/g, " ")
    .trim();
  return showNotesText || "暂无文字摘要";
}

function CandidateCover({
  candidate,
  className,
}: {
  candidate: DiscoveryCandidate;
  className: string;
}) {
  const cover = candidate.image_url || candidate.podcast_cover_url;
  const [coverFailed, setCoverFailed] = useState(false);

  useEffect(() => {
    setCoverFailed(false);
  }, [cover]);

  if (!cover || coverFailed) {
    return (
      <div className={`${className} discovery-cover-missing`}>
        <span>暂无封面</span>
      </div>
    );
  }

  return (
    <PlainImage
      src={cover}
      alt={`${candidate.podcast_title}封面`}
      className={`${className} object-cover`}
      loading="lazy"
      onError={() => setCoverFailed(true)}
    />
  );
}

export default function DiscoveryDesk({
  candidates,
  reportContent,
  focusContent,
  noticeContent,
  onDecision,
  onRead,
  onLoadCandidateDetails,
}: DiscoveryDeskProps) {
  const [displayCandidates, setDisplayCandidates] = useState(candidates);
  const [activeFilter, setActiveFilter] = useState<RecentFilter>("all");
  const [recentPagination, dispatchRecentPagination] = useReducer(
    recentPaginationReducer,
    { page: 0, pageSize: DEFAULT_RECENT_PAGE_SIZE },
  );
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const [savingEpisodeID, setSavingEpisodeID] = useState<number | null>(null);
  const [decisionError, setDecisionError] = useState("");
  const [detailErrorEpisodeID, setDetailErrorEpisodeID] = useState<
    number | null
  >(null);
  const [detailRetryVersion, setDetailRetryVersion] = useState(0);
  const [isMetadataEditorOpen, setIsMetadataEditorOpen] = useState(false);
  const originalRecovery = useOriginalEpisodeRecovery();
  const candidateButtonRefs = useRef(new Map<number, HTMLButtonElement>());
  const candidateActionRefs = useRef(new Map<number, HTMLButtonElement>());
  const previewRef = useRef<HTMLElement>(null);
  const previewCloseRef = useRef<HTMLButtonElement>(null);
  const showNotesPaneRef = useRef<HTMLElement>(null);
  const touchStartX = useRef<number | null>(null);
  const visibleCandidatesRef = useRef<DiscoveryCandidate[]>([]);
  const selectedIDRef = useRef<number | null>(null);
  const recentPageSizeRef = useRef(DEFAULT_RECENT_PAGE_SIZE);
  const pendingResizeFocusRef = useRef<{
    episodeID: number;
    control: "candidate" | "action";
  } | null>(null);

  useEffect(() => {
    const restoredID = window.history.state?.magicpodcastDiscoveryEpisodeID;
    if (
      typeof restoredID === "number" &&
      candidates.some((candidate) => candidate.episode_id === restoredID)
    ) {
      setSelectedID(restoredID);
    }
  }, [candidates]);

  useEffect(() => {
    setDisplayCandidates((currentCandidates) => {
      const currentByEpisodeID = new Map(
        currentCandidates.map((candidate) => [candidate.episode_id, candidate]),
      );
      return candidates.map((candidate) => {
        const current = currentByEpisodeID.get(candidate.episode_id);
        if (!candidate.metadata_only || !current || current.metadata_only) {
          return candidate;
        }
        return {
          ...candidate,
          metadata_only: false,
          show_notes: current.show_notes,
          pre_reads: current.pre_reads,
        };
      });
    });
    setSelectedID((currentID) =>
      currentID !== null &&
      candidates.some((candidate) => candidate.episode_id === currentID)
        ? currentID
        : null,
    );
  }, [candidates]);

  const filterCounts = useMemo(
    () => ({
      all: displayCandidates.length,
      unread: displayCandidates.filter((candidate) => !candidate.read_at)
        .length,
      uncollected: displayCandidates.filter(
        (candidate) => !candidate.queue_state && !candidate.dismissed_at,
      ).length,
    }),
    [displayCandidates],
  );
  const visibleCandidates = useMemo(
    () =>
      displayCandidates.filter((candidate) => {
        if (activeFilter === "unread") return !candidate.read_at;
        if (activeFilter === "uncollected") {
          return !candidate.queue_state && !candidate.dismissed_at;
        }
        return true;
      }),
    [activeFilter, displayCandidates],
  );
  visibleCandidatesRef.current = visibleCandidates;
  selectedIDRef.current = selectedID;
  recentPageSizeRef.current = recentPagination.pageSize;

  useEffect(() => {
    const visualViewport = window.visualViewport;
    const updatePageSize = () => {
      const width = visualViewport?.width ?? window.innerWidth;
      const height = visualViewport?.height ?? window.innerHeight;
      const nextPageSize = recentPageSizeForViewport(width, height);
      if (nextPageSize === recentPageSizeRef.current) return;

      const focusedCandidateID = Array.from(
        candidateButtonRefs.current.entries(),
      ).find(([, button]) => button === document.activeElement)?.[0];
      const focusedActionID = Array.from(
        candidateActionRefs.current.entries(),
      ).find(([, button]) => button === document.activeElement)?.[0];
      const focusedControl =
        focusedCandidateID !== undefined
          ? { episodeID: focusedCandidateID, control: "candidate" as const }
          : focusedActionID !== undefined
            ? { episodeID: focusedActionID, control: "action" as const }
            : null;
      const preferredEpisodeID =
        selectedIDRef.current ?? focusedControl?.episodeID;
      const preferredIndex =
        preferredEpisodeID === undefined || preferredEpisodeID === null
          ? -1
          : visibleCandidatesRef.current.findIndex(
              (candidate) => candidate.episode_id === preferredEpisodeID,
            );

      pendingResizeFocusRef.current =
        selectedIDRef.current === null ? focusedControl : null;

      dispatchRecentPagination({
        type: "resize",
        pageSize: nextPageSize,
        preferredIndex: preferredIndex >= 0 ? preferredIndex : undefined,
      });
    };

    updatePageSize();
    window.addEventListener("resize", updatePageSize);
    visualViewport?.addEventListener("resize", updatePageSize);
    return () => {
      window.removeEventListener("resize", updatePageSize);
      visualViewport?.removeEventListener("resize", updatePageSize);
    };
  }, []);

  const { page: recentPage, pageSize: recentPageSize } = recentPagination;
  const recentPageCount = Math.max(
    1,
    Math.ceil(visibleCandidates.length / recentPageSize),
  );
  const safeRecentPage = Math.min(
    Math.max(recentPage, 0),
    recentPageCount - 1,
  );
  const recentPageStart = safeRecentPage * recentPageSize;
  const pagedCandidates = visibleCandidates.slice(
    recentPageStart,
    recentPageStart + recentPageSize,
  );

  useEffect(() => {
    const pendingFocus = pendingResizeFocusRef.current;
    if (!pendingFocus) return;

    const frame = requestAnimationFrame(() => {
      const refs =
        pendingFocus.control === "candidate"
          ? candidateButtonRefs.current
          : candidateActionRefs.current;
      const control = refs.get(pendingFocus.episodeID);
      pendingResizeFocusRef.current = null;
      if (!control) return;
      control.focus();
    });

    return () => cancelAnimationFrame(frame);
  }, [recentPageSize, safeRecentPage]);
  const selected = useMemo(
    () =>
      selectedID === null
        ? undefined
        : displayCandidates.find(
            (candidate) => candidate.episode_id === selectedID,
          ),
    [displayCandidates, selectedID],
  );
  const selectedIndex = selected
    ? displayCandidates.findIndex(
        (candidate) => candidate.episode_id === selected.episode_id,
      )
    : -1;
  const selectedOriginalPlan = selected
    ? planSafeOriginalEpisodeOpen(selected.original_url)
    : null;

  useEffect(() => {
    if (typeof window === "undefined") return;
    const nextState = { ...window.history.state };
    if (!selected) {
      delete nextState.magicpodcastDiscoveryEpisodeID;
    } else {
      nextState.magicpodcastDiscoveryEpisodeID = selected.episode_id;
    }
    window.history.replaceState(nextState, "");
  }, [selected]);

  useEffect(() => {
    if (showNotesPaneRef.current) {
      showNotesPaneRef.current.scrollTop = 0;
    }
  }, [selected?.episode_id]);

  useEffect(() => {
    if (recentPage !== safeRecentPage) {
      dispatchRecentPagination({ type: "set-page", page: safeRecentPage });
    }
  }, [activeFilter, recentPage, safeRecentPage]);

  const selectedEpisodeID = selected?.episode_id;
  const selectedMetadataOnly = Boolean(selected?.metadata_only);

  useEffect(() => {
    setDetailErrorEpisodeID(null);
    if (
      selectedEpisodeID === undefined ||
      !selectedMetadataOnly ||
      !onLoadCandidateDetails
    ) {
      return;
    }

    let active = true;
    void onLoadCandidateDetails(selectedEpisodeID)
      .then((details) => {
        if (!active) return;
        setDisplayCandidates((items) =>
          items.map((item) =>
            item.episode_id === selectedEpisodeID
              ? {
                  ...details,
                  excerpt: item.excerpt ?? details.excerpt,
                  metadata_only: false,
                  decision_state: item.decision_state,
                  decision_updated_at: item.decision_updated_at,
                  queue_state: item.queue_state,
                  dismissed_at: item.dismissed_at,
                  queue_updated_at: item.queue_updated_at,
                  in_progress_at: item.in_progress_at,
                  read_at: item.read_at,
                }
              : item,
          ),
        );
      })
      .catch(() => {
        if (active) setDetailErrorEpisodeID(selectedEpisodeID);
      });

    return () => {
      active = false;
    };
  }, [
    detailRetryVersion,
    onLoadCandidateDetails,
    selectedEpisodeID,
    selectedMetadataOnly,
  ]);

  const closePreview = useCallback(() => {
    const episodeID = selectedEpisodeID;
    setIsMetadataEditorOpen(false);
    setSelectedID(null);
    requestAnimationFrame(() => {
      if (episodeID !== undefined) {
        candidateButtonRefs.current.get(episodeID)?.focus();
      }
    });
  }, [selectedEpisodeID]);

  useEffect(() => {
    if (selectedEpisodeID === undefined) return;

    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    previewCloseRef.current?.focus();

    const handleKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        if (isMetadataEditorOpen) {
          setIsMetadataEditorOpen(false);
        } else {
          closePreview();
        }
        return;
      }
      if (event.key !== "Tab" || !previewRef.current) return;
      const focusable = previewRef.current.querySelectorAll<HTMLElement>(
        'button:not([disabled]), [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => {
      document.body.style.overflow = previousOverflow;
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [closePreview, isMetadataEditorOpen, selectedEpisodeID]);

  const updateDecision = async (
    candidate: DiscoveryCandidate,
    state: TriageDecisionState,
  ) => {
    if (!onDecision || savingEpisodeID !== null) return;

    const previous = candidate;
    setDecisionError("");
    setSavingEpisodeID(candidate.episode_id);
    setDisplayCandidates((items) =>
      items.map((candidate) =>
        candidate.episode_id === previous.episode_id
          ? {
              ...candidate,
              decision_state: state,
              queue_state: state === "shortlisted" ? "inbox" : null,
              dismissed_at:
                state === "discarded" ? new Date().toISOString() : undefined,
            }
          : candidate,
      ),
    );

    try {
      const serverDecision = await onDecision(previous.episode_id, state);
      setDisplayCandidates((items) =>
        items.map((candidate) =>
          candidate.episode_id === previous.episode_id
            ? {
                ...candidate,
                decision_state: serverDecision.state,
                decision_updated_at: serverDecision.decision_updated_at,
              }
            : candidate,
        ),
      );
    } catch {
      setDisplayCandidates((items) =>
        items.map((candidate) =>
          candidate.episode_id === previous.episode_id ? previous : candidate,
        ),
      );
      setDecisionError(
        state === "shortlisted"
          ? "收集失败，已恢复服务端原状态，可重试。"
          : state === "pending" && previous.queue_state === "inbox"
            ? "移除失败，已恢复服务端原状态，可重试。"
            : "状态保存失败，已恢复服务端原状态，可重试。",
      );
    } finally {
      setSavingEpisodeID(null);
    }
  };

  const selectCandidateAt = (index: number) => {
    const candidate = displayCandidates[index];
    if (!candidate) return;
    const visibleIndex = visibleCandidates.findIndex(
      (item) => item.episode_id === candidate.episode_id,
    );
    if (visibleIndex >= 0) {
      dispatchRecentPagination({
        type: "set-page",
        page: Math.floor(visibleIndex / recentPageSize),
      });
    }
    setSelectedID(candidate.episode_id);
    if (!candidate.read_at && onRead) {
      const previousReadAt = candidate.read_at;
      const optimisticReadAt = new Date().toISOString();
      setDisplayCandidates((items) =>
        items.map((item) =>
          item.episode_id === candidate.episode_id
            ? { ...item, read_at: optimisticReadAt }
            : item,
        ),
      );
      void onRead(candidate.episode_id)
        .then((state) => {
          setDisplayCandidates((items) =>
            items.map((item) =>
              item.episode_id === candidate.episode_id
                ? { ...item, read_at: state.read_at }
                : item,
            ),
          );
        })
        .catch(() => {
          setDisplayCandidates((items) =>
            items.map((item) =>
              item.episode_id === candidate.episode_id
                ? { ...item, read_at: previousReadAt }
                : item,
            ),
          );
          setDecisionError("已打开单集，但未读状态未能保存，可稍后重试。");
        });
    }
  };

  const openMetadataEditor = () => {
    if (isMetadataEditorOpen) return;
    setIsMetadataEditorOpen(true);
  };

  const closeMetadataEditor = () => {
    if (!isMetadataEditorOpen) return;
    setIsMetadataEditorOpen(false);
  };

  const handleTouchStart = (event: TouchEvent<HTMLElement>) => {
    const touch = event.touches[0];
    if (!touch) return;
    const bounds = event.currentTarget.getBoundingClientRect();
    if (touch.clientX - bounds.left < 32 || bounds.right - touch.clientX < 32) {
      touchStartX.current = null;
      return;
    }
    touchStartX.current = touch.clientX;
  };

  const handleTouchEnd = (event: TouchEvent<HTMLElement>) => {
    const startX = touchStartX.current;
    touchStartX.current = null;
    const touch = event.changedTouches[0];
    if (startX === null || !touch) return;
    const distance = touch.clientX - startX;
    if (Math.abs(distance) < 56) return;
    selectCandidateAt(selectedIndex + (distance < 0 ? 1 : -1));
  };

  const discardActionLabel = selected?.dismissed_at ? "恢复显示" : "不感兴趣";
  const collectActionLabel =
    selected?.queue_state === "inbox"
      ? "从 Inbox 移除"
      : selected?.queue_state
        ? `已在 ${queueLabels[selected.queue_state]}`
        : "收集到 Inbox";

  return (
    <main className="discovery-desk discovery-unified-layout">
      <aside className="discovery-sidebar" aria-label="Discovery 导航与筛选">
        <div className="discovery-workbench-copy editorial-title-group">
          <h1 className="editorial-section-title">Discovery</h1>
          <span className="discovery-source-label">最近更新 · 14 天</span>
        </div>
        <div className="discovery-status-filters" aria-label="最近更新筛选">
          {(
            [
              ["all", "全部"],
              ["unread", "未读"],
              ["uncollected", "未收集"],
            ] as const
          ).map(([value, label]) => (
            <button
              key={value}
              type="button"
              className={activeFilter === value ? "is-active" : ""}
              aria-pressed={activeFilter === value}
              onClick={() => {
                setActiveFilter(value);
                dispatchRecentPagination({ type: "reset-page" });
              }}
            >
              <span>{label}</span>
              <strong>{filterCounts[value]}</strong>
            </button>
          ))}
        </div>
        <section
          className="discovery-focus-rail"
          aria-label="Focus 快捷区域"
        >
          {focusContent}
        </section>
      </aside>

      <section className="discovery-stream">
        {reportContent}
        {noticeContent}
        <section className="discovery-list-section" aria-label="工作流最近更新">
          <header className="discovery-section-heading">
            <div>
              <p className="discovery-kicker">RECENT UPDATES</p>
              <h2>最近更新</h2>
            </div>
            <span>按工作流同步时间 · 最近 14 天</span>
          </header>

          {decisionError && !selected && (
            <p className="discovery-decision-error" role="alert">
              {decisionError}
            </p>
          )}

          <div
            className="discovery-candidate-viewport"
            data-testid="discovery-candidate-viewport"
          >
            {displayCandidates.length === 0 ? (
              <div className="discovery-inline-empty" aria-live="polite">
                <h3>工作流暂时没有同步到新单集</h3>
                <p>工作流抓取到的新单集会按系统同步时间显示在这里。</p>
              </div>
            ) : visibleCandidates.length === 0 ? (
              <div className="discovery-inline-empty" aria-live="polite">
                <h3>当前筛选没有单集</h3>
                <p>切换到“全部”继续浏览最近 14 天更新。</p>
                <button
                  type="button"
                  onClick={() => {
                    setActiveFilter("all");
                    dispatchRecentPagination({ type: "reset-page" });
                  }}
                >
                  查看全部
                </button>
              </div>
            ) : (
              <ol
                className="discovery-candidate-list"
                data-testid="discovery-candidate-list"
              >
                {pagedCandidates.map((candidate, index) => {
                const isSelected =
                  candidate.episode_id === selected?.episode_id;
                const isInInbox = candidate.queue_state === "inbox";
                const hasProtectedQueue = Boolean(
                  candidate.queue_state && !isInInbox,
                );
                const queueActionLabel = isInInbox
                  ? "从 Inbox 移除"
                  : candidate.queue_state
                    ? `已在 ${queueLabels[candidate.queue_state]}`
                    : candidate.dismissed_at
                      ? "不感兴趣"
                      : "收集到 Inbox";
                const dateGroup = candidateDateGroup(candidate.candidate_time);
                const previousDateGroup =
                  index > 0
                    ? candidateDateGroup(
                        pagedCandidates[index - 1].candidate_time,
                      )
                    : "";

                return (
                  <Fragment key={candidate.episode_id}>
                    {dateGroup !== previousDateGroup && (
                      <li
                        className="discovery-date-group"
                        aria-label={`${dateGroup}更新`}
                      >
                        <span>{dateGroup}</span>
                      </li>
                    )}
                    <li>
                      <article
                        className="discovery-candidate-card"
                        data-selected={isSelected || undefined}
                      >
                        <button
                          ref={(node) => {
                            if (node) {
                              candidateButtonRefs.current.set(
                                candidate.episode_id,
                                node,
                              );
                            } else {
                              candidateButtonRefs.current.delete(
                                candidate.episode_id,
                              );
                            }
                          }}
                          type="button"
                          className="discovery-candidate"
                          aria-label={`预读 ${candidate.episode_title}`}
                          aria-haspopup="dialog"
                          aria-expanded={isSelected}
                          onClick={() =>
                            selectCandidateAt(
                              displayCandidates.findIndex(
                                (item) =>
                                  item.episode_id === candidate.episode_id,
                              ),
                            )
                          }
                        >
                          <span className="discovery-index">
                            <span>
                              {String(recentPageStart + index + 1).padStart(
                                2,
                                "0",
                              )}
                            </span>
                            {!candidate.read_at && (
                              <span
                                className="discovery-unread-dot"
                                aria-hidden="true"
                                title="未读"
                              />
                            )}
                          </span>
                          <CandidateCover
                            candidate={candidate}
                            className="discovery-list-cover"
                          />
                          <span className="discovery-candidate-copy">
                            <span className="discovery-meta-line">
                              <span>{candidate.podcast_title}</span>
                              <span>
                                {formatCandidateDate(candidate.candidate_time)}
                              </span>
                            </span>
                            <strong>{candidate.episode_title}</strong>
                            <span
                              className="discovery-candidate-excerpt"
                              data-testid={`candidate-excerpt-${candidate.episode_id}`}
                            >
                              {candidateExcerpt(candidate)}
                            </span>
                            <span className="discovery-candidate-details">
                              {formatCandidateEpisodeMeta(
                                candidate.episode_no,
                                candidate.duration,
                              )}
                            </span>
                          </span>
                        </button>
                        <span className="discovery-candidate-state">
                          <button
                            ref={(node) => {
                              if (node) {
                                candidateActionRefs.current.set(
                                  candidate.episode_id,
                                  node,
                                );
                              } else {
                                candidateActionRefs.current.delete(
                                  candidate.episode_id,
                                );
                              }
                            }}
                            type="button"
                            className="discovery-card-action"
                            data-state={
                              candidate.queue_state ??
                              (candidate.dismissed_at ? "discarded" : "pending")
                            }
                            aria-label={queueActionLabel}
                            aria-pressed={isInInbox}
                            aria-busy={
                              savingEpisodeID === candidate.episode_id ||
                              undefined
                            }
                            title={queueActionLabel}
                            disabled={
                              !onDecision ||
                              savingEpisodeID !== null ||
                              Boolean(candidate.dismissed_at) ||
                              hasProtectedQueue
                            }
                            onClick={() =>
                              void updateDecision(
                                candidate,
                                isInInbox ? "pending" : "shortlisted",
                              )
                            }
                          >
                            {candidate.dismissed_at ? (
                              <IconEyeOff aria-hidden="true" />
                            ) : candidate.queue_state ? (
                              <IconBookmarkFilled aria-hidden="true" />
                            ) : (
                              <IconBookmarkPlus aria-hidden="true" />
                            )}
                          </button>
                        </span>
                      </article>
                    </li>
                  </Fragment>
                );
                })}
              </ol>
            )}
          </div>

          <footer className="discovery-list-footer">
            <span>共 {String(visibleCandidates.length).padStart(2, "0")} 集</span>
            <div
              className="discovery-list-pagination"
              role="group"
              aria-label="最近更新翻页"
            >
              <button
                type="button"
                aria-label="上一页"
                title="上一页"
                disabled={safeRecentPage <= 0}
                onClick={() =>
                  dispatchRecentPagination({
                    type: "set-page",
                    page: Math.max(0, recentPage - 1),
                  })
                }
              >
                <IconChevronLeft aria-hidden="true" />
              </button>
              <span role="status" aria-live="polite" aria-atomic="true">
                <span aria-hidden="true">
                  {safeRecentPage + 1} / {recentPageCount}
                </span>
                <span className="sr-only">
                  当前筛选{recentFilterLabels[activeFilter]}，共
                  {visibleCandidates.length} 集，第 {safeRecentPage + 1} 页，共
                  {recentPageCount} 页，本页显示 {pagedCandidates.length} 集
                </span>
              </span>
              <button
                type="button"
                aria-label="下一页"
                title="下一页"
                disabled={safeRecentPage >= recentPageCount - 1}
                onClick={() =>
                  dispatchRecentPagination({
                    type: "set-page",
                    page: Math.min(recentPageCount - 1, recentPage + 1),
                  })
                }
              >
                <IconChevronRight aria-hidden="true" />
              </button>
            </div>
          </footer>
        </section>
      </section>

      {selected && (
        <div className="discovery-preview-overlay" role="presentation">
          <button
            type="button"
            className="discovery-preview-backdrop"
            aria-label="关闭单集预读"
            tabIndex={-1}
            onClick={closePreview}
          />
          <aside
            ref={previewRef}
            className="discovery-preview"
            data-editor-open={isMetadataEditorOpen}
            role="dialog"
            aria-modal="true"
            aria-labelledby="discovery-preview-title"
            data-testid="discovery-mobile-card"
            onTouchStart={handleTouchStart}
            onTouchEnd={handleTouchEnd}
          >
            <header className="discovery-preview-heading">
              <div className="discovery-preview-identity">
                <span>{selected.podcast_title}</span>
                <h2 id="discovery-preview-title">{selected.episode_title}</h2>
                <small>
                  {formatCandidateEpisodeMeta(
                    selected.episode_no,
                    selected.duration,
                  )}
                </small>
              </div>
              <div className="discovery-preview-heading-tools">
                <strong className="discovery-current-count">
                  {String(selectedIndex + 1).padStart(2, "0")} /{" "}
                  {String(displayCandidates.length).padStart(2, "0")}
                </strong>
                <div
                  className="discovery-quick-actions"
                  aria-label="单集快捷操作"
                >
                  {selectedOriginalPlan ? (
                    <a
                      className="discovery-action-button"
                      href={selectedOriginalPlan.openUrl}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label="打开节目页面"
                      data-tooltip="打开节目页面"
                      title="打开节目页面"
                      onClick={() =>
                        originalRecovery.activate(
                          selected.episode_id,
                          selectedOriginalPlan,
                        )
                      }
                    >
                      <IconExternalLink aria-hidden="true" stroke={1.8} />
                    </a>
                  ) : (
                    <button
                      type="button"
                      className="discovery-action-button"
                      aria-label="节目链接暂缺"
                      data-tooltip="节目链接暂缺"
                      title="节目链接暂缺"
                      disabled
                    >
                      <IconExternalLink aria-hidden="true" stroke={1.8} />
                    </button>
                  )}
                  <button
                    type="button"
                    className="discovery-action-button"
                    aria-label={discardActionLabel}
                    aria-pressed={Boolean(selected.dismissed_at)}
                    aria-busy={
                      savingEpisodeID === selected.episode_id || undefined
                    }
                    data-tooltip={discardActionLabel}
                    title={discardActionLabel}
                    disabled={
                      !onDecision ||
                      savingEpisodeID !== null ||
                      Boolean(selected.queue_state)
                    }
                    onClick={() =>
                      void updateDecision(
                        selected,
                        selected.dismissed_at ? "pending" : "discarded",
                      )
                    }
                  >
                    {selected.dismissed_at ? (
                      <IconEye aria-hidden="true" stroke={1.8} />
                    ) : (
                      <IconEyeOff aria-hidden="true" stroke={1.8} />
                    )}
                  </button>
                  <button
                    type="button"
                    className="discovery-action-button is-primary"
                    aria-label={collectActionLabel}
                    aria-pressed={selected.queue_state === "inbox"}
                    aria-busy={
                      savingEpisodeID === selected.episode_id || undefined
                    }
                    data-tooltip={collectActionLabel}
                    title={collectActionLabel}
                    disabled={
                      !onDecision ||
                      savingEpisodeID !== null ||
                      Boolean(
                        selected.queue_state &&
                          selected.queue_state !== "inbox",
                      )
                    }
                    onClick={() =>
                      void updateDecision(
                        selected,
                        selected.queue_state === "inbox"
                          ? "pending"
                          : "shortlisted",
                      )
                    }
                  >
                    {selected.queue_state ? (
                      <IconBookmarkFilled aria-hidden="true" />
                    ) : (
                      <IconBookmarkPlus aria-hidden="true" stroke={1.8} />
                    )}
                  </button>
                </div>
                <button
                  type="button"
                  className="discovery-edit-toggle"
                  aria-label={
                    isMetadataEditorOpen ? "收起编辑" : "编辑标签与备注"
                  }
                  aria-expanded={isMetadataEditorOpen}
                  aria-controls="discovery-metadata-editor"
                  title={isMetadataEditorOpen ? "收起编辑" : "编辑标签与备注"}
                  onClick={
                    isMetadataEditorOpen
                      ? closeMetadataEditor
                      : openMetadataEditor
                  }
                >
                  <IconPencil aria-hidden="true" stroke={1.8} />
                </button>
                <button
                  ref={previewCloseRef}
                  type="button"
                  className="discovery-preview-close"
                  aria-label="关闭单集预读"
                  title="关闭"
                  onClick={closePreview}
                >
                  <IconX aria-hidden="true" stroke={1.8} />
                </button>
              </div>
              <div className="discovery-mobile-progress">
                <button
                  type="button"
                  aria-label="上一项"
                  disabled={selectedIndex <= 0}
                  onClick={() => selectCandidateAt(selectedIndex - 1)}
                >
                  上一项
                </button>
                <strong>
                  {selectedIndex + 1} / {displayCandidates.length}
                </strong>
                <button
                  type="button"
                  aria-label="下一项"
                  disabled={selectedIndex >= displayCandidates.length - 1}
                  onClick={() => selectCandidateAt(selectedIndex + 1)}
                >
                  下一项
                </button>
              </div>
            </header>

            <div
              className={`discovery-preview-workarea ${
                isMetadataEditorOpen ? "is-editing" : ""
              }`}
            >
              <div className="discovery-preview-primary">
                {originalRecovery.plan &&
                  originalRecovery.activeKey === selected.episode_id && (
                    <OriginalEpisodeRecovery
                      copyError={originalRecovery.copyError}
                      onRetry={originalRecovery.retry}
                      onOpenApp={originalRecovery.openApp}
                      onCopy={() => void originalRecovery.copy()}
                      onDismiss={originalRecovery.dismiss}
                    />
                  )}
                <section
                  ref={showNotesPaneRef}
                  className="discovery-show-notes"
                  aria-label="Show Notes"
                  aria-busy={
                    selected.metadata_only &&
                    detailErrorEpisodeID !== selected.episode_id
                  }
                >
                  {decisionError && (
                    <p className="discovery-decision-error" role="alert">
                      {decisionError}
                    </p>
                  )}
                  {selected.metadata_only ? (
                    detailErrorEpisodeID === selected.episode_id ? (
                      <div className="discovery-show-notes-error" role="alert">
                        <p>
                          Show Notes 暂时无法加载，最近更新列表仍可继续使用。
                        </p>
                        <button
                          type="button"
                          onClick={() => {
                            setDetailErrorEpisodeID(null);
                            setDetailRetryVersion((version) => version + 1);
                          }}
                        >
                          重新加载 Show Notes
                        </button>
                      </div>
                    ) : (
                      <p className="discovery-show-notes-loading">
                        正在加载 Show Notes…
                      </p>
                    )
                  ) : selected.show_notes_status === "available" &&
                    selected.show_notes?.trim() ? (
                    <RichText
                      html={selected.show_notes}
                      className="discovery-show-notes-content"
                    />
                  ) : (
                    <p className="discovery-show-notes-empty">
                      暂无 Show Notes
                    </p>
                  )}
                </section>
              </div>
              {isMetadataEditorOpen ? (
                <DiscoveryMetadataEditor
                  episodeId={selected.episode_id}
                  podcastId={selected.podcast_id}
                  onClose={closeMetadataEditor}
                />
              ) : null}
            </div>
          </aside>
        </div>
      )}
    </main>
  );
}
