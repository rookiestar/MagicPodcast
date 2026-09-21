import { api, handleResponse } from "./client";
import type { ApiResponse, PodcastHistorySyncTask } from "@/types";
import { isOperationCompletionEvent } from "@/lib/syncOperationMessages";
import { sseFormDataRequest, sseRequest } from "@/lib/sseClient";

const TEN_MINUTES = 10 * 60 * 1000;

type SyncProgressCallback = (
  type: string,
  message: string,
  current?: number,
  total?: number,
  data?: any,
) => void;

function pickSummaryData(data: any) {
  return {
    operation: data.operation,
    total_podcasts: data.total_podcasts,
    success_podcasts: data.success_podcasts,
    failed_podcasts: data.failed_podcasts,
    skipped_podcasts: data.skipped_podcasts,
    no_update_podcasts: data.no_update_podcasts,
    stub_podcasts: data.stub_podcasts,
    merged_podcasts: data.merged_podcasts,
    conflict_podcasts: data.conflict_podcasts,
    unchanged_podcasts: data.unchanged_podcasts,
    total_episodes: data.total_episodes,
    new_episodes: data.new_episodes,
    updated_episodes: data.updated_episodes,
    duration: data.duration,
  };
}

function isSummaryComplete(data: { type?: string; message?: string }) {
  return isOperationCompletionEvent("sync", data.type || "", data.message || "");
}

export const syncApi = {
  importOPMLSSE: async (
    file: File,
    onProgress: SyncProgressCallback,
    confirmationText: string,
    decisions?: Record<string, string>,
  ): Promise<void> => {
    const formData = new FormData();
    formData.append("opml_file", file);
    // 用户对需确认条目（清单关联/已删除恢复/换址合并）的显式决定。
    if (decisions && Object.keys(decisions).length > 0) {
      formData.append("decisions", JSON.stringify(decisions));
    }

    return sseFormDataRequest(
      "/api/v1/sync/import-sse",
      formData,
      (type, message, current, total, data) => {
        const dataToPass = type === "summary" ? pickSummaryData(data) : data;
        onProgress(type, message, current, total, dataToPass);
      },
      {
        headers: { "X-MagicPodcast-Confirmation": confirmationText },
        timeout: TEN_MINUTES,
        logPrefix: "[Import]",
        emptyMessage: "未收到任何导入消息",
        abortMessage: "导入被取消",
        timeoutMessage: "导入超时（10分钟）",
        requireCompletion: true,
        incompleteMessage: "导入连接提前结束，未收到完成确认；已建立的任务可按任务编号查询",
        isComplete: isSummaryComplete,
      },
    );
  },

  syncPodcastsMetadataSSE: async (
    onProgress: SyncProgressCallback,
    confirmationText: string,
  ): Promise<void> => {
    return sseRequest(
      {
        endpoint: "/api/v1/sync/podcasts/metadata-sse",
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ confirmation_text: confirmationText }),
        timeout: TEN_MINUTES,
        logPrefix: "[Sync Metadata]",
        emptyMessage: "未收到任何同步消息",
        abortMessage: "同步被取消",
        timeoutMessage: "同步超时（10分钟）",
        completeOnTypeComplete: false,
        requireCompletion: true,
        incompleteMessage: "同步连接提前结束，未收到完成确认",
        isComplete: isSummaryComplete,
      },
      (type, message, current, total, data) => {
        const dataToPass = type === "summary" ? pickSummaryData(data) : data;
        onProgress(type, message, current, total, dataToPass);
      },
    );
  },

  // 启动（或复用/重试）指定节目的持久历史同步任务。任务在后台执行，
  // 页面关闭不影响同步；活动任务重复调用返回同一任务（#462）。
  startPodcastEpisodeSync: async (
    podcastId: number,
  ): Promise<{ created: boolean; task: PodcastHistorySyncTask }> => {
    const response = await api.post<
      ApiResponse<{ created: boolean; task: PodcastHistorySyncTask }>
    >(`/api/v1/podcasts/${podcastId}/episodes/sync`);
    const data = handleResponse(response);
    return {
      created: Boolean(data?.created),
      task: data.task,
    };
  },

  // 查询指定节目最新一条历史同步任务；从未同步时返回 null。
  fetchPodcastSyncTask: async (
    podcastId: number,
  ): Promise<PodcastHistorySyncTask | null> => {
    const response = await api.get<
      ApiResponse<PodcastHistorySyncTask | null>
    >(`/api/v1/podcasts/${podcastId}/episodes/sync`);
    const data = handleResponse(response);
    return data ?? null;
  },
};
