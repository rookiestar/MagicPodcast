"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import type {
  ImportNewPodcast,
} from "@/lib/api/importTasks";
import {
  workflowApi,
  type WorkflowAppendResponse,
} from "@/lib/api/workflow";
import type { Workflow } from "@/types";

interface AppendToWorkflowDialogProps {
  podcasts: ImportNewPodcast[];
  selectedIds: number[];
  onCancel: () => void;
  onAppended: (result: WorkflowAppendResponse) => void;
}

// AppendToWorkflowDialog 是补入已有工作流的目标选择器：沿用项目弹层样式，
// 展示每个指定节目工作流的新增/已包含数量，待同步节目默认不纳入。
export default function AppendToWorkflowDialog({
  podcasts,
  selectedIds,
  onCancel,
  onAppended,
}: AppendToWorkflowDialogProps) {
  const [workflows, setWorkflows] = useState<Workflow[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [targetId, setTargetId] = useState<number | null>(null);
  const [includePending, setIncludePending] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const dialogRef = useRef<HTMLDivElement>(null);

  const loadWorkflows = () => {
    setLoadError(null);
    workflowApi
      .list({ page: 1, page_size: 100 })
      .then((response) => {
        const targets = (response.workflows ?? []).filter(
          (workflow) => workflow.scope_type === "specific_podcasts",
        );
        setWorkflows(targets);
        setTargetId((current) => current ?? targets[0]?.id ?? null);
      })
      .catch(() => {
        setLoadError("目标工作流读取失败，请重试");
      });
  };

  useEffect(() => {
    loadWorkflows();
  }, []);

  useEffect(() => {
    dialogRef.current?.focus();
  }, []);

  const selected = useMemo(
    () => podcasts.filter((podcast) => selectedIds.includes(podcast.id)),
    [podcasts, selectedIds],
  );
  const pendingSelected = selected.filter((podcast) => !podcast.ready);

  const target = workflows?.find((workflow) => workflow.id === targetId) ?? null;
  const targetMembers = target?.scope_config?.podcast_ids ?? [];
  const alreadyIncluded = selected.filter((podcast) =>
    targetMembers.includes(podcast.id),
  ).length;
  const candidates = selected.filter(
    (podcast) =>
      !targetMembers.includes(podcast.id) && (includePending || podcast.ready),
  );

  const handleSave = async () => {
    if (!target || candidates.length === 0 || saving) return;
    setSaving(true);
    setSaveError(null);
    try {
      const result = await workflowApi.appendPodcasts(
        target.id,
        candidates.map((podcast) => podcast.id),
      );
      onAppended(result);
    } catch {
      setSaveError("添加失败，请重试");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-[60] flex items-start justify-center overflow-y-auto bg-black/50 p-0 sm:items-center sm:p-4"
      onMouseDown={(event) => {
        if (event.currentTarget === event.target && !saving) onCancel();
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="append-to-workflow-title"
        tabIndex={-1}
        onKeyDown={(event) => {
          if (event.key === "Escape" && !saving) onCancel();
        }}
        className="w-full max-w-lg overflow-hidden rounded-none bg-white shadow-2xl outline-none sm:rounded-lg dark:bg-slate-800"
      >
        <div className="border-b border-slate-200 p-4 dark:border-slate-700">
          <h2
            id="append-to-workflow-title"
            className="text-base font-medium text-slate-900 dark:text-slate-100"
          >
            添加到工作流
          </h2>
          <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">
            已选 {selected.length} 档
          </p>
        </div>

        <div className="max-h-[60vh] overflow-y-auto p-4">
          {loadError && (
            <div className="text-sm">
              <p className="text-red-600 dark:text-red-400" role="alert">
                {loadError}
              </p>
              <button
                type="button"
                onClick={loadWorkflows}
                className="mt-2 cursor-pointer rounded px-2 py-1 text-xs text-blue-600 hover:text-blue-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 dark:text-blue-400"
              >
                重试
              </button>
            </div>
          )}

          {!loadError && workflows === null && (
            <p className="text-sm text-slate-500 dark:text-slate-400" role="status">
              正在读取工作流...
            </p>
          )}

          {workflows !== null && workflows.length === 0 && (
            <p className="text-sm text-slate-500 dark:text-slate-400">
              没有指定节目范围的工作流
            </p>
          )}

          {workflows !== null && workflows.length > 0 && (
            <div className="space-y-1" role="radiogroup" aria-label="目标工作流">
              {workflows.map((workflow) => {
                const members = workflow.scope_config?.podcast_ids ?? [];
                const addition = selected.filter(
                  (podcast) =>
                    !members.includes(podcast.id) &&
                    (includePending || podcast.ready),
                ).length;
                const included = selected.filter((podcast) =>
                  members.includes(podcast.id),
                ).length;
                return (
                  <label
                    key={workflow.id}
                    className={`flex cursor-pointer items-center justify-between rounded-md border px-3 py-2 text-sm transition-colors focus-within:outline focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-blue-500 ${
                      workflow.id === targetId
                        ? "border-blue-400 bg-blue-50 dark:border-blue-600 dark:bg-blue-900/20"
                        : "border-slate-200 hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-800/60"
                    }`}
                  >
                    <span className="flex items-center gap-2">
                      <input
                        type="radio"
                        name="append-target-workflow"
                        value={workflow.id}
                        checked={workflow.id === targetId}
                        onChange={() => setTargetId(workflow.id)}
                        aria-label={`选择工作流 ${workflow.name}`}
                        className="h-4 w-4 cursor-pointer"
                      />
                      <span className="font-medium text-slate-800 dark:text-slate-100">
                        {workflow.name}
                      </span>
                    </span>
                    <span className="text-xs text-slate-500 dark:text-slate-400">
                      {(members ?? []).length} 档
                      <span className="ml-1 font-medium text-blue-600 dark:text-blue-400">
                        +{addition}
                      </span>
                      {included > 0 && <span className="ml-1">· 已包含 {included} 档</span>}
                    </span>
                  </label>
                );
              })}
            </div>
          )}

          {pendingSelected.length > 0 && (
            <label className="mt-3 flex cursor-pointer items-center gap-2 text-xs text-slate-600 dark:text-slate-300">
              <input
                type="checkbox"
                checked={includePending}
                onChange={(event) => setIncludePending(event.target.checked)}
                className="h-4 w-4 cursor-pointer"
              />
              包含待同步节目（{pendingSelected.length}）
            </label>
          )}
        </div>

        <div className="flex items-center justify-between gap-3 border-t border-slate-200 p-4 dark:border-slate-700">
          <p className="text-xs text-slate-500 dark:text-slate-400" aria-live="polite">
            {target && (
              <>
                新增 <strong className="text-slate-800 dark:text-slate-100">{candidates.length}</strong> 档
                {alreadyIncluded > 0 && ` · 已包含 ${alreadyIncluded} 档`}
              </>
            )}
          </p>
          <div className="flex items-center gap-2">
            {saveError && (
              <span className="text-xs text-red-600 dark:text-red-400" role="alert">
                {saveError}
              </span>
            )}
            <button
              type="button"
              onClick={onCancel}
              disabled={saving}
              className="cursor-pointer rounded-md border border-slate-300 px-3 py-1.5 text-sm text-slate-700 transition-colors hover:bg-slate-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-600 dark:text-slate-200 dark:hover:bg-slate-700"
            >
              取消
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={!target || candidates.length === 0 || saving}
              className="editorial-btn editorial-btn--primary min-h-[40px] cursor-pointer px-4 py-1.5 text-sm font-medium focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {saving ? "添加中..." : `添加 ${candidates.length} 档`}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
