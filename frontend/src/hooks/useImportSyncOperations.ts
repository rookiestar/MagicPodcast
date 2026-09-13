import type { ChangeEvent } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import { syncApi } from "@/lib/api";
import {
  CONFIRMABLE_KINDS,
  importTasksApi,
  type ImportPreview,
  type ImportPreviewEntry,
  type ImportTask,
} from "@/lib/api/importTasks";
import { removeStorageValue, writeStorageValue } from "@/lib/browserStorage";
import { STORAGE_KEYS } from "@/lib/config";
import {
  isOpmlFileSizeAllowed,
  isValidOpmlFile,
  MAX_OPML_FILE_SIZE_BYTES,
} from "@/lib/importFileValidation";
import {
  buildImportErrorLogs,
  buildSyncErrorMessage,
} from "@/lib/syncOperationMessages";
import { runSseOperation, type AddSyncLog } from "@/lib/syncSseOperation";
import type { SyncLogMode } from "@/lib/syncLogState";
import { toast } from "@/lib/toast";
import { requestTypedConfirmation } from "@/lib/confirmation";
import { useExclusiveAsyncAction } from "./useExclusiveAsyncAction";

interface UseImportSyncOperationsOptions {
  addLog: AddSyncLog;
  resetLogScroll: () => void;
  startLogSession: (mode: SyncLogMode) => void;
}

function useStoredOperationMarker(active: boolean, storageKey: string) {
  useEffect(() => {
    if (active) {
      writeStorageValue(storageKey, "true");
    } else {
      removeStorageValue(storageKey);
    }
  }, [active, storageKey]);
}

// 任务轮询间隔与上限：断线后任务仍在后台执行，前端按预算轮询而非无限等待。
const TASK_POLL_INTERVAL_MS = 4000;
const TASK_POLL_MAX_ATTEMPTS = 150;

export function useImportSyncOperations({
  addLog,
  resetLogScroll,
  startLogSession,
}: UseImportSyncOperationsOptions) {
  const [file, setFile] = useState<File | null>(null);
  const [importing, setImporting] = useState(false);
  const [syncing, setSyncing] = useState(false);

  // 导入预览：写入前的差异核对（只读，不抓取）。
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);
  // 用户对需确认条目的显式决定（xmlUrl → confirm）。
  const [confirmedUrls, setConfirmedUrls] = useState<Record<string, boolean>>({});

  // 最近一次导入任务：页面刷新/断线后恢复查看，断连后轮询终态。
  const [lastTask, setLastTask] = useState<ImportTask | null>(null);
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pollAttemptsRef = useRef(0);

  const runExclusiveOperation = useExclusiveAsyncAction({
    isBlocked: importing || syncing,
  });

  useStoredOperationMarker(syncing, STORAGE_KEYS.SYNCING);
  useStoredOperationMarker(importing, STORAGE_KEYS.IMPORTING);

  const stopTaskPolling = useCallback(() => {
    if (pollTimerRef.current) {
      clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
    pollAttemptsRef.current = 0;
  }, []);

  // pollTaskUntilSettled 轮询任务直到终态，把结果写回日志面板。
  const pollTaskUntilSettled = useCallback(
    (taskId: number) => {
      stopTaskPolling();
      const tick = async () => {
        pollAttemptsRef.current += 1;
        try {
          const payload = await importTasksApi.fetchImportTask(taskId);
          if (payload.task && payload.task.status !== "running") {
            setLastTask(payload.task);
            if (payload.task.status === "completed") {
              addLog(
                "success",
                `后台导入任务 #${payload.task.id} 已完成：成功 ${payload.task.success_count}，待同步 ${payload.task.pending_count}，冲突 ${payload.task.conflict_count}，失败 ${payload.task.failed_count}`,
              );
            } else if (payload.task.status === "interrupted") {
              addLog(
                "error",
                `后台导入任务 #${payload.task.id} 因服务重启中断，已完成 ${payload.task.processed}/${payload.task.total}，可重新导入补齐`,
              );
            } else {
              addLog(
                "error",
                `后台导入任务 #${payload.task.id} 失败：${payload.task.error_message || "未知错误"}`,
              );
            }
            return;
          }
          if (payload.task) {
            setLastTask(payload.task);
          }
        } catch {
          // 查询失败不中断轮询，按预算继续。
        }
        if (pollAttemptsRef.current < TASK_POLL_MAX_ATTEMPTS) {
          pollTimerRef.current = setTimeout(tick, TASK_POLL_INTERVAL_MS);
        }
      };
      pollTimerRef.current = setTimeout(tick, TASK_POLL_INTERVAL_MS);
    },
    [addLog, stopTaskPolling],
  );

  const refreshLatestTask = useCallback(async () => {
    try {
      const payload = await importTasksApi.fetchLatestImportTask();
      setLastTask(payload.task);
      if (payload.task?.status === "running") {
        pollTaskUntilSettled(payload.task.id);
      }
      return payload.task;
    } catch {
      return null;
    }
  }, [pollTaskUntilSettled]);

  // 页面挂载时恢复最近一次导入任务视图（刷新/断线不丢任务）。
  useEffect(() => {
    void refreshLatestTask();
    return () => stopTaskPolling();
  }, [refreshLatestTask, stopTaskPolling]);

  const loadPreview = useCallback(async (selectedFile: File) => {
    setPreviewLoading(true);
    setPreviewError(null);
    try {
      const result = await importTasksApi.previewImportOPML(selectedFile);
      setPreview(result);
      setConfirmedUrls({});
    } catch (error: unknown) {
      setPreview(null);
      const message =
        error instanceof Error && error.message
          ? error.message
          : "预览失败，请检查文件后重试";
      setPreviewError(message);
    } finally {
      setPreviewLoading(false);
    }
  }, []);

  const handleFileChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      const selectedFile = event.target.files?.[0];
      if (!selectedFile) return;

      if (!isValidOpmlFile(selectedFile)) {
        toast.warning("请选择OPML或XML文件");
        return;
      }

      if (!isOpmlFileSizeAllowed(selectedFile)) {
        toast.warning(
          `OPML文件不能超过 ${(MAX_OPML_FILE_SIZE_BYTES / 1024 / 1024).toFixed(0)} MB`,
        );
        return;
      }

      setFile(selectedFile);
      setConfirmedUrls({});
      startLogSession("import");
      resetLogScroll();
      void loadPreview(selectedFile);
    },
    [loadPreview, resetLogScroll, startLogSession],
  );

  const toggleConfirmed = useCallback((entry: ImportPreviewEntry) => {
    if (!CONFIRMABLE_KINDS.has(entry.kind)) return;
    setConfirmedUrls((prev) => {
      const next = { ...prev };
      if (next[entry.xml_url]) {
        delete next[entry.xml_url];
      } else {
        next[entry.xml_url] = true;
      }
      return next;
    });
  }, []);

  const confirmAllPending = useCallback(() => {
    if (!preview) return;
    const next: Record<string, boolean> = {};
    for (const entry of preview.entries) {
      if (CONFIRMABLE_KINDS.has(entry.kind)) {
        next[entry.xml_url] = true;
      }
    }
    setConfirmedUrls(next);
  }, [preview]);

  const handleImport = useCallback(async () => {
    if (!file) {
      toast.warning("请先选择OPML文件");
      return;
    }

    await runExclusiveOperation(async () => {
      const confirmedCount = Object.values(confirmedUrls).filter(Boolean).length;
      const confirmationText = requestTypedConfirmation({
        action: `导入文件“${file.name}”`,
        impact:
          `会写入播客订阅数据，文件大小 ${(file.size / 1024).toFixed(1)} KB。` +
          (confirmedCount > 0 ? ` 将关联确认的 ${confirmedCount} 条待确认条目。` : ""),
        phrase: "IMPORT OPML",
      });
      if (!confirmationText) return;

      setImporting(true);
      resetLogScroll();
      startLogSession("import");

      // 只有收到 SSE task 事件才说明任务已建立；此前的失败（如文件/参数
      // 非法）没有后台任务，不得宣称“仍在后台执行”。
      let taskStarted = false;
      try {
        const decisions: Record<string, string> = {};
        for (const [url, confirmed] of Object.entries(confirmedUrls)) {
          if (confirmed) decisions[url] = "confirm";
        }
        await runSseOperation({
          mode: "import",
          addLog,
          startMessage: "开始导入OPML（本地匹配 + 在线同步）...",
          fallbackSuccessMessage: "导入完成",
          run: (onProgress) =>
            syncApi.importOPMLSSE(
              file,
              (type, message, current, total, data) => {
                if (type === "task" && data?.task_id) {
                  taskStarted = true;
                }
                onProgress(type, message, current, total, data);
              },
              confirmationText,
              Object.keys(decisions).length > 0 ? decisions : undefined,
            ),
        });
        // 导入完成后刷新最近任务视图（逐条结果与缓存刷新后的库状态）。
        stopTaskPolling();
        void refreshLatestTask();
      } catch (error) {
        console.error("导入失败:", error);

        buildImportErrorLogs(error).forEach((log) => {
          addLog(log.type, log.message);
        });
        if (taskStarted) {
          // 连接中断不代表任务失败：任务仍在后台执行，按预算轮询终态。
          toast.info("连接已中断，任务仍在后台执行，可稍后刷新查看结果");
          void refreshLatestTask();
        }
      } finally {
        setImporting(false);
      }
    });
  }, [
    addLog,
    confirmedUrls,
    file,
    refreshLatestTask,
    resetLogScroll,
    runExclusiveOperation,
    startLogSession,
    stopTaskPolling,
  ]);

  const handleRetry = useCallback(async () => {
    if (!lastTask || lastTask.status !== "completed") return;
    const retryable = lastTask.failed_count + lastTask.pending_count;
    if (retryable <= 0) {
      toast.info("没有失败或待同步的条目，无需重试");
      return;
    }
    const confirmationText = requestTypedConfirmation({
      action: `重试任务 #${lastTask.id} 中的失败/待同步条目`,
      impact: `只会重新处理 ${retryable} 条失败/待同步条目，其他节目不变。`,
      phrase: "RETRY IMPORT",
    });
    if (!confirmationText) return;

    try {
      const result = await importTasksApi.retryImportTask(lastTask.id);
      addLog(
        "info",
        `开始重试任务 #${lastTask.id} 中的 ${result.total_podcasts} 条条目`,
      );
      if (result.task_id) {
        addLog("info", `重试任务编号 #${result.task_id}，可刷新页面查看结果`);
      }
      addLog(
        "success",
        `重试完成：成功 ${result.success_count}，待同步 ${result.stub_podcasts}，失败 ${result.failed_count}`,
      );
      (result.errors || []).forEach((message) => addLog("error", message));
      stopTaskPolling();
      await refreshLatestTask();
    } catch (error: unknown) {
      const message =
        error instanceof Error && error.message ? error.message : "重试失败";
      addLog("error", message);
    }
  }, [addLog, lastTask, refreshLatestTask, stopTaskPolling]);

  const handleSync = useCallback(async () => {
    await runExclusiveOperation(async () => {
      const confirmationText = requestTypedConfirmation({
        action: "同步全部订阅播客",
        impact:
          "会刷新全部订阅播客的资料，并按各节目同步范围写入单集（可能新增或更新单集内容），可能耗时较长。",
        phrase: "SYNC ALL",
      });
      if (!confirmationText) return;

      setSyncing(true);
      resetLogScroll();
      startLogSession("sync");

      try {
        await runSseOperation({
          mode: "sync",
          addLog,
          startMessage: "开始同步所有播客的元数据...",
          fallbackSuccessMessage: "同步已完成",
          run: (onProgress) =>
            syncApi.syncPodcastsMetadataSSE(onProgress, confirmationText),
        });
      } catch (error) {
        console.error("同步失败:", error);
        addLog("error", buildSyncErrorMessage(error));
      } finally {
        setSyncing(false);
      }
    });
  }, [addLog, resetLogScroll, runExclusiveOperation, startLogSession]);

  return {
    file,
    importing,
    syncing,
    preview,
    previewLoading,
    previewError,
    confirmedUrls,
    lastTask,
    handleFileChange,
    handleImport,
    handleSync,
    toggleConfirmed,
    confirmAllPending,
    handleRetry,
  };
}
