"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import type { TouchEvent } from "react";
import Link from "next/link";
import PlainImage from "@/components/ui/PlainImage";
import { sanitizeRichTextHtml } from "@/lib/contentSanitizer";
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

const emptyPreReads: DiscoveryCandidate["pre_reads"] = [];

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
        <div className="discovery-workbench-copy">
          <div className="discovery-workbench-title-line">
            <p className="discovery-kicker">个人播客知识库</p>
            <span className="discovery-shelf-label">你的播客书架</span>
          </div>
          <h1>最近更新</h1>
          <p className="discovery-workbench-description">
            订阅更新、单集摘录、标签与备注，按原始内容留在同一处。
          </p>
          <p className="discovery-workbench-note">
            按发布时间陈列；日期缺失时，以更新时间补位。
          </p>
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

      <div className="discovery-workspace">
        <section className="discovery-list-section">
          <div className="discovery-section-heading">
            <h2>单集</h2>
            <span>最近更新在前</span>
          </div>
          <ol
            className="discovery-candidate-list"
            data-testid="discovery-candidate-list"
          >
            {displayCandidates.map((candidate, index) => {
              const isSelected = candidate.episode_id === selected.episode_id;
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
                        {candidate.episode_no || "单集"} ·{" "}
                        {formatDuration(candidate.duration)}
                      </span>
                    </span>
                    <span className="discovery-open-state">
                      {candidate.decision_state === "shortlisted"
                        ? "今日备选"
                        : candidate.decision_state === "discarded"
                          ? "已略过"
                          : isSelected
                            ? "当前单集"
                            : ""}
                    </span>
                  </button>
                </li>
              );
            })}
          </ol>
        </section>

        <aside
          className="discovery-preview"
          aria-live="polite"
          data-testid="discovery-mobile-card"
          onTouchStart={handleTouchStart}
          onTouchEnd={handleTouchEnd}
        >
          <div className="discovery-preview-heading">
            <div>
              <p className="discovery-kicker">内容摘录</p>
              <span>摘要、观点、关联与质疑</span>
            </div>
            <b>个人库</b>
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
            <div>
              <span>{selected.podcast_title}</span>
              <h2>{selected.episode_title}</h2>
              <p>
                {selected.episode_no || "单集"} ·{" "}
                {formatDuration(selected.duration)} ·{" "}
                {formatCandidateDate(selected.candidate_time)}
              </p>
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
                    {preRead.label}
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
                aria-label={`${selectedPreRead.label}预读`}
              >
                <header>
                  <div>
                    <span>{selectedPreRead.label}</span>
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
            <details className="discovery-original-evidence">
              <summary>节目原文</summary>
              <div className="discovery-preview-label">
                <span>Show Notes</span>
                <b>
                  {selected.time_basis === "published_date"
                    ? "按发布时间"
                    : "按更新时间"}
                </b>
              </div>
              {selected.show_notes_status === "available" ? (
                <div
                  className="discovery-show-notes"
                  dangerouslySetInnerHTML={{
                    __html: sanitizeRichTextHtml(selected.show_notes),
                  }}
                />
              ) : (
                <div className="discovery-degraded">
                  <strong>Show Notes 暂缺</strong>
                  <p>仍可浏览候选身份、时间和时长；缺失信息不会阻塞比较。</p>
                </div>
              )}
            </details>
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
                aria-pressed={selected.decision_state === "discarded"}
                disabled={!onDecision || savingDecision}
                onClick={() =>
                  void updateDecision(
                    selected.decision_state === "discarded"
                      ? "pending"
                      : "discarded",
                  )
                }
              >
                {selected.decision_state === "discarded" ? "恢复显示" : "略过"}
              </button>
              <button
                type="button"
                className="is-primary"
                aria-pressed={selected.decision_state === "shortlisted"}
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
                  ? "移出今日"
                  : "留到今天"}
              </button>
            </div>
            {decisionError && (
              <p className="discovery-decision-error" role="alert">
                {decisionError}
              </p>
            )}
          </div>

          <footer className="discovery-preview-footer">
            <span>个人播客库</span>
            {selected.original_url ? (
              <a href={selected.original_url} target="_blank" rel="noreferrer">
                节目页面
              </a>
            ) : (
              <span>原始链接暂缺</span>
            )}
          </footer>
        </aside>
      </div>
    </main>
  );
}
