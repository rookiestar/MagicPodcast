import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ImportPage from "../page";
import { syncApi } from "@/lib/api";
import { toast } from "@/lib/toast";

vi.mock("@/components/layout/PageLayout", () => ({
  default: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/lib/api", () => ({
  syncApi: {
    importOPMLSSE: vi.fn(),
    syncPodcastsMetadataSSE: vi.fn(),
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

  it("restores the saved log operation so the title matches the log content", async () => {
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
    expect(screen.getByRole("tab", { name: "同步元数据" })).toHaveAttribute(
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

    fireEvent.click(screen.getByRole("tab", { name: "同步元数据" }));
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

    fireEvent.click(screen.getByRole("tab", { name: "同步元数据" }));
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
      onProgress("success", "导入完成：已处理 1 个播客");
    });

    render(<ImportPage />);

    const fileInput = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    const file = new File(["<opml></opml>"], "feeds.opml", {
      type: "text/opml",
    });

    fireEvent.change(fileInput, { target: { files: [file] } });
    fireEvent.click(screen.getByRole("button", { name: "开始导入" }));

    await waitFor(() => {
      expect(screen.getByText("导入完成：已处理 1 个播客")).toBeInTheDocument();
    });
    expect(screen.queryByText("导入完成")).not.toBeInTheDocument();
  });

  it("shows sync errors even when the thrown value is not an Error", async () => {
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});
    syncPodcastsMetadataSSE.mockRejectedValue("离线");

    try {
      render(<ImportPage />);

      fireEvent.click(screen.getByRole("tab", { name: "同步元数据" }));
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

      fireEvent.click(screen.getByRole("tab", { name: "同步元数据" }));
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

    fireEvent.click(screen.getByRole("tab", { name: "同步元数据" }));
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

    fireEvent.click(screen.getByRole("tab", { name: "同步元数据" }));
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
