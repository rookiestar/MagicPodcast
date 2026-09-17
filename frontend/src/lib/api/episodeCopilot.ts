import type { ApiResponse } from "@/types";
import type {
  EpisodeCopilotContextScope,
  EpisodeCopilotQuestion,
  EpisodeCopilotStreamEvent,
  EpisodePeoplePayload,
 PersonReviewDraft,
 PersonReviewMatch,
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

export type PersonRuntimePhase = "ready" | "active" | "generating" | "reconnecting";

export type PersonPreparationEvent = {
  type: "stage" | "heartbeat" | "runtime" | "complete" | "error";
  episode_id: number;
  request_id: string;
  source_version?: string;
  stage?: "read" | "identify" | "save";
  runtime?: { phase: PersonRuntimePhase; will_retry?: boolean; classification?: string; last_activity_age_ms?: number };
  elapsed_ms?: number;
  message?: string;
  code?: string;
  classification?: string;
  retryable?: boolean;
  data?: EpisodePeoplePayload;
};

// Structured preparation failure surfaced from the SSE terminal error, the
// streaming proxy, or a lost completion. The component keeps the specific
// reason and the short request id instead of a generic "connection lost".
export class PersonPreparationFailure extends Error {
  code: string;
  classification: string;
  retryable: boolean;
  requestId: string | undefined;

  constructor(init: { message: string; code?: string; classification?: string; retryable?: boolean; requestId?: string }) {
    super(init.message);
    this.name = "PersonPreparationFailure";
    this.code = init.code ?? "PERSON_PREPARATION_FAILED";
    this.classification = init.classification ?? "unknown";
    this.retryable = init.retryable ?? true;
    this.requestId = init.requestId;
  }
}

export const episodeCopilotApi = {
 reviewPeople: async (episodeId: number, body: {draft_id: number; revision: number; source_version: string; matches: PersonReviewMatch[]}, apply: boolean): Promise<EpisodePeoplePayload> =>
  handleResponse(await api.post<ApiResponse<EpisodePeoplePayload>>(`/api/v1/episodes/${episodeId}/people/${apply ? "apply" : "draft"}`, body, inlineApiErrorConfig)),
 peopleDrafts: async (episodeId: number): Promise<PersonReviewDraft[]> =>
  handleResponse(await api.get<ApiResponse<PersonReviewDraft[]>>(`/api/v1/episodes/${episodeId}/people/drafts`, inlineApiErrorConfig)),
 manualPerson: async (episodeId: number, body: {revision: number; source_version: string; fragment_order: number; scope: string; person_id: number; display_name: string; clear: boolean}): Promise<EpisodePeoplePayload> =>
  handleResponse(await api.post<ApiResponse<EpisodePeoplePayload>>(`/api/v1/episodes/${episodeId}/people/manual`, body, inlineApiErrorConfig)),

  preparePeople: async (episodeId: number, signal?: AbortSignal, onProgress?: (event: PersonPreparationEvent) => void): Promise<EpisodePeoplePayload> => {
    if (signal?.aborted) throw new DOMException("aborted", "AbortError");
    const controller = new AbortController();
    const abort = () => controller.abort();
    signal?.addEventListener("abort", abort, { once: true });
    if (signal?.aborted) controller.abort();
    let timedOut = false;
    const timer = setTimeout(() => { timedOut = true; abort(); }, 180_000);
    let result: EpisodePeoplePayload | undefined;
    let failure: PersonPreparationFailure | undefined;
    let requestID: string | undefined;
    let sourceVersion: string | undefined;
    let terminal = false;
    const failureFrom = (message: string, extra?: Partial<ConstructorParameters<typeof PersonPreparationFailure>[0]>) =>
      new PersonPreparationFailure({ message, requestId: requestID, ...extra });
    try {
      let response: Response;
      try {
        response = await fetch(`${apiBaseUrl}/api/v1/episodes/${episodeId}/people/prepare`, {
          method: "POST", headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
          body: "{}", signal: controller.signal,
        });
      } catch (fetchError) {
        if (controller.signal.aborted) throw fetchError;
        // A network-level failure never carries a confirmed execution result.
        throw failureFrom("识别服务暂时不可用，请核对已保存结果。", {
          code: "PERSON_PREPARATION_UNAVAILABLE", classification: "connection_interrupted",
        });
      }
      if (!response.ok) {
        const auth = response.status === 401 || response.status === 403;
        const quota = response.status === 429;
        throw failureFrom(auth ? "识别服务认证失效，请检查账号授权。" : quota ? "识别服务额度或请求频率受限，请稍后再试。" : "识别服务暂时不可用，请核对已保存结果。", {
          code: "PERSON_PREPARATION_UNAVAILABLE", classification: auth ? "authentication_failed" : quota ? "quota_exceeded" : "connection_interrupted", retryable: !auth,
        });
      }
      const reader = response.body?.getReader();
      if (!reader) throw failureFrom("识别响应为空，请核对已保存结果。", { code: "PERSON_PREPARATION_UNAVAILABLE" });
      try {
        await readSSEStream({ reader, decoder: new TextDecoder(), state: createSSEReadState(), startedAt: Date.now(),
          options: normalizeSSEOptions({ endpoint: "", requireCompletion: true, completeOnTypeComplete: false,
            isComplete: () => terminal,
            incompleteMessage: "识别连接中断，请核对已保存结果。" }),
          onProgress: (_type, _message, _current, _total, data) => {
            const event = data as PersonPreparationEvent;
            if (controller.signal.aborted || event.episode_id !== episodeId || !event.request_id) return;
            if (requestID && event.request_id !== requestID) return;
            if (sourceVersion && event.source_version && event.source_version !== sourceVersion) return;
            requestID = event.request_id;
            if (event.source_version) sourceVersion = event.source_version;
            if (event.type === "complete") {
              if (!event.data || event.data.episode_id !== episodeId ||
                  event.data.source_version !== sourceVersion || !event.data.draft ||
                  event.data.draft.source_version !== sourceVersion) return;
              result = event.data;
              terminal = true;
            }
            if (event.type === "error") {
              failure = failureFrom(event.message || "识别未完成，请核对已保存结果后重试。", {
                code: event.code, classification: event.classification, retryable: event.retryable,
              });
              terminal = true;
            }
            onProgress?.(event);
          },
        });
      } catch (streamError) {
        if (controller.signal.aborted) throw streamError;
        // The stream itself broke before any terminal event: a transport-level
        // interruption, not a confirmed execution result.
        throw failure instanceof PersonPreparationFailure ? failure
          : failureFrom("识别连接中断，请核对已保存结果。", { classification: "connection_interrupted" });
      } finally { await reader.cancel().catch(() => {}); }
      if (failure) throw failure;
      if (!result) throw failureFrom("尚未收到草稿保存确认，请重新读取。", { classification: "completion_lost" });
      return result;
    } catch (error) {
      if (timedOut && !signal?.aborted) throw failureFrom("识别等待超时，请核对已保存结果后重试。", { code: "PERSON_PREPARATION_TIMEOUT", classification: "deadline" });
      throw error;
    } finally {
      clearTimeout(timer);
      signal?.removeEventListener("abort", abort);
    }
  },
  getPeople: async (episodeId: number, signal?: AbortSignal): Promise<EpisodePeoplePayload> => {
    const response = await api.get<ApiResponse<EpisodePeoplePayload>>(
      `/api/v1/episodes/${episodeId}/people`,
      { ...inlineApiErrorConfig, signal },
    );
    return handleResponse(response);
  },

  correctAppearance: async (episodeId: number, personId: number, body: { role?: string; excluded?: boolean }, signal?: AbortSignal): Promise<EpisodePeoplePayload> => {
    const response = await api.post<ApiResponse<EpisodePeoplePayload>>(
      `/api/v1/episodes/${episodeId}/people/${personId}/appearance-corrections`, body, { ...inlineApiErrorConfig, signal },
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
