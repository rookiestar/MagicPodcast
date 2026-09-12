"use client";

import { useEffect, useState, useRef, useMemo } from "react";
import { closeTo, navigate, positiveID, singleParam, updateQuery, useLocationHref } from "@/lib/navigation";
import Link from "next/link";
import dynamic from "next/dynamic";
import { workflowApi } from "@/lib/api";
import { showSuccess } from "@/lib/api/errorHandler";
import { requestTypedConfirmation } from "@/lib/confirmation";
import { useWorkflows } from "@/hooks/useWorkflowSWR";
import type { Workflow, WorkflowSortByType } from "@/types";
import WorkflowActionMenu from "@/components/workflows/WorkflowActionMenu";
import EditorialSortControls from "@/components/layout/EditorialSortControls";
import PageLayout from "@/components/layout/PageLayout";
import PrefetchLink from "@/components/common/PrefetchLink";
import { WorkflowStatusBadge } from "@/components/ui/StatusBadge";
import { formatDateTime } from "@/lib/timeUtils";
import {
  IconCircleCheck,
  IconEdit,
  IconLoader2,
  IconPlayerPause,
  IconPlayerPlay,
  IconTrash,
} from "@tabler/icons-react";

// 动态导入 WorkflowFormModal，减少首屏 bundle 大小
const WorkflowFormModal = dynamic(
  () => import("@/components/workflows/WorkflowFormModal"),
  { ssr: false }
);

export default function WorkflowsPage() {
  const href = useLocationHref();
  const query = useMemo(()=>new URL(href || "/", "http://navigation.local").searchParams,[href]);
  const dialog = singleParam(query,"dialog");
  const routeEditingId = dialog === "edit" ? positiveID(/^\/workflows\/([^/?#]+)/.exec(href)?.[1]) : null;
  const showCreateModal = dialog === "create" || dialog === "edit";
  const modalReturn = useRef("/workflows");
  const setShowCreateModal = (open:boolean) => {
    if (open) { modalReturn.current=window.location.pathname+window.location.search; updateQuery({dialog:"create"}); }
    else {
      if(dialog === "create"){const parent=new URLSearchParams(window.location.search);parent.delete("dialog");closeTo(`/workflows${parent.size?`?${parent}`:""}`);}
      else closeTo(modalReturn.current);
    }
  };
  const [editError,setEditError]=useState("");
  const [editRetry,setEditRetry]=useState(0);
  useEffect(()=>{
    const patch:Record<string,string|null>={};
    if(query.has("sort_by")&&!["updated","execution"].includes(singleParam(query,"sort_by")??""))patch.sort_by=null;
    if(query.has("dialog")&&!["create","edit"].includes(singleParam(query,"dialog")??""))patch.dialog=null;
    if(Object.keys(patch).length)updateQuery(patch,true);
  },[query]);
  const [editingWorkflow, setEditingWorkflow] = useState<Workflow | null>(null);
  const [triggeringId, setTriggeringId] = useState<number | null>(null);
  const sortBy: WorkflowSortByType = singleParam(query,"sort_by") === "execution" ? "execution" : "updated";

  // 使用 SWR 获取工作流列表
  const { workflows, isLoading, isError, mutate } = useWorkflows({
    sort_by: sortBy,
    view: "summary",
  });
  const error = isError ? "加载失败" : null;

  useEffect(() => {
    if (!routeEditingId) { setEditingWorkflow(null); setEditError(""); return; }
    let active=true;
    setEditError("");
    workflowApi.get(routeEditingId).then((workflow)=>{if(active)setEditingWorkflow(workflow);}).catch(()=>{if(active)setEditError("工作流读取失败，请重试。");});
    return ()=>{active=false;};
  },[routeEditingId,editRetry]);
  const handleSortChange = (newSortBy:WorkflowSortByType) => updateQuery({sort_by:newSortBy});

  const handleToggle = async (id: number, e: React.MouseEvent) => {
    e.preventDefault();
    try {
      await workflowApi.toggle(id);
      await mutate();
    } catch (err) {
      console.error("Failed to toggle workflow:", err);
    }
  };

  const handleTrigger = async (id: number, e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();

    if (triggeringId === id) return;
    const workflow = workflows.find((item) => item.id === id);
    const confirmationText = requestTypedConfirmation({
      action: `立即执行工作流“${workflow?.name || id}”`,
      impact: "可能抓取网络内容、写入数据库并调用 LLM。",
      phrase: `RUN WORKFLOW ${id}`,
    });
    if (!confirmationText) return;

    try {
      setTriggeringId(id);
      await workflowApi.trigger(id, confirmationText);
      showSuccess("工作流已开始执行，请在执行历史中查看进度");
      await mutate();
    } catch (err) {
      console.error("Failed to trigger workflow:", err);
      // 错误已通过axios拦截器自动处理
    } finally {
      setTriggeringId(null);
    }
  };

  const handleEdit = async (id:number,e:React.MouseEvent) => {
    e.preventDefault();
    modalReturn.current=window.location.pathname+window.location.search;
    navigate(`/workflows/${id}?dialog=edit`);
  };

  const handleDelete = async (id: number) => {
    const workflow = workflows.find((item) => item.id === id);
    const confirmationText = requestTypedConfirmation({
      action: `删除工作流“${workflow?.name || id}”`,
      impact: "会删除该工作流及其执行入口，此操作不可恢复。",
      phrase: `DELETE WORKFLOW ${id}`,
    });
    if (!confirmationText) return;

    try {
      await workflowApi.delete(id, confirmationText);
      await mutate();
    } catch (err) {
      console.error("Failed to delete workflow:", err);
    }
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

  return (
    <PageLayout
      rootClassName="editorial-page-shell"
      className="workflow-page wf-editorial"
      toolbar={{
        title: "工作流管理",
        description: workflows.length > 0 ? `${workflows.length} 个工作流` : undefined,
        rightContent: (
          <div className="flex items-center gap-3">
            <button
              onClick={() => setShowCreateModal(true)}
              className="editorial-btn editorial-btn--primary"
            >
              + 创建工作流
            </button>

            <EditorialSortControls<WorkflowSortByType>
              sortBy={sortBy}
              options={[
                { label: "最近更新", value: "updated" },
                { label: "下次执行", value: "execution" },
              ]}
              onSortChange={handleSortChange}
            />
          </div>
        ),
        className: "editorial-page-toolbar",
      }}
    >
      <div className="workflow-content py-6">
        {/* Error State */}
        {error && (
          <div className="editorial-state is-error">
            <h3>加载失败</h3>
            <p>{error}</p>
            <button onClick={() => mutate()} className="editorial-btn editorial-btn--danger">
              重试
            </button>
          </div>
        )}

        {/* Loading State - loading.tsx 已处理，此处不再重复显示骨架屏 */}

        {/* Empty State - 只在非加载状态且无数据时显示 */}
        {!error && !isLoading && workflows.length === 0 && (
          <div className="editorial-state">
            <h3>暂无工作流</h3>
            <p>创建你的第一个工作流，自动抓取、筛选并整理感兴趣的播客单集。</p>
            <button
              onClick={() => setShowCreateModal(true)}
              className="editorial-btn editorial-btn--primary"
            >
              创建工作流
            </button>
          </div>
        )}

        {/* Workflows List */}
        {!error && !isLoading && workflows.length > 0 && (
          <div className="workflow-list">
            {workflows.map((workflow) => (
              <div key={workflow.id} className="workflow-card">
                {/* Mobile: Simplified Card */}
                <div className="workflow-card-mobile md:hidden p-4">
                  <div className="flex items-start justify-between mb-3">
                    <div className="flex-1 min-w-0">
                      {/* 标题 + 状态 */}
                      <div className="flex items-center gap-2 mb-2">
                        <h3 className="text-base font-semibold text-slate-900 truncate">
                          {workflow.name}
                        </h3>
                        <WorkflowStatusBadge isEnabled={workflow.is_enabled} compact />
                      </div>

                      {/* 关键信息 */}
                      {workflow.stats && (
                        <div className="text-xs text-slate-600 space-y-1">
                          <p>下次执行: {formatDateTime(workflow.stats.next_execution)}</p>
                          <p>上次执行: {formatDateTime(workflow.stats.last_execution)}</p>
                          <p>匹配单集: <span className="text-blue-600 font-medium">{workflow.stats.total_episodes.toFixed(1)}</span></p>
                        </div>
                      )}
                    </div>

                    {/* 操作按钮 */}
                    <div className="flex items-center gap-2 ml-3">
                      {/* 执行 */}
                      <button
                        onClick={(e) => {
                          e.preventDefault();
                          handleTrigger(workflow.id, e);
                        }}
                        disabled={triggeringId === workflow.id}
                        className={`p-2.5 border border-slate-200 rounded-lg transition-all flex-shrink-0 ${
                          triggeringId === workflow.id
                            ? "opacity-50 cursor-not-allowed bg-slate-100"
                            : "text-blue-600 hover:bg-slate-50 hover:border-blue-300 active:scale-95"
                        }`}
                        style={{ minWidth: "44px", minHeight: "44px" }}
                        title="执行"
                      >
                        {triggeringId === workflow.id ? (
                          <IconLoader2 className="w-5 h-5 animate-spin" aria-hidden="true" />
                        ) : (
                          <IconPlayerPlay className="w-5 h-5" aria-hidden="true" />
                        )}
                      </button>

                      {/* 更多菜单 */}
                      <WorkflowActionMenu
                        workflow={workflow}
                        onToggle={(id) => {
                          const e = new MouseEvent("click") as any;
                          handleToggle(id, e);
                        }}
                        onEdit={(id) => {
                          const e = new MouseEvent("click") as any;
                          handleEdit(id, e);
                        }}
                        onDelete={handleDelete}
                      />
                    </div>
                  </div>

                  {/* 查看详情链接 */}
                  <Link
                    href={`/workflows/${workflow.id}${sortBy === "updated" ? "" : `?sort_by=${sortBy}`}`}
                    prefetch={false}
                    className="block text-center text-sm text-blue-600 py-2 border-t border-slate-200 hover:text-blue-700 transition-colors"
                  >
                    查看详情 →
                  </Link>
                </div>

                {/* Desktop: Full Card */}
                <div className="workflow-card-desktop hidden md:block">
                  <PrefetchLink
                    href={`/workflows/${workflow.id}${sortBy === "updated" ? "" : `?sort_by=${sortBy}`}`}
                    prefetchId={workflow.id}
                    prefetchType="workflow"
                    className="workflow-card-body block"
                  >
                    <div className="workflow-card-main">
                      <div className="workflow-card-heading">
                        <span className="workflow-card-index" aria-hidden="true">
                          {String(workflow.id).padStart(2, "0")}
                        </span>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2 mb-2">
                            <h3 className="text-lg font-semibold text-slate-900">
                              {workflow.name}
                            </h3>
                            <WorkflowStatusBadge isEnabled={workflow.is_enabled} size="sm" />
                          </div>

                          {workflow.description && (
                            <p className="workflow-card-description text-slate-600 text-sm">
                              {workflow.description}
                            </p>
                          )}
                        </div>
                      </div>

                      <div className="workflow-card-metadata text-sm text-slate-600">
                        <div className="workflow-card-meta">
                          <span className="font-medium">范围</span>
                          <span className="text-slate-500">
                            {getScopeTypeLabel(workflow)}
                          </span>
                        </div>

                        <div className="workflow-card-meta">
                          <span className="font-medium">时间范围</span>
                          <span className="text-slate-500">
                            {formatTimeRange(workflow.rules_config?.time_range)}
                          </span>
                        </div>

                        <div className="workflow-card-meta">
                          <span className="font-medium">定时</span>
                          <code className="px-1.5 py-0.5 bg-slate-100 rounded text-xs">
                            {workflow.schedule}
                          </code>
                        </div>

                        {workflow.stats && (
                          <>
                            <div className="workflow-card-meta">
                              <span className="font-medium">上次执行</span>
                              <span className="text-slate-500">
                                {formatDateTime(workflow.stats.last_execution)}
                              </span>
                            </div>

                            <div className="workflow-card-meta">
                              <span className="font-medium">匹配单集</span>
                              <span className="text-blue-500">
                                {workflow.stats.total_episodes.toFixed(1)}
                              </span>
                            </div>

                            <div className="workflow-card-meta">
                              <span className="font-medium">下次执行</span>
                              <span className="text-slate-500">
                                {formatDateTime(workflow.stats.next_execution)}
                              </span>
                            </div>

                            <div className="workflow-card-meta">
                              <span className="font-medium">执行次数</span>
                              <span className="text-slate-500">
                                {workflow.stats.total_jobs}
                              </span>
                            </div>
                          </>
                        )}
                      </div>
                    </div>

                    {/* Actions */}
                    <div className="workflow-card-actions">
                      <span className="workflow-card-detail-cue">查看详情</span>
                      <div className="workflow-card-action-buttons">
                        {/* 执行 */}
                        <button
                          onClick={(e) => handleTrigger(workflow.id, e)}
                          disabled={triggeringId === workflow.id}
                          className={`workflow-card-action ${
                            triggeringId === workflow.id
                              ? "opacity-50 cursor-not-allowed bg-slate-100"
                              : "text-blue-600"
                          }`}
                          title="执行"
                          aria-label={`执行工作流：${workflow.name}`}
                        >
                          {triggeringId === workflow.id ? (
                            <IconLoader2 className="animate-spin" aria-hidden="true" />
                          ) : (
                            <IconPlayerPlay aria-hidden="true" />
                          )}
                        </button>

                        {/* 启用/停用 */}
                        <button
                          onClick={(e) => handleToggle(workflow.id, e)}
                          className={`workflow-card-action ${
                            workflow.is_enabled
                              ? "text-amber-600 dark:text-amber-400"
                              : "text-green-600 dark:text-green-400"
                          }`}
                          title={workflow.is_enabled ? "停用" : "启用"}
                          aria-label={`${workflow.is_enabled ? "停用" : "启用"}工作流：${workflow.name}`}
                        >
                          {workflow.is_enabled ? (
                            <IconPlayerPause aria-hidden="true" />
                          ) : (
                            <IconCircleCheck aria-hidden="true" />
                          )}
                        </button>

                        {/* 编辑 */}
                        <button
                          onClick={(e) => handleEdit(workflow.id, e)}
                          className="workflow-card-action text-slate-800 dark:text-slate-200"
                          title="编辑"
                          aria-label={`编辑工作流：${workflow.name}`}
                        >
                          <IconEdit aria-hidden="true" />
                        </button>

                        {/* 删除 */}
                        <button
                          onClick={(e) => {
                            e.preventDefault();
                            handleDelete(workflow.id);
                          }}
                          className="workflow-card-action text-red-600 dark:text-red-400"
                          title="删除"
                          aria-label={`删除工作流：${workflow.name}`}
                        >
                          <IconTrash aria-hidden="true" />
                        </button>
                      </div>
                    </div>
                  </PrefetchLink>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Create/Edit Workflow Modal */}
      {dialog === "edit" && !routeEditingId && <p role="alert">编辑地址缺少有效的工作流编号。</p>}
      {showCreateModal && routeEditingId && editingWorkflow?.id !== routeEditingId && <div role={editError ? "alert" : "status"}>{editError || "正在读取工作流…"}{editError && <button onClick={()=>setEditRetry(value=>value+1)}>重试读取工作流</button>}<button onClick={()=>setShowCreateModal(false)}>返回列表</button></div>}
      <WorkflowFormModal
        key={routeEditingId ?? "create"}
        isOpen={showCreateModal && (dialog === "create" || (routeEditingId !== null && editingWorkflow?.id === routeEditingId))}
        workflow={routeEditingId ? editingWorkflow : null}
        onClose={() => {
          setShowCreateModal(false);
          setEditingWorkflow(null);
        }}
        onSuccess={async () => {
          await mutate();
          setShowCreateModal(false);
          setEditingWorkflow(null);
        }}
      />
    </PageLayout>
  );
}
