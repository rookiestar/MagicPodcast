import type { ChangeEvent } from "react";
import { useCallback, useEffect, useState } from "react";
import { syncApi } from "@/lib/api";
import { removeStorageValue, writeStorageValue } from "@/lib/browserStorage";
import { STORAGE_KEYS } from "@/lib/config";
import { isValidOpmlFile } from "@/lib/importFileValidation";
import {
  buildImportErrorLogs,
  buildSyncErrorMessage,
  isOperationCompletionEvent,
} from "@/lib/syncOperationMessages";
import {
  normalizeLogType,
  type LogType,
  type SyncLogMode,
} from "@/lib/syncLogState";
import { toast } from "@/lib/toast";
import { useExclusiveAsyncAction } from "./useExclusiveAsyncAction";

type AddLog = (
  type: LogType,
  message: string,
  current?: number,
  total?: number,
  data?: Record<string, any>,
) => void;

interface UseImportSyncOperationsOptions {
  addLog: AddLog;
  resetLogScroll: () => void;
  startLogSession: (mode: SyncLogMode) => void;
}

export function useImportSyncOperations({
  addLog,
  resetLogScroll,
  startLogSession,
}: UseImportSyncOperationsOptions) {
  const [file, setFile] = useState<File | null>(null);
  const [importing, setImporting] = useState(false);
  const [syncing, setSyncing] = useState(false);

  const runExclusiveOperation = useExclusiveAsyncAction({
    isBlocked: importing || syncing,
  });

  useEffect(() => {
    if (syncing) {
      writeStorageValue(STORAGE_KEYS.SYNCING, "true");
    } else {
      removeStorageValue(STORAGE_KEYS.SYNCING);
    }
  }, [syncing]);

  useEffect(() => {
    if (importing) {
      writeStorageValue(STORAGE_KEYS.IMPORTING, "true");
    } else {
      removeStorageValue(STORAGE_KEYS.IMPORTING);
    }
  }, [importing]);

  const handleFileChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      const selectedFile = event.target.files?.[0];
      if (!selectedFile) return;

      if (!isValidOpmlFile(selectedFile)) {
        toast.warning("请选择OPML或XML文件");
        return;
      }

      setFile(selectedFile);
      startLogSession("import");
      resetLogScroll();
    },
    [resetLogScroll, startLogSession],
  );

  const handleImport = useCallback(async () => {
    if (!file) {
      toast.warning("请先选择OPML文件");
      return;
    }

    await runExclusiveOperation(async () => {
      setImporting(true);
      resetLogScroll();
      startLogSession("import");

      addLog("info", "开始导入OPML（本地匹配模式）...");

      let receivedSummary = false;

      try {
        await syncApi.importOPMLSSE(file, (type, message, current, total) => {
          addLog(normalizeLogType(type), message, current, total);
          if (isOperationCompletionEvent("import", type, message)) {
            receivedSummary = true;
          }
        });

        if (!receivedSummary) {
          addLog("success", "导入完成");
        }
      } catch (error) {
        console.error("导入失败:", error);

        buildImportErrorLogs(error).forEach((log) => {
          addLog(log.type, log.message);
        });
      } finally {
        setImporting(false);
      }
    });
  }, [addLog, file, resetLogScroll, runExclusiveOperation, startLogSession]);

  const handleSync = useCallback(async () => {
    await runExclusiveOperation(async () => {
      setSyncing(true);
      resetLogScroll();
      startLogSession("sync");

      addLog("info", "开始同步所有播客的元数据...");

      let receivedSummary = false;

      try {
        await syncApi.syncPodcastsMetadataSSE(
          (type, message, current, total, data) => {
            addLog(normalizeLogType(type), message, current, total, data);
            if (isOperationCompletionEvent("sync", type, message)) {
              receivedSummary = true;
            }
          },
        );

        if (!receivedSummary) {
          addLog("success", "同步已完成");
        }
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
    handleFileChange,
    handleImport,
    handleSync,
  };
}
