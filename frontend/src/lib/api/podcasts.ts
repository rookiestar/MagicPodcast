import { api } from "./client";
import { handleResponse, handleVoidResponse } from "./client";
import {
  buildPodcastBatchPath,
  buildPodcastCustomCoverPath,
  buildPodcastDetailPath,
  buildPodcastListPath,
  buildPodcastNotesPath,
  buildPodcastTagPath,
  buildPodcastTagsPath,
} from "@/lib/podcastApiPaths";
import type { ApiResponse } from "@/types";

export interface PodcastFilters {
  tag_id?: number | number[];
  page?: number;
  page_size?: number;
  sort_by?: string;
  search?: string;
  view?: "summary" | "full";
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
  type PodcastListApiResponse = ApiResponse<any[]> & {
    pagination?: {
      page: number;
      page_size: number;
      total: number;
      total_pages: number;
    };
  };

  const response = await api.get<PodcastListApiResponse>(
    buildPodcastListPath({ view: "summary", ...params }),
  );
  const data = handleResponse(response);

  return {
    data,
    pagination: response.data.pagination || {
      page: 1,
      page_size: 15,
      total: data.length,
      total_pages: 1,
    },
  };
}

// 获取单个播客详情
export async function getPodcast(id: number): Promise<any> {
  const response = await api.get<ApiResponse<any>>(buildPodcastDetailPath(id));
  return handleResponse(response);
}

// 批量获取播客详情
export async function batchGetPodcasts(ids: number[]): Promise<any[]> {
  const response = await api.post<ApiResponse<any[]>>(
    buildPodcastBatchPath(),
    { ids, view: "summary" },
  );
  return handleResponse(response);
}

// 获取播客备注
export async function getPodcastNotes(id: number): Promise<string> {
  const response = await api.get<ApiResponse<{ id: number; notes: string }>>(
    buildPodcastNotesPath(id),
  );
  return handleResponse(response).notes;
}

// 更新播客备注
export async function updatePodcastNotes(
  id: number,
  notes: string,
): Promise<void> {
  const response = await api.put<ApiResponse<void>>(
    buildPodcastNotesPath(id),
    { notes },
  );
  handleVoidResponse(response);
}

// 获取播客的所有标签
export async function getPodcastTags(id: number): Promise<any[]> {
  const response = await api.get<ApiResponse<{ tags: any[] }>>(
    buildPodcastTagsPath(id),
  );
  return handleResponse(response).tags;
}

// 为播客添加标签
export async function addTagToPodcast(
  id: number,
  tagId: number,
): Promise<void> {
  const response = await api.post<ApiResponse<void>>(
    buildPodcastTagsPath(id),
    { tag_id: tagId },
  );
  handleVoidResponse(response);
}

// 移除播客标签
export async function removeTagFromPodcast(
  id: number,
  tagId: number,
): Promise<void> {
  const response = await api.delete<ApiResponse<void>>(
    buildPodcastTagPath(id, tagId),
  );
  handleVoidResponse(response);
}

// 更新播客自定义封面
export async function updateCustomCover(
  id: number,
  customCoverUrl: string,
): Promise<void> {
  const response = await api.put<ApiResponse<void>>(
    buildPodcastCustomCoverPath(id),
    { custom_cover_url: customCoverUrl },
  );
  handleVoidResponse(response);
}

// 导出为对象形式以保持向后兼容
export const podcastApi = {
  list: listPodcasts,
  get: getPodcast,
  batchGet: batchGetPodcasts,
  getNotes: getPodcastNotes,
  updateNotes: updatePodcastNotes,
  getTags: getPodcastTags,
  addTag: addTagToPodcast,
  removeTag: removeTagFromPodcast,
  updateCustomCover,
};
