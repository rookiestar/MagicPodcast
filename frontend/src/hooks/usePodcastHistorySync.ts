"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import useSWR from "swr";
import { syncApi } from "@/lib/api/sync";
import { getErrorMessage } from "@/lib/errorMessage";
import type { PodcastHistorySyncTask } from "@/types";

const SYNC_TASK_POLL_INTERVAL_MS = 3000;

function isActiveTask(task: PodcastHistorySyncTask | null | undefined) {
  return (
    task != null &&
    (task.status === "pending" ||
      task.status === "queued" ||
      task.status === "running")
  );
}

interface UsePodcastHistorySyncOptions {
  podcastId: number;
  enabled: boolean;
  /** 任务到达终态（完成/部分完成/失败）后回调一次，供刷新列表与详情 */
  onSettled?: () => void;
}

// usePodcastHistorySync 面向节目详情页的持久历史同步任务：轮询任务状态
// （排队/运行中每 3 秒）、提供启动/重试入口。推送不是唯一事实来源，刷新
// 后从持久任务恢复（#462/#465）。
export function usePodcastHistorySync({
  podcastId,
  enabled,
  onSettled,
}: UsePodcastHistorySyncOptions) {
  const shouldPoll = Boolean(enabled && podcastId);
  const { data: task, mutate } = useSWR(
    shouldPoll ? ["podcast-history-sync-task", podcastId] : null,
    () => syncApi.fetchPodcastSyncTask(podcastId),
    {
      // SWR 以最新数据决定下一次轮询间隔：活动任务 3 秒，终态停止。
      refreshInterval: (latest: PodcastHistorySyncTask | null | undefined) =>
        isActiveTask(latest) ? SYNC_TASK_POLL_INTERVAL_MS : 0,
      revalidateOnFocus: true,
    },
  );

  const [starting, setStarting] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const settledNotifiedRef = useRef(false);
  const onSettledRef = useRef(onSettled);
  useEffect(() => {
    onSettledRef.current = onSettled;
  }, [onSettled]);

  // 任务从活动态进入终态时通知一次（成功或失败都刷新页面数据）。
  useEffect(() => {
    if (task && !isActiveTask(task) && !settledNotifiedRef.current) {
      settledNotifiedRef.current = true;
      onSettledRef.current?.();
    }
    if (isActiveTask(task)) {
      settledNotifiedRef.current = false;
    }
  }, [task]);

  const start = useCallback(async () => {
    if (!shouldPoll || starting) {
      return;
    }
    setStarting(true);
    setActionError(null);
    try {
      const result = await syncApi.startPodcastEpisodeSync(podcastId);
      settledNotifiedRef.current = false;
      await mutate(result.task, {
        revalidate: isActiveTask(result.task),
      });
    } catch (error) {
      setActionError(getErrorMessage(error));
    } finally {
      setStarting(false);
    }
  }, [mutate, podcastId, shouldPoll, starting]);

  return {
    task: task ?? null,
    starting,
    actionError,
    start,
  };
}
