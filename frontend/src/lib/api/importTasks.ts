import { api } from "./client";

// 导入预览条目分类（与后端 sync.ImportEntryKind 对应）。
export type ImportEntryKind =
  | "new"
  | "existing"
  | "collection"
  | "deleted"
  | "invalid"
  | "duplicate";

export interface ImportPreviewEntry {
  xml_url: string;
  title: string;
  kind: ImportEntryKind;
  podcast_id?: number;
  podcast_title?: string;
  current_feed?: string;
  subscribed?: boolean;
  evidence?: string;
  reason?: string;
  duplicate_in_file?: number;
}

export interface ImportPreview {
  total: number;
  entries: ImportPreviewEntry[];
  new_count: number;
  existing_count: number;
  collection_count: number;
  deleted_count: number;
  invalid_count: number;
  duplicate_merged_count: number;
}

// 需要用户确认后才会写入的条目分类。
export const CONFIRMABLE_KINDS: ReadonlySet<ImportEntryKind> = new Set([
  "collection",
  "deleted",
]);

export const RETRY_CONFIRMATION_TEXT = "RETRY IMPORT";

export type ImportTaskStatus =
  | "running"
  | "completed"
  | "failed"
  | "interrupted";

export interface ImportTask {
  id: number;
  status: ImportTaskStatus;
  file_name: string;
  total: number;
  processed: number;
  success_count: number;
  pending_count: number;
  conflict_count: number;
  merged_count: number;
  unchanged_count: number;
  skipped_count: number;
  failed_count: number;
  error_message: string;
  started_at: string;
  finished_at?: string | null;
}

export interface ImportEntryResult {
  title: string;
  feed_url: string;
  outcome: string;
  detail?: string;
  podcast_id?: number;
  created?: boolean;
}

// 工作流覆盖归属的最小信息（与后端 workflow.WorkflowRef 对应）。
export interface WorkflowRef {
  id: number;
  name: string;
  scope_type: string;
  is_enabled: boolean;
}

// 本批实际新建的节目条目；ready/is_subscribed 取节目当前记录，
// history_sync 为节目历史同步状态摘要（#462/#465）。
export interface ImportNewPodcast {
  id: number;
  title: string;
  feed_url: string;
  ready: boolean;
  is_subscribed: boolean;
  workflows: WorkflowRef[];
  history_sync?: {
    task_id: number;
    status: "pending" | "queued" | "running" | "completed" | "partial" | "failed";
    trigger: string;
    processed_count: number;
    total_known?: number | null;
    error_message?: string;
    source_note?: string;
  } | null;
}

export interface ImportNewPodcastsPayload {
  success: boolean;
  task_id: number;
  total: number;
  podcasts: ImportNewPodcast[];
}

interface ImportTaskPayload {
  success: boolean;
  task: ImportTask | null;
  entries: ImportEntryResult[];
}

export const importTasksApi = {
  // 只读预览：不抓取网络、不写库，也不需要确认短语。
  previewImportOPML: async (file: File): Promise<ImportPreview> => {
    const formData = new FormData();
    formData.append("opml_file", file);
    const response = await api.post<{ preview: ImportPreview }>(
      "/api/v1/sync/import/preview",
      formData,
      { headers: { "Content-Type": "multipart/form-data" } },
    );
    return response.data.preview;
  },

  fetchLatestImportTask: async (): Promise<ImportTaskPayload> => {
    const response = await api.get<ImportTaskPayload>(
      "/api/v1/sync/import/tasks/latest",
    );
    return response.data;
  },

  fetchImportTask: async (taskId: number): Promise<ImportTaskPayload> => {
    const response = await api.get<ImportTaskPayload>(
      `/api/v1/sync/import/tasks/${taskId}`,
    );
    return response.data;
  },

  // 本批实际新建的节目（含重试链），附当前资料状态与工作流归属。
  fetchTaskNewPodcasts: async (
    taskId: number,
  ): Promise<ImportNewPodcastsPayload> => {
    const response = await api.get<ImportNewPodcastsPayload>(
      `/api/v1/sync/import/tasks/${taskId}/new-podcasts`,
    );
    return response.data;
  },

  // 仅重试失败/待同步条目；decisions 可选，用于对冲突条目补充确认。
  retryImportTask: async (
    taskId: number,
    decisions?: Record<string, string>,
  ): Promise<ImportRetryResponse> => {
    const formData = new FormData();
    if (decisions && Object.keys(decisions).length > 0) {
      formData.append("decisions", JSON.stringify(decisions));
    }
    formData.append("confirmation_text", RETRY_CONFIRMATION_TEXT);
    const response = await api.post<ImportRetryResponse>(
      `/api/v1/sync/import/tasks/${taskId}/retry`,
      formData,
      { headers: { "Content-Type": "multipart/form-data" } },
    );
    return response.data;
  },
};

export interface ImportRetryResponse {
  success: boolean;
  task_id?: number;
  parent_task_id: number;
  status?: ImportTaskStatus;
  task?: ImportTask;
  message: string;
  total_podcasts: number;
  success_count: number;
  failed_count: number;
  stub_podcasts: number;
  merged_podcasts?: number;
  conflict_podcasts?: number;
  unchanged_podcasts?: number;
  entries: ImportEntryResult[];
  errors?: string[];
}
