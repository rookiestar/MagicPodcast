/**
 * SSE (Server-Sent Events) 客户端工具
 * 提供通用的 SSE 流式请求处理
 */

import {
  createSSEReadState,
  normalizeSSEOptions,
  readSSEStream,
} from "./sseStreamReader";

// 在浏览器环境中使用相对路径（支持 tunnel/代理访问）
// 在 SSR 环境中使用完整 URL
const API_URL =
  typeof window !== "undefined"
    ? process.env.NEXT_PUBLIC_API_URL || ""
    : process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

/**
 * SSE 消息类型
 */
export interface SSEMessage {
  type: string;
  message: string;
  current?: number;
  total?: number;
  [key: string]: any;
}

/**
 * SSE 进度回调
 */
export type SSEProgressCallback = (
  type: string,
  message: string,
  current?: number,
  total?: number,
  data?: any,
) => void;

/**
 * SSE 请求配置
 */
export interface SSERequestOptions {
  /** 请求路径（不包含 API_URL） */
  endpoint: string;
  /** HTTP 方法 */
  method?: "GET" | "POST";
  /** 请求头 */
  headers?: Record<string, string>;
  /** 请求体（POST 时使用） */
  body?: BodyInit | null;
  /** 超时时间（毫秒），默认 10 分钟 */
  timeout?: number;
  /** 日志前缀 */
  logPrefix?: string;
  /** 自定义完成条件检查 */
  isComplete?: (data: SSEMessage) => boolean;
  /** 是否把 type=complete 作为结束信号，默认开启 */
  completeOnTypeComplete?: boolean;
  /** 流中断但已收到消息时是否视为结束，用于兼容旧同步行为 */
  resolveOnStreamErrorAfterMessage?: boolean;
  /** 是否必须收到明确完成信号才算成功 */
  requireCompletion?: boolean;
  /** 空响应错误文案 */
  emptyMessage?: string;
  /** 已收到消息但没有完成信号时的错误文案 */
  incompleteMessage?: string;
  /** 主动取消错误文案 */
  abortMessage?: string;
  /** 超时错误文案 */
  timeoutMessage?: string;
}

function createResponseTimeout({
  controller,
  timeout,
  logPrefix,
  onTimeout,
}: {
  controller: AbortController;
  timeout: number;
  logPrefix: string;
  onTimeout: () => void;
}) {
  return setTimeout(() => {
    onTimeout();
    console.error(`${logPrefix} 请求超时（${timeout / 1000}秒）`);
    controller.abort();
  }, timeout);
}

/**
 * 通用 SSE 流式请求
 */
export async function sseRequest(
  requestOptions: SSERequestOptions,
  onProgress: SSEProgressCallback,
): Promise<void> {
  const options = normalizeSSEOptions(requestOptions);
  const { endpoint, method, headers, body, timeout, logPrefix } = options;

  console.log(`${logPrefix} 开始请求: ${endpoint}`);

  const controller = new AbortController();
  let responseTimedOut = false;
  const timeoutId = createResponseTimeout({
    controller,
    timeout,
    logPrefix,
    onTimeout: () => {
      responseTimedOut = true;
    },
  });
  const startedAt = Date.now();

  try {
    const response = await fetch(`${API_URL}${endpoint}`, {
      method,
      headers,
      body,
      signal: controller.signal,
    });

    clearTimeout(timeoutId);

    const elapsedTime = Date.now() - startedAt;
    console.log(
      `${logPrefix} 收到响应，状态: ${response.status}，耗时: ${elapsedTime}ms`,
    );

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const reader = response.body?.getReader();
    if (!reader) {
      throw new Error("Response body is null");
    }

    await readSSEStream({
      reader,
      decoder: new TextDecoder(),
      state: createSSEReadState(),
      options,
      onProgress,
      startedAt,
    });
  } catch (error: any) {
    clearTimeout(timeoutId);

    if (error.name === "AbortError") {
      throw new Error(
        responseTimedOut ? options.timeoutMessage : "请求超时被取消",
      );
    }

    console.error(`${logPrefix} 请求失败:`, error);
    throw error;
  }
}

/**
 * 创建 FormData SSE 请求（用于文件上传）
 */
export async function sseFormDataRequest(
  endpoint: string,
  formData: FormData,
  onProgress: SSEProgressCallback,
  options?: Omit<SSERequestOptions, "endpoint" | "body" | "headers">,
): Promise<void> {
  return sseRequest(
    {
      endpoint,
      method: "POST",
      body: formData,
      ...options,
    },
    onProgress,
  );
}
