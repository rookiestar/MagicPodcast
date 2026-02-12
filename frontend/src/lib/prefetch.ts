import { mutate } from "swr";
import { apiClient } from "./fetcher";

/**
 * 预取播客详情数据（包括标签、备注、单集）
 * 在用户 hover 卡片时触发，提前加载数据到 SWR 缓存
 */
export async function prefetchPodcastData(podcastId: number) {
  try {
    // 并行预取所有播客详情数据
    const [podcastRes, tagsRes, notesRes, episodesRes] = await Promise.all([
      apiClient.get(`/api/v1/podcasts/${podcastId}`),
      apiClient.get(`/api/v1/podcasts/${podcastId}/tags`),
      apiClient.get(`/api/v1/podcasts/${podcastId}/notes`),
      apiClient.get(`/api/v1/podcasts/${podcastId}/episodes?page=1&page_size=20`),
    ]);

    // 将数据写入 SWR 缓存（不触发重新验证）
    if (podcastRes.data?.success && podcastRes.data?.data) {
      mutate(`/api/v1/podcasts/${podcastId}`, podcastRes.data.data, false);
    }
    if (tagsRes.data?.success && tagsRes.data?.data) {
      mutate(`/api/v1/podcasts/${podcastId}/tags`, tagsRes.data.data, false);
    }
    if (notesRes.data?.success && notesRes.data?.data) {
      mutate(`/api/v1/podcasts/${podcastId}/notes`, notesRes.data.data, false);
    }
    if (episodesRes.data?.success && episodesRes.data?.data) {
      mutate(
        `/api/v1/podcasts/${podcastId}/episodes?page=1&page_size=20`,
        episodesRes.data.data,
        false
      );
    }
  } catch (error) {
    // 预取失败不报错，静默处理
    console.debug("[prefetch] Failed to prefetch podcast:", podcastId, error);
  }
}

/**
 * 预取工作流详情数据
 */
export async function prefetchWorkflowData(workflowId: number) {
  try {
    const [workflowRes, jobsRes] = await Promise.all([
      apiClient.get(`/api/v1/workflows/${workflowId}`),
      apiClient.get(`/api/v1/workflows/${workflowId}/jobs?page=1&page_size=10`),
    ]);

    if (workflowRes.data?.success && workflowRes.data?.data) {
      mutate(`/api/v1/workflows/${workflowId}`, workflowRes.data.data, false);
    }
    if (jobsRes.data?.success && jobsRes.data?.data) {
      mutate(
        `/api/v1/workflows/${workflowId}/jobs?page=1&page_size=10`,
        jobsRes.data.data,
        false
      );
    }
  } catch (error) {
    console.debug("[prefetch] Failed to prefetch workflow:", workflowId, error);
  }
}
