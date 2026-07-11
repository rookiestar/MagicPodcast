import { describe, expect, it } from "vitest";
import { getErrorMessage } from "@/lib/errorMessage";
import {
  buildImportErrorLogs,
  buildSyncErrorMessage,
  isOperationCompletionEvent,
} from "../syncOperationMessages";

describe("syncOperationMessages", () => {
  it("normalizes different thrown error shapes", () => {
    expect(getErrorMessage(new Error("boom"))).toBe("boom");
    expect(getErrorMessage("plain failure")).toBe("plain failure");
    expect(getErrorMessage({ message: "object failure" })).toBe(
      "object failure",
    );
    expect(getErrorMessage(null)).toBe("未知错误");
  });

  it("builds import error messages by failure category", () => {
    expect(buildImportErrorLogs(new Error("请求超时"))).toEqual([
      { type: "error", message: "导入超时：可能是网络较慢或文件太大" },
      {
        type: "info",
        message: "提示：您可以重新导入，系统会自动跳过已导入的播客",
      },
    ]);

    expect(buildImportErrorLogs("Network down")[0]).toEqual({
      type: "error",
      message: "网络连接错误：Network down",
    });

    expect(buildImportErrorLogs("用户取消")).toEqual([
      { type: "error", message: "导入被取消" },
    ]);
  });

  it("detects completion events without adding duplicate fallback logs", () => {
    expect(isOperationCompletionEvent("import", "summary", "")).toBe(true);
    expect(isOperationCompletionEvent("import", "success", "导入完成")).toBe(
      true,
    );
    expect(isOperationCompletionEvent("sync", "success", "同步已完成")).toBe(
      true,
    );
    expect(isOperationCompletionEvent("sync", "success", "更新成功")).toBe(
      false,
    );
  });

  it("builds sync error messages from non-Error throws", () => {
    expect(buildSyncErrorMessage("离线")).toBe("同步失败：离线");
  });
});
