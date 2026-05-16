import { LOG_CONFIG } from "@/lib/config";

export type LogType =
  | "info"
  | "success"
  | "error"
  | "progress"
  | "summary"
  | "complete"
  | "skip_paid"
  | "skip_cert"
  | "skip_not_found"
  | "skip_no_update"
  | "skip_access_denied"
  | "skip_geo_blocked"
  | "skip_duplicate"
  | "skip_invalid"
  | "skip_other";

export interface LogEntry {
  id: string;
  type: LogType;
  message: string;
  timestamp: string;
  current?: number;
  total?: number;
  reason?: string;
  data?: Record<string, any>;
}

export interface SyncStats {
  total: number;
  success: number;
  errors: number;
  skips: number;
  skipPaid: number;
  skipCert: number;
  skipNotFound: number;
  skipAccess: number;
  skipGeo: number;
  skipOther: number;
  skipNoUpdate: number;
  duration: string;
  fromSummary: boolean;
}

export type SyncLogFilter =
  | "all"
  | "errors"
  | "success"
  | "skips"
  | "no_update";
export type SyncLogMode = "import" | "sync";

export interface RestoredSyncLogSession {
  logs: LogEntry[];
  mode: SyncLogMode;
}

const KNOWN_LOG_TYPES = new Set<LogType>([
  "info",
  "success",
  "error",
  "progress",
  "summary",
  "complete",
  "skip_paid",
  "skip_cert",
  "skip_not_found",
  "skip_no_update",
  "skip_access_denied",
  "skip_geo_blocked",
  "skip_duplicate",
  "skip_invalid",
  "skip_other",
]);

export function createEmptySyncStats(): SyncStats {
  return {
    total: 0,
    success: 0,
    errors: 0,
    skips: 0,
    skipPaid: 0,
    skipCert: 0,
    skipNotFound: 0,
    skipAccess: 0,
    skipGeo: 0,
    skipOther: 0,
    skipNoUpdate: 0,
    duration: "",
    fromSummary: false,
  };
}

export function normalizeLogType(type: string): LogType {
  return KNOWN_LOG_TYPES.has(type as LogType) ? (type as LogType) : "info";
}

export function normalizeSyncLogMode(value: string | null): SyncLogMode | null {
  return value === "import" || value === "sync" ? value : null;
}

export function inferSyncLogMode(logs: LogEntry[]): SyncLogMode | null {
  const firstMessage = logs[0]?.message || "";
  if (firstMessage.includes("同步")) return "sync";
  if (firstMessage.includes("导入") || firstMessage.includes("OPML")) {
    return "import";
  }
  return null;
}

function numberOrZero(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

export function computeSyncStats(logs: LogEntry[]): SyncStats {
  const stats = createEmptySyncStats();

  for (const log of logs) {
    if (log.type === "summary" && log.data) {
      stats.total = numberOrZero(log.data.total_podcasts);
      stats.success = numberOrZero(log.data.success_podcasts);
      stats.errors = numberOrZero(log.data.failed_podcasts);
      stats.skips = numberOrZero(log.data.skipped_podcasts);
      stats.skipNoUpdate = numberOrZero(log.data.no_update_podcasts);
      stats.duration =
        typeof log.data.duration === "string" ? log.data.duration : "";
      stats.fromSummary = true;
      continue;
    }

    if (log.type === "success") {
      stats.success += 1;
      stats.total += 1;
      continue;
    }

    if (log.type === "error") {
      stats.errors += 1;
      stats.total += 1;
      continue;
    }

    if (!log.type.startsWith("skip_")) {
      continue;
    }

    stats.total += 1;

    switch (log.type) {
      case "skip_no_update":
        stats.skipNoUpdate += 1;
        break;
      case "skip_paid":
        stats.skips += 1;
        stats.skipPaid += 1;
        break;
      case "skip_cert":
        stats.skips += 1;
        stats.skipCert += 1;
        break;
      case "skip_not_found":
        stats.skips += 1;
        stats.skipNotFound += 1;
        break;
      case "skip_access_denied":
        stats.skips += 1;
        stats.skipAccess += 1;
        break;
      case "skip_geo_blocked":
        stats.skips += 1;
        stats.skipGeo += 1;
        break;
      default:
        stats.skips += 1;
        stats.skipOther += 1;
        break;
    }
  }

  return stats;
}

export function createLogEntry(
  type: LogType,
  message: string,
  current?: number,
  total?: number,
  data?: Record<string, any>,
): LogEntry {
  const entry: LogEntry = {
    id: Date.now() + Math.random().toString(),
    type,
    message,
    timestamp: new Date().toLocaleTimeString(),
  };

  if (current !== undefined) entry.current = current;
  if (total !== undefined) entry.total = total;
  if (data !== undefined) entry.data = data;

  return entry;
}

export function trimLogs(
  logs: LogEntry[],
  maxLogs: number = LOG_CONFIG.MAX_LOGS,
): LogEntry[] {
  if (logs.length <= maxLogs) return logs;
  return logs.slice(logs.length - maxLogs);
}

export function appendLogEntry(logs: LogEntry[], entry: LogEntry): LogEntry[] {
  return trimLogs([...logs, entry]);
}

export function filterSyncLogs(
  logs: LogEntry[],
  filter: SyncLogFilter,
): LogEntry[] {
  if (filter === "all") return logs;
  if (filter === "errors") return logs.filter((log) => log.type === "error");
  if (filter === "success") {
    return logs.filter((log) => log.type === "success");
  }
  if (filter === "skips") {
    return logs.filter((log) => {
      return log.type.startsWith("skip_") && log.type !== "skip_no_update";
    });
  }
  if (filter === "no_update") {
    return logs.filter((log) => log.type === "skip_no_update");
  }

  return logs;
}

export function parseSavedLogs(value: string | null): LogEntry[] {
  if (!value) return [];

  try {
    const parsed = JSON.parse(value);
    if (!Array.isArray(parsed)) return [];

    return parsed
      .filter((item): item is Record<string, any> => {
        return (
          item !== null &&
          typeof item === "object" &&
          typeof item.message === "string"
        );
      })
      .map((item) => ({
        id:
          typeof item.id === "string"
            ? item.id
            : `${Date.now()}-${Math.random()}`,
        type: normalizeLogType(
          typeof item.type === "string" ? item.type : "info",
        ),
        message: item.message,
        timestamp:
          typeof item.timestamp === "string"
            ? item.timestamp
            : new Date().toLocaleTimeString(),
        ...(typeof item.current === "number" ? { current: item.current } : {}),
        ...(typeof item.total === "number" ? { total: item.total } : {}),
        ...(typeof item.reason === "string" ? { reason: item.reason } : {}),
        ...(item.data && typeof item.data === "object"
          ? { data: item.data }
          : {}),
      }));
  } catch {
    return [];
  }
}

export function restoreSyncLogSession({
  savedLogs,
  savedLogMode,
  wasSyncing,
  wasImporting,
}: {
  savedLogs: string | null;
  savedLogMode: string | null;
  wasSyncing: boolean;
  wasImporting: boolean;
}): RestoredSyncLogSession {
  const restoredLogs = parseSavedLogs(savedLogs);
  let mode =
    normalizeSyncLogMode(savedLogMode) ||
    inferSyncLogMode(restoredLogs) ||
    "import";

  if (wasSyncing) {
    mode = "sync";
    restoredLogs.push(createLogEntry("info", "页面已刷新，上次同步状态已丢失"));
  }

  if (wasImporting) {
    mode = "import";
    restoredLogs.push(createLogEntry("info", "页面已刷新，导入需要重新开始"));
  }

  return {
    logs: trimLogs(restoredLogs),
    mode,
  };
}
