import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ImportPage from "../ImportPageClient";
import { syncApi } from "@/lib/api";
import { importTasksApi, type ImportPreview } from "@/lib/api/importTasks";
import { toast } from "@/lib/toast";
import { navigate } from "@/lib/navigation";

vi.mock("@/components/layout/PageLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/lib/api", () => ({
  syncApi: {
    importOPMLSSE: vi.fn(),
    syncPodcastsMetadataSSE: vi.fn(),
  },
}));

vi.mock("@/lib/api/importTasks", () => ({
  CONFIRMABLE_KINDS: new Set(["collection", "deleted"]),
  RETRY_CONFIRMATION_TEXT: "RETRY IMPORT",
  importTasksApi: {
    previewImportOPML: vi.fn(),
    fetchLatestImportTask: vi.fn(),
    fetchImportTask: vi.fn(),
    fetchTaskNewPodcasts: vi.fn(),
    retryImportTask: vi.fn(),
  },
}));

vi.mock("@/lib/toast", () => ({
  toast: {
    warning: vi.fn(),
  },
}));

const memoryStorage = new Map<string, string>();
const importOPMLSSE = vi.mocked(syncApi.importOPMLSSE);
const syncPodcastsMetadataSSE = vi.mocked(syncApi.syncPodcastsMetadataSSE);
const toastWarning = vi.mocked(toast.warning);
const previewImportOPML = vi.mocked(importTasksApi.previewImportOPML);
const fetchLatestImportTask = vi.mocked(importTasksApi.fetchLatestImportTask);
const fetchTaskNewPodcasts = vi.mocked(importTasksApi.fetchTaskNewPodcasts);
const retryImportTask = vi.mocked(importTasksApi.retryImportTask);

function successfulPreview(): ImportPreview {
  return {
    total: 1,
    entries: [
      { xml_url: "https://example.com/a.xml", title: "A", kind: "new" },
    ],
    new_count: 1,
    existing_count: 0,
    collection_count: 0,
    deleted_count: 0,
    invalid_count: 0,
    duplicate_merged_count: 0,
  };
}

function installLocalStorageMock() {
  memoryStorage.clear();

  const localStorageMock = {
    getItem: vi.fn((key: string) => memoryStorage.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      memoryStorage.set(key, value);
    }),
    removeItem: vi.fn((key: string) => {
      memoryStorage.delete(key);
    }),
    clear: vi.fn(() => {
      memoryStorage.clear();
    }),
  };

  Object.defineProperty(globalThis, "localStorage", {
    value: localStorageMock,
    configurable: true,
  });
  Object.defineProperty(window, "localStorage", {
    value: localStorageMock,
    configurable: true,
  });
}

function deferred<T = void>() {
  let resolve: (value: T | PromiseLike<T>) => void = () => {};
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });

  return { promise, resolve };
}

describe("ImportPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.history.replaceState({}, "", "/import");
    Object.defineProperty(window, "confirm", {
      configurable: true,
      writable: true,
      value: vi.fn(() => true),
    });
    Object.defineProperty(window, "prompt", {
      configurable: true,
      writable: true,
      value: vi.fn((message: string) =>
        message.includes("IMPORT OPML") ? "IMPORT OPML" : "SYNC ALL",
      ),
    });
    installLocalStorageMock();
    localStorage.clear();
    fetchLatestImportTask.mockResolvedValue({
      success: true,
      task: null,
      entries: [],
    });
    fetchTaskNewPodcasts.mockResolvedValue({
      success: true,
      task_id: 0,
      total: 0,
      podcasts: [],
    });
    previewImportOPML.mockResolvedValue(successfulPreview());
  });

  it("restores URL tabs without starting import or sync", async () => {
    window.history.replaceState({}, "", "/import?tab=sync");
    render(<ImportPage />);
    expect(screen.getByRole("tab", { name: "同步已关注节目" })).toHaveAttribute("aria-selected", "true");
    fireEvent.click(screen.getByRole("tab", { name: "导入 OPML" }));
    expect(window.location.search).toBe("?tab=import");
    act(() => { navigate("/import?tab=sync"); });
    expect(screen.getByRole("tab", { name: "同步已关注节目" })).toHaveAttribute("aria-selected", "true");
    expect(importOPMLSSE).not.toHaveBeenCalled();
    expect(syncPodcastsMetadataSSE).not.toHaveBeenCalled();
  });

  it("confirms interrupted-task retry in the page instead of using window.prompt", async () => {
    fetchLatestImportTask.mockResolvedValue({
      success: true,
      task: {
        id: 1,
        status: "interrupted",
        file_name: "cosmos.opml",
        total: 2,
        processed: 1,
        success_count: 1,
        pending_count: 0,
        conflict_count: 0,
        merged_count: 0,
        unchanged_count: 0,
        skipped_count: 0,
        failed_count: 0,
        error_message: "",
        started_at: "2026-09-16T00:00:00Z",
      },
      entries: [{ title: "未完成节目", feed_url: "https://example.com/unprocessed.xml", outcome: "unprocessed" }],
    });
    retryImportTask.mockResolvedValue({
      success: true,
      task_id: 2,
      parent_task_id: 1,
      message: "重试完成",
      total_podcasts: 1,
      success_count: 1,
      failed_count: 0,
      stub_podcasts: 0,
      entries: [],
    });

    render(<ImportPage />);
    const retry = await screen.findByRole("button", { name: "继续未完成条目（1 条）" });
    fireEvent.click(retry);

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    const confirmationInput = screen.getByLabelText(/输入确认文字/);
    fireEvent.change(confirmationInput, { target: { value: "RETRY IMPORT" } });
    fireEvent.click(screen.getByRole("button", { name: "确认重试" }));

    await waitFor(() => expect(retryImportTask).toHaveBeenCalledWith(1, undefined));
    expect(window.prompt).not.toHaveBeenCalled();
  });

  it("uses preview consumption rather than server time and keeps sync layout independent", async () => {
    fetchLatestImportTask.mockResolvedValue({ success: true, entries: [], task: {
      id: 9, status: "completed", file_name: "old.opml", total: 1, processed: 1,
      success_count: 1, pending_count: 0, conflict_count: 0, merged_count: 0,
      unchanged_count: 0, skipped_count: 0, failed_count: 0, error_message: "",
      started_at: "2099-01-01T00:00:00Z", finished_at: "2099-01-01T00:01:00Z",
    } });
    const { container } = render(<ImportPage />);
    await screen.findByText(/上次导入任务 #9/);
    fireEvent.change(screen.getByLabelText("选择 OPML 文件"), {
      target: { files: [new File(["<opml/>"], "new.opml")] },
    });
    await waitFor(() => expect(screen.getByRole("button", { name: "开始导入" })).toBeEnabled());
    expect(container.querySelector(".import-workspace")).toHaveAttribute("data-stage", "preview");
    fireEvent.click(screen.getByRole("tab", { name: "同步已关注节目" }));
    expect(container.querySelector(".import-workspace")).toHaveAttribute("data-stage", "idle");
    fireEvent.click(screen.getByRole("tab", { name: "导入 OPML" }));
    importOPMLSSE.mockResolvedValue(undefined);
    fireEvent.click(screen.getByRole("button", { name: "开始导入" }));
    await waitFor(() => expect(container.querySelector(".import-workspace")).toHaveAttribute("data-stage", "done"));
  });

  it("normalizes repeated tabs without dropping unrelated parameters", async () => {
    window.history.replaceState({}, "", "/import?tab=sync&tab=import&keep=1");
    render(<ImportPage />);
    await waitFor(() => expect(window.location.search).toBe("?keep=1"));
    expect(screen.getByRole("tab", { name: "导入 OPML" })).toHaveAttribute("aria-selected", "true");
  });

  it("restores saved logs and derives stats from them", async () => {
    localStorage.setItem(
      "syncLogs",
      JSON.stringify([
        { id: "1", type: "success", message: "ok", timestamp: "12:00:00" },
        { id: "2", type: "error", message: "bad", timestamp: "12:00:01" },
        { id: "3", type: "skip_paid", message: "paid", timestamp: "12:00:02" },
        {
          id: "4",
          type: "skip_no_update",
          message: "no update",
          timestamp: "12:00:03",
        },
      ]),
    );

    render(<ImportPage />);

    await waitFor(() => {
      expect(screen.getByText("全部 (4)")).toBeInTheDocument();
    });
    expect(screen.getByText("成功 (1)")).toBeInTheDocument();
    expect(screen.getByText("失败 (1)")).toBeInTheDocument();
    expect(screen.getByText("跳过 (1)")).toBeInTheDocument();
    expect(screen.getByText("无更新 (1)")).toBeInTheDocument();
  });

  it("shows a clear refresh message for interrupted sync state", async () => {
    localStorage.setItem("syncing", "true");

    render(<ImportPage />);

    await waitFor(() => {
      expect(
        screen.getByText("页面已刷新，上次同步状态已丢失"),
      ).toBeInTheDocument();
    });
  });

  it("restores log labels without overriding the URL-selected operation", async () => {
    window.history.replaceState({}, "", "/import?tab=import");
    localStorage.setItem("syncLogMode", "sync");
    localStorage.setItem(
      "syncLogs",
      JSON.stringify([
        {
          id: "1",
          type: "success",
          message: "同步已完成",
          timestamp: "12:00:00",
        },
      ]),
    );

    render(<ImportPage />);

    await waitFor(() => {
      expect(
        screen.getByRole("heading", { name: "同步日志" }),
      ).toBeInTheDocument();
    });
    expect(screen.getByRole("tab", { name: "导入 OPML" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(
      screen.queryByRole("heading", { name: "导入日志" }),
    ).not.toBeInTheDocument();
  });

  it("does not start duplicate sync jobs while one is already running", async () => {
    const syncRun = deferred();
    syncPodcastsMetadataSSE.mockReturnValue(syncRun.promise);

    render(<ImportPage />);

    fireEvent.click(screen.getByRole("tab", { name: "同步已关注节目" }));
    const startButton = screen.getByRole("button", { name: "开始同步" });

    fireEvent.click(startButton);
    fireEvent.click(startButton);

    expect(syncPodcastsMetadataSSE).toHaveBeenCalledTimes(1);

    syncRun.resolve();
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "开始同步" })).toBeEnabled();
    });
  });

  it("persists the active sync marker only while sync is running", async () => {
    const syncRun = deferred();
    syncPodcastsMetadataSSE.mockReturnValue(syncRun.promise);

    render(<ImportPage />);

    fireEvent.click(screen.getByRole("tab", { name: "同步已关注节目" }));
    fireEvent.click(screen.getByRole("button", { name: "开始同步" }));

    await waitFor(() => {
      expect(localStorage.getItem("syncing")).toBe("true");
    });

    await act(async () => {
      syncRun.resolve();
      await syncRun.promise;
    });

    await waitFor(() => {
      expect(localStorage.getItem("syncing")).toBeNull();
    });
  });

  it("rejects invalid OPML files without changing the selected file", () => {
    render(<ImportPage />);

    const fileInput = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;

    fireEvent.change(fileInput, {
      target: {
        files: [new File(["plain"], "notes.txt", { type: "text/plain" })],
      },
    });

    expect(toastWarning).toHaveBeenCalledWith("请选择OPML或XML文件");
    expect(screen.queryByText(/已选择:/)).not.toBeInTheDocument();
  });

  it("does not add fallback import completion when the stream already completed", async () => {
    importOPMLSSE.mockImplementation(async (_file, onProgress) => {
      onProgress("summary", "导入完成", undefined, undefined, {
        operation: "import",
        total_podcasts: 1,
        success_podcasts: 1,
        failed_podcasts: 0,
        skipped_podcasts: 0,
        stub_podcasts: 0,
      });
    });

    render(<ImportPage />);

    const fileInput = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    const file = new File(["<opml></opml>"], "feeds.opml", {
      type: "text/opml",
    });

    fireEvent.change(fileInput, { target: { files: [file] } });
    await waitFor(() => {
      expect(screen.getByLabelText("导入预览")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: "开始导入" }));

    await waitFor(() => {
      expect(screen.getAllByText("导入完成")).toHaveLength(1);
    });
  });

  it("cannot start the import while the preview request is still pending (AC2)", async () => {
    let resolvePreview: (value: ImportPreview) => void = () => {};
    previewImportOPML.mockReturnValue(
      new Promise<ImportPreview>((resolve) => {
        resolvePreview = resolve;
      }),
    );

    render(<ImportPage />);

    const fileInput = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    fireEvent.change(fileInput, {
      target: {
        files: [new File(["<opml/>"], "slow.opml", { type: "text/opml" })],
      },
    });

    // 预览未完成：按钮不可用，直接调用处理函数也不能绕过（toast 拦截）。
    await waitFor(() => {
      expect(screen.getByText("正在核对本地库差异...")).toBeInTheDocument();
    });
    const startButton = screen.getByRole("button", { name: "开始导入" });
    expect(startButton).toBeDisabled();
    fireEvent.click(startButton);
    await act(async () => {
      await Promise.resolve();
    });
    expect(importOPMLSSE).not.toHaveBeenCalled();

    resolvePreview(successfulPreview());
    await waitFor(() => {
      expect(startButton).toBeEnabled();
    });
  });

  it("cannot start the import after a preview failure until retry succeeds (AC2/AC3)", async () => {
    previewImportOPML
      .mockRejectedValueOnce(new Error("预览服务暂时不可用"))
      .mockResolvedValueOnce(successfulPreview());

    render(<ImportPage />);

    const fileInput = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    fireEvent.change(fileInput, {
      target: {
        files: [new File(["<opml/>"], "fail.opml", { type: "text/opml" })],
      },
    });

    await waitFor(() => {
      expect(screen.getByText("预览服务暂时不可用")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "开始导入" })).toBeDisabled();
    expect(importOPMLSSE).not.toHaveBeenCalled();

    // 同一文件的直接重试入口恢复可导入状态。
    fireEvent.click(screen.getByRole("button", { name: /重试预览「fail\.opml」/ }));
    await waitFor(() => {
      expect(screen.getByLabelText("导入预览")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "开始导入" })).toBeEnabled();
  });

  it("shows sync errors even when the thrown value is not an Error", async () => {
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    syncPodcastsMetadataSSE.mockRejectedValue("离线");

    try {
      render(<ImportPage />);

      fireEvent.click(screen.getByRole("tab", { name: "同步已关注节目" }));
      fireEvent.click(screen.getByRole("button", { name: "开始同步" }));

      await waitFor(() => {
        expect(screen.getByText("同步失败：离线")).toBeInTheDocument();
      });
    } finally {
      consoleError.mockRestore();
    }
  });

  it("shows an error instead of completion when sync stream ends early", async () => {
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    syncPodcastsMetadataSSE.mockImplementation(async (onProgress) => {
      onProgress("progress", "已开始同步", 1, 10);
      throw new Error("同步连接提前结束，未收到完成确认");
    });

    try {
      render(<ImportPage />);

      fireEvent.click(screen.getByRole("tab", { name: "同步已关注节目" }));
      fireEvent.click(screen.getByRole("button", { name: "开始同步" }));

      await waitFor(() => {
        expect(screen.getByText("已开始同步")).toBeInTheDocument();
      });
      await waitFor(() => {
        expect(
          screen.getByText("同步失败：同步连接提前结束，未收到完成确认"),
        ).toBeInTheDocument();
      });
      expect(screen.queryByText("同步已完成")).not.toBeInTheDocument();
    } finally {
      consoleError.mockRestore();
    }
  });

  it("keeps completed sync logs labeled as sync logs after switching tabs", async () => {
    const syncRun = deferred();
    syncPodcastsMetadataSSE.mockReturnValue(syncRun.promise);

    render(<ImportPage />);

    fireEvent.click(screen.getByRole("tab", { name: "同步已关注节目" }));
    fireEvent.click(screen.getByRole("button", { name: "开始同步" }));

    await act(async () => {
      syncRun.resolve();
      await syncRun.promise;
    });

    await waitFor(() => {
      expect(screen.getByText("同步已完成")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("tab", { name: "导入 OPML" }));

    expect(
      screen.getByRole("heading", { name: "同步日志" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "导入日志" }),
    ).not.toBeInTheDocument();
  });

  it("keeps a manually paused log position while new logs arrive", async () => {
    let pushLog:
      | ((
          type: string,
          message: string,
          current?: number,
          total?: number,
        ) => void)
      | undefined;
    const syncRun = deferred();

    syncPodcastsMetadataSSE.mockImplementation((onProgress) => {
      pushLog = onProgress;
      onProgress("progress", "第一条日志", 1, 3);
      onProgress("progress", "第二条日志", 2, 3);
      return syncRun.promise;
    });

    render(<ImportPage />);

    fireEvent.click(screen.getByRole("tab", { name: "同步已关注节目" }));
    fireEvent.click(screen.getByRole("button", { name: "开始同步" }));

    await waitFor(() => {
      expect(screen.getByText("第一条日志")).toBeInTheDocument();
    });

    const logContainer = screen.getByLabelText("同步日志内容");
    Object.defineProperty(logContainer, "clientHeight", {
      value: 200,
      configurable: true,
    });
    Object.defineProperty(logContainer, "scrollHeight", {
      value: 800,
      configurable: true,
    });
    Object.defineProperty(logContainer, "scrollTop", {
      value: 160,
      writable: true,
      configurable: true,
    });

    fireEvent.scroll(logContainer);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "恢复自动滚动" }),
      ).toBeInTheDocument();
    });

    await act(async () => {
      pushLog?.("progress", "第三条日志", 3, 3);
    });

    await waitFor(() => {
      expect(screen.getByText("第三条日志")).toBeInTheDocument();
    });
    expect(logContainer.scrollTop).toBe(160);

    await act(async () => {
      syncRun.resolve();
      await syncRun.promise;
    });
  });

  it("clears restored logs and removes the saved copy", async () => {
    localStorage.setItem(
      "syncLogs",
      JSON.stringify([
        { id: "1", type: "success", message: "ok", timestamp: "12:00:00" },
      ]),
    );

    render(<ImportPage />);

    await waitFor(() => {
      expect(screen.getByText("全部 (1)")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "清空日志" }));

    await waitFor(() => {
      expect(screen.queryByText("全部 (1)")).not.toBeInTheDocument();
    });
    expect(localStorage.getItem("syncLogs")).toBeNull();
    expect(localStorage.getItem("syncLogMode")).toBeNull();
  });
});
