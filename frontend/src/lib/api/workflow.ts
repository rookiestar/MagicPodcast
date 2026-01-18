import { api } from './client'
import type { Workflow, WorkflowRequest, WorkflowsResponse, JobsResponse, Job, Report } from '@/types'

export const workflowApi = {
  // 获取工作流列表
  list: async (params?: { page?: number; page_size?: number }): Promise<WorkflowsResponse> => {
    const queryParams = new URLSearchParams()
    if (params?.page) queryParams.append('page', params.page.toString())
    if (params?.page_size) queryParams.append('page_size', params.page_size.toString())

    const url = queryParams.toString()
      ? `/api/v1/workflows?${queryParams.toString()}`
      : '/api/v1/workflows'

    const response = await api.get<{ success: boolean; data: WorkflowsResponse }>(url)
    return response.data.data
  },

  // 获取工作流详情
  get: async (id: number): Promise<Workflow> => {
    const response = await api.get<{ success: boolean; data: Workflow }>(`/api/v1/workflows/${id}`)
    return response.data.data
  },

  // 创建工作流
  create: async (data: WorkflowRequest): Promise<Workflow> => {
    const response = await api.post<{ success: boolean; data: Workflow }>(
      '/api/v1/workflows',
      data
    )
    return response.data.data
  },

  // 更新工作流
  update: async (id: number, data: WorkflowRequest): Promise<Workflow> => {
    const response = await api.put<{ success: boolean; data: Workflow }>(
      `/api/v1/workflows/${id}`,
      data
    )
    return response.data.data
  },

  // 删除工作流
  delete: async (id: number): Promise<void> => {
    await api.delete(`/api/v1/workflows/${id}`)
  },

  // 启用/禁用工作流
  toggle: async (id: number): Promise<Workflow> => {
    const response = await api.post<{ success: boolean; data: Workflow }>(
      `/api/v1/workflows/${id}/toggle`
    )
    return response.data.data
  },

  // 获取工作流的执行历史
  listJobs: async (id: number, params?: { page?: number; page_size?: number }): Promise<JobsResponse> => {
    const queryParams = new URLSearchParams()
    if (params?.page) queryParams.append('page', params.page.toString())
    if (params?.page_size) queryParams.append('page_size', params.page_size.toString())

    const url = queryParams.toString()
      ? `/api/v1/workflows/${id}/jobs?${queryParams.toString()}`
      : `/api/v1/workflows/${id}/jobs`

    const response = await api.get<{ success: boolean; data: JobsResponse }>(url)
    return response.data.data
  },

  // 获取任务详情
  getJob: async (id: number): Promise<Job> => {
    const response = await api.get<{ success: boolean; data: Job }>(`/api/v1/jobs/${id}`)
    return response.data.data
  },

  // 手动触发工作流
  trigger: async (id: number): Promise<void> => {
    await api.post(`/api/v1/workflows/${id}/trigger`)
  },

  // 获取Job报告
  getJobReport: async (id: number): Promise<Report> => {
    const response = await api.get<{ success: boolean; data: Report }>(`/api/v1/jobs/${id}/report`)
    return response.data.data
  },
}
