"use client";

import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent,
} from "react";
import {
  IconBookmarkPlus,
  IconChevronLeft,
  IconChevronRight,
  IconChevronDown,
  IconHistory,
  IconX,
} from "@tabler/icons-react";
import MarkdownViewer from "@/components/workflows/MarkdownViewer";
import PlainImage from "@/components/ui/PlainImage";
import {
  fetchHomepageReportDetail,
  formatReportDay,
  formatReportDate,
  formatReportTime,
  reportDayKey,
  reportTypeClassName,
  reportTypeLabel,
  HOMEPAGE_HISTORY_METADATA_LIMIT,
} from "@/lib/discoveryReports";
import {
  buildWorkflowFilterOptions,
  filterReportsByWorkflowSelection,
  workflowOptionMatchesKeyword,
  type WorkflowFilterOption,
} from "@/lib/homepageReportWorkflowFilter";
import { sanitizeContentUrl } from "@/lib/imageSourcePolicy";
import type {
  DiscoveryConsumptionResponse,
  HomepageReport,
  HomepageReportEpisode,
  QueueState,
  TriageDecisionResponse,
  TriageDecisionState,
} from "@/types/discovery";

export interface WorkflowReportWorkbenchProps {
  todayReports: HomepageReport[];
  historyReports?: HomepageReport[];
  timezone?: string;
  onDecision?: (
    episodeID: number,
    state: TriageDecisionState,
  ) => Promise<TriageDecisionResponse>;
  decisionOverrides?: Record<number, TriageDecisionState>;
  consumptionOverrides?: Record<number, DiscoveryConsumptionResponse>;
  failed?: boolean;
  loading?: boolean;
  onRetry?: () => void;
}

const queueLabels: Record<QueueState, string> = {
  inbox: "Inbox",
  focus: "Focus",
  someday: "Someday",
  done: "Done",
};

function episodeMeta(episode: HomepageReportEpisode) {
  const parts: string[] = [];
  if (episode.episode_no) parts.push(`第 ${episode.episode_no} 期`);
  if (episode.duration && episode.duration > 0) {
    parts.push(`${Math.round(episode.duration / 60)} 分钟`);
  }
  return parts.join(" · ");
}

function episodeShowNotesPreview(episode: HomepageReportEpisode) {
  return (episode.context || episode.excerpt || "").trim();
}

function splitReportMarkdown(content: string, fallbackTitle: string) {
  const normalized = content.trimStart();
  const match = normalized.match(/^(#\s+[^\r\n]+)(?:\r?\n+|$)([\s\S]*)$/);

  if (match) {
    return {
      titleMarkdown: match[1],
      bodyMarkdown: match[2].trimStart(),
    };
  }

  return {
    titleMarkdown: fallbackTitle.trim() ? `# ${fallbackTitle.trim()}` : "",
    bodyMarkdown: content,
  };
}

function EpisodeCover({
  episode,
  className,
}: {
  episode: HomepageReportEpisode;
  className: string;
}) {
  const cover = episode.image_url || episode.podcast_cover_url;
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    setFailed(false);
  }, [cover]);

  if (!cover || failed) {
    return (
      <div className={`${className} discovery-cover-missing`}>
        <span>暂无封面</span>
      </div>
    );
  }
  return (
    <PlainImage
      src={cover}
      alt={`${episode.podcast_title}封面`}
      className={`${className} object-cover`}
      loading="lazy"
      onError={() => setFailed(true)}
    />
  );
}

export default function WorkflowReportWorkbench({
  todayReports,
  historyReports = [],
  timezone,
  onDecision,
  decisionOverrides,
  consumptionOverrides,
  failed = false,
  loading = false,
  onRetry,
}: WorkflowReportWorkbenchProps) {
  const [activeIndex, setActiveIndex] = useState(0);
  const [indexBeforeHistory, setIndexBeforeHistory] = useState(0);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historySelection, setHistorySelection] =
    useState<HomepageReport | null>(null);
  const [historyDayDetails, setHistoryDayDetails] = useState<
    Record<number, HomepageReport>
  >({});
  const [latestHistoryLoading, setLatestHistoryLoading] = useState(false);
  const [latestHistoryLoadError, setLatestHistoryLoadError] = useState("");
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyLoadError, setHistoryLoadError] = useState("");
  const [workflowFilterIds, setWorkflowFilterIds] = useState<Set<number>>(
    () => new Set(),
  );
  const [workflowFilterOpen, setWorkflowFilterOpen] = useState(false);
  const [workflowFilterKeyword, setWorkflowFilterKeyword] = useState("");
  const [expandedEpisodeIDs, setExpandedEpisodeIDs] = useState<Set<number>>(
    () => new Set(),
  );
  const [savingEpisodeID, setSavingEpisodeID] = useState<number | null>(null);
  const [decisionError, setDecisionError] = useState("");
  const [localDecisions, setLocalDecisions] = useState<
    Record<number, TriageDecisionState>
  >({});
  const [localQueues, setLocalQueues] = useState<
    Record<number, QueueState | null>
  >({});
  const previewRef = useRef<HTMLDivElement>(null);
  const historyTriggerRef = useRef<HTMLButtonElement>(null);

  const hasToday = todayReports.length > 0;
  const workflowFilterOptions = useMemo(
    () => buildWorkflowFilterOptions(historyReports),
    [historyReports],
  );
  // Data refresh: drop selections whose workflow left the loaded window,
  // keep the ones still present (#144). Derived during render so a refresh
  // never flashes the filtered-empty state before pruning lands; the prune
  // is also committed back to source state so a removed selection cannot
  // resurrect if a later refresh brings the workflow back into the window.
  const effectiveWorkflowFilterIds = useMemo(() => {
    if (workflowFilterIds.size === 0) return workflowFilterIds;
    const available = new Set(
      workflowFilterOptions.map((option) => option.workflowId),
    );
    const next = new Set(
      [...workflowFilterIds].filter((workflowId) => available.has(workflowId)),
    );
    return next.size === workflowFilterIds.size ? workflowFilterIds : next;
  }, [workflowFilterIds, workflowFilterOptions]);
  if (effectiveWorkflowFilterIds.size !== workflowFilterIds.size) {
    setWorkflowFilterIds(new Set(effectiveWorkflowFilterIds));
  }
  const filteredHistoryReports = useMemo(
    () =>
      filterReportsByWorkflowSelection(
        historyReports,
        effectiveWorkflowFilterIds,
      ),
    [historyReports, effectiveWorkflowFilterIds],
  );

  const latestHistoryDay = historyReports[0]
    ? reportDayKey(historyReports[0].completed_at, timezone)
    : "";
  const latestHistoryDayReports = latestHistoryDay
    ? historyReports.filter(
        (report) =>
          reportDayKey(report.completed_at, timezone) === latestHistoryDay,
      )
    : [];
  const defaultReports = hasToday
    ? todayReports
    : latestHistoryDayReports.map(
        (report) => historyDayDetails[report.id] ?? report,
      );

  const carouselReports = historySelection
    ? [historySelection]
    : defaultReports;
  const safeIndex = Math.min(
    Math.max(activeIndex, 0),
    Math.max(carouselReports.length - 1, 0),
  );
  const activeReport = carouselReports[safeIndex] ?? null;
  const canSwitch = !historySelection && defaultReports.length > 1;

  useEffect(() => {
    if (hasToday || historySelection || !activeReport) {
      setLatestHistoryLoading(false);
      setLatestHistoryLoadError("");
      return;
    }
    if (!activeReport.metadata_only && activeReport.content) {
      setLatestHistoryLoading(false);
      setLatestHistoryLoadError("");
      return;
    }

    let cancelled = false;
    setLatestHistoryLoading(true);
    setLatestHistoryLoadError("");
    void fetchHomepageReportDetail(activeReport.id)
      .then((report) => {
        if (!cancelled) {
          setHistoryDayDetails((current) => ({
            ...current,
            [report.id]: report,
          }));
        }
      })
      .catch(() => {
        if (!cancelled) {
          setLatestHistoryLoadError("最新报告正文加载失败，可从往期重新选择。");
        }
      })
      .finally(() => {
        if (!cancelled) setLatestHistoryLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [activeReport, hasToday, historySelection]);

  useEffect(() => {
    if (historySelection) return;
    setActiveIndex((current) =>
      Math.min(current, Math.max(defaultReports.length - 1, 0)),
    );
  }, [defaultReports.length, historySelection]);

  useEffect(() => {
    setExpandedEpisodeIDs(new Set());
    if (previewRef.current) previewRef.current.scrollTop = 0;
  }, [safeIndex, historySelection?.id]);

  const resolveDecision = useCallback(
    (episode: HomepageReportEpisode): TriageDecisionState => {
      if (decisionOverrides?.[episode.episode_id]) {
        return decisionOverrides[episode.episode_id];
      }
      if (localDecisions[episode.episode_id]) {
        return localDecisions[episode.episode_id];
      }
      return episode.decision_state || "pending";
    },
    [decisionOverrides, localDecisions],
  );

  const resolveQueue = useCallback(
    (episode: HomepageReportEpisode): QueueState | null => {
      if (episode.episode_id in localQueues) {
        return localQueues[episode.episode_id] ?? null;
      }
      const override = consumptionOverrides?.[episode.episode_id];
      if (override) return override.queue_state;
      if (episode.queue_state) return episode.queue_state;
      return resolveDecision(episode) === "shortlisted" ? "inbox" : null;
    },
    [consumptionOverrides, localQueues, resolveDecision],
  );

  const selectReport = (index: number) => {
    if (!canSwitch) return;
    const next = Math.min(Math.max(index, 0), defaultReports.length - 1);
    if (next === safeIndex) return;
    setActiveIndex(next);
  };

  const goPrev = () => selectReport(safeIndex - 1);
  const goNext = () => selectReport(safeIndex + 1);

  const handleWorkbenchKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (historyOpen || !canSwitch) return;
    if (event.key === "ArrowLeft") {
      event.preventDefault();
      goPrev();
    } else if (event.key === "ArrowRight") {
      event.preventDefault();
      goNext();
    }
  };

  const toggleExpand = (episodeID: number) => {
    setExpandedEpisodeIDs((current) => {
      const next = new Set(current);
      if (next.has(episodeID)) next.delete(episodeID);
      else next.add(episodeID);
      return next;
    });
  };

  const collectToInbox = async (
    event: MouseEvent<HTMLButtonElement>,
    episode: HomepageReportEpisode,
  ) => {
    event.preventDefault();
    event.stopPropagation();
    if (!onDecision || savingEpisodeID === episode.episode_id) return;

    const previousDecision = resolveDecision(episode);
    const previousQueue = resolveQueue(episode);
    if (previousQueue) return;
    setDecisionError("");
    setSavingEpisodeID(episode.episode_id);
    setLocalDecisions((map) => ({
      ...map,
      [episode.episode_id]: "shortlisted",
    }));
    setLocalQueues((map) => ({ ...map, [episode.episode_id]: "inbox" }));

    try {
      const result = await onDecision(episode.episode_id, "shortlisted");
      setLocalDecisions((map) => ({
        ...map,
        [episode.episode_id]: result.state,
      }));
    } catch {
      setLocalDecisions((map) => ({
        ...map,
        [episode.episode_id]: previousDecision,
      }));
      setLocalQueues((map) => ({
        ...map,
        [episode.episode_id]: previousQueue,
      }));
      setDecisionError("收集失败，已恢复原状态，可重试。");
    } finally {
      setSavingEpisodeID(null);
    }
  };

  const openHistory = () => {
    setIndexBeforeHistory(safeIndex);
    setHistoryOpen(true);
  };

  const closeHistory = useCallback(() => {
    setHistoryOpen(false);
    // Filter lifecycle on close (#144): keep selections for the page visit,
    // but collapse the panel and drop the transient keyword.
    setWorkflowFilterOpen(false);
    setWorkflowFilterKeyword("");
    // Restore focus to trigger after close (#94).
    requestAnimationFrame(() => {
      historyTriggerRef.current?.focus();
    });
  }, []);

  const toggleWorkflowFilterSelection = (
    workflowId: number,
    checked: boolean,
  ) => {
    setWorkflowFilterIds((current) => {
      const next = new Set(current);
      if (checked) next.add(workflowId);
      else next.delete(workflowId);
      return next;
    });
  };

  const clearWorkflowFilterSelection = () => {
    setWorkflowFilterIds(new Set());
  };

  const pickHistoryReport = async (report: HomepageReport) => {
    setHistoryLoadError("");
    // Picking a report closes the drawer through the same cleanup path as the
    // close button / Escape (#144): collapse the filter panel and drop the
    // transient keyword while keeping the selection.
    closeHistory();
    // Metadata-only history: fetch full body on demand (#95).
    if (report.metadata_only || !report.content) {
      setHistoryLoading(true);
      try {
        const full = await fetchHomepageReportDetail(report.id);
        setHistorySelection(full);
        setActiveIndex(0);
      } catch {
        setHistoryLoadError("往期报告加载失败，可重试选择。");
        setHistorySelection(null);
      } finally {
        setHistoryLoading(false);
      }
      return;
    }
    setHistorySelection(report);
    setActiveIndex(0);
  };

  const clearHistorySelection = () => {
    setHistorySelection(null);
    setHistoryLoadError("");
    setActiveIndex(
      Math.min(indexBeforeHistory, Math.max(defaultReports.length - 1, 0)),
    );
  };

  const hasAvailableReport = defaultReports.length > 0;

  if (loading && !hasAvailableReport && !failed) {
    return (
      <section
        className="workflow-report-workbench is-loading"
        aria-label="正在读取精选报告"
        aria-busy="true"
      >
        <p className="sr-only">正在后台读取精选报告，最近更新不受影响。</p>
      </section>
    );
  }

  if (failed && !hasAvailableReport) {
    return (
      <section
        className="workflow-report-workbench is-error"
        aria-label="精选报告"
        role="alert"
      >
        <div className="workflow-report-error">
          <span>
            <strong>精选报告暂时无法读取</strong>
            <small>最近更新仍可继续使用。</small>
          </span>
          {onRetry && (
            <button type="button" onClick={onRetry}>
              重新尝试
            </button>
          )}
        </div>
      </section>
    );
  }

  if (!activeReport) {
    return null;
  }

  const positionLabel = historySelection
    ? "往期"
    : !hasToday
      ? defaultReports.length > 1
        ? `最新往期 · ${safeIndex + 1} / ${defaultReports.length}`
        : "最新往期"
      : todayReports.length > 1
        ? `${safeIndex + 1} / ${todayReports.length}`
        : null;
  const hasEpisodes = activeReport.episodes.length > 0;
  const reportMarkdown = hasEpisodes
    ? splitReportMarkdown(activeReport.content || "", activeReport.title)
    : null;

  return (
    <section
      className="workflow-report-workbench"
      aria-label="精选报告"
      tabIndex={canSwitch && !historyOpen ? 0 : undefined}
      onKeyDown={handleWorkbenchKeyDown}
    >
      <header className="workflow-report-header">
        <div className="workflow-report-header-copy editorial-title-group">
          <p className="workflow-report-kicker">CURATED REPORTS</p>
          <h2 className="editorial-section-title">精选报告</h2>
        </div>
        <div className="workflow-report-header-actions">
          {historySelection && (
            <button
              type="button"
              className="workflow-report-back-today"
              onClick={clearHistorySelection}
            >
              {hasToday ? "回到今日" : "回到最新"}
            </button>
          )}
          <button
            ref={historyTriggerRef}
            type="button"
            className="workflow-report-history-trigger"
            onClick={openHistory}
            aria-haspopup="dialog"
            aria-expanded={historyOpen}
          >
            <IconHistory size={18} aria-hidden />
            往期
          </button>
          {canSwitch && (
            <div
              className="workflow-report-switchers"
              role="group"
              aria-label={hasToday ? "切换当天报告" : "切换最近历史报告"}
            >
              <button
                type="button"
                className="workflow-report-arrow"
                onClick={goPrev}
                disabled={safeIndex <= 0}
                aria-label="上一份报告"
              >
                <IconChevronLeft size={20} aria-hidden />
              </button>
              <button
                type="button"
                className="workflow-report-arrow"
                onClick={goNext}
                disabled={safeIndex >= defaultReports.length - 1}
                aria-label="下一份报告"
              >
                <IconChevronRight size={20} aria-hidden />
              </button>
            </div>
          )}
        </div>
      </header>
      <div className="workflow-report-meta-row">
        <span
          className={`workflow-report-type ${reportTypeClassName(
            activeReport.report_type,
          )}`}
        >
          {reportTypeLabel(activeReport.report_type)}
        </span>
        <span className="workflow-report-date">
          {formatReportDate(activeReport.completed_at, timezone)}
        </span>
        <span className="workflow-report-workflow">
          {activeReport.workflow_name}
        </span>
        {positionLabel && (
          <span className="workflow-report-position" aria-live="polite">
            {positionLabel}
          </span>
        )}
      </div>

      {decisionError && (
        <div className="workflow-report-inline-error" role="alert">
          {decisionError}
        </div>
      )}
      {historyLoadError && (
        <div className="workflow-report-inline-error" role="alert">
          {historyLoadError}
        </div>
      )}
      {latestHistoryLoadError && (
        <div className="workflow-report-inline-error" role="alert">
          {latestHistoryLoadError}
        </div>
      )}
      {latestHistoryLoading && (
        <p className="workflow-report-history-loading" role="status">
          正在加载报告正文…
        </p>
      )}
      {historyLoading && (
        <p className="workflow-report-history-loading" role="status">
          正在加载往期报告…
        </p>
      )}

      <div className="workflow-report-preview" ref={previewRef}>
        {reportMarkdown?.titleMarkdown && (
          <div className="workflow-report-title">
            <MarkdownViewer
              content={reportMarkdown.titleMarkdown}
              density="reading"
            />
          </div>
        )}

        {hasEpisodes && (
          <div className="workflow-report-episodes" aria-label="报告单集">
            {activeReport.episodes.map((episode) => {
              const expanded = expandedEpisodeIDs.has(episode.episode_id);
              const queue = resolveQueue(episode);
              const saving = savingEpisodeID === episode.episode_id;
              const collectLabel = queue
                ? `已在 ${queueLabels[queue]}`
                : "收集到 Inbox";
              const showNotesPreview = episodeShowNotesPreview(episode);
              const safeLink = sanitizeContentUrl(episode.link);

              return (
                <article
                  key={`${activeReport.id}-${episode.episode_id}-${episode.order}`}
                  className={`workflow-report-episode ${expanded ? "is-expanded" : ""}`}
                >
                  {/* #94: expand control and bookmark are siblings — never nested. */}
                  <div className="workflow-report-episode-row">
                    <button
                      type="button"
                      className="workflow-report-episode-toggle"
                      aria-expanded={expanded}
                      aria-controls={`report-ep-detail-${activeReport.id}-${episode.episode_id}`}
                      onClick={() => toggleExpand(episode.episode_id)}
                    >
                      <EpisodeCover
                        episode={episode}
                        className="workflow-report-episode-cover"
                      />
                      <div className="workflow-report-episode-copy">
                        <p className="workflow-report-episode-podcast">
                          {episode.podcast_title}
                        </p>
                        <h3 className="workflow-report-episode-title">
                          {episode.episode_title}
                        </h3>
                        {episodeMeta(episode) && (
                          <p className="workflow-report-episode-meta">
                            {episodeMeta(episode)}
                          </p>
                        )}
                      </div>
                      <IconChevronDown
                        size={18}
                        aria-hidden
                        className={`workflow-report-episode-chevron ${expanded ? "is-open" : ""}`}
                      />
                      <span className="sr-only">
                        {expanded ? "收起单集详情" : "展开单集详情"}
                      </span>
                    </button>
                    <button
                      type="button"
                      className={`workflow-report-shortlist ${queue ? `is-on is-${queue}` : ""}`}
                      aria-label={collectLabel}
                      aria-pressed={Boolean(queue)}
                      title={collectLabel}
                      disabled={saving || !onDecision || Boolean(queue)}
                      onClick={(event) => void collectToInbox(event, episode)}
                    >
                      <IconBookmarkPlus size={20} aria-hidden />
                    </button>
                  </div>
                  {expanded && (
                    <div
                      id={`report-ep-detail-${activeReport.id}-${episode.episode_id}`}
                      className="workflow-report-episode-detail"
                    >
                      {showNotesPreview && (
                        <p className="workflow-report-episode-show-notes">
                          <strong>Show Notes</strong>
                          {showNotesPreview}
                        </p>
                      )}
                      {safeLink && (
                        <a
                          href={safeLink}
                          target="_blank"
                          rel="noreferrer"
                          className="workflow-report-episode-link"
                        >
                          打开原单集
                        </a>
                      )}
                    </div>
                  )}
                </article>
              );
            })}
          </div>
        )}

        {(!hasEpisodes || reportMarkdown?.bodyMarkdown) && (
          <div
            className={`workflow-report-body ${
              hasEpisodes ? "is-continuation" : ""
            }`}
          >
            <MarkdownViewer
              content={
                hasEpisodes
                  ? reportMarkdown?.bodyMarkdown || ""
                  : activeReport.content || ""
              }
              density="report"
            />
          </div>
        )}
      </div>

      {historyOpen && (
        <HistoryDrawer
          reports={filteredHistoryReports}
          timezone={timezone}
          onClose={closeHistory}
          onSelect={(report) => {
            void pickHistoryReport(report);
          }}
          filter={{
            options: workflowFilterOptions,
            selectedIds: effectiveWorkflowFilterIds,
            keyword: workflowFilterKeyword,
            open: workflowFilterOpen,
            onToggleOpen: () => setWorkflowFilterOpen((open) => !open),
            onKeywordChange: setWorkflowFilterKeyword,
            onToggleSelection: toggleWorkflowFilterSelection,
            onClearSelection: clearWorkflowFilterSelection,
          }}
        />
      )}
    </section>
  );
}

/** Filter state and callbacks the history drawer renders (#144). */
interface HistoryDrawerFilter {
  options: WorkflowFilterOption[];
  selectedIds: ReadonlySet<number>;
  keyword: string;
  open: boolean;
  onToggleOpen: () => void;
  onKeywordChange: (keyword: string) => void;
  onToggleSelection: (workflowId: number, checked: boolean) => void;
  onClearSelection: () => void;
}

// Exported for component tests that exercise the drawer's rendering
// contract directly (e.g. the defensive empty-result state).
export function HistoryDrawer({
  reports,
  timezone,
  onClose,
  onSelect,
  filter,
}: {
  reports: HomepageReport[];
  timezone?: string;
  onClose: () => void;
  onSelect: (report: HomepageReport) => void;
  filter: HistoryDrawerFilter;
}) {
  const {
    options: filterOptions,
    selectedIds: filterSelectedIds,
    keyword: filterKeyword,
    open: filterOpen,
    onToggleOpen: onToggleFilterOpen,
    onKeywordChange: onFilterKeywordChange,
    onToggleSelection: onToggleFilterSelection,
    onClearSelection: onClearFilterSelection,
  } = filter;
  const titleId = useId();
  const filterPanelId = useId();
  const drawerRef = useRef<HTMLElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);

  const hasFilterSelection = filterSelectedIds.size > 0;
  const showFilterEntry = filterOptions.length >= 2;
  const visibleFilterOptions = useMemo(
    () =>
      filterOptions.filter((option) =>
        workflowOptionMatchesKeyword(option, filterKeyword),
      ),
    [filterOptions, filterKeyword],
  );

  const ordered = useMemo(
    () =>
      [...reports].sort(
        (a, b) =>
          new Date(b.completed_at).getTime() -
          new Date(a.completed_at).getTime(),
      ),
    [reports],
  );
  const groups = useMemo(() => {
    const result: Array<{
      key: string;
      label: string;
      reports: HomepageReport[];
    }> = [];

    for (const report of ordered) {
      const key = reportDayKey(report.completed_at, timezone);
      const previous = result[result.length - 1];
      if (previous?.key === key) {
        previous.reports.push(report);
        continue;
      }
      result.push({
        key,
        label: formatReportDay(report.completed_at, timezone),
        reports: [report],
      });
    }

    return result;
  }, [ordered, timezone]);

  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement | null;
    closeButtonRef.current?.focus();

    const onKey = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        onClose();
        return;
      }
      // Block background report arrow keys while drawer is open (#94).
      if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
        const target = event.target as Node | null;
        if (drawerRef.current && target && drawerRef.current.contains(target)) {
          return;
        }
        event.preventDefault();
        event.stopPropagation();
      }
      if (event.key !== "Tab" || !drawerRef.current) return;
      const focusable = drawerRef.current.querySelectorAll<HTMLElement>(
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

    window.addEventListener("keydown", onKey, true);
    return () => {
      window.removeEventListener("keydown", onKey, true);
      previouslyFocused?.focus?.();
    };
  }, [onClose]);

  return (
    <div className="workflow-report-history-overlay" role="presentation">
      <button
        type="button"
        className="workflow-report-history-backdrop"
        aria-label="关闭往期抽屉"
        onClick={onClose}
        tabIndex={-1}
      />
      <aside
        ref={drawerRef}
        className="workflow-report-history-drawer"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
      >
        <header className="workflow-report-history-header">
          <h2 id={titleId}>往期报告</h2>
          <button
            ref={closeButtonRef}
            type="button"
            className="editorial-modal-close"
            onClick={onClose}
            aria-label="关闭"
            title="关闭"
          >
            <IconX aria-hidden stroke={1.8} />
          </button>
        </header>
        {showFilterEntry && (
          <section
            className="workflow-report-history-filter"
            aria-label="工作流筛选"
          >
            <div className="workflow-report-history-filter-bar">
              <button
                type="button"
                className="workflow-report-history-filter-toggle"
                aria-expanded={filterOpen}
                aria-controls={filterPanelId}
                onClick={onToggleFilterOpen}
              >
                筛选工作流
                {hasFilterSelection && (
                  <span className="workflow-report-history-filter-badge">
                    已选 {filterSelectedIds.size}
                  </span>
                )}
                <IconChevronDown
                  size={16}
                  aria-hidden
                  className={`workflow-report-history-filter-chevron ${filterOpen ? "is-open" : ""}`}
                />
              </button>
              {hasFilterSelection && (
                <button
                  type="button"
                  className="workflow-report-history-filter-clear"
                  onClick={onClearFilterSelection}
                >
                  清除筛选
                </button>
              )}
            </div>
            {filterOpen && (
              <div
                id={filterPanelId}
                className="workflow-report-history-filter-panel"
              >
                <p className="workflow-report-history-filter-scope">
                  筛选最近 {HOMEPAGE_HISTORY_METADATA_LIMIT} 份报告
                </p>
                <input
                  type="search"
                  className="workflow-report-history-filter-search"
                  value={filterKeyword}
                  onChange={(event) =>
                    onFilterKeywordChange(event.target.value)
                  }
                  placeholder="搜索工作流"
                  aria-label="搜索工作流"
                />
                {visibleFilterOptions.length === 0 ? (
                  <p className="workflow-report-history-filter-empty">
                    没有匹配的工作流。
                  </p>
                ) : (
                  <ul className="workflow-report-history-filter-options">
                    {visibleFilterOptions.map((option) => (
                      <li key={option.workflowId}>
                        <label
                          className="workflow-report-history-filter-option"
                        >
                          <input
                            type="checkbox"
                            checked={filterSelectedIds.has(option.workflowId)}
                            onChange={(event) =>
                              onToggleFilterSelection(
                                option.workflowId,
                                event.target.checked,
                              )
                            }
                          />
                          <span className="workflow-report-history-filter-name">
                            {option.label}
                          </span>
                          <span className="workflow-report-history-filter-count">
                            {option.reportCount} 份
                          </span>
                        </label>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </section>
        )}
        {reports.length === 0 && hasFilterSelection ? (
          <div className="workflow-report-history-filter-noresults">
            <p>没有符合所选工作流的报告。</p>
            <button
              type="button"
              className="workflow-report-history-filter-clear"
              onClick={onClearFilterSelection}
            >
              清除筛选
            </button>
          </div>
        ) : groups.length === 0 ? (
          <p className="workflow-report-history-empty">暂无往期报告。</p>
        ) : (
          <div className="workflow-report-history-groups">
            {groups.map((group) => (
              <section
                key={group.key}
                className="workflow-report-history-group"
                aria-label={group.label}
              >
                <header className="workflow-report-history-day">
                  <h3>
                    <time dateTime={group.key}>{group.label}</time>
                  </h3>
                  <span>{group.reports.length} 份</span>
                </header>
                <ul className="workflow-report-history-list">
                  {group.reports.map((report) => (
                    <li key={report.id}>
                      <button
                        type="button"
                        className="workflow-report-history-item"
                        onClick={() => onSelect(report)}
                      >
                        <span className="workflow-report-history-item-main">
                          <span
                            className={`workflow-report-type ${reportTypeClassName(
                              report.report_type,
                            )}`}
                          >
                            {reportTypeLabel(report.report_type)}
                          </span>
                          <span className="workflow-report-history-name">
                            {report.workflow_name}
                          </span>
                        </span>
                        <span className="workflow-report-history-meta">
                          <time
                            className="workflow-report-history-date"
                            dateTime={report.completed_at}
                          >
                            {formatReportTime(report.completed_at, timezone)}
                          </time>
                          <span aria-hidden>·</span>
                          <span className="workflow-report-history-count">
                            {report.episode_count} 条单集
                          </span>
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              </section>
            ))}
          </div>
        )}
      </aside>
    </div>
  );
}
