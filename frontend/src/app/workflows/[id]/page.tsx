"use client";

import { Suspense, useCallback, useEffect, useMemo, useState, useRef } from "react";
import { useParams, useSearchParams } from "next/navigation";
import Link from "next/link";
import dynamic from "next/dynamic";
import { podcastApi } from "@/lib/api";
import { schedulerApi } from "@/lib/api/scheduler";
import { useWorkflow, useWorkflowJobs } from "@/hooks/useWorkflowSWR";
import { useWorkflowActions } from "@/hooks/useWorkflowActions";
import { useJobExpansion } from "@/hooks/useJobExpansion";
import { getEffectiveCoverUrl } from "@/lib/imageProxy";
import { prefetchWorkflowJobsSummary } from "@/lib/prefetch";
import { formatDateTime } from "@/lib/timeUtils";
import type { Podcast } from "@/types";
import PageLayout from "@/components/layout/PageLayout";
import LoadingLayout from "@/components/layout/LoadingLayout";
import PodcastCover from "@/components/podcasts/PodcastCover";
import { WorkflowDetailSkeleton } from "@/components/ui/Skeleton";
import { WorkflowStatusBadge, JobStatusBadge } from "@/components/ui/StatusBadge";

const TAB_VALUES = ["overview", "jobs", "config"] as const;
type TabType = (typeof TAB_VALUES)[number];

function parseTab(value: string | null | undefined): TabType {
  if (value === "jobs" || value === "config" || value === "overview") {
    return value;
  }
  return "overview";
}

/** Sync tab into the shareable URL without App Router / RSC navigation. */
function replaceTabInUrl(tab: TabType) {
  if (typeof window === "undefined") return;
  const url = new URL(window.location.href);
  url.searchParams.set("tab", tab);
  const next = `${url.pathname}?${url.searchParams.toString()}`;
  const current = `${window.location.pathname}${window.location.search}`;
  if (next !== current) {
    window.history.replaceState(window.history.state, "", next);
  }
}

// 动态导入大型模态框组件，减少首屏 bundle 大小
const WorkflowFormModal = dynamic(
  () => import("@/components/workflows/WorkflowFormModal"),
  { ssr: false }
);

const ReportModal = dynamic(
  () => import("@/components/workflows/ReportModal"),
  { ssr: false }
);

const OVERVIEW_SCOPE_PODCAST_LIMIT = 12;

// 内部组件：使用 useSearchParams
function WorkflowDetailContent() {
  const params = useParams();
  const searchParams = useSearchParams();
  const id = parseInt(params.id as string);

  // 使用 SWR 获取工作流数据
  const { workflow, isLoading: workflowLoading, isError: workflowError, mutate: mutateWorkflow } = useWorkflow(id);

  // 从URL读取tab状态，如果没有则默认为overview
  const tabFromUrl = parseTab(searchParams.get("tab"));
  const [activeTab, setActiveTabState] = useState<TabType>(tabFromUrl);

  const setActiveTab = useCallback((tab: TabType) => {
    setActiveTabState(tab);
    replaceTabInUrl(tab);
  }, []);

  // 意图预取：悬停/聚焦/触摸时只拉第一页摘要
  const intentPrefetchJobs = useCallback(() => {
    if (!Number.isFinite(id) || id <= 0) return;
    void prefetchWorkflowJobsSummary(id);
  }, [id]);

  // Job分页状态
  const [jobsPage, setJobsPage] = useState(1);

  // 执行历史只在进入对应标签时加载，避免详情首屏被历史数据拖慢
  const {
    jobs,
    pagination,
    isLoading: jobsLoading,
    isError: jobsError,
    mutate: mutateJobs,
  } = useWorkflowJobs(id, jobsPage, 10, activeTab === "jobs");

  const [overviewPodcasts, setOverviewPodcasts] = useState<Podcast[]>([]);
  const [configPodcasts, setConfigPodcasts] = useState<Podcast[]>([]);
  const [isLoadingOverviewPodcasts, setIsLoadingOverviewPodcasts] =
    useState(false);
  const [isLoadingConfigPodcasts, setIsLoadingConfigPodcasts] =
    useState(false);

  // 构建返回列表页的链接（保留sort_by参数）
  const sortBy = searchParams.get("sort_by");
  const backLink = sortBy ? `/workflows?sort_by=${sortBy}` : "/workflows";

  const [showEditModal, setShowEditModal] = useState(false);

  // 使用自定义 Hooks
  const { handleToggle, handleTrigger, handleDelete } = useWorkflowActions({
    workflowId: id,
    workflow,
    onSuccess: () => mutateWorkflow(),
  });

  const { selectedJobId, jobDetails, loadingJobId, fetchJobDetail } = useJobExpansion();

  // 报告弹窗状态
  const [reportModalJobId, setReportModalJobId] = useState<number | null>(null);

  // 移动端更多菜单状态
  const [showMoreMenu, setShowMoreMenu] = useState(false);
  const moreMenuRef = useRef<HTMLDivElement>(null);
  const scopePodcastIds = useMemo(() => {
    if (workflow?.scope_type !== "specific_podcasts") {
      return [];
    }
    return workflow.scope_config?.podcast_ids ?? [];
  }, [workflow]);

  const overviewPodcastIds = useMemo(
    () => scopePodcastIds.slice(0, OVERVIEW_SCOPE_PODCAST_LIMIT),
    [scopePodcastIds],
  );

  const hiddenOverviewPodcastCount = Math.max(
    0,
    scopePodcastIds.length - overviewPodcastIds.length,
  );

  // 点击外部关闭更多菜单
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (moreMenuRef.current && !moreMenuRef.current.contains(event.target as Node)) {
        setShowMoreMenu(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // 概览页只拉前几个节目；离开概览后取消，避免与执行历史关键路径竞争
  useEffect(() => {
    let cancelled = false;

    if (activeTab !== "overview") {
      return () => {
        cancelled = true;
      };
    }

    if (workflow?.scope_type !== "specific_podcasts" || overviewPodcastIds.length === 0) {
      setOverviewPodcasts([]);
      setIsLoadingOverviewPodcasts(false);
      return;
    }

    const abortController = new AbortController();
    setIsLoadingOverviewPodcasts(true);
    podcastApi
      .batchGet(overviewPodcastIds, { signal: abortController.signal })
      .then((podcasts) => {
        if (!cancelled) {
          setOverviewPodcasts(podcasts);
        }
      })
      .catch((err) => {
        if (abortController.signal.aborted) {
          return;
        }
        if (!cancelled) {
          console.error("Failed to fetch overview podcasts:", err);
          setOverviewPodcasts([]);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setIsLoadingOverviewPodcasts(false);
        }
      });

    return () => {
      cancelled = true;
      abortController.abort();
    };
  }, [activeTab, overviewPodcastIds, workflow?.scope_type]);

  useEffect(() => {
    let cancelled = false;

    if (workflow?.scope_type !== "specific_podcasts" || scopePodcastIds.length === 0) {
      setConfigPodcasts([]);
      setIsLoadingConfigPodcasts(false);
      return;
    }

    if (activeTab !== "config") {
      return;
    }

    setIsLoadingConfigPodcasts(true);
    podcastApi
      .batchGet(scopePodcastIds)
      .then((podcasts) => {
        if (!cancelled) {
          setConfigPodcasts(podcasts);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          console.error("Failed to fetch config podcasts:", err);
          setConfigPodcasts([]);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setIsLoadingConfigPodcasts(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [activeTab, scopePodcastIds, workflow?.scope_type]);

  // 轮询Job状态：当有running状态的Job时，定期刷新
  useEffect(() => {
    if (activeTab !== "jobs") {
      return;
    }

    // 检查是否有running状态的job
    const hasRunningJob = jobs.some((job) => job.status === "running");

    if (!hasRunningJob) {
      return;
    }

    // 每3秒刷新一次Job列表
    const interval = setInterval(() => {
      mutateJobs();
    }, 3000);

    return () => clearInterval(interval);
  }, [activeTab, jobs, mutateJobs]);

  // 浏览器前进/后退时恢复 tab（URL 由 replaceState 维护，不经 App Router）
  useEffect(() => {
    const onPopState = () => {
      const tab = parseTab(new URLSearchParams(window.location.search).get("tab"));
      setActiveTabState(tab);
    };
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  // Jobs 分页切换
  const handleJobsPageChange = (newPage: number) => {
    setJobsPage(newPage);
  };

  // 格式化token数量
  const formatTokenCount = (tokens: number): string => {
    if (tokens === 0) return "0";
    if (tokens < 1000) return tokens.toString();
    if (tokens < 1000000) return `${(tokens / 1000).toFixed(1)}K`;
    return `${(tokens / 1000000).toFixed(1)}M`;
  };

  // 初始加载中：渲染骨架屏，避免空白
  if (workflowLoading) {
    return (
      <PageLayout
        toolbar={{
          breadcrumbs: [{ label: "返回列表", href: backLink }],
          title: "加载中...",
        }}
      >
        <WorkflowDetailSkeleton />
      </PageLayout>
    );
  }

  // 错误或不存在状态（加载完成后才判断）
  if (workflowError || !workflow) {
    return (
      <PageLayout
        toolbar={{
          breadcrumbs: [{ label: "返回列表", href: backLink }],
          title: "工作流详情",
        }}
      >
        <div className="bg-red-50 border border-red-200 rounded-lg p-6">
          <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
          <p className="text-red-600">{workflowError ? "加载失败" : "工作流不存在"}</p>
          <Link
            href={backLink}
            className="mt-4 inline-block text-blue-600 hover:text-blue-700"
          >
            ← 返回列表
          </Link>
        </div>
      </PageLayout>
    );
  }

  return (
    <PageLayout
      toolbar={{
        breadcrumbs: [{ label: "返回列表", href: backLink }],
        title: (
          <div className="flex items-center gap-3">
            <span>{`${workflow.id}: ${workflow.name}`}</span>
            <WorkflowStatusBadge isEnabled={workflow.is_enabled} />
          </div>
        ),
        description: workflow.description || undefined,
        rightContent: (
          <>
            {/* 移动端：更多操作下拉菜单 */}
            <div className="sm:hidden relative" ref={moreMenuRef}>
              <button
                onClick={() => setShowMoreMenu(!showMoreMenu)}
                className="p-2.5 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg"
                title="更多操作"
              >
                <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z" />
                </svg>
              </button>
              {showMoreMenu && (
                <div className="absolute right-0 top-full mt-1 bg-white dark:bg-slate-800 rounded-lg shadow-lg border border-slate-200 dark:border-slate-700 py-1 min-w-[140px] z-50">
                  <button
                    onClick={() => { handleTrigger(); setShowMoreMenu(false); }}
                    className="w-full px-4 py-3 text-left text-sm hover:bg-slate-50 dark:hover:bg-slate-700 flex items-center gap-3"
                  >
                    <svg className="w-4 h-4 text-blue-600 dark:text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M13 10V3L4 14h7v7l9-11h-7z" />
                    </svg>
                    执行
                  </button>
                  <button
                    onClick={() => { handleToggle(); setShowMoreMenu(false); }}
                    className="w-full px-4 py-3 text-left text-sm hover:bg-slate-50 dark:hover:bg-slate-700 flex items-center gap-3"
                  >
                    {workflow.is_enabled ? (
                      <>
                        <svg className="w-4 h-4 text-amber-600 dark:text-amber-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M10 9v6m4-6v6m7-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                        停用
                      </>
                    ) : (
                      <>
                        <svg className="w-4 h-4 text-green-600 dark:text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                        启用
                      </>
                    )}
                  </button>
                  <button
                    onClick={() => { setShowEditModal(true); setShowMoreMenu(false); }}
                    className="w-full px-4 py-3 text-left text-sm hover:bg-slate-50 dark:hover:bg-slate-700 flex items-center gap-3"
                  >
                    <svg className="w-4 h-4 text-slate-600 dark:text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2h2.828l8.586-8.586z" />
                    </svg>
                    编辑
                  </button>
                  <div className="border-t border-slate-200 dark:border-slate-700 my-1" />
                  <button
                    onClick={() => { handleDelete(); setShowMoreMenu(false); }}
                    className="w-full px-4 py-3 text-left text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 flex items-center gap-3"
                  >
                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                    </svg>
                    删除
                  </button>
                </div>
              )}
            </div>

            {/* 桌面端：原有按钮组 */}
            <div className="hidden sm:flex items-center gap-2">
              <button
                onClick={handleTrigger}
                className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors text-sm font-bold flex items-center gap-2"
                title="执行"
              >
                <svg
                  className="w-4 h-4 text-blue-600 dark:text-blue-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2.5}
                    d="M13 10V3L4 14h7v7l9-11h-7z"
                  />
                </svg>
                <span>执行</span>
              </button>
              <button
                onClick={handleToggle}
                className={`px-4 py-2 rounded-lg transition-colors text-sm font-bold flex items-center gap-2 ${
                  workflow.is_enabled
                    ? "bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-600"
                    : "bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-600"
                }`}
                title={workflow.is_enabled ? "停用" : "启用"}
              >
                {workflow.is_enabled ? (
                  <>
                    <svg
                      className="w-4 h-4 text-amber-600 dark:text-amber-400"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2.5}
                        d="M10 9v6m4-6v6m7-3a9 9 0 11-18 0 9 9 0 0118 0z"
                      />
                    </svg>
                    <span>停用</span>
                  </>
                ) : (
                  <>
                    <svg
                      className="w-4 h-4 text-green-600 dark:text-green-400"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2.5}
                        d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                      />
                    </svg>
                    <span>启用</span>
                  </>
                )}
              </button>
              <button
                onClick={() => setShowEditModal(true)}
                className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors text-sm font-bold flex items-center gap-2"
                title="编辑"
              >
                <svg
                  className="w-4 h-4 text-slate-800 dark:text-slate-200"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2.5}
                    d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2h2.828l8.586-8.586z"
                  />
                </svg>
                <span>编辑</span>
              </button>
              <button
                onClick={handleDelete}
                className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-red-600 dark:text-red-400 rounded-lg hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors text-sm font-bold flex items-center gap-2"
                title="删除"
              >
                <svg
                  className="w-4 h-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2.5}
                    d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                  />
                </svg>
                <span>删除</span>
              </button>
            </div>
          </>
        ),
      }}
    >
      <div className="py-6">
        {/* Tabs */}
        <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg p-4 mb-6">
          <div className="flex gap-6">
            <button
              type="button"
              onClick={() => setActiveTab("overview")}
              className={`pb-2 border-b-2 transition-colors text-base ${
                activeTab === "overview"
                  ? "border-blue-600 text-blue-600 dark:text-blue-400"
                  : "border-transparent text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200"
              }`}
            >
              📊 概览
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("jobs")}
              onMouseEnter={intentPrefetchJobs}
              onFocus={intentPrefetchJobs}
              onTouchStart={intentPrefetchJobs}
              className={`pb-2 border-b-2 transition-colors text-base ${
                activeTab === "jobs"
                  ? "border-blue-600 text-blue-600 dark:text-blue-400"
                  : "border-transparent text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200"
              }`}
            >
              📜 执行历史
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("config")}
              className={`pb-2 border-b-2 transition-colors text-base ${
                activeTab === "config"
                  ? "border-blue-600 text-blue-600 dark:text-blue-400"
                  : "border-transparent text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200"
              }`}
            >
              ⚙️ 配置
            </button>
          </div>
        </div>

        {/* Tab Content */}
        <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg p-6">
          {activeTab === "overview" && (
            <div>
              {/* 配置详情 */}
              <div className="space-y-6">
                <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-50">
                  配置详情
                </h2>

                {/* 调度配置 */}
                <div>
                  <h3 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-3 flex items-center gap-2">
                    <svg
                      className="w-4 h-4 text-green-600 dark:text-green-400"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                      />
                    </svg>
                    调度配置
                  </h3>
                  <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-5">
                    <div className="grid md:grid-cols-3 gap-6">
                      <div>
                        <p className="text-xs text-slate-600 dark:text-slate-400 mb-1">
                          定时规则
                        </p>
                        <code className="block px-3 py-2 bg-white dark:bg-slate-800 rounded-lg text-sm font-mono text-slate-900 dark:text-slate-50 border border-slate-200 dark:border-slate-700">
                          {workflow.schedule}
                        </code>
                      </div>
                      <div>
                        <p className="text-xs text-slate-600 dark:text-slate-400 mb-1">
                          上次执行
                        </p>
                        {workflow.stats?.last_execution ? (
                          <p className="text-sm font-medium text-slate-900 dark:text-slate-50">
                            {new Date(
                              workflow.stats.last_execution,
                            ).toLocaleString("zh-CN")}
                          </p>
                        ) : (
                          <p className="text-sm text-slate-500 dark:text-slate-400">
                            暂无记录
                          </p>
                        )}
                      </div>
                      <div>
                        <p className="text-xs text-slate-600 dark:text-slate-400 mb-1">
                          下次执行
                        </p>
                        {workflow.stats?.next_execution ? (
                          <p className="text-sm font-semibold text-blue-600 dark:text-blue-400">
                            {new Date(
                              workflow.stats.next_execution,
                            ).toLocaleString("zh-CN")}
                          </p>
                        ) : (
                          <p className="text-sm text-slate-500 dark:text-slate-400">
                            {workflow.is_enabled
                              ? "等待调度..."
                              : "工作流已禁用"}
                          </p>
                        )}
                      </div>
                    </div>
                  </div>
                </div>

                {/* 抓取与筛选配置 */}
                <div>
                  <h3 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-3 flex items-center gap-2">
                    <svg
                      className="w-4 h-4 text-blue-600 dark:text-blue-400"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                        d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                      />
                    </svg>
                    抓取与筛选
                  </h3>
                  <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-5">
                    {/* 抓取范围 */}
                    <div className="mb-6">
                      <div className="flex items-baseline gap-3 mb-4">
                        <span className="text-slate-600 dark:text-slate-400 text-sm">
                          抓取范围：
                        </span>
                        <span className="text-sm font-semibold text-slate-900 dark:text-slate-50">
                          {workflow.scope_type === "all_subscribed" &&
                            "全部订阅"}
                          {workflow.scope_type === "specific_podcasts" &&
                            `指定节目 (${workflow.scope_config?.podcast_ids?.length || 0}个)`}
                          {workflow.scope_type === "custom_sources" &&
                            "自定义源"}
                        </span>
                      </div>
                      {workflow.scope_type === "specific_podcasts" && (
                        <>
                          {overviewPodcasts.length > 0 ? (
                            <div className="flex flex-wrap gap-2 sm:gap-3">
                              {overviewPodcasts.map((podcast, index) => {
                                const effectiveCover = getEffectiveCoverUrl(podcast.custom_cover_url, podcast.cover_url);
                                return (
                                  <Link
                                    key={podcast.id}
                                    href={`/podcasts/${podcast.id}`}
                                    prefetch={false}
                                    className="group flex items-center gap-2 p-1 sm:px-3 sm:py-2 bg-white dark:bg-slate-800 rounded-lg border border-slate-200 dark:border-slate-700 hover:border-blue-400 dark:hover:border-blue-500 hover:shadow-md transition-all"
                                    title={podcast.title}
                                  >
                                    <div className="w-8 h-8 rounded-lg overflow-hidden flex-shrink-0">
                                      <PodcastCover
                                        coverUrl={effectiveCover}
                                        title={podcast.title}
                                        index={index}
                                        priority="low"
                                        sizes="32px"
                                      />
                                    </div>
                                    <span className="hidden sm:block text-xs font-semibold text-slate-900 dark:text-slate-50 group-hover:text-blue-600 dark:group-hover:text-blue-400 transition-colors truncate max-w-[120px]">
                                      {podcast.title}
                                    </span>
                                  </Link>
                                );
                              })}
                              {hiddenOverviewPodcastCount > 0 && (
                                <button
                                  type="button"
                                  onClick={() => setActiveTab("config")}
                                  className="inline-flex items-center rounded-lg border border-dashed border-slate-300 bg-white px-3 py-2 text-xs font-semibold text-slate-600 hover:border-blue-400 hover:text-blue-600 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300"
                                >
                                  +{hiddenOverviewPodcastCount} 个
                                </button>
                              )}
                            </div>
                          ) : isLoadingOverviewPodcasts ? (
                            <div className="text-sm text-slate-500 dark:text-slate-400">
                              正在加载播客列表...
                            </div>
                          ) : (
                            <div className="text-sm text-slate-500 dark:text-slate-400">
                              暂无可显示节目
                            </div>
                          )}
                        </>
                      )}
                    </div>

                    {/* 筛选规则 */}
                    <div>
                      {workflow.rules_config?.time_range ||
                      workflow.rules_config?.min_duration ||
                      workflow.rules_config?.max_results ||
                      workflow.rules_config?.keywords ||
                      workflow.rules_config?.exclude_words ? (
                        <div className="grid md:grid-cols-2 lg:grid-cols-4 gap-4">
                          {workflow.rules_config?.time_range &&
                            workflow.rules_config.time_range > 0 && (
                              <div className="bg-white dark:bg-slate-800 rounded-lg p-4 border border-slate-200 dark:border-slate-700">
                                <p className="text-xs text-slate-600 dark:text-slate-400 mb-1">
                                  时间范围
                                </p>
                                <p className="text-sm font-semibold text-slate-900 dark:text-slate-50">
                                  最近 {workflow.rules_config.time_range} 天
                                </p>
                              </div>
                            )}
                          {workflow.rules_config?.min_duration &&
                            workflow.rules_config.min_duration > 0 && (
                              <div className="bg-white dark:bg-slate-800 rounded-lg p-4 border border-slate-200 dark:border-slate-700">
                                <p className="text-xs text-slate-600 dark:text-slate-400 mb-1">
                                  最小时长
                                </p>
                                <p className="text-sm font-semibold text-slate-900 dark:text-slate-50">
                                  {Math.floor(
                                    workflow.rules_config.min_duration / 60,
                                  )}{" "}
                                  分钟
                                </p>
                              </div>
                            )}
                          {workflow.rules_config?.max_results &&
                            workflow.rules_config.max_results > 0 && (
                              <div className="bg-white dark:bg-slate-800 rounded-lg p-4 border border-slate-200 dark:border-slate-700">
                                <p className="text-xs text-slate-600 dark:text-slate-400 mb-1">
                                  最大结果数
                                </p>
                                <p className="text-sm font-semibold text-slate-900 dark:text-slate-50">
                                  {workflow.rules_config.max_results} 个
                                </p>
                              </div>
                            )}
                          {workflow.rules_config?.keywords && (
                            <div className="bg-white dark:bg-slate-800 rounded-lg p-4 border border-slate-200 dark:border-slate-700">
                              <p className="text-xs text-slate-600 dark:text-slate-400 mb-1">
                                关键词
                              </p>
                              <p className="text-base font-medium text-slate-900 dark:text-slate-50 break-all">
                                {workflow.rules_config.keywords}
                              </p>
                            </div>
                          )}
                          {workflow.rules_config?.exclude_words && (
                            <div className="bg-white dark:bg-slate-800 rounded-lg p-4 border border-slate-200 dark:border-slate-700">
                              <p className="text-xs text-slate-600 dark:text-slate-400 mb-1">
                                排除词
                              </p>
                              <p className="text-base font-medium text-slate-900 dark:text-slate-50 break-all">
                                {workflow.rules_config.exclude_words}
                              </p>
                            </div>
                          )}
                        </div>
                      ) : (
                        <div className="text-center py-8">
                          <svg
                            className="w-12 h-12 mx-auto text-slate-400 dark:text-slate-600 mb-3"
                            fill="none"
                            stroke="currentColor"
                            viewBox="0 0 24 24"
                          >
                            <path
                              strokeLinecap="round"
                              strokeLinejoin="round"
                              strokeWidth={2}
                              d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                            />
                          </svg>
                          <p className="text-sm text-slate-600 dark:text-slate-400">
                            无特殊筛选规则
                          </p>
                        </div>
                      )}
                    </div>
                  </div>
                </div>
              </div>

              {workflow.stats && (
                <div className="mt-6">
                  <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mb-3">
                    统计数据
                  </h3>
                  <div className="grid grid-cols-2 gap-4">
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <p className="text-xl font-bold text-slate-900 dark:text-slate-50">
                        {workflow.stats.total_jobs}
                      </p>
                      <p className="text-sm text-slate-600 dark:text-slate-400">
                        执行次数
                      </p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <p className="text-xl font-bold text-slate-900 dark:text-slate-50">
                        {workflow.stats.total_jobs > 0
                          ? workflow.stats.total_episodes.toFixed(1)
                          : "0.0"}
                      </p>
                      <p className="text-sm text-slate-600 dark:text-slate-400">
                        匹配单集/次
                      </p>
                    </div>
                  </div>
                </div>
              )}

              {workflow.last_job && (
                <div className="mt-6">
                  <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mb-3">
                    最近执行
                  </h3>
                  <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-3">
                        <JobStatusBadge status={workflow.last_job.status} />
                        <span className="text-sm text-slate-600 dark:text-slate-400">
                          {new Date(
                            workflow.last_job.created_at,
                          ).toLocaleString("zh-CN")}
                        </span>
                      </div>
                      {workflow.last_job.duration && (
                        <span className="text-sm text-slate-600 dark:text-slate-400">
                          耗时 {Math.floor(workflow.last_job.duration / 1000)}秒
                        </span>
                      )}
                    </div>
                    <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 sm:gap-4 text-sm">
                      <div>
                        <span className="text-slate-600 dark:text-slate-400">
                          处理节目:
                        </span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.last_job.podcasts_processed}
                        </span>
                      </div>
                      <div>
                        <span className="text-slate-600 dark:text-slate-400">
                          发现单集:
                        </span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.last_job.episodes_found}
                        </span>
                      </div>
                      <div>
                        <span className="text-slate-600 dark:text-slate-400">
                          创建单集:
                        </span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.last_job.episodes_created}
                        </span>
                      </div>
                      <div>
                        <span className="text-slate-600 dark:text-slate-400">
                          匹配单集:
                        </span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.last_job.episodes_matched}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}

          {activeTab === "jobs" && (
            <div>
              <h2 className="text-xl font-semibold text-slate-900 dark:text-slate-50 mb-4">
                执行历史
              </h2>
              {jobsLoading && jobs.length === 0 ? (
                <div className="text-center py-8 text-slate-500 dark:text-slate-400">
                  正在加载执行历史...
                </div>
              ) : jobsError && jobs.length === 0 ? (
                <div
                  className="text-center py-8"
                  data-testid="jobs-load-error"
                >
                  <p className="text-red-600 dark:text-red-400 mb-3">
                    执行历史加载失败，请重试
                  </p>
                  <button
                    type="button"
                    onClick={() => mutateJobs()}
                    className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm font-medium"
                  >
                    重试
                  </button>
                </div>
              ) : jobs.length === 0 ? (
                <div className="text-center py-8 text-slate-500 dark:text-slate-400">
                  暂无执行记录
                </div>
              ) : (
                <div className="space-y-3">
                  {jobs.map((job) => (
                    <div
                      key={job.id}
                      className="border border-slate-200 dark:border-slate-700 rounded-lg overflow-hidden"
                    >
                      {/* Job摘要卡片 - 可点击展开/收起 */}
                      <div
                        onClick={() => fetchJobDetail(job.id)}
                        className="p-3 sm:p-4 hover:bg-slate-50 dark:hover:bg-slate-900 transition-colors cursor-pointer"
                      >
                        {/* === 移动端：单行紧凑布局 === */}
                        <div className="flex sm:hidden items-center justify-between gap-2">
                          {/* 左侧：状态信息 */}
                          <div className="flex items-center gap-1.5 flex-shrink-0">
                            <span className="text-xs px-1.5 py-0.5 bg-slate-100 dark:bg-slate-700 rounded">
                              {job.triggered_by === "cron" ? "定时" : "手动"}
                            </span>
                            <JobStatusBadge status={job.status} />
                          </div>

                          {/* 中间：简化统计（仅匹配数和错误数） */}
                          <div className="flex items-center gap-3 text-xs flex-1 justify-center">
                            <span className="whitespace-nowrap">
                              <span className="text-slate-500">匹配数:</span>
                              <span className="ml-1 font-medium text-slate-900 dark:text-slate-50">{job.episodes_matched || 0}</span>
                            </span>
                            <span className="whitespace-nowrap">
                              <span className={job.error_count > 0 ? "text-red-500" : "text-slate-500"}>错误数:</span>
                              <span className={`ml-1 font-medium ${job.error_count > 0 ? "text-red-600" : "text-slate-900 dark:text-slate-50"}`}>{job.error_count}</span>
                            </span>
                          </div>

                          {/* 右侧：操作按钮 */}
                          <div className="flex items-center gap-1.5 flex-shrink-0">
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                if (job.status === "completed") {
                                  setReportModalJobId(job.id);
                                }
                              }}
                              disabled={job.status !== "completed"}
                              className={`p-1.5 rounded ${
                                job.status === "completed"
                                  ? "bg-blue-50 dark:bg-blue-900/20 text-blue-600 dark:text-blue-400"
                                  : "bg-slate-100 dark:bg-slate-800 text-slate-400"
                              }`}
                            >
                              <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z" />
                              </svg>
                            </button>
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                fetchJobDetail(job.id);
                              }}
                              className="p-1.5 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded"
                            >
                              <svg className={`w-3.5 h-3.5 transition-transform ${selectedJobId === job.id ? "rotate-180" : ""}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                              </svg>
                            </button>
                          </div>
                        </div>

                        {/* === 桌面端：两行布局 === */}
                        <div className="hidden sm:block">
                          <div className="flex items-start justify-between mb-2">
                            <div className="flex items-center gap-3">
                              <span className="text-sm px-2 py-1 bg-slate-100 dark:bg-slate-700 rounded flex-shrink-0">
                                {job.triggered_by === "cron" ? "定时" : "手动"}
                              </span>
                              <JobStatusBadge status={job.status} />
                              <span className="text-sm text-slate-600 dark:text-slate-400">
                                {new Date(job.created_at).toLocaleString("zh-CN")}
                              </span>
                              {job.duration && (
                                <span className="text-sm font-medium text-slate-700 dark:text-slate-300 px-2 py-1 bg-slate-100 dark:bg-slate-700 rounded flex-shrink-0">
                                  耗时：{Math.floor(job.duration / 1000)}s
                                </span>
                              )}
                              {job.llm_tokens_used && job.llm_model_used && (
                                <span className="text-xs px-2 py-1 bg-purple-100 dark:bg-purple-900/30 text-purple-800 dark:text-purple-300 rounded-full font-medium flex-shrink-0">
                                  🤖 AI: {formatTokenCount(job.llm_tokens_used)} ({job.llm_model_used})
                                </span>
                              )}
                            </div>
                            <div className="flex items-center gap-3">
                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  if (job.status === "completed") {
                                    setReportModalJobId(job.id);
                                  }
                                }}
                                disabled={job.status !== "completed"}
                                className={`px-4 py-1.5 rounded text-sm font-medium flex items-center gap-2 flex-shrink-0 transition-colors ${
                                  job.status === "completed"
                                    ? "bg-blue-50 dark:bg-blue-900/20 text-blue-600 dark:text-blue-400 hover:bg-blue-100 dark:hover:bg-blue-900/30 cursor-pointer"
                                    : "bg-slate-100 dark:bg-slate-800 text-slate-400 dark:text-slate-600 cursor-not-allowed"
                                }`}
                              >
                                <svg className="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z" />
                                </svg>
                                {job.status === "completed" ? "报告" : "生成中"}
                              </button>
                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  fetchJobDetail(job.id);
                                }}
                                className="px-4 py-1.5 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded text-sm font-medium hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors flex items-center gap-2 flex-shrink-0"
                              >
                                <svg className="w-4 h-4 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                                </svg>
                                {selectedJobId === job.id ? "收起" : "展开"}
                              </button>
                            </div>
                          </div>

                          {/* 统计信息网格 */}
                          <div className="grid grid-cols-5 gap-3 text-sm">
                            <div>
                              <span className="text-slate-600 dark:text-slate-400">处理节目:</span>
                              <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">{job.podcasts_processed}</span>
                            </div>
                            <div>
                              <span className="text-slate-600 dark:text-slate-400">发现单集:</span>
                              <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">{job.episodes_found}</span>
                            </div>
                            <div>
                              <span className="text-slate-600 dark:text-slate-400">创建单集:</span>
                              <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">{job.episodes_created}</span>
                            </div>
                            <div>
                              <span className="text-slate-600 dark:text-slate-400">匹配数:</span>
                              <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">{job.episodes_matched}</span>
                            </div>
                            <div>
                              <span className="text-slate-600 dark:text-slate-400">错误数:</span>
                              <span className={`ml-2 font-medium ${job.error_count > 0 ? "text-red-600" : "text-slate-900 dark:text-slate-50"}`}>
                                {job.error_count}
                              </span>
                            </div>
                          </div>
                        </div>
                      </div>

                      {/* 展开的详细执行记录 */}
                      {selectedJobId === job.id && (
                        <div className="border-t border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900/50">
                          {loadingJobId === job.id ? (
                            <div className="p-4 text-center text-sm text-slate-500 dark:text-slate-400">
                              加载中...
                            </div>
                          ) : jobDetails[job.id]?.executions ? (
                            <div className="p-4">
                              <h4 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-3">
                                详细执行记录
                              </h4>
                              <div className="space-y-2">
                                {jobDetails[job.id]?.executions?.map((exec) => (
                                  <div
                                    key={exec.id}
                                    className="bg-white dark:bg-slate-800 rounded-lg p-3 border border-slate-200 dark:border-slate-700"
                                  >
                                    <div className="flex items-start justify-between gap-2 mb-2">
                                      <div className="flex-1 min-w-0">
                                        <div className="flex items-center gap-2 mb-1">
                                          {exec.status === "success" && (
                                            <span className="text-green-600 dark:text-green-400 flex-shrink-0">
                                              ✓
                                            </span>
                                          )}
                                          {exec.status === "failed" && (
                                            <span className="text-red-600 dark:text-red-400 flex-shrink-0">
                                              ✗
                                            </span>
                                          )}
                                          {exec.status === "skipped" && (
                                            <span className="text-yellow-600 dark:text-yellow-400 flex-shrink-0">
                                              ○
                                            </span>
                                          )}
                                          <span className="font-medium text-slate-900 dark:text-slate-50 truncate">
                                            {exec.podcast_title}
                                          </span>
                                        </div>
                                        <a
                                          href={exec.podcast_feed_url}
                                          target="_blank"
                                          rel="noopener noreferrer"
                                          className="text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 truncate block"
                                        >
                                          {exec.podcast_feed_url}
                                        </a>
                                      </div>
                                      <span className="text-xs text-slate-500 dark:text-slate-400 flex-shrink-0 whitespace-nowrap">
                                        {exec.processing_time}ms
                                      </span>
                                    </div>

                                    <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
                                      <div>
                                        <span className="text-slate-600 dark:text-slate-400">
                                          状态:
                                        </span>
                                        <span className="ml-1 font-medium text-slate-900 dark:text-slate-50">
                                          {exec.status === "success" && "成功"}
                                          {exec.status === "failed" && "失败"}
                                          {exec.status === "skipped" && "跳过"}
                                          {exec.status === "running" &&
                                            "执行中"}
                                        </span>
                                      </div>
                                      <div>
                                        <span className="text-slate-600 dark:text-slate-400">
                                          发现:
                                        </span>
                                        <span className="ml-1 font-medium text-slate-900 dark:text-slate-50">
                                          {exec.episodes_found}
                                        </span>
                                      </div>
                                      <div>
                                        <span className="text-slate-600 dark:text-slate-400">
                                          新建:
                                        </span>
                                        <span className="ml-1 font-medium text-slate-900 dark:text-slate-50">
                                          {exec.episodes_created}
                                        </span>
                                      </div>
                                      <div>
                                        <span className="text-slate-600 dark:text-slate-400">
                                          匹配:
                                        </span>
                                        <span className="ml-1 font-medium text-slate-900 dark:text-slate-50">
                                          {exec.episodes_matched}
                                        </span>
                                      </div>
                                    </div>

                                    <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-slate-600 dark:text-slate-400">
                                      <span>
                                        Feed访问: {exec.feed_target_domain || "未知域名"} · {exec.feed_http_status ?? "无HTTP状态"} · {exec.feed_error_category || "未观测"}
                                      </span>
                                      <span>
                                        来源: {exec.feed_source_type || "未知"} · {exec.feed_freshness || "未知"} · 缓存: {exec.feed_cache_status || "未知"}
                                      </span>
                                      <span>
                                        来源地址: {exec.feed_source_url || "未记录"} · 身份: {exec.feed_identity_verification || "未检查"}
                                      </span>
                                      <span>
                                        内容时间: {formatDateTime(exec.feed_snapshot_retrieved_at, "未记录")}
                                      </span>
                                      <span>
                                        响应: {exec.feed_response_time_ms}ms · 出口: {exec.feed_egress_id || "未知"}
                                      </span>
                                      <span>
                                        断路: {exec.feed_circuit_state || "未使用"}
                                      </span>
                                    </div>

                                    {(exec.feed_retry_after || exec.feed_etag || exec.feed_last_modified || exec.feed_cache_control) && (
                                      <div className="mt-1 text-xs text-slate-500 dark:text-slate-400">
                                        响应元数据:
                                        {exec.feed_retry_after && ` Retry-After=${exec.feed_retry_after}`}
                                        {exec.feed_etag && ` ETag=${exec.feed_etag}`}
                                        {exec.feed_last_modified && ` Last-Modified=${exec.feed_last_modified}`}
                                        {exec.feed_cache_control && ` Cache-Control=${exec.feed_cache_control}`}
                                      </div>
                                    )}

                                    {exec.error_message && (
                                      <div className="mt-2 text-xs text-red-600 bg-red-50 dark:bg-red-900/20 rounded p-2">
                                        错误: {exec.error_message}
                                      </div>
                                    )}
                                  </div>
                                ))}
                              </div>

                              {jobDetails[job.id]?.executions?.length === 0 && (
                                <div className="text-center py-4 text-sm text-slate-500 dark:text-slate-400">
                                  暂无详细执行记录
                                </div>
                              )}
                            </div>
                          ) : (
                            <div className="p-4 text-center text-sm text-slate-500 dark:text-slate-400">
                              点击获取详细记录
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}

              {/* 分页 */}
              {pagination && pagination.total_pages > 1 && (
                <div className="mt-6 flex items-center justify-between">
                  <div className="text-sm text-slate-600 dark:text-slate-400">
                    第 {jobsPage} / {pagination.total_pages} 页
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => handleJobsPageChange(jobsPage - 1)}
                      disabled={jobsPage === 1}
                      className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm font-medium"
                    >
                      上一页
                    </button>
                    <button
                      onClick={() => handleJobsPageChange(jobsPage + 1)}
                      disabled={jobsPage === pagination.total_pages}
                      className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm font-medium"
                    >
                      下一页
                    </button>
                  </div>
                </div>
              )}
            </div>
          )}

          {activeTab === "config" && (
            <div>
              <h2 className="text-xl font-semibold text-slate-900 dark:text-slate-50 mb-4">
                配置预览
              </h2>
              <div className="space-y-4">
                <div>
                  <h3 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                    范围配置
                  </h3>
                  {workflow.scope_type === "specific_podcasts" ? (
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      {isLoadingConfigPodcasts ? (
                        <div className="text-sm text-slate-500 dark:text-slate-400">
                          正在加载完整节目列表...
                        </div>
                      ) : configPodcasts.length > 0 ? (
                        <div className="flex flex-wrap gap-2">
                          {configPodcasts.map((podcast, index) => {
                            const effectiveCover = getEffectiveCoverUrl(podcast.custom_cover_url, podcast.cover_url);
                            return (
                              <Link
                                key={podcast.id}
                                href={`/podcasts/${podcast.id}`}
                                prefetch={false}
                                className="text-sm px-3 py-2 bg-white dark:bg-slate-800 text-slate-900 dark:text-slate-50 rounded-lg border border-slate-200 dark:border-slate-700 hover:border-blue-400 dark:hover:border-blue-500 hover:shadow-sm transition-all"
                              >
                                <div className="flex items-center gap-2">
                                  <div className="w-8 h-8 rounded overflow-hidden flex-shrink-0">
                                    <PodcastCover
                                      coverUrl={effectiveCover}
                                      title={podcast.title}
                                      index={index}
                                      priority="low"
                                      sizes="32px"
                                    />
                                  </div>
                                  <span className="font-medium">
                                    {podcast.title}
                                  </span>
                                </div>
                              </Link>
                            );
                          })}
                        </div>
                      ) : (
                        <pre className="text-sm overflow-x-auto">
                          {JSON.stringify(workflow.scope_config, null, 2)}
                        </pre>
                      )}
                    </div>
                  ) : (
                    <pre className="bg-slate-100 dark:bg-slate-900 rounded-lg p-4 text-sm overflow-x-auto">
                      {JSON.stringify(workflow.scope_config, null, 2)}
                    </pre>
                  )}
                </div>
                <div>
                  <h3 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                    规则配置
                  </h3>
                  <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                    {workflow.rules_config?.time_range &&
                      workflow.rules_config.time_range > 0 && (
                        <div className="mb-2">
                          <span className="text-slate-600 dark:text-slate-400">
                            时间范围：
                          </span>
                          <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                            最近 {workflow.rules_config.time_range} 天
                          </span>
                        </div>
                      )}
                    {workflow.rules_config?.min_duration &&
                      workflow.rules_config.min_duration > 0 && (
                        <div className="mb-2">
                          <span className="text-slate-600 dark:text-slate-400">
                            最小时长：
                          </span>
                          <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                            {Math.floor(
                              workflow.rules_config.min_duration / 60,
                            )}{" "}
                            分钟
                          </span>
                        </div>
                      )}
                    {workflow.rules_config?.max_results &&
                      workflow.rules_config.max_results > 0 && (
                        <div className="mb-2">
                          <span className="text-slate-600 dark:text-slate-400">
                            最大结果数：
                          </span>
                          <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                            {workflow.rules_config.max_results} 个
                          </span>
                        </div>
                      )}
                    {workflow.rules_config?.keywords && (
                      <div className="mb-2">
                        <span className="text-slate-600 dark:text-slate-400">
                          关键词：
                        </span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.rules_config.keywords}
                        </span>
                      </div>
                    )}
                    {workflow.rules_config?.exclude_words && (
                      <div>
                        <span className="text-slate-600 dark:text-slate-400">
                          排除词：
                        </span>
                        <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                          {workflow.rules_config.exclude_words}
                        </span>
                      </div>
                    )}
                    {(!workflow.rules_config?.time_range ||
                      workflow.rules_config.time_range === 0) &&
                      !workflow.rules_config?.min_duration &&
                      !workflow.rules_config?.max_results &&
                      !workflow.rules_config?.keywords &&
                      !workflow.rules_config?.exclude_words && (
                        <p className="text-slate-500 dark:text-slate-400 text-sm">
                          无特殊规则
                        </p>
                      )}
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Edit Workflow Modal */}
      {workflow && (
        <WorkflowFormModal
          isOpen={showEditModal}
          onClose={() => setShowEditModal(false)}
          onSuccess={async () => {
            // 如果修改了schedule或is_enabled，需要重载调度器
            try {
              await schedulerApi.reload();
            } catch (err) {
              console.error("Failed to reload scheduler:", err);
            }
            mutateWorkflow();
            setShowEditModal(false);
          }}
          workflow={workflow}
        />
      )}

      {/* Report Modal */}
      {reportModalJobId !== null && (
        <ReportModal
          isOpen={reportModalJobId !== null}
          onClose={() => setReportModalJobId(null)}
          jobId={reportModalJobId}
          jobStatus={jobDetails[reportModalJobId]?.status || "pending"}
        />
      )}
    </PageLayout>
  );
}

// 默认导出：用 Suspense 包裹以支持 useSearchParams
export default function WorkflowDetailPage() {
  return (
    <Suspense fallback={
      <LoadingLayout
        showBack
        title="加载中..."
        rightContent={
          <div className="flex gap-2 animate-pulse">
            <div className="h-10 w-20 bg-slate-200 rounded-lg" />
            <div className="h-10 w-24 bg-slate-200 rounded-lg" />
          </div>
        }
      >
        <WorkflowDetailSkeleton />
      </LoadingLayout>
    }>
      <WorkflowDetailContent />
    </Suspense>
  );
}
