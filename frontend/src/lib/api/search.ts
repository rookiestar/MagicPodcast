import { api } from "./client";
import { handleResponse } from "./client";
import type { ApiResponse, SearchData } from "@/types";
import type { GenericAbortSignal } from "axios";
import type { SearchParams } from "./types";
import { buildSearchPath } from "@/lib/searchApiPaths";

export const searchApi = {
  // 全局搜索
  search: async (
    params: SearchParams,
    options: { signal?: GenericAbortSignal } = {},
  ): Promise<{ data: SearchData }> => {
    const response = await api.get<ApiResponse<SearchData>>(buildSearchPath(params), {
      signal: options.signal,
    });
    return { data: handleResponse(response) };
  },
};
