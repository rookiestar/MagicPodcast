import { useCallback, useEffect, useMemo, useState } from "react";
import {
  readStorageValue,
  removeStorageValue,
  writeStorageValue,
} from "@/lib/browserStorage";
import { STORAGE_KEYS } from "@/lib/config";
import {
  appendLogEntry,
  computeSyncStats,
  createLogEntry,
  filterSyncLogs,
  restoreSyncLogSession,
  trimLogs,
  type LogEntry,
  type LogType,
  type SyncLogFilter,
  type SyncLogMode,
} from "@/lib/syncLogState";

export function useSyncLogSession() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [logMode, setLogMode] = useState<SyncLogMode>("import");
  const [restoredMode, setRestoredMode] = useState<SyncLogMode | null>(null);
  const [filter, setFilter] = useState<SyncLogFilter>("all");
  const [storageReady, setStorageReady] = useState(false);

  const stats = useMemo(() => computeSyncStats(logs), [logs]);
  const filteredLogs = useMemo(
    () => filterSyncLogs(logs, filter),
    [logs, filter],
  );

  useEffect(() => {
    const restoredSession = restoreSyncLogSession({
      savedLogs: readStorageValue(STORAGE_KEYS.SYNC_LOGS),
      savedLogMode: readStorageValue(STORAGE_KEYS.SYNC_LOG_MODE),
      wasSyncing: readStorageValue(STORAGE_KEYS.SYNCING) === "true",
      wasImporting: readStorageValue(STORAGE_KEYS.IMPORTING) === "true",
    });

    if (restoredSession.logs.length > 0) {
      setLogs(restoredSession.logs);
      setLogMode(restoredSession.mode);
      setRestoredMode(restoredSession.mode);
    }
    setStorageReady(true);
  }, []);

  useEffect(() => {
    if (!storageReady) return;

    if (logs.length === 0) {
      removeStorageValue(STORAGE_KEYS.SYNC_LOGS);
      removeStorageValue(STORAGE_KEYS.SYNC_LOG_MODE);
      return;
    }

    writeStorageValue(STORAGE_KEYS.SYNC_LOGS, JSON.stringify(trimLogs(logs)));
    writeStorageValue(STORAGE_KEYS.SYNC_LOG_MODE, logMode);
  }, [logs, logMode, storageReady]);

  const addLog = useCallback(
    (
      type: LogType,
      message: string,
      current?: number,
      total?: number,
      data?: Record<string, any>,
    ) => {
      const newLog = createLogEntry(type, message, current, total, data);
      setLogs((prev) => appendLogEntry(prev, newLog));
    },
    [],
  );

  const startLogSession = useCallback((mode: SyncLogMode) => {
    setLogMode(mode);
    setLogs([]);
    setFilter("all");
  }, []);

  const clearLogSession = useCallback((mode: SyncLogMode) => {
    setLogMode(mode);
    setLogs([]);
    setFilter("all");
    removeStorageValue(STORAGE_KEYS.SYNC_LOGS);
    removeStorageValue(STORAGE_KEYS.SYNC_LOG_MODE);
  }, []);

  return {
    logs,
    logMode,
    filter,
    setFilter,
    restoredMode,
    stats,
    filteredLogs,
    addLog,
    startLogSession,
    clearLogSession,
  };
}
