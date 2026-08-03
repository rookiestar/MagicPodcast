"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import type {
  DiscoveryCandidate,
  TodayShortlistData,
  TriageDecisionResponse,
  TriageDecisionState,
} from "@/types/discovery";

interface TodayShortlistProps {
  data: TodayShortlistData;
  error?: string;
  onDecision?: (
    episodeID: number,
    state: TriageDecisionState,
  ) => Promise<TriageDecisionResponse>;
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

export default function TodayShortlist({
  data,
  error,
  onDecision,
}: TodayShortlistProps) {
  const [items, setItems] = useState(data.candidates);
  const [removed, setRemoved] = useState<{
    candidate: DiscoveryCandidate;
    index: number;
  } | null>(null);
  const [saving, setSaving] = useState(false);
  const [decisionError, setDecisionError] = useState("");

  useEffect(() => {
    setItems(data.candidates);
  }, [data.candidates]);

  const removeCandidate = async (candidate: DiscoveryCandidate) => {
    if (!onDecision || saving) return;
    const index = items.findIndex(
      (item) => item.episode_id === candidate.episode_id,
    );
    if (index < 0) return;

    setSaving(true);
    setDecisionError("");
    setItems((current) =>
      current.filter((item) => item.episode_id !== candidate.episode_id),
    );
    try {
      await onDecision(candidate.episode_id, "pending");
      setRemoved({ candidate, index });
    } catch {
      setItems((current) => {
        const restored = [...current];
        restored.splice(index, 0, candidate);
        return restored;
      });
      setDecisionError("移出失败，今日备选未改变。");
    } finally {
      setSaving(false);
    }
  };

  const undoRemoval = async () => {
    if (!onDecision || !removed || saving) return;
    const snapshot = removed;
    setSaving(true);
    setDecisionError("");
    setItems((current) => {
      if (
        current.some(
          (item) => item.episode_id === snapshot.candidate.episode_id,
        )
      ) {
        return current;
      }
      const restored = [...current];
      restored.splice(snapshot.index, 0, snapshot.candidate);
      return restored;
    });
    setRemoved(null);
    try {
      await onDecision(snapshot.candidate.episode_id, "shortlisted");
    } catch {
      setItems((current) =>
        current.filter(
          (item) => item.episode_id !== snapshot.candidate.episode_id,
        ),
      );
      setRemoved(snapshot);
      setDecisionError("撤销失败，单集仍未加入今日备选。");
    } finally {
      setSaving(false);
    }
  };

  return (
    <main className="today-shortlist discovery-desk">
      <header className="today-shortlist-header discovery-workbench-header">
        <div className="today-shortlist-copy discovery-workbench-copy">
          <h1 className="editorial-section-title">今日备选</h1>
          <p className="discovery-workbench-description">留下来的单集</p>
          <div
            className="today-shortlist-meta"
            aria-label={`${data.date} ${data.timezone}`}
          >
            <span>{data.date}</span>
            <span>{data.timezone}</span>
          </div>
        </div>
        <div className="today-shortlist-actions discovery-workbench-actions">
          <div
            className="discovery-count"
            aria-label={`共 ${items.length} 集留存`}
          >
            <strong>{String(items.length).padStart(2, "0")}</strong>
            <span>集留存</span>
          </div>
          <Link className="discovery-today-link" href="/discovery">
            返回继续初筛
          </Link>
        </div>
      </header>

      {error ? (
        <section className="today-shortlist-state editorial-card" role="alert">
          <strong>{error}</strong>
          <p>已保留返回入口，可继续处理最近更新。</p>
        </section>
      ) : items.length === 0 ? (
        <section className="today-shortlist-state editorial-card">
          <strong>今日还没有备选</strong>
          <p>回到最近更新，保留真正想继续了解的单集。</p>
        </section>
      ) : (
        <section
          className="today-shortlist-list editorial-card"
          aria-label="今日备选列表"
        >
          {items.map((candidate, index) => {
            const summary = candidate.pre_reads.find(
              (preRead) => preRead.kind === "summary",
            );
            return (
              <article key={candidate.episode_id}>
                <span className="today-shortlist-index">
                  {String(index + 1).padStart(2, "0")}
                </span>
                <div className="today-shortlist-copy">
                  <p>
                    <b>{candidate.podcast_title}</b>
                    <span>{formatTime(candidate.candidate_time)}</span>
                  </p>
                  <h2>{candidate.episode_title}</h2>
                  <p>{summary?.content || "必要摘要暂缺，请核对原始信息。"}</p>
                </div>
                <button
                  type="button"
                  aria-label={`移出 ${candidate.episode_title}`}
                  disabled={!onDecision || saving}
                  onClick={() => void removeCandidate(candidate)}
                >
                  移出备选
                </button>
              </article>
            );
          })}
        </section>
      )}

      {removed && !error && (
        <section className="today-shortlist-undo" aria-live="polite">
          <span>已移出：{removed.candidate.episode_title}</span>
          <button
            type="button"
            disabled={!onDecision || saving}
            onClick={() => void undoRemoval()}
          >
            撤销最近移除
          </button>
        </section>
      )}
      {decisionError && (
        <p className="today-shortlist-error" role="alert">
          {decisionError}
        </p>
      )}
    </main>
  );
}
