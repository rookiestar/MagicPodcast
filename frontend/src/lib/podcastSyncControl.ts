import type {
  Podcast,
  PodcastHistorySyncStatus,
  PodcastHistorySyncTask,
} from "@/types";

// PodcastSyncControl 是节目详情页同步入口的展示模型：按钮文案、禁用态、
// 进度与失败原因都从持久任务推导，重复提交被禁用阻断（#462/#465）。
export interface PodcastSyncControl {
  status: PodcastHistorySyncStatus | "idle";
  /** 按钮文案：同步历史单集 / 同步单集 / 排队中… / 同步中… / 重试同步 / 继续同步 */
  label: string;
  /** 排队与运行中禁止重复提交 */
  disabled: boolean;
  /** 任务正在排队或运行 */
  inFlight: boolean;
  /** 真实处理数量；总量未知只显示已处理数量 */
  progressText: string | null;
  /** 失败原因（仅失败态） */
  errorMessage: string | null;
  /** 来源覆盖说明（如「源当前未提供可获取单集」） */
  sourceNote: string | null;
  onStart: () => void;
}

export function shouldPollPodcastSync(
  task:
    | Pick<PodcastHistorySyncTask, "status" | "next_retry_at">
    | null
    | undefined,
): boolean {
  return (
    !!task &&
    (isActiveStatus(task.status) ||
      ((task.status === "failed" || task.status === "partial") &&
        task.next_retry_at != null))
  );
}

function isActiveStatus(status: PodcastHistorySyncStatus): boolean {
  return status === "pending" || status === "queued" || status === "running";
}

function buildProgressText(task: PodcastHistorySyncTask): string | null {
  if (task.processed_count <= 0) {
    return null;
  }
  if (task.total_known != null && task.total_known > 0) {
    return `已处理 ${task.processed_count} / ${task.total_known}`;
  }
  return `已处理 ${task.processed_count} 集`;
}

// getPodcastSyncControl 从最新任务推导同步控件；task 为 null 表示从未同步。
// startError 是本次启动动作本身的失败（如限流），优先于任务错误展示。
export function getPodcastSyncControl(options: {
  podcast: Pick<Podcast, "episode_count">;
  task: PodcastHistorySyncTask | null | undefined;
  starting?: boolean;
  startError?: string | null;
  onStart: () => void;
}): PodcastSyncControl {
  const { podcast, task, starting = false, startError, onStart } = options;
  const hasEpisodes = (podcast.episode_count || 0) > 0;

  if (!task) {
    return {
      status: "idle",
      label: starting ? "同步中…" : hasEpisodes ? "同步单集" : "同步历史单集",
      disabled: starting,
      inFlight: false,
      progressText: null,
      errorMessage: startError ?? null,
      sourceNote: null,
      onStart,
    };
  }

  const progressText = buildProgressText(task);

  if (isActiveStatus(task.status) && task.status !== "pending") {
    const label =
      task.status === "running"
        ? "同步中…"
        : task.status === "queued"
          ? "排队中…"
          : "待同步…";
    return {
      status: task.status,
      label,
      disabled: true,
      inFlight: true,
      progressText,
      errorMessage: null,
      sourceNote: null,
      onStart,
    };
  }

  switch (task.status) {
    case "pending":
      return {
        status: "pending",
        label: starting ? "同步中…" : hasEpisodes ? "同步单集" : "同步历史单集",
        disabled: starting,
        inFlight: false,
        progressText,
        errorMessage: startError ?? null,
        sourceNote: "等待自动同步，也可立即手动同步",
        onStart,
      };
    case "failed":
      return {
        status: "failed",
        label: "重试同步",
        disabled: starting,
        inFlight: false,
        progressText,
        errorMessage: startError ?? (task.error_message || "同步失败，请重试"),
        sourceNote: task.source_note || null,
        onStart,
      };
    case "partial":
      return {
        status: "partial",
        label: "继续同步",
        disabled: starting,
        inFlight: false,
        progressText,
        errorMessage: startError ?? null,
        sourceNote: task.source_note || null,
        onStart,
      };
    case "completed":
      return {
        status: "completed",
        label: hasEpisodes ? "同步单集" : "同步历史单集",
        disabled: starting,
        inFlight: false,
        progressText: null,
        errorMessage: startError ?? null,
        sourceNote: task.source_note || null,
        onStart,
      };
    default:
      return {
        status: task.status,
        label: hasEpisodes ? "同步单集" : "同步历史单集",
        disabled: starting,
        inFlight: false,
        progressText,
        errorMessage: startError ?? null,
        sourceNote: task.source_note || null,
        onStart,
      };
  }
}
