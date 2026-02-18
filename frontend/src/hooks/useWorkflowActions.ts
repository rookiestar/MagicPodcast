import { useCallback } from "react";
import { useRouter } from "next/navigation";
import { workflowApi } from "@/lib/api";
import { toast } from "@/lib/toast";
import type { Workflow } from "@/types";

interface UseWorkflowActionsOptions {
  workflowId: number;
  workflow: Workflow | undefined;
  onSuccess?: () => void;
}

interface UseWorkflowActionsReturn {
  handleToggle: () => Promise<void>;
  handleTrigger: () => Promise<void>;
  handleDelete: () => Promise<void>;
}

export function useWorkflowActions({
  workflowId,
  workflow,
  onSuccess,
}: UseWorkflowActionsOptions): UseWorkflowActionsReturn {
  const router = useRouter();

  const handleToggle = useCallback(async () => {
    if (!workflow) return;

    try {
      await workflowApi.toggle(workflowId);
      toast.success(workflow.is_enabled ? "工作流已停用" : "工作流已启用");
      onSuccess?.();
    } catch (error) {
      console.error("Failed to toggle workflow:", error);
      toast.error("操作失败");
    }
  }, [workflow, workflowId, onSuccess]);

  const handleTrigger = useCallback(async () => {
    if (!workflow) return;

    if (!workflow.is_enabled) {
      toast.warning("请先启用工作流");
      return;
    }

    try {
      await workflowApi.trigger(workflowId);
      toast.success("工作流已开始执行");
      onSuccess?.();
    } catch (error: unknown) {
      console.error("Failed to trigger workflow:", error);
      const errorMessage = error instanceof Error ? error.message : "执行失败";
      toast.error(errorMessage);
    }
  }, [workflow, workflowId, onSuccess]);

  const handleDelete = useCallback(async () => {
    if (!workflow) return;

    if (!confirm(`确定要删除工作流 "${workflow.name}" 吗？此操作不可恢复。`)) {
      return;
    }

    try {
      await workflowApi.delete(workflowId);
      toast.success("工作流已删除");
      router.push("/workflows");
    } catch (error) {
      console.error("Failed to delete workflow:", error);
      toast.error("删除失败");
    }
  }, [workflow, workflowId, router]);

  return {
    handleToggle,
    handleTrigger,
    handleDelete,
  };
}

export default useWorkflowActions;
