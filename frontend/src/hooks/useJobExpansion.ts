import { useState, useCallback } from "react";
import { workflowApi } from "@/lib/api";
import type { Job } from "@/types";

interface UseJobExpansionReturn {
  selectedJobId: number | null;
  jobDetails: Record<number, Job>;
  loadingJobId: number | null;
  fetchJobDetail: (jobId: number) => Promise<void>;
  getJobDetail: (jobId: number) => Job | undefined;
}

export function useJobExpansion(): UseJobExpansionReturn {
  const [selectedJobId, setSelectedJobId] = useState<number | null>(null);
  const [jobDetails, setJobDetails] = useState<Record<number, Job>>({});
  const [loadingJobId, setLoadingJobId] = useState<number | null>(null);

  const fetchJobDetail = useCallback(async (jobId: number) => {
    // 如果已经缓存，直接切换展开状态
    if (jobDetails[jobId]) {
      setSelectedJobId(selectedJobId === jobId ? null : jobId);
      return;
    }

    // 加载详情
    setLoadingJobId(jobId);
    try {
      const detail = await workflowApi.getJob(jobId);
      setJobDetails((prev) => ({ ...prev, [jobId]: detail }));
      setSelectedJobId(selectedJobId === jobId ? null : jobId);
    } catch (error) {
      console.error("Failed to fetch job detail:", error);
    } finally {
      setLoadingJobId(null);
    }
  }, [jobDetails, selectedJobId]);

  const getJobDetail = useCallback((jobId: number) => {
    return jobDetails[jobId];
  }, [jobDetails]);

  return {
    selectedJobId,
    jobDetails,
    loadingJobId,
    fetchJobDetail,
    getJobDetail,
  };
}
