import type { ApiResponse } from "@/types";
import type {
  EpisodeCopilotContextScope,
  EpisodeCopilotQuestion,
  EpisodeCopilotStreamEvent,
  EpisodePeoplePayload,
} from "@/types/episodeCopilot";
import { apiBaseUrl } from "../apiBaseUrl";
import {
  createSSEReadState,
  normalizeSSEOptions,
  readSSEStream,
} from "../sseStreamReader";
import { api, handleResponse, inlineApiErrorConfig } from "./client";

const episodeCopilotTimeoutMS = 10 * 60 * 1000;

export class EpisodeCopilotCancellationError extends Error {
  constructor() {
    super("回答已取消");
    this.name = "EpisodeCopilotCancellationError";
  }
}

export function isEpisodeCopilotCancellation(error: unknown) {
  return error instanceof EpisodeCopilotCancellationError;
}

function errorMessage(error: unknown) {
  if (error instanceof Error && error.message) return error.message;
  return "单集助手暂时无法回答，请稍后重试";
}

function errorCode(error: unknown) {
  if (
    typeof error === "object" &&
    error !== null &&
    "code" in error &&
    typeof error.code === "string"
  ) {
    return error.code;
  }
  return undefined;
}

export const episodeCopilotApi = {
  preparePeople: async (episodeId: number, signal?: AbortSignal): Promise<EpisodePeoplePayload> => {
    const response = await api.post<ApiResponse<EpisodePeoplePayload>>(
      `/api/v1/episodes/${episodeId}/people/prepare`, {},
      { ...inlineApiErrorConfig, timeout: 180_000, signal },
    );
    return handleResponse(response);
  },
  getPeople: async (episodeId: number): Promise<EpisodePeoplePayload> => {
    const response = await api.get<ApiResponse<EpisodePeoplePayload>>(
      `/api/v1/episodes/${episodeId}/people`,
      inlineApiErrorConfig,
    );
    return handleResponse(response);
  },

  correctName: async (
    episodeId: number,
    personId: number,
    body: {
      display_name: string;
      aliases?: string[];
      identity_note?: string;
    },
  ): Promise<EpisodePeoplePayload> => {
    const response = await api.post<ApiResponse<EpisodePeoplePayload>>(
      `/api/v1/episodes/${episodeId}/people/${personId}/corrections`,
      body,
      inlineApiErrorConfig,
    );
    return handleResponse(response);
  },

  correctAttribution: async (
    episodeId: number,
    body: {
      source_kind: string;
      source_version?: string;
      fragment_order: number;
      assigned_person_id: number | null;
      status: string;
    },
  ): Promise<EpisodePeoplePayload> => {
    const response = await api.post<ApiResponse<EpisodePeoplePayload>>(
      `/api/v1/episodes/${episodeId}/attributions/corrections`,
      body,
      inlineApiErrorConfig,
    );
    return handleResponse(response);
  },

  getContext: async (
    episodeId: number,
  ): Promise<EpisodeCopilotContextScope> => {
    const response = await api.get<ApiResponse<EpisodeCopilotContextScope>>(
      `/api/v1/episodes/${episodeId}/copilot/context`,
      inlineApiErrorConfig,
    );
    return handleResponse(response);
  },

  ask: async (
    episodeId: number,
    question: EpisodeCopilotQuestion,
    onEvent: (event: EpisodeCopilotStreamEvent) => void,
    signal: AbortSignal,
  ): Promise<void> => {
    if (signal.aborted) {
      throw new EpisodeCopilotCancellationError();
    }
    const controller = new AbortController();
    let timedOut = false;
    const abortFromCaller = () => controller.abort();
    signal.addEventListener("abort", abortFromCaller, { once: true });
    const timeout = window.setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, episodeCopilotTimeoutMS);
    let terminalError: EpisodeCopilotStreamEvent | null = null;

    try {
      const response = await fetch(
        `${apiBaseUrl}/api/v1/episodes/${episodeId}/copilot/questions`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(question),
          signal: controller.signal,
        },
      );
      if (!response.ok) {
        const payload = (await response.json().catch(() => null)) as {
          error?: { message?: string; code?: string };
        } | null;
        const failure = new Error(
          payload?.error?.message || `助手请求失败 (${response.status})`,
        ) as Error & { code?: string };
        failure.code = payload?.error?.code;
        throw failure;
      }
      const reader = response.body?.getReader();
      if (!reader) throw new Error("助手响应为空");

      const options = normalizeSSEOptions({
        endpoint: "",
        method: "POST",
        timeout: episodeCopilotTimeoutMS,
        logPrefix: "[EpisodeCopilot]",
        requireCompletion: true,
        completeOnTypeComplete: false,
        isComplete: (data) => data.type === "complete" || data.type === "error",
        emptyMessage: "助手没有返回任何内容",
        incompleteMessage: "助手连接提前结束，请重试",
      });
      await readSSEStream({
        reader,
        decoder: new TextDecoder(),
        state: createSSEReadState(),
        options,
        onProgress: (_type, _message, _current, _total, data) => {
          const event = data as EpisodeCopilotStreamEvent;
          onEvent(event);
          if (event.type === "error") terminalError = event;
        },
        startedAt: Date.now(),
      });
      if (terminalError) {
        const failure = new Error(
          terminalError.message || "助手回答失败",
        ) as Error & { code?: string };
        failure.code = terminalError.code;
        throw failure;
      }
    } catch (error) {
      if (signal.aborted && !timedOut) {
        throw new EpisodeCopilotCancellationError();
      }
      if (timedOut) {
        throw new Error("回答超时，问题和选区已保留，可重试");
      }
      const failure = new Error(errorMessage(error)) as Error & {
        code?: string;
      };
      failure.code = errorCode(error);
      throw failure;
    } finally {
      window.clearTimeout(timeout);
      signal.removeEventListener("abort", abortFromCaller);
    }
  },
};
