import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ImportOpmlPanel from "../ImportOpmlPanel";
import SyncLogStats from "../SyncLogStats";
import { computeSyncStats } from "@/lib/syncLogState";
import type { ImportPreview } from "@/lib/api/importTasks";

const baseProps = {
  file: null,
  disabled: false,
  importing: false,
  preview: null,
  previewLoading: false,
  previewError: null as string | null,
  confirmedUrls: {} as Record<string, boolean>,
  lastTask: null,
  onFileChange: vi.fn(),
  onImport: vi.fn(),
  onToggleConfirmed: vi.fn(),
  onConfirmAllPending: vi.fn(),
  onRetry: vi.fn(),
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
});

describe("ImportOpmlPanel preview", () => {
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
  const completedTask = {
    id: 7,
    status: "completed" as const,
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
    renderPanel(false, { lastTask: completedTask });

    expect(screen.getByText(/上次导入任务 #7 · 已完成/)).toBeDefined();
    expect(screen.getByText(/仅重试失败\/待同步条目（1 条）/)).toBeDefined();
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
