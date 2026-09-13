import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { importTasksApi } from "@/lib/api/importTasks";
import { useImportSyncOperations } from "../useImportSyncOperations";

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers(); });

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
