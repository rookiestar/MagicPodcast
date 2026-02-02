import axios from "axios";
import type {
  ApiResponse,
  Podcast,
  Tag,
  Episode,
  SearchData,
  Workflow,
  WorkflowRequest,
  WorkflowsResponse,
  JobsResponse,
  Job,
} from "@/types";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

// 调试：输出API_URL
if (typeof window !== "undefined") {
  console.log("🔧 API_URL:", API_URL);
  console.log("🔧 Process env:", process.env.NEXT_PUBLIC_API_URL);
}

// 创建 axios 实例
const api = axios.create({
  baseURL: API_URL,
  timeout: 60000, // 增加到60秒，支持分页加载大量数据
  headers: {
    "Content-Type": "application/json",
  },
  withCredentials: false, // 不发送凭证，避免CORS问题
});

// 添加请求拦截器
api.interceptors.request.use(
  (config) => {
    console.log(`[API] ${config.method?.toUpperCase()} ${config.url}`);
    return config;
  },
  (error) => {
    console.error("[API] Request error:", error);
    return Promise.reject(error);
  },
);

// 添加响应拦截器
api.interceptors.response.use(
  (response) => {
    console.log(`[API] Response:`, response.status, response.config.url);
    return response;
  },
  (error) => {
    console.error("[API] Response error:", error.message, error.config?.url);

    if (error.code === "ECONNABORTED") {
      console.error("[API] Request timeout");
    } else if (error.response) {
      console.error(
        "[API] Server responded with:",
        error.response.status,
        error.response.data,
      );
    } else if (error.request) {
      console.error("[API] No response received:", error.request);
    }

    return Promise.reject(error);
  },
);

// API 方法
export const podcastApi = {
  // 获取播客列表
  list: async (params?: {
    tag_id?: number | number[];
    page?: number;
    page_size?: number;
    sort_by?: string;
    search?: string;
  }): Promise<{
    data: Podcast[];
    pagination: {
      page: number;
      page_size: number;
      total: number;
      total_pages: number;
    };
  }> => {
    const queryParams = new URLSearchParams();

    if (params?.tag_id) {
      // 支持多个tag_id（数组）
      if (Array.isArray(params.tag_id)) {
        params.tag_id.forEach((id) =>
          queryParams.append("tag_id", id.toString()),
        );
      } else {
        queryParams.append("tag_id", params.tag_id.toString());
      }
    }

    // 添加分页参数
    if (params?.page) queryParams.append("page", params.page.toString());
    if (params?.page_size)
      queryParams.append("page_size", params.page_size.toString());

    // 添加排序和搜索参数
    if (params?.sort_by) queryParams.append("sort_by", params.sort_by);
    if (params?.search) queryParams.append("search", params.search);

    const url = queryParams.toString()
      ? `/api/v1/podcasts?${queryParams.toString()}`
      : "/api/v1/podcasts";

    console.log("[podcastApi.list] Requesting:", url);

    const response = await api.get<{
      success: boolean;
      data: Podcast[];
      pagination: {
        page: number;
        page_size: number;
        total: number;
        total_pages: number;
      };
      error?: { message: string };
    }>(url);

    console.log("[podcastApi.list] Response:", response.data);

    if (response.data.success && response.data.data) {
      return {
        data: response.data.data,
        pagination: response.data.pagination || {
          page: 1,
          page_size: 15,
          total: response.data.data.length,
          total_pages: 1,
        },
      };
    }
    throw new Error(response.data.error?.message || "Failed to fetch podcasts");
  },

  // 获取单个播客详情
  get: async (id: number): Promise<Podcast> => {
    const response = await api.get<ApiResponse<Podcast>>(
      `/api/v1/podcasts/${id}`,
    );
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error(response.data.error?.message || "Failed to fetch podcast");
  },

  // 获取播客备注
  getNotes: async (id: number): Promise<string> => {
    const response = await api.get<ApiResponse<{ id: number; notes: string }>>(
      `/api/v1/podcasts/${id}/notes`,
    );
    if (response.data.success && response.data.data) {
      return response.data.data.notes;
    }
    throw new Error(response.data.error?.message || "Failed to fetch notes");
  },

  // 更新播客备注
  updateNotes: async (id: number, notes: string): Promise<void> => {
    const response = await api.put<ApiResponse<{ id: number; notes: string }>>(
      `/api/v1/podcasts/${id}/notes`,
      { notes },
    );
    if (!response.data.success) {
      throw new Error(response.data.error?.message || "Failed to update notes");
    }
  },

  // 获取播客的所有标签
  getTags: async (id: number): Promise<Tag[]> => {
    const response = await api.get<ApiResponse<Tag[]>>(
      `/api/v1/podcasts/${id}/tags`,
    );
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error(response.data.error?.message || "Failed to fetch tags");
  },

  // 为播客添加标签
  addTag: async (id: number, tagId: number): Promise<void> => {
    const response = await api.post<ApiResponse<any>>(
      `/api/v1/podcasts/${id}/tags`,
      { tag_id: tagId },
    );
    if (!response.data.success) {
      throw new Error(response.data.error?.message || "Failed to add tag");
    }
  },

  // 移除播客标签
  removeTag: async (id: number, tagId: number): Promise<void> => {
    const response = await api.delete<ApiResponse<any>>(
      `/api/v1/podcasts/${id}/tags/${tagId}`,
    );
    if (!response.data.success) {
      throw new Error(response.data.error?.message || "Failed to remove tag");
    }
  },
};

export const episodeApi = {
  // 获取播客的单集列表
  listByPodcast: async (podcastId: number): Promise<Episode[]> => {
    const response = await api.get<ApiResponse<Episode[]>>(
      `/api/v1/podcasts/${podcastId}/episodes`,
    );
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error(response.data.error?.message || "Failed to fetch episodes");
  },

  // 获取单集备注
  getNotes: async (id: number): Promise<string> => {
    const response = await api.get<ApiResponse<{ id: number; notes: string }>>(
      `/api/v1/episodes/${id}/notes`,
    );
    if (response.data.success && response.data.data) {
      return response.data.data.notes;
    }
    throw new Error(response.data.error?.message || "Failed to fetch notes");
  },

  // 更新单集备注
  updateNotes: async (id: number, notes: string): Promise<void> => {
    const response = await api.put<ApiResponse<{ id: number; notes: string }>>(
      `/api/v1/episodes/${id}/notes`,
      { notes },
    );
    if (!response.data.success) {
      throw new Error(response.data.error?.message || "Failed to update notes");
    }
  },

  // 获取单集的所有标签
  getTags: async (id: number): Promise<Tag[]> => {
    const response = await api.get<ApiResponse<Tag[]>>(
      `/api/v1/episodes/${id}/tags`,
    );
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error(response.data.error?.message || "Failed to fetch tags");
  },

  // 为单集添加标签
  addTag: async (id: number, tagId: number): Promise<void> => {
    const response = await api.post<ApiResponse<any>>(
      `/api/v1/episodes/${id}/tags`,
      { tag_id: tagId },
    );
    if (!response.data.success) {
      throw new Error(response.data.error?.message || "Failed to add tag");
    }
  },

  // 移除单集标签
  removeTag: async (id: number, tagId: number): Promise<void> => {
    const response = await api.delete<ApiResponse<any>>(
      `/api/v1/episodes/${id}/tags/${tagId}`,
    );
    if (!response.data.success) {
      throw new Error(response.data.error?.message || "Failed to remove tag");
    }
  },
};

export const tagApi = {
  // 获取所有标签
  list: async (): Promise<Tag[]> => {
    const response = await api.get<ApiResponse<Tag[]>>("/api/v1/tags");
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error(response.data.error?.message || "Failed to fetch tags");
  },

  // 获取单个标签详情
  get: async (id: number): Promise<Tag> => {
    const response = await api.get<ApiResponse<Tag>>(`/api/v1/tags/${id}`);
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error(response.data.error?.message || "Failed to fetch tag");
  },

  // 创建标签
  create: async (data: { name: string; color?: string }): Promise<Tag> => {
    const response = await api.post<ApiResponse<Tag>>("/api/v1/tags", data);
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error(response.data.error?.message || "Failed to create tag");
  },

  // 更新标签
  update: async (id: number, data: { name?: string; color?: string }): Promise<Tag> => {
    const response = await api.put<ApiResponse<Tag>>(
      `/api/v1/tags/${id}`,
      data,
    );
    if (response.data.success && response.data.data) {
      return response.data.data;
    }
    throw new Error(response.data.error?.message || "Failed to update tag");
  },

  // 删除标签
  delete: async (id: number): Promise<void> => {
    try {
      const response = await api.delete<ApiResponse<any>>(`/api/v1/tags/${id}`);
      if (!response.data.success) {
        throw new Error(response.data.error?.message || "Failed to delete tag");
      }
    } catch (error: any) {
      // 如果是axios错误，尝试提取后端返回的错误消息
      if (error.response?.data?.error?.message) {
        throw new Error(error.response.data.error.message);
      }
      throw error;
    }
  },
};

// 健康检查
export const healthApi = {
  check: async (): Promise<{ status: string; database: string }> => {
    const response = await api.get("/health");
    return response.data;
  },
};

// 搜索API
export const searchApi = {
  // 全局搜索
  search: async (params: {
    q: string;
    type?: "all" | "podcasts" | "episodes";
    tag_id?: number | number[];
    page?: number;
    page_size?: number;
    episode_page?: number;
    episode_page_size?: number;
  }): Promise<{ data: SearchData }> => {
    const queryParams = new URLSearchParams();
    queryParams.append("q", params.q);

    if (params.type) queryParams.append("type", params.type);

    if (params.tag_id) {
      if (Array.isArray(params.tag_id)) {
        params.tag_id.forEach((id) =>
          queryParams.append("tag_id", id.toString()),
        );
      } else {
        queryParams.append("tag_id", params.tag_id.toString());
      }
    }

    if (params.page) queryParams.append("page", params.page.toString());
    if (params.page_size)
      queryParams.append("page_size", params.page_size.toString());
    if (params.episode_page)
      queryParams.append("episode_page", params.episode_page.toString());
    if (params.episode_page_size)
      queryParams.append(
        "episode_page_size",
        params.episode_page_size.toString(),
      );

    const url = `/api/v1/search?${queryParams.toString()}`;
    const response = await api.get<{
      success: boolean;
      data: SearchData;
      error?: { message: string };
    }>(url);

    if (!response.data.success) {
      throw new Error(response.data.error?.message || "Search failed");
    }

    return { data: response.data.data };
  },
};

// 同步API
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

  // 导入OPML文件（旧版本，用于回退）
  importOPML: async (
    file: File,
  ): Promise<{
    success: boolean;
    message: string;
    total_podcasts: number;
    success_count: number;
    failed_count: number;
    errors?: string[];
  }> => {
    const formData = new FormData();
    formData.append("opml_file", file);

    const response = await api.post("/api/v1/sync/import", formData, {
      headers: {
        "Content-Type": "multipart/form-data",
      },
      timeout: 300000, // 5分钟超时，用于处理大量feed导入
    });

    return response.data;
  },

  // 同步所有订阅
  syncSubscriptions: async (): Promise<{
    success: boolean;
    message: string;
    total_podcasts: number;
    success_count: number;
    failed_count: number;
    new_episodes: number;
    errors?: string[];
  }> => {
    const response = await api.post(
      "/api/v1/sync/subscriptions",
      {},
      {
        timeout: 300000, // 5分钟超时，用于处理大量订阅同步
      },
    );
    return response.data;
  },

  // 获取同步状态
  getStatus: async (): Promise<{
    success: boolean;
    total_podcasts: number;
    podcast_sources: Record<string, number>;
    last_sync_time: string | null;
  }> => {
    const response = await api.get("/api/v1/sync/status");
    return response.data;
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

// Workflow API
export const workflowApi = {
  // 获取工作流列表
  list: async (params?: {
    page?: number;
    page_size?: number;
    sort_by?: WorkflowSortByType;
  }): Promise<WorkflowsResponse> => {
    const queryParams = new URLSearchParams();
    if (params?.page) queryParams.append("page", params.page.toString());
    if (params?.page_size)
      queryParams.append("page_size", params.page_size.toString());
    if (params?.sort_by) queryParams.append("sort_by", params.sort_by);

    const url = queryParams.toString()
      ? `/api/v1/workflows?${queryParams.toString()}`
      : "/api/v1/workflows";

    const response = await api.get<{
      success: boolean;
      data: WorkflowsResponse;
    }>(url);
    return response.data.data;
  },

  // 获取工作流详情
  get: async (id: number): Promise<Workflow> => {
    const response = await api.get<{ success: boolean; data: Workflow }>(
      `/api/v1/workflows/${id}`,
    );
    return response.data.data;
  },

  // 创建工作流
  create: async (data: WorkflowRequest): Promise<Workflow> => {
    const response = await api.post<{ success: boolean; data: Workflow }>(
      "/api/v1/workflows",
      data,
    );
    return response.data.data;
  },

  // 更新工作流
  update: async (id: number, data: WorkflowRequest): Promise<Workflow> => {
    const response = await api.put<{ success: boolean; data: Workflow }>(
      `/api/v1/workflows/${id}`,
      data,
    );
    return response.data.data;
  },

  // 删除工作流
  delete: async (id: number): Promise<void> => {
    await api.delete(`/api/v1/workflows/${id}`);
  },

  // 启用/禁用工作流
  toggle: async (id: number): Promise<Workflow> => {
    const response = await api.post<{ success: boolean; data: Workflow }>(
      `/api/v1/workflows/${id}/toggle`,
    );
    return response.data.data;
  },

  // 获取工作流的执行历史
  listJobs: async (
    id: number,
    params?: { page?: number; page_size?: number },
  ): Promise<JobsResponse> => {
    const queryParams = new URLSearchParams();
    if (params?.page) queryParams.append("page", params.page.toString());
    if (params?.page_size)
      queryParams.append("page_size", params.page_size.toString());

    const url = queryParams.toString()
      ? `/api/v1/workflows/${id}/jobs?${queryParams.toString()}`
      : `/api/v1/workflows/${id}/jobs`;

    const response = await api.get<{ success: boolean; data: JobsResponse }>(
      url,
    );
    return response.data.data;
  },

  // 获取任务详情
  getJob: async (id: number): Promise<Job> => {
    const response = await api.get<{ success: boolean; data: Job }>(
      `/api/v1/jobs/${id}`,
    );
    return response.data.data;
  },

  // 手动触发工作流
  trigger: async (id: number): Promise<void> => {
    await api.post(`/api/v1/workflows/${id}/trigger`);
  },
};
