/**
 * SSE (Server-Sent Events) 客户端工具
 * 提供通用的 SSE 流式请求处理
 */

// 在浏览器环境中使用相对路径（支持 tunnel/代理访问）
// 在 SSR 环境中使用完整 URL
const API_URL =
  typeof window !== "undefined"
    ? process.env.NEXT_PUBLIC_API_URL || "" // 浏览器：相对路径或自定义 URL
    : process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"; // SSR：需要完整 URL

/**
 * SSE 消息类型
 */
export interface SSEMessage {
  type: string;
  message: string;
  current?: number;
  total?: number;
  [key: string]: any; // 允许额外的字段
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

/**
 * 通用 SSE 流式请求
 */
export async function sseRequest(
  options: SSERequestOptions,
  onProgress: SSEProgressCallback,
): Promise<void> {
  const {
    endpoint,
    method = "POST",
    headers = {},
    body = null,
    timeout = 10 * 60 * 1000, // 10分钟
    logPrefix = "[SSE]",
    isComplete,
    completeOnTypeComplete = true,
    resolveOnStreamErrorAfterMessage = false,
    requireCompletion = false,
    emptyMessage = "未收到任何消息",
    incompleteMessage = "连接提前结束，未收到完成确认",
    abortMessage = "请求被取消",
    timeoutMessage = `请求超时（${timeout / 1000}秒）`,
  } = options;

  return new Promise((resolve, reject) => {
    console.log(`${logPrefix} 开始请求: ${endpoint}`);

    // 使用 AbortController 设置超时
    const controller = new AbortController();
    const timeoutId = setTimeout(() => {
      console.error(`${logPrefix} 请求超时（${timeout / 1000}秒）`);
      controller.abort();
      reject(new Error(timeoutMessage));
    }, timeout);

    const startTime = Date.now();
    let messageCount = 0;
    let completed = false;

    fetch(`${API_URL}${endpoint}`, {
      method,
      headers,
      body,
      signal: controller.signal,
    })
      .then((response) => {
        clearTimeout(timeoutId);
        const elapsedTime = Date.now() - startTime;
        console.log(
          `${logPrefix} 收到响应，状态: ${response.status}，耗时: ${elapsedTime}ms`,
        );

        if (!response.ok) {
          throw new Error(`HTTP error! status: ${response.status}`);
        }

        const reader = response.body?.getReader();
        const decoder = new TextDecoder();

        if (!reader) {
          throw new Error("Response body is null");
        }

        let buffer = ""; // 缓冲区，用于处理被截断的消息

        function readStream() {
          reader!
            .read()
            .then(({ done, value }) => {
              if (done) {
                const totalTime = Date.now() - startTime;
                console.log(
                  `${logPrefix} 流结束，总耗时: ${totalTime}ms，消息数: ${messageCount}，completed: ${completed}`,
                );

                if (completed) {
                  resolve();
                } else if (messageCount > 0 && requireCompletion) {
                  reject(new Error(incompleteMessage));
                } else if (messageCount > 0) {
                  console.log(`${logPrefix} 完成（流正常结束）`);
                  resolve();
                } else {
                  reject(new Error(emptyMessage));
                }
                return;
              }

              try {
                // 解码数据并追加到缓冲区
                buffer += decoder.decode(value, { stream: true });

                // 按行分割，但保留最后一个可能不完整的行
                const lines = buffer.split("\n");
                buffer = lines.pop() || "";

                for (const line of lines) {
                  const trimmedLine = line.trim();

                  // 跳过空行
                  if (!trimmedLine) {
                    continue;
                  }

                  // 跳过 SSE 注释（用于 keepalive）
                  if (trimmedLine.startsWith(":")) {
                    console.log(`${logPrefix} 收到 ping: ${trimmedLine}`);
                    continue;
                  }

                  // 处理 data 消息
                  if (trimmedLine.startsWith("data: ")) {
                    const dataContent = trimmedLine.slice(6).trim();

                    // 检查是否是结束标记
                    if (dataContent === "[DONE]") {
                      console.log(`${logPrefix} 收到 [DONE] 标记，完成`);
                      completed = true;
                      resolve();
                      reader!.cancel();
                      return;
                    }

                    // 处理 JSON 消息
                    try {
                      const data: SSEMessage = JSON.parse(dataContent);
                      const { type, message, current, total } = data;
                      messageCount++;

                      // 打印每条消息（前 10 条和每 50 条）
                      if (messageCount <= 10 || messageCount % 50 === 0) {
                        console.log(
                          `${logPrefix} Msg #${messageCount}: ${type} ${message?.substring(0, 50)} current: ${current} total: ${total}`,
                        );
                      }

                      onProgress(type, message || "", current, total, data);

                      // 检查自定义完成条件
                      if (isComplete?.(data)) {
                        const totalTime = Date.now() - startTime;
                        console.log(
                          `${logPrefix} 收到完成消息，总耗时: ${totalTime}ms，总消息数: ${messageCount}`,
                        );
                        completed = true;
                        resolve();
                        reader!.cancel();
                        return;
                      }

                      // 默认完成条件
                      if (completeOnTypeComplete && type === "complete") {
                        const totalTime = Date.now() - startTime;
                        console.log(
                          `${logPrefix} 收到 complete 消息，总耗时: ${totalTime}ms`,
                        );
                        completed = true;
                        resolve();
                        reader!.cancel();
                        return;
                      }
                    } catch (parseError) {
                      console.warn(
                        `${logPrefix} JSON 解析失败: ${dataContent.substring(0, 100)}`,
                        parseError,
                      );
                    }
                  }
                }

                // 继续读取
                readStream();
              } catch (error) {
                console.error(`${logPrefix} 处理消息时出错:`, error);
                if (resolveOnStreamErrorAfterMessage && messageCount > 0) {
                  resolve();
                } else {
                  reject(error);
                }
              }
            })
            .catch((error) => {
              if (error.name === "AbortError") {
                reject(new Error(abortMessage));
              } else if (
                resolveOnStreamErrorAfterMessage &&
                messageCount > 0
              ) {
                console.log(
                  `${logPrefix} 读取流出错但已收到 ${messageCount} 条消息，视为完成`,
                );
                resolve();
              } else {
                console.error(`${logPrefix} 读取流出错:`, error);
                reject(error);
              }
            });
        }

        readStream();
      })
      .catch((error) => {
        clearTimeout(timeoutId);
        if (error.name === "AbortError") {
          reject(new Error("请求超时被取消"));
        } else {
          console.error(`${logPrefix} 请求失败:`, error);
          reject(error);
        }
      });
  });
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
