"use client";

import { useMemo, useState, type ChangeEventHandler } from "react";
import { IconFileUpload } from "@tabler/icons-react";
import {
  CONFIRMABLE_KINDS,
  type ImportPreview,
  type ImportPreviewEntry,
  type ImportTask,
  type ImportEntryResult,
} from "@/lib/api/importTasks";
import NewPodcastsSection from "./NewPodcastsSection";

interface ImportOpmlPanelProps {
  file: File | null;
  disabled: boolean;
  importing: boolean;
  preview: ImportPreview | null;
  previewLoading: boolean;
  previewError: string | null;
  canSubmitImport: boolean;
  confirmedUrls: Record<string, boolean>;
  lastTask: ImportTask | null;
  taskEntries?: ImportEntryResult[];
  latestTaskError: boolean;
  onFileChange: ChangeEventHandler<HTMLInputElement>;
  onImport: () => void;
  onToggleConfirmed: (entry: ImportPreviewEntry) => void;
  onConfirmAllPending: () => void;
  onRetry: (entry?: ImportEntryResult) => void;
  onRetryPreview: () => void;
  onRetryLatestTask: () => void;
  countRetryableEntries: (task: ImportTask, entries: ImportEntryResult[]) => number;
}

const KIND_LABELS: Record<ImportPreviewEntry["kind"], string> = {
  new: "新增",
  existing: "已在库中",
  collection: "清单已收录",
  deleted: "已删除记录",
  invalid: "无效地址",
  duplicate: "文件内重复",
};

const KIND_BADGES: Record<ImportPreviewEntry["kind"], string> = {
  new: "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300",
  existing:
    "bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300",
  collection:
    "bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300",
  deleted:
    "bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300",
  invalid: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300",
  duplicate:
    "bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400",
};

// 预览明细的固定展示顺序；新增默认展开，其余类别按需查看。
const DETAIL_KIND_ORDER: ReadonlyArray<ImportPreviewEntry["kind"]> = [
  "new",
  "existing",
  "collection",
  "deleted",
  "invalid",
  "duplicate",
];

const OUTCOME_LABELS: Record<string, string> = {
  new: "新增",
  updated: "更新",
  unchanged: "未变化",
  pending: "待同步",
  conflict: "待核对",
  merged: "已关联",
  failed: "失败",
  deleted: "已删除，未恢复",
  unprocessed: "尚无完成记录",
  skipped: "跳过",
};

// 结果定位的固定展示顺序：待处理类在前，已完成类在后。
const OUTCOME_ORDER = [
  "unprocessed",
  "failed",
  "pending",
  "conflict",
  "merged",
  "new",
  "updated",
  "unchanged",
  "deleted",
  "skipped",
] as const;

const ACTION_BUTTON_CLASS =
  "min-h-[44px] cursor-pointer rounded-md border px-3 text-sm font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50";

function formatTaskTime(value: string | null | undefined) {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "";
  return parsed.toLocaleString();
}

function TaskBanner({
  task,
  taskEntries,
  onRetry,
  disabled,
  countRetryableEntries,
}: {
  task: ImportTask;
  taskEntries: ImportEntryResult[];
  onRetry: () => void;
  disabled: boolean;
  countRetryableEntries: (task: ImportTask, entries: ImportEntryResult[]) => number;
}) {
  const statusLabel =
    task.status === "running"
      ? "正在后台执行"
      : task.status === "completed"
        ? "已完成"
        : task.status === "interrupted"
          ? "因服务重启中断（待继续）"
          : "失败";
  // 与 handleRetry/后端能力同口径的可重试集合；running 不提供重试入口。
  const retryable =
    task.status === "running" ? 0 : countRetryableEntries(task, taskEntries);
  const isInterrupted = task.status === "interrupted";
  const startedAt = formatTaskTime(task.started_at);

  return (
    <div
      role="status"
      className="import-task-banner rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm dark:border-blue-800 dark:bg-blue-900/20"
    >
      <p className="font-medium text-blue-800 dark:text-blue-200">
        上次导入任务 #{task.id} · {statusLabel}
        {task.status === "running" && `（${task.processed}/${task.total}）`}
      </p>
      <p className="mt-0.5 text-xs text-blue-700 dark:text-blue-300">
        来源文件「{task.file_name}」
        {startedAt && ` · 开始于 ${startedAt}`}
      </p>
      <p className="mt-1 text-xs text-blue-700 dark:text-blue-300">
        新增/更新 {task.success_count} · 未变化 {task.unchanged_count} · 待同步{" "}
        {task.pending_count} · 冲突 {task.conflict_count} · 失败{" "}
        {task.failed_count}
      </p>
      {task.status === "failed" && task.error_message && (
        <p className="mt-1 break-words text-xs text-red-700 dark:text-red-300">
          失败原因：{task.error_message}
        </p>
      )}
      {task.status !== "running" && retryable > 0 && (
        <button
          type="button"
          onClick={onRetry}
          disabled={disabled}
          className={`mt-2 border-blue-300 bg-transparent text-blue-700 hover:bg-blue-100 focus-visible:outline-blue-500 dark:border-blue-700 dark:text-blue-200 dark:hover:bg-blue-900/40 ${ACTION_BUTTON_CLASS}`}
        >
          {isInterrupted
            ? `继续未完成条目（${retryable} 条）`
            : `仅重试失败/待同步条目（${retryable} 条）`}
        </button>
      )}
    </div>
  );
}

// TaskResults 按 outcome 提供结果定位与筛选；key=任务ID 保证切换任务时
// 筛选复位，不会把上一次任务的筛选套到新任务上。
function TaskResults({
  task,
  taskEntries,
  disabled,
  onRetry,
}: {
  task: ImportTask;
  taskEntries: ImportEntryResult[];
  disabled: boolean;
  onRetry: (entry?: ImportEntryResult) => void;
}) {
  const [outcomeFilter, setOutcomeFilter] = useState<string>("all");

  const outcomeChips = useMemo(() => {
    const counts = new Map<string, number>();
    for (const entry of taskEntries) {
      counts.set(entry.outcome, (counts.get(entry.outcome) ?? 0) + 1);
    }
    return OUTCOME_ORDER.filter((outcome) => counts.has(outcome)).map(
      (outcome) => ({ outcome, count: counts.get(outcome) ?? 0 }),
    );
  }, [taskEntries]);

  const visibleEntries = useMemo(() => {
    if (outcomeFilter === "all") return taskEntries;
    return taskEntries.filter((entry) => entry.outcome === outcomeFilter);
  }, [taskEntries, outcomeFilter]);

  return (
    <details className="import-task-results mt-3" data-testid="task-results">
      <summary className="cursor-pointer text-sm font-medium">
        逐项结果（{taskEntries.length} 条）
      </summary>
      {outcomeChips.length > 1 && (
        <div
          role="group"
          aria-label="按结果状态筛选"
          className="mt-2 flex flex-wrap gap-2"
        >
          <button
            type="button"
            aria-pressed={outcomeFilter === "all"}
            onClick={() => setOutcomeFilter("all")}
            className={`cursor-pointer rounded-full border px-3 py-1.5 text-xs font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
              outcomeFilter === "all"
                ? "border-blue-500 bg-blue-50 text-blue-700 dark:border-blue-600 dark:bg-blue-900/30 dark:text-blue-300"
                : "border-slate-300 bg-white text-slate-600 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-300"
            }`}
          >
            全部（{taskEntries.length}）
          </button>
          {outcomeChips.map(({ outcome, count }) => (
            <button
              key={outcome}
              type="button"
              aria-pressed={outcomeFilter === outcome}
              onClick={() => setOutcomeFilter(outcome)}
              className={`cursor-pointer rounded-full border px-3 py-1.5 text-xs font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
                outcomeFilter === outcome
                  ? "border-blue-500 bg-blue-50 text-blue-700 dark:border-blue-600 dark:bg-blue-900/30 dark:text-blue-300"
                  : "border-slate-300 bg-white text-slate-600 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-300"
              }`}
            >
              {OUTCOME_LABELS[outcome] ?? outcome}（{count}）
            </button>
          ))}
        </div>
      )}
      <ul
        className="max-h-80 space-y-2 overflow-y-auto pr-1 text-sm"
        aria-live="off"
      >
        {visibleEntries.map((entry) => (
          <li key={entry.feed_url} className="my-2 break-words">
            <p>
              {entry.title} ·{" "}
              {OUTCOME_LABELS[entry.outcome] ?? entry.outcome}
            </p>
            <p className="break-all text-xs text-slate-500 dark:text-slate-400">
              {entry.feed_url}
            </p>
            {entry.detail && (
              <p className="text-xs text-slate-500 dark:text-slate-400">
                {entry.detail}
              </p>
            )}
            {entry.outcome === "conflict" &&
              !!entry.podcast_id &&
              task.status !== "running" && (
                <button
                  type="button"
                  disabled={disabled}
                  onClick={() => onRetry(entry)}
                  className={`mt-1 border-blue-300 bg-transparent text-blue-700 hover:bg-blue-100 focus-visible:outline-blue-500 dark:border-blue-700 dark:text-blue-200 dark:hover:bg-blue-900/40 ${ACTION_BUTTON_CLASS}`}
                >
                  核对并确认关联
                </button>
              )}
          </li>
        ))}
        {visibleEntries.length === 0 && (
          <li className="rounded-md border border-dashed border-slate-200 px-3 py-3 text-center text-xs text-slate-500 dark:border-slate-700 dark:text-slate-400">
            当前筛选下没有条目
          </li>
        )}
      </ul>
    </details>
  );
}

export default function ImportOpmlPanel({
  file,
  disabled,
  importing,
  preview,
  previewLoading,
  previewError,
  canSubmitImport,
  confirmedUrls,
  lastTask,
  taskEntries = [],
  latestTaskError,
  onFileChange,
  onImport,
  onToggleConfirmed,
  onConfirmAllPending,
  onRetry,
  onRetryPreview,
  onRetryLatestTask,
  countRetryableEntries,
}: ImportOpmlPanelProps) {
  const confirmableEntries =
    preview?.entries.filter((entry) => CONFIRMABLE_KINDS.has(entry.kind)) ?? [];
  const pendingConfirmCount = confirmableEntries.length;

  const detailKinds = useMemo(() => {
    if (!preview) return [];
    return DETAIL_KIND_ORDER.map((kind) => ({
      kind,
      entries: preview.entries.filter((entry) => entry.kind === kind),
    })).filter(({ entries }) => entries.length > 0);
  }, [preview]);

  const submitHint = !file || canSubmitImport || importing || disabled
    ? null
    : previewLoading
      ? "正在核对当前文件差异，预览完成后可开始导入"
      : previewError
        ? "预览失败，重试成功后才能开始导入"
        : "等待当前文件的预览完成后可开始导入";

  return (
    <>
      {latestTaskError && (
        <div
          role="alert"
          className="mb-4 rounded-lg border border-amber-300 bg-amber-50 p-3 text-sm dark:border-amber-700 dark:bg-amber-900/20"
        >
          <p className="font-medium text-amber-800 dark:text-amber-200">
            最近导入任务{lastTask ? "状态" : "记录"}读取失败
          </p>
          <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">
            {lastTask
              ? "以下显示的是最后一次成功读取的任务结果，不代表最新状态。"
              : "无法确认是否存在历史导入任务，请重试读取。"}
          </p>
          <button
            type="button"
            onClick={onRetryLatestTask}
            className={`mt-2 border-amber-400 bg-transparent text-amber-800 hover:bg-amber-100 focus-visible:outline-amber-500 dark:border-amber-600 dark:text-amber-200 dark:hover:bg-amber-900/40 ${ACTION_BUTTON_CLASS}`}
          >
            重试读取
          </button>
        </div>
      )}

      {lastTask && (
        <div className="mb-4">
          <TaskBanner
            task={lastTask}
            taskEntries={taskEntries}
            onRetry={() => onRetry()}
            disabled={disabled}
            countRetryableEntries={countRetryableEntries}
          />
          {taskEntries.length > 0 && (
            <TaskResults
              key={lastTask.id}
              task={lastTask}
              taskEntries={taskEntries}
              disabled={disabled}
              onRetry={onRetry}
            />
          )}
          {lastTask.status !== "running" && <NewPodcastsSection taskId={lastTask.id} />}
        </div>
      )}

      <div className="import-guidance">
        <p className="import-eyebrow">从其他应用迁移</p>
        <h3 className="text-base font-medium text-slate-900 dark:text-slate-100">
          导入 OPML
        </h3>
        <p className="import-guidance-copy">
          选择订阅文件，预览差异后导入。保留已有关注和标签，文件夹不转为标签；
          RSS 暂不可用的有效链接仍会收录并标记待同步。
        </p>
      </div>

      <div className="import-file-field">
        <label
          htmlFor="opml-file-input"
          aria-disabled={disabled}
          className={`import-file-picker${disabled ? " is-disabled" : ""}`}
        >
          <IconFileUpload aria-hidden="true" />
          <span>{file ? "更换 OPML 文件" : "选择 OPML 文件"}</span>
          <input
            id="opml-file-input"
            type="file"
            accept=".opml,.xml"
            onChange={onFileChange}
            disabled={disabled}
            className="sr-only"
          />
        </label>
        <p className="import-file-name break-all">
          {file
            ? `${file.name} · ${(file.size / 1024).toFixed(2)} KB`
            : "支持 .opml 与 .xml 文件，最大 8MB"}
        </p>
      </div>

      {previewLoading && (
        <p className="text-sm text-slate-500 dark:text-slate-400" role="status">
          正在核对本地库差异...
        </p>
      )}
      {previewError && (
        <div
          role="alert"
          className="mt-3 rounded-lg border border-red-200 bg-red-50 p-3 text-sm dark:border-red-800 dark:bg-red-900/20"
        >
          <p className="text-red-700 dark:text-red-300">{previewError}</p>
          {file && (
            <button
              type="button"
              onClick={onRetryPreview}
              className={`mt-2 border-red-300 bg-transparent text-red-700 hover:bg-red-100 focus-visible:outline-red-500 dark:border-red-700 dark:text-red-300 dark:hover:bg-red-900/40 ${ACTION_BUTTON_CLASS}`}
            >
              重试预览「{file.name}」
            </button>
          )}
        </div>
      )}

      {preview && !previewLoading && (
        <div
          className="import-preview rounded-lg border border-slate-200 p-3 text-sm dark:border-slate-700"
          aria-label="导入预览"
        >
          <div className="flex flex-wrap items-center gap-2 text-xs text-slate-600 dark:text-slate-300">
            <span className="font-medium">
              共 {preview.total} 条
              {preview.duplicate_merged_count > 0 &&
                `（文件内重复已归并 ${preview.duplicate_merged_count} 条）`}
            </span>
            <span className="rounded-full bg-green-100 px-2 py-0.5 text-green-700 dark:bg-green-900/30 dark:text-green-300">
              新增 {preview.new_count}
            </span>
            <span className="rounded-full bg-slate-100 px-2 py-0.5 text-slate-600 dark:bg-slate-800 dark:text-slate-300">
              已在库中 {preview.existing_count}
            </span>
            {preview.collection_count > 0 && (
              <span className="rounded-full bg-blue-100 px-2 py-0.5 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
                清单已收录 {preview.collection_count}
              </span>
            )}
            {preview.deleted_count > 0 && (
              <span className="rounded-full bg-amber-100 px-2 py-0.5 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
                已删除记录 {preview.deleted_count}
              </span>
            )}
            {preview.invalid_count > 0 && (
              <span className="rounded-full bg-red-100 px-2 py-0.5 text-red-700 dark:bg-red-900/30 dark:text-red-300">
                无效 {preview.invalid_count}
              </span>
            )}
          </div>
          {file && (
            <p className="mt-1 text-xs text-slate-400 dark:text-slate-500">
              以上为「{file.name}」的预览；开始导入提交的是同一文件。
            </p>
          )}

          {preview.total === 0 && (
            <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">
              文件中没有订阅条目（0 条），导入不会新增节目。
            </p>
          )}

          {pendingConfirmCount > 0 && (
            <div className="mt-3">
              <div className="flex items-center justify-between gap-2">
                <p className="text-xs font-medium text-slate-700 dark:text-slate-200">
                  以下 {pendingConfirmCount} 条需要确认后才会写入（默认跳过）
                </p>
                <button
                  type="button"
                  onClick={onConfirmAllPending}
                  className="min-h-[44px] cursor-pointer rounded px-3 text-sm font-medium text-blue-600 hover:text-blue-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 dark:text-blue-400 dark:hover:text-blue-300"
                >
                  全部确认（{pendingConfirmCount} 条）
                </button>
              </div>
              <ul className="mt-2 space-y-2">
                {confirmableEntries.map((entry) => (
                  <li
                    key={entry.xml_url}
                    className="flex items-start gap-2 rounded-md bg-slate-50 p-2 dark:bg-slate-800/60"
                  >
                    <input
                      id={`confirm-${entry.xml_url}`}
                      type="checkbox"
                      checked={confirmedUrls[entry.xml_url] === true}
                      onChange={() => onToggleConfirmed(entry)}
                      className="mt-0.5 h-4 w-4 cursor-pointer"
                    />
                    <label
                      htmlFor={`confirm-${entry.xml_url}`}
                      className="cursor-pointer text-xs text-slate-600 dark:text-slate-300"
                    >
                      <span
                        className={`mr-1 rounded-full px-1.5 py-0.5 ${KIND_BADGES[entry.kind]}`}
                      >
                        {KIND_LABELS[entry.kind]}
                      </span>
                      <span className="font-medium">{entry.title}</span>
                      {entry.podcast_title && (
                        <span> → 关联本地「{entry.podcast_title}」</span>
                      )}
                      <span className="block break-all text-slate-400">
                        {entry.xml_url}
                      </span>
                      {entry.reason && <span>{entry.reason}</span>}
                    </label>
                  </li>
                ))}
              </ul>
            </div>
          )}

          {detailKinds.length > 0 && (
            <div
              className="mt-3 border-t border-slate-100 pt-2 dark:border-slate-800"
              data-testid="preview-details"
            >
              {detailKinds.map(({ kind, entries }) => (
                <details
                  key={kind}
                  className="mt-1"
                  open={kind === "new"}
                >
                  <summary className="cursor-pointer text-xs font-medium text-slate-600 dark:text-slate-300">
                    <span
                      className={`mr-1 inline-block rounded-full px-1.5 py-0.5 ${KIND_BADGES[kind]}`}
                    >
                      {KIND_LABELS[kind]}
                    </span>
                    明细（{entries.length} 条）
                  </summary>
                  <ul className="max-h-60 space-y-2 overflow-y-auto p-2">
                    {entries.map((entry) => (
                      <li key={`${kind}-${entry.xml_url}`} className="break-words text-xs">
                        <p className="font-medium text-slate-700 dark:text-slate-200">
                          {entry.title}
                          {entry.podcast_title && (
                            <span className="font-normal">
                              {" "}
                              → 关联本地「{entry.podcast_title}」
                            </span>
                          )}
                        </p>
                        <p className="break-all text-slate-400 dark:text-slate-500">
                          {entry.xml_url}
                        </p>
                        {(entry.reason || entry.evidence) && (
                          <p className="text-slate-500 dark:text-slate-400">
                            {entry.reason || entry.evidence}
                          </p>
                        )}
                        {entry.duplicate_in_file &&
                          entry.duplicate_in_file > 1 && (
                            <p className="text-slate-400 dark:text-slate-500">
                              文件内出现 {entry.duplicate_in_file} 次，已归并处理
                            </p>
                          )}
                      </li>
                    ))}
                  </ul>
                </details>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="import-primary-action">
        <button
          type="button"
          onClick={onImport}
          disabled={!canSubmitImport || disabled}
          aria-describedby={submitHint ? "import-submit-hint" : undefined}
          className={`editorial-btn editorial-btn--primary min-h-[44px] px-6 py-2.5 text-sm font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
            !canSubmitImport || disabled
              ? "cursor-not-allowed is-disabled"
              : "cursor-pointer"
          }`}
        >
          {importing ? "导入中..." : "开始导入"}
        </button>
        {submitHint && (
          <p
            id="import-submit-hint"
            className="mt-2 text-xs text-slate-500 dark:text-slate-400"
          >
            {submitHint}
          </p>
        )}
      </div>
    </>
  );
}
