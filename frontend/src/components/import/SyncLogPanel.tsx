import type { RefObject } from "react";
import {
  filterSyncLogs,
  type LogEntry,
  type SyncLogFilter,
  type SyncStats,
} from "@/lib/syncLogState";

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

function getLogLabel(type: LogEntry["type"]) {
  switch (type) {
    case "success":
      return "成功";
    case "error":
      return "失败";
    case "progress":
      return "进度";
    case "summary":
      return "汇总";
    case "complete":
      return "完成";
    case "skip_paid":
    case "skip_cert":
    case "skip_not_found":
    case "skip_access_denied":
    case "skip_geo_blocked":
    case "skip_duplicate":
    case "skip_invalid":
    case "skip_other":
      return "跳过";
    case "skip_no_update":
      return "无更新";
    default:
      return "信息";
  }
}

function getLogBadgeClass(type: LogEntry["type"]) {
  switch (type) {
    case "success":
      return "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300";
    case "error":
      return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300";
    case "progress":
      return "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300";
    case "summary":
      return "bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-300";
    case "complete":
      return "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300";
    default:
      if (type.startsWith("skip_")) {
        return "bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300";
      }
      return "bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300";
  }
}

function getLogRowClass(type: LogEntry["type"]) {
  switch (type) {
    case "success":
      return "border-green-100 bg-green-50/70 dark:border-green-900/40 dark:bg-green-950/20";
    case "error":
      return "border-red-100 bg-red-50/80 dark:border-red-900/40 dark:bg-red-950/25";
    case "progress":
      return "border-blue-100 bg-blue-50/70 dark:border-blue-900/40 dark:bg-blue-950/20";
    case "summary":
      return "border-violet-100 bg-violet-50/70 dark:border-violet-900/40 dark:bg-violet-950/20";
    default:
      return "border-slate-100 bg-white dark:border-slate-800 dark:bg-slate-900";
  }
}

function getLogTypeLabel(type: LogEntry["type"]) {
  switch (type) {
    case "skip_paid":
      return "付费播客";
    case "skip_cert":
      return "证书过期";
    case "skip_not_found":
      return "不存在";
    case "skip_no_update":
      return "无更新";
    case "skip_access_denied":
      return "访问拒绝";
    case "skip_geo_blocked":
      return "地区限制";
    case "skip_duplicate":
      return "重复";
    case "skip_invalid":
      return "格式无效";
    case "skip_other":
      return "其他";
    default:
      return "";
  }
}

function FilterButton({
  active,
  children,
  tone,
  onClick,
}: {
  active: boolean;
  children: React.ReactNode;
  tone: "blue" | "green" | "red" | "amber" | "slate";
  onClick: () => void;
}) {
  const activeClass = {
    blue: "border-blue-500 bg-blue-50 text-blue-700 dark:border-blue-600 dark:bg-blue-900/30 dark:text-blue-300",
    green:
      "border-green-500 bg-green-50 text-green-700 dark:border-green-600 dark:bg-green-900/30 dark:text-green-300",
    red: "border-red-500 bg-red-50 text-red-700 dark:border-red-600 dark:bg-red-900/30 dark:text-red-300",
    amber:
      "border-amber-500 bg-amber-50 text-amber-700 dark:border-amber-600 dark:bg-amber-900/30 dark:text-amber-300",
    slate:
      "border-slate-500 bg-slate-100 text-slate-700 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-300",
  }[tone];

  return (
    <button
      type="button"
      onClick={onClick}
      className={`min-h-[44px] cursor-pointer rounded-lg border px-3 py-2 text-sm transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
        active
          ? activeClass
          : "border-slate-300 bg-white text-slate-700 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
      }`}
    >
      {children}
    </button>
  );
}

function StatCard({
  value,
  label,
  className = "text-slate-900 dark:text-slate-50",
}: {
  value: number;
  label: string;
  className?: string;
}) {
  return (
    <div className="rounded-lg bg-slate-50 p-4 dark:bg-slate-900">
      <p className={`text-2xl font-bold ${className}`}>{value}</p>
      <p className="text-sm text-slate-600 dark:text-slate-400">{label}</p>
    </div>
  );
}

function LogEntryRow({ log }: { log: LogEntry }) {
  return (
    <div
      role={log.type === "error" ? "alert" : undefined}
      className={`rounded-md border px-3 py-2 text-xs ${getLogRowClass(log.type)}`}
    >
      <div className="flex flex-col gap-1 sm:flex-row sm:items-start">
        <span className="font-mono text-[11px] text-slate-400 sm:w-20 sm:flex-none">
          [{log.timestamp}]
        </span>
        <span
          className={`inline-flex w-fit flex-none rounded-full px-2 py-0.5 text-[11px] font-medium ${getLogBadgeClass(log.type)}`}
        >
          {getLogLabel(log.type)}
        </span>
        {log.type === "summary" && log.data ? (
          <div className="min-w-0 flex-1 space-y-1 text-slate-700 dark:text-slate-200">
            <p className="font-semibold">同步完成</p>
            <p>
              播客统计: 总计 {log.data.total_podcasts} | 成功{" "}
              {log.data.success_podcasts} | 失败 {log.data.failed_podcasts} |
              跳过 {log.data.skipped_podcasts} | 无更新{" "}
              {log.data.no_update_podcasts || 0}
            </p>
            <p>
              单集统计: 总处理 {log.data.total_episodes || 0} | 新增{" "}
              {log.data.new_episodes || 0} | 更新{" "}
              {log.data.updated_episodes || 0}
              {log.data.duration && <span> | 耗时: {log.data.duration}</span>}
            </p>
          </div>
        ) : (
          <p className="min-w-0 flex-1 break-words text-slate-700 dark:text-slate-200">
            {log.type === "progress" &&
            log.current !== undefined &&
            log.total !== undefined ? (
              <span className="font-mono text-slate-500 dark:text-slate-400">
                [{log.current}/{log.total}]{" "}
              </span>
            ) : null}
            {log.type.startsWith("skip_") && (
              <span className="font-semibold">
                [{getLogTypeLabel(log.type)}]{" "}
              </span>
            )}
            {log.message}
          </p>
        )}
      </div>
    </div>
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
  const hasStats =
    stats.total > 0 ||
    stats.errors > 0 ||
    stats.success > 0 ||
    stats.skips > 0 ||
    stats.skipNoUpdate > 0;
  const filterCounts = {
    all: logs.length,
    success: filterSyncLogs(logs, "success").length,
    errors: filterSyncLogs(logs, "errors").length,
    skips: filterSyncLogs(logs, "skips").length,
    noUpdate: filterSyncLogs(logs, "no_update").length,
  };

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
        <div className="mb-4 grid grid-cols-2 gap-2 sm:grid-cols-5">
          <FilterButton
            active={filter === "all"}
            tone="blue"
            onClick={() => onFilterChange("all")}
          >
            全部 ({filterCounts.all})
          </FilterButton>
          <FilterButton
            active={filter === "success"}
            tone="green"
            onClick={() => onFilterChange("success")}
          >
            成功 ({filterCounts.success})
          </FilterButton>
          <FilterButton
            active={filter === "errors"}
            tone="red"
            onClick={() => onFilterChange("errors")}
          >
            失败 ({filterCounts.errors})
          </FilterButton>
          <FilterButton
            active={filter === "skips"}
            tone="slate"
            onClick={() => onFilterChange("skips")}
          >
            跳过 ({filterCounts.skips})
          </FilterButton>
          <FilterButton
            active={filter === "no_update"}
            tone="amber"
            onClick={() => onFilterChange("no_update")}
          >
            无更新 ({filterCounts.noUpdate})
          </FilterButton>
        </div>
      )}

      {hasStats && (
        <div className="mb-4">
          {stats.fromSummary && stats.duration && (
            <div className="mb-3 text-xs font-medium text-blue-600 dark:text-blue-400">
              总耗时: {stats.duration}
            </div>
          )}

          <div className="grid grid-cols-2 gap-4 md:grid-cols-5">
            <StatCard value={stats.total} label="总计" />
            <StatCard
              value={stats.success}
              label="成功"
              className="text-green-600 dark:text-green-400"
            />
            <StatCard
              value={stats.errors}
              label="失败"
              className="text-red-600 dark:text-red-400"
            />
            <StatCard
              value={stats.skips}
              label="跳过"
              className="text-slate-600 dark:text-slate-400"
            />
            <StatCard
              value={stats.skipNoUpdate}
              label="无更新"
              className="text-amber-600 dark:text-amber-400"
            />
          </div>

          {stats.skips > 0 && (
            <div className="mt-3 flex flex-wrap gap-2 text-xs text-slate-600 dark:text-slate-400">
              <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
                付费 {stats.skipPaid}
              </span>
              <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
                证书 {stats.skipCert}
              </span>
              <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
                不存在 {stats.skipNotFound}
              </span>
              <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
                访问拒绝 {stats.skipAccess}
              </span>
              <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
                地区限制 {stats.skipGeo}
              </span>
              <span className="rounded-full bg-slate-100 px-2 py-1 dark:bg-slate-800">
                其他 {stats.skipOther}
              </span>
            </div>
          )}

        </div>
      )}

      <div
        ref={logContainerRef}
        tabIndex={0}
        aria-label={`${title}内容`}
        aria-live={autoScroll ? "polite" : "off"}
        onScroll={onLogScroll}
        className="max-h-96 space-y-2 overflow-y-auto pr-1 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500"
      >
        {hasFilteredLogs ? (
          filteredLogs.map((log) => <LogEntryRow key={log.id} log={log} />)
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
