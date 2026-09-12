import { mutate } from "swr";
import { apiClient } from "@/lib/fetcher";
import type {
  CollectionAdoptedFilter,
  CollectionAdoptResult,
  CollectionApiError,
  CollectionDetail,
  CollectionImportResult,
  CollectionPreview,
  CollectionSummary,
} from "@/types/collection";

export const COLLECTIONS_PATH = "/api/v1/collections";

/** 把后端错误映射为可区分的用户可见原因；不把失败伪装成空清单。 */
export function collectionErrorMessage(
  error: unknown,
  fallback: string,
): string {
  if (error && typeof error === "object" && "response" in error) {
    const payload = (
      error as { response?: { data?: { error?: CollectionApiError } } }
    ).response?.data?.error;
    if (payload?.message) return payload.message;
  }
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

export function collectionErrorCode(error: unknown): string | undefined {
  if (!error || typeof error !== "object") return undefined;
  const candidate = error as {
    code?: string;
    response?: { data?: { error?: CollectionApiError } };
  };
  return candidate.response?.data?.error?.code ?? candidate.code;
}

function invalidateCollectionViews() {
  // A saved mutation stays successful even when a subsequent cache read fails.
  void mutate((key: unknown) => {
    const path = Array.isArray(key) ? key[0] : key;
    return (
      typeof path === "string" &&
      (path.startsWith(COLLECTIONS_PATH) ||
        path.startsWith("/api/v1/podcasts") ||
        path.startsWith("/api/v1/consumption"))
    );
  }).catch(() => undefined);
}

export async function previewCollection(
  url: string,
  signal?: AbortSignal,
): Promise<CollectionPreview> {
  const response = await apiClient.post<{
    success: boolean;
    data?: CollectionPreview;
    error?: CollectionApiError;
  }>(`${COLLECTIONS_PATH}/preview`, { url }, { signal });
  if (response.data.success && response.data.data) {
    return response.data.data;
  }
  throw Object.assign(new Error(response.data.error?.message || "预览失败"), {
    code: response.data.error?.code,
  });
}

export async function confirmCollectionImport(
  previewID: string,
): Promise<CollectionImportResult> {
  const response = await apiClient.post<{
    success: boolean;
    data?: CollectionImportResult;
    error?: CollectionApiError;
  }>(COLLECTIONS_PATH, { preview_id: previewID });
  if (response.data.success && response.data.data) {
    invalidateCollectionViews();
    return response.data.data;
  }
  throw Object.assign(new Error(response.data.error?.message || "导入失败"), {
    code: response.data.error?.code,
  });
}

export async function adoptCollectionItem(
  collectionID: number,
  itemID: number,
): Promise<CollectionAdoptResult> {
  const response = await apiClient.post<{
    success: boolean;
    data?: CollectionAdoptResult;
    error?: CollectionApiError;
  }>(`${COLLECTIONS_PATH}/${collectionID}/items/${itemID}/adopt`);
  if (response.data.success && response.data.data) {
    invalidateCollectionViews();
    return response.data.data;
  }
  throw Object.assign(new Error(response.data.error?.message || "收录失败"), {
    code: response.data.error?.code,
  });
}

export interface RefreshChangeSummary {
  added_count: number;
  removed_count: number;
  reordered_count: number;
  recommendation_changed_count: number;
  unchanged_count: number;
  metadata_changed_count?: number;
  collection_changed?: boolean;
}

export interface RefreshRemovedItem {
  position: number;
  external_episode_id: string;
  episode_title: string;
  podcast_title: string;
  adopted: boolean;
}

export interface CollectionRefreshPreview {
  preview_id: string;
  collection_id: number;
  base_revision: number;
  read_count: number;
  changes: RefreshChangeSummary;
  removed: RefreshRemovedItem[];
  title?: string;
  author?: string;
  items?: import("@/types/collection").CollectionPreviewItem[];
}

export interface CollectionApplyRefreshResult {
  applied: boolean;
  no_changes: boolean;
  revision: number;
  item_count: number;
  adopted_kept_count: number;
  removed_adopted_count: number;
}

export async function refreshCollectionPreview(
  collectionID: number,
  signal?: AbortSignal,
): Promise<CollectionRefreshPreview> {
  const response = await apiClient.post<{
    success: boolean;
    data?: CollectionRefreshPreview;
    error?: CollectionApiError;
  }>(`${COLLECTIONS_PATH}/${collectionID}/refresh-preview`, undefined, {
    signal,
  });
  if (response.data.success && response.data.data) {
    return response.data.data;
  }
  throw Object.assign(new Error(response.data.error?.message || "刷新失败"), {
    code: response.data.error?.code,
  });
}

export async function applyCollectionRefresh(
  collectionID: number,
  previewID: string,
  baseRevision: number,
): Promise<CollectionApplyRefreshResult> {
  const response = await apiClient.post<{
    success: boolean;
    data?: CollectionApplyRefreshResult;
    error?: CollectionApiError;
  }>(`${COLLECTIONS_PATH}/${collectionID}/apply-refresh`, {
    preview_id: previewID,
    base_revision: baseRevision,
  });
  if (response.data.success && response.data.data) {
    invalidateCollectionViews();
    return response.data.data;
  }
  throw Object.assign(
    new Error(response.data.error?.message || "应用刷新失败"),
    {
      code: response.data.error?.code,
    },
  );
}

export async function deleteCollection(collectionID: number): Promise<void> {
  await apiClient.delete(`${COLLECTIONS_PATH}/${collectionID}`);
  invalidateCollectionViews();
}

export async function fetchCollectionSummaries(
  search: string,
): Promise<CollectionSummary[]> {
  const response = await apiClient.get<{
    success: boolean;
    data?: CollectionSummary[];
  }>(COLLECTIONS_PATH, {
    params: search ? { search } : undefined,
  });
  return response.data.data ?? [];
}

export async function fetchCollectionDetail(
  id: number,
): Promise<CollectionDetail> {
  const response = await apiClient.get<{
    success: boolean;
    data?: CollectionDetail;
    error?: CollectionApiError;
  }>(`${COLLECTIONS_PATH}/${id}`);
  if (response.data.success && response.data.data) {
    return response.data.data;
  }
  throw new Error(response.data.error?.message || "清单不存在");
}

export const ADOPTED_FILTERS: Array<{
  value: CollectionAdoptedFilter;
  label: string;
}> = [
  { value: "all", label: "全部" },
  { value: "unadopted", label: "未收录" },
  { value: "adopted", label: "已收录" },
];

export function filterItemsByAdoptedState<
  T extends { adopted_episode_id: number | null },
>(items: T[], filter: CollectionAdoptedFilter): T[] {
  if (filter === "adopted") {
    return items.filter((item) => item.adopted_episode_id !== null);
  }
  if (filter === "unadopted") {
    return items.filter((item) => item.adopted_episode_id === null);
  }
  return items;
}

export function formatCollectionDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "";
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`;
  return `${minutes} 分钟`;
}

export function formatCollectionDate(value: string | null): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "long",
    day: "numeric",
  });
}
