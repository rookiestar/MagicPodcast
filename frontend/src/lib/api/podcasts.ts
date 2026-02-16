import { api } from "./client";
import type { ApiResponse } from "@/types";

export interface PodcastFilters {
  tag_id?: number | number[];
  page?: number;
  page_size?: number;
  sort_by?: string;
  search?: string;
}

export interface PodcastListResponse {
  data: any[];
  pagination: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
  };
}

// 获取播客列表
export async function listPodcasts(
  params?: PodcastFilters,
): Promise<PodcastListResponse> {
  const queryParams = new URLSearchParams();

  if (params?.tag_id) {
    if (Array.isArray(params.tag_id)) {
      params.tag_id.forEach((id) =>
        queryParams.append("tag_id", id.toString()),
      );
    } else {
      queryParams.append("tag_id", params.tag_id.toString());
    }
  }

  if (params?.page) queryParams.append("page", params.page.toString());
  if (params?.page_size)
    queryParams.append("page_size", params.page_size.toString());
  if (params?.sort_by) queryParams.append("sort_by", params.sort_by);
  if (params?.search) queryParams.append("search", params.search);

  const url = queryParams.toString()
    ? `/api/v1/podcasts?${queryParams.toString()}`
    : "/api/v1/podcasts";

  console.log("[podcastApi.list] Requesting:", url);

  const response = await api.get<
    ApiResponse<{
      data: any[];
      pagination?: {
        page: number;
        page_size: number;
        total: number;
        total_pages: number;
      };
    }>
  >(url);

  console.log("[podcastApi.list] Response:", response.data);

  if (response.data.success && response.data.data) {
    const result = response.data.data;
    return {
      data: result.data,
      pagination: result.pagination || {
        page: 1,
        page_size: 15,
        total: result.data.length,
        total_pages: 1,
      },
    };
  }
  throw new Error(response.data.error?.message || "Failed to fetch podcasts");
}

// 获取单个播客详情
export async function getPodcast(id: number): Promise<any> {
  const response = await api.get<ApiResponse<any>>(`/api/v1/podcasts/${id}`);
  if (response.data.success && response.data.data) {
    return response.data.data;
  }
  throw new Error(response.data.error?.message || "Failed to fetch podcast");
}

// 获取播客备注
export async function getPodcastNotes(id: number): Promise<string> {
  const response = await api.get<ApiResponse<{ id: number; notes: string }>>(
    `/api/v1/podcasts/${id}/notes`,
  );
  if (response.data.success && response.data.data) {
    return response.data.data.notes;
  }
  throw new Error(response.data.error?.message || "Failed to fetch notes");
}

// 更新播客备注
export async function updatePodcastNotes(
  id: number,
  notes: string,
): Promise<void> {
  const response = await api.put<ApiResponse<{ id: number; notes: string }>>(
    `/api/v1/podcasts/${id}/notes`,
    { notes },
  );
  if (!response.data.success) {
    throw new Error(response.data.error?.message || "Failed to update notes");
  }
}

// 获取播客的所有标签
export async function getPodcastTags(id: number): Promise<any[]> {
  const response = await api.get<ApiResponse<{ tags: any[] }>>(
    `/api/v1/podcasts/${id}/tags`,
  );
  if (response.data.success && response.data.data) {
    return response.data.data.tags;
  }
  throw new Error(response.data.error?.message || "Failed to fetch tags");
}

// 为播客添加标签
export async function addTagToPodcast(
  id: number,
  tagId: number,
): Promise<void> {
  const response = await api.post<ApiResponse<any>>(
    `/api/v1/podcasts/${id}/tags`,
    { tag_id: tagId },
  );
  if (!response.data.success) {
    throw new Error(response.data.error?.message || "Failed to add tag");
  }
}

// 移除播客标签
export async function removeTagFromPodcast(
  id: number,
  tagId: number,
): Promise<void> {
  const response = await api.delete<ApiResponse<any>>(
    `/api/v1/podcasts/${id}/tags/${tagId}`,
  );
  if (!response.data.success) {
    throw new Error(response.data.error?.message || "Failed to remove tag");
  }
}

// 更新播客自定义封面
export async function updateCustomCover(
  id: number,
  customCoverUrl: string,
): Promise<void> {
  const response = await api.put<ApiResponse<{ id: number; custom_cover_url: string }>>(
    `/api/v1/podcasts/${id}/custom-cover`,
    { custom_cover_url: customCoverUrl },
  );
  if (!response.data.success) {
    throw new Error(response.data.error?.message || "Failed to update custom cover");
  }
}

// 导出为对象形式以保持向后兼容
export const podcastApi = {
  list: listPodcasts,
  get: getPodcast,
  getNotes: getPodcastNotes,
  updateNotes: updatePodcastNotes,
  getTags: getPodcastTags,
  addTag: addTagToPodcast,
  removeTag: removeTagFromPodcast,
  updateCustomCover,
};
