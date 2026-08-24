import { api, handleResponse } from "./client";
import type { ApiResponse } from "@/types";
import type {
  ArtifactContent,
  EpisodeAudioAsset,
  ProcessingErrorDetails,
  ProcessingRun,
  ProcessingRunDetail,
  ProcessingStartResult,
} from "@/types/processing";

interface AxiosLikeError {
  message?: string;
  response?: {
    status?: number;
    data?: {
      error?: {
        code?: string;
        message?: string;
      };
    };
  };
}

export function getProcessingErrorDetails(
  error: unknown,
): ProcessingErrorDetails {
  const candidate =
    error && typeof error === "object" ? (error as AxiosLikeError) : undefined;
  return {
    code: candidate?.response?.data?.error?.code,
    message:
      candidate?.response?.data?.error?.message ||
      candidate?.message ||
      "加工请求失败，请稍后重试",
    status: candidate?.response?.status,
  };
}

export const processingApi = {
  listEpisodeRuns: async (episodeId: number): Promise<ProcessingRun[]> => {
    const response = await api.get<ApiResponse<ProcessingRun[]>>(
      `/api/v1/episodes/${episodeId}/processing-runs`,
    );
    return handleResponse(response);
  },

  start: async (
    episodeId: number,
    force = false,
  ): Promise<ProcessingStartResult> => {
    const response = await api.post<ApiResponse<ProcessingStartResult>>(
      `/api/v1/episodes/${episodeId}/processing-runs`,
      { force },
    );
    return handleResponse(response);
  },

  getRun: async (runId: number): Promise<ProcessingRunDetail> => {
    const response = await api.get<ApiResponse<ProcessingRunDetail>>(
      `/api/v1/processing-runs/${runId}`,
    );
    return handleResponse(response);
  },

  getLatestAudio: async (episodeId: number): Promise<EpisodeAudioAsset> => {
    const response = await api.get<ApiResponse<EpisodeAudioAsset>>(
      `/api/v1/episodes/${episodeId}/audio-assets/latest`,
    );
    return handleResponse(response);
  },

  cancel: async (runId: number): Promise<ProcessingRun> => {
    const response = await api.post<ApiResponse<ProcessingRun>>(
      `/api/v1/processing-runs/${runId}/cancel`,
    );
    return handleResponse(response);
  },

  retry: async (runId: number): Promise<ProcessingStartResult> => {
    const response = await api.post<ApiResponse<ProcessingStartResult>>(
      `/api/v1/processing-runs/${runId}/retry`,
    );
    return handleResponse(response);
  },

  getArtifactContent: async (
    artifactSetId: number,
    kind: "transcript" | "episode_notes",
  ): Promise<ArtifactContent> => {
    const response = await api.get<ApiResponse<ArtifactContent>>(
      `/api/v1/artifact-sets/${artifactSetId}/${kind}`,
    );
    return handleResponse(response);
  },
};
