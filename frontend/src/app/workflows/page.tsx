"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { workflowApi } from "@/lib/api";
import { showSuccess } from "@/lib/api/errorHandler";
import type { Workflow, WorkflowSortByType } from "@/types";
import WorkflowFormModal from "@/components/workflows/WorkflowFormModal";
import PageLayout from "@/components/layout/PageLayout";

export default function WorkflowsPage() {
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [editingWorkflow, setEditingWorkflow] = useState<Workflow | null>(null);
  const [triggeringId, setTriggeringId] = useState<number | null>(null);
  const [sortBy, setSortBy] = useState<WorkflowSortByType>("updated");

  useEffect(() => {
    // 从URL加载排序参数
    const params = new URLSearchParams(window.location.search);
    const sortFromUrl =
      (params.get("sort_by") as WorkflowSortByType) || "updated";
    setSortBy(sortFromUrl);

    fetchWorkflows(sortFromUrl);
  }, []);

  // 监听 URL 参数变化（用于浏览器前进/后退）
  useEffect(() => {
    const handlePopState = () => {
      const params = new URLSearchParams(window.location.search);
      const sortFromUrl =
        (params.get("sort_by") as WorkflowSortByType) || "updated";

      console.log("[PopState] sortBy:", sortFromUrl);

      setSortBy(sortFromUrl);
      fetchWorkflows(sortFromUrl);
    };

    window.addEventListener("popstate", handlePopState);
    return () => window.removeEventListener("popstate", handlePopState);
  }, []);

  const fetchWorkflows = async (currentSortBy: WorkflowSortByType = sortBy) => {
    try {
      setLoading(true);
      setError(null);
      const response = await workflowApi.list({ sort_by: currentSortBy });
      setWorkflows(response.workflows);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unknown error");
      console.error("Failed to fetch workflows:", err);
    } finally {
      setLoading(false);
    }
  };

  const handleSortChange = (newSortBy: WorkflowSortByType) => {
    console.log("[Sort] Changing from", sortBy, "to", newSortBy);

    // 更新 URL 参数
    const url = new URL(window.location.href);
    url.searchParams.set("sort_by", newSortBy);
    window.history.replaceState({}, "", url.toString());

    // 更新状态并重新获取数据
    setSortBy(newSortBy);
    fetchWorkflows(newSortBy);
  };

  const handleToggle = async (id: number, e: React.MouseEvent) => {
    e.preventDefault();
    try {
      await workflowApi.toggle(id);
      await fetchWorkflows();
    } catch (err) {
      // 错误已通过axios拦截器自动处理
      console.error("Failed to toggle workflow:", err);
    }
  };

  const handleTrigger = async (id: number, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();

    try {
      setTriggeringId(id);
      await workflowApi.trigger(id);
      showSuccess("工作流已开始执行，请在执行历史中查看进度");
      await fetchWorkflows();
    } catch (err) {
      console.error("Failed to trigger workflow:", err);
      // 错误已通过axios拦截器自动处理
    } finally {
      setTriggeringId(null);
    }
  };

  const handleEdit = async (id: number, e: React.MouseEvent) => {
    e.preventDefault();
    try {
      console.log("[Edit] Fetching workflow from API, ID:", id);
      const latestWorkflow = await workflowApi.get(id);
      console.log("[Edit] Latest workflow from API:", latestWorkflow);
      console.log("[Edit] rules_config from API:", latestWorkflow.rules_config);
      console.log(
        "[Edit] llm_enabled:",
        latestWorkflow.rules_config?.llm_enabled,
      );
      setEditingWorkflow(latestWorkflow);
      setShowCreateModal(true);
    } catch (err) {
      console.error("[Edit] Failed to fetch workflow from API:", err);
      // Fallback to local state
      const workflow = workflows.find((w) => w.id === id);
      if (workflow) {
        console.log("[Edit] Using local state fallback:", workflow);
        console.log("[Edit] Local rules_config:", workflow.rules_config);
        setEditingWorkflow(workflow);
        setShowCreateModal(true);
      }
    }
  };

  const handleDelete = async (id: number) => {
    if (!confirm("确定要删除这个工作流吗？")) return;

    try {
      await workflowApi.delete(id);
      await fetchWorkflows();
    } catch (err) {
      // 错误已通过axios拦截器自动处理
      console.error("Failed to delete workflow:", err);
    }
  };

  const getStatusBadge = (status: boolean) => {
    return status ? (
      <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
        启用中
      </span>
    ) : (
      <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-800">
        已禁用
      </span>
    );
  };

  const getScopeTypeLabel = (workflow: Workflow) => {
    let label = "";
    switch (workflow.scope_type) {
      case "specific_podcasts":
        label = "指定节目";
        break;
      case "all_subscribed":
        label = "全部订阅";
        break;
      case "custom_sources":
        label = "自定义源";
        break;
      default:
        label = workflow.scope_type;
    }

    // 如果有统计信息且有节目数，添加节目数
    if (
      workflow.stats &&
      workflow.stats.podcast_count !== undefined &&
      workflow.stats.podcast_count > 0
    ) {
      label += `（${workflow.stats.podcast_count}）`;
    }

    return label;
  };

  const formatTimeRange = (timeRange?: number) => {
    if (!timeRange || timeRange === 0) return "不限制";
    return `最近${timeRange}天`;
  };

  const formatDateTime = (dateStr?: string) => {
    if (!dateStr) return "-";
    const date = new Date(dateStr);
    return date.toLocaleString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  return (
    <PageLayout
      toolbar={{
        breadcrumbs: [{ label: "返回首页", href: "/" }],
        title: "工作流管理",
        description: !loading && workflows.length > 0 ? `${workflows.length} 个工作流` : undefined,
        rightContent: (
          <div className="flex items-center gap-3">
            <button
              onClick={() => setShowCreateModal(true)}
              className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors text-sm font-medium"
            >
              + 创建工作流
            </button>

            <select
              value={sortBy}
              onChange={(e) =>
                handleSortChange(e.target.value as WorkflowSortByType)
              }
              className="px-3 py-2 pr-8 border border-slate-300 rounded-lg bg-white text-sm text-slate-700 focus:ring-2 focus:ring-violet-500 focus:border-transparent transition-colors appearance-none cursor-pointer"
            >
              <option value="updated">最近更新</option>
              <option value="execution">下次执行</option>
            </select>
          </div>
        ),
      }}
    >
      <div className="py-6">
        {/* Loading State */}
        {loading && (
          <div className="text-center py-12">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
            <p className="mt-4 text-slate-600">加载中...</p>
          </div>
        )}

        {/* Error State */}
        {error && (
          <div className="bg-red-50 border border-red-200 rounded-lg p-6">
            <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
            <p className="text-red-600">{error}</p>
          </div>
        )}

        {/* Empty State */}
        {!loading && !error && workflows.length === 0 && (
          <div className="bg-white rounded-lg p-12 text-center shadow-sm">
            <div className="text-6xl mb-4">⚙️</div>
            <p className="text-slate-600 text-lg">暂无工作流</p>
            <p className="text-slate-5000 text-sm mt-2">
              点击上方按钮创建你的第一个工作流
            </p>
          </div>
        )}

        {/* Workflows List */}
        {!loading && !error && workflows.length > 0 && (
          <div className="space-y-4">
            {workflows.map((workflow, index) => (
              <Link
                key={workflow.id}
                href={`/workflows/${workflow.id}${window.location.search}`}
                className={`block rounded-lg shadow-sm hover:shadow-md transition-shadow p-5 ${
                  index % 2 === 0 ? "bg-white" : "bg-neutral-50"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-2 mb-2">
                      <h3 className="text-lg font-semibold text-slate-900">
                        {workflow.id}: {workflow.name}
                      </h3>
                      {getStatusBadge(workflow.is_enabled)}
                    </div>

                    {workflow.description && (
                      <p className="text-slate-600 text-sm mb-4">
                        {workflow.description}
                      </p>
                    )}

                    <div className="flex flex-wrap gap-x-5 gap-y-2 text-sm text-slate-600">
                      <div className="flex items-center gap-1.5">
                        <span className="font-medium">范围:</span>
                        <span className="text-slate-500">
                          {getScopeTypeLabel(workflow)}
                        </span>
                      </div>

                      <div className="flex items-center gap-1.5">
                        <span className="font-medium">时间范围:</span>
                        <span className="text-slate-500">
                          {formatTimeRange(workflow.rules_config?.time_range)}
                        </span>
                      </div>

                      <div className="flex items-center gap-1.5">
                        <span className="font-medium">定时:</span>
                        <code className="px-1.5 py-0.5 bg-slate-100 rounded text-xs">
                          {workflow.schedule}
                        </code>
                      </div>

                      {workflow.stats && (
                        <>
                          <div className="flex items-center gap-1.5">
                            <span className="font-medium">上次执行:</span>
                            <span className="text-slate-500">
                              {formatDateTime(workflow.stats.last_execution)}
                            </span>
                          </div>

                          <div className="flex items-center gap-1.5">
                            <span className="font-medium">匹配单集:</span>
                            <span className="text-blue-500">
                              {workflow.stats.total_episodes.toFixed(1)}
                            </span>
                          </div>

                          <div className="flex items-center gap-1.5">
                            <span className="font-medium">下次执行:</span>
                            <span className="text-slate-500">
                              {formatDateTime(workflow.stats.next_execution)}
                            </span>
                          </div>

                          <div className="flex items-center gap-1.5">
                            <span className="font-medium">执行次数:</span>
                            <span className="text-slate-500">
                              {workflow.stats.total_jobs}
                            </span>
                          </div>
                        </>
                      )}
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center gap-2 ml-4">
                    {/* 手动执行 */}
                    <button
                      onClick={(e) => handleTrigger(workflow.id, e)}
                      disabled={triggeringId === workflow.id}
                      className={`p-2.5 border border-slate-200 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 hover:border-blue-300 dark:hover:border-blue-500 transition-all ${
                        triggeringId === workflow.id
                          ? "opacity-50 cursor-not-allowed bg-slate-100"
                          : "text-blue-600 dark:text-blue-400"
                      }`}
                      title="手动执行"
                    >
                      {triggeringId === workflow.id ? (
                        <svg
                          className="w-5 h-5 animate-spin"
                          fill="none"
                          viewBox="0 0 24 24"
                        >
                          <circle
                            className="opacity-25"
                            cx="12"
                            cy="12"
                            r="10"
                            stroke="currentColor"
                            strokeWidth="4"
                          ></circle>
                          <path
                            className="opacity-75"
                            fill="currentColor"
                            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                          ></path>
                        </svg>
                      ) : (
                        <svg
                          className="w-5 h-5"
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
                      )}
                    </button>

                    {/* 启用/停用 */}
                    <button
                      onClick={(e) => handleToggle(workflow.id, e)}
                      className={`p-2.5 border border-slate-200 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 hover:border-slate-300 dark:hover:border-slate-500 transition-all ${
                        workflow.is_enabled
                          ? "text-amber-600 dark:text-amber-400"
                          : "text-green-600 dark:text-green-400"
                      }`}
                      title={workflow.is_enabled ? "停用" : "启用"}
                    >
                      {workflow.is_enabled ? (
                        <svg
                          className="w-5 h-5"
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
                      ) : (
                        <svg
                          className="w-5 h-5"
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
                      )}
                    </button>

                    {/* 编辑 */}
                    <button
                      onClick={(e) => handleEdit(workflow.id, e)}
                      className="p-2.5 text-slate-800 dark:text-slate-200 border border-slate-200 dark:border-slate-600 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-700 hover:border-slate-300 dark:hover:border-slate-500 transition-all"
                      title="编辑"
                    >
                      <svg
                        className="w-5 h-5"
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
                    </button>

                    {/* 删除 */}
                    <button
                      onClick={(e) => {
                        e.preventDefault();
                        handleDelete(workflow.id);
                      }}
                      className="p-2.5 text-red-600 dark:text-red-400 border border-slate-200 dark:border-slate-600 rounded-lg hover:bg-red-50 dark:hover:bg-red-900/20 hover:border-red-300 dark:hover:border-red-500 transition-all"
                      title="删除"
                    >
                      <svg
                        className="w-5 h-5"
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
                    </button>
                  </div>
                </div>
              </Link>
            ))}
          </div>
        )}
      </div>

      {/* Create/Edit Workflow Modal */}
      <WorkflowFormModal
        isOpen={showCreateModal}
        workflow={editingWorkflow}
        onClose={() => {
          setShowCreateModal(false);
          setEditingWorkflow(null);
        }}
        onSuccess={() => {
          fetchWorkflows();
          setShowCreateModal(false);
          setEditingWorkflow(null);
        }}
      />
    </PageLayout>
  );
}
