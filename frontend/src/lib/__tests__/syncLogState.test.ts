import { describe, expect, it } from "vitest";
import {
  appendLogEntry,
  computeSyncStats,
  filterSyncLogs,
  inferSyncLogMode,
  normalizeLogType,
  normalizeSyncLogMode,
  parseSavedLogs,
  restoreSyncLogSession,
  trimLogs,
  type LogEntry,
} from "../syncLogState";

function makeLog(type: LogEntry["type"], message = "message"): LogEntry {
  return {
    id: `${type}-${message}`,
    type,
    message,
    timestamp: "12:00:00",
  };
}

describe("syncLogState", () => {
  it("derives stats from incremental logs and summary data", () => {
    const stats = computeSyncStats([
      makeLog("success"),
      makeLog("error"),
      makeLog("skip_paid"),
      {
        ...makeLog("summary"),
        data: {
          total_podcasts: 10,
          success_podcasts: 4,
          failed_podcasts: 1,
          skipped_podcasts: 2,
          no_update_podcasts: 3,
          duration: "1m2s",
        },
      },
    ]);

    expect(stats).toMatchObject({
      total: 10,
      success: 4,
      errors: 1,
      skips: 2,
      skipPaid: 1,
      skipNoUpdate: 3,
      duration: "1m2s",
      fromSummary: true,
    });
  });

  it("ignores invalid saved logs and normalizes unknown types", () => {
    expect(parseSavedLogs("{bad json")).toEqual([]);

    const logs = parseSavedLogs(
      JSON.stringify([
        { type: "unknown", message: "old", timestamp: "12:00:00" },
        { type: "complete", message: "done", timestamp: "12:00:01" },
        { type: "success" },
      ]),
    );

    expect(logs).toHaveLength(2);
    expect(logs[0].type).toBe("info");
    expect(logs[1].type).toBe("complete");
    expect(normalizeLogType("skip_paid")).toBe("skip_paid");
    expect(normalizeLogType("bad")).toBe("info");
  });

  it("keeps only the newest logs when appending past the cap", () => {
    const logs = [makeLog("info", "1"), makeLog("info", "2")];
    const appended = appendLogEntry(logs, makeLog("info", "3"));

    expect(trimLogs(appended, 2).map((log) => log.message)).toEqual(["2", "3"]);
  });

  it("filters logs consistently for the import page controls", () => {
    const logs = [
      makeLog("success", "done"),
      makeLog("error", "failed"),
      makeLog("skip_paid", "paid"),
      makeLog("skip_no_update", "单集: no update"),
      makeLog("skip_no_update", "podcast no update"),
    ];

    expect(filterSyncLogs(logs, "errors").map((log) => log.message)).toEqual([
      "failed",
    ]);
    expect(filterSyncLogs(logs, "success").map((log) => log.message)).toEqual([
      "done",
    ]);
    expect(filterSyncLogs(logs, "skips").map((log) => log.message)).toEqual([
      "paid",
    ]);
    expect(
      filterSyncLogs(logs, "no_update").map((log) => log.message),
    ).toEqual(["单集: no update", "podcast no update"]);
  });

  it("restores log mode from saved mode, content, and interrupted flags", () => {
    expect(normalizeSyncLogMode("sync")).toBe("sync");
    expect(normalizeSyncLogMode("bad")).toBeNull();
    expect(inferSyncLogMode([makeLog("info", "开始同步所有播客")])).toBe(
      "sync",
    );
    expect(inferSyncLogMode([makeLog("info", "开始导入OPML")])).toBe("import");

    expect(
      restoreSyncLogSession({
        savedLogs: JSON.stringify([makeLog("success", "同步已完成")]),
        savedLogMode: "sync",
        wasSyncing: false,
        wasImporting: false,
      }),
    ).toMatchObject({
      mode: "sync",
      logs: [{ message: "同步已完成" }],
    });

    expect(
      restoreSyncLogSession({
        savedLogs: null,
        savedLogMode: null,
        wasSyncing: true,
        wasImporting: false,
      }),
    ).toMatchObject({
      mode: "sync",
      logs: [{ message: "页面已刷新，上次同步状态已丢失" }],
    });
  });
});
