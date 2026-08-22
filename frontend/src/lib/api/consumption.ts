import { api, handleResponse } from "./client";
import { mutate } from "swr";
import type { ApiResponse } from "@/types";
import type {
  ConsumptionErrorDetails,
  ConsumptionItem,
  ConsumptionQueue,
  ConsumptionQueuePlacementRequest,
  ConsumptionQueuePlacementResult,
  ConsumptionQueuePayload,
  ConsumptionSummary,
} from "@/types/consumption";

interface ConsumptionApiErrorPayload {
  success?: boolean;
  error?: {
    code?: string;
    message?: string;
    current_count?: number;
    focus_limit?: number;
  };
}

interface AxiosLikeError {
  message?: string;
  response?: {
    status?: number;
    data?: ConsumptionApiErrorPayload;
  };
}

export const FOCUS_CONFIRMATION_ERROR = "FOCUS_LIMIT_CONFIRMATION_REQUIRED";
export const QUEUE_ORDER_CONFLICT = "QUEUE_ORDER_CONFLICT";
export const CONSUMPTION_SUMMARY_KEY = "/api/v1/consumption/summary";

export function revalidateConsumptionSummary() {
  return mutate(CONSUMPTION_SUMMARY_KEY);
}

export function getConsumptionErrorDetails(
  error: unknown,
): ConsumptionErrorDetails {
  const candidate =
    error && typeof error === "object" ? (error as AxiosLikeError) : undefined;
  const payload = candidate?.response?.data?.error;

  return {
    code: payload?.code,
    message:
      payload?.message ||
      candidate?.message ||
      (typeof error === "string" ? error : "请求失败，请稍后重试"),
    status: candidate?.response?.status,
    currentCount: payload?.current_count,
    focusLimit: payload?.focus_limit,
  };
}

export function requiresFocusConfirmation(error: unknown) {
  return getConsumptionErrorDetails(error).code === FOCUS_CONFIRMATION_ERROR;
}

export function isQueueOrderConflict(error: unknown) {
  return getConsumptionErrorDetails(error).code === QUEUE_ORDER_CONFLICT;
}

export const consumptionApi = {
  getSummary: async (): Promise<ConsumptionSummary> => {
    const response = await api.get<ApiResponse<ConsumptionSummary>>(
      CONSUMPTION_SUMMARY_KEY,
    );
    return handleResponse(response);
  },

  listQueue: async (
    queue: ConsumptionQueue,
  ): Promise<ConsumptionQueuePayload> => {
    const response = await api.get<ApiResponse<ConsumptionQueuePayload>>(
      `/api/v1/consumption/queues/${queue}`,
    );
    return handleResponse(response);
  },

  getItem: async (episodeId: number): Promise<ConsumptionItem> => {
    const response = await api.get<ApiResponse<ConsumptionItem>>(
      `/api/v1/consumption/episodes/${episodeId}`,
    );
    return handleResponse(response);
  },

  setQueue: async (
    episodeId: number,
    queueState: ConsumptionQueue,
    options: { acknowledgeFocusLimit?: boolean } = {},
  ): Promise<ConsumptionItem> => {
    const response = await api.put<ApiResponse<ConsumptionItem>>(
      `/api/v1/consumption/episodes/${episodeId}/queue`,
      {
        queue_state: queueState,
        acknowledge_focus_limit: options.acknowledgeFocusLimit ?? false,
      },
    );
    const item = handleResponse(response);
    void revalidateConsumptionSummary();
    return item;
  },

  placeQueue: async (
    episodeId: number,
    request: ConsumptionQueuePlacementRequest,
  ): Promise<ConsumptionQueuePlacementResult> => {
    const response = await api.put<ApiResponse<ConsumptionQueuePlacementResult>>(
      `/api/v1/consumption/episodes/${episodeId}/placement`,
      request,
    );
    const result = handleResponse(response);
    void revalidateConsumptionSummary();
    return result;
  },

  clearQueue: async (episodeId: number): Promise<ConsumptionItem> => {
    const response = await api.delete<ApiResponse<ConsumptionItem>>(
      `/api/v1/consumption/episodes/${episodeId}/queue`,
    );
    const item = handleResponse(response);
    void revalidateConsumptionSummary();
    return item;
  },

  markRead: async (episodeId: number): Promise<ConsumptionItem> => {
    const response = await api.post<ApiResponse<ConsumptionItem>>(
      `/api/v1/consumption/episodes/${episodeId}/read`,
    );
    return handleResponse(response);
  },

  markInProgress: async (episodeId: number): Promise<ConsumptionItem> => {
    const response = await api.post<ApiResponse<ConsumptionItem>>(
      `/api/v1/consumption/episodes/${episodeId}/in-progress`,
    );
    return handleResponse(response);
  },
};
