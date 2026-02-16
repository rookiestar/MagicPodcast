import axios from "axios";

// 在浏览器环境中使用相对路径（支持 tunnel/代理访问）
// 在 SSR 环境中使用完整 URL
const API_URL = typeof window !== "undefined"
  ? (process.env.NEXT_PUBLIC_API_URL || "")  // 浏览器：相对路径或自定义 URL
  : (process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080");  // SSR：需要完整 URL

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
  baseURL: API_URL,
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

// 带分页的 fetcher
export const fetcherWithPagination = async <T>(url: string): Promise<{
  data: T;
  pagination?: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
    has_more?: boolean;
  };
}> => {
  const response = await apiClient.get<{
    success: boolean;
    data: T;
    pagination?: {
      page: number;
      page_size: number;
      total: number;
      total_pages: number;
      has_more?: boolean;
    };
    error?: { message: string };
  }>(url);

  if (response.data.success) {
    return {
      data: response.data.data,
      pagination: response.data.pagination,
    };
  }

  throw new Error(response.data.error?.message || "Request failed");
};
