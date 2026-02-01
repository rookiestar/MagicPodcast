"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { workflowApi, podcastApi } from "@/lib/api";
import { schedulerApi } from "@/lib/api/scheduler";
import type { Workflow, Job, Podcast } from "@/types";
import WorkflowFormModal from "@/components/workflows/WorkflowFormModal";
import ReportModal from "@/components/workflows/ReportModal";

type TabType = "overview" | "jobs" | "config";

export default function WorkflowDetailPage() {
  const params = useParams();
  const searchParams = useSearchParams();
  const router = useRouter();
  const id = parseInt(params.id as string);

  const [workflow, setWorkflow] = useState<Workflow | null>(null);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [podcasts, setPodcasts] = useState<Podcast[]>([]);

  // 从URL读取tab状态，如果没有则默认为overview
  const tabFromUrl = (searchParams.get("tab") as TabType) || "overview";
  const [activeTab, setActiveTab] = useState<TabType>(tabFromUrl);

  // 构建返回列表页的链接（保留sort_by参数）
  const sortBy = searchParams.get("sort_by");
  const backLink = sortBy ? `/workflows?sort_by=${sortBy}` : "/workflows";

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showEditModal, setShowEditModal] = useState(false);

  // Job分页状态
  const [jobsPage, setJobsPage] = useState(1);
  const [jobsTotalPages, setJobsTotalPages] = useState(1);

  // Job详情展开状态
  const [selectedJobId, setSelectedJobId] = useState<number | null>(null);
  const [jobDetails, setJobDetails] = useState<Record<number, Job>>({});
  const [loadingJobId, setLoadingJobId] = useState<number | null>(null);

  // 报告弹窗状态
  const [reportModalJobId, setReportModalJobId] = useState<number | null>(null);

  useEffect(() => {
    if (id) {
      fetchWorkflow();
      fetchJobs();
    }
  }, [id]);

  // 轮询Job状态：当有running状态的Job时，定期刷新
  useEffect(() => {
    // 检查是否有running状态的job
    const hasRunningJob = jobs.some((job) => job.status === "running");

    if (!hasRunningJob) {
      return;
    }

    // 每3秒刷新一次Job列表
    const interval = setInterval(() => {
      fetchJobs();
    }, 3000);

    return () => clearInterval(interval);
  }, [jobs]);

  // 同步activeTab到URL参数
  useEffect(() => {
    const currentUrl = new URL(window.location.href);
    const params = new URLSearchParams(currentUrl.search);

    if (params.get("tab") !== activeTab) {
      params.set("tab", activeTab);
      const newUrl = `${currentUrl.pathname}?${params.toString()}`;
      router.replace(newUrl);
    }
  }, [activeTab, router]);

  const fetchWorkflow = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await workflowApi.get(id);
      setWorkflow(data);

      // 如果是指定节目类型，获取播客列表
      if (
        data.scope_type === "specific_podcasts" &&
        data.scope_config?.podcast_ids &&
        data.scope_config.podcast_ids.length > 0
      ) {
        fetchPodcasts(data.scope_config.podcast_ids);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
      console.error("Failed to fetch workflow:", err);
    } finally {
      setLoading(false);
    }
  };

  const fetchPodcasts = async (podcastIds: number[]) => {
    try {
      // 直接使用 fetch API 调用批量查询接口
      const response = await fetch(
        "http://localhost:8080/api/v1/podcasts/batch",
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ ids: podcastIds }),
        },
      );

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }

      const result = await response.json();

      if (result.success && result.data) {
        const podcasts = result.data;
        setPodcasts(podcasts);
      } else {
        throw new Error(result.error?.message || "Failed to fetch podcasts");
      }
    } catch (err) {
      console.error("Failed to fetch podcasts:", err);
    }
  };

  const fetchJobs = async (page: number = jobsPage) => {
    try {
      const response = await workflowApi.listJobs(id, { page, page_size: 10 });
      setJobs(response.jobs);
      setJobsTotalPages(response.pagination.total_pages);
      setJobsPage(page);
    } catch (err) {
      console.error("Failed to fetch jobs:", err);
    }
  };

  const fetchJobDetail = async (jobId: number) => {
    // 如果已经缓存，直接切换展开状态
    if (jobDetails[jobId]) {
      setSelectedJobId(selectedJobId === jobId ? null : jobId);
      return;
    }

    // 如果是同一个Job且正在加载，不重复请求
    if (loadingJobId === jobId) {
      return;
    }

    try {
      setLoadingJobId(jobId);
      const detail = await workflowApi.getJob(jobId);
      setJobDetails((prev) => ({ ...prev, [jobId]: detail }));
      setSelectedJobId(jobId);
    } catch (err) {
      console.error("Failed to fetch job detail:", err);
      alert("获取详情失败");
    } finally {
      setLoadingJobId(null);
    }
  };

  const handleToggle = async () => {
    if (!workflow) return;
    try {
      const updated = await workflowApi.toggle(id);
      setWorkflow(updated);
    } catch (err) {
      alert(
        `操作失败: ${err instanceof Error ? err.message : "Unknown error"}`,
      );
    }
  };

  const handleTrigger = async () => {
    if (!workflow) return;
    if (!confirm("确定要立即执行此工作流吗?")) return;

    try {
      await workflowApi.trigger(id);
      alert("工作流已触发");
      fetchWorkflow();
      fetchJobs();
    } catch (err) {
      alert(
        `触发失败: ${err instanceof Error ? err.message : "Unknown error"}`,
      );
    }
  };

  const handleDelete = async () => {
    if (!workflow) return;
    if (!confirm("确定要删除这个工作流吗？此操作不可恢复。")) return;

    try {
      await workflowApi.delete(id);
      router.push("/workflows");
    } catch (err) {
      alert(
        `删除失败: ${err instanceof Error ? err.message : "Unknown error"}`,
      );
    }
  };

  const getStatusBadge = (isEnabled: boolean) => {
    return isEnabled ? (
      <span className="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
        ● 启用中
      </span>
    ) : (
      <span className="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300">
        ○ 已禁用
      </span>
    );
  };

  const getJobStatusBadge = (status: string) => {
    const statusMap: Record<string, { text: string; className: string }> = {
      pending: {
        text: "等待中",
        className:
          "bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-300",
      },
      running: {
        text: "执行中",
        className:
          "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
      },
      completed: {
        text: "已完成",
        className:
          "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
      },
      failed: {
        text: "失败",
        className: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
      },
      cancelled: {
        text: "已取消",
        className:
          "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200",
      },
    };
    const statusInfo = statusMap[status] || statusMap.pending;
    return (
      <span
        className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-sm font-medium ${statusInfo.className} flex-shrink-0`}
      >
        {statusInfo.text}
      </span>
    );
  };

  if (loading) {
    return (
      <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
        <div className="container mx-auto px-4 py-8">
          <div className="text-center py-12">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
            <p className="mt-4 text-slate-600 dark:text-slate-400">加载中...</p>
          </div>
        </div>
      </main>
    );
  }

  if (error || !workflow) {
    return (
      <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
        <div className="container mx-auto px-4 py-8">
          <div className="bg-red-50 border border-red-200 rounded-lg p-6">
            <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
            <p className="text-red-600">{error || "工作流不存在"}</p>
            <Link
              href={backLink}
              className="mt-4 inline-block text-blue-600 hover:text-blue-700"
            >
              ← 返回列表
            </Link>
          </div>
        </div>
      </main>
    );
  }

  return (
    <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
      <div className="container mx-auto px-4 py-8">
        {/* Header */}
        <div className="mb-8">
          <div className="mb-4">
            <Link
              href={backLink}
              className="w-36 h-11 px-4 bg-white dark:bg-slate-700 text-slate-800 dark:text-slate-200 font-medium rounded-xl border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-600 hover:border-slate-400 dark:hover:border-slate-500 transition-colors flex items-center justify-center gap-2"
            >
              <span>←</span>
              <span>返回列表</span>
            </Link>
          </div>

          <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg p-6">
            <div className="flex items-start justify-between mb-4">
              <div className="flex-1">
                <div className="flex items-center gap-3 mb-2">
                  <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-50">
                    {workflow.id}: {workflow.name}
                  </h1>
                  {getStatusBadge(workflow.is_enabled)}
                </div>
                {workflow.description && (
                  <p className="text-slate-600 dark:text-slate-400 mt-2">
                    {workflow.description}
                  </p>
                )}
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={handleTrigger}
                  className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors text-sm font-bold flex items-center gap-2"
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
                  手动执行
                </button>
                <button
                  onClick={handleToggle}
                  className={`px-4 py-2 rounded-lg transition-colors text-sm font-bold flex items-center gap-2 ${
                    workflow.is_enabled
                      ? "bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-600"
                      : "bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-600"
                  }`}
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
                      停用
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
                      启用
                    </>
                  )}
                </button>
                <button
                  onClick={() => setShowEditModal(true)}
                  className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors text-sm font-bold flex items-center gap-2"
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
                  编辑
                </button>
                <button
                  onClick={handleDelete}
                  className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-red-600 dark:text-red-400 rounded-lg hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors text-sm font-bold flex items-center gap-2"
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
                  删除
                </button>
              </div>
            </div>

            {/* Tabs */}
            <div className="border-t border-slate-200 dark:border-slate-700 pt-4">
              <div className="flex gap-6">
                <button
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
                  onClick={() => setActiveTab("jobs")}
                  className={`pb-2 border-b-2 transition-colors text-base ${
                    activeTab === "jobs"
                      ? "border-blue-600 text-blue-600 dark:text-blue-400"
                      : "border-transparent text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200"
                  }`}
                >
                  📜 执行历史
                </button>
              </div>
            </div>
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
                          {podcasts.length > 0 ? (
                            <div className="flex flex-wrap gap-3">
                              {podcasts.map((podcast) => (
                                <Link
                                  key={podcast.id}
                                  href={`/podcasts/${podcast.id}`}
                                  className="group flex items-center gap-2 px-3 py-2 bg-white dark:bg-slate-800 rounded-lg border border-slate-200 dark:border-slate-700 hover:border-blue-400 dark:hover:border-blue-500 hover:shadow-md transition-all"
                                >
                                  {podcast.cover_url && (
                                    <img
                                      src={podcast.cover_url}
                                      alt={podcast.title}
                                      className="w-8 h-8 rounded-lg object-cover"
                                    />
                                  )}
                                  <span className="text-xs font-semibold text-slate-900 dark:text-slate-50 group-hover:text-blue-600 dark:group-hover:text-blue-400 transition-colors">
                                    {podcast.title}
                                  </span>
                                </Link>
                              ))}
                            </div>
                          ) : (
                            <div className="text-sm text-slate-500 dark:text-slate-400">
                              正在加载播客列表...
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
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <p className="text-xl font-bold text-slate-900 dark:text-slate-50">
                        {workflow.stats.total_jobs}
                      </p>
                      <p className="text-sm text-slate-600 dark:text-slate-400">
                        执行次数
                      </p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <p className="text-xl font-bold text-green-600">
                        {workflow.stats.success_rate.toFixed(1)}%
                      </p>
                      <p className="text-sm text-slate-600 dark:text-slate-400">
                        成功率
                      </p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <p className="text-xl font-bold text-slate-900 dark:text-slate-50">
                        {workflow.stats.total_jobs > 0
                          ? (
                              workflow.stats.total_episodes /
                              workflow.stats.total_jobs
                            ).toFixed(1)
                          : "0.0"}
                      </p>
                      <p className="text-sm text-slate-600 dark:text-slate-400">
                        匹配单集/次
                      </p>
                    </div>
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <p className="text-xl font-bold text-slate-900 dark:text-slate-50">
                        {workflow.stats.success_jobs}
                      </p>
                      <p className="text-sm text-slate-600 dark:text-slate-400">
                        成功次数
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
                        {getJobStatusBadge(workflow.last_job.status)}
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
                    <div className="grid grid-cols-4 gap-4 text-sm">
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
              {jobs.length === 0 ? (
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
                        className="p-4 hover:bg-slate-50 dark:hover:bg-slate-900 transition-colors cursor-pointer"
                      >
                        <div className="flex items-start justify-between mb-2">
                          <div className="flex items-center gap-3">
                            <span className="text-sm px-2 py-1 bg-slate-100 dark:bg-slate-700 rounded flex-shrink-0">
                              {job.triggered_by === "cron" ? "定时" : "手动"}
                            </span>
                            {getJobStatusBadge(job.status)}
                            <span className="text-sm text-slate-600 dark:text-slate-400">
                              {new Date(job.created_at).toLocaleString("zh-CN")}
                            </span>
                            {job.duration && (
                              <span className="text-sm font-medium text-slate-700 dark:text-slate-300 px-2 py-1 bg-slate-100 dark:bg-slate-700 rounded flex-shrink-0">
                                耗时：{Math.floor(job.duration / 1000)}s
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
                              title={
                                job.status !== "completed"
                                  ? `报告生成中... (当前状态: ${job.status})`
                                  : "查看报告"
                              }
                            >
                              <svg
                                className="w-4 h-4 flex-shrink-0"
                                fill="none"
                                stroke="currentColor"
                                viewBox="0 0 24 24"
                              >
                                <path
                                  strokeLinecap="round"
                                  strokeLinejoin="round"
                                  strokeWidth={2}
                                  d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"
                                />
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
                              <svg
                                className="w-4 h-4 flex-shrink-0"
                                fill="none"
                                stroke="currentColor"
                                viewBox="0 0 24 24"
                              >
                                <path
                                  strokeLinecap="round"
                                  strokeLinejoin="round"
                                  strokeWidth={2}
                                  d="M19 9l-7 7-7-7"
                                />
                              </svg>
                              {selectedJobId === job.id ? "收起" : "展开"}
                            </button>
                          </div>
                        </div>

                        <div className="grid grid-cols-5 gap-3 text-sm">
                          <div>
                            <span className="text-slate-600 dark:text-slate-400">
                              处理节目:
                            </span>
                            <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                              {job.podcasts_processed}
                            </span>
                          </div>
                          <div>
                            <span className="text-slate-600 dark:text-slate-400">
                              发现单集:
                            </span>
                            <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                              {job.episodes_found}
                            </span>
                          </div>
                          <div>
                            <span className="text-slate-600 dark:text-slate-400">
                              创建单集:
                            </span>
                            <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                              {job.episodes_created}
                            </span>
                          </div>
                          <div>
                            <span className="text-slate-600 dark:text-slate-400">
                              匹配数:
                            </span>
                            <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                              {job.episodes_matched}
                            </span>
                          </div>
                          <div>
                            <span className="text-slate-600 dark:text-slate-400">
                              错误数:
                            </span>
                            <span
                              className={`ml-2 font-medium ${
                                job.error_count > 0
                                  ? "text-red-600"
                                  : "text-slate-900 dark:text-slate-50"
                              }`}
                            >
                              {job.error_count}
                            </span>
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
                                    <div className="flex items-start justify-between mb-2">
                                      <div className="flex-1">
                                        <div className="flex items-center gap-2 mb-1">
                                          {exec.status === "success" && (
                                            <span className="text-green-600 dark:text-green-400">
                                              ✓
                                            </span>
                                          )}
                                          {exec.status === "failed" && (
                                            <span className="text-red-600 dark:text-red-400">
                                              ✗
                                            </span>
                                          )}
                                          {exec.status === "skipped" && (
                                            <span className="text-yellow-600 dark:text-yellow-400">
                                              ○
                                            </span>
                                          )}
                                          <span className="font-medium text-slate-900 dark:text-slate-50">
                                            {exec.podcast_title}
                                          </span>
                                        </div>
                                        <a
                                          href={exec.podcast_feed_url}
                                          target="_blank"
                                          rel="noopener noreferrer"
                                          className="text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 truncate max-w-md block"
                                        >
                                          {exec.podcast_feed_url}
                                        </a>
                                      </div>
                                      <span className="text-xs text-slate-500 dark:text-slate-400">
                                        {exec.processing_time}ms
                                      </span>
                                    </div>

                                    <div className="grid grid-cols-4 gap-3 text-xs">
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
              {jobsTotalPages > 1 && (
                <div className="mt-6 flex items-center justify-between">
                  <div className="text-sm text-slate-600 dark:text-slate-400">
                    第 {jobsPage} / {jobsTotalPages} 页
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => fetchJobs(jobsPage - 1)}
                      disabled={jobsPage === 1}
                      className="px-4 py-2 bg-slate-100 dark:bg-slate-700 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-600 transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm font-medium"
                    >
                      上一页
                    </button>
                    <button
                      onClick={() => fetchJobs(jobsPage + 1)}
                      disabled={jobsPage === jobsTotalPages}
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
                  {workflow.scope_type === "specific_podcasts" &&
                  podcasts.length > 0 ? (
                    <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                      <div className="flex flex-wrap gap-2">
                        {podcasts.map((podcast) => (
                          <Link
                            key={podcast.id}
                            href={`/podcasts/${podcast.id}`}
                            className="text-sm px-3 py-2 bg-white dark:bg-slate-800 text-slate-900 dark:text-slate-50 rounded-lg border border-slate-200 dark:border-slate-700 hover:border-blue-400 dark:hover:border-blue-500 hover:shadow-sm transition-all"
                          >
                            <div className="flex items-center gap-2">
                              {podcast.cover_url && (
                                <img
                                  src={podcast.cover_url}
                                  alt={podcast.title}
                                  className="w-8 h-8 rounded object-cover"
                                />
                              )}
                              <span className="font-medium">
                                {podcast.title}
                              </span>
                            </div>
                          </Link>
                        ))}
                      </div>
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
            fetchWorkflow();
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
    </main>
  );
}
