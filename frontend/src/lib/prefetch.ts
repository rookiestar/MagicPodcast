import { mutate } from "swr";
import { apiClient } from "./fetcher";
import {
  buildPodcastDetailPath,
  buildPodcastNotesPath,
  buildPodcastTagsPath,
} from "./podcastApiPaths";

const inFlightPrefetches = new Map<string, Promise<void>>();

function runDedupedPrefetch(key: string, task: () => Promise<void>) {
  const inFlightPrefetch = inFlightPrefetches.get(key);
  if (inFlightPrefetch) {
    return inFlightPrefetch;
  }

  const nextPrefetch = task().finally(() => {
    inFlightPrefetches.delete(key);
  });
  inFlightPrefetches.set(key, nextPrefetch);

  return nextPrefetch;
}

/**
 * 预取播客详情数据（包括标签、备注）
 * 在用户 hover 卡片时触发，提前加载数据到 SWR 缓存
 */
export async function prefetchPodcastData(podcastId: number) {
  return runDedupedPrefetch(`podcast:${podcastId}`, async () => {
    try {
      const podcastPath = buildPodcastDetailPath(podcastId);
      const tagsPath = buildPodcastTagsPath(podcastId);
      const notesPath = buildPodcastNotesPath(podcastId);

      // 并行预取所有播客详情数据
      const [podcastRes, tagsRes, notesRes] = await Promise.all([
        apiClient.get(podcastPath),
        apiClient.get(tagsPath),
        apiClient.get(notesPath),
      ]);

      // 将数据写入 SWR 缓存（不触发重新验证）
      if (podcastRes.data?.success && podcastRes.data?.data) {
        mutate(podcastPath, podcastRes.data.data, false);
      }
      if (tagsRes.data?.success && tagsRes.data?.data) {
        mutate(tagsPath, tagsRes.data.data, false);
      }
      if (notesRes.data?.success && notesRes.data?.data) {
        mutate(notesPath, notesRes.data.data, false);
      }
    } catch (error) {
      // 预取失败不报错，静默处理
      console.debug("[prefetch] Failed to prefetch podcast:", podcastId, error);
    }
  });
}

/**
 * 预取工作流详情数据
 */
export async function prefetchWorkflowData(workflowId: number) {
  return runDedupedPrefetch(`workflow:${workflowId}`, async () => {
    try {
      const [workflowRes, jobsRes] = await Promise.all([
        apiClient.get(`/api/v1/workflows/${workflowId}`),
        apiClient.get(
          `/api/v1/workflows/${workflowId}/jobs?page=1&page_size=10&view=summary`,
        ),
      ]);

      if (workflowRes.data?.success && workflowRes.data?.data) {
        mutate(`/api/v1/workflows/${workflowId}`, workflowRes.data.data, false);
      }
      if (jobsRes.data?.success && jobsRes.data?.data) {
        mutate(
          `/api/v1/workflows/${workflowId}/jobs?page=1&page_size=10&view=summary`,
          jobsRes.data.data,
          false,
        );
      }
    } catch (error) {
      console.debug("[prefetch] Failed to prefetch workflow:", workflowId, error);
    }
  });
}
