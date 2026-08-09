"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent, PointerEvent, TouchEvent } from "react";
import {
  IconArrowRight,
  IconBookmarkMinus,
  IconBookmarkPlus,
  IconEye,
  IconEyeOff,
} from "@tabler/icons-react";
import Link from "next/link";
import PlainImage from "@/components/ui/PlainImage";
import { formatEpisodeNumber } from "@/lib/episodeDisplay";
import type {
  DiscoveryCandidate,
  DiscoveryPreReadKind,
  DiscoveryPreReadStatus,
  TriageDecisionResponse,
  TriageDecisionState,
} from "@/types/discovery";

interface DiscoveryDeskProps {
  candidates: DiscoveryCandidate[];
  onDecision?: (
    episodeID: number,
    state: TriageDecisionState,
  ) => Promise<TriageDecisionResponse>;
}

const preReadStatusLabels: Record<DiscoveryPreReadStatus, string> = {
  available: "可核对",
  pending: "尚未完成",
  insufficient: "证据不足",
  failed: "生成失败",
  missing: "信息缺失",
};

const preReadPresentation: Record<
  DiscoveryPreReadKind,
  { label: string; purpose: string }
> = {
  summary: {
    label: "摘要",
    purpose: "这一集讲了什么",
  },
  viewpoints: {
    label: "核心观点",
    purpose: "节目提出的核心主张",
  },
  relevant: {
    label: "与我相关",
    purpose: "与你的标签和备注有何关联",
  },
  challenge: {
    label: "证据边界",
    purpose: "证据缺口、适用边界与待核问题",
  },
};

const emptyPreReads: DiscoveryCandidate["pre_reads"] = [];

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
  const [selectedPreReadKind, setSelectedPreReadKind] =
    useState<DiscoveryPreReadKind>("summary");
  const [splitRatio, setSplitRatio] = useState(60);
  const workspaceRef = useRef<HTMLDivElement>(null);
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
  const selectedPreReads = selected?.pre_reads ?? emptyPreReads;
  const selectedPreRead = useMemo(
    () =>
      selectedPreReads.find(
        (preRead) => preRead.kind === selectedPreReadKind,
      ) ?? selectedPreReads[0],
    [selectedPreReads, selectedPreReadKind],
  );
  const selectedPreReadSources = selectedPreRead?.sources ?? [];

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
    selected.decision_state === "discarded" ? "恢复显示" : "略过";
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
    setSelectedPreReadKind("summary");
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
            <h2>单集</h2>
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
          aria-label="调整单集列表和单集预读宽度"
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
          aria-live="polite"
          data-testid="discovery-mobile-card"
          onTouchStart={handleTouchStart}
          onTouchEnd={handleTouchEnd}
        >
          <div className="discovery-preview-heading">
            <p className="discovery-kicker">单集预读</p>
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

          <div className="discovery-preview-identity">
            <CandidateCover
              candidate={selected}
              className="discovery-preview-cover"
            />
            <div className="discovery-preview-copy">
              <span className="discovery-preview-podcast">
                {selected.podcast_title}
              </span>
              <h2>{selected.episode_title}</h2>
              <div className="discovery-preview-meta">
                <p>
                  {formatCandidateEpisodeMeta(
                    selected.episode_no,
                    selected.duration,
                  )}{" "}·{" "}
                  {formatCandidateDate(selected.candidate_time)}
                </p>
                {selected.original_url ? (
                  <a
                    className="discovery-episode-link"
                    href={selected.original_url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    打开节目页面
                  </a>
                ) : (
                  <span className="discovery-episode-link is-unavailable">
                    节目链接暂缺
                  </span>
                )}
              </div>
            </div>
          </div>

          <div className="discovery-preview-body">
            {selectedPreReads.length > 0 ? (
              <div className="discovery-preread-tabs" aria-label="四类预读">
                {selectedPreReads.map((preRead) => (
                  <button
                    key={preRead.kind}
                    type="button"
                    aria-pressed={selectedPreRead?.kind === preRead.kind}
                    className={
                      preRead.kind === "relevant" ? "is-relevant" : undefined
                    }
                    onClick={() => setSelectedPreReadKind(preRead.kind)}
                  >
                    {preReadPresentation[preRead.kind].label}
                  </button>
                ))}
              </div>
            ) : (
              <div className="discovery-degraded">
                <strong>预读内容尚未就绪</strong>
                <p>原始信息仍在，留存状态不受影响。</p>
              </div>
            )}
            {selectedPreRead && (
              <section
                className={`discovery-preread-panel is-${selectedPreRead.kind}`}
                aria-label={`${preReadPresentation[selectedPreRead.kind].label}预读`}
              >
                <header>
                  <div>
                    <span>
                      {preReadPresentation[selectedPreRead.kind].purpose}
                    </span>
                    {selectedPreRead.relation_strength && (
                      <strong>{selectedPreRead.relation_strength}</strong>
                    )}
                  </div>
                  <b data-status={selectedPreRead.status}>
                    {preReadStatusLabels[selectedPreRead.status]}
                  </b>
                </header>
                <p>{selectedPreRead.content}</p>
                <footer>
                  <span>
                    {selectedPreRead.version} ·{" "}
                    {formatCandidateDate(selectedPreRead.generated_at)}
                  </span>
                  <span className="discovery-preread-sources">
                    {selectedPreReadSources.length > 0
                      ? selectedPreReadSources.map((source) =>
                          source.url ? (
                            <a
                              key={`${source.kind}-${source.label}`}
                              href={source.url}
                              target="_blank"
                              rel="noreferrer"
                            >
                              {source.label}
                            </a>
                          ) : (
                            <span key={`${source.kind}-${source.label}`}>
                              {source.label}
                            </span>
                          ),
                        )
                      : "暂无可核对来源"}
                  </span>
                </footer>
              </section>
            )}
          </div>

          <div className="discovery-decision-area">
            <p className="discovery-decision-status">
              留存：
              <strong>
                {selected.decision_state === "shortlisted"
                  ? "今日备选"
                  : selected.decision_state === "discarded"
                    ? "已略过"
                    : "尚未标记"}
              </strong>
            </p>
            <div className="discovery-decision-actions">
              <button
                type="button"
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
                  <IconEye aria-hidden="true" />
                ) : (
                  <IconEyeOff aria-hidden="true" />
                )}
              </button>
              <button
                type="button"
                className="is-primary"
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
                {selected.decision_state === "shortlisted"
                  ? <IconBookmarkMinus aria-hidden="true" />
                  : <IconBookmarkPlus aria-hidden="true" />}
              </button>
            </div>
            {decisionError && (
              <p className="discovery-decision-error" role="alert">
                {decisionError}
              </p>
            )}
          </div>

        </aside>
      </div>
    </main>
  );
}
