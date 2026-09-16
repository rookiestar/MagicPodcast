import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ImportOpmlPanel from "../ImportOpmlPanel";
import SyncLogStats from "../SyncLogStats";
import { computeSyncStats } from "@/lib/syncLogState";
import { countRetryableEntries } from "@/hooks/useImportSyncOperations";
import type { ImportEntryResult, ImportPreview, ImportTask } from "@/lib/api/importTasks";

// 面板内嵌的本批新增区块会随任务终态拉取数据；面板测试不覆盖该区块细节，
// 统一以空清单应答（细节见 NewPodcastsSection.test.tsx）。
vi.mock("@/lib/api/importTasks", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/importTasks")>();
  return {
    ...actual,
    importTasksApi: {
      ...actual.importTasksApi,
      fetchTaskNewPodcasts: vi
        .fn()
        .mockResolvedValue({ success: true, task_id: 7, total: 0, podcasts: [] }),
    },
  };
});

const baseProps = {
  file: null,
  disabled: false,
  importing: false,
  preview: null,
  previewLoading: false,
  previewError: null as string | null,
  canSubmitImport: false,
  confirmedUrls: {} as Record<string, boolean>,
  lastTask: null as ImportTask | null,
  taskEntries: [] as ImportEntryResult[],
  latestTaskError: false,
  onFileChange: vi.fn(),
  onImport: vi.fn(),
  onToggleConfirmed: vi.fn(),
  onConfirmAllPending: vi.fn(),
  onRetry: vi.fn(),
  onRetryPreview: vi.fn(),
  onRetryLatestTask: vi.fn(),
  countRetryableEntries,
};

function renderPanel(disabled: boolean, overrides: Partial<typeof baseProps> = {}) {
  return render(<ImportOpmlPanel {...baseProps} disabled={disabled} {...overrides} />);
}

describe("ImportOpmlPanel", () => {
  it("marks the file picker as unavailable while an operation is running", () => {
    renderPanel(true);

    const picker = screen.getByText("选择 OPML 文件").closest("label");
    expect(picker).toHaveAttribute("aria-disabled", "true");
    expect(picker).toHaveClass("is-disabled");
    expect(screen.getByLabelText("选择 OPML 文件")).toBeDisabled();
  });

  it("keeps the file picker interactive when no operation is running", () => {
    renderPanel(false);

    const picker = screen.getByText("选择 OPML 文件").closest("label");
    expect(picker).not.toHaveAttribute("aria-disabled", "true");
    expect(picker).not.toHaveClass("is-disabled");
    expect(screen.getByLabelText("选择 OPML 文件")).not.toBeDisabled();
  });

  it("mentions the supported formats and the 8MB limit before a file is chosen", () => {
    renderPanel(false);
    expect(screen.getByText(/支持 \.opml 与 \.xml 文件，最大 8MB/)).toBeDefined();
  });
});

describe("ImportOpmlPanel submit constraint (#427 AC2)", () => {
  it("keeps the primary action disabled with a hint while preview has not succeeded", () => {
    renderPanel(false, { file: new File(["<opml/>"], "a.opml"), canSubmitImport: false });

    const button = screen.getByRole("button", { name: "开始导入" });
    expect(button).toBeDisabled();
    expect(
      screen.getByText(/等待当前文件的预览完成后可开始导入/),
    ).toBeDefined();
  });

  it("shows the loading hint while the preview is in flight", () => {
    renderPanel(false, {
      file: new File(["<opml/>"], "a.opml"),
      previewLoading: true,
    });

    expect(
      screen.getByText(/正在核对当前文件差异，预览完成后可开始导入/),
    ).toBeDefined();
  });

  it("enables the primary action only with a successful preview", () => {
    renderPanel(false, {
      file: new File(["<opml/>"], "a.opml"),
      canSubmitImport: true,
      preview: {
        total: 1,
        entries: [{ xml_url: "https://example.com/a.xml", title: "A", kind: "new" }],
        new_count: 1,
        existing_count: 0,
        collection_count: 0,
        deleted_count: 0,
        invalid_count: 0,
        duplicate_merged_count: 0,
      },
    });

    expect(screen.getByRole("button", { name: "开始导入" })).toBeEnabled();
  });

  it("offers a direct preview retry bound to the current file after a failure", () => {
    const onRetryPreview = vi.fn();
    renderPanel(false, {
      file: new File(["<opml/>"], "a.opml"),
      previewError: "预览服务暂时不可用",
      onRetryPreview,
    });

    expect(screen.getByText("预览服务暂时不可用")).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: /重试预览「a\.opml」/ }));
    expect(onRetryPreview).toHaveBeenCalledTimes(1);
    expect(
      screen.getByText(/预览失败，重试成功后才能开始导入/),
    ).toBeDefined();
  });
});

describe("ImportOpmlPanel preview detail (#426 AC4)", () => {
  const preview: ImportPreview = {
    total: 3,
    entries: [
      { xml_url: "https://example.com/a.xml", title: "新节目", kind: "new" },
      {
        xml_url: "https://www.xiaoyuzhoufm.com/podcast/629bf20356b3d7e17a71cfa1",
        title: "清单节目",
        kind: "collection",
        podcast_title: "本地清单节目",
        reason: "确认后转为关注并绑定订阅地址",
      },
      { xml_url: "not-a-url", title: "无效", kind: "invalid", reason: "需要完整的 HTTP 或 HTTPS 链接" },
    ],
    new_count: 1,
    existing_count: 0,
    collection_count: 1,
    deleted_count: 0,
    invalid_count: 1,
    duplicate_merged_count: 1,
  };

  it("renders category counts with the deduplicated total", () => {
    renderPanel(false, { preview });

    expect(screen.getByLabelText("导入预览")).toHaveTextContent("共 3 条");
    expect(screen.getByLabelText("导入预览")).toHaveTextContent(
      "文件内重复已归并 1 条",
    );
    expect(screen.getByText(/以下 1 条需要确认/)).toBeDefined();
  });

  it("lists per-category detail entries with url and reason", () => {
    renderPanel(false, { preview });

    const details = screen.getByTestId("preview-details");
    expect(details).toHaveTextContent("新增");
    expect(details).toHaveTextContent("明细（1 条）");
    expect(details).toHaveTextContent("https://example.com/a.xml");
    expect(details).toHaveTextContent("需要完整的 HTTP 或 HTTPS 链接");
  });

  it("states preview ownership so an old task is not mixed with the new file", () => {
    renderPanel(false, {
      file: new File(["<opml/>"], "fresh.opml"),
      canSubmitImport: true,
      preview,
    });

    expect(screen.getByText(/以上为「fresh\.opml」的预览/)).toBeDefined();
  });

  it("shows an explicit zero-entry note for empty OPML files", () => {
    renderPanel(false, {
      preview: {
        ...preview,
        total: 0,
        entries: [],
        new_count: 0,
        collection_count: 0,
        invalid_count: 0,
      },
    });

    expect(
      screen.getByText(/文件中没有订阅条目（0 条），导入不会新增节目/),
    ).toBeDefined();
  });
});

describe("ImportOpmlPanel task banner", () => {
  const completedTask: ImportTask = {
    id: 7,
    status: "completed",
    file_name: "subs.opml",
    total: 4,
    processed: 4,
    success_count: 2,
    pending_count: 1,
    conflict_count: 1,
    merged_count: 0,
    unchanged_count: 0,
    skipped_count: 0,
    failed_count: 0,
    error_message: "",
    started_at: "2026-09-14T00:00:00Z",
    finished_at: "2026-09-14T00:01:00Z",
  };

  it("offers retry only for failed/pending entries of a completed task", () => {
    renderPanel(false, { lastTask: completedTask, taskEntries: [{ title: "pending", feed_url: "a", outcome: "pending" }] });

    expect(screen.getByText(/上次导入任务 #7 · 已完成/)).toBeDefined();
    expect(screen.getByText(/仅重试失败\/待同步条目（1 条）/)).toBeDefined();
  });

  it("attributes the old task to its own file", () => {
    renderPanel(false, { lastTask: completedTask });

    expect(screen.getByText(/来源文件「subs\.opml」/)).toBeDefined();
  });

  it("gives failed tasks a retry entry when retryable entries exist (#427)", () => {
    const failedTask: ImportTask = {
      ...completedTask,
      id: 8,
      status: "failed",
      failed_count: 2,
      pending_count: 1,
      error_message: "上游抓取中断",
    };
    renderPanel(false, { lastTask: failedTask, taskEntries: [
      { title: "a", feed_url: "a", outcome: "failed" },
      { title: "b", feed_url: "b", outcome: "failed" },
      { title: "c", feed_url: "c", outcome: "pending" },
    ] });

    expect(screen.getByText(/上次导入任务 #8 · 失败/)).toBeDefined();
    expect(screen.getByText(/失败原因：上游抓取中断/)).toBeDefined();
    expect(screen.getByText(/仅重试失败\/待同步条目（3 条）/)).toBeDefined();
  });

  it("does not offer retry while the task is running", () => {
    const runningTask: ImportTask = {
      ...completedTask,
      id: 9,
      status: "running",
      failed_count: 1,
      pending_count: 1,
    };
    renderPanel(false, { lastTask: runningTask });

    expect(screen.queryByRole("button", { name: /仅重试/ })).toBeNull();
  });

  it("counts interrupted unprocessed entries as continuable", () => {
    const interruptedTask: ImportTask = {
      ...completedTask,
      id: 10,
      status: "interrupted",
      total: 5,
      processed: 3,
      failed_count: 0,
      pending_count: 0,
    };
    renderPanel(false, { lastTask: interruptedTask, taskEntries: [
      { title: "a", feed_url: "a", outcome: "unprocessed" },
      { title: "b", feed_url: "b", outcome: "unprocessed" },
    ] });

    expect(
      screen.getByRole("button", { name: /继续未完成条目（2 条）/ }),
    ).toBeDefined();
  });

  it("shows a visible read failure with retry while keeping the last known task", () => {
    const onRetryLatestTask = vi.fn();
    renderPanel(false, {
      lastTask: completedTask,
      latestTaskError: true,
      onRetryLatestTask,
    });

    expect(
      screen.getByText(/最近导入任务状态读取失败/),
    ).toBeDefined();
    expect(screen.getByText(/最后一次成功读取的任务结果/)).toBeDefined();
    fireEvent.click(screen.getByRole("button", { name: "重试读取" }));
    expect(onRetryLatestTask).toHaveBeenCalledTimes(1);
  });

  it("distinguishes no history from a read failure", () => {
    renderPanel(false, { lastTask: null, latestTaskError: true });

    expect(screen.getByText(/最近导入任务记录读取失败/)).toBeDefined();
    expect(screen.queryByText(/上次导入任务/)).toBeNull();
  });

  it("shows complete results and lets the user confirm a single identity conflict", () => {
    const entry = {title:"换址节目", feed_url:"https://example.com/new", outcome:"conflict", podcast_id:42, detail:"稳定身份匹配"};
    const onRetry = vi.fn();
    render(<ImportOpmlPanel {...baseProps} lastTask={completedTask} taskEntries={[entry]} onRetry={onRetry} />);
    fireEvent.click(screen.getByText("逐项结果（1 条）"));
    expect(screen.getByText(entry.feed_url)).toBeDefined();
    fireEvent.click(screen.getByRole("button", {name:"核对并确认关联"}));
    expect(onRetry).toHaveBeenCalledWith(entry);
  });

  it("filters per-entry results by outcome with pressed semantics", () => {
    const entries = [
      { title: "A", feed_url: "https://example.com/a", outcome: "failed" },
      { title: "B", feed_url: "https://example.com/b", outcome: "pending" },
      { title: "C", feed_url: "https://example.com/c", outcome: "new" },
    ];
    render(<ImportOpmlPanel {...baseProps} lastTask={completedTask} taskEntries={entries} />);

    fireEvent.click(screen.getByText("逐项结果（3 条）"));
    const failedChip = screen.getByRole("button", { name: "失败（1）" });
    expect(failedChip).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(failedChip);
    expect(failedChip).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("A · 失败")).toBeDefined();
    expect(screen.queryByText("B · 待同步")).toBeNull();
    expect(screen.queryByText("C · 新增")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "全部（3）" }));
    expect(screen.getByText("B · 待同步")).toBeDefined();
    expect(screen.getByText("C · 新增")).toBeDefined();
  });
});

describe("countRetryableEntries", () => {
  const anyTask: ImportTask = {
    id: 1,
    status: "completed",
    file_name: "t.opml",
    total: 3,
    processed: 3,
    success_count: 0,
    pending_count: 0,
    conflict_count: 0,
    merged_count: 0,
    unchanged_count: 0,
    skipped_count: 0,
    failed_count: 0,
    error_message: "",
    started_at: "2026-09-14T00:00:00Z",
  };

  it("matches the server contract for failed, pending and interrupted tasks", () => {
    const failed = { ...anyTask, status: "failed" as const, failed_count: 2, pending_count: 1 };
    expect(countRetryableEntries(failed, [])).toBe(0);

    const interrupted = {
      ...anyTask,
      status: "interrupted" as const,
      total: 5,
      processed: 3,
      failed_count: 0,
      pending_count: 0,
    };
    expect(countRetryableEntries(interrupted, [])).toBe(0);

    const withEntries = countRetryableEntries(anyTask, [
      { title: "a", feed_url: "a", outcome: "failed" },
      { title: "b", feed_url: "b", outcome: "conflict" },
      { title: "c", feed_url: "c", outcome: "new" },
    ]);
    expect(withEntries).toBe(1);
  });
});

it("shows pending imports separately from failures and skipped subscriptions", () => {
  const stats = computeSyncStats([{id: "summary", type: "summary", message: "导入完成", timestamp: "12:00:00", data: {operation: "import", total_podcasts: 2, success_podcasts: 0, failed_podcasts: 0, skipped_podcasts: 0, stub_podcasts: 2}}]);
  render(<SyncLogStats stats={stats} />);
  expect(screen.getByText("待同步").parentElement).toHaveTextContent("2");
  expect(screen.getByText("跳过").parentElement).toHaveTextContent("0");
  expect(screen.getByText("失败").parentElement).toHaveTextContent("0");
});

it("breaks out merged, conflict, and unchanged counts from the summary", () => {
  const stats = computeSyncStats([{id: "summary", type: "summary", message: "导入完成", timestamp: "12:00:00", data: {
    operation: "import",
    total_podcasts: 6,
    success_podcasts: 1,
    failed_podcasts: 0,
    skipped_podcasts: 0,
    stub_podcasts: 2,
    merged_podcasts: 1,
    conflict_podcasts: 1,
    unchanged_podcasts: 1,
  }}]);
  render(<SyncLogStats stats={stats} />);
  expect(screen.getByText("确认关联").parentElement).toHaveTextContent("1");
  expect(screen.getByText("冲突跳过").parentElement).toHaveTextContent("1");
  expect(screen.getByText("未变化").parentElement).toHaveTextContent("1");
  expect(screen.getByText("待同步").parentElement).toHaveTextContent("2");
});
