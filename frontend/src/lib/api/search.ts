import { api } from "./client";
import { handleResponse } from "./client";
import type { ApiResponse, SearchData } from "@/types";

export const searchApi = {
  // 全局搜索
  search: async (params: {
    q: string;
    type?: "all" | "podcasts" | "episodes";
    tag_id?: number | number[];
    page?: number;
    page_size?: number;
    episode_page?: number;
    episode_page_size?: number;
  }): Promise<{ data: SearchData }> => {
    const queryParams = new URLSearchParams();
    queryParams.append("q", params.q);

    if (params.type) queryParams.append("type", params.type);

    if (params.tag_id) {
      if (Array.isArray(params.tag_id)) {
        params.tag_id.forEach((id) =>
          queryParams.append("tag_id", id.toString()),
        );
      } else {
        queryParams.append("tag_id", params.tag_id.toString());
      }
    }

    if (params.page) queryParams.append("page", params.page.toString());
    if (params.page_size)
      queryParams.append("page_size", params.page_size.toString());
    if (params.episode_page)
      queryParams.append("episode_page", params.episode_page.toString());
    if (params.episode_page_size)
      queryParams.append(
        "episode_page_size",
        params.episode_page_size.toString(),
      );

    const url = `/api/v1/search?${queryParams.toString()}`;
    const response = await api.get<ApiResponse<SearchData>>(url);
    return { data: handleResponse(response) };
  },
};
