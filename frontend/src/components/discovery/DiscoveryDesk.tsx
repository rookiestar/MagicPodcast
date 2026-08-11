"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent, PointerEvent, TouchEvent } from "react";
import {
  IconArrowRight,
  IconBookmarkMinus,
  IconBookmarkPlus,
  IconEye,
  IconEyeOff,
  IconExternalLink,
  IconPencil,
} from "@tabler/icons-react";
import dynamic from "next/dynamic";
import Link from "next/link";
import DiscoveryMetadataEditor from "@/components/discovery/DiscoveryMetadataEditor";
import PlainImage from "@/components/ui/PlainImage";
import { formatEpisodeNumber } from "@/lib/episodeDisplay";
import type {
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
  onDecision?: (
    episodeID: number,
    state: TriageDecisionState,
  ) => Promise<TriageDecisionResponse>;
}

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

function candidateExcerpt(candidate: DiscoveryCandidate) {
  const summary = candidate.pre_reads?.find(
    (preRead) => preRead.kind === "summary",
  )?.content;
  if (summary?.trim()) return summary.trim();

  const showNotesText = candidate.show_notes
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
  onDecision,
}: DiscoveryDeskProps) {
  const [displayCandidates, setDisplayCandidates] = useState(candidates);
  const [selectedID, setSelectedID] = useState(() => {
    if (typeof window !== "undefined") {
      const restoredID = window.history.state?.magicpodcastDiscoveryEpisodeID;
      if (
        typeof restoredID === "number" &&
        candidates.some((candidate) => candidate.episode_id === restoredID)
      ) {
        return restoredID;
      }
    }
    return candidates[0]?.episode_id;
  });
  const [savingDecision, setSavingDecision] = useState(false);
  const [decisionError, setDecisionError] = useState("");
  const [isMetadataEditorOpen, setIsMetadataEditorOpen] = useState(false);
  const [splitRatio, setSplitRatio] = useState(60);
  const workspaceRef = useRef<HTMLDivElement>(null);
  const showNotesPaneRef = useRef<HTMLElement>(null);
  const splitBeforeEditingRef = useRef(60);
  const touchStartX = useRef<number | null>(null);

  useEffect(() => {
    setDisplayCandidates(candidates);
    setSelectedID((currentID) =>
      candidates.some((candidate) => candidate.episode_id === currentID)
        ? currentID
        : candidates[0]?.episode_id,
    );
  }, [candidates]);

  const selected = useMemo(
    () =>
      displayCandidates.find(
        (candidate) => candidate.episode_id === selectedID,
      ) ?? displayCandidates[0],
    [displayCandidates, selectedID],
  );
  const selectedIndex = selected
    ? displayCandidates.findIndex(
        (candidate) => candidate.episode_id === selected.episode_id,
      )
    : -1;
  useEffect(() => {
    if (!selected || typeof window === "undefined") return;
    window.history.replaceState(
      {
        ...window.history.state,
        magicpodcastDiscoveryEpisodeID: selected.episode_id,
      },
      "",
    );
  }, [selected]);

  useEffect(() => {
    if (showNotesPaneRef.current) {
      showNotesPaneRef.current.scrollTop = 0;
    }
  }, [selected?.episode_id]);

  useEffect(() => {
    if (!isMetadataEditorOpen) return;

    const handleEscape = (event: globalThis.KeyboardEvent) => {
      if (event.key !== "Escape") return;
      setIsMetadataEditorOpen(false);
      setSplitRatio(splitBeforeEditingRef.current);
    };

    window.addEventListener("keydown", handleEscape);
    return () => window.removeEventListener("keydown", handleEscape);
  }, [isMetadataEditorOpen]);

  if (!selected) {
    return (
      <section className="discovery-empty" aria-live="polite">
        <p className="discovery-kicker">最近更新</p>
        <h1>个人库暂时没有新到单集</h1>
        <p>同步播客库后，最新单集会按可核对时间稳定显示在这里。</p>
      </section>
    );
  }

  const discardActionLabel =
    selected.decision_state === "discarded" ? "恢复显示" : "忽略";
  const shortlistActionLabel =
    selected.decision_state === "shortlisted"
      ? "移出今日备选"
      : "加入今日备选";

  const updateDecision = async (state: TriageDecisionState) => {
    if (!onDecision || savingDecision) return;

    const previous = selected;
    setDecisionError("");
    setSavingDecision(true);
    setDisplayCandidates((items) =>
      items.map((candidate) =>
        candidate.episode_id === selected.episode_id
          ? { ...candidate, decision_state: state }
          : candidate,
      ),
    );

    try {
      const serverDecision = await onDecision(selected.episode_id, state);
      setDisplayCandidates((items) =>
        items.map((candidate) =>
          candidate.episode_id === selected.episode_id
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
      setDecisionError("决定保存失败，已恢复服务端原状态。");
    } finally {
      setSavingDecision(false);
    }
  };

  const selectCandidateAt = (index: number) => {
    const candidate = displayCandidates[index];
    if (!candidate) return;
    setSelectedID(candidate.episode_id);
  };

  const openMetadataEditor = () => {
    if (isMetadataEditorOpen) return;
    splitBeforeEditingRef.current = splitRatio;
    setSplitRatio(Math.min(splitRatio, 48));
    setIsMetadataEditorOpen(true);
  };

  const closeMetadataEditor = () => {
    if (!isMetadataEditorOpen) return;
    setIsMetadataEditorOpen(false);
    setSplitRatio(splitBeforeEditingRef.current);
  };

  const setSplitFromClientX = (clientX: number) => {
    const bounds = workspaceRef.current?.getBoundingClientRect();
    if (!bounds || bounds.width <= 0) return;

    const availableWidth = bounds.width - 18;
    const minimumListRatio = Math.max(42, (320 / availableWidth) * 100);
    const maximumListRatio = Math.min(
      68,
      100 - (400 / availableWidth) * 100,
    );
    if (maximumListRatio < minimumListRatio) return;

    const nextRatio =
      ((clientX - bounds.left) / availableWidth) * 100;
    setSplitRatio(
      Math.round(
        Math.min(
          maximumListRatio,
          Math.max(minimumListRatio, nextRatio),
        ),
      ),
    );
  };

  const handleResizePointerDown = (
    event: PointerEvent<HTMLButtonElement>,
  ) => {
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    setSplitFromClientX(event.clientX);
  };

  const handleResizePointerMove = (
    event: PointerEvent<HTMLButtonElement>,
  ) => {
    if (!event.currentTarget.hasPointerCapture(event.pointerId)) return;
    setSplitFromClientX(event.clientX);
  };

  const handleResizePointerUp = (
    event: PointerEvent<HTMLButtonElement>,
  ) => {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  };

  const handleResizeKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
  ) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    setSplitRatio((current) =>
      Math.min(
        68,
        Math.max(42, current + (event.key === "ArrowLeft" ? -3 : 3)),
      ),
    );
  };

  const handleTouchStart = (event: TouchEvent<HTMLElement>) => {
    const touch = event.touches[0];
    if (!touch) return;
    const bounds = event.currentTarget.getBoundingClientRect();
    if (
      touch.clientX - bounds.left < 32 ||
      bounds.right - touch.clientX < 32
    ) {
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

  return (
    <main className="discovery-desk">
      <section
        className="discovery-workbench-header"
        aria-label="个人库最近更新"
      >
        <div className="discovery-workbench-copy editorial-title-group">
          <h1 className="editorial-section-title">最近更新</h1>
        </div>
        <div className="discovery-workbench-actions">
          <div
            className="discovery-count"
            aria-label={`共 ${candidates.length} 项`}
          >
            <strong>{String(candidates.length).padStart(2, "0")}</strong>
            <span>集新到</span>
          </div>
          <Link className="discovery-today-link" href="/discovery/today">
            今日备选
          </Link>
        </div>
      </section>

      <div
        ref={workspaceRef}
        className="discovery-workspace"
        data-editor-open={isMetadataEditorOpen}
        style={{
          gridTemplateColumns: `minmax(320px, ${splitRatio}fr) 18px minmax(400px, ${100 - splitRatio}fr)`,
        }}
      >
        <section
          className={`discovery-list-section ${
            displayCandidates.length >= 4 ? "is-filled" : "is-sparse"
          }`}
        >
          <div className="discovery-section-heading">
            <h2>Episodes</h2>
            <span>最近更新在前</span>
          </div>
          <ol
            className="discovery-candidate-list"
            data-testid="discovery-candidate-list"
            style={
              displayCandidates.length >= 4
                ? {
                    gridTemplateRows: `repeat(${displayCandidates.length}, minmax(124px, 1fr))`,
                  }
                : undefined
            }
          >
            {displayCandidates.map((candidate, index) => {
              const isSelected = candidate.episode_id === selected.episode_id;
              const visualState = isSelected
                ? "current"
                : candidate.decision_state === "shortlisted"
                  ? "shortlisted"
                  : candidate.decision_state === "discarded"
                    ? "discarded"
                    : undefined;
              const visualStateLabel =
                visualState === "current"
                  ? "当前单集"
                  : visualState === "shortlisted"
                    ? "今日备选"
                    : visualState === "discarded"
                      ? "已略过"
                      : undefined;

              return (
                <li key={candidate.episode_id}>
                  <button
                    type="button"
                    className="discovery-candidate"
                    aria-label={`查看 ${candidate.episode_title}`}
                    aria-pressed={isSelected}
                    onClick={() => selectCandidateAt(index)}
                  >
                    <span className="discovery-index">
                      {String(index + 1).padStart(2, "0")}
                    </span>
                    <CandidateCover
                      candidate={candidate}
                      className="discovery-list-cover"
                    />
                    <span className="discovery-candidate-copy">
                      <span className="discovery-meta-line">
                        <span>{candidate.podcast_title}</span>
                        <span>{formatCandidateDate(candidate.candidate_time)}</span>
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
                    <span
                      className="discovery-open-state"
                      data-state={visualState}
                      title={visualStateLabel}
                    >
                      {visualState === "current" ? (
                        <IconArrowRight aria-hidden="true" />
                      ) : visualState === "shortlisted" ? (
                        <IconBookmarkPlus aria-hidden="true" />
                      ) : visualState === "discarded" ? (
                        <IconEyeOff aria-hidden="true" />
                      ) : null}
                      {visualStateLabel ? (
                        <span className="sr-only">{visualStateLabel}</span>
                      ) : null}
                    </span>
                  </button>
                </li>
              );
            })}
          </ol>
          <footer className="discovery-list-footer">
            <span>
              本轮 {String(displayCandidates.length).padStart(2, "0")} 集
            </span>
            <span>最近更新已到底</span>
          </footer>
        </section>

        <button
          type="button"
          role="separator"
          aria-label="调整 Episodes 列表与 Quick Actions 区域宽度"
          aria-orientation="vertical"
          aria-valuemin={42}
          aria-valuemax={68}
          aria-valuenow={splitRatio}
          className="discovery-column-resizer"
          title="拖动或使用方向键调整宽度"
          onPointerDown={handleResizePointerDown}
          onPointerMove={handleResizePointerMove}
          onPointerUp={handleResizePointerUp}
          onPointerCancel={handleResizePointerUp}
          onKeyDown={handleResizeKeyDown}
        >
          <span aria-hidden="true" />
        </button>

        <aside
          className="discovery-preview"
          data-editor-open={isMetadataEditorOpen}
          aria-live="polite"
          data-testid="discovery-mobile-card"
          onTouchStart={handleTouchStart}
          onTouchEnd={handleTouchEnd}
        >
          <div className="discovery-preview-heading">
            <h2>Quick Actions</h2>
            <div className="discovery-preview-heading-tools">
              <div className="discovery-quick-actions" aria-label="单集快捷操作">
                {selected.original_url ? (
                  <a
                    className="discovery-action-button"
                    href={selected.original_url}
                    target="_blank"
                    rel="noreferrer"
                    aria-label="打开节目页面"
                    data-tooltip="打开节目页面"
                    title="打开节目页面"
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
                  aria-pressed={selected.decision_state === "discarded"}
                  data-tooltip={discardActionLabel}
                  title={discardActionLabel}
                  disabled={!onDecision || savingDecision}
                  onClick={() =>
                    void updateDecision(
                      selected.decision_state === "discarded"
                        ? "pending"
                        : "discarded",
                    )
                  }
                >
                  {selected.decision_state === "discarded" ? (
                    <IconEye aria-hidden="true" stroke={1.8} />
                  ) : (
                    <IconEyeOff aria-hidden="true" stroke={1.8} />
                  )}
                </button>
                <button
                  type="button"
                  className="discovery-action-button is-primary"
                  aria-label={shortlistActionLabel}
                  aria-pressed={selected.decision_state === "shortlisted"}
                  data-tooltip={shortlistActionLabel}
                  title={shortlistActionLabel}
                  disabled={!onDecision || savingDecision}
                  onClick={() =>
                    void updateDecision(
                      selected.decision_state === "shortlisted"
                        ? "pending"
                        : "shortlisted",
                    )
                  }
                >
                  {selected.decision_state === "shortlisted" ? (
                    <IconBookmarkMinus aria-hidden="true" stroke={1.8} />
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
              <strong className="discovery-current-count">
                {String(selectedIndex + 1).padStart(2, "0")} /{" "}
                {String(displayCandidates.length).padStart(2, "0")}
              </strong>
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
          </div>

          <div
            className={`discovery-preview-workarea ${
              isMetadataEditorOpen ? "is-editing" : ""
            }`}
          >
            <section
              ref={showNotesPaneRef}
              className="discovery-show-notes"
              aria-label="Show Notes"
            >
              {decisionError && (
                <p className="discovery-decision-error" role="alert">
                  {decisionError}
                </p>
              )}
              {selected.show_notes_status === "available" &&
              selected.show_notes.trim() ? (
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
    </main>
  );
}
