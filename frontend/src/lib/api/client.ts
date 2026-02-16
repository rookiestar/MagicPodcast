import axios, { type AxiosInstance } from "axios";
import { handleApiError } from "./errorHandler";

// 在浏览器环境中使用相对路径（支持 tunnel/代理访问）
// 在 SSR 环境中使用完整 URL
const API_URL = typeof window !== "undefined"
  ? (process.env.NEXT_PUBLIC_API_URL || "")  // 浏览器：相对路径或自定义 URL
  : (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080");  // SSR：需要完整 URL

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
  baseURL: API_URL,
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

    // 使用全局错误处理器处理错误
    handleApiError(error, error.config?.url);

    return Promise.reject(error);
  },
);
