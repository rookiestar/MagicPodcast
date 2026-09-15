"use client";

import type { ChangeEventHandler } from "react";
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
  confirmedUrls: Record<string, boolean>;
  lastTask: ImportTask | null;
  taskEntries?: ImportEntryResult[];
  onFileChange: ChangeEventHandler<HTMLInputElement>;
  onImport: () => void;
  onToggleConfirmed: (entry: ImportPreviewEntry) => void;
  onConfirmAllPending: () => void;
  onRetry: (entry?: ImportEntryResult) => void;
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

function TaskBanner({
  task,
  onRetry,
  disabled,
}: {
  task: ImportTask;
  onRetry: () => void;
  disabled: boolean;
}) {
  const statusLabel =
    task.status === "running"
      ? "正在后台执行"
      : task.status === "completed"
        ? "已完成"
        : task.status === "interrupted"
          ? "因服务重启中断（待继续）"
          : "失败";
  const retryable = task.failed_count + task.pending_count;
  return (
    <div
      role="status"
      className="import-task-banner rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm dark:border-blue-800 dark:bg-blue-900/20"
    >
      <p className="font-medium text-blue-800 dark:text-blue-200">
        上次导入任务 #{task.id} · {statusLabel}
        {task.status === "running" && `（${task.processed}/${task.total}）`}
      </p>
      <p className="mt-1 text-xs text-blue-700 dark:text-blue-300">
        新增/更新 {task.success_count} · 未变化 {task.unchanged_count} · 待同步{" "}
        {task.pending_count} · 冲突 {task.conflict_count} · 失败{" "}
        {task.failed_count}
      </p>
      {(task.status === "completed" || task.status === "interrupted") && retryable > 0 && (
        <button
          type="button"
          onClick={onRetry}
          disabled={disabled}
          className="mt-2 cursor-pointer rounded-md border border-blue-300 px-3 py-1.5 text-xs font-medium text-blue-700 transition-colors hover:bg-blue-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 dark:border-blue-700 dark:text-blue-200 dark:hover:bg-blue-900/40"
        >
          仅重试失败/待同步条目（{retryable} 条）
        </button>
      )}
    </div>
  );
}

export default function ImportOpmlPanel({
  file,
  disabled,
  importing,
  preview,
  previewLoading,
  previewError,
  confirmedUrls,
  lastTask,
  taskEntries = [],
  onFileChange,
  onImport,
  onToggleConfirmed,
  onConfirmAllPending,
  onRetry,
}: ImportOpmlPanelProps) {
  const confirmableEntries =
    preview?.entries.filter((entry) => CONFIRMABLE_KINDS.has(entry.kind)) ?? [];
  const pendingConfirmCount = confirmableEntries.length;

  return (
    <>
      {lastTask && (
        <div className="mb-4">
          <TaskBanner task={lastTask} onRetry={() => onRetry()} disabled={disabled} />
          {lastTask.status === "interrupted" && taskEntries.some(entry => entry.outcome === "unprocessed") && (
            <button type="button" disabled={disabled} onClick={() => onRetry()}>继续未完成条目</button>
          )}
          {taskEntries.length > 0 && (
            <details className="mt-3">
              <summary>逐项结果（{taskEntries.length} 条）</summary>
              <ul className="max-h-80 overflow-y-auto text-sm">
                {taskEntries.map(entry => (
                  <li key={entry.feed_url} className="my-2 break-words">
                    <p>{entry.title} · {({new: "新增", updated: "更新", unchanged: "未变化", pending: "待同步", conflict: "待核对", merged: "已关联", failed: "失败", deleted: "已删除，未恢复", unprocessed: "尚无完成记录", skipped: "跳过"} as Record<string, string>)[entry.outcome] || entry.outcome}</p>
                    <p>{entry.feed_url}</p><p>{entry.detail}</p>
                    {entry.outcome === "conflict" && !!entry.podcast_id && lastTask.status !== "running" && (
                      <button type="button" disabled={disabled} onClick={() => onRetry(entry)}>核对并确认关联</button>
                    )}
                  </li>
                ))}
              </ul>
            </details>
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
          读取小宇宙、Apple Podcasts 等应用导出的订阅列表。先匹配本地索引，未命中或需要刷新时会在线抓取
          RSS。只补充节目，保留已有关注和标签；文件夹分类不会转成标签。
          RSS 暂不可用的有效链接会保留订阅并标记待同步。选择文件后先预览差异，再开始导入。
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
        <p className="import-file-name">
          {file
            ? `${file.name} · ${(file.size / 1024).toFixed(2)} KB`
            : "支持 .opml 与 .xml 文件"}
        </p>
      </div>

      {previewLoading && (
        <p className="text-sm text-slate-500 dark:text-slate-400" role="status">
          正在核对本地库差异...
        </p>
      )}
      {previewError && (
        <p className="text-sm text-red-600 dark:text-red-400" role="alert">
          {previewError}
        </p>
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

          {preview.total === 0 && (
            <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">
              文件中没有订阅条目（0 条），导入不会新增节目。
            </p>
          )}

          {pendingConfirmCount > 0 && (
            <div className="mt-3">
              <div className="flex items-center justify-between">
                <p className="text-xs font-medium text-slate-700 dark:text-slate-200">
                  以下 {pendingConfirmCount} 条需要确认后才会写入（默认跳过）
                </p>
                <button
                  type="button"
                  onClick={onConfirmAllPending}
                  className="cursor-pointer rounded px-2 py-1 text-xs text-blue-600 hover:text-blue-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 dark:text-blue-400 dark:hover:text-blue-300"
                >
                  全部确认
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
        </div>
      )}

      <div className="import-primary-action">
        <button
          type="button"
          onClick={onImport}
          disabled={!file || disabled}
          className={`editorial-btn editorial-btn--primary min-h-[44px] px-6 py-2.5 text-sm font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
            !file || disabled
              ? "cursor-not-allowed is-disabled"
              : "cursor-pointer"
          }`}
        >
          {importing ? "导入中..." : "开始导入"}
        </button>
      </div>
    </>
  );
}
