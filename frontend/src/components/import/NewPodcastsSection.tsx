"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import type { ImportNewPodcast } from "@/lib/api/importTasks";
import { importTasksApi } from "@/lib/api/importTasks";
import { toast } from "@/lib/toast";
import AppendToWorkflowDialog from "./AppendToWorkflowDialog";

const PAGE_SIZE = 20;

interface NewPodcastsSectionProps {
  taskId: number;
}

// NewPodcastsSection 列出一次导入任务（含重试链）实际新建的节目，支持
// 搜索、仅看待同步、跨页多选与批量补入已有工作流（#418）。数据为该批
// 次的完整持久化结果，筛选与分页在客户端进行。
export default function NewPodcastsSection({ taskId }: NewPodcastsSectionProps) {
  const [podcasts, setPodcasts] = useState<ImportNewPodcast[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [pendingOnly, setPendingOnly] = useState(false);
  const [page, setPage] = useState(1);
  const [selectedIds, setSelectedIds] = useState<number[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);

  const requestVersion = useRef(Symbol());
  const load = () => {
    const version = Symbol();
    requestVersion.current = version;
    setLoading(true);
    setLoadError(null);
    importTasksApi
      .fetchTaskNewPodcasts(taskId)
      .then((payload) => {
        if (version !== requestVersion.current) return;
        setPodcasts(payload.podcasts ?? []);
      })
      .catch(() => {
        if (version !== requestVersion.current) return;
        setLoadError("本批新增节目读取失败");
      })
      .finally(() => {
        if (version !== requestVersion.current) return;
        setLoading(false);
      });
  };

  useEffect(() => {
    setDialogOpen(false);
    setPodcasts(null);
    setSelectedIds([]);
    setSearch("");
    setPendingOnly(false);
    setPage(1);
    load();
    return () => { requestVersion.current = Symbol(); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId]);

  const filtered = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    return (podcasts ?? []).filter((podcast) => {
      if (pendingOnly && podcast.ready) return false;
      if (
        keyword &&
        !podcast.title.toLowerCase().includes(keyword) &&
        !podcast.feed_url.toLowerCase().includes(keyword)
      ) {
        return false;
      }
      return true;
    });
  }, [podcasts, search, pendingOnly]);

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const currentPage = Math.min(page, totalPages);
  const visible = filtered.slice(
    (currentPage - 1) * PAGE_SIZE,
    currentPage * PAGE_SIZE,
  );

  const selectedSet = useMemo(() => new Set(selectedIds), [selectedIds]);
  const allFilteredSelected =
    filtered.length > 0 && filtered.every((podcast) => selectedSet.has(podcast.id));
  const someFilteredSelected = filtered.some((podcast) => selectedSet.has(podcast.id));

  const toggleOne = (id: number) => {
    setSelectedIds((current) =>
      current.includes(id)
        ? current.filter((value) => value !== id)
        : [...current, id],
    );
  };

  const toggleAllFiltered = () => {
    setSelectedIds((current) => {
      const set = new Set(current);
      if (allFilteredSelected) {
        filtered.forEach((podcast) => set.delete(podcast.id));
      } else {
        filtered.forEach((podcast) => set.add(podcast.id));
      }
      return [...set];
    });
  };

  const handleAppended = (result: { added: number }) => {
    setDialogOpen(false);
    setSelectedIds([]);
    toast.success(`已添加 ${result.added} 档节目到工作流`);
    load();
  };

  if (loading && podcasts === null) {
    return (
      <div className="wf-editorial mt-3 rounded-lg border border-slate-200 p-3 text-sm dark:border-slate-700">
        <p className="text-slate-500 dark:text-slate-400" role="status">
          正在读取本批新增节目...
        </p>
      </div>
    );
  }

  return (
    <div className="wf-editorial mt-3 rounded-lg border border-slate-200 p-3 text-sm dark:border-slate-700">
      {loadError && (
        <div>
          <p className="text-red-600 dark:text-red-400" role="alert">
            {loadError}
          </p>
          <button
            type="button"
            onClick={load}
            className="mt-2 cursor-pointer rounded px-2 py-1 text-xs text-blue-600 hover:text-blue-800 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 dark:text-blue-400"
          >
            重试
          </button>
        </div>
      )}
      {(!loadError || podcasts !== null) && (
        <>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-xs font-medium text-slate-700 dark:text-slate-200">
              本批新增节目（{filtered.length}）
            </p>
            <div className="flex flex-wrap items-center gap-2">
              <label className="flex cursor-pointer items-center gap-1.5 text-xs text-slate-600 dark:text-slate-300">
                <input
                  type="checkbox"
                  checked={pendingOnly}
                  onChange={(event) => {
                    setPendingOnly(event.target.checked);
                    setPage(1);
                  }}
                  className="h-4 w-4 cursor-pointer"
                />
                仅看待同步
              </label>
              <input
                type="search"
                value={search}
                onChange={(event) => {
                  setSearch(event.target.value);
                  setPage(1);
                }}
                placeholder="搜索节目名称或地址"
                aria-label="搜索本批新增节目"
                className="w-48 rounded-md border border-slate-300 px-2 py-1 text-xs focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100"
              />
            </div>
          </div>

          {filtered.length === 0 ? (
            <p className="mt-3 text-xs text-slate-500 dark:text-slate-400">
              {podcasts !== null && podcasts.length === 0
                ? "本批没有新增节目"
                : "没有匹配的节目"}
            </p>
          ) : (
            <>
              <div className="mt-2 grid grid-cols-[2rem_minmax(0,1fr)] items-center gap-2 border-b border-slate-200 pb-1.5 text-xs text-slate-500 sm:grid-cols-[2rem_minmax(0,1fr)_5rem_minmax(0,10rem)] dark:border-slate-700 dark:text-slate-400">
                <input
                  type="checkbox"
                  checked={allFilteredSelected}
                  ref={(node) => {
                    if (node) node.indeterminate = !allFilteredSelected && someFilteredSelected;
                  }}
                  onChange={toggleAllFiltered}
                  aria-label="选择全部匹配节目"
                  className="h-4 w-4 cursor-pointer"
                />
                <span className="whitespace-nowrap">节目</span>
                <span className="hidden whitespace-nowrap sm:block">资料状态</span>
                <span className="hidden whitespace-nowrap sm:block">已加入的工作流</span>
              </div>
              <ul className="mt-1">
                {visible.map((podcast) => (
                  <li
                    key={podcast.id}
                    className="border-b border-slate-100 py-2 text-xs sm:grid sm:grid-cols-[2rem_minmax(0,1fr)_5rem_minmax(0,10rem)] sm:items-center sm:gap-2 dark:border-slate-800"
                  >
                    <input
                      type="checkbox"
                      checked={selectedSet.has(podcast.id)}
                      onChange={() => toggleOne(podcast.id)}
                      aria-label={`选择「${podcast.title}」`}
                      className="h-4 w-4 cursor-pointer"
                    />
                    <span className="mt-1 block min-w-0 sm:mt-0">
                      <span className="block truncate font-medium text-slate-800 dark:text-slate-100">
                        {podcast.title}
                      </span>
                      <span className="block truncate text-slate-400">{podcast.feed_url}</span>
                    </span>
                    <span className="mt-1 block sm:mt-0">
                      {podcast.ready ? (
                        <span className="text-slate-500 dark:text-slate-400">就绪</span>
                      ) : (
                        <span className="rounded-full bg-amber-100 px-2 py-0.5 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
                          待同步
                        </span>
                      )}
                    </span>
                    <span className="mt-1 block min-w-0 truncate text-slate-500 sm:mt-0 dark:text-slate-400">
                      {podcast.workflows.length > 0
                        ? podcast.workflows.map((workflow) => workflow.name).join("、")
                        : "—"}
                    </span>
                  </li>
                ))}
              </ul>

              <div className="mt-2 flex items-center justify-between text-xs text-slate-500 dark:text-slate-400">
                {totalPages > 1 ? (
                  <span className="flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() => setPage(currentPage - 1)}
                      disabled={currentPage <= 1}
                      className="cursor-pointer rounded border border-slate-300 px-2 py-1 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-600"
                    >
                      上一页
                    </button>
                    第 {currentPage} / {totalPages} 页 · 共 {filtered.length} 档
                    <button
                      type="button"
                      onClick={() => setPage(currentPage + 1)}
                      disabled={currentPage >= totalPages}
                      className="cursor-pointer rounded border border-slate-300 px-2 py-1 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-600"
                    >
                      下一页
                    </button>
                  </span>
                ) : (
                  <span>共 {filtered.length} 档</span>
                )}
                <span>
                  已选 <strong className="text-slate-800 dark:text-slate-100">{selectedIds.length}</strong> 档
                </span>
              </div>

              <div className="mt-3 flex justify-end">
                <button
                  type="button"
                  onClick={() => setDialogOpen(true)}
                  disabled={selectedIds.length === 0}
                  className="editorial-btn editorial-btn--primary min-h-[40px] cursor-pointer px-4 py-1.5 text-sm font-medium focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
                >
                  添加到工作流
                </button>
              </div>
            </>
          )}
        </>
      )}

      {dialogOpen && podcasts !== null && (
        <AppendToWorkflowDialog
          podcasts={podcasts}
          selectedIds={selectedIds}
          onCancel={() => setDialogOpen(false)}
          onAppended={handleAppended}
        />
      )}
    </div>
  );
}
