// 在浏览器环境中使用相对路径（支持 tunnel/代理访问）
// 在 SSR 环境中使用完整 URL
const API_URL = typeof window !== "undefined"
  ? (process.env.NEXT_PUBLIC_API_URL || "")  // 浏览器：相对路径或自定义 URL
  : (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080");  // SSR：需要完整 URL

export const syncApi = {
  // 导入OPML文件（使用SSE流式响应）
  importOPMLSSE: async (
    file: File,
    onProgress: (
      type: string,
      message: string,
      current?: number,
      total?: number,
    ) => void,
  ): Promise<void> => {
    return new Promise((resolve, reject) => {
      const formData = new FormData();
      formData.append("opml_file", file);

      console.log("[Import] 开始导入，文件:", file.name, "大小:", file.size);

      // 使用AbortController设置超时
      const controller = new AbortController();
      const timeoutId = setTimeout(
        () => {
          console.error("[Import] 导入超时（10分钟）");
          controller.abort();
          reject(new Error("导入超时（10分钟），可能是网络较慢或文件太大"));
        },
        10 * 60 * 1000,
      ); // 10分钟超时

      const startTime = Date.now();
      let messageCount = 0;
      let completed = false; // 标记是否收到complete消息

      // 使用fetch来获取stream
      fetch(`${API_URL}/api/v1/sync/import-sse`, {
        method: "POST",
        body: formData,
        headers: {},
        signal: controller.signal,
      })
        .then((response) => {
          clearTimeout(timeoutId);
          const elapsedTime = Date.now() - startTime;
          console.log(
            "[Import] 收到响应，状态:",
            response.status,
            "耗时:",
            elapsedTime + "ms",
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

          // 读取流
          function readStream() {
            // TypeScript type narrowing workaround: reader is non-null here
            reader!
              .read()
              .then(({ done, value }) => {
                if (done) {
                  const totalTime = Date.now() - startTime;
                  console.log(
                    "[Import] 流结束，总耗时:",
                    totalTime + "ms",
                    "消息数:",
                    messageCount,
                    "completed:",
                    completed,
                  );

                  // 如果收到过消息，即使没有收到complete也认为成功
                  if (messageCount > 0) {
                    console.log("[Import] 导入完成（流正常结束）");
                    resolve();
                  } else {
                    reject(new Error("未收到任何导入消息"));
                  }
                  return;
                }

                try {
                  // 解码数据并追加到缓冲区
                  buffer += decoder.decode(value, { stream: true });

                  // 按行分割，但保留最后一个可能不完整的行
                  const lines = buffer.split("\n");
                  buffer = lines.pop() || ""; // 保留最后一个可能不完整的行

                  for (const line of lines) {
                    const trimmedLine = line.trim();

                    // 跳过空行
                    if (!trimmedLine) {
                      continue;
                    }

                    // 跳过SSE注释（用于keepalive）
                    if (trimmedLine.startsWith(":")) {
                      console.log("[Import] 收到ping:", trimmedLine);
                      continue;
                    }

                    // 处理data消息
                    if (trimmedLine.startsWith("data: ")) {
                      try {
                        const data = JSON.parse(trimmedLine.slice(6));
                        const { type, message, current, total } = data;
                        messageCount++;

                        // 打印每条消息（前10条和每50条）
                        if (messageCount <= 10 || messageCount % 50 === 0) {
                          console.log(
                            `[Import Msg #${messageCount}]`,
                            type,
                            message,
                            "current:",
                            current,
                            "total:",
                            total,
                          );
                        }

                        onProgress(type, message, current, total);

                        // 只在收到真正的complete消息时才结束（不要在每条success消息时结束）
                        if (
                          type === "complete" ||
                          (type === "success" && message.includes("导入完成"))
                        ) {
                          const totalTime = Date.now() - startTime;
                          console.log(
                            "[Import] 收到complete消息，总耗时:",
                            totalTime + "ms",
                            "总消息数:",
                            messageCount,
                          );
                          completed = true;
                          resolve();
                          reader!.cancel();
                          return;
                        }
                      } catch (e) {
                        console.error(
                          "[Import] 解析SSE消息失败:",
                          e,
                          trimmedLine,
                        );
                        // 继续处理下一条消息，不中断整个流程
                      }
                    }
                  }

                  // 继续读取
                  readStream();
                } catch (error) {
                  console.error("[Import] 流处理错误:", error);

                  // 如果已经收到过消息，认为是部分成功
                  if (messageCount > 0) {
                    console.log(
                      "[Import] 流出错但已收到",
                      messageCount,
                      "条消息，视为成功",
                    );
                    resolve();
                  } else {
                    reject(error);
                  }
                }
              })
              .catch((error) => {
                const totalTime = Date.now() - startTime;
                console.error(
                  "[Import] 读取错误:",
                  error,
                  "耗时:",
                  totalTime + "ms",
                  "消息数:",
                  messageCount,
                  "completed:",
                  completed,
                );

                if (error.name === "AbortError") {
                  reject(new Error("导入被取消"));
                } else if (messageCount > 0) {
                  // 如果已经收到过消息，认为是部分成功
                  console.log(
                    "[Import] 连接错误但已收到",
                    messageCount,
                    "条消息，视为成功",
                  );
                  resolve();
                } else {
                  reject(error);
                }
              });
          }

          readStream();
        })
        .catch((error) => {
          clearTimeout(timeoutId);
          const totalTime = Date.now() - startTime;
          console.error(
            "[Import] Fetch错误:",
            error,
            "耗时:",
            totalTime + "ms",
            "消息数:",
            messageCount,
          );

          if (error.name === "AbortError") {
            reject(new Error("导入超时被取消"));
          } else {
            reject(error);
          }
        });
    });
  },

  // 同步所有播客的单集元数据（SSE流式）
  syncPodcastsMetadataSSE: async (
    onProgress: (
      type: string,
      message: string,
      current?: number,
      total?: number,
      data?: any,
    ) => void,
  ): Promise<void> => {
    return new Promise((resolve, reject) => {
      console.log("[Sync Metadata] 开始同步元数据");

      // 使用AbortController设置超时
      const controller = new AbortController();
      const timeoutId = setTimeout(
        () => {
          console.error("[Sync Metadata] 同步超时（10分钟）");
          controller.abort();
          reject(new Error("同步超时（10分钟）"));
        },
        10 * 60 * 1000,
      ); // 10分钟超时

      const startTime = Date.now();
      let messageCount = 0;
      let completed = false;

      // 使用fetch来获取stream
      fetch(`${API_URL}/api/v1/sync/podcasts/metadata-sse`, {
        method: "POST",
        headers: {},
        signal: controller.signal,
      })
        .then((response) => {
          clearTimeout(timeoutId);
          const elapsedTime = Date.now() - startTime;
          console.log(
            "[Sync Metadata] 收到响应，状态:",
            response.status,
            "耗时:",
            elapsedTime + "ms",
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

          // 读取流
          function readStream() {
            reader!
              .read()
              .then(({ done, value }) => {
                if (done) {
                  const totalTime = Date.now() - startTime;
                  console.log(
                    "[Sync Metadata] 流结束，总耗时:",
                    totalTime + "ms",
                    "消息数:",
                    messageCount,
                    "completed:",
                    completed,
                  );

                  if (messageCount > 0) {
                    console.log("[Sync Metadata] 同步完成（流正常结束）");
                    resolve();
                  } else {
                    reject(new Error("未收到任何同步消息"));
                  }
                  return;
                }

                try {
                  // 解码数据并追加到缓冲区
                  buffer += decoder.decode(value, { stream: true });

                  // 按行分割，但保留最后一个可能不完整的行
                  const lines = buffer.split("\n");
                  buffer = lines.pop() || ""; // 保留最后一个可能不完整的行

                  for (const line of lines) {
                    const trimmedLine = line.trim();

                    // 跳过空行
                    if (!trimmedLine) {
                      continue;
                    }

                    // 跳过SSE注释（用于keepalive）
                    if (trimmedLine.startsWith(":")) {
                      continue;
                    }

                    // 处理data消息
                    if (trimmedLine.startsWith("data: ")) {
                      const dataContent = trimmedLine.slice(6).trim();

                      // 检查是否是结束标记
                      if (dataContent === "[DONE]") {
                        console.log("[Sync Metadata] 收到[DONE]标记，同步完成");
                        const totalTime = Date.now() - startTime;
                        console.log(
                          "[Sync Metadata] 总耗时:",
                          totalTime + "ms",
                          "总消息数:",
                          messageCount,
                        );
                        completed = true;
                        resolve();
                        reader!.cancel();
                        return;
                      }

                      // 处理JSON消息
                      try {
                        const data = JSON.parse(dataContent);
                        const { type, message, current, total } = data;
                        messageCount++;

                        console.log(
                          "[Sync Metadata] 收到消息:",
                          type,
                          message?.substring(0, 50),
                        );

                        // 对summary消息，创建新对象传递（避免可能的引用问题）
                        const dataToPass =
                          type === "summary"
                            ? {
                                total_podcasts: data.total_podcasts,
                                success_podcasts: data.success_podcasts,
                                failed_podcasts: data.failed_podcasts,
                                skipped_podcasts: data.skipped_podcasts,
                                no_update_podcasts: data.no_update_podcasts,
                                total_episodes: data.total_episodes,
                                new_episodes: data.new_episodes,
                                updated_episodes: data.updated_episodes,
                                duration: data.duration,
                              }
                            : data;

                        onProgress(type, message, current, total, dataToPass);

                        // 只在收到complete消息时准备结束，等待[DONE]标记
                        if (
                          type === "success" &&
                          message &&
                          message.includes("同步完成")
                        ) {
                          console.log(
                            "[Sync Metadata] 收到完成消息，等待[DONE]标记...",
                          );
                          completed = true;
                        }
                      } catch (e) {
                        console.error(
                          "[Sync Metadata] 解析SSE消息失败:",
                          e,
                          trimmedLine,
                        );
                      }
                    }
                  }

                  // 继续读取
                  readStream();
                } catch (error) {
                  console.error("[Sync Metadata] 流处理错误:", error);

                  if (messageCount > 0) {
                    console.log(
                      "[Sync Metadata] 流出错但已收到",
                      messageCount,
                      "条消息，视为成功",
                    );
                    resolve();
                  } else {
                    reject(error);
                  }
                }
              })
              .catch((error) => {
                const totalTime = Date.now() - startTime;
                console.error(
                  "[Sync Metadata] 读取错误:",
                  error,
                  "耗时:",
                  totalTime + "ms",
                  "消息数:",
                  messageCount,
                );

                if (error.name === "AbortError") {
                  reject(new Error("同步被取消"));
                } else if (messageCount > 0) {
                  console.log(
                    "[Sync Metadata] 连接错误但已收到",
                    messageCount,
                    "条消息，视为成功",
                  );
                  resolve();
                } else {
                  reject(error);
                }
              });
          }

          readStream();
        })
        .catch((error) => {
          clearTimeout(timeoutId);
          const totalTime = Date.now() - startTime;
          console.error(
            "[Sync Metadata] Fetch错误:",
            error,
            "耗时:",
            totalTime + "ms",
            "消息数:",
            messageCount,
          );

          if (error.name === "AbortError") {
            reject(new Error("同步超时被取消"));
          } else {
            reject(error);
          }
        });
    });
  },
};
