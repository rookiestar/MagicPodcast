import type { LogEntry } from "@/lib/syncLogState";
import {
  getLogBadgeClass,
  getLogLabel,
  getLogRowClass,
  getSkipLogTypeLabel,
} from "./syncLogDisplay";

export default function SyncLogEntryRow({ log }: { log: LogEntry }) {
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
                [{getSkipLogTypeLabel(log.type)}]{" "}
              </span>
            )}
            {log.message}
          </p>
        )}
      </div>
    </div>
  );
}
