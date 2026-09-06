"use client";

import { useState } from "react";
import {
  IconAlertTriangle,
  IconBan,
  IconBrain,
  IconCheck,
  IconChevronDown,
  IconChevronUp,
  IconCircle,
  IconCircleCheck,
  IconInfoCircle,
  IconListCheck,
  IconMessage,
  IconPointFilled,
  IconSearch,
} from "@tabler/icons-react";
import type { EpisodeCopilotActivityCategory } from "@/types/episodeCopilot";
import type {
  CopilotRunState,
  RunActivity,
  StageProgress,
  StageStatus,
} from "./episodeCopilotRun";
import styles from "./EpisodeCopilotActivityCard.module.css";

interface EpisodeCopilotActivityCardProps {
  run: CopilotRunState;
  /** Ticking clock (ms) so elapsed times update without new events. */
  now: number;
  isSlow: boolean;
  profileName: string;
}

const stageStatusLabels: Record<StageStatus, string> = {
  pending: "待执行",
  running: "进行中",
  done: "完成",
  failed: "失败",
  cancelled: "已取消",
};

const categoryLabels: Record<
  EpisodeCopilotActivityCategory,
  string
> = {
  stage: "阶段",
  web_search: "公开搜索",
  reasoning: "推理摘要",
  plan: "计划",
  agent_message: "回答",
  turn: "执行",
  item: "活动",
};

const runningActivityLimit = 6;
const expandedActivityLimit = 50;

function formatDuration(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  if (totalSeconds < 60) return `${totalSeconds} 秒`;
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes} 分 ${seconds} 秒`;
}

function formatRelative(at: number | null, now: number): string {
  if (at === null) return "尚未收到新活动";
  const elapsedSeconds = Math.max(0, Math.floor((now - at) / 1000));
  if (elapsedSeconds < 5) return "刚刚";
  if (elapsedSeconds < 60) return `${elapsedSeconds} 秒前`;
  const minutes = Math.floor(elapsedSeconds / 60);
  return `${minutes} 分前`;
}

function StageIcon({ status }: { status: StageStatus }) {
  switch (status) {
    case "done":
      return (
        <IconCircleCheck
          size={15}
          stroke={1.8}
          aria-hidden="true"
          className={styles.stageIconDone}
        />
      );
    case "running":
      return (
        <IconPointFilled
          size={15}
          stroke={1.8}
          aria-hidden="true"
          className={styles.stageIconRunning}
        />
      );
    case "failed":
      return (
        <IconAlertTriangle
          size={15}
          stroke={1.8}
          aria-hidden="true"
          className={styles.stageIconFailed}
        />
      );
    case "cancelled":
      return (
        <IconBan
          size={15}
          stroke={1.8}
          aria-hidden="true"
          className={styles.stageIconCancelled}
        />
      );
    default:
      return (
        <IconCircle
          size={15}
          stroke={1.8}
          aria-hidden="true"
          className={styles.stageIconPending}
        />
      );
  }
}

function ActivityCategoryIcon({
  category,
}: {
  category: EpisodeCopilotActivityCategory;
}) {
  switch (category) {
    case "web_search":
      return <IconSearch size={14} stroke={1.8} aria-hidden="true" />;
    case "reasoning":
      return <IconBrain size={14} stroke={1.8} aria-hidden="true" />;
    case "plan":
      return <IconListCheck size={14} stroke={1.8} aria-hidden="true" />;
    case "agent_message":
      return <IconMessage size={14} stroke={1.8} aria-hidden="true" />;
    default:
      return <IconInfoCircle size={14} stroke={1.8} aria-hidden="true" />;
  }
}

function ActivityRow({
  activity,
  now,
}: {
  activity: RunActivity;
  now: number;
}) {
  return (
    <li className={styles.activityRow} data-state={activity.state}>
      <ActivityCategoryIcon category={activity.category} />
      <span className={styles.activityCategory}>
        {categoryLabels[activity.category]}
        {activity.state === "failed" ? "失败" : ""}
      </span>
      {activity.text ? (
        <span className={styles.activityText}>{activity.text}</span>
      ) : null}
      {activity.metadata?.candidate_count ? (
        <span className={styles.activityMeta}>
          候选 {activity.metadata.candidate_count}
        </span>
      ) : null}
      {activity.metadata?.candidate_domains ? (
        <span className={styles.activityMeta}>
          {activity.metadata.candidate_domains}
        </span>
      ) : null}
      <span className={styles.activityTime}>
        {formatRelative(activity.receivedAt, now)}
      </span>
    </li>
  );
}

function StageRow({
  stage,
  isCurrent,
  now,
}: {
  stage: StageProgress;
  isCurrent: boolean;
  now: number;
}) {
  return (
    <li
      className={styles.stageRow}
      data-status={stage.status}
      data-current={isCurrent || undefined}
    >
      <StageIcon status={stage.status} />
      <span className={styles.stageLabel}>{stage.label}</span>
      <span className={styles.stageStatus}>{stageStatusLabels[stage.status]}</span>
      {isCurrent && stage.startedAt !== null ? (
        <span className={styles.stageElapsed}>
          {formatDuration(now - stage.startedAt)}
        </span>
      ) : null}
    </li>
  );
}

export default function EpisodeCopilotActivityCard({
  run,
  now,
  isSlow,
  profileName,
}: EpisodeCopilotActivityCardProps) {
  // While running the card stays open; once finished it collapses to the
  // summary and the user can re-open this session's full activity.
  const [userExpanded, setUserExpanded] = useState<boolean | null>(null);
  const outcome = run.finish?.outcome ?? "running";
  const expanded =
    outcome === "running" ? true : (userExpanded ?? false);
  const currentIndex = run.stages.findIndex(
    (stage) => stage.status === "running",
  );
  const lastDoneIndex = run.stages.reduce(
    (last, stage, index) => (stage.status === "done" ? index : last),
    -1,
  );
  const summaryParts: string[] = [];
  if (outcome === "completed") {
    summaryParts.push(`档位 ${profileName || "默认"}`);
    if (run.finish?.firstContentMs !== undefined) {
      summaryParts.push(`首字 ${run.finish.firstContentMs}ms`);
    }
    if (run.finish?.totalMs !== undefined) {
      summaryParts.push(`总耗时 ${run.finish.totalMs}ms`);
    }
    if (run.degradedResearch) {
      summaryParts.push("公开检索降级，仅依据单集内部内容");
    } else if (run.verifiedSources !== null) {
      summaryParts.push(`已验证公开来源 ${run.verifiedSources} 个`);
    }
  }
  const visibleActivities = expanded
    ? run.activities.slice(-expandedActivityLimit)
    : run.activities.slice(-runningActivityLimit);
  const hiddenCount = run.activities.length - visibleActivities.length;

  const outcomeLabel =
    outcome === "running"
      ? "运行中"
      : outcome === "completed"
        ? "已完成"
        : outcome === "failed"
          ? "失败"
          : "已取消";

  return (
    <section
      className={styles.card}
      aria-label="助手执行活动"
      data-outcome={outcome}
    >
      <span className={styles.srOnly} role="status" aria-live="polite">
        {run.announcement ?? ""}
      </span>

      <header className={styles.cardHeader}>
        <span className={styles.kicker}>RUNTIME ACTIVITY</span>
        <span className={styles.outcomeLabel} data-outcome={outcome}>
          {outcome === "failed" ? (
            <IconAlertTriangle size={14} stroke={1.8} aria-hidden="true" />
          ) : outcome === "cancelled" ? (
            <IconBan size={14} stroke={1.8} aria-hidden="true" />
          ) : outcome === "completed" ? (
            <IconCircleCheck size={14} stroke={1.8} aria-hidden="true" />
          ) : (
            <IconPointFilled size={14} stroke={1.8} aria-hidden="true" />
          )}
          {outcomeLabel}
        </span>
        <span className={styles.totalWait}>
          已等待 {formatDuration(now - run.startedAt)}
        </span>
        {outcome !== "running" ? (
          <button
            type="button"
            className={styles.toggleButton}
            aria-expanded={expanded}
            onClick={() => setUserExpanded(!expanded)}
          >
            {expanded ? (
              <IconChevronUp size={15} stroke={1.8} aria-hidden="true" />
            ) : (
              <IconChevronDown size={15} stroke={1.8} aria-hidden="true" />
            )}
            {expanded ? "收起执行详情" : "展开执行详情"}
          </button>
        ) : null}
      </header>

      {outcome === "completed" ? (
        <p className={styles.summaryLine} data-testid="copilot-run-summary">
          {summaryParts.join(" · ")}
        </p>
      ) : null}
      {outcome === "failed" && run.finish?.errorMessage ? (
        <p className={styles.failureLine}>
          最后完成阶段：
          {lastDoneIndex >= 0 ? run.stages[lastDoneIndex].label : "无"}
          ；{run.finish.errorMessage}
          {run.finish.errorCode === "profile_unavailable"
            ? "；请更换其他档位后重新提问。"
            : ""}
        </p>
      ) : null}

      <ol className={styles.stageList}>
        {run.stages.map((stage, index) => (
          <StageRow
            key={stage.id}
            stage={stage}
            isCurrent={index === currentIndex}
            now={now}
          />
        ))}
      </ol>

      {outcome === "running" ? (
        <p className={styles.currentLine}>
          当前阶段：
          {currentIndex >= 0 ? run.stages[currentIndex].label : "等待开始"}
          {" · "}
          阶段已进行{" "}
          {currentIndex >= 0 && run.stages[currentIndex].startedAt !== null
            ? formatDuration(
                now - (run.stages[currentIndex].startedAt ?? now),
              )
            : "0 秒"}
          {" · "}
          最近活动 {formatRelative(run.lastActivityAt, now)}
        </p>
      ) : null}
      {outcome === "running" && isSlow ? (
        <p className={styles.slowLine}>响应较慢；单集仍可阅读，可随时取消。</p>
      ) : null}
      {run.mergedActivities > 0 ? (
        <p className={styles.mergedLine}>
          较早的 {run.mergedActivities + hiddenCount} 条活动已合并保留为计数。
        </p>
      ) : null}

      {expanded ? (
        <ul className={styles.activityList} data-testid="copilot-activities">
          {visibleActivities.map((activity) => (
            <ActivityRow key={activity.id} activity={activity} now={now} />
          ))}
        </ul>
      ) : null}
    </section>
  );
}
