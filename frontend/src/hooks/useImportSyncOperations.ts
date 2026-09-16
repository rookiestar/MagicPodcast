import type { ChangeEvent } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import { syncApi } from "@/lib/api";
import {
  CONFIRMABLE_KINDS,
  importTasksApi,
  type ImportPreview,
  type ImportPreviewEntry,
  type ImportTask,
  type ImportEntryResult,
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

// 与后端 retryableOutcomes 同口径：failed/pending 条目可重试，中断任务
// 的 unprocessed 条目继续处理；冲突条目仅在显式确认后参与，不计入这里。
export function countRetryableEntries(
  task: ImportTask,
  entries: ImportEntryResult[],
): number {
  return entries.filter(
    (entry) =>
      entry.outcome === "failed" ||
      entry.outcome === "pending" ||
      (task.status === "interrupted" && entry.outcome === "unprocessed"),
  ).length;
}

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
  const [taskEntries, setTaskEntries] = useState<ImportEntryResult[]>([]);
  // 最近任务读取失败与“没有历史任务”是两种状态：失败时保留最后已知结果。
  const [latestTaskError, setLatestTaskError] = useState(false);
  const previewRequestRef = useRef(0);
  const [previewConsumed, setPreviewConsumed] = useState(false);
  const [backgroundTaskId, setBackgroundTaskId] = useState<number | null>(null);
  // 作废已停止轮询或被新读取替代的在途响应，避免旧任务覆盖当前结果。
  const taskRequestRef = useRef(0);
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pollAttemptsRef = useRef(0);

  const runExclusiveOperation = useExclusiveAsyncAction({
    isBlocked: importing || syncing || backgroundTaskId !== null || lastTask?.status === "running",
  });

  useStoredOperationMarker(syncing, STORAGE_KEYS.SYNCING);
  useStoredOperationMarker(importing, STORAGE_KEYS.IMPORTING);

  const stopTaskPolling = useCallback(() => {
    taskRequestRef.current += 1;
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
      const requestId = taskRequestRef.current;
      let lastLoggedProgress = -1;
      const tick = async () => {
        pollAttemptsRef.current += 1;
        try {
          const payload = await importTasksApi.fetchImportTask(taskId);
          if (requestId !== taskRequestRef.current) return;
          if (!payload.task) throw new Error("任务记录不可用");
          setLatestTaskError(false);
          setTaskEntries(payload.entries);
          if (payload.task && payload.task.status !== "running") {
            setBackgroundTaskId(null);
            setLastTask(payload.task);
            if (payload.task.status === "completed") {
              addLog(
                "summary",
                `后台导入任务 #${payload.task.id} 已完成：成功 ${payload.task.success_count}，待同步 ${payload.task.pending_count}，冲突 ${payload.task.conflict_count}，失败 ${payload.task.failed_count}`,
                undefined, undefined, {
                  operation: "import",
                  total_podcasts: payload.task.total,
                  success_podcasts: payload.task.success_count,
                  failed_podcasts: payload.task.failed_count,
                  stub_podcasts: payload.task.pending_count,
                  conflict_podcasts: payload.task.conflict_count,
                  merged_podcasts: payload.task.merged_count,
                  unchanged_podcasts: payload.task.unchanged_count,
                  skipped_podcasts: payload.task.skipped_count,
                },
              );
            } else if (payload.task.status === "interrupted") {
              addLog(
                "error",
                `后台导入任务 #${payload.task.id} 因服务重启中断，已记录 ${payload.task.processed}/${payload.task.total} 条结果，请核对逐项结果后继续`,
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
            if (payload.task.processed !== lastLoggedProgress) {
              lastLoggedProgress = payload.task.processed;
              addLog("progress", `已处理 ${payload.task.processed}/${payload.task.total} 条`,
                payload.task.processed, payload.task.total);
            }
          }
        } catch {
          if (requestId !== taskRequestRef.current) return;
          setLatestTaskError(true);
        }
        if (pollAttemptsRef.current < TASK_POLL_MAX_ATTEMPTS) {
          pollTimerRef.current = setTimeout(tick, TASK_POLL_INTERVAL_MS);
        } else {
          setLatestTaskError(true);
        }
      };
      pollTimerRef.current = setTimeout(tick, TASK_POLL_INTERVAL_MS);
    },
    [addLog, stopTaskPolling],
  );

  const refreshLatestTask = useCallback(async () => {
    stopTaskPolling();
    const requestId = taskRequestRef.current;
    try {
      const payload = await importTasksApi.fetchLatestImportTask();
      if (requestId !== taskRequestRef.current) return null;
      setLatestTaskError(false);
      setBackgroundTaskId(null);
      setLastTask(payload.task);
      setTaskEntries(payload.entries);
      if (payload.task?.status === "running") {
        pollTaskUntilSettled(payload.task.id);
      }
      return payload.task;
    } catch {
      if (requestId !== taskRequestRef.current) return null;
      // 读取失败不是“没有任务”：保留最后已知结果，由界面提供重试读取。
      setLatestTaskError(true);
      return null;
    }
  }, [pollTaskUntilSettled, stopTaskPolling]);

  // 页面挂载时恢复最近一次导入任务视图（刷新/断线不丢任务）。
  useEffect(() => {
    void refreshLatestTask();
    return () => stopTaskPolling();
  }, [refreshLatestTask, stopTaskPolling]);

  const loadPreview = useCallback(async (selectedFile: File) => {
    const requestId = ++previewRequestRef.current;
    setPreview(null);
    setPreviewConsumed(false);
    setPreviewLoading(true);
    setPreviewError(null);
    try {
      const result = await importTasksApi.previewImportOPML(selectedFile);
      if (requestId !== previewRequestRef.current) return;
      setPreview(result);
      setConfirmedUrls({});
    } catch (error: unknown) {
      if (requestId !== previewRequestRef.current) return;
      setPreview(null);
      const message =
        error instanceof Error && error.message
          ? error.message
          : "预览失败，请检查文件后重试";
      setPreviewError(message);
    } finally {
      if (requestId === previewRequestRef.current) setPreviewLoading(false);
    }
  }, []);

  // 预览失败后直接重试当前选中文件的预览，不需要重新选择文件。
  const retryPreview = useCallback(() => {
    if (!file) return;
    void loadPreview(file);
  }, [file, loadPreview]);

  const handleFileChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      const selectedFile = event.target.files?.[0];
      // 保留状态中的已选文件，同时让下一次选择同文件也触发 change。
      event.target.value = "";
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

  // 提交约束与按钮展示使用同一条件：必须存在当前文件的一次成功预览。
  // 预览读取中或失败时不允许进入正式导入，避免提交与核对对象不一致。
  const canSubmitImport = Boolean(
    file && preview && !previewLoading && !previewError,
  );

  const handleImport = useCallback(async () => {
    if (!file) {
      toast.warning("请先选择OPML文件");
      return;
    }
    // 处理函数在按钮之外也检查同一状态：直接重复激活不能越过预览约束。
    if (!preview || previewLoading || previewError) {
      toast.warning("请先完成当前文件的预览核对，再开始导入");
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
      stopTaskPolling();
      resetLogScroll();
      startLogSession("import");

      // 只有收到 SSE task 事件才说明任务已建立；此前的失败（如文件/参数
      // 非法）没有后台任务，不得宣称“仍在后台执行”。
      let startedTaskId: number | null = null;
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
                  startedTaskId = Number(data.task_id);
                  setBackgroundTaskId(startedTaskId);
                  setPreviewConsumed(true);
                }
                onProgress(type, message, current, total, data);
              },
              confirmationText,
              Object.keys(decisions).length > 0 ? decisions : undefined,
            ),
        });
        // 导入完成后刷新最近任务视图（逐条结果与缓存刷新后的库状态）。
        stopTaskPolling();
        setPreviewConsumed(true);
        await refreshLatestTask();
      } catch (error) {
        console.error("导入失败:", error);

        buildImportErrorLogs(error).forEach((log) => {
          addLog(log.type, log.message);
        });
        if (startedTaskId) {
          // 连接中断不代表任务失败：任务仍在后台执行，按预算轮询终态。
          toast.info("连接已中断，任务仍在后台执行，可稍后刷新查看结果");
          pollTaskUntilSettled(startedTaskId);
        }
      } finally {
        setImporting(false);
      }
    });
  }, [
    addLog,
    confirmedUrls,
    file,
    preview,
    previewError,
    previewLoading,
    refreshLatestTask,
    resetLogScroll,
    runExclusiveOperation,
    startLogSession,
    stopTaskPolling,
    pollTaskUntilSettled,
  ]);

  const handleRetry = useCallback(async (conflictEntry?: ImportEntryResult) => {
    if (!lastTask || lastTask.status === "running") return;
    const retryable = conflictEntry ? 1 : countRetryableEntries(lastTask, taskEntries);
    if (retryable <= 0) {
      toast.info("没有失败或待同步的条目，无需重试");
      return;
    }
    const confirmationText = requestTypedConfirmation({
      action: `重试任务 #${lastTask.id} 中的失败/待同步条目`,
      impact: conflictEntry ? `${conflictEntry.title}：${conflictEntry.detail}。确认后将复用已有节目并绑定本次地址，同时重试失败或待同步条目。` : `重新核对并处理 ${retryable} 条失败、待同步或未完成条目。`,
      phrase: "RETRY IMPORT",
    });
    if (!confirmationText) return;

    await runExclusiveOperation(async () => {
    setImporting(true);
    try {
      const result = await importTasksApi.retryImportTask(lastTask.id, conflictEntry ? { [conflictEntry.feed_url]: "confirm" } : undefined);
      addLog(
        "info",
        `开始重试任务 #${lastTask.id} 中的 ${result.total_podcasts} 条条目`,
      );
      if (result.task_id) {
        addLog("info", `重试任务编号 #${result.task_id}，可刷新页面查看结果`);
      }
      addLog(
        "summary",
        `重试完成：成功 ${result.success_count}，待同步 ${result.stub_podcasts}，失败 ${result.failed_count}`,
        undefined, undefined, {
          operation: "import", total_podcasts: result.total_podcasts,
          success_podcasts: result.success_count, failed_podcasts: result.failed_count,
          stub_podcasts: result.stub_podcasts, conflict_podcasts: result.conflict_podcasts,
          merged_podcasts: result.merged_podcasts, unchanged_podcasts: result.unchanged_podcasts,
        },
      );
      (result.errors || []).forEach((message) => addLog("error", message));
      stopTaskPolling();
      await refreshLatestTask();
    } catch (error: unknown) {
      const message =
        error instanceof Error && error.message ? error.message : "重试失败";
      addLog("error", message);
      await refreshLatestTask();
    } finally {
      setImporting(false);
    }
    });
  }, [addLog, lastTask, taskEntries, refreshLatestTask, stopTaskPolling, runExclusiveOperation]);

  const handleSync = useCallback(async () => {
    await runExclusiveOperation(async () => {
      const confirmationText = requestTypedConfirmation({
        action: "同步已关注节目",
        impact:
          "会检查全部已关注节目的 RSS，更新节目资料，并按各节目的同步范围新增或更新单集，可能耗时较长。",
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
          startMessage: "开始同步已关注节目的元数据...",
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
    importing: importing || backgroundTaskId !== null || lastTask?.status === "running",
    syncing,
    preview,
    previewLoading,
    previewError,
    previewConsumed,
    canSubmitImport,
    confirmedUrls,
    lastTask,
    taskEntries,
    latestTaskError,
    countRetryableEntries,
    handleFileChange,
    handleImport,
    handleSync,
    toggleConfirmed,
    confirmAllPending,
    handleRetry,
    retryPreview,
    refreshLatestTask,
  };
}
