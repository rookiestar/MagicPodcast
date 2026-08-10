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
  IconBookmarkMinus,
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
  formatReportDate,
  reportTypeLabel,
} from "@/lib/discoveryReports";
import { sanitizeContentUrl } from "@/lib/imageSourcePolicy";
import type {
  HomepageReport,
  HomepageReportEpisode,
  TriageDecisionResponse,
  TriageDecisionState,
} from "@/types/discovery";

export interface WorkflowReportWorkbenchProps {
  todayReports: HomepageReport[];
  historyReports?: HomepageReport[];
  onDecision?: (
    episodeID: number,
    state: TriageDecisionState,
  ) => Promise<TriageDecisionResponse>;
  decisionOverrides?: Record<number, TriageDecisionState>;
  failed?: boolean;
  loading?: boolean;
  onRetry?: () => void;
}

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
  const match = normalized.match(
    /^(#\s+[^\r\n]+)(?:\r?\n+|$)([\s\S]*)$/,
  );

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
  onDecision,
  decisionOverrides,
  failed = false,
  loading = false,
  onRetry,
}: WorkflowReportWorkbenchProps) {
  const [activeIndex, setActiveIndex] = useState(0);
  const [indexBeforeHistory, setIndexBeforeHistory] = useState(0);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historySelection, setHistorySelection] =
    useState<HomepageReport | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyLoadError, setHistoryLoadError] = useState("");
  const [expandedEpisodeIDs, setExpandedEpisodeIDs] = useState<Set<number>>(
    () => new Set(),
  );
  const [savingEpisodeID, setSavingEpisodeID] = useState<number | null>(null);
  const [decisionError, setDecisionError] = useState("");
  const [localDecisions, setLocalDecisions] = useState<
    Record<number, TriageDecisionState>
  >({});
  const previewRef = useRef<HTMLDivElement>(null);
  const historyTriggerRef = useRef<HTMLButtonElement>(null);

  // #94: no today report => hide entire region (even if history exists).
  const hasToday = todayReports.length > 0;

  const carouselReports = historySelection ? [historySelection] : todayReports;
  const safeIndex = Math.min(
    Math.max(activeIndex, 0),
    Math.max(carouselReports.length - 1, 0),
  );
  const activeReport = carouselReports[safeIndex] ?? null;
  const canSwitch = !historySelection && todayReports.length > 1;

  useEffect(() => {
    if (!hasToday) return;
    setActiveIndex((current) =>
      Math.min(current, Math.max(todayReports.length - 1, 0)),
    );
  }, [todayReports, hasToday]);

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

  const selectReport = (index: number) => {
    if (!canSwitch) return;
    const next = Math.min(Math.max(index, 0), todayReports.length - 1);
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

  const toggleShortlist = async (
    event: MouseEvent<HTMLButtonElement>,
    episode: HomepageReportEpisode,
  ) => {
    event.preventDefault();
    event.stopPropagation();
    if (!onDecision || savingEpisodeID === episode.episode_id) return;

    const current = resolveDecision(episode);
    const nextState: TriageDecisionState =
      current === "shortlisted" ? "pending" : "shortlisted";
    const previous = current;
    setDecisionError("");
    setSavingEpisodeID(episode.episode_id);
    setLocalDecisions((map) => ({ ...map, [episode.episode_id]: nextState }));

    try {
      const result = await onDecision(episode.episode_id, nextState);
      setLocalDecisions((map) => ({
        ...map,
        [episode.episode_id]: result.state,
      }));
    } catch {
      setLocalDecisions((map) => ({
        ...map,
        [episode.episode_id]: previous,
      }));
      setDecisionError("备选操作失败，已恢复原状态，可重试。");
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
    // Restore focus to trigger after close (#94).
    requestAnimationFrame(() => {
      historyTriggerRef.current?.focus();
    });
  }, []);

  const pickHistoryReport = async (report: HomepageReport) => {
    setHistoryLoadError("");
    setHistoryOpen(false);
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
    // Restore day-index from before opening history (#94).
    setActiveIndex(
      Math.min(indexBeforeHistory, Math.max(todayReports.length - 1, 0)),
    );
  };

  if (loading && todayReports.length === 0 && !failed) {
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

  if (failed && todayReports.length === 0) {
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

  // #94: hide entirely when there is no valid today report — even if history exists.
  if (!hasToday || !activeReport) {
    return null;
  }

  const positionLabel = historySelection
    ? "往期"
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
          <h2 className="editorial-section-title">精选报告</h2>
          <div className="workflow-report-meta-row">
            <span
              className={`workflow-report-type ${
                activeReport.report_type === "weekly"
                  ? "is-weekly"
                  : "is-daily"
              }`}
            >
              {reportTypeLabel(activeReport.report_type)}
            </span>
            <span className="workflow-report-date">
              {formatReportDate(activeReport.completed_at)}
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
        </div>
        <div className="workflow-report-header-actions">
          {historySelection && (
            <button
              type="button"
              className="workflow-report-back-today"
              onClick={clearHistorySelection}
            >
              回到今日
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
              aria-label="切换当天报告"
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
                disabled={safeIndex >= todayReports.length - 1}
                aria-label="下一份报告"
              >
                <IconChevronRight size={20} aria-hidden />
              </button>
            </div>
          )}
        </div>
      </header>

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
              const decision = resolveDecision(episode);
              const shortlisted = decision === "shortlisted";
              const saving = savingEpisodeID === episode.episode_id;
              const shortlistLabel = shortlisted
                ? "移出今日备选"
                : "加入今日备选";
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
                      className={`workflow-report-shortlist ${shortlisted ? "is-on" : ""}`}
                      aria-label={shortlistLabel}
                      aria-pressed={shortlisted}
                      title={shortlistLabel}
                      disabled={saving || !onDecision}
                      onClick={(event) => void toggleShortlist(event, episode)}
                    >
                      {shortlisted ? (
                        <IconBookmarkMinus size={20} aria-hidden />
                      ) : (
                        <IconBookmarkPlus size={20} aria-hidden />
                      )}
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
              density="reading"
            />
          </div>
        )}
      </div>

      {historyOpen && (
        <HistoryDrawer
          reports={historyReports}
          onClose={closeHistory}
          onSelect={(report) => {
            void pickHistoryReport(report);
          }}
        />
      )}
    </section>
  );
}

function HistoryDrawer({
  reports,
  onClose,
  onSelect,
}: {
  reports: HomepageReport[];
  onClose: () => void;
  onSelect: (report: HomepageReport) => void;
}) {
  const titleId = useId();
  const drawerRef = useRef<HTMLElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);

  const ordered = useMemo(
    () =>
      [...reports].sort(
        (a, b) =>
          new Date(b.completed_at).getTime() -
          new Date(a.completed_at).getTime(),
      ),
    [reports],
  );

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
            onClick={onClose}
            aria-label="关闭"
          >
            <IconX size={18} aria-hidden />
          </button>
        </header>
        {ordered.length === 0 ? (
          <p className="workflow-report-history-empty">暂无往期报告。</p>
        ) : (
          <ul className="workflow-report-history-list">
            {ordered.map((report) => (
              <li key={report.id}>
                <button
                  type="button"
                  className="workflow-report-history-item"
                  onClick={() => onSelect(report)}
                >
                  <span
                    className={`workflow-report-type ${
                      report.report_type === "weekly"
                        ? "is-weekly"
                        : "is-daily"
                    }`}
                  >
                    {reportTypeLabel(report.report_type)}
                  </span>
                  <span className="workflow-report-history-name">
                    {report.workflow_name}
                  </span>
                  <span className="workflow-report-history-date">
                    {formatReportDate(report.completed_at)}
                  </span>
                  <span className="workflow-report-history-count">
                    {report.episode_count} 条单集
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </aside>
    </div>
  );
}
