import { getErrorMessage } from "@/lib/errorMessage";
import type { LogType, SyncLogMode } from "@/lib/syncLogState";

interface OperationLogMessage {
  type: Extract<LogType, "error" | "info">;
  message: string;
}

export function isOperationCompletionEvent(
  mode: SyncLogMode,
  type: string,
  message: string,
) {
  if (type === "summary" || type === "complete") {
    return true;
  }

  if (type !== "success") {
    return false;
  }

  if (mode === "import") {
    return message.includes("导入完成");
  }

  return message.includes("同步") && message.includes("完成");
}

export function buildImportErrorLogs(error: unknown): OperationLogMessage[] {
  const message = getErrorMessage(error);

  if (message.includes("超时")) {
    return [
      { type: "error", message: "导入超时：可能是网络较慢或文件太大" },
      {
        type: "info",
        message: "提示：您可以重新导入，系统会自动跳过已导入的播客",
      },
    ];
  }

  if (message.includes("Network") || message.includes("fetch")) {
    return [
      { type: "error", message: `网络连接错误：${message}` },
      { type: "info", message: "提示：请检查网络连接后重试" },
    ];
  }

  if (message.includes("abort") || message.includes("取消")) {
    return [{ type: "error", message: "导入被取消" }];
  }

  return [
    { type: "error", message: `导入失败：${message}` },
    {
      type: "info",
      message: "提示：部分播客可能已成功导入，您可以查看播客列表",
    },
  ];
}

export function buildSyncErrorMessage(error: unknown) {
  return `同步失败：${getErrorMessage(error)}`;
}
