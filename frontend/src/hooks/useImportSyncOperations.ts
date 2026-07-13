import type { ChangeEvent } from "react";
import { useCallback, useEffect, useState } from "react";
import { syncApi } from "@/lib/api";
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

  useStoredOperationMarker(syncing, STORAGE_KEYS.SYNCING);
  useStoredOperationMarker(importing, STORAGE_KEYS.IMPORTING);

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
      const confirmationText = requestTypedConfirmation({
        action: `导入文件“${file.name}”`,
        impact: `会写入播客订阅数据，文件大小 ${(file.size / 1024).toFixed(1)} KB。`,
        phrase: "IMPORT OPML",
      });
      if (!confirmationText) return;

      setImporting(true);
      resetLogScroll();
      startLogSession("import");

      try {
        await runSseOperation({
          mode: "import",
          addLog,
          startMessage: "开始导入OPML（本地匹配模式）...",
          fallbackSuccessMessage: "导入完成",
          run: (onProgress) =>
            syncApi.importOPMLSSE(file, onProgress, confirmationText),
        });
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
      const confirmationText = requestTypedConfirmation({
        action: "同步全部订阅播客",
        impact: "会刷新全部订阅播客的元数据并发起网络请求，可能耗时较长。",
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
    handleFileChange,
    handleImport,
    handleSync,
  };
}
