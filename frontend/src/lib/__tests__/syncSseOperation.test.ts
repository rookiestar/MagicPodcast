import { describe, expect, it } from "vitest";
import {
  runSseOperation,
  type AddSyncLog,
  type SseProgressHandler,
} from "../syncSseOperation";
import type { LogType } from "../syncLogState";

function createLogCollector() {
  const logs: Array<{
    type: LogType;
    message: string;
    current?: number;
    total?: number;
    data?: Record<string, any>;
  }> = [];

  const addLog: AddSyncLog = (type, message, current, total, data) => {
    logs.push({ type, message, current, total, data });
  };

  return { logs, addLog };
}

describe("runSseOperation", () => {
  it("adds a fallback success log when the stream has no completion event", async () => {
    const { logs, addLog } = createLogCollector();

    await runSseOperation({
      mode: "sync",
      addLog,
      startMessage: "开始同步",
      fallbackSuccessMessage: "同步已完成",
      run: async (onProgress: SseProgressHandler) => {
        onProgress("progress", "处理中", 1, 2);
      },
    });

    expect(logs.map((log) => [log.type, log.message])).toEqual([
      ["info", "开始同步"],
      ["progress", "处理中"],
      ["success", "同步已完成"],
    ]);
  });

  it("does not add fallback success when the stream already completed", async () => {
    const { logs, addLog } = createLogCollector();

    await runSseOperation({
      mode: "import",
      addLog,
      startMessage: "开始导入",
      fallbackSuccessMessage: "导入完成",
      run: async (onProgress: SseProgressHandler) => {
        onProgress("summary", "导入完成", undefined, undefined, {
          operation: "import",
          success_podcasts: 1,
        });
      },
    });

    expect(logs.map((log) => [log.type, log.message])).toEqual([
      ["info", "开始导入"],
      ["summary", "导入完成"],
    ]);
  });
});
