import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { syncApi } from "@/lib/api";
import { importTasksApi, RETRY_CONFIRMATION_TEXT } from "@/lib/api/importTasks";
import { toast } from "@/lib/toast";
import { countRetryableEntries, useImportSyncOperations } from "../useImportSyncOperations";
import type { ImportTask } from "@/lib/api/importTasks";

vi.mock("@/lib/toast", () => ({
  toast: {
    warning: vi.fn(),
    info: vi.fn(),
    success: vi.fn(),
    error: vi.fn(),
  },
}));

vi.mock("@/lib/confirmation", () => ({
  requestTypedConfirmation: vi.fn(() => "IMPORT OPML"),
}));

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers(); });

function setup() {
  const addLog = vi.fn();
  const resetLogScroll = vi.fn();
  const startLogSession = vi.fn();
  const hook = renderHook(() =>
    useImportSyncOperations({ addLog, resetLogScroll, startLogSession }),
  );
  return { hook, addLog, resetLogScroll, startLogSession };
}

function opmlFile(name = "feeds.opml") {
  return new File(["<opml/>"], name, { type: "text/opml" });
}

const reviewTask: ImportTask = {
  id: 41, status: "running", file_name: "review.opml", total: 3, processed: 1,
  success_count: 1, pending_count: 0, conflict_count: 0, merged_count: 0,
  unchanged_count: 0, skipped_count: 0, failed_count: 0, error_message: "",
  started_at: "2099-01-01T00:00:00Z",
};

it("does not invent retryable entries from counters when persisted entries are absent", () => {
  expect(countRetryableEntries({ ...reviewTask, status: "interrupted", failed_count: 1 }, [])).toBe(0);
});

it("keeps restored running tasks busy and exposes polling failure without losing results", async () => {
  vi.useFakeTimers();
  vi.spyOn(importTasksApi, "fetchLatestImportTask").mockResolvedValue({ success: true, task: reviewTask, entries: [] });
  vi.spyOn(importTasksApi, "fetchImportTask")
    .mockRejectedValueOnce(new Error("offline"))
    .mockResolvedValueOnce({ success: true, task: { ...reviewTask, status: "completed" }, entries: [] });
  const { hook } = setup();
  await act(async () => { await Promise.resolve(); });
  expect(hook.result.current.importing).toBe(true);
  await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
  expect(hook.result.current.latestTaskError).toBe(true);
  expect(hook.result.current.lastTask?.id).toBe(41);
  await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
  expect(hook.result.current.latestTaskError).toBe(false);
  expect(hook.result.current.importing).toBe(false);
  hook.unmount();
});

it("ignores an old in-flight poll after a newer task has been restored", async () => {
  vi.useFakeTimers();
  vi.spyOn(importTasksApi, "fetchLatestImportTask")
    .mockResolvedValueOnce({ success: true, task: reviewTask, entries: [] })
    .mockResolvedValueOnce({ success: true, task: { ...reviewTask, id: 42, status: "completed" }, entries: [] });
  let resolvePoll!: (value: Awaited<ReturnType<typeof importTasksApi.fetchImportTask>>) => void;
  vi.spyOn(importTasksApi, "fetchImportTask").mockReturnValue(new Promise(resolve => { resolvePoll = resolve; }));
  const { hook, addLog } = setup();
  await act(async () => { await Promise.resolve(); });
  await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
  await act(async () => { await hook.result.current.refreshLatestTask(); });
  await act(async () => {
    resolvePoll({ success: true, task: { ...reviewTask, status: "completed" }, entries: [] });
  });
  expect(hook.result.current.lastTask?.id).toBe(42);
  expect(addLog).not.toHaveBeenCalled();
  expect(vi.getTimerCount()).toBe(0);
  hook.unmount();
});

it("restores the same task and derives summary statistics from persisted results", async () => {
  vi.useFakeTimers();
  const task = {id:7, status:"running" as const, file_name:"test.opml", total:3, processed:1, success_count:1, pending_count:0, conflict_count:0, merged_count:0, unchanged_count:0, skipped_count:0, failed_count:0, error_message:"", started_at:"2026-09-14T00:00:00Z"};
  const entries = [{title:"one", feed_url:"https://example.com/rss", outcome:"new"}];
  vi.spyOn(importTasksApi, "fetchLatestImportTask").mockResolvedValue({success:true, task, entries});
  const fetchTask = vi.spyOn(importTasksApi, "fetchImportTask").mockResolvedValue({success:true, task:{...task, status:"completed", processed:3, pending_count:1, failed_count:1}, entries});
  const addLog = vi.fn();
  const resetLogScroll = vi.fn();
  const startLogSession = vi.fn();
  const {result, unmount} = renderHook(() => useImportSyncOperations({addLog, resetLogScroll, startLogSession}));
  await act(async () => { await Promise.resolve(); });
  expect(result.current.lastTask?.id).toBe(7);
  expect(result.current.taskEntries).toEqual(entries);
  await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
  expect(fetchTask).toHaveBeenCalledWith(7);
  expect(addLog).toHaveBeenCalledWith("summary", expect.any(String), undefined, undefined, expect.objectContaining({total_podcasts:3, success_podcasts:1, stub_podcasts:1, failed_podcasts:1}));
  unmount();
  expect(vi.getTimerCount()).toBe(0);
});

it("tracks an async retry child immediately instead of waiting for the HTTP result", async () => {
  vi.useFakeTimers();
  const interruptedTask: ImportTask = {
    ...reviewTask,
    id: 41,
    status: "interrupted",
    file_name: "interrupted.opml",
    total: 1,
    processed: 1,
  };
  const entry = { title: "Pending", feed_url: "https://example.com/pending", outcome: "pending" };
  const runningChild: ImportTask = {
    ...interruptedTask,
    id: 42,
    status: "running",
    file_name: "重试任务#41(1条)",
    processed: 0,
    started_at: "2026-09-17T00:00:00Z",
  };
  const completedChild: ImportTask = {
    ...runningChild,
    status: "completed",
    processed: 1,
    pending_count: 1,
    finished_at: "2026-09-17T00:00:01Z",
  };
  vi.spyOn(importTasksApi, "fetchLatestImportTask").mockResolvedValue({ success: true, task: interruptedTask, entries: [entry] });
  const fetchTask = vi.spyOn(importTasksApi, "fetchImportTask").mockResolvedValue({ success: true, task: completedChild, entries: [entry] });
  const retry = vi.spyOn(importTasksApi, "retryImportTask").mockResolvedValue({
    success: true,
    task_id: runningChild.id,
    parent_task_id: interruptedTask.id,
    status: "running",
    task: runningChild,
    message: "重试任务已创建",
    total_podcasts: 1,
    success_count: 0,
    failed_count: 0,
    stub_podcasts: 0,
    entries: [],
  });
  const { hook, addLog } = setup();
  await act(async () => { await Promise.resolve(); });

  await act(async () => {
    await hook.result.current.handleRetry(undefined, RETRY_CONFIRMATION_TEXT);
  });

  expect(retry).toHaveBeenCalledWith(interruptedTask.id, undefined);
  expect(hook.result.current.lastTask?.id).toBe(runningChild.id);
  expect(hook.result.current.lastTask?.status).toBe("running");
  expect(hook.result.current.importing).toBe(true);
  expect(addLog).toHaveBeenCalledWith("info", expect.stringContaining("后台处理"));

  await act(async () => { await vi.advanceTimersByTimeAsync(4000); });
  expect(fetchTask).toHaveBeenCalledWith(runningChild.id);
  expect(hook.result.current.importing).toBe(false);
  expect(addLog).toHaveBeenCalledWith("summary", expect.any(String), undefined, undefined, expect.objectContaining({ total_podcasts: 1 }));
  hook.unmount();
});

it("does not start the import while the preview has not succeeded (#427 AC2)", async () => {
  const previewRun = vi.spyOn(importTasksApi, "previewImportOPML").mockReturnValue(new Promise(() => {}));
  const importSSE = vi.spyOn(syncApi, "importOPMLSSE").mockResolvedValue(undefined);
  vi.spyOn(importTasksApi, "fetchLatestImportTask").mockResolvedValue({ success: true, task: null, entries: [] });

  const { hook } = setup();
  act(() => {
    hook.result.current.handleFileChange({ target: { files: [opmlFile()] } } as never);
  });
  expect(hook.result.current.previewLoading).toBe(true);
  expect(hook.result.current.canSubmitImport).toBe(false);

  await act(async () => {
    await hook.result.current.handleImport();
  });
  expect(importSSE).not.toHaveBeenCalled();
  expect(toast.warning).toHaveBeenCalledWith("请先完成当前文件的预览核对，再开始导入");
  expect(previewRun).toHaveBeenCalledTimes(1);
});

it("lets a failed preview be retried for the same file without reselecting (#427 AC3)", async () => {
  const previewSpy = vi.spyOn(importTasksApi, "previewImportOPML")
    .mockRejectedValueOnce(new Error("预览服务暂时不可用"))
    .mockResolvedValueOnce({
      total: 1,
      entries: [{ xml_url: "https://example.com/a.xml", title: "A", kind: "new" }],
      new_count: 1, existing_count: 0, collection_count: 0,
      deleted_count: 0, invalid_count: 0, duplicate_merged_count: 0,
    });
  vi.spyOn(importTasksApi, "fetchLatestImportTask").mockResolvedValue({ success: true, task: null, entries: [] });

  const { hook } = setup();
  act(() => {
    hook.result.current.handleFileChange({ target: { files: [opmlFile("same.opml")] } } as never);
  });
  await waitFor(() => expect(hook.result.current.previewError).toBe("预览服务暂时不可用"));
  expect(hook.result.current.canSubmitImport).toBe(false);

  act(() => {
    hook.result.current.retryPreview();
  });
  await waitFor(() => expect(hook.result.current.preview).not.toBeNull());
  expect(hook.result.current.previewError).toBeNull();
  expect(hook.result.current.canSubmitImport).toBe(true);
  expect(previewSpy).toHaveBeenCalledTimes(2);
  expect(previewSpy.mock.calls[1][0].name).toBe("same.opml");
});

it("keeps the newest file's preview when an older request settles late (#427 AC3)", async () => {
  let resolveA: (value: never) => void = () => {};
  const deferredA = new Promise((resolve) => { resolveA = resolve as typeof resolveA; });
  const previewSpy = vi.spyOn(importTasksApi, "previewImportOPML")
    .mockImplementationOnce(() => deferredA as never)
    .mockResolvedValueOnce({
      total: 1,
      entries: [{ xml_url: "https://example.com/b.xml", title: "B", kind: "new" }],
      new_count: 1, existing_count: 0, collection_count: 0,
      deleted_count: 0, invalid_count: 0, duplicate_merged_count: 0,
    });
  vi.spyOn(importTasksApi, "fetchLatestImportTask").mockResolvedValue({ success: true, task: null, entries: [] });

  const { hook } = setup();
  act(() => {
    hook.result.current.handleFileChange({ target: { files: [opmlFile("a.opml")] } } as never);
  });
  act(() => {
    hook.result.current.handleFileChange({ target: { files: [opmlFile("b.opml")] } } as never);
  });
  await waitFor(() => expect(hook.result.current.preview).not.toBeNull());
  expect(hook.result.current.file?.name).toBe("b.opml");

  // A 的慢响应晚于 B 到达：不得覆盖 B 的预览结果。
  await act(async () => {
    resolveA({
      total: 9,
      entries: [{ xml_url: "https://example.com/a.xml", title: "A", kind: "new" }],
      new_count: 9, existing_count: 0, collection_count: 0,
      deleted_count: 0, invalid_count: 0, duplicate_merged_count: 0,
    } as never);
    await deferredA.catch(() => {});
  });
  expect(hook.result.current.preview?.total).toBe(1);
  expect(hook.result.current.preview?.entries[0]?.title).toBe("B");
  expect(previewSpy).toHaveBeenCalledTimes(2);
});

it("marks latest-task read failures and keeps the last known task (#427 AC8)", async () => {
  const task = {id:7, status:"completed" as const, file_name:"old.opml", total:1, processed:1, success_count:1, pending_count:0, conflict_count:0, merged_count:0, unchanged_count:0, skipped_count:0, failed_count:0, error_message:"", started_at:"2026-09-14T00:00:00Z"};
  const latestSpy = vi.spyOn(importTasksApi, "fetchLatestImportTask")
    .mockResolvedValueOnce({ success: true, task, entries: [] })
    .mockRejectedValueOnce(new Error("network down"))
    .mockResolvedValueOnce({ success: true, task: { ...task, id: 8 }, entries: [] });

  const { hook } = setup();
  await act(async () => { await Promise.resolve(); });
  expect(hook.result.current.lastTask?.id).toBe(7);
  expect(hook.result.current.latestTaskError).toBe(false);

  await act(async () => {
    await hook.result.current.refreshLatestTask();
  });
  expect(hook.result.current.latestTaskError).toBe(true);
  // 读取失败不把已知结果清成“无任务”。
  expect(hook.result.current.lastTask?.id).toBe(7);

  await act(async () => {
    await hook.result.current.refreshLatestTask();
  });
  expect(hook.result.current.latestTaskError).toBe(false);
  expect(hook.result.current.lastTask?.id).toBe(8);
  expect(latestSpy).toHaveBeenCalledTimes(3);
});

it("counts retryable entries for a failed task so the banner can offer retry (#427 AC7)", async () => {
  const task = {id:9, status:"failed" as const, file_name:"f.opml", total:3, processed:3, success_count:0, pending_count:1, conflict_count:0, merged_count:0, unchanged_count:0, skipped_count:0, failed_count:2, error_message:"boom", started_at:"2026-09-14T00:00:00Z"};
  const entries = [
    { title: "a", feed_url: "https://example.com/a", outcome: "failed" },
    { title: "b", feed_url: "https://example.com/b", outcome: "failed" },
    { title: "c", feed_url: "https://example.com/c", outcome: "pending" },
  ];
  vi.spyOn(importTasksApi, "fetchLatestImportTask").mockResolvedValue({ success: true, task, entries });

  const { hook } = setup();
  await act(async () => { await Promise.resolve(); });
  expect(hook.result.current.countRetryableEntries(hook.result.current.lastTask!, hook.result.current.taskEntries)).toBe(3);
});

it("updates the log while a restored import is running, without repeating unchanged progress", async () => {
  vi.useFakeTimers();
  vi.spyOn(importTasksApi, "fetchLatestImportTask").mockResolvedValue({success:true,task:reviewTask,entries:[]});
  vi.spyOn(importTasksApi, "fetchImportTask")
    .mockResolvedValueOnce({success:true,task:{...reviewTask,processed:2},entries:[]})
    .mockResolvedValueOnce({success:true,task:{...reviewTask,processed:2},entries:[]});
  const {hook,addLog}=setup();
  await act(async()=>{ await Promise.resolve(); });
  await act(async()=>{ await vi.advanceTimersByTimeAsync(4000); });
  expect(addLog).toHaveBeenCalledWith("progress",expect.any(String),2,3);
  await act(async()=>{ await vi.advanceTimersByTimeAsync(4000); });
  expect(addLog.mock.calls.filter(call=>call[0]==="progress")).toHaveLength(1);
  hook.unmount();
});
