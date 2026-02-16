import { api } from "./client";
import { handleResponse, handleVoidResponse } from "./client";
import type { ApiResponse, Tag } from "@/types";

export const tagApi = {
  // 获取所有标签
  list: async (): Promise<Tag[]> => {
    const response = await api.get<ApiResponse<Tag[]>>("/api/v1/tags");
    return handleResponse(response);
  },

  // 获取单个标签详情
  get: async (id: number): Promise<Tag> => {
    const response = await api.get<ApiResponse<Tag>>(`/api/v1/tags/${id}`);
    return handleResponse(response);
  },

  // 创建标签
  create: async (data: { name: string; color?: string }): Promise<Tag> => {
    const response = await api.post<ApiResponse<Tag>>("/api/v1/tags", data);
    return handleResponse(response);
  },

  // 更新标签
  update: async (id: number, data: { name?: string; color?: string }): Promise<Tag> => {
    const response = await api.put<ApiResponse<Tag>>(
      `/api/v1/tags/${id}`,
      data,
    );
    return handleResponse(response);
  },

  // 删除标签
  delete: async (id: number): Promise<void> => {
    const response = await api.delete<ApiResponse<void>>(`/api/v1/tags/${id}`);
    handleVoidResponse(response);
  },
};
