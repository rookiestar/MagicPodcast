import { api } from "./client";
import { handleResponse, handleVoidResponse } from "./client";
import { buildPodcastEpisodesCollectionPath } from "@/lib/podcastApiPaths";
import type { ApiResponse, Episode, Tag } from "@/types";

export const episodeApi = {
  // 获取播客的单集列表（支持分页）
  listByPodcast: async (
    podcastId: number,
    page: number = 1,
    pageSize: number = 20,
  ): Promise<{
    episodes: Episode[];
    pagination: {
      page: number;
      page_size: number;
      total: number;
      total_pages: number;
      has_more: boolean;
    };
  }> => {
    type EpisodeListResponse = ApiResponse<Episode[]> & {
      pagination: {
        page: number;
        page_size: number;
        total: number;
        total_pages: number;
        has_more: boolean;
      };
    };

    const response = await api.get<EpisodeListResponse>(
      buildPodcastEpisodesCollectionPath(podcastId),
      { params: { page, page_size: pageSize } },
    );

    return {
      episodes: handleResponse(response),
      pagination: response.data.pagination,
    };
  },

  // 获取单集备注
  getNotes: async (id: number): Promise<string> => {
    const response = await api.get<ApiResponse<{ id: number; notes: string }>>(
      `/api/v1/episodes/${id}/notes`,
    );
    return handleResponse(response).notes;
  },

  // 更新单集备注
  updateNotes: async (id: number, notes: string): Promise<void> => {
    const response = await api.put<ApiResponse<void>>(
      `/api/v1/episodes/${id}/notes`,
      { notes },
    );
    handleVoidResponse(response);
  },

  // 获取单集的所有标签
  getTags: async (id: number): Promise<Tag[]> => {
    const response = await api.get<ApiResponse<{ tags: Tag[] }>>(
      `/api/v1/episodes/${id}/tags`,
    );
    return handleResponse(response).tags;
  },

  // 为单集添加标签
  addTag: async (id: number, tagId: number): Promise<void> => {
    const response = await api.post<ApiResponse<void>>(
      `/api/v1/episodes/${id}/tags`,
      { tag_id: tagId },
    );
    handleVoidResponse(response);
  },

  // 移除单集标签
  removeTag: async (id: number, tagId: number): Promise<void> => {
    const response = await api.delete<ApiResponse<void>>(
      `/api/v1/episodes/${id}/tags/${tagId}`,
    );
    handleVoidResponse(response);
  },
};
