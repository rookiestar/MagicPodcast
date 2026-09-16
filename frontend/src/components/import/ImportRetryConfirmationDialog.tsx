"use client";

import { useEffect, useRef, useState } from "react";
import type { ImportEntryResult, ImportTask } from "@/lib/api/importTasks";

interface ImportRetryConfirmationDialogProps {
  task: ImportTask;
  retryableCount: number;
  conflictEntry?: ImportEntryResult;
  disabled: boolean;
  confirmationText: string;
  onCancel: () => void;
  onConfirm: (confirmationText: string) => void | Promise<void>;
}

export default function ImportRetryConfirmationDialog({
  task,
  retryableCount,
  conflictEntry,
  disabled,
  confirmationText,
  onCancel,
  onConfirm,
}: ImportRetryConfirmationDialogProps) {
  const [value, setValue] = useState("");
  const dialogRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    dialogRef.current?.querySelector<HTMLInputElement>("input")?.focus();
    return () => previous?.focus();
  }, []);

  const submit = () => {
    if (disabled || value.trim() !== confirmationText) return;
    void onConfirm(confirmationText);
  };

  return (
    <div
      className="workflow-form-modal wf-editorial fixed inset-0 z-[60] flex items-start justify-center overflow-y-auto bg-black/50 p-0 sm:items-center sm:p-4"
      onMouseDown={(event) => {
        if (event.currentTarget === event.target && !disabled) onCancel();
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="import-retry-confirmation-title"
        tabIndex={-1}
        onKeyDown={(event) => {
          if (event.key === "Escape" && !disabled) {
            event.stopPropagation();
            onCancel();
          }
          if (event.key === "Enter" && event.target instanceof HTMLInputElement) submit();
          if (event.key === "Tab") {
            const controls = Array.from(
              event.currentTarget.querySelectorAll<HTMLElement>(
                "input:not(:disabled), button:not(:disabled)",
              ),
            );
            const first = controls[0];
            const last = controls.at(-1);
            if (event.shiftKey && document.activeElement === first) {
              event.preventDefault();
              last?.focus();
            } else if (!event.shiftKey && document.activeElement === last) {
              event.preventDefault();
              first?.focus();
            }
          }
        }}
        className="w-full max-w-lg overflow-hidden rounded-none bg-white shadow-2xl outline-none sm:rounded-lg dark:bg-slate-800"
      >
        <div className="workflow-modal-header border-b border-slate-200 p-4 dark:border-slate-700">
          <h2 id="import-retry-confirmation-title" className="type-section-title text-slate-900 dark:text-slate-100">
            继续导入任务 #{task.id}
          </h2>
        </div>
        <div className="space-y-4 p-4">
          <p className="text-sm text-slate-700 dark:text-slate-200">
            {conflictEntry
              ? `确认关联「${conflictEntry.title}」，并重试其余可处理条目。`
              : `将处理 ${retryableCount} 条未完成条目。`}
          </p>
          <div>
            <label htmlFor="import-retry-confirmation-input" className="block text-sm text-slate-700 dark:text-slate-200">
              输入确认文字 <code className="rounded bg-slate-100 px-1 py-0.5 text-xs dark:bg-slate-700">{confirmationText}</code>
            </label>
            <input
              id="import-retry-confirmation-input"
              value={value}
              onChange={(event) => setValue(event.target.value)}
              autoComplete="off"
              spellCheck={false}
              disabled={disabled}
              className="mt-2 min-h-[44px] w-full rounded-md border border-slate-300 px-3 text-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-100"
            />
          </div>
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={onCancel}
              disabled={disabled}
              className="min-h-[44px] cursor-pointer rounded-md border border-slate-300 px-4 text-sm text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-600 dark:text-slate-200 dark:hover:bg-slate-700"
            >
              取消
            </button>
            <button
              type="button"
              onClick={submit}
              disabled={disabled || value.trim() !== confirmationText}
              className="editorial-btn editorial-btn--primary min-h-[44px] cursor-pointer px-4 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-50"
            >
              确认重试
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
