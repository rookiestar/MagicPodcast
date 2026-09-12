import { apiClient } from "@/lib/fetcher";
import type {
  CollectionAdoptedFilter,
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
    const payload = (error as { response?: { data?: { error?: CollectionApiError } } })
      .response?.data?.error;
    if (payload?.message) return payload.message;
  }
  return fallback;
}

export async function previewCollection(url: string): Promise<CollectionPreview> {
  const response = await apiClient.post<{
    success: boolean;
    data?: CollectionPreview;
    error?: CollectionApiError;
  }>(`${COLLECTIONS_PATH}/preview`, { url });
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
    return response.data.data;
  }
  throw Object.assign(new Error(response.data.error?.message || "导入失败"), {
    code: response.data.error?.code,
  });
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
