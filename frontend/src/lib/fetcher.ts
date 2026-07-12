import axios from "axios";
import { apiBaseUrl } from "./apiBaseUrl";

// 自定义参数序列化函数（兼容 Gin 的 QueryArray）
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

// 创建专用于 SWR 的 axios 实例（简化版，不带调试日志）
export const apiClient = axios.create({
  baseURL: apiBaseUrl,
  timeout: 60000,
  headers: {
    "Content-Type": "application/json",
  },
  withCredentials: false,
  paramsSerializer,
});

// SWR 通用 fetcher
export const fetcher = async <T>(url: string): Promise<T> => {
  const response = await apiClient.get<{
    success: boolean;
    data?: T;
    error?: { message: string };
  }>(url);

  if (response.data.success && response.data.data !== undefined) {
    return response.data.data;
  }

  throw new Error(response.data.error?.message || "Request failed");
};
