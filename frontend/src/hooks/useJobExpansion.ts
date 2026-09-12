import { useState, useCallback, useEffect } from "react";
import { workflowApi } from "@/lib/api";
import type { Job } from "@/types";

/** Job identity is independent of the loaded history page. */
export function useJobExpansion(workflowId?: number, controlledId?: number | null, onSelect?: (id: number | null) => void) {
  const [localId, setLocalId] = useState<number | null>(null);
  const selectedJobId = controlledId === undefined ? localId : controlledId;
  const [jobDetails, setJobDetails] = useState<Record<number, Job>>({});
  const [loadingJobId, setLoadingJobId] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    if (!selectedJobId) { setLoadingJobId(null); setError(null); return; }
    if (jobDetails[selectedJobId] && (workflowId === undefined || jobDetails[selectedJobId].workflow_id === workflowId)) { setError(null); setLoadingJobId(null); return; }
    let active = true;
    setLoadingJobId(selectedJobId); setError(null);
    workflowApi.getJob(selectedJobId).then((job) => {
      if (!active) return;
      if (workflowId !== undefined && job.workflow_id !== workflowId) { setError("该执行不属于当前工作流。"); return; }
      setJobDetails((previous) => ({...previous,[job.id]:job}));
    }).catch((error: {response?:{status?:number}}) => { if (active) setError(error.response?.status === 404 ? "执行记录不存在。" : "执行记录读取失败，请重试。"); }).finally(() => { if (active) setLoadingJobId(null); });
    return () => { active=false; };
  }, [selectedJobId, workflowId, jobDetails, retry]);
  const fetchJobDetail = useCallback(async (id: number) => {
    const next = selectedJobId === id ? null : id;
    if (onSelect) onSelect(next); else setLocalId(next);
  }, [selectedJobId, onSelect]);
  return {selectedJobId,jobDetails,loadingJobId,fetchJobDetail,error,retryRead:()=>{if(selectedJobId)setJobDetails((previous)=>{const next={...previous};delete next[selectedJobId];return next;});setRetry((value)=>value+1);},getJobDetail:(id:number)=>jobDetails[id]};
}
