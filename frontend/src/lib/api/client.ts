import axios, { type AxiosInstance } from "axios";
import { apiBaseUrl } from "../apiBaseUrl";
import { getApiErrorMessage, handleApiError } from "./errorHandler";
import type { ApiResponse } from "@/types";

// ============ API 响应处理辅助函数 ============

/**
 * 自定义 API 错误类，包含错误码信息
 */
class ApiError extends Error {
  constructor(
    message: string,
    public code?: string
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/**
 * 处理 API 响应，提取数据或抛出错误
 * 统一的响应处理，消除每个 API 方法的重复代码
 */
export function handleResponse<T>(response: { data: ApiResponse<T> }): T {
  if (response.data.success && response.data.data !== undefined) {
    return response.data.data;
  }
  throw new ApiError(
    response.data.error?.message || "Request failed",
    response.data.error?.code
  );
}

/**
 * 处理没有返回数据的 API 响应（如删除操作）
 */
export function handleVoidResponse(response: { data: ApiResponse<void> }): void {
  if (!response.data.success) {
    throw new ApiError(
      response.data.error?.message || "Request failed",
      response.data.error?.code
    );
  }
}

// 自定义参数序列化函数（兼容 Gin 的 QueryArray）
// 将 { tag_id: [1, 2] } 序列化为 ?tag_id=1&tag_id=2 而不是 ?tag_id[]=1&tag_id[]=2
const paramsSerializer = (params: Record<string, unknown>): string => {
  const searchParams = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null) return;
    if (Array.isArray(value)) {
      value.forEach((item) => searchParams.append(key, String(item)));
    } else {
      searchParams.append(key, String(value));
    }
  });
  return searchParams.toString();
};

// 创建 axios 实例
export const api: AxiosInstance = axios.create({
  baseURL: apiBaseUrl,
  timeout: 60000, // 增加到60秒，支持分页加载大量数据
  headers: {
    "Content-Type": "application/json",
  },
  withCredentials: false, // 不发送凭证，避免CORS问题
  paramsSerializer,
});

// 添加请求拦截器
api.interceptors.request.use(
  (config) => {
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
    return response;
  },
  (error) => {
    if (axios.isCancel(error) || error.code === "ERR_CANCELED") {
      return Promise.reject(error);
    }

    console.error("[API] Response error:", error.message, error.config?.url);

    const apiMessage = getApiErrorMessage(error.response?.data);
    if (apiMessage) {
      error.message = apiMessage;
    }

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

    // 使用全局错误处理器处理错误
    handleApiError(error, error.config?.url);

    return Promise.reject(error);
  },
);
