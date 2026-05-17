import { isOperationCompletionEvent } from "@/lib/syncOperationMessages";
import {
  normalizeLogType,
  type LogType,
  type SyncLogMode,
} from "@/lib/syncLogState";

export type AddSyncLog = (
  type: LogType,
  message: string,
  current?: number,
  total?: number,
  data?: Record<string, any>,
) => void;

export type SseProgressHandler = (
  type: string,
  message: string,
  current?: number,
  total?: number,
  data?: Record<string, any>,
) => void;

export async function runSseOperation({
  mode,
  addLog,
  startMessage,
  fallbackSuccessMessage,
  run,
}: {
  mode: SyncLogMode;
  addLog: AddSyncLog;
  startMessage: string;
  fallbackSuccessMessage: string;
  run: (onProgress: SseProgressHandler) => Promise<void>;
}) {
  addLog("info", startMessage);

  let receivedCompletion = false;

  await run((type, message, current, total, data) => {
    addLog(normalizeLogType(type), message, current, total, data);
    if (isOperationCompletionEvent(mode, type, message)) {
      receivedCompletion = true;
    }
  });

  if (!receivedCompletion) {
    addLog("success", fallbackSuccessMessage);
  }
}
