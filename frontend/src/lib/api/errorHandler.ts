import { toast } from "@/lib/toast";

// API 错误响应数据结构
interface ApiErrorResponse {
  code?: string;
  message: string;
  details?: any;
}

/**
 * 全局API错误处理器
 * 根据不同的错误状态码和错误类型显示友好的错误提示
 */
export const handleApiError = (error: any, context?: string) => {
  console.error(`[API Error]${context ? ` (${context})` : ""}:`, error);

  // 网络错误或请求超时
  if (error.code === "ECONNABORTED") {
    toast.error("请求超时，请检查网络连接或稍后再试");
    return;
  }

  // 请求已发出但没有收到响应
  if (error.request) {
    toast.error("网络错误，请检查网络连接");
    return;
  }

  // 服务器返回了错误响应
  if (error.response) {
    const status = error.response.status;
    const data: ApiErrorResponse = error.response.data || {};

    // 根据状态码显示不同的错误消息
    switch (status) {
      case 400:
        toast.error(data.message || "请求参数错误，请检查输入");
        break;

      case 401:
        toast.error("未授权，请重新登录");
        break;

      case 403:
        toast.error("无权限访问该资源");
        break;

      case 404:
        toast.error("请求的资源不存在");
        break;

      case 409:
        toast.error(data.message || "操作冲突，请稍后再试");
        break;

      case 429:
        toast.error("请求过于频繁，请稍后再试");
        break;

      case 500:
        toast.error("服务器内部错误，请稍后再试");
        break;

      case 502:
      case 503:
      case 504:
        toast.error("服务暂时不可用，请稍后再试");
        break;

      default:
        toast.error(data.message || `请求失败 (${status})`);
    }

    return;
  }

  // 未知错误
  toast.error("未知错误，请稍后再试");
};

/**
 * 成功提示辅助函数
 */
export const showSuccess = (message: string) => {
  toast.success(message);
};

/**
 * 信息提示辅助函数
 */
export const showInfo = (message: string) => {
  toast.info(message);
};

/**
 * 警告提示辅助函数
 */
export const showWarning = (message: string) => {
  toast.warning(message);
};

/**
 * 表单验证提示辅助函数
 */
export const showValidationError = (message: string) => {
  toast.warning(message);
};
