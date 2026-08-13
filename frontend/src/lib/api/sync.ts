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
  ): Promise<void> => {
    const formData = new FormData();
    formData.append("opml_file", file);

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
        timeoutMessage: "导入超时（10分钟），可能是网络较慢或文件太大",
        requireCompletion: true,
        incompleteMessage: "导入连接提前结束，未收到完成确认",
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
};
