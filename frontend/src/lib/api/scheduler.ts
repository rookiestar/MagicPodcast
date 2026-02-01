import { api } from "./client";

export interface WorkflowScheduleInfo {
  entry_id: number;
  next_run?: string;
  prev_run?: string;
}

export interface SchedulerStatus {
  is_running: boolean;
  total_jobs: number;
  workflows: Record<string, WorkflowScheduleInfo>;
}

export const schedulerApi = {
  // 获取调度器状态
  getStatus: async (): Promise<SchedulerStatus> => {
    const response = await api.get<{ success: boolean; data: SchedulerStatus }>(
      "/api/v1/scheduler/status",
    );
    return response.data.data;
  },

  // 重新加载调度器
  reload: async (): Promise<void> => {
    await api.post("/api/v1/scheduler/reload");
  },

  // 暂停工作流调度
  pauseWorkflow: async (workflowId: number): Promise<void> => {
    await api.post(`/api/v1/scheduler/workflows/${workflowId}/pause`);
  },

  // 恢复工作流调度
  resumeWorkflow: async (workflowId: number): Promise<void> => {
    await api.post(`/api/v1/scheduler/workflows/${workflowId}/resume`);
  },
};
