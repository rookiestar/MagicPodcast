import useSWR from "swr";
import { fetcher } from "@/lib/fetcher";
import { swrConfig, cacheStrategies } from "@/lib/swrConfig";
import type { Workflow, WorkflowsResponse, JobsResponse, WorkflowSortByType } from "@/types";

// ============ 工作流列表 Hook ============

interface UseWorkflowsParams {
  page?: number;
  page_size?: number;
  sort_by?: WorkflowSortByType;
  view?: "summary" | "full";
}

export function useWorkflows(params: UseWorkflowsParams = {}) {
  const queryParams = new URLSearchParams();

  if (params.page) queryParams.set("page", params.page.toString());
  if (params.page_size) queryParams.set("page_size", params.page_size.toString());
  if (params.sort_by) queryParams.set("sort_by", params.sort_by);
  if (params.view) queryParams.set("view", params.view);

  const key = queryParams.toString()
    ? `/api/v1/workflows?${queryParams.toString()}`
    : "/api/v1/workflows";

  const { data, error, isLoading, mutate } = useSWR(
    key,
    () => fetcher<WorkflowsResponse>(key),
    { ...swrConfig, ...cacheStrategies.workflows }
  );

  return {
    workflows: data?.workflows ?? [],
    pagination: data?.pagination,
    isLoading,
    isError: !!error,
    error,
    mutate,
  };
}

// ============ 工作流详情 Hook ============

export function useWorkflow(id: number | null) {
  const { data, error, isLoading, mutate } = useSWR(
    id ? `/api/v1/workflows/${id}` : null,
    () => fetcher<Workflow>(`/api/v1/workflows/${id}`),
    { ...swrConfig, ...cacheStrategies.workflows }
  );

  return {
    workflow: data,
    isLoading,
    isError: !!error,
    error,
    mutate,
  };
}

// ============ 工作流执行历史 Hook ============

export function useWorkflowJobs(
  workflowId: number | null,
  page: number = 1,
  pageSize: number = 10,
  enabled: boolean = true,
) {
  const key = workflowId && enabled
    ? `/api/v1/workflows/${workflowId}/jobs?page=${page}&page_size=${pageSize}&view=summary`
    : null;

  const { data, error, isLoading, mutate } = useSWR(
    key,
    () => fetcher<JobsResponse>(key as string),
    { ...swrConfig, ...cacheStrategies.workflows }
  );

  return {
    jobs: data?.jobs ?? [],
    pagination: data?.pagination,
    isLoading,
    isError: !!error,
    error,
    mutate,
  };
}
