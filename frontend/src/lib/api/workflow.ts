import { api } from "./client";
import { handleResponse, handleVoidResponse } from "./client";
import type {
  ApiResponse,
  Workflow,
  WorkflowRequest,
  WorkflowsResponse,
  JobsResponse,
  Job,
  Report,
} from "@/types";

export const workflowApi = {
  // 获取工作流列表
  list: async (params?: {
    page?: number;
    page_size?: number;
    view?: "summary" | "full";
  }): Promise<WorkflowsResponse> => {
    const queryParams = new URLSearchParams();
    if (params?.page) queryParams.append("page", params.page.toString());
    if (params?.page_size)
      queryParams.append("page_size", params.page_size.toString());
    if (params?.view) queryParams.append("view", params.view);

    const url = queryParams.toString()
      ? `/api/v1/workflows?${queryParams.toString()}`
      : "/api/v1/workflows";

    const response = await api.get<ApiResponse<WorkflowsResponse>>(url);
    return handleResponse(response);
  },

  // 获取工作流详情
  get: async (id: number): Promise<Workflow> => {
    const response = await api.get<ApiResponse<Workflow>>(
      `/api/v1/workflows/${id}`,
    );
    return handleResponse(response);
  },

  // 创建工作流
  create: async (data: WorkflowRequest): Promise<Workflow> => {
    const response = await api.post<ApiResponse<Workflow>>(
      "/api/v1/workflows",
      data,
    );
    return handleResponse(response);
  },

  // 更新工作流
  update: async (id: number, data: WorkflowRequest): Promise<Workflow> => {
    const response = await api.put<ApiResponse<Workflow>>(
      `/api/v1/workflows/${id}`,
      data,
    );
    return handleResponse(response);
  },

  // 删除工作流
  delete: async (id: number, confirmationText: string): Promise<void> => {
    const response = await api.delete<ApiResponse<void>>(`/api/v1/workflows/${id}`, {
      data: { confirmation_text: confirmationText },
    });
    handleVoidResponse(response);
  },

  // 启用/禁用工作流
  toggle: async (id: number): Promise<Workflow> => {
    const response = await api.post<ApiResponse<Workflow>>(
      `/api/v1/workflows/${id}/toggle`,
    );
    return handleResponse(response);
  },

  // 把明确选中的节目集合追加到「指定节目」工作流；服务端在事务内做
  // 集合追加，重复请求安全（#418）。返回实际新增数量与当前成员数。
  appendPodcasts: async (
    id: number,
    podcastIds: number[],
  ): Promise<WorkflowAppendResponse> => {
    const response = await api.post<WorkflowAppendResponse>(
      `/api/v1/workflows/${id}/podcasts/append`,
      { podcast_ids: podcastIds },
    );
    return response.data;
  },

  // 获取工作流的执行历史
  listJobs: async (
    id: number,
    params?: { page?: number; page_size?: number; view?: "full" | "summary" },
  ): Promise<JobsResponse> => {
    const queryParams = new URLSearchParams();
    if (params?.page) queryParams.append("page", params.page.toString());
    if (params?.page_size)
      queryParams.append("page_size", params.page_size.toString());
    if (params?.view) queryParams.append("view", params.view);

    const url = queryParams.toString()
      ? `/api/v1/workflows/${id}/jobs?${queryParams.toString()}`
      : `/api/v1/workflows/${id}/jobs`;

    const response = await api.get<ApiResponse<JobsResponse>>(url);
    return handleResponse(response);
  },

  // 获取任务详情
  getJob: async (id: number): Promise<Job> => {
    const response = await api.get<ApiResponse<Job>>(`/api/v1/jobs/${id}`);
    return handleResponse(response);
  },

  // 手动触发工作流
  trigger: async (id: number, confirmationText: string): Promise<void> => {
    const response = await api.post<ApiResponse<void>>(`/api/v1/workflows/${id}/trigger`, {
      confirmation_text: confirmationText,
    });
    handleVoidResponse(response);
  },

  regenerateLLMSummary: async (id: number, confirmationText: string): Promise<Report> => {
    const response = await api.post<ApiResponse<Report>>(
      `/api/v1/jobs/${id}/regenerate-llm`,
      { confirmation_text: confirmationText },
    );
    return handleResponse(response);
  },

  /** partial Job: confirm-gated retry of only final failed Feeds (#40). */
  compensateFailed: async (id: number, confirmationText: string): Promise<void> => {
    const response = await api.post<ApiResponse<void>>(
      `/api/v1/jobs/${id}/compensate-failed`,
      { confirmation_text: confirmationText },
    );
    handleVoidResponse(response);
  },

  // 获取Job报告
  getJobReport: async (id: number): Promise<Report> => {
    const response = await api.get<ApiResponse<Report>>(
      `/api/v1/jobs/${id}/report`,
    );
    return handleResponse(response);
  },
};

// 成员追加结果：实际新增数量以服务端成功结果为准（#418 AC13）。
// history_sync 区分「成员已保存」与「历史同步状态」，提供可查询任务标识（#462）。
export interface WorkflowAppendResponse {
  success: boolean;
  workflow_id: number;
  workflow_name: string;
  added: number;
  already_member: number;
  podcast_count: number;
  history_sync?: Array<{
    podcast_id: number;
    task_id: number | null;
    status: string | null;
  }>;
}
