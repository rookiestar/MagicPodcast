"use client";
import { formatWorkflowSchedule } from "@/components/workflows/workflowFormConstants";

import { useEffect, useState, useRef, useMemo } from "react";
import { closeTo, navigate, positiveID, singleParam, updateQuery, useLocationHref } from "@/lib/navigation";
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
import { IconLoader2, IconPlayerPlay } from "@tabler/icons-react";

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

  const handleToggle = async (id: number, e?: React.MouseEvent) => {
    e?.preventDefault();
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

  const handleEdit = async (id:number,e?:React.MouseEvent) => {
    e?.preventDefault();
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

  return (
    <PageLayout
      rootClassName="editorial-page-shell rhythm-page-shell"
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

        {!error && !isLoading && workflows.length > 0 && (
          <div className="workflow-rhythm-list">
            {(["每日", "每周", "其他"] as const).map((group) => {
              const members = workflows.filter((workflow) => {
                const schedule = formatWorkflowSchedule(workflow.schedule);
                return (schedule.startsWith("每天 ") ? "每日" : schedule.startsWith("每周") ? "每周" : "其他") === group;
              });
              if (!members.length) return null;
              return (
                <section className="workflow-rhythm-group" key={group} aria-label={`${group}工作流`}>
                  <h2>{group}<span>{members.length} 个工作流</span></h2>
                  <ul>
                    {members.map((workflow) => (
                      <li key={workflow.id} className="workflow-rhythm-row">
                        <PrefetchLink
                          href={`/workflows/${workflow.id}${sortBy === "updated" ? "" : `?sort_by=${sortBy}`}`}
                          prefetchId={workflow.id}
                          prefetchType="workflow"
                          className="workflow-rhythm-name"
                        >
                          <h3>{workflow.name}</h3>
                          <p>{getScopeTypeLabel(workflow)}{workflow.description && ` · ${workflow.description}`}</p>
                        </PrefetchLink>
                        <WorkflowStatusBadge isEnabled={workflow.is_enabled} size="sm" />
                        <div className="workflow-rhythm-schedule">
                          <span>{formatWorkflowSchedule(workflow.schedule)}</span>
                          <small>下次执行 · {workflow.is_enabled ? formatDateTime(workflow.stats?.next_execution) : "已停用"}</small>
                        </div>
                        <div className="workflow-rhythm-actions">
                          <button type="button" className="workflow-quiet-action" onClick={(e) => handleTrigger(workflow.id, e)} disabled={triggeringId === workflow.id} aria-label={`执行工作流：${workflow.name}`} title="执行工作流">
                            {triggeringId === workflow.id ? <IconLoader2 size={18} className="animate-spin" aria-hidden="true" /> : <IconPlayerPlay size={18} aria-hidden="true" />}
                          </button>
                          <WorkflowActionMenu workflow={workflow} onToggle={handleToggle} onEdit={handleEdit} onDelete={handleDelete} />
                        </div>
                      </li>
                    ))}
                  </ul>
                </section>
              );
            })}
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
