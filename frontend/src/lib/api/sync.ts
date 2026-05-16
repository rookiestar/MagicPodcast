import { sseFormDataRequest, sseRequest } from "@/lib/sseClient";

const TEN_MINUTES = 10 * 60 * 1000;

type BasicProgressCallback = (
  type: string,
  message: string,
  current?: number,
  total?: number,
) => void;

type SyncProgressCallback = (
  type: string,
  message: string,
  current?: number,
  total?: number,
  data?: any,
) => void;

function pickSummaryData(data: any) {
  return {
    total_podcasts: data.total_podcasts,
    success_podcasts: data.success_podcasts,
    failed_podcasts: data.failed_podcasts,
    skipped_podcasts: data.skipped_podcasts,
    no_update_podcasts: data.no_update_podcasts,
    total_episodes: data.total_episodes,
    new_episodes: data.new_episodes,
    updated_episodes: data.updated_episodes,
    duration: data.duration,
  };
}

export const syncApi = {
  importOPMLSSE: async (
    file: File,
    onProgress: BasicProgressCallback,
  ): Promise<void> => {
    const formData = new FormData();
    formData.append("opml_file", file);

    return sseFormDataRequest(
      "/api/v1/sync/import-sse",
      formData,
      (type, message, current, total) => {
        onProgress(type, message, current, total);
      },
      {
        timeout: TEN_MINUTES,
        logPrefix: "[Import]",
        emptyMessage: "未收到任何导入消息",
        abortMessage: "导入被取消",
        timeoutMessage: "导入超时（10分钟），可能是网络较慢或文件太大",
        requireCompletion: true,
        incompleteMessage: "导入连接提前结束，未收到完成确认",
        isComplete: (data) =>
          data.type === "success" && data.message.includes("导入完成"),
      },
    );
  },

  syncPodcastsMetadataSSE: async (
    onProgress: SyncProgressCallback,
  ): Promise<void> => {
    return sseRequest(
      {
        endpoint: "/api/v1/sync/podcasts/metadata-sse",
        method: "POST",
        timeout: TEN_MINUTES,
        logPrefix: "[Sync Metadata]",
        emptyMessage: "未收到任何同步消息",
        abortMessage: "同步被取消",
        timeoutMessage: "同步超时（10分钟）",
        completeOnTypeComplete: false,
        requireCompletion: true,
        incompleteMessage: "同步连接提前结束，未收到完成确认",
        isComplete: (data) => data.type === "summary",
      },
      (type, message, current, total, data) => {
        const dataToPass = type === "summary" ? pickSummaryData(data) : data;
        onProgress(type, message, current, total, dataToPass);
      },
    );
  },
};
