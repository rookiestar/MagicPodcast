import type { RefObject } from "react";
import type { LogEntry, SyncLogFilter, SyncStats } from "@/lib/syncLogState";
import SyncLogEntryRow from "./SyncLogEntryRow";
import SyncLogFilters from "./SyncLogFilters";
import SyncLogStats from "./SyncLogStats";

interface SyncLogPanelProps {
  title: string;
  logs: LogEntry[];
  filteredLogs: LogEntry[];
  stats: SyncStats;
  filter: SyncLogFilter;
  isRunning: boolean;
  autoScroll: boolean;
  onFilterChange: (filter: SyncLogFilter) => void;
  onLogScroll: () => void;
  onResumeAutoScroll: () => void;
  onClearLogs: () => void;
  logContainerRef: RefObject<HTMLDivElement>;
  logEndRef: RefObject<HTMLDivElement>;
}

function hasVisibleStats(stats: SyncStats) {
  return (
    stats.total > 0 ||
    stats.errors > 0 ||
    stats.success > 0 ||
    stats.skips > 0 ||
    stats.skipNoUpdate > 0
  );
}

export default function SyncLogPanel({
  title,
  logs,
  filteredLogs,
  stats,
  filter,
  isRunning,
  autoScroll,
  onFilterChange,
  onLogScroll,
  onResumeAutoScroll,
  onClearLogs,
  logContainerRef,
  logEndRef,
}: SyncLogPanelProps) {
  const hasLogs = logs.length > 0;
  const hasFilteredLogs = filteredLogs.length > 0;
  const hasStats = hasVisibleStats(stats);

  return (
    <div className="rounded-lg border border-slate-200 bg-white p-4 dark:border-slate-700 dark:bg-slate-900">
      <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50">
              {title}
            </h3>
            {isRunning && (
              <span className="inline-flex items-center rounded-full bg-blue-100 px-2.5 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
                进行中
              </span>
            )}
          </div>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            {!hasLogs
              ? "等待开始"
              : filteredLogs.length === logs.length
                ? `共 ${logs.length} 条记录`
                : `显示 ${filteredLogs.length} / ${logs.length} 条记录`}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          {!autoScroll && hasLogs && (
            <button
              type="button"
              onClick={onResumeAutoScroll}
              className="cursor-pointer rounded-md px-2 py-1 text-sm text-blue-600 hover:text-blue-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 dark:text-blue-400 dark:hover:text-blue-300"
              title="恢复自动滚动"
            >
              恢复自动滚动
            </button>
          )}
          {!isRunning && hasLogs && (
            <button
              type="button"
              onClick={onClearLogs}
              className="cursor-pointer rounded-md px-2 py-1 text-sm text-slate-500 hover:text-slate-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 dark:text-slate-400 dark:hover:text-slate-300"
            >
              清空日志
            </button>
          )}
        </div>
      </div>

      {hasLogs && (
        <SyncLogFilters
          logs={logs}
          filter={filter}
          onFilterChange={onFilterChange}
        />
      )}

      {hasStats && <SyncLogStats stats={stats} />}

      <div
        ref={logContainerRef}
        tabIndex={0}
        aria-label={`${title}内容`}
        aria-live={autoScroll ? "polite" : "off"}
        onScroll={onLogScroll}
        className="max-h-96 space-y-2 overflow-y-auto pr-1 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500"
      >
        {hasFilteredLogs ? (
          filteredLogs.map((log) => (
            <SyncLogEntryRow key={log.id} log={log} />
          ))
        ) : (
          <div className="rounded-md border border-dashed border-slate-200 bg-slate-50 px-4 py-6 text-center text-sm text-slate-500 dark:border-slate-700 dark:bg-slate-900/70 dark:text-slate-400">
            {hasLogs ? "当前筛选下没有日志" : "开始后这里会显示实时日志"}
          </div>
        )}
        <div ref={logEndRef} />
      </div>

      {isRunning && autoScroll && hasLogs && (
        <div className="mt-2 flex items-center justify-center gap-2 text-center text-xs text-gray-500">
          <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-blue-500" />
          正在自动跟随最新日志
        </div>
      )}
    </div>
  );
}
