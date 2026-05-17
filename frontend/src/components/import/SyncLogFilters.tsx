import type { ReactNode } from "react";
import {
  filterSyncLogs,
  type LogEntry,
  type SyncLogFilter,
} from "@/lib/syncLogState";

interface SyncLogFiltersProps {
  logs: LogEntry[];
  filter: SyncLogFilter;
  onFilterChange: (filter: SyncLogFilter) => void;
}

function FilterButton({
  active,
  children,
  tone,
  onClick,
}: {
  active: boolean;
  children: ReactNode;
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

export default function SyncLogFilters({
  logs,
  filter,
  onFilterChange,
}: SyncLogFiltersProps) {
  const filterCounts = {
    all: logs.length,
    success: filterSyncLogs(logs, "success").length,
    errors: filterSyncLogs(logs, "errors").length,
    skips: filterSyncLogs(logs, "skips").length,
    noUpdate: filterSyncLogs(logs, "no_update").length,
  };

  return (
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
  );
}
