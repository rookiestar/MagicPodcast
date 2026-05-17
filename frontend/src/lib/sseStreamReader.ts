import type {
  SSEMessage,
  SSEProgressCallback,
  SSERequestOptions,
} from "./sseClient";

export type NormalizedSSERequestOptions = Required<
  Omit<SSERequestOptions, "headers" | "body" | "isComplete">
> &
  Pick<SSERequestOptions, "headers" | "body" | "isComplete">;

interface SSEReadState {
  buffer: string;
  completed: boolean;
  messageCount: number;
}

interface ProcessLineResult {
  done: boolean;
}

export function normalizeSSEOptions(
  options: SSERequestOptions,
): NormalizedSSERequestOptions {
  const timeout = options.timeout ?? 10 * 60 * 1000;

  return {
    endpoint: options.endpoint,
    method: options.method ?? "POST",
    headers: options.headers ?? {},
    body: options.body ?? null,
    timeout,
    logPrefix: options.logPrefix ?? "[SSE]",
    isComplete: options.isComplete,
    completeOnTypeComplete: options.completeOnTypeComplete ?? true,
    resolveOnStreamErrorAfterMessage:
      options.resolveOnStreamErrorAfterMessage ?? false,
    requireCompletion: options.requireCompletion ?? false,
    emptyMessage: options.emptyMessage ?? "未收到任何消息",
    incompleteMessage:
      options.incompleteMessage ?? "连接提前结束，未收到完成确认",
    abortMessage: options.abortMessage ?? "请求被取消",
    timeoutMessage:
      options.timeoutMessage ?? `请求超时（${timeout / 1000}秒）`,
  };
}

export function createSSEReadState(): SSEReadState {
  return {
    buffer: "",
    completed: false,
    messageCount: 0,
  };
}

function finishStream({
  state,
  startedAt,
  options,
}: {
  state: SSEReadState;
  startedAt: number;
  options: NormalizedSSERequestOptions;
}) {
  const totalTime = Date.now() - startedAt;
  console.log(
    `${options.logPrefix} 流结束，总耗时: ${totalTime}ms，消息数: ${state.messageCount}，completed: ${state.completed}`,
  );

  if (state.completed) return;

  if (state.messageCount > 0 && options.requireCompletion) {
    throw new Error(options.incompleteMessage);
  }

  if (state.messageCount > 0) {
    console.log(`${options.logPrefix} 完成（流正常结束）`);
    return;
  }

  throw new Error(options.emptyMessage);
}

function processDataLine({
  dataContent,
  state,
  options,
  onProgress,
  startedAt,
}: {
  dataContent: string;
  state: SSEReadState;
  options: NormalizedSSERequestOptions;
  onProgress: SSEProgressCallback;
  startedAt: number;
}): ProcessLineResult {
  if (dataContent === "[DONE]") {
    console.log(`${options.logPrefix} 收到 [DONE] 标记，完成`);
    state.completed = true;
    return { done: true };
  }

  try {
    const data: SSEMessage = JSON.parse(dataContent);
    const { type, message, current, total } = data;
    state.messageCount += 1;

    if (state.messageCount <= 10 || state.messageCount % 50 === 0) {
      console.log(
        `${options.logPrefix} Msg #${state.messageCount}: ${type} ${message?.substring(0, 50)} current: ${current} total: ${total}`,
      );
    }

    onProgress(type, message || "", current, total, data);

    if (options.isComplete?.(data)) {
      const totalTime = Date.now() - startedAt;
      console.log(
        `${options.logPrefix} 收到完成消息，总耗时: ${totalTime}ms，总消息数: ${state.messageCount}`,
      );
      state.completed = true;
      return { done: true };
    }

    if (options.completeOnTypeComplete && type === "complete") {
      const totalTime = Date.now() - startedAt;
      console.log(
        `${options.logPrefix} 收到 complete 消息，总耗时: ${totalTime}ms`,
      );
      state.completed = true;
      return { done: true };
    }
  } catch (parseError) {
    console.warn(
      `${options.logPrefix} JSON 解析失败: ${dataContent.substring(0, 100)}`,
      parseError,
    );
  }

  return { done: false };
}

function processSSELine({
  line,
  state,
  options,
  onProgress,
  startedAt,
}: {
  line: string;
  state: SSEReadState;
  options: NormalizedSSERequestOptions;
  onProgress: SSEProgressCallback;
  startedAt: number;
}): ProcessLineResult {
  const trimmedLine = line.trim();

  if (!trimmedLine) return { done: false };

  if (trimmedLine.startsWith(":")) {
    console.log(`${options.logPrefix} 收到 ping: ${trimmedLine}`);
    return { done: false };
  }

  if (!trimmedLine.startsWith("data: ")) return { done: false };

  return processDataLine({
    dataContent: trimmedLine.slice(6).trim(),
    state,
    options,
    onProgress,
    startedAt,
  });
}

function processChunk({
  chunk,
  decoder,
  state,
  options,
  onProgress,
  startedAt,
}: {
  chunk: Uint8Array;
  decoder: TextDecoder;
  state: SSEReadState;
  options: NormalizedSSERequestOptions;
  onProgress: SSEProgressCallback;
  startedAt: number;
}): ProcessLineResult {
  state.buffer += decoder.decode(chunk, { stream: true });

  const lines = state.buffer.split("\n");
  state.buffer = lines.pop() || "";

  for (const line of lines) {
    const result = processSSELine({
      line,
      state,
      options,
      onProgress,
      startedAt,
    });

    if (result.done) return result;
  }

  return { done: false };
}

export async function readSSEStream({
  reader,
  decoder,
  state,
  options,
  onProgress,
  startedAt,
}: {
  reader: ReadableStreamDefaultReader<Uint8Array>;
  decoder: TextDecoder;
  state: SSEReadState;
  options: NormalizedSSERequestOptions;
  onProgress: SSEProgressCallback;
  startedAt: number;
}) {
  while (true) {
    let readResult: ReadableStreamReadResult<Uint8Array>;

    try {
      readResult = await reader.read();
    } catch (error: any) {
      if (error.name === "AbortError") {
        throw new Error(options.abortMessage);
      }

      if (options.resolveOnStreamErrorAfterMessage && state.messageCount > 0) {
        console.log(
          `${options.logPrefix} 读取流出错但已收到 ${state.messageCount} 条消息，视为完成`,
        );
        return;
      }

      console.error(`${options.logPrefix} 读取流出错:`, error);
      throw error;
    }

    if (readResult.done) {
      finishStream({ state, startedAt, options });
      return;
    }

    try {
      const result = processChunk({
        chunk: readResult.value,
        decoder,
        state,
        options,
        onProgress,
        startedAt,
      });

      if (result.done) {
        await reader.cancel();
        return;
      }
    } catch (error) {
      console.error(`${options.logPrefix} 处理消息时出错:`, error);
      if (options.resolveOnStreamErrorAfterMessage && state.messageCount > 0) {
        return;
      }
      throw error;
    }
  }
}
